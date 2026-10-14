package relay

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestNewAPICredentialValidation(t *testing.T) {
	if _, err := encodeNewAPICredential(newAPICredential{Username: "admin"}); err == nil {
		t.Fatal("missing password accepted")
	}
	if _, err := encodeNewAPICredential(newAPICredential{Token: "token"}); err == nil {
		t.Fatal("missing user ID accepted")
	}
	raw, err := encodeNewAPICredential(newAPICredential{Token: "token", UserID: "7"})
	if err != nil {
		t.Fatal(err)
	}
	credential, ok := decodeNewAPICredential(raw)
	if !ok || credential.Token != "token" || credential.UserID != "7" {
		t.Fatalf("credential = %#v", credential)
	}
	if _, ok := decodeNewAPICredential("sub2api-key"); ok {
		t.Fatal("legacy key recognized as New API")
	}
}

func TestNewAPICredentialModeSwitchAndBlankSecrets(t *testing.T) {
	old := newAPICredential{AuthMode: "password", Username: "admin", Password: "password", GatewayKey: "gateway"}
	empty := ""
	got := mergeRelayCredential("newapi", old, UpdateInput{Password: &empty})
	if got.Password != "password" {
		t.Fatal("blank secret overwrote password")
	}
	mode, token, userID := "access_token", "token", "7"
	got = mergeRelayCredential("newapi", old, UpdateInput{AuthMode: &mode, AdminToken: &token, AdminUserID: &userID})
	raw, err := encodeNewAPICredential(got)
	if err != nil {
		t.Fatal(err)
	}
	decoded, ok := decodeNewAPICredential(raw)
	if !ok || decoded.Username != "" || decoded.Password != "" || decoded.AuthMode != "access_token" || decoded.Token != "token" || decoded.GatewayKey != "gateway" {
		t.Fatalf("credential = %#v", decoded)
	}
}

func TestNewAPIConcurrentLoginAndExpiry(t *testing.T) {
	var logins atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/nested/api/user/login/encryption-key":
			http.NotFound(w, r)
		case "/nested/api/user/login":
			logins.Add(1)
			http.SetCookie(w, &http.Cookie{Name: "session", Value: "valid"})
			json.NewEncoder(w).Encode(map[string]any{"success": true, "data": map[string]any{"id": 7, "role": 100}})
		case "/nested/api/group/":
			if r.Header.Get("Cookie") != "session=valid" || r.Header.Get("New-Api-User") != "7" {
				w.WriteHeader(401)
				json.NewEncoder(w).Encode(map[string]any{"success": false, "message": "login expired"})
				return
			}
			json.NewEncoder(w).Encode(map[string]any{"success": true, "data": []string{"default"}})
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	service := NewService(nil, nil)
	credential := newAPICredential{Username: "admin", Password: "password"}
	var workers sync.WaitGroup
	for i := 0; i < 10; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			if _, err := service.newAPIRequest(context.Background(), "GET", server.URL+"/nested/api/group/", credential, nil); err != nil {
				t.Error(err)
			}
		}()
	}
	workers.Wait()
	if logins.Load() != 1 {
		t.Fatalf("login count = %d", logins.Load())
	}
	entry := service.newAPISessionEntry(server.URL+"/nested", credential)
	entry.mu.Lock()
	entry.session.Expires = time.Now().Add(-time.Minute)
	entry.mu.Unlock()
	if _, err := service.newAPIRequest(context.Background(), "GET", server.URL+"/nested/api/group/", credential, nil); err != nil {
		t.Fatal(err)
	}
	if logins.Load() != 2 {
		t.Fatalf("login count after expiry = %d", logins.Load())
	}
}

