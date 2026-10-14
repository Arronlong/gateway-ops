package relay

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

func operationsReply(t *testing.T, w http.ResponseWriter, data any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]any{"success": true, "data": data}); err != nil {
		t.Errorf("写入响应失败: %v", err)
	}
}

func operationsPage(t *testing.T, w http.ResponseWriter, items any, total, page int) {
	t.Helper()
	operationsReply(t, w, map[string]any{"items": items, "total": total, "page": page, "page_size": 1})
}

func operationsCredential() newAPICredential {
	return newAPICredential{Token: "admin-token", UserID: "7"}
}

func TestNewAPIOperationsClonePreservesSettingsAndFindsNewID(t *testing.T) {
	source := map[string]any{
		"id": 12, "type": 1, "name": "source", "key": "source-secret", "status": 1,
		"base_url": "https://source.example", "models": "gpt-test,claude-test", "group": "default,vip",
		"priority": 8, "weight": 3, "test_model": "gpt-test", "openai_organization": "org-test",
		"model_mapping": `{"gpt-test":"gpt-actual"}`, "status_code_mapping": `{"429":"503"}`,
		"auto_ban": 0, "tag": "pool", "setting": `{"force_format":true}`,
		"settings": `{"pass_through_body_enabled":true}`, "param_override": `{"temperature":0.2}`,
		"header_override": `{"X-Custom":"keep"}`, "remark": "inherited",
		"created_time": 10, "test_time": 20, "response_time": 30, "balance": 40,
		"balance_updated_time": 50, "used_quota": 60, "other_info": `{"status_reason":"secret","status_time":123,"custom":"keep"}`,
		"channel_info": map[string]any{"is_multi_key": true, "multi_key_size": 4, "multi_key_status_list": map[string]any{"0": 2}},
	}
	old := map[string]any{"id": 13, "type": 1, "name": "target", "status": 1, "base_url": "https://target.example"}
	var created map[string]any
	writes := 0
	lists := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer admin-token" || r.Header.Get("New-Api-User") != "7" {
			t.Errorf("管理认证头不正确: %v", r.Header)
		}
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/channel/12":
			operationsReply(t, w, source)
		case r.Method == http.MethodGet && r.URL.Path == "/api/channel/":
			lists++
			page, _ := strconv.Atoi(r.URL.Query().Get("p"))
			total := 2
			if created != nil {
				total = 3
			}
			item := source
			if page == 2 {
				item = old
			}
			if page == 3 {
				item = created
			}
			if item == nil || page > total {
				operationsPage(t, w, []any{}, total, page)
				return
			}
			operationsPage(t, w, []any{item}, total, page)
		case r.Method == http.MethodPost && r.URL.Path == "/api/channel/":
			writes++
			var body struct {
				Mode    string         `json:"mode"`
				Channel map[string]any `json:"channel"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if body.Mode != "single" || body.Channel == nil {
				t.Errorf("创建协议错误: %#v", body)
			}
			created = body.Channel
			if created["name"] != "target" || created["base_url"] != "https://target.example" || created["key"] != "new-secret" {
				t.Errorf("新凭据未写入: %#v", created)
			}
			for _, key := range []string{"id", "created_time", "test_time", "response_time", "balance", "balance_updated_time", "used_quota", "channel_info"} {
				if _, exists := created[key]; exists {
					t.Errorf("不应继承运行数据: %s", key)
				}
			}
			var inheritedInfo string
			if raw, ok := created["other_info"].(string); !ok || json.Unmarshal([]byte(raw), &map[string]any{}) != nil {
				t.Errorf("自定义附加信息未继承: %v", created["other_info"])
			} else {
				inheritedInfo = raw
				var info map[string]any
				_ = json.Unmarshal([]byte(inheritedInfo), &info)
				if info["custom"] != "keep" || info["status_reason"] != nil || info["status_time"] != nil {
					t.Errorf("附加信息未清理运行态字段: %v", info)
				}
			}
			for _, key := range []string{"type", "status", "models", "group", "priority", "weight", "test_model", "openai_organization", "model_mapping", "status_code_mapping", "auto_ban", "tag", "setting", "settings", "param_override", "header_override", "remark"} {
				if fmt.Sprint(created[key]) != fmt.Sprint(source[key]) {
					t.Errorf("设置丢失: %s = %v", key, created[key])
				}
			}
			created["id"] = 14
			// 上游正常创建响应没有 ID。
			operationsReply(t, w, nil)
		default:
			t.Errorf("意外请求: %s %s", r.Method, r.URL)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	account, err := NewService(nil, nil).newAPICloneAccount(context.Background(), server.URL+"/", operationsCredential(), 12, BatchCloneAccount{Name: "target", APIKey: "new-secret", BaseURL: "https://target.example"})
	if err != nil {
		t.Fatal(err)
	}
	if account == nil || account.ID != 14 || account.Name != "target" || account.Credentials.BaseURL != "https://target.example" || len(account.GroupIDs) != 2 || account.Priority != newAPIPriority(8) {
		t.Fatalf("克隆结果不正确: %#v", account)
	}
	if writes != 1 || lists != 5 {
		t.Fatalf("请求次数不正确: writes=%d lists=%d", writes, lists)
	}
}

func TestNewAPIOperationsCloneNeverUsesExistingOrAmbiguousChannel(t *testing.T) {
	for _, scenario := range []string{"existing_only", "ambiguous", "forged_id", "post_failure", "source_missing"} {
		t.Run(scenario, func(t *testing.T) {
			posted := false
			writes := 0
			channel := func(id int64) map[string]any {
				return map[string]any{"id": id, "type": 1, "status": 1, "name": "target", "base_url": "https://target.example", "models": "gpt-test", "group": "default"}
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.Method == http.MethodGet && r.URL.Path == "/api/channel/12":
					if scenario == "source_missing" {
						operationsReply(t, w, nil)
					} else {
						operationsReply(t, w, channel(12))
					}
				case r.Method == http.MethodGet && r.URL.Path == "/api/channel/":
					items := []any{channel(12)}
					if posted && scenario == "ambiguous" {
						items = append(items, channel(13), channel(14))
					}
					operationsReply(t, w, map[string]any{"items": items, "total": len(items), "page": 1, "page_size": 100})
				case r.Method == http.MethodPost && r.URL.Path == "/api/channel/":
					posted = true
					writes++
					if scenario == "post_failure" {
						json.NewEncoder(w).Encode(map[string]any{"success": false, "message": "upstream denied"})
						return
					}
					if scenario == "forged_id" {
						operationsReply(t, w, channel(12))
					} else {
						operationsReply(t, w, nil)
					}
				default:
					t.Errorf("不得修改旧渠道: %s %s", r.Method, r.URL)
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			account, err := NewService(nil, nil).newAPICloneAccount(context.Background(), server.URL, operationsCredential(), 12, BatchCloneAccount{Name: "target", APIKey: "new-secret", BaseURL: "https://target.example"})
			if err == nil || account != nil {
				t.Fatalf("应安全失败: account=%#v error=%v", account, err)
			}
			if writes > 1 || (scenario == "source_missing" && writes != 0) {
				t.Fatalf("不应重复创建: %d", writes)
			}
			if scenario == "post_failure" && !strings.Contains(err.Error(), "upstream denied") {
				t.Fatalf("上游错误丢失: %v", err)
			}
		})
	}
}

func TestNewAPIOperationsTestUsesNativeGETAndLogicalStatus(t *testing.T) {
	for _, tc := range []struct {
		mode, stream string
		success      bool
	}{
		{"", "false", true}, {"regular", "false", true}, {"stream", "true", true}, {"non-stream", "false", true}, {"stream", "true", false},
	} {
		t.Run(tc.mode+"_"+strconv.FormatBool(tc.success), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != "/api/channel/test/12" || r.URL.Query().Get("model") != "gpt test/+" || r.URL.Query().Get("stream") != tc.stream {
					t.Errorf("测试请求不正确: %s %s", r.Method, r.URL)
				}
				json.NewEncoder(w).Encode(map[string]any{"success": tc.success, "message": "upstream result", "time": 0.125, "data": map[string]any{"echo": "channel-secret"}})
			}))
			defer server.Close()
			result, err := NewService(nil, nil).newAPITestAccount(context.Background(), server.URL, operationsCredential(), 12, "gpt test/+", tc.mode)
			if tc.success && err != nil {
				t.Fatal(err)
			}
			if !tc.success && (err == nil || !strings.Contains(err.Error(), "upstream result")) {
				t.Fatalf("逻辑失败未返回错误: %v", err)
			}
			var output map[string]any
			if json.Unmarshal([]byte(result.Output), &output) != nil || output["success"] != tc.success || result.StatusCode != http.StatusOK || result.DurationMS < 0 || strings.Contains(result.Output, "channel-secret") || output["data"] != nil {
				t.Fatalf("测试结果不完整或包含上游秘密: %#v", result)
			}
		})
	}
}

func TestNewAPIOperationsDeleteGroupChecksAllReferences(t *testing.T) {
	for _, reference := range []string{"channel", "user", "token", "permission", "pagination"} {
		t.Run(reference, func(t *testing.T) {
			writes := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet {
					writes++
					t.Errorf("引用检查失败后不得写入: %s", r.Method)
				}
				switch r.URL.Path {
				case "/api/group/":
					operationsReply(t, w, []string{"default", "vip"})
				case "/api/option/":
					operationsReply(t, w, []any{map[string]any{"key": "GroupRatio", "value": `{"default":1,"vip":0.5}`}})
				case "/api/channel/", "/api/user/", "/api/token/":
					path := r.URL.Path
					if reference == "permission" && path == "/api/token/" {
						json.NewEncoder(w).Encode(map[string]any{"success": false, "message": "permission denied"})
						return
					}
					page, _ := strconv.Atoi(r.URL.Query().Get("p"))
					if reference == "pagination" && path == "/api/channel/" {
						operationsPage(t, w, []any{}, 2, page)
						return
					}
					item := map[string]any{"id": page + 10, "name": "ref", "username": "admin", "group": "default", "status": 2}
					if page == 2 && ((reference == "channel" && path == "/api/channel/") || (reference == "user" && path == "/api/user/") || (reference == "token" && path == "/api/token/")) {
						item["group"] = "vip"
						if reference == "channel" {
							item["group"] = "default, vip"
						}
					}
					operationsPage(t, w, []any{item}, 2, page)
				default:
					t.Errorf("意外请求: %s", r.URL)
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			err := NewService(nil, nil).newAPIDeleteGroup(context.Background(), server.URL, operationsCredential(), newAPIGroupID("vip"))
			if err == nil || writes != 0 {
				t.Fatalf("不得删除使用中或未完成检查的分组: error=%v writes=%d", err, writes)
			}
			expected := map[string]string{"channel": "渠道", "user": "用户", "token": "令牌"}[reference]
			if expected != "" && !strings.Contains(err.Error(), expected) {
				t.Fatalf("错误缺少可操作提示: %v", err)
			}
		})
	}
}

func TestNewAPIOperationsDeleteGroupPreservesWholeRatioOption(t *testing.T) {
	for _, key := range []string{"GroupRatio", "group_ratio_setting.group_ratio"} {
		t.Run(key, func(t *testing.T) {
			writes := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/api/group/":
					operationsReply(t, w, []string{"default", "vip", "other"})
				case "/api/channel/", "/api/user/", "/api/token/":
					operationsReply(t, w, map[string]any{"items": []any{}, "total": 0, "page_size": 100})
				case "/api/option/":
					if r.Method == http.MethodGet {
						operationsReply(t, w, []any{map[string]any{"key": key, "value": `{"default":1,"vip":0.5,"other":1.2345678901234567}`}})
						return
					}
					if r.Method != http.MethodPut {
						t.Errorf("写入方法错误: %s", r.Method)
					}
					writes++
					var body struct {
						Key   string `json:"key"`
						Value string `json:"value"`
					}
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Fatal(err)
					}
					var ratios map[string]json.RawMessage
					if body.Key != key || json.Unmarshal([]byte(body.Value), &ratios) != nil || len(ratios) != 2 || string(ratios["default"]) != "1" || string(ratios["other"]) != "1.2345678901234567" {
						t.Errorf("其他分组或数值精度丢失: %#v", body)
					}
					operationsReply(t, w, nil)
				default:
					t.Errorf("意外请求: %s", r.URL)
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			if err := NewService(nil, nil).newAPIDeleteGroup(context.Background(), server.URL+"/", operationsCredential(), newAPIGroupID("vip")); err != nil {
				t.Fatal(err)
			}
			if writes != 1 {
				t.Fatalf("写入次数错误: %d", writes)
			}
		})
	}
}

func TestNewAPIOperationsDefaultGroupCannotBeDeleted(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests++; operationsReply(t, w, []string{"default"}) }))
	defer server.Close()
	err := NewService(nil, nil).newAPIDeleteGroup(context.Background(), server.URL, operationsCredential(), newAPIGroupID("default"))
	if err == nil || !strings.Contains(err.Error(), "default") || requests > 1 {
		t.Fatalf("默认分组保护失效: %v", err)
	}
}

func TestNewAPIOperationsGroupModelsUsesEveryActiveMatchingChannel(t *testing.T) {
	lists := 0
	channels := []map[string]any{
		{"id": 1, "status": 1, "group": "default, vip", "models": " z-model, a-model,z-model "},
		{"id": 2, "status": 2, "group": "vip", "models": "disabled-model"},
		{"id": 3, "status": 1, "group": "vip-plus", "models": "wrong-group-model"},
		{"id": 4, "status": 1, "group": "vip", "models": "b-model,a-model"},
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/group/":
			operationsReply(t, w, []string{"default", "vip"})
		case "/api/channel/":
			lists++
			if r.URL.Query().Get("tag_mode") == "true" {
				t.Errorf("不能使用标签聚合分页")
			}
			page, _ := strconv.Atoi(r.URL.Query().Get("p"))
			if page < 1 || page > len(channels) {
				t.Errorf("页码错误: %d", page)
				operationsPage(t, w, []any{}, len(channels), page)
				return
			}
			operationsPage(t, w, []any{channels[page-1]}, len(channels), page)
		default:
			t.Errorf("意外请求: %s", r.URL)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	models, err := NewService(nil, nil).newAPIGroupModels(context.Background(), server.URL, operationsCredential(), newAPIGroupID("vip"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(models, ",") != "a-model,b-model,z-model" || lists != 4 {
		t.Fatalf("模型或分页不正确: %v lists=%d", models, lists)
	}
}

func TestNewAPIOperationsCloneInheritsOptionalNameAndBase(t *testing.T) {
	created := false
	writes := 0
	source := map[string]any{"id": 12, "type": 1, "status": 1, "name": "source", "base_url": "https://source.example", "models": "gpt-test", "group": "default"}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/channel/12":
			operationsReply(t, w, source)
		case r.Method == http.MethodGet && r.URL.Path == "/api/channel/":
			items := []any{source}
			if created {
				items = append(items, map[string]any{"id": 13, "type": 1, "status": 1, "name": "source", "base_url": "https://source.example", "models": "gpt-test", "group": "default"})
			}
			operationsReply(t, w, map[string]any{"items": items, "total": len(items)})
		case r.Method == http.MethodPost && r.URL.Path == "/api/channel/":
			writes++
			var body struct {
				Channel map[string]any `json:"channel"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if body.Channel["name"] != "source" || body.Channel["base_url"] != "https://source.example" || body.Channel["key"] != "new-key" {
				t.Errorf("可选字段未继承: %v", body)
			}
			created = true
			operationsReply(t, w, nil)
		default:
			t.Errorf("意外请求: %s %s", r.Method, r.URL)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	account, err := NewService(nil, nil).newAPICloneAccount(context.Background(), server.URL, operationsCredential(), 12, BatchCloneAccount{APIKey: "new-key"})
	if err != nil || account == nil || account.ID != 13 || writes != 1 {
		t.Fatalf("继承克隆失败: %#v %v", account, err)
	}
}

