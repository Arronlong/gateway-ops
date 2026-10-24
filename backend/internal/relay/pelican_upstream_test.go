package relay

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/guliansheng/gateway-ops/internal/storage"
)

type pelicanRewriteTransport struct {
	base   http.RoundTripper
	target string
}

func (t pelicanRewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	copy := req.Clone(req.Context())
	u, _ := url.Parse(t.target)
	copy.URL.Scheme, copy.URL.Host = u.Scheme, u.Host
	return t.base.RoundTrip(copy)
}

func TestPelicanModelPriorityUsesNextAvailableModel(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/admin/accounts/42/models":
			w.Write([]byte(`{"code":0,"data":["kimi-k3","deepseek-v4.1-flash","glm-5.3","deepseek-flash","glm-5.3-flash"]}`))
		case "/api/v1/admin/accounts/data":
			w.Write([]byte(`{"code":0,"data":{"accounts":[{"platform":"openai","type":"apikey","credentials":{"base_url":"http://model.invalid","api_key":"upstream-secret","model_mapping":{"deepseek-flash":"actual-deepseek-flash"}},"extra":{"openai_responses_supported":false}}]}}`))
		case "/v1/chat/completions":
			requests++
			var input struct {
				Model string `json:"model"`
			}
			if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
				t.Error(err)
			}
			if input.Model != "actual-deepseek-flash" {
				t.Errorf("wrong fallback model: %s", input.Model)
			}
			w.Write([]byte(`{"choices":[{"message":{"content":"<svg></svg>"},"finish_reason":"stop"}]}`))
		default:
			t.Errorf("unexpected route: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	s := NewService(nil, nil)
	s.client.Transport = pelicanRewriteTransport{base: server.Client().Transport, target: server.URL}
	html, err := s.generatePelican(context.Background(), storage.RelayStation{BaseURL: server.URL}, "admin-secret", storage.PelicanConfig{AccountExternalID: 42, Model: "gpt-6.1-sol\ndeepseek-flash", Prompt: "draw"})
	if err != nil || html != "<svg></svg>" || requests != 1 {
		t.Fatalf("requests=%d html=%q err=%v", requests, html, err)
	}
}

func TestSelectPelicanModel(t *testing.T) {
	for _, tc := range []struct {
		name, preferred string
		models          []string
		want            string
	}{
		{"prefer_configured", "kimi-k3", []string{"deepseek-flash", "kimi-k3"}, "kimi-k3"},
		{"priority_first_available", "gpt-6.1-sol\ndeepseek-flash", []string{"deepseek-flash", "gpt-6.1-sol"}, "gpt-6.1-sol"},
		{"priority_uses_next_available", "gpt-6.1-sol, deepseek-flash", []string{"deepseek-flash", "glm-5.3"}, "deepseek-flash"},
		{"wildcard_priority_returns_concrete_model", "gpt-*", []string{"gpt-6.1-sol", "gpt-5.6-sol"}, "gpt-6.1-sol"},
		{"preserve_wildcard", "gpt-6.1-sol", []string{"deepseek-flash", "gpt-*"}, "gpt-6.1-sol"},
		{"wildcard_catalog_does_not_select_configured_wildcard", "gpt-*", []string{"gpt-*"}, ""},
		{"empty_preference_never_selects_catalog_model", "", []string{"codex-auto-review", "kimi-k3"}, ""},
		{"separator_only_preference_never_selects_catalog_model", ",;\n", []string{"codex-auto-review", "kimi-k3"}, ""},
		{"skip_non_text", "gpt-image-1, kimi-k3", []string{"text-embedding-3-large", "gpt-image-1", "kimi-k3"}, "kimi-k3"},
		{"priority_all_unavailable", "gpt-6.1-sol, gpt-5.6-sol", []string{"kimi-k3", "glm-5.3"}, ""},
		{"no_concrete_model", "gpt-6.1-sol", []string{"deepseek-*", "text-embedding-3-large"}, ""},
		{"empty_catalog", "gpt-6.1-sol", nil, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := selectPelicanModel(tc.preferred, tc.models)
			if got != tc.want || (err != nil) != (tc.want == "") {
				t.Fatalf("got=%q err=%v want=%q", got, err, tc.want)
			}
		})
	}
}

func TestPelicanModelPriorityList(t *testing.T) {
	got := storage.PelicanModelPriorityList(" gpt-6.1-sol\n\uFF0C gpt-5.6-sol, gpt-6.1-sol; kimi-k3 ")
	want := []string{"gpt-6.1-sol", "gpt-5.6-sol", "kimi-k3"}
	if len(got) != len(want) {
		t.Fatalf("got=%#v want=%#v", got, want)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("got=%#v want=%#v", got, want)
		}
	}
}