func TestNewAPIRequestDoesNotFollowCredentialRedirect(t *testing.T) {
	var redirected atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		redirected.Add(1)
	}))
	defer target.Close()
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer secret-token" {
			t.Errorf("authorization header = %q", r.Header.Get("Authorization"))
		}
		http.Redirect(w, r, target.URL+"/collect", http.StatusTemporaryRedirect)
	}))
	defer source.Close()

	service := NewService(nil, nil)
	_, err := service.newAPIRequest(context.Background(), http.MethodPost, source.URL+"/api/verify", newAPICredential{Token: "secret-token", UserID: "7"}, map[string]any{"password": "secret-password"})
	if err == nil {
		t.Fatal("redirect response unexpectedly succeeded")
	}
	if redirected.Load() != 0 {
		t.Fatalf("redirect target received %d requests", redirected.Load())
	}
}

func TestNewAPILogFiltersUseExactTimesAndUsername(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/status":
			json.NewEncoder(w).Encode(map[string]any{"success": true, "data": map[string]any{"quota_per_unit": 100}})
		case "/api/user/8":
			json.NewEncoder(w).Encode(map[string]any{"success": true, "data": map[string]any{"id": 8, "username": "alice"}})
		case "/api/log/":
			q := r.URL.Query()
			if q.Get("username") != "alice" || q.Get("start_timestamp") != "1700000060" || q.Get("end_timestamp") != "1700000120" {
				t.Errorf("query = %v", q)
			}
			json.NewEncoder(w).Encode(map[string]any{"success": true, "data": map[string]any{"items": []any{}, "total": 0}})
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	service := NewService(nil, nil)
	_, err := service.newAPILogs(context.Background(), server.URL, newAPICredential{Token: "token", UserID: "7"}, url.Values{"user_id": {"8"}, "start_date": {"2023-11-14"}, "start_time": {"2023-11-14T22:14:20Z"}, "end_date": {"2023-11-14"}, "end_time": {"2023-11-14T22:15:20Z"}})
	if err != nil {
		t.Fatal(err)
	}
}

func TestNewAPIFetchUsersUsesNativeUserAPI(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/status":
			json.NewEncoder(w).Encode(map[string]any{"success": true, "data": map[string]any{"quota_per_unit": 100000}})
		case "/api/user/":
			if r.URL.Query().Get("p") != "1" || r.URL.Query().Get("page_size") != "100" {
				t.Errorf("user query = %v", r.URL.Query())
			}
			json.NewEncoder(w).Encode(map[string]any{"success": true, "data": map[string]any{"items": []any{map[string]any{"id": 7, "username": "alice", "email": "alice@example.com", "role": 1, "status": 1, "quota": 250000, "created_time": 1791939600}}, "total": 1, "page": 1, "page_size": 100}})
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	rawCredential, err := encodeNewAPICredential(newAPICredential{Token: "token", UserID: "5"})
	if err != nil {
		t.Fatal(err)
	}
	users, err := NewService(nil, nil).fetchUsers(context.Background(), server.URL, rawCredential, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(users) != 1 || users[0].ID != 7 || users[0].Balance != 2.5 || users[0].Status != "active" {
		t.Fatalf("users = %#v", users)
	}
}

func TestNewAPIAdminBalanceHistoryRoute(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/status":
			json.NewEncoder(w).Encode(map[string]any{"success": true, "data": map[string]any{"quota_per_unit": 100000}})
		case "/api/user/42":
			json.NewEncoder(w).Encode(map[string]any{"success": true, "data": map[string]any{"id": 42, "username": "alice", "email": "alice@example.com", "role": 1, "status": 1, "quota": 500000, "created_time": 1791939600}})
		case "/api/log/":
			query := r.URL.Query()
			if query.Get("username") != "alice" || query.Get("type") != "1" || query.Get("p") != "1" {
				t.Errorf("history query = %v", query)
			}
			json.NewEncoder(w).Encode(map[string]any{"success": true, "data": map[string]any{"items": []any{map[string]any{"id": 3, "type": 1, "user_id": 42, "username": "alice", "quota": 200000, "created_at": 1791939601, "content": "manual top-up"}}, "total": 1, "page": 1, "page_size": 10}})
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	var got UserBalanceHistory
	err := NewService(nil, nil).newAPIAdmin(context.Background(), http.MethodGet, server.URL+"/api/v1/admin/users/42/balance-history?page=1&page_size=10", newAPICredential{Token: "token", UserID: "5"}, nil, &got)
	if err != nil {
		t.Fatal(err)
	}
	if got.User.ID != 42 || got.User.Balance != 5 || got.TotalRecharged != 2 || len(got.Items) != 1 || got.Items[0].Value != 2 {
		t.Fatalf("history = %#v", got)
	}
}

func TestNewAPIManagementCredentialIsNotForwardedAsSub2APIKey(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-api-key") != "" {
			t.Error("management credential leaked in x-api-key")
		}
		if r.URL.Path != "/api/channel/" {
			t.Errorf("path = %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(map[string]any{"success": true, "data": map[string]any{"items": []any{}, "total": 0}})
	}))
	defer server.Close()
	raw, err := encodeNewAPICredential(newAPICredential{Token: "secret", UserID: "7"})
	if err != nil {
		t.Fatal(err)
	}
	service := NewService(nil, nil)
	if err := service.get(context.Background(), server.URL+"/api/v1/admin/accounts?page=1", raw, &remoteAccountPage{}); err != nil {
		t.Fatal(err)
	}
}

