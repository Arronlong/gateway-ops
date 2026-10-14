package relay

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestNewAPIUsageLogsMergeConsumptionAndFailures(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/status":
			fmt.Fprint(w, `{"success":true,"data":{"quota_per_unit":100}}`)
		case "/api/log/":
			q := r.URL.Query()
			if q.Get("p") != "1" || q.Get("page_size") != "2" {
				t.Errorf("unexpected pagination: %v", q)
			}
			var item map[string]any
			if q.Get("type") == "2" {
				item = map[string]any{"id": 30, "type": 2, "quota": 100, "created_at": 300}
			} else if q.Get("type") == "5" {
				item = map[string]any{"id": 20, "type": 5, "quota": 0, "created_at": 200}
			} else {
				t.Errorf("missing precise log type: %v", q)
			}
			json.NewEncoder(w).Encode(map[string]any{"success": true, "data": map[string]any{"items": []any{item}, "total": 1, "page": 1, "page_size": 2}})
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	page, err := NewService(nil, nil).newAPIUsageLogs(context.Background(), server.URL, newAPICredential{Token: "token", UserID: "7"}, url.Values{"page": {"1"}, "page_size": {"2"}})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 2 || len(page.Items) != 2 || page.Items[0].ID != 30 || page.Items[0].StatusCode != 200 || page.Items[1].ID != 20 || page.Items[1].StatusCode != 502 {
		t.Fatalf("merged page = %#v", page)
	}
}

func TestNewAPIRequestRejectsNonLoopbackHTTPWithoutSendingCredentials(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"success":true,"data":[]}`)
	}))
	defer server.Close()
	s := NewService(nil, nil)
	_, err := s.newAPIRequest(context.Background(), http.MethodGet, "http://public.example/api/group/", newAPICredential{Token: "admin-secret", UserID: "7"}, nil)
	if err == nil || called {
		t.Fatalf("expected HTTPS refusal before sending credentials; called=%v err=%v", called, err)
	}
	if _, err := s.newAPIRequest(context.Background(), http.MethodGet, server.URL+"/api/group/", newAPICredential{Token: "loopback", UserID: "7"}, nil); err != nil {
		t.Fatalf("loopback development HTTP should remain available: %v", err)
	}
}
