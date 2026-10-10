package newapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/guliansheng/gateway-ops/internal/connector"
)

func TestCheckAuthUsesCustomAccessTokenHeaders(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer permanent-token" {
			t.Fatalf("Authorization = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"message":"","data":{"id":1}}`))
	}))
	defer server.Close()

	err := New().CheckAuth(context.Background(), &connector.Channel{SiteURL: server.URL}, &connector.AuthSession{
		AccessToken: "permanent-token",
		Headers:     map[string]string{"Authorization": "Bearer permanent-token"},
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestListUsageParsesFirstResponseTimeFromOther(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/status":
			_, _ = w.Write([]byte(`{"success":true,"message":"","data":{"quota_per_unit":500000}}`))
		case "/api/log/self":
			_, _ = w.Write([]byte(`{"success":true,"message":"","data":{"page":1,"page_size":100,"total":3,"items":[
				{"id":1,"created_at":1770000000,"model_name":"gpt-5","quota":500,"prompt_tokens":10,"completion_tokens":2,"use_time":3,"request_id":"req-1","other":"{\"frt\":1778}"},
				{"id":2,"created_at":1770000001,"model_name":"gpt-5","quota":500,"prompt_tokens":10,"completion_tokens":2,"use_time":3,"request_id":"req-2","other":"{\"frt\":-1000}"},
				{"id":3,"created_at":1770000002,"model_name":"gpt-5","quota":500,"prompt_tokens":10,"completion_tokens":2,"use_time":3,"request_id":"req-3","other":"not-json"}
			]}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	page, err := New().ListUsage(context.Background(), &connector.Channel{SiteURL: server.URL}, &connector.AuthSession{
		Headers: map[string]string{"Authorization": "Bearer permanent-token"},
	}, connector.UsageQuery{Page: 1, PageSize: 100})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Records) != 3 {
		t.Fatalf("records = %d, want 3", len(page.Records))
	}
	if page.Records[0].FirstTokenMS == nil || *page.Records[0].FirstTokenMS != 1778 {
		t.Fatalf("first_token_ms = %v, want 1778", page.Records[0].FirstTokenMS)
	}
	if page.Records[1].FirstTokenMS != nil {
		t.Fatalf("negative frt should be unavailable, got %d", *page.Records[1].FirstTokenMS)
	}
	if page.Records[2].FirstTokenMS != nil {
		t.Fatalf("invalid other JSON should be unavailable, got %d", *page.Records[2].FirstTokenMS)
	}
}