func TestNewAPIChannelPaginationAndGroups(t *testing.T) {
	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer token" || r.Header.Get("New-Api-User") != "7" {
			t.Errorf("headers = %v", r.Header)
		}
		switch r.URL.Path {
		case "/api/channel/":
			requests++
			if r.URL.Query().Get("p") == "2" {
				json.NewEncoder(w).Encode(map[string]any{"success": true, "data": map[string]any{"items": []any{}, "total": 1}})
				return
			}
			json.NewEncoder(w).Encode(map[string]any{"success": true, "data": map[string]any{"items": []any{map[string]any{"id": 12, "name": "channel", "type": 1, "status": 1, "base_url": "https://upstream.example", "group": "default,vip", "models": "gpt-test,claude-test", "priority": 8}}, "total": 1, "page_size": 1}})
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	credential := newAPICredential{Token: "token", UserID: "7"}
	service := NewService(nil, nil)
	var page remoteAccountPage
	err := service.newAPIAdmin(context.Background(), http.MethodGet, server.URL+"/api/v1/admin/accounts?page=1&page_size=100", credential, nil, &page)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].ID != 12 || !page.Items[0].Schedulable || page.Items[0].Credentials.BaseURL != "https://upstream.example" {
		t.Fatalf("page = %#v", page)
	}
	if page.Items[0].Priority != newAPIPriority(8) {
		t.Fatalf("priority = %d", page.Items[0].Priority)
	}
	if len(page.Items[0].GroupIDs) != 2 || page.Items[0].GroupIDs[1] != newAPIGroupID("vip") {
		t.Fatalf("groups = %v", page.Items[0].GroupIDs)
	}
	if requests != 1 {
		t.Fatalf("requests = %d", requests)
	}
}

func TestNewAPIChannelUpdatePreservesFields(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/channel/12":
			json.NewEncoder(w).Encode(map[string]any{"success": true, "data": map[string]any{"id": 12, "name": "channel", "type": 1, "models": "gpt-test", "group": "default", "priority": 8, "status": 1, "setting": "preserved"}})
		case r.Method == http.MethodGet && r.URL.Path == "/api/group/":
			json.NewEncoder(w).Encode(map[string]any{"success": true, "data": []string{"default", "vip"}})
		case r.Method == http.MethodPut && r.URL.Path == "/api/channel/":
			var body map[string]any
			json.NewDecoder(r.Body).Decode(&body)
			if _, ok := body["status"]; ok {
				t.Errorf("channel status must use the dedicated route: %v", body["status"])
			}
			if body["id"] != float64(12) || body["group"] != "vip" || body["models"] != "gpt-test" || body["setting"] != "preserved" {
				t.Errorf("body = %v", body)
			}
			json.NewEncoder(w).Encode(map[string]any{"success": true})
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	service := NewService(nil, nil)
	err := service.newAPIAdmin(context.Background(), http.MethodPut, server.URL+"/api/v1/admin/accounts/12", newAPICredential{Token: "token", UserID: "7"}, map[string]any{"group_ids": []int64{newAPIGroupID("vip")}}, nil)
	if err != nil {
		t.Fatal(err)
	}
}

