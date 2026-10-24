package relay

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPelicanKeyGroupMatchesExactKeyAndPreservesZero(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/sub2api/billing" && r.Header.Get("Authorization") != "Bearer login-session" {
			t.Error("wrong session")
		}
		switch r.URL.Path {
		case "/api/v1/keys":
			fmt.Fprint(w, `{"code":0,"data":{"items":[{"key":"other-key","group_id":1,"group":{"id":1,"name":"错误分组","rate_multiplier":9}},{"key":"actual-key","group_id":2,"group":{"id":2,"name":"当前 Key 对应的长分组名称","rate_multiplier":3}}],"pages":1}}`)
		case "/api/v1/groups/rates":
			fmt.Fprint(w, `{"code":0,"data":{"2":0}}`)
		case "/v1/sub2api/billing":
			http.NotFound(w, r)
		default:
			t.Errorf("unexpected request: %s", r.URL.Path)
		}
	}))
	defer server.Close()
	target := &pelicanTarget{client: *server.Client(), endpoint: server.URL + "/v1/responses", token: "actual-key"}
	info := target.readKeyGroup(context.Background(), server.URL, "login-session")
	if info.Name != "当前 Key 对应的长分组名称" || info.Multiplier == nil || *info.Multiplier != 0 {
		t.Fatalf("wrong group: %#v", info)
	}
}

func TestPelicanKeyGroupUnknownDoesNotGuessAndDoesNotLeakCredentials(t *testing.T) {
	for _, body := range []string{
		`{"code":0,"data":{"items":[{"key":"other-key","group":{"id":1,"name":"其他分组","rate_multiplier":1}}],"pages":1}}`,
		`{"code":0,"data":{"items":[{"key":"actual-key","group_ids":[1,2],"group":{"id":1,"name":"不确定的多分组","rate_multiplier":1}}],"pages":1}}`,
	} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, body) }))
		target := &pelicanTarget{client: *server.Client(), endpoint: server.URL + "/v1/responses", token: "actual-key"}
		info := target.readKeyGroup(context.Background(), server.URL, "session")
		if info.Name != "" || info.Multiplier != nil {
			t.Fatalf("must not infer a group: %#v", info)
		}
		server.Close()
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer session" {
			t.Error("only the source-specific login session may be sent")
		}
		fmt.Fprint(w, `{"code":0,"data":{"items":[],"pages":1}}`)
	}))
	defer server.Close()
	target := &pelicanTarget{client: *server.Client(), endpoint: "https://different.example/v1/responses", token: "secret"}
	info := target.readKeyGroup(context.Background(), server.URL, "session")
	if info.Name != "" || info.Multiplier != nil {
		t.Fatal("unexpected metadata")
	}
}

func TestPelicanKeyGroupUnavailableRateKeepsKnownName(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/keys" {
			fmt.Fprint(w, `{"code":0,"data":{"items":[{"key":"actual-key","group_id":2,"group":{"id":2,"name":"已确认分组","rate_multiplier":3}}],"pages":1}}`)
			return
		}
		http.Error(w, "unavailable", http.StatusNotFound)
	}))
	defer server.Close()
	target := &pelicanTarget{client: *server.Client(), endpoint: server.URL + "/v1/responses", token: "actual-key"}
	info := target.readKeyGroup(context.Background(), server.URL, "session")
	if info.Name != "已确认分组" || info.Multiplier != nil {
		t.Fatalf("must not substitute default rate: %#v", info)
	}
}