func TestNewAPIOperationsDeleteGroupDetectsAutoTokens(t *testing.T) {
	for _, groups := range [][]string{{"vip"}, {}, nil} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet {
				t.Errorf("不得写入: %s", r.Method)
			}
			switch r.URL.Path {
			case "/api/group/":
				operationsReply(t, w, []string{"default", "vip"})
			case "/api/channel/", "/api/user/":
				operationsReply(t, w, map[string]any{"items": []any{}, "total": 0})
			case "/api/token/":
				operationsReply(t, w, map[string]any{"items": []any{map[string]any{"id": 1, "group": "auto", "auto_groups": groups}}, "total": 1})
			default:
				t.Errorf("意外请求: %s", r.URL)
				http.NotFound(w, r)
			}
		}))
		err := NewService(nil, nil).newAPIDeleteGroup(context.Background(), server.URL, operationsCredential(), newAPIGroupID("vip"))
		server.Close()
		if err == nil || !strings.Contains(err.Error(), "令牌") {
			t.Fatalf("自动分组令牌未检出: %v", err)
		}
	}
}

func TestNewAPIProbeAccountsRequiresManualCostWithoutReadingChannelKey(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		t.Errorf("自动成本探测不应请求 New API 管理接口: %s %s", r.Method, r.URL)
		http.Error(w, "unexpected request", http.StatusInternalServerError)
	}))
	defer server.Close()

	oldExtra := map[string]any{"keep": "value", "upstream_billing_probe": map[string]any{"status": "ok", "data": map[string]any{"resolved_rate_multiplier": 1.0}}}
	input := []remoteAccount{{ID: 12, Type: "apikey", Extra: oldExtra}}
	result, err := NewService(nil, nil).newAPIProbeAccounts(context.Background(), server.URL, operationsCredential(), input)
	if err != nil || len(result) != 1 {
		t.Fatalf("未知成本不应终止同步: %#v %v", result, err)
	}
	cost, observed := upstreamProbeCost(result[0].Extra)
	if cost != nil || observed != nil || result[0].RateMultiplier != nil {
		t.Fatalf("无法安全确定成本时必须保持未知: %#v", result[0])
	}
	probe, ok := result[0].Extra["upstream_billing_probe"].(map[string]any)
	if !ok || probe["status"] != "manual_required" || !strings.Contains(fmt.Sprint(probe["error"]), "手工") {
		t.Fatalf("必须说明需要手工配置成本: %#v", result[0].Extra["upstream_billing_probe"])
	}
	encoded, _ := json.Marshal(result)
	if called || strings.Contains(string(encoded), "admin-token") || result[0].Extra["keep"] != "value" {
		t.Fatalf("触发了上游请求、泄漏凭据或丢失附加配置: called=%v result=%s", called, encoded)
	}
	if input[0].RateMultiplier != nil || input[0].Extra["upstream_billing_probe"].(map[string]any)["status"] != "ok" {
		t.Fatal("探测修改了调用方数据")
	}
}