func TestNewAPIChannelSchedulingUsesStatusEndpoint(t *testing.T) {
	var updates int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/channel/12/status" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL)
			http.NotFound(w, r)
			return
		}
		var body map[string]int
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["status"] != 2 {
			t.Errorf("status body = %v", body)
		}
		updates++
		json.NewEncoder(w).Encode(map[string]any{"success": true, "data": true})
	}))
	defer server.Close()
	service := NewService(nil, nil)
	err := service.newAPIAdmin(context.Background(), http.MethodPut, server.URL+"/api/v1/admin/accounts/12", newAPICredential{Token: "token", UserID: "7"}, map[string]any{"schedulable": false}, nil)
	if err != nil || updates != 1 {
		t.Fatalf("updates=%d err=%v", updates, err)
	}
}

func TestNewAPIChannelSchedulingVerifiesUnchangedStatus(t *testing.T) {
	tests := []struct {
		name         string
		remoteStatus int
		wantError    bool
	}{
		{name: "unchanged target state", remoteStatus: 2},
		{name: "remote state did not change", remoteStatus: 1, wantError: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.Method == http.MethodPost && r.URL.Path == "/api/channel/12/status":
					json.NewEncoder(w).Encode(map[string]any{"success": true, "data": false})
				case r.Method == http.MethodGet && r.URL.Path == "/api/channel/12":
					json.NewEncoder(w).Encode(map[string]any{"success": true, "data": map[string]any{"id": 12, "status": test.remoteStatus}})
				default:
					t.Errorf("unexpected request: %s %s", r.Method, r.URL)
					http.NotFound(w, r)
				}
			}))
			defer server.Close()

			err := NewService(nil, nil).newAPIAdmin(context.Background(), http.MethodPut, server.URL+"/api/v1/admin/accounts/12", newAPICredential{Token: "token", UserID: "7"}, map[string]any{"schedulable": false}, nil)
			if (err != nil) != test.wantError {
				t.Fatalf("error = %v, wantError %v", err, test.wantError)
			}
		})
	}
}

func TestNewAPILogQuotaAndFailureMapping(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/status":
			json.NewEncoder(w).Encode(map[string]any{"success": true, "data": map[string]any{"quota_per_unit": 100}})
		case "/api/log/":
			query := r.URL.Query()
			if query.Get("channel") != "12" {
				t.Errorf("query = %v", r.URL.Query())
			}
			var item any
			switch query.Get("type") {
			case "2":
				item = map[string]any{"id": 4, "channel": 12, "user_id": 8, "type": 2, "quota": 200, "prompt_tokens": 10, "completion_tokens": 20, "created_at": 1700000000, "model_name": "gpt-test", "group": "vip", "other": "{\"frt\":125}"}
			case "5":
				item = map[string]any{"id": 5, "channel": 12, "type": 5, "created_at": 1700000001, "content": "upstream failed"}
			default:
				t.Errorf("用量日志应明确筛选成功或失败类型: %v", query)
			}
			json.NewEncoder(w).Encode(map[string]any{"success": true, "data": map[string]any{"items": []any{item}, "total": 1, "page_size": 100}})
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	service := NewService(nil, nil)
	var page remoteUsagePage
	err := service.newAPIAdmin(context.Background(), http.MethodGet, server.URL+"/api/v1/admin/usage?account_id=12&page=1&page_size=100", newAPICredential{Token: "token", UserID: "7"}, nil, &page)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 2 || page.Total != 2 || page.Items[0].ID != 5 || page.Items[0].StatusCode != 502 || page.Items[1].ID != 4 || page.Items[1].ActualCost != 2 || page.Items[1].AccountID != 12 || page.Items[1].GroupID != newAPIGroupID("vip") {
		t.Fatalf("usage = %#v", page)
	}
}

func TestNewAPIRejectsLogicalFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"success": false, "message": "permission denied"})
	}))
	defer server.Close()
	service := NewService(nil, nil)
	err := service.newAPIAdmin(context.Background(), http.MethodGet, server.URL+"/api/v1/admin/accounts?page=1", newAPICredential{Token: "token", UserID: "7"}, nil, &remoteAccountPage{})
	if err == nil || !strings.Contains(err.Error(), "permission denied") {
		t.Fatalf("error = %v", err)
	}
}

func TestNewAPIUserMutationsUseScopedSecurityProof(t *testing.T) {
	var verifyCalls, manageCalls, unprotectedWrites int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/user/login/encryption-key" {
			json.NewEncoder(w).Encode(map[string]any{"success": true, "data": map[string]any{"enabled": false}})
			return
		}
		if r.URL.Path == "/api/user/login" {
			http.SetCookie(w, &http.Cookie{Name: "session", Value: "valid"})
			json.NewEncoder(w).Encode(map[string]any{"success": true, "data": map[string]any{"id": 7, "role": 100}})
			return
		}
		if r.Header.Get("Cookie") != "session=valid" || r.Header.Get("New-Api-User") != "7" {
			t.Errorf("missing session headers for %s %s: %v", r.Method, r.URL.Path, r.Header)
		}
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/status":
			json.NewEncoder(w).Encode(map[string]any{"success": true, "data": map[string]any{"quota_per_unit": 100}})
		case r.Method == http.MethodGet && r.URL.Path == "/api/verify/methods":
			if r.URL.Query().Get("scope") != "admin.user.manage" {
				t.Errorf("verification scope = %q", r.URL.Query().Get("scope"))
			}
			json.NewEncoder(w).Encode(map[string]any{"success": true, "data": map[string]any{"scope": "admin.user.manage", "methods": []any{map[string]any{"method": "password", "available": true}}}})
		case r.Method == http.MethodPost && r.URL.Path == "/api/verify":
			verifyCalls++
			if r.Header.Get("X-Security-Proof") != "" {
				t.Errorf("verification request already has a proof: %q", r.Header.Get("X-Security-Proof"))
			}
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if body["method"] != "password" || body["scope"] != "admin.user.manage" || body["password"] != "password" {
				t.Errorf("verification body = %v", body)
			}
			context, ok := body["context"].(map[string]any)
			if !ok || context["user_id"] != float64(8) || context["action"] != "disable" {
				t.Errorf("verification context = %v", body["context"])
			}
			json.NewEncoder(w).Encode(map[string]any{"success": true, "data": map[string]any{"proof_token": "proof-1"}})
		case r.Method == http.MethodPost && r.URL.Path == "/api/user/manage":
			if r.Header.Get("X-Security-Proof") == "" {
				unprotectedWrites++
				t.Errorf("mutation sent before proof availability was checked")
				return
			}
			manageCalls++
			if r.Header.Get("X-Security-Proof") != "proof-1" {
				t.Errorf("manage proof = %q", r.Header.Get("X-Security-Proof"))
			}
			json.NewEncoder(w).Encode(map[string]any{"success": true})
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	service := NewService(nil, nil)
	credential := newAPICredential{Username: "admin", Password: "password"}
	if err := service.newAPIAdmin(context.Background(), http.MethodPut, server.URL+"/api/v1/admin/users/8", credential, map[string]any{"status": "inactive"}, nil); err != nil {
		t.Fatal(err)
	}
	if verifyCalls != 1 || manageCalls != 1 || unprotectedWrites != 0 {
		t.Fatalf("proof calls = verify %d, manage %d, unprotected writes %d", verifyCalls, manageCalls, unprotectedWrites)
	}
}