func TestPelicanModelLookupFailureNeverGenerates(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/admin/accounts/42/models" {
			t.Errorf("unexpected paid request: %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer server.Close()
	s := NewService(nil, nil)
	_, err := s.generatePelican(context.Background(), storage.RelayStation{BaseURL: server.URL}, "admin-secret", storage.PelicanConfig{AccountExternalID: 42, Model: "test-model"})
	if err == nil {
		t.Fatal("model lookup failure must stop generation")
	}
}

func TestPelicanNativeResponses(t *testing.T) {
	cases := []struct {
		name, body, contentType string
		good                    bool
	}{
		{"responses_delta", `data: {"type":"response.output_text.delta","delta":"<svg></svg>"}` + "\n\n" + `data: {"type":"response.completed","response":{"status":"completed"}}` + "\n\n", "text/event-stream", true},
		{"responses_final_only", `data: {"type":"response.completed","response":{"status":"completed","output":[{"type":"message","content":[{"type":"output_text","text":"<svg></svg>"}]}]}}` + "\n\n", "text/event-stream", true},
		{"responses_json", `{"status":"completed","output":[{"type":"message","content":[{"type":"output_text","text":"<svg></svg>"}]}]}`, "application/json", true},
		{"responses_snapshot_does_not_duplicate", `data: {"type":"response.output_text.delta","delta":"<svg></svg>"}` + "\n\n" + `data: {"type":"response.completed","response":{"status":"completed","output":[{"type":"message","content":[{"type":"output_text","text":"<svg></svg>"}]}]}}` + "\n\n", "text/event-stream", true},
		{"responses_done_only", `data: {"type":"response.output_text.done","text":"<svg></svg>"}` + "\n\n" + `data: {"type":"response.completed","response":{"status":"completed"}}` + "\n\n", "text/event-stream", true},
		{"responses_text_configuration", `data: {"type":"response.created","response":{"status":"in_progress","text":{"format":{"type":"text"}}}}` + "\n\n" + `data: {"type":"response.output_text.delta","delta":"<svg></svg>"}` + "\n\n" + `data: {"type":"response.completed","response":{"status":"completed","text":{"format":{"type":"text"}},"output":[{"type":"message","content":[{"type":"output_text","text":"<svg></svg>"}]}]}}` + "\n\n", "text/event-stream", true},
		{"responses_json_text_configuration", `{"status":"completed","text":{"format":{"type":"text"}},"output":[{"type":"message","content":[{"type":"output_text","text":"<svg></svg>"}]}]}`, "application/json", true},
		{"chat_stream", `data: {"choices":[{"index":0,"delta":{"content":"<svg></svg>"},"finish_reason":null}]}` + "\n\n" + `data: {"choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}` + "\n\n" + "data: [DONE]\n\n", "text/event-stream", true},
		{"chat_json", `{"choices":[{"index":0,"message":{"content":"<svg></svg>"},"finish_reason":"stop"}]}`, "application/json", true},
		{"truncated", `data: {"type":"response.output_text.delta","delta":"<svg></svg>"}` + "\n\n", "text/event-stream", false},
		{"incomplete", `{"status":"incomplete","output":[{"content":[{"type":"output_text","text":"<svg></svg>"}]}]}`, "application/json", false},
		{"chat_length", `{"choices":[{"message":{"content":"<svg></svg>"},"finish_reason":"length"}]}`, "application/json", false},
		{"failed", `data: {"type":"response.failed","response":{"error":{"message":"model not found"}}}` + "\n\n", "text/event-stream", false},
		{"no_artwork", `{"status":"completed","output":[{"type":"message","content":[{"type":"output_text","text":"Hi!"}]}]}`, "application/json", false},
		{"broken_artwork", `{"status":"completed","output":[{"type":"message","content":[{"type":"output_text","text":"<html><body>"}]}]}`, "application/json", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/api/v1/admin/accounts/42/models":
					w.Write([]byte(`{"code":0,"data":["test-model"]}`))
				case "/api/v1/admin/accounts/data":
					fmt.Fprintf(w, `{"code":0,"data":{"accounts":[{"platform":"openai","type":"apikey","credentials":{"base_url":"http://model.invalid/v1","api_key":"upstream-secret"}}]}}`)
				case "/v1/responses":
					w.Header().Set("Content-Type", tc.contentType)
					w.Write([]byte(tc.body))
				default:
					t.Errorf("unexpected request: %s", r.URL.Path)
					w.WriteHeader(404)
				}
			}))
			defer srv.Close()
			s := NewService(nil, nil)
			s.client.Transport = pelicanRewriteTransport{base: srv.Client().Transport, target: srv.URL}
			html, err := s.generatePelican(context.Background(), storage.RelayStation{BaseURL: srv.URL}, "admin-secret", storage.PelicanConfig{AccountExternalID: 42, Model: "test-model", Prompt: "draw", ReasoningEffort: "medium"})
			if (err == nil) != tc.good || (tc.good && html != "<svg></svg>") {
				t.Fatalf("html=%q err=%v", html, err)
			}
		})
	}
}

