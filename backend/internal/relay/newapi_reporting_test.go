package relay

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"
)

func newAPIReportingServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.Header.Get("Authorization") != "Bearer reporting-token" || r.Header.Get("New-Api-User") != "7" {
			t.Errorf("unexpected request: %s %s, headers=%v", r.Method, r.URL, r.Header)
		}
		w.Header().Set("Content-Type", "application/json")
		handler(w, r)
	}))
	t.Cleanup(server.Close)
	return server
}

func newAPIReportingJSON(t *testing.T, w http.ResponseWriter, data any) {
	t.Helper()
	if err := json.NewEncoder(w).Encode(map[string]any{"success": true, "message": "", "data": data}); err != nil {
		t.Error(err)
	}
}

func newAPIReportingCredential() newAPICredential {
	return newAPICredential{Token: "reporting-token", UserID: "7"}
}

func TestNewAPIReportingRequestErrors(t *testing.T) {
	created := time.Date(2026, 10, 14, 1, 2, 3, 0, time.UTC)
	query := url.Values{
		"page": {"2"}, "page_size": {"2"}, "type": {"2"},
		"group_id": {strconv.FormatInt(newAPIGroupID("vip"), 10)},
		"model":    {"claude-test"}, "account_id": {"12"}, "request_id": {"req-1"},
		"start_time": {"2026-10-14T09:00:00.123+08:00"}, "end_time": {"2026-10-14T09:30:00+08:00"},
	}
	before := query.Encode()
	server := newAPIReportingServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/gateway/api/group/":
			newAPIReportingJSON(t, w, []string{"default", "vip"})
		case "/gateway/api/log/":
			want := url.Values{
				"p": {"2"}, "page_size": {"2"}, "type": {"5"}, "group": {"vip"},
				"model_name": {"claude-test"}, "channel": {"12"}, "request_id": {"req-1"},
				"start_timestamp": {"1791939600"}, "end_timestamp": {"1791941400"},
			}
			if !reflect.DeepEqual(r.URL.Query(), want) {
				t.Errorf("log query = %v, want %v", r.URL.Query(), want)
			}
			newAPIReportingJSON(t, w, map[string]any{
				"page": 2, "page_size": 2, "total": 5,
				"items": []map[string]any{
					{"id": 31, "type": 5, "created_at": created.Unix(), "model_name": "claude-test", "group": "vip", "channel": 12, "request_id": "req-1", "use_time": 2,
						"content": "status_code=429, rate limit reached", "other": `{"status_code":429,"error_type":"openai_error","error_code":"rate_limit_exceeded","frt":123,"upstream_model_name":"claude-upstream"}`},
					{"id": 30, "type": 5, "created_at": created.Add(-time.Second).Unix(), "model_name": "claude-test", "group": "vip", "request_id": "req-2", "use_time": 1,
						"content": "status_code=401, invalid credential", "other": `{"status_code":401,"error_type":"openai_error"}`},
				},
			})
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
			http.NotFound(w, r)
		}
	})
	var got remoteRequestErrorPage
	if err := NewService(nil, nil).newAPIRequestErrors(context.Background(), server.URL+"/gateway", newAPIReportingCredential(), query, &got); err != nil {
		t.Fatal(err)
	}
	if got.Page != 2 || got.PageSize != 2 || got.Pages != 3 || got.Total != 5 || len(got.Items) != 2 {
		t.Fatalf("page = %#v", got)
	}
	item := got.Items[0]
	if item.RequestID != "req-1" || item.StatusCode != 429 || item.CreatedAt != created.Format(time.RFC3339) || item.Model != "claude-test" || item.RequestedModel != "claude-test" || item.UpstreamModel != "claude-upstream" || item.GroupID != newAPIGroupID("vip") {
		t.Fatalf("error = %#v", item)
	}
	if item.ErrorOwner != "provider" || item.ErrorPhase != "upstream" || item.Message != "status_code=429, rate limit reached" || !strings.Contains(item.Type, "rate_limit_exceeded") {
		t.Fatalf("error metadata = %#v", item)
	}
	if item.DurationMS == nil || *item.DurationMS != 2000 || item.TimeToFirstTokenMS == nil || *item.TimeToFirstTokenMS != 123 {
		t.Fatalf("timing = %#v", item)
	}
	events := errorMonitorEvents(got.Items, newAPIGroupID("vip"))
	if len(events) != 2 || events[0].failureReason != "\u8bf7\u6c42\u9891\u7387\u6216\u5e76\u53d1\u5df2\u8fbe\u4e0a\u9650" || !events[0].createdAt.Equal(created) {
		t.Fatalf("monitor events = %#v", events)
	}
	if query.Encode() != before {
		t.Fatalf("input query changed: %v", query)
	}
}

