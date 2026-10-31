package relay

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/guliansheng/gateway-ops/internal/storage"
)

type pelicanExportAccount struct {
	Platform    string         `json:"platform"`
	Type        string         `json:"type"`
	Credentials map[string]any `json:"credentials"`
	Extra       map[string]any `json:"extra"`
	ProxyKey    *string        `json:"proxy_key"`
}

type pelicanExportProxy struct {
	ProxyKey string `json:"proxy_key"`
	Protocol string `json:"protocol"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Username string `json:"username"`
	Password string `json:"password"`
}

type pelicanTarget struct {
	client    http.Client
	transport *http.Transport
	endpoint  string
	model     string
	token     string
	oauth     bool
	chat      bool
	headers   http.Header
	secrets   []string
}

func pelicanModelMatches(pattern, model string) bool {
	expression := "^" + strings.ReplaceAll(regexp.QuoteMeta(pattern), `\*`, ".*") + "$"
	matched, _ := regexp.MatchString(expression, model)
	return matched
}

func isPelicanTextModel(model string) bool {
	value := strings.ToLower(strings.TrimSpace(model))
	if value == "" {
		return false
	}
	for _, marker := range []string{"embedding", "rerank", "image", "dall-e", "whisper", "tts", "audio", "speech", "transcri", "video", "sora", "moderation"} {
		if strings.Contains(value, marker) {
			return false
		}
	}
	return true
}

func selectPelicanModel(preferred string, models []string) (string, error) {
	priorities := storage.PelicanModelPriorityList(preferred)
	if len(priorities) > 0 {
		for _, candidate := range priorities {
			if !isPelicanTextModel(candidate) {
				continue
			}
			for _, available := range models {
				available = strings.TrimSpace(available)
				if !isPelicanTextModel(available) {
					continue
				}
				if strings.Contains(candidate, "*") {
					if !strings.Contains(available, "*") && pelicanModelMatches(candidate, available) {
						return available, nil
					}
					continue
				}
				if pelicanModelMatches(available, candidate) {
					return candidate, nil
				}
			}
		}
		return "", fmt.Errorf("\u8be5\u8d26\u53f7\u4e0d\u652f\u6301\u7cfb\u7edf\u914d\u7f6e\u7684\u6d4b\u667a\u6a21\u578b %q\uff1b\u53ef\u7528\u6a21\u578b: %s", strings.Join(priorities, "\u3001"), strings.Join(models, ", "))
	}
	return "", errors.New("\u8bf7\u5148\u5728\u7cfb\u7edf\u8bbe\u7f6e\u4e2d\u914d\u7f6e\u9e48\u9e55\u6d4b\u667a\u6a21\u578b\u4f18\u5148\u5217\u8868")
}

func (s *Service) preparePelican(ctx context.Context, station storage.RelayStation, key string, config storage.PelicanConfig) (*pelicanTarget, error) {
	client := *s.client
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	admin := &Service{client: &client}
	models, err := admin.accountModelsFromUpstream(ctx, strings.TrimRight(station.BaseURL, "/"), key, config.AccountExternalID)
	if err != nil {
		return nil, fmt.Errorf("读取账号模型目录失败，未发送测智请求: %w", err)
	}
	selectedModel, err := selectPelicanModel(config.Model, models)
	if err != nil {
		return nil, err
	}
	var exported struct {
		Accounts []pelicanExportAccount `json:"accounts"`
		Proxies  []pelicanExportProxy   `json:"proxies"`
	}
	endpoint := fmt.Sprintf("%s/api/v1/admin/accounts/data?ids=%d&include_proxies=true", strings.TrimRight(station.BaseURL, "/"), config.AccountExternalID)
	if err := admin.get(ctx, endpoint, key, &exported); err != nil {
		return nil, fmt.Errorf("读取指定账号凭据失败（管理员需具备账号导出权限）: %w", err)
	}
	if len(exported.Accounts) != 1 {
		return nil, errors.New("上游未返回唯一的指定账号，未发送测智请求")
	}
	account := exported.Accounts[0]
	if account.Platform != "openai" || (account.Type != "apikey" && account.Type != "oauth") {
		return nil, errors.New("当前鹈鹕直连生成支持 OpenAI 协议的 API Key 和 OAuth 账号")
	}
	credential := func(name string) string {
		value, _ := account.Credentials[name].(string)
		return strings.TrimSpace(value)
	}
	target := &pelicanTarget{client: *s.client, model: selectedModel, oauth: account.Type == "oauth", headers: make(http.Header), secrets: []string{key, credential("api_key"), credential("access_token"), credential("refresh_token")}}
	target.token = credential("api_key")
	if target.oauth {
		target.token = credential("access_token")
	}
	if target.token == "" || strings.Contains(target.token, "***") {
		return nil, errors.New("指定账号未提供可用凭据，未发送测智请求")
	}
	var mapping map[string]string
	if raw, err := json.Marshal(account.Credentials["model_mapping"]); err == nil {
		_ = json.Unmarshal(raw, &mapping)
	}
	if mapped := mapping[selectedModel]; mapped != "" {
		target.model = mapped
	} else {
		patterns := make([]string, 0, len(mapping))
		for pattern := range mapping {
			patterns = append(patterns, pattern)
		}
		sort.Slice(patterns, func(i, j int) bool {
			if len(patterns[i]) == len(patterns[j]) {
				return patterns[i] < patterns[j]
			}
			return len(patterns[i]) > len(patterns[j])
		})
		for _, pattern := range patterns {
			if mapping[pattern] != "" && pelicanModelMatches(pattern, selectedModel) {
				target.model = mapping[pattern]
				break
			}
		}
	}
	base := credential("base_url")
	if base == "" {
		base = "https://api.openai.com"
	}
	suffix := "responses"
	if !target.oauth {
		mode, _ := account.Extra["openai_responses_mode"].(string)
		supported, known := account.Extra["openai_responses_supported"].(bool)
		target.chat = mode == "force_chat_completions" || (mode != "force_responses" && known && !supported)
		if target.chat {
			suffix = "chat/completions"
		}
	}
	if target.oauth {
		target.endpoint = "https://chatgpt.com/backend-api/codex/responses"
	} else {
		target.endpoint, err = pelicanUpstreamURL(base, suffix)
		if err != nil {
			return nil, err
		}
	}
	if ua := credential("user_agent"); ua != "" {
		target.headers.Set("User-Agent", ua)
	}
	if target.oauth {
		target.headers.Set("OpenAI-Beta", "responses=experimental")
		target.headers.Set("Originator", "codex_cli_rs")
		if target.headers.Get("User-Agent") == "" {
			target.headers.Set("User-Agent", "codex_cli_rs/0.116.0")
		}
		if id := credential("chatgpt_account_id"); id != "" {
			target.headers.Set("ChatGPT-Account-Id", id)
		}
	}
	target.client.Timeout = pelicanTimeout
	target.client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	if account.ProxyKey != nil && *account.ProxyKey != "" {
		var proxy *pelicanExportProxy
		for i := range exported.Proxies {
			if exported.Proxies[i].ProxyKey == *account.ProxyKey {
				proxy = &exported.Proxies[i]
				break
			}
		}
		if proxy == nil {
			return nil, errors.New("账号代理配置不完整，不允许绕过代理发送测智请求")
		}
		proxyURL, err := pelicanProxyURL(*proxy)
		if err != nil {
			return nil, err
		}
		transport := s.client.Transport
		if transport == nil {
			transport = http.DefaultTransport
		}
		original, ok := transport.(*http.Transport)
		if !ok {
			return nil, errors.New("当前传输层不支持账号代理")
		}
		target.transport = original.Clone()
		target.transport.Proxy = http.ProxyURL(proxyURL)
		target.client.Transport = target.transport
		target.secrets = append(target.secrets, proxy.Password, proxyURL.String())
	}
	return target, nil
}

func (s *Service) preparePelicanWithManualKey(ctx context.Context, station storage.RelayStation, account storage.RelayAccount, key string, config storage.PelicanConfig, adminKey string) (*pelicanTarget, error) {
	if strings.TrimSpace(account.BaseURL) == "" {
		return nil, errors.New("关联中转站账号没有上游 Base URL，无法使用渠道测智 Key")
	}
	client := *s.client
	client.Timeout = pelicanTimeout
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	admin := &Service{client: &client}
	models, err := admin.accountModelsFromUpstream(ctx, strings.TrimRight(station.BaseURL, "/"), adminKey, config.AccountExternalID)
	if err != nil {
		return nil, err
	}
	selectedModel, err := selectPelicanModel(config.Model, models)
	if err != nil {
		return nil, err
	}
	if selectedModel == "" {
		return nil, errors.New("请先在系统设置中配置测智模型")
	}
	endpoint, err := pelicanOpenAIChatURL(account.BaseURL)
	if err != nil {
		return nil, err
	}
	return &pelicanTarget{
		client: client, endpoint: endpoint, model: selectedModel, token: key, chat: true,
		headers: make(http.Header), secrets: []string{key, adminKey},
	}, nil
}

func (s *Service) prepareDirectPelican(channel storage.Channel, key string, config storage.PelicanConfig) (*pelicanTarget, error) {
	if strings.TrimSpace(key) == "" {
		return nil, errors.New("渠道未配置测智 API Key")
	}
	models := storage.PelicanModelPriorityList(config.Model)
	if len(models) == 0 || strings.Contains(models[0], "*") {
		return nil, errors.New("直连测智需要配置具体模型")
	}
	endpoint, err := pelicanOpenAIChatURL(channel.SiteURL)
	if err != nil {
		return nil, err
	}
	client := *s.client
	client.Timeout = pelicanTimeout
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &pelicanTarget{client: client, endpoint: endpoint, model: models[0], token: key, chat: true, headers: make(http.Header), secrets: []string{key}}, nil
}

func (s *Service) prepareNewAPIPelican(ctx context.Context, station storage.RelayStation, credential newAPICredential, accountID int64, key string, account storage.RelayAccount, config storage.PelicanConfig) (*pelicanTarget, error) {
	if strings.TrimSpace(key) == "" {
		return nil, errors.New("NewAPI 测智需要在关联的监控渠道中配置测智 API Key")
	}
	if accountID <= 0 {
		return nil, errors.New("NewAPI 渠道 ID 无效")
	}
	envelope, err := s.newAPIRequest(ctx, http.MethodGet, fmt.Sprintf("%s/api/channel/%d", strings.TrimRight(station.BaseURL, "/"), accountID), credential, nil)
	if err != nil {
		return nil, fmt.Errorf("读取 NewAPI 渠道信息失败: %w", err)
	}
	var remote newAPIChannel
	if err := json.Unmarshal(envelope.Data, &remote); err != nil || remote.ID != accountID {
		return nil, errors.New("NewAPI 未返回匹配的渠道信息")
	}
	model, err := selectPelicanModel(config.Model, newAPISplit(remote.Models))
	if err != nil {
		return nil, err
	}
	base := strings.TrimSpace(remote.BaseURL)
	if base == "" {
		base = account.BaseURL
	}
	endpoint, err := pelicanOpenAIChatURL(base)
	if err != nil {
		return nil, err
	}
	client := *s.client
	client.Timeout = pelicanTimeout
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &pelicanTarget{
		client: client, endpoint: endpoint, model: model, token: key, chat: true,
		headers: make(http.Header), secrets: []string{key, credential.Token, credential.Cookie, credential.Password},
	}, nil
}

func (s *Service) preparePelicanForStation(ctx context.Context, station storage.RelayStation, relayCredential, manualKey string, account storage.RelayAccount, config storage.PelicanConfig) (*pelicanTarget, error) {
	if station.Type == string(storage.ChannelTypeNewAPI) {
		credential, ok := decodeNewAPICredential(relayCredential)
		if !ok {
			return nil, errors.New("NewAPI 管理凭据格式无效")
		}
		return s.prepareNewAPIPelican(ctx, station, credential, config.AccountExternalID, manualKey, account, config)
	}
	if strings.TrimSpace(manualKey) != "" {
		return s.preparePelicanWithManualKey(ctx, station, account, manualKey, config, relayCredential)
	}
	return s.preparePelican(ctx, station, relayCredential, config)
}

func pelicanOpenAIChatURL(base string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(base))
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return "", errors.New("上游 Base URL 无效")
	}
	path := strings.TrimRight(u.Path, "/")
	for _, ending := range []string{"/chat/completions", "/responses", "/models"} {
		path = strings.TrimSuffix(path, ending)
	}
	if !strings.HasSuffix(path, "/v1") && !strings.HasSuffix(path, "/v4") {
		path += "/v1"
	}
	u.Path, u.RawPath = path+"/chat/completions", ""
	return u.String(), nil
}

func pelicanUpstreamURL(base, suffix string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(base))
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return "", errors.New("账号上游 Base URL 无效")
	}
	path := strings.TrimRight(u.Path, "/")
	for _, ending := range []string{"/chat/completions", "/responses"} {
		path = strings.TrimSuffix(path, ending)
	}
	if !strings.HasSuffix(path, "/v1") {
		path += "/v1"
	}
	u.Path, u.RawPath = path+"/"+suffix, ""
	return u.String(), nil
}

func pelicanProxyURL(proxy pelicanExportProxy) (*url.URL, error) {
	switch proxy.Protocol {
	case "http", "https", "socks5", "socks5h":
	default:
		return nil, errors.New("账号代理协议不支持")
	}
	if proxy.Host == "" || proxy.Port < 1 || proxy.Port > 65535 {
		return nil, errors.New("账号代理地址无效")
	}
	u := &url.URL{Scheme: proxy.Protocol, Host: net.JoinHostPort(proxy.Host, strconv.Itoa(proxy.Port))}
	if proxy.Username != "" {
		u.User = url.UserPassword(proxy.Username, proxy.Password)
	}
	return u, nil
}

func (t *pelicanTarget) close() {
	if t.transport != nil {
		t.transport.CloseIdleConnections()
	}
}
func (t *pelicanTarget) redact(text string) string {
	for _, secret := range t.secrets {
		if secret != "" {
			text = strings.ReplaceAll(text, secret, "[REDACTED]")
		}
	}
	return text
}

func (t *pelicanTarget) generate(ctx context.Context, config storage.PelicanConfig) (html string, err error) {
	defer func() {
		html = t.redact(html)
		if err != nil {
			err = errors.New(t.redact(err.Error()))
		}
	}()
	prompt := config.Prompt + pelicanContract
	payload := map[string]any{"model": t.model, "stream": true}
	if t.chat {
		payload["messages"] = []map[string]string{{"role": "user", "content": prompt}}
		if config.ReasoningEffort != "" {
			payload["reasoning_effort"] = config.ReasoningEffort
		}
	} else {
		payload["input"] = []map[string]any{{"role": "user", "content": []map[string]string{{"type": "input_text", "text": prompt}}}}
		payload["store"] = false
		if config.ReasoningEffort != "" {
			payload["reasoning"] = map[string]string{"effort": config.ReasoningEffort}
		}
		if t.oauth {
			payload["instructions"] = "遵循用户题目，只输出完整的独立 HTML 文档。"
		}
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.endpoint, bytes.NewReader(encoded))
	if err != nil {
		return "", err
	}
	req.Header = t.headers.Clone()
	req.Header.Set("Authorization", "Bearer "+t.token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream, application/json")
	response, err := t.client.Do(req)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		data, err := io.ReadAll(io.LimitReader(response.Body, 64<<10))
		if err != nil {
			return "", err
		}
		return "", fmt.Errorf("HTTP %d: %s", response.StatusCode, redactTestText(t.redact(string(data)), ""))
	}
	return readPelicanResponse(response.Body)
}