func TestPelicanUsesAccountProxyMappingAndProtocol(t *testing.T) {
	for _, tc := range []struct {
		name, mode string
		supported  bool
		path       string
	}{
		{"responses", "auto", true, "/v1/responses"},
		{"chat", "auto", false, "/v1/chat/completions"},
		{"forced_chat", "force_chat_completions", true, "/v1/chat/completions"},
		{"forced_responses", "force_responses", false, "/v1/responses"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.URL.Host != "model.invalid" || r.URL.Path != tc.path || r.Header.Get("Authorization") != "Bearer upstream-secret" || r.Header.Get("x-api-key") != "" {
					t.Errorf("incorrect account request: %s headers=%v", r.URL, r.Header)
				}
				if r.Header.Get("Proxy-Authorization") == "" {
					t.Error("proxy authentication missing")
				}
				var payload map[string]json.RawMessage
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					t.Fatal(err)
				}
				var model string
				_ = json.Unmarshal(payload["model"], &model)
				if model != "actual-model" {
					t.Errorf("mapped model=%s", model)
				}
				if tc.path == "/v1/responses" {
					var reasoning struct {
						Effort string `json:"effort"`
					}
					_ = json.Unmarshal(payload["reasoning"], &reasoning)
					if reasoning.Effort != "high" || !strings.Contains(string(payload["input"]), "draw") {
						t.Errorf("invalid responses payload")
					}
				} else {
					var effort string
					_ = json.Unmarshal(payload["reasoning_effort"], &effort)
					if effort != "high" || !strings.Contains(string(payload["messages"]), "draw") {
						t.Errorf("invalid chat payload")
					}
				}
				w.Write([]byte(`{"status":"completed","output":[{"content":[{"type":"output_text","text":"<svg></svg>"}]}]}`))
			}))
			defer proxy.Close()
			u, _ := url.Parse(proxy.URL)
			station := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("x-api-key") != "admin-secret" {
					t.Error("missing admin authentication")
				}
				if r.URL.Path == "/api/v1/admin/accounts/42/models" {
					w.Write([]byte(`{"code":0,"data":["test-model"]}`))
					return
				}
				if r.URL.Path != "/api/v1/admin/accounts/data" || r.URL.Query().Get("ids") != "42" || r.URL.Query().Get("include_proxies") != "true" {
					t.Errorf("incorrect export scope: %s", r.URL)
				}
				fmt.Fprintf(w, `{"code":0,"data":{"accounts":[{"platform":"openai","type":"apikey","credentials":{"base_url":"http://model.invalid","api_key":"upstream-secret","model_mapping":{"test-model":"actual-model"}},"extra":{"openai_responses_supported":%t,"openai_responses_mode":%q},"proxy_key":"account-proxy"}],"proxies":[{"proxy_key":"account-proxy","protocol":"http","host":"127.0.0.1","port":%s,"username":"proxy-user","password":"proxy-secret"}]}}`, tc.supported, tc.mode, u.Port())
			}))
			defer station.Close()
			s := NewService(nil, nil)
			html, err := s.generatePelican(context.Background(), storage.RelayStation{BaseURL: station.URL}, "admin-secret", storage.PelicanConfig{AccountExternalID: 42, Model: "test-model", Prompt: "draw", ReasoningEffort: "high"})
			if err != nil || html != "<svg></svg>" || calls != 1 {
				t.Fatalf("calls=%d html=%q err=%v", calls, html, err)
			}
		})
	}
}

func TestPelicanRejectsMissingProxyAndAmbiguousAccount(t *testing.T) {
	for _, accounts := range []string{
		`[{"platform":"openai","type":"apikey","credentials":{"base_url":"http://model.invalid","api_key":"secret"},"proxy_key":"missing"}]`,
		`[]`,
		`[{"platform":"openai"},{"platform":"openai"}]`,
	} {
		t.Run(accounts, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet {
					t.Errorf("unexpected generation request")
				}
				if strings.HasSuffix(r.URL.Path, "/models") {
					w.Write([]byte(`{"code":0,"data":["test-model"]}`))
					return
				}
				fmt.Fprintf(w, `{"code":0,"data":{"accounts":%s}}`, accounts)
			}))
			defer srv.Close()
			s := NewService(nil, nil)
			_, err := s.generatePelican(context.Background(), storage.RelayStation{BaseURL: srv.URL}, "admin-secret", storage.PelicanConfig{AccountExternalID: 42, Model: "test-model"})
			if err == nil {
				t.Fatal("unsafe generation must be rejected")
			}
		})
	}
}