func TestNewAPIReportingRequestErrorStatusFallbacks(t *testing.T) {
	tests := []struct {
		name, other, content string
		status               int
	}{
		{"metadata", `{"status_code":503}`, "status_code=401, unavailable", 503},
		{"string_status", `{"status_code":"429"}`, "rate limit", 429},
		{"content", `{}`, "status_code=504, gateway timeout", 504},
		{"malformed_other", `{invalid`, "status_code=403, denied", 403},
		{"legacy_content", `{}`, "upstream returned status code: 401", 401},
		{"unknown", `{}`, "upstream failed", 502},
		{"disconnect", `{"status_code":499}`, "client disconnected", 499},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := newAPIReportingServer(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/api/log/" || r.URL.Query().Get("type") != "5" {
					t.Errorf("unexpected URL: %s", r.URL)
				}
				newAPIReportingJSON(t, w, map[string]any{"total": 1, "items": []map[string]any{
					{"id": 1, "type": 5, "created_at": 1791939600, "model_name": "gpt-test", "group": "default", "content": tt.content, "other": tt.other},
				}})
			})
			var got remoteRequestErrorPage
			if err := NewService(nil, nil).newAPIRequestErrors(context.Background(), server.URL, newAPIReportingCredential(), nil, &got); err != nil {
				t.Fatal(err)
			}
			if len(got.Items) != 1 || got.Items[0].StatusCode != tt.status || got.Page != 1 || got.Pages != 1 || got.PageSize != 100 {
				t.Fatalf("page = %#v, want status %d", got, tt.status)
			}
			if tt.status == 499 && len(errorMonitorEvents(got.Items, newAPIGroupID("default"))) != 0 {
				t.Fatal("client disconnect counted as a provider failure")
			}
		})
	}
}

func TestNewAPIReportingRequestErrorsSkipNonErrors(t *testing.T) {
	server := newAPIReportingServer(t, func(w http.ResponseWriter, r *http.Request) {
		newAPIReportingJSON(t, w, map[string]any{"total": 0, "items": []map[string]any{{"type": 2}, {"type": 1}}})
	})
	var got remoteRequestErrorPage
	if err := NewService(nil, nil).newAPIRequestErrors(context.Background(), server.URL, newAPIReportingCredential(), nil, &got); err != nil {
		t.Fatal(err)
	}
	if got.Items == nil || len(got.Items) != 0 || got.Pages != 1 {
		t.Fatalf("page = %#v", got)
	}
}

