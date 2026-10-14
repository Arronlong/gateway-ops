package relay

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNewAPIGroupGatewayKeyUsesMatchingFullToken(t *testing.T) {
	var keyReads int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/group/":
			fmt.Fprint(w, `{"success":true,"data":["default","vip"]}`)
		case "/api/user/self":
			fmt.Fprint(w, `{"success":true,"data":{"group":"default"}}`)
		case "/api/token/":
			fmt.Fprintf(w, `{"success":true,"data":{"items":[{"id":1,"group":"default","status":1,"unlimited_quota":true,"key":"masked"},{"id":2,"group":"vip","status":1,"unlimited_quota":true,"expired_time":%d,"key":"masked"},{"id":3,"group":"vip","status":1,"unlimited_quota":true,"expired_time":-1,"key":"masked"}],"total":3,"page":1,"page_size":100}}`, time.Now().Add(-time.Hour).Unix())
		case "/api/token/3/key":
			if r.Method != http.MethodPost {
				t.Errorf("method = %s", r.Method)
			}
			keyReads++
			fmt.Fprint(w, `{"success":true,"data":{"key":"sk-gateway-vip"}}`)
		case "/api/token/1/key":
			fmt.Fprint(w, `{"success":true,"data":{"key":"sk-gateway-default"}}`)
		default:
			t.Errorf("unexpected %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	s := &Service{client: server.Client()}
	c := newAPICredential{AuthMode: "access_token", Token: "management-secret", UserID: "1"}
	key, err := s.newAPIGroupGatewayKey(context.Background(), server.URL, c, newAPIGroupID("vip"))
	if err != nil || key != "sk-gateway-vip" || keyReads != 1 {
		t.Fatalf("key=%q reads=%d err=%v", key, keyReads, err)
	}
	c.GatewayKey = "sk-other-group"
	_, err = s.newAPIGroupGatewayKey(context.Background(), server.URL, c, newAPIGroupID("vip"))
	if err == nil || strings.Contains(err.Error(), c.GatewayKey) {
		t.Fatalf("unmatched secret must fail safely: %v", err)
	}
}

func TestNewAPIGroupGatewayKeyAcceptsUnambiguousAutoGroupToken(t *testing.T) {
	var keyReads int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/group/":
			fmt.Fprint(w, `{"success":true,"data":["vip","default"]}`)
		case "/api/token/":
			fmt.Fprint(w, `{"success":true,"data":{"items":[{"id":5,"group":"auto","auto_groups":["vip"],"status":1,"unlimited_quota":true,"key":"masked"},{"id":6,"group":"auto","auto_groups":["vip","default"],"status":1,"unlimited_quota":true,"key":"masked"}],"total":2,"page":1,"page_size":100}}`)
		case "/api/token/5/key":
			keyReads++
			fmt.Fprint(w, `{"success":true,"data":{"key":"sk-gateway-vip"}}`)
		default:
			t.Errorf("unexpected %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	s := &Service{client: server.Client()}
	key, err := s.newAPIGroupGatewayKey(context.Background(), server.URL, newAPICredential{Token: "management-secret", UserID: "1"}, newAPIGroupID("vip"))
	if err != nil || key != "sk-gateway-vip" || keyReads != 1 {
		t.Fatalf("key=%q reads=%d err=%v", key, keyReads, err)
	}
}