func TestNewAPIOperationsTestRedactsManagementSecretsAndOmitsRawData(t *testing.T) {
	service := NewService(nil, nil)
	credential := newAPICredential{Username: "admin", Password: "admin-password", UserID: "7", GatewayKey: "gateway-secret", Cookie: "session=cookie-secret"}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer session-token" || r.Header.Get("Cookie") != "session=cookie-secret" {
			t.Errorf("会话认证头不正确: %v", r.Header)
		}
		json.NewEncoder(w).Encode(map[string]any{"success": false, "message": "failed admin-password gateway-secret cookie-secret session-token", "data": map[string]any{"echo": "admin-password channel-secret"}})
	}))
	defer server.Close()
	entry := service.newAPISessionEntry(server.URL, credential)
	entry.session = newAPISession{Token: "session-token", Cookie: "session=cookie-secret", UserID: "7", Expires: time.Now().Add(time.Hour)}
	result, err := service.newAPITestAccount(context.Background(), server.URL, credential, 12, "model", "regular")
	if err == nil || result.StatusCode != http.StatusOK {
		t.Fatalf("测试错误或状态码丢失: %#v %v", result, err)
	}
	for _, secret := range []string{"admin-password", "gateway-secret", "cookie-secret", "session-token", "channel-secret"} {
		if strings.Contains(result.Output, secret) || strings.Contains(err.Error(), secret) {
			t.Fatalf("测试结果泄漏凭据 %q: %s %v", secret, result.Output, err)
		}
	}
	if strings.Contains(result.Output, "data") || !strings.Contains(result.Output, "[已隐藏]") {
		t.Fatalf("未对测试输出做脱敏: %s", result.Output)
	}
}