func TestNewAPIReportingDefaultModelPricing(t *testing.T) {
	tests := []struct {
		name string
		item string
		want remoteDefaultModelPricing
	}{
		{"token_cache_per_million", `{"model_name":"model-test","quota_type":0,"model_ratio":0.75,"completion_ratio":4,"cache_ratio":0.1,"create_cache_ratio":1.25,"image_ratio":2,"owner_by":"anthropic"}`,
			remoteDefaultModelPricing{Found: true, Provider: "anthropic", BillingMode: "token", InputPrice: floatPointer(1.5e-6), OutputPrice: floatPointer(6e-6), CacheReadPrice: floatPointer(0.15e-6), CacheWritePrice: floatPointer(1.875e-6), ImageInputPrice: floatPointer(3e-6)}},
		{"no_cache", `{"model_name":"model-test","quota_type":0,"model_ratio":1,"completion_ratio":2}`,
			remoteDefaultModelPricing{Found: true, BillingMode: "token", InputPrice: floatPointer(2e-6), OutputPrice: floatPointer(4e-6)}},
		{"free_cache", `{"model_name":"model-test","quota_type":0,"model_ratio":1,"completion_ratio":0,"cache_ratio":0,"create_cache_ratio":0}`,
			remoteDefaultModelPricing{Found: true, BillingMode: "token", InputPrice: floatPointer(2e-6), OutputPrice: floatPointer(0), CacheReadPrice: floatPointer(0), CacheWritePrice: floatPointer(0)}},
		{"free_model", `{"model_name":"model-test","quota_type":0,"model_ratio":0,"completion_ratio":2}`,
			remoteDefaultModelPricing{Found: true, BillingMode: "token", InputPrice: floatPointer(0), OutputPrice: floatPointer(0)}},
		{"per_request", `{"model_name":"model-test","quota_type":1,"model_price":0.023,"model_ratio":999,"completion_ratio":999,"cache_ratio":1}`,
			remoteDefaultModelPricing{Found: true, BillingMode: "per_request", PerRequestPrice: floatPointer(0.023)}},
		{"free_request", `{"model_name":"model-test","quota_type":1,"model_price":0}`,
			remoteDefaultModelPricing{Found: true, BillingMode: "per_request", PerRequestPrice: floatPointer(0)}},
		{"missing", `{"model_name":"another-model","quota_type":0,"model_ratio":1,"completion_ratio":2}`, remoteDefaultModelPricing{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := newAPIReportingServer(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/api/pricing" {
					t.Errorf("unexpected path: %s", r.URL.Path)
				}
				newAPIReportingJSON(t, w, json.RawMessage("["+tt.item+"]"))
			})
			got := remoteDefaultModelPricing{Found: true, InputPrice: floatPointer(999)}
			if err := NewService(nil, nil).newAPIDefaultModelPricing(context.Background(), server.URL, newAPIReportingCredential(), "model-test", &got); err != nil {
				t.Fatal(err)
			}
			if got.Found != tt.want.Found || got.Provider != tt.want.Provider || got.BillingMode != tt.want.BillingMode {
				t.Fatalf("pricing = %#v, want %#v", got, tt.want)
			}
			for name, pair := range map[string][2]*float64{
				"input": {got.InputPrice, tt.want.InputPrice}, "output": {got.OutputPrice, tt.want.OutputPrice},
				"cache_read": {got.CacheReadPrice, tt.want.CacheReadPrice}, "cache_write": {got.CacheWritePrice, tt.want.CacheWritePrice},
				"image_input": {got.ImageInputPrice, tt.want.ImageInputPrice}, "per_request": {got.PerRequestPrice, tt.want.PerRequestPrice},
			} {
				if (pair[0] == nil) != (pair[1] == nil) || pair[0] != nil && math.Abs(*pair[0]-*pair[1]) > 1e-14 {
					t.Errorf("%s price = %v, want %v; pricing=%#v", name, pair[0], pair[1], got)
				}
			}
		})
	}
}