func TestNewAPIUserMutationRejectsUnavailablePasswordVerificationBeforeWrite(t *testing.T) {
	var writes int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/verify/methods" {
			json.NewEncoder(w).Encode(map[string]any{"success": true, "data": map[string]any{"scope": "admin.user.manage", "methods": []any{map[string]any{"method": "2fa", "available": true}, map[string]any{"method": "password", "available": false, "reason": "disabled"}}}})
			return
		}
		if r.URL.Path == "/api/user/manage" {
			writes++
		}
		t.Errorf("unexpected request: %s %s", r.Method, r.URL)
		http.NotFound(w, r)
	}))
	defer server.Close()
	service := NewService(nil, nil)
	err := service.newAPIProtectedRequest(context.Background(), http.MethodPost, server.URL, "/api/user/manage", newAPICredential{Token: "token", UserID: "7"}, map[string]any{"id": 8, "action": "disable"}, "admin.user.manage", map[string]any{"user_id": 8, "action": "disable"})
	if err == nil || !strings.Contains(err.Error(), "交互式安全验证") {
		t.Fatalf("error = %v", err)
	}
	if writes != 0 {
		t.Fatalf("mutation requests = %d", writes)
	}
}

func TestNewAPIUserMutationUsesLegacyRouteOnlyWhenVerificationMethodsAreMissing(t *testing.T) {
	var writes int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/verify/methods" {
			http.NotFound(w, r)
			return
		}
		if r.Method == http.MethodPost && r.URL.Path == "/api/user/manage" {
			writes++
			json.NewEncoder(w).Encode(map[string]any{"success": true})
			return
		}
		t.Errorf("unexpected request: %s %s", r.Method, r.URL)
		http.NotFound(w, r)
	}))
	defer server.Close()
	service := NewService(nil, nil)
	err := service.newAPIProtectedRequest(context.Background(), http.MethodPost, server.URL, "/api/user/manage", newAPICredential{Token: "token", UserID: "7"}, map[string]any{"id": 8, "action": "disable"}, "admin.user.manage", map[string]any{"user_id": 8, "action": "disable"})
	if err != nil {
		t.Fatal(err)
	}
	if writes != 1 {
		t.Fatalf("legacy mutation requests = %d", writes)
	}
}

func TestNewAPIUserMutationDoesNotWriteWhenVerificationMethodsRequestFails(t *testing.T) {
	var writes int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/verify/methods" {
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]any{"success": false, "message": "unauthorized"})
			return
		}
		if r.Method == http.MethodPost && r.URL.Path == "/api/user/manage" {
			writes++
		}
		t.Errorf("unexpected request: %s %s", r.Method, r.URL)
		http.NotFound(w, r)
	}))
	defer server.Close()
	service := NewService(nil, nil)
	err := service.newAPIProtectedRequest(context.Background(), http.MethodPost, server.URL, "/api/user/manage", newAPICredential{Token: "token", UserID: "7"}, map[string]any{"id": 8, "action": "disable"}, "admin.user.manage", map[string]any{"user_id": 8, "action": "disable"})
	if err == nil || writes != 0 {
		t.Fatalf("error=%v writes=%d", err, writes)
	}
}

func TestNewAPILoginRejectsInsecureRemoteTransportBeforeSendingPassword(t *testing.T) {
	service := NewService(nil, nil)
	_, err := service.newAPILogin(context.Background(), "http://new-api.example", newAPICredential{Username: "admin", Password: "secret"})
	if err == nil || !strings.Contains(err.Error(), "需要 HTTPS") {
		t.Fatalf("error = %v", err)
	}
}