func TestNewAPIOperationsConcurrentGroupDeletesSerializeRatioUpdates(t *testing.T) {
	service := NewService(nil, nil)
	var mu sync.Mutex
	ratios := map[string]json.RawMessage{"default": json.RawMessage("1"), "vip": json.RawMessage("0.5"), "other": json.RawMessage("2")}
	reads := 0
	firstRead := make(chan struct{})
	releaseFirst := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/group/":
			operationsReply(t, w, []string{"default", "vip", "other"})
		case "/api/channel/", "/api/user/", "/api/token/":
			operationsReply(t, w, map[string]any{"items": []any{}, "total": 0})
		case "/api/option/":
			if r.Method == http.MethodGet {
				mu.Lock()
				reads++
				count := reads
				snapshot, _ := json.Marshal(ratios)
				mu.Unlock()
				if count == 1 {
					close(firstRead)
					<-releaseFirst
				}
				operationsReply(t, w, []any{map[string]any{"key": "GroupRatio", "value": string(snapshot)}})
				return
			}
			var body struct {
				Key   string `json:"key"`
				Value string `json:"value"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
				return
			}
			var next map[string]json.RawMessage
			if err := json.Unmarshal([]byte(body.Value), &next); err != nil {
				t.Error(err)
				return
			}
			mu.Lock()
			ratios = next
			mu.Unlock()
			operationsReply(t, w, nil)
		default:
			t.Errorf("意外请求: %s", r.URL)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	errs := make(chan error, 2)
	go func() {
		errs <- service.newAPIDeleteGroup(context.Background(), server.URL, operationsCredential(), newAPIGroupID("vip"))
	}()
	<-firstRead
	go func() {
		errs <- service.newAPIDeleteGroup(context.Background(), server.URL, operationsCredential(), newAPIGroupID("other"))
	}()
	time.Sleep(30 * time.Millisecond)
	mu.Lock()
	readsBeforeRelease := reads
	mu.Unlock()
	if readsBeforeRelease != 1 {
		close(releaseFirst)
		t.Fatalf("并发配置读取未串行: %d", readsBeforeRelease)
	}
	close(releaseFirst)
	for range 2 {
		if err := <-errs; err != nil {
			t.Fatal(err)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if len(ratios) != 1 || string(ratios["default"]) != "1" {
		t.Fatalf("并发删除丢失分组倍率: %s", ratios)
	}
}