func TestNewAPIReportingUserBalanceHistory(t *testing.T) {
	query := url.Values{"page": {"2"}, "page_size": {"2"}, "user_id": {"99"}, "username": {"another-user"}, "type": {"2"}}
	before := query.Encode()
	visits := map[string]int{}
	server := newAPIReportingServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/user/42":
			newAPIReportingJSON(t, w, map[string]any{"id": 42, "username": "alice+vip", "email": "alice@example.com", "role": 1, "status": 1, "quota": 250000, "created_time": 1791939600})
		case "/api/status":
			newAPIReportingJSON(t, w, map[string]any{"quota_per_unit": 100000})
		case "/api/log/":
			q := r.URL.Query()
			if q.Get("username") != "alice+vip" || q.Has("user_id") || q.Get("type") != "1" || q.Get("page_size") != "2" {
				t.Errorf("log query = %v", q)
			}
			visits[q.Get("p")]++
			var logs []map[string]any
			for _, log := range []map[string]any{
				{"id": 5, "type": 1, "user_id": 42, "username": "alice+vip", "quota": 100000, "created_at": 1791939605, "content": "\u5145\u503c\u4e94"},
				{"id": 4, "type": 1, "user_id": 42, "username": "alice+vip", "quota": 200000, "created_at": 1791939604, "content": "\u5145\u503c\u56db"},
				{"id": 3, "type": 1, "user_id": 42, "username": "alice+vip", "quota": 300000, "created_at": 1791939603, "content": "\u5145\u503c\u4e09"},
				{"id": 2, "type": 1, "user_id": 42, "username": "alice+vip", "quota": 400000, "created_at": 1791939602, "content": "\u5145\u503c\u4e8c"},
				{"id": 1, "type": 1, "user_id": 42, "username": "alice+vip", "quota": 500000, "created_at": 1791939601, "content": "\u5145\u503c\u4e00"},
			} {
				id := log["id"].(int)
				page, _ := strconv.Atoi(q.Get("p"))
				if (5-id)/2+1 == page {
					logs = append(logs, log)
				}
			}
			newAPIReportingJSON(t, w, map[string]any{"page_size": 2, "total": 5, "items": logs})
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
			http.NotFound(w, r)
		}
	})
	var got UserBalanceHistory
	if err := NewService(nil, nil).newAPIUserBalanceHistory(context.Background(), server.URL, newAPIReportingCredential(), 42, query, &got); err != nil {
		t.Fatal(err)
	}
	if got.Page != 2 || got.Pages != 3 || got.Total != 5 || got.TotalRecharged != 15 || len(got.Items) != 2 {
		t.Fatalf("history = %#v", got)
	}
	if got.User.ID != 42 || got.User.Username != "alice+vip" || got.User.Balance != 2.5 || got.User.Email != "alice@example.com" {
		t.Fatalf("user = %#v", got.User)
	}
	if got.Items[0].ID != 3 || got.Items[0].Type != "recharge" || got.Items[0].Value != 3 || got.Items[0].Notes != "\u5145\u503c\u4e09" || !got.Items[0].CreatedAt.Equal(time.Unix(1791939603, 0).UTC()) || got.Items[1].ID != 2 {
		t.Fatalf("items = %#v", got.Items)
	}
	if !reflect.DeepEqual(visits, map[string]int{"1": 1, "2": 1, "3": 1}) || query.Encode() != before {
		t.Fatalf("visits=%v, query=%v", visits, query)
	}
}

func TestNewAPIReportingUserBalanceHistoryFiltersOtherLogs(t *testing.T) {
	server := newAPIReportingServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/user/42":
			newAPIReportingJSON(t, w, map[string]any{"id": 42, "username": "alice"})
		case "/api/status":
			newAPIReportingJSON(t, w, map[string]any{"quota_per_unit": 500000})
		case "/api/log/":
			newAPIReportingJSON(t, w, map[string]any{"total": 1, "items": []map[string]any{
				{"id": 1, "type": 1, "user_id": 42, "username": "alice", "quota": 500000},
				{"id": 2, "type": 2, "user_id": 42, "quota": 9000000},
				{"id": 3, "type": 3, "user_id": 42, "quota": 9000000},
				{"id": 4, "type": 1, "user_id": 99, "username": "bob", "quota": 9000000},
			}})
		default:
			http.NotFound(w, r)
		}
	})
	var got UserBalanceHistory
	if err := NewService(nil, nil).newAPIUserBalanceHistory(context.Background(), server.URL, newAPIReportingCredential(), 42, nil, &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Items) != 1 || got.TotalRecharged != 1 {
		t.Fatalf("history = %#v", got)
	}
}