func TestPelicanRedactsUpstreamCredentialsAndDoesNotFollowRedirects(t *testing.T) {
	for _, redirect := range []bool{false, true} {
		t.Run(fmt.Sprint(redirect), func(t *testing.T) {
			sinkCalls := 0
			sink := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { sinkCalls++ }))
			defer sink.Close()
			model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if redirect {
					http.Redirect(w, r, sink.URL, http.StatusTemporaryRedirect)
					return
				}
				w.WriteHeader(http.StatusBadRequest)
				w.Write([]byte(`{"error":{"message":"upstream-secret admin-secret"}}`))
			}))
			defer model.Close()
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.HasSuffix(r.URL.Path, "/models") {
					w.Write([]byte(`{"code":0,"data":["test-model"]}`))
					return
				}
				fmt.Fprintf(w, `{"code":0,"data":{"accounts":[{"platform":"openai","type":"apikey","credentials":{"base_url":%q,"api_key":"upstream-secret"}}]}}`, model.URL)
			}))
			defer srv.Close()
			s := NewService(nil, nil)
			_, err := s.generatePelican(context.Background(), storage.RelayStation{BaseURL: srv.URL}, "admin-secret", storage.PelicanConfig{AccountExternalID: 42, Model: "test-model"})
			if err == nil || strings.Contains(err.Error(), "upstream-secret") || strings.Contains(err.Error(), "admin-secret") || sinkCalls != 0 {
				t.Fatalf("sinkCalls=%d err=%v", sinkCalls, err)
			}
		})
	}
}

func TestPelicanOAuthCarriesDrawingPrompt(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/admin/accounts/42/models":
			w.Write([]byte(`{"code":0,"data":["test-model"]}`))
		case "/api/v1/admin/accounts/data":
			w.Write([]byte(`{"code":0,"data":{"accounts":[{"platform":"openai","type":"oauth","credentials":{"access_token":"oauth-secret","chatgpt_account_id":"account-42"}}]}}`))
		case "/backend-api/codex/responses":
			if r.Header.Get("Authorization") != "Bearer oauth-secret" || r.Header.Get("ChatGPT-Account-Id") != "account-42" || r.Header.Get("OpenAI-Beta") == "" {
				t.Error("OAuth headers missing")
			}
			var payload struct {
				Input []struct {
					Content []struct {
						Text string `json:"text"`
					} `json:"content"`
				} `json:"input"`
				Store        bool   `json:"store"`
				Instructions string `json:"instructions"`
			}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Error(err)
				return
			}
			if payload.Store || payload.Instructions == "" || len(payload.Input) != 1 || len(payload.Input[0].Content) != 1 || !strings.HasPrefix(payload.Input[0].Content[0].Text, "draw a pelican") {
				t.Error("OAuth drawing prompt missing")
			}
			w.Write([]byte(`{"status":"completed","output":[{"content":[{"type":"output_text","text":"<svg></svg>"}]}]}`))
		default:
			t.Errorf("unexpected request: %s", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	s := NewService(nil, nil)
	s.client.Transport = pelicanRewriteTransport{base: server.Client().Transport, target: server.URL}
	html, err := s.generatePelican(context.Background(), storage.RelayStation{BaseURL: server.URL}, "admin-secret", storage.PelicanConfig{AccountExternalID: 42, Model: "test-model", Prompt: "draw a pelican"})
	if err != nil || html != "<svg></svg>" {
		t.Fatalf("html=%q err=%v", html, err)
	}
}

func TestPelicanAdminRequestsNeverFollowRedirects(t *testing.T) {
	for _, redirectPath := range []string{"/api/v1/admin/accounts/42/models", "/api/v1/admin/accounts/data"} {
		t.Run(redirectPath, func(t *testing.T) {
			sinkCalls := 0
			sink := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { sinkCalls++; w.Write([]byte(`{"code":0,"data":[]}`)) }))
			defer sink.Close()
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == redirectPath {
					http.Redirect(w, r, sink.URL, http.StatusFound)
					return
				}
				w.Write([]byte(`{"code":0,"data":["test-model"]}`))
			}))
			defer srv.Close()
			s := NewService(nil, nil)
			_, err := s.generatePelican(context.Background(), storage.RelayStation{BaseURL: srv.URL}, "admin-secret", storage.PelicanConfig{AccountExternalID: 42, Model: "test-model"})
			if err == nil || sinkCalls != 0 {
				t.Fatalf("redirect target calls=%d err=%v", sinkCalls, err)
			}
		})
	}
}