func TestNewAPIReportingUserBalanceHistoryEmpty(t *testing.T) {
	server := newAPIReportingServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/user/42":
			newAPIReportingJSON(t, w, map[string]any{"id": 42, "username": "alice"})
		case "/api/status":
			newAPIReportingJSON(t, w, map[string]any{"quota_per_unit": 500000})
		case "/api/log/":
			newAPIReportingJSON(t, w, map[string]any{"total": 0, "page": 1, "page_size": 15, "items": []any{}})
		}
	})
	var got UserBalanceHistory
	if err := NewService(nil, nil).newAPIUserBalanceHistory(context.Background(), server.URL, newAPIReportingCredential(), 42, nil, &got); err != nil {
		t.Fatal(err)
	}
	if got.Items == nil || len(got.Items) != 0 || got.Total != 0 || got.TotalRecharged != 0 || got.Pages != 1 {
		t.Fatalf("history = %#v", got)
	}
}

func TestNewAPIReportingUserBalanceHistoryRejectsInvalidUser(t *testing.T) {
	for name, user := range map[string]any{
		"missing_username": map[string]any{"id": 42},
		"wrong_user":       map[string]any{"id": 99, "username": "bob"},
		"empty_user":       nil,
	} {
		t.Run(name, func(t *testing.T) {
			server := newAPIReportingServer(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/api/user/42" {
					t.Errorf("invalid user must not query logs or quota: %s", r.URL)
				}
				newAPIReportingJSON(t, w, user)
			})
			if err := NewService(nil, nil).newAPIUserBalanceHistory(context.Background(), server.URL, newAPIReportingCredential(), 42, nil, &UserBalanceHistory{}); err == nil {
				t.Fatal("expected invalid user error")
			}
		})
	}
}

func TestNewAPIReportingUserBalanceHistoryLaterPageFailure(t *testing.T) {
	server := newAPIReportingServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/user/42":
			newAPIReportingJSON(t, w, map[string]any{"id": 42, "username": "alice"})
		case "/api/status":
			newAPIReportingJSON(t, w, map[string]any{"quota_per_unit": 500000})
		case "/api/log/":
			if r.URL.Query().Get("p") == "2" {
				w.WriteHeader(http.StatusServiceUnavailable)
				json.NewEncoder(w).Encode(map[string]any{"success": false, "message": "unavailable"})
				return
			}
			newAPIReportingJSON(t, w, map[string]any{"total": 2, "page_size": 1, "items": []map[string]any{{"id": 1, "type": 1, "user_id": 42, "quota": 500000}}})
		}
	})
	got := UserBalanceHistory{TotalRecharged: 999}
	err := NewService(nil, nil).newAPIUserBalanceHistory(context.Background(), server.URL, newAPIReportingCredential(), 42, url.Values{"page_size": {"1"}}, &got)
	if err == nil || !strings.Contains(err.Error(), "503") || got.TotalRecharged != 999 {
		t.Fatalf("error=%v, history=%#v", err, got)
	}
}

func TestNewAPIReportingPropagatesUpstreamErrors(t *testing.T) {
	for _, response := range []string{`{"success":false,"message":"access denied"}`, `{"success":true,"data":"invalid data"}`} {
		server := newAPIReportingServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(response))
		})
		service := NewService(nil, nil)
		for name, call := range map[string]func() error{
			"errors": func() error {
				return service.newAPIRequestErrors(context.Background(), server.URL, newAPIReportingCredential(), nil, &remoteRequestErrorPage{})
			},
			"pricing": func() error {
				return service.newAPIDefaultModelPricing(context.Background(), server.URL, newAPIReportingCredential(), "model", &remoteDefaultModelPricing{})
			},
			"balance": func() error {
				return service.newAPIUserBalanceHistory(context.Background(), server.URL, newAPIReportingCredential(), 42, nil, &UserBalanceHistory{})
			},
		} {
			if err := call(); err == nil {
				t.Errorf("%s: expected upstream error for %s", name, response)
			}
		}
	}
}
