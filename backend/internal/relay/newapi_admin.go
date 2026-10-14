package relay

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/guliansheng/gateway-ops/internal/storage"
)

const newAPICredentialPrefix = "gatewayops:newapi:"

const relayStationSub2API = "sub2api"
const relayStationNewAPI = "newapi"

type newAPICredential struct {
	AuthMode   string `json:"auth_mode,omitempty"`
	Username   string `json:"username,omitempty"`
	Password   string `json:"password,omitempty"`
	Token      string `json:"token,omitempty"`
	Cookie     string `json:"cookie,omitempty"`
	UserID     string `json:"user_id,omitempty"`
	GatewayKey string `json:"gateway_key,omitempty"`
}

func normalizeRelayStationType(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return relayStationSub2API
	}
	return value
}

func relayCredentialFromCreate(in CreateInput, stationType string) newAPICredential {
	if stationType == relayStationSub2API {
		return newAPICredential{GatewayKey: strings.TrimSpace(in.APIKey)}
	}
	return newAPICredential{AuthMode: in.AuthMode, Username: in.Username, Password: in.Password, Token: in.AdminToken, Cookie: in.AdminCookie, UserID: in.AdminUserID, GatewayKey: in.GatewayKey}
}

func encodeRelayCredential(stationType string, credential newAPICredential) (string, error) {
	if stationType != relayStationSub2API && stationType != relayStationNewAPI {
		return "", errors.New("中转站类型只支持 sub2api 或 newapi")
	}
	if stationType == relayStationSub2API {
		if strings.TrimSpace(credential.GatewayKey) == "" {
			return "", errors.New("名称和管理员 API Key 不能为空")
		}
		return strings.TrimSpace(credential.GatewayKey), nil
	}
	return encodeNewAPICredential(credential)
}

func relayCredentialProvided(in UpdateInput) bool {
	for _, value := range []*string{in.AuthMode, in.APIKey, in.Username, in.Password, in.AdminToken, in.AdminCookie, in.AdminUserID, in.GatewayKey} {
		if value != nil && strings.TrimSpace(*value) != "" {
			return true
		}
	}
	return false
}

func mergeRelayCredential(stationType string, previous newAPICredential, in UpdateInput) newAPICredential {
	if stationType == relayStationSub2API {
		if in.APIKey != nil && strings.TrimSpace(*in.APIKey) != "" {
			previous.GatewayKey = strings.TrimSpace(*in.APIKey)
		}
		return previous
	}
	if in.AuthMode != nil && strings.TrimSpace(*in.AuthMode) != "" {
		previous.AuthMode = strings.TrimSpace(*in.AuthMode)
	}
	if in.Username != nil && strings.TrimSpace(*in.Username) != "" {
		previous.Username = strings.TrimSpace(*in.Username)
	}
	if in.Password != nil && strings.TrimSpace(*in.Password) != "" {
		previous.Password = *in.Password
	}
	if in.AdminToken != nil && strings.TrimSpace(*in.AdminToken) != "" {
		previous.Token = strings.TrimSpace(*in.AdminToken)
	}
	if in.AdminCookie != nil && strings.TrimSpace(*in.AdminCookie) != "" {
		previous.Cookie = strings.TrimSpace(*in.AdminCookie)
	}
	if in.AdminUserID != nil && strings.TrimSpace(*in.AdminUserID) != "" {
		previous.UserID = strings.TrimSpace(*in.AdminUserID)
	}
	if in.GatewayKey != nil && strings.TrimSpace(*in.GatewayKey) != "" {
		previous.GatewayKey = strings.TrimSpace(*in.GatewayKey)
	}
	return previous
}

func (s *Service) decryptRelayCredential(station *storage.RelayStation) (newAPICredential, error) {
	raw, err := s.cipher.Decrypt(station.APIKeyCipher)
	if err != nil {
		return newAPICredential{}, fmt.Errorf("decrypt relay credential: %w", err)
	}
	if station.Type == relayStationNewAPI {
		credential, ok := decodeNewAPICredential(raw)
		if !ok {
			return newAPICredential{}, errors.New("New API 管理凭据格式无效，请重新填写管理员凭据")
		}
		return credential, nil
	}
	return newAPICredential{GatewayKey: raw}, nil
}

func encodeNewAPICredential(c newAPICredential) (string, error) {
	c.Username = strings.TrimSpace(c.Username)
	c.Token = strings.TrimSpace(c.Token)
	c.Cookie = strings.TrimSpace(c.Cookie)
	c.UserID = strings.TrimSpace(c.UserID)
	c.GatewayKey = strings.TrimSpace(c.GatewayKey)
	if c.AuthMode == "" {
		switch {
		case c.Username != "":
			c.AuthMode = "password"
		case c.Token != "":
			c.AuthMode = "access_token"
		case c.Cookie != "":
			c.AuthMode = "cookie"
		}
	}
	switch c.AuthMode {
	case "password":
		if c.Username == "" {
			return "", errors.New("New API 管理员用户名不能为空")
		}
		if c.Password == "" {
			return "", errors.New("New API 管理员密码不能为空")
		}
		c.Token, c.Cookie, c.UserID = "", "", ""
	case "access_token", "cookie":
		if c.AuthMode == "access_token" {
			c.Cookie = ""
			if c.Token == "" {
				return "", errors.New("New API 管理员访问令牌不能为空")
			}
		}
		if c.AuthMode == "cookie" {
			c.Token = ""
			if c.Cookie == "" {
				return "", errors.New("New API 管理员 Cookie 不能为空")
			}
		}
		id, err := strconv.ParseInt(c.UserID, 10, 64)
		if err != nil || id <= 0 {
			return "", errors.New("New API 用户 ID 必须为正整数")
		}
		c.Username, c.Password = "", ""
	default:
		return "", errors.New("请配置 New API 管理员账号密码或访问凭据")
	}
	raw, err := json.Marshal(c)
	return newAPICredentialPrefix + string(raw), err
}

func decodeNewAPICredential(raw string) (newAPICredential, bool) {
	var c newAPICredential
	if !strings.HasPrefix(raw, newAPICredentialPrefix) {
		return c, false
	}
	return c, json.Unmarshal([]byte(strings.TrimPrefix(raw, newAPICredentialPrefix)), &c) == nil
}

func newAPIGroupID(name string) int64 {
	if strings.TrimSpace(name) == "" {
		return 0
	}
	sum := sha256.Sum256([]byte(name))
	// JavaScript 必须能无损表示该标识；最多使用 52 位。
	return int64(binary.BigEndian.Uint64(sum[:8])&((1<<52)-1)) + 1
}

func newAPIPriority(native int) int { return 1000 - native }

type newAPIEnvelope struct {
	Success    bool               `json:"success"`
	Code       string             `json:"code"`
	Message    string             `json:"message"`
	Data       json.RawMessage    `json:"data"`
	GroupRatio map[string]float64 `json:"group_ratio"`
	HTTPStatus int                `json:"-"`
}

type newAPISession struct {
	Token, Cookie, UserID string
	Expires               time.Time
}
type newAPISessionEntry struct {
	mu           sync.Mutex
	session      newAPISession
	quotaMu      sync.Mutex
	quota        float64
	quotaExpires time.Time
}

func (s *Service) newAPISessionEntry(base string, c newAPICredential) *newAPISessionEntry {
	raw, _ := json.Marshal(c)
	digest := sha256.Sum256(append([]byte(base), raw...))
	key := fmt.Sprintf("%x", digest)
	s.newAPIMu.Lock()
	defer s.newAPIMu.Unlock()
	if s.newAPISessions == nil {
		s.newAPISessions = make(map[string]*newAPISessionEntry)
	}
	entry := s.newAPISessions[key]
	if entry == nil {
		entry = &newAPISessionEntry{}
		s.newAPISessions[key] = entry
	}
	return entry
}

func (s *Service) newAPIRequest(ctx context.Context, method, endpoint string, c newAPICredential, body any) (newAPIEnvelope, error) {
	return s.newAPIRequestWithHeaders(ctx, method, endpoint, c, body, nil)
}

func (s *Service) newAPIRequestWithHeaders(ctx context.Context, method, endpoint string, c newAPICredential, body any, headers map[string]string) (newAPIEnvelope, error) {
	if err := validateNewAPITransport(endpoint); err != nil {
		return newAPIEnvelope{}, err
	}
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return newAPIEnvelope{}, err
	}
	base := parsed.Scheme + "://" + parsed.Host
	if index := strings.Index(parsed.Path, "/api/"); index >= 0 {
		base += parsed.Path[:index]
	}
	entry := s.newAPISessionEntry(base, c)
	for attempt := 0; attempt < 2; attempt++ {
		entry.mu.Lock()
		if c.Username != "" && (entry.session.UserID == "" || time.Now().Add(time.Minute).After(entry.session.Expires)) {
			session, loginErr := s.newAPILogin(ctx, base, c)
			if loginErr != nil {
				entry.mu.Unlock()
				return newAPIEnvelope{}, errors.New(sanitizeGatewayMessage(loginErr.Error(), c.Password, c.Token, c.Cookie, c.GatewayKey))
			}
			entry.session = session
		}
		session := entry.session
		entry.mu.Unlock()
		if c.Username == "" {
			session = newAPISession{Token: c.Token, Cookie: c.Cookie, UserID: c.UserID}
		}
		envelope, status, requestErr := s.newAPIRawRequestWithHeaders(ctx, method, endpoint, session, body, headers)
		if requestErr == nil {
			return envelope, nil
		}
		if attempt == 0 && c.Username != "" && (status == http.StatusUnauthorized || strings.Contains(strings.ToLower(envelope.Message), "login") || strings.Contains(envelope.Message, "未登录") || strings.Contains(envelope.Message, "登录已过期")) {
			entry.mu.Lock()
			if entry.session.Token == session.Token && entry.session.Cookie == session.Cookie {
				entry.session = newAPISession{}
			}
			entry.mu.Unlock()
			continue
		}
		message := requestErr.Error()
		for _, secret := range []string{c.Password, c.Token, c.Cookie, c.GatewayKey, session.Token, session.Cookie} {
			if secret != "" {
				message = strings.ReplaceAll(message, secret, "[已隐藏]")
			}
		}
		return envelope, errors.New(message)
	}
	return newAPIEnvelope{}, errors.New("New API 管理会话失效")
}

func validateNewAPITransport(endpoint string) error {
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return err
	}
	if parsed.Scheme != "https" && parsed.Scheme != "http" {
		return errors.New("New API 地址必须使用 HTTPS")
	}
	if parsed.Scheme == "http" {
		host := strings.ToLower(parsed.Hostname())
		if host != "localhost" && host != "127.0.0.1" && host != "::1" {
			return errors.New("New API 管理凭据需要 HTTPS 传输；请更新站点地址为 HTTPS 后重试")
		}
	}
	return nil
}

func (s *Service) newAPIVerifyProof(ctx context.Context, base string, c newAPICredential, scope string, operationContext any) (string, error) {
	if err := validateNewAPITransport(base + "/api/verify"); err != nil {
		return "", err
	}
	if err := requireNewAPIPasswordCredential(c); err != nil {
		return "", err
	}
	rawContext, err := json.Marshal(operationContext)
	if err != nil {
		return "", err
	}
	body, err := s.newAPIPasswordPayload(ctx, base, c.Password)
	if err != nil {
		return "", err
	}
	body["method"] = "password"
	body["scope"] = scope
	body["context"] = json.RawMessage(rawContext)
	envelope, err := s.newAPIRequest(ctx, http.MethodPost, strings.TrimRight(base, "/")+"/api/verify", c, body)
	if err != nil {
		return "", fmt.Errorf("New API 安全验证失败: %w", err)
	}
	var proof struct {
		ProofToken string `json:"proof_token"`
	}
	if err := json.Unmarshal(envelope.Data, &proof); err != nil {
		return "", fmt.Errorf("New API 安全验证响应无效: %w", err)
	}
	if strings.TrimSpace(proof.ProofToken) == "" {
		return "", errors.New("New API 安全验证没有返回一次性凭据")
	}
	return proof.ProofToken, nil
}

func requireNewAPIPasswordCredential(c newAPICredential) error {
	if c.Username == "" || c.Password == "" {
		return errors.New("New API 用户管理操作需要密码模式凭据以完成安全验证；访问令牌或 Cookie 凭据请在 New API 控制台操作")
	}
	return nil
}

func (s *Service) newAPIProtectedRequest(ctx context.Context, method, base, path string, c newAPICredential, body any, scope string, operationContext any) error {
	methodsEndpoint := strings.TrimRight(base, "/") + "/api/verify/methods?scope=" + url.QueryEscape(scope)
	envelope, err := s.newAPIRequest(ctx, http.MethodGet, methodsEndpoint, c, nil)
	if err != nil {
		if envelope.HTTPStatus == http.StatusNotFound || envelope.HTTPStatus == http.StatusMethodNotAllowed {
			_, legacyErr := s.newAPIRequest(ctx, method, base+path, c, body)
			return legacyErr
		}
		return fmt.Errorf("读取 New API 安全验证方式失败: %w", err)
	}
	var requirements struct {
		Scope   string `json:"scope"`
		Methods []struct {
			Method    string `json:"method"`
			Available bool   `json:"available"`
			Reason    string `json:"reason"`
		} `json:"methods"`
	}
	if err := json.Unmarshal(envelope.Data, &requirements); err != nil {
		return fmt.Errorf("解析 New API 安全验证方式失败: %w", err)
	}
	if requirements.Scope != scope || len(requirements.Methods) == 0 {
		return errors.New("New API 安全验证方式与请求范围不匹配")
	}
	passwordAvailable := false
	availableMethods := make([]string, 0, len(requirements.Methods))
	for _, method := range requirements.Methods {
		if method.Available {
			availableMethods = append(availableMethods, method.Method)
			if method.Method == "password" {
				passwordAvailable = true
			}
		}
	}
	if !passwordAvailable {
		return fmt.Errorf("New API 当前要求交互式安全验证（可用方式：%s），请在 New API 控制台完成此操作", strings.Join(availableMethods, ", "))
	}
	proof, err := s.newAPIVerifyProof(ctx, base, c, scope, operationContext)
	if err != nil {
		return err
	}
	_, err = s.newAPIRequestWithHeaders(ctx, method, base+path, c, body, map[string]string{"X-Security-Proof": proof})
	return err
}

func (s *Service) newAPIRawRequest(ctx context.Context, method, endpoint string, session newAPISession, body any) (newAPIEnvelope, int, error) {
	return s.newAPIRawRequestWithHeaders(ctx, method, endpoint, session, body, nil)
}

func (s *Service) newAPIRawRequestWithHeaders(ctx context.Context, method, endpoint string, session newAPISession, body any, headers map[string]string) (newAPIEnvelope, int, error) {
	var payload io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return newAPIEnvelope{}, 0, err
		}
		payload = strings.NewReader(string(raw))
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, payload)
	if err != nil {
		return newAPIEnvelope{}, 0, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "GatewayOps")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if session.Token != "" {
		req.Header.Set("Authorization", "Bearer "+strings.TrimPrefix(session.Token, "Bearer "))
	}
	if session.Cookie != "" {
		req.Header.Set("Cookie", session.Cookie)
	}
	if session.UserID != "" {
		req.Header.Set("New-Api-User", session.UserID)
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	response, err := s.newAPIHTTPClient().Do(req)
	if err != nil {
		return newAPIEnvelope{}, 0, err
	}
	defer response.Body.Close()
	var envelope newAPIEnvelope
	if err := json.NewDecoder(io.LimitReader(response.Body, 16<<20)).Decode(&envelope); err != nil {
		envelope.HTTPStatus = response.StatusCode
		return envelope, response.StatusCode, fmt.Errorf("New API HTTP %d: 响应不是有效的 JSON", response.StatusCode)
	}
	envelope.HTTPStatus = response.StatusCode
	if response.StatusCode < 200 || response.StatusCode >= 300 || !envelope.Success {
		message := strings.TrimSpace(envelope.Message)
		if message == "" {
			message = "管理请求失败"
		}
		return envelope, response.StatusCode, fmt.Errorf("New API HTTP %d: %s", response.StatusCode, message)
	}
	return envelope, response.StatusCode, nil
}

func (s *Service) newAPIHTTPClient() *http.Client {
	client := s.client
	if client == nil {
		client = http.DefaultClient
	}
	copy := *client
	copy.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &copy
}

func (s *Service) newAPIPasswordPayload(ctx context.Context, base, password string) (map[string]any, error) {
	body := map[string]any{"password": password}
	// 兼容启用 RSA-OAEP 密码加密的新版本，也兼容没有该端点的旧版本。
	encryption, _, err := s.newAPIRawRequest(ctx, http.MethodGet, base+"/api/user/login/encryption-key", newAPISession{}, nil)
	if err != nil {
		return body, nil
	}
	var config struct {
		Enabled   bool   `json:"enabled"`
		KID       string `json:"kid"`
		PublicKey string `json:"public_key"`
	}
	if err := json.Unmarshal(encryption.Data, &config); err != nil {
		return nil, err
	}
	if !config.Enabled {
		return body, nil
	}
	block, _ := pem.Decode([]byte(config.PublicKey))
	if block == nil {
		return nil, errors.New("New API 登录公钥无效")
	}
	key, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	public, ok := key.(*rsa.PublicKey)
	if !ok {
		return nil, errors.New("New API 登录公钥不是 RSA")
	}
	encrypted, err := rsa.EncryptOAEP(sha256.New(), rand.Reader, public, []byte(password), nil)
	if err != nil {
		return nil, err
	}
	delete(body, "password")
	body["password_encrypted"] = base64.StdEncoding.EncodeToString(encrypted)
	body["encryption_key_id"] = config.KID
	return body, nil
}

func (s *Service) newAPILogin(ctx context.Context, base string, c newAPICredential) (newAPISession, error) {
	if err := validateNewAPITransport(base + "/api/user/login"); err != nil {
		return newAPISession{}, err
	}
	body, err := s.newAPIPasswordPayload(ctx, base, c.Password)
	if err != nil {
		return newAPISession{}, err
	}
	body["username"] = c.Username
	raw, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/api/user/login", strings.NewReader(string(raw)))
	if err != nil {
		return newAPISession{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	response, err := s.newAPIHTTPClient().Do(req)
	if err != nil {
		return newAPISession{}, err
	}
	defer response.Body.Close()
	var envelope newAPIEnvelope
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&envelope); err != nil {
		return newAPISession{}, errors.New("New API 登录响应无效")
	}
	if response.StatusCode != http.StatusOK || !envelope.Success {
		return newAPISession{}, fmt.Errorf("New API 登录失败: %s", envelope.Message)
	}
	var data struct {
		ID                   int64  `json:"id"`
		Role                 int    `json:"role"`
		Require2FA           bool   `json:"require_2fa"`
		VerificationRequired bool   `json:"verification_required"`
		Token                string `json:"access_token"`
		Expires              int64  `json:"access_expires_at"`
		User                 struct {
			ID   int64 `json:"id"`
			Role int   `json:"role"`
		} `json:"user"`
	}
	if err := json.Unmarshal(envelope.Data, &data); err != nil {
		return newAPISession{}, err
	}
	if data.Require2FA || data.VerificationRequired {
		return newAPISession{}, errors.New("New API 要求二次验证，请使用已验证的管理员 Cookie 或访问令牌")
	}
	if data.ID == 0 {
		data.ID = data.User.ID
		data.Role = data.User.Role
	}
	if data.ID <= 0 || data.Role < 10 {
		return newAPISession{}, errors.New("New API 登录账号没有管理员权限")
	}
	parts := []string{}
	for _, cookie := range response.Cookies() {
		if cookie.Value != "" {
			parts = append(parts, cookie.Name+"="+cookie.Value)
		}
	}
	session := newAPISession{Token: data.Token, Cookie: strings.Join(parts, "; "), UserID: strconv.FormatInt(data.ID, 10), Expires: time.Now().Add(6 * time.Hour)}
	if data.Expires > 0 {
		session.Expires = time.Unix(data.Expires, 0)
	}
	if session.Token == "" && session.Cookie == "" {
		return session, errors.New("New API 登录没有返回会话凭据")
	}
	return session, nil
}

type newAPIChannel struct {
	ID       int64  `json:"id"`
	Type     int    `json:"type"`
	Name     string `json:"name"`
	Status   int    `json:"status"`
	BaseURL  string `json:"base_url"`
	Group    string `json:"group"`
	Models   string `json:"models"`
	Priority int    `json:"priority"`
}

func newAPIAccount(channel newAPIChannel) remoteAccount {
	groups := []int64{}
	for _, name := range newAPISplit(channel.Group) {
		groups = append(groups, newAPIGroupID(name))
	}
	status := "inactive"
	if channel.Status == 1 {
		status = "active"
	}
	return remoteAccount{ID: channel.ID, Name: channel.Name, Platform: "newapi", Type: "apikey", Status: status, Schedulable: channel.Status == 1, Priority: newAPIPriority(channel.Priority), GroupIDs: groups, Credentials: remoteAccountCredentials{BaseURL: channel.BaseURL}}
}

func newAPISplit(raw string) []string {
	values := []string{}
	seen := map[string]bool{}
	for _, value := range strings.Split(raw, ",") {
		value = strings.TrimSpace(value)
		if value != "" && !seen[value] {
			values = append(values, value)
			seen[value] = true
		}
	}
	return values
}

func assignNewAPI(out, value any) error {
	if out == nil {
		return nil
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, out)
}

func newAPIPageQuery(q url.Values) url.Values {
	out := url.Values{}
	page := q.Get("page")
	if page == "" {
		page = "1"
	}
	size := q.Get("page_size")
	if size == "" {
		size = "100"
	}
	out.Set("p", page)
	out.Set("page_size", size)
	return out
}

type newAPIPage struct {
	Items    json.RawMessage `json:"items"`
	Total    int             `json:"total"`
	Page     int             `json:"page"`
	PageSize int             `json:"page_size"`
}

func decodeNewAPIPage(raw json.RawMessage, q url.Values, out any) (newAPIPage, int, error) {
	page := newAPIPage{}
	if len(raw) > 0 && raw[0] == byte(91) {
		page.Items = raw
	} else if err := json.Unmarshal(raw, &page); err != nil {
		return page, 0, err
	}
	if page.Page <= 0 {
		page.Page, _ = strconv.Atoi(q.Get("p"))
	}
	if page.PageSize <= 0 {
		page.PageSize, _ = strconv.Atoi(q.Get("page_size"))
	}
	if page.PageSize <= 0 {
		page.PageSize = 100
	}
	if len(page.Items) == 0 {
		page.Items = json.RawMessage("[]")
	}
	if err := json.Unmarshal(page.Items, out); err != nil {
		return page, 0, err
	}
	pages := (page.Total + page.PageSize - 1) / page.PageSize
	if pages < 1 {
		pages = 1
	}
	return page, pages, nil
}

func (s *Service) newAPIQuota(ctx context.Context, base string, c newAPICredential) (float64, error) {
	entry := s.newAPISessionEntry(base, c)
	entry.quotaMu.Lock()
	defer entry.quotaMu.Unlock()
	if entry.quota > 0 && time.Now().Before(entry.quotaExpires) {
		return entry.quota, nil
	}
	env, err := s.newAPIRequest(ctx, http.MethodGet, base+"/api/status", c, nil)
	if err != nil {
		return 0, err
	}
	var status struct {
		Quota float64 `json:"quota_per_unit"`
	}
	if err := json.Unmarshal(env.Data, &status); err != nil {
		return 0, err
	}
	if status.Quota <= 0 {
		status.Quota = 500000
	}
	entry.quota, entry.quotaExpires = status.Quota, time.Now().Add(5*time.Minute)
	return status.Quota, nil
}

func (s *Service) newAPIGroups(ctx context.Context, base string, c newAPICredential) ([]remoteGroup, error) {
	env, err := s.newAPIRequest(ctx, http.MethodGet, base+"/api/group/", c, nil)
	if err != nil {
		return nil, err
	}
	var names []string
	if err := json.Unmarshal(env.Data, &names); err != nil {
		return nil, err
	}
	ratios := map[string]float64{}
	options, optionErr := s.newAPIOptions(ctx, base, c)
	if optionErr == nil {
		for _, key := range []string{"GroupRatio", "group_ratio_setting.group_ratio"} {
			if raw, ok := options[key]; ok {
				if err := json.Unmarshal([]byte(raw), &ratios); err != nil {
					return nil, err
				}
				break
			}
		}
	}
	if len(ratios) == 0 {
		pricing, err := s.newAPIRequest(ctx, http.MethodGet, base+"/api/pricing", c, nil)
		if err != nil {
			return nil, err
		}
		ratios = pricing.GroupRatio
	}
	sort.Strings(names)
	groups := make([]remoteGroup, 0, len(names))
	for i, name := range names {
		ratio, ok := ratios[name]
		if !ok {
			return nil, fmt.Errorf("New API 分组 %s 的倍率不可见，请使用具有全局设置读取权限的管理员凭据", name)
		}
		groups = append(groups, remoteGroup{ID: newAPIGroupID(name), Name: name, Platform: "newapi", Status: "active", RateMultiplier: ratio, SortOrder: i})
	}
	return groups, nil
}

func (s *Service) newAPIOptions(ctx context.Context, base string, c newAPICredential) (map[string]string, error) {
	env, err := s.newAPIRequest(ctx, http.MethodGet, base+"/api/option/", c, nil)
	if err != nil {
		return nil, err
	}
	var options []struct {
		Key   string `json:"key"`
		Value string `json:"value"`
	}
	if err := json.Unmarshal(env.Data, &options); err != nil {
		return nil, err
	}
	result := map[string]string{}
	for _, option := range options {
		result[option.Key] = option.Value
	}
	return result, nil
}

func (s *Service) newAPIGroupName(ctx context.Context, base string, c newAPICredential, id int64) (string, error) {
	env, err := s.newAPIRequest(ctx, http.MethodGet, base+"/api/group/", c, nil)
	if err != nil {
		return "", err
	}
	var names []string
	if err := json.Unmarshal(env.Data, &names); err != nil {
		return "", err
	}
	for _, name := range names {
		if newAPIGroupID(name) == id {
			return name, nil
		}
	}
	return "", errors.New("New API 分组不存在，请同步后重试")
}

func newAPIObject(body any) (map[string]any, error) {
	var object map[string]any
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(raw, &object)
	return object, err
}

func (s *Service) newAPIUpdateChannel(ctx context.Context, base string, c newAPICredential, id int64, body any) error {
	updates, err := newAPIObject(body)
	if err != nil {
		return err
	}
	var desiredStatus *int
	if value, ok := updates["schedulable"]; ok {
		enabled, ok := value.(bool)
		if !ok {
			return errors.New("调度参数无效")
		}
		status := 2
		if enabled {
			status = 1
		}
		desiredStatus = &status
		delete(updates, "schedulable")
	}
	if len(updates) == 0 {
		if desiredStatus == nil {
			return errors.New("New API 账号更新内容为空")
		}
		return s.newAPIUpdateChannelStatus(ctx, base, c, id, *desiredStatus)
	}
	env, err := s.newAPIRequest(ctx, http.MethodGet, fmt.Sprintf("%s/api/channel/%d", base, id), c, nil)
	if err != nil {
		return err
	}
	var channel map[string]any
	if err := json.Unmarshal(env.Data, &channel); err != nil {
		return err
	}
	for key, value := range updates {
		switch key {
		case "group_ids":
			ids, ok := value.([]any)
			if !ok {
				return errors.New("分组参数无效")
			}
			names := []string{}
			for _, rawID := range ids {
				number, ok := rawID.(float64)
				if !ok {
					return errors.New("分组 ID 无效")
				}
				name, err := s.newAPIGroupName(ctx, base, c, int64(number))
				if err != nil {
					return err
				}
				names = append(names, name)
			}
			channel["group"] = strings.Join(names, ",")
		case "priority":
			number, ok := value.(float64)
			if !ok {
				return errors.New("优先级参数无效")
			}
			channel["priority"] = 1000 - int(number)
		case "name", "models", "base_url":
			channel[key] = value
		case "credentials":
			credentials, ok := value.(map[string]any)
			if !ok {
				return errors.New("渠道凭据无效")
			}
			for field, fieldValue := range credentials {
				switch field {
				case "api_key":
					channel["key"] = fieldValue
				case "base_url":
					channel["base_url"] = fieldValue
				default:
					return fmt.Errorf("New API 不支持账号字段 %s", field)
				}
			}
		default:
			return fmt.Errorf("New API 不支持账号字段 %s", key)
		}
	}
	delete(channel, "key")
	delete(channel, "status")
	if credentials, ok := updates["credentials"].(map[string]any); ok {
		if key, ok := credentials["api_key"]; ok {
			channel["key"] = key
		}
	}
	channel["id"] = id
	if _, err := s.newAPIRequest(ctx, http.MethodPut, base+"/api/channel/", c, channel); err != nil {
		return err
	}
	if desiredStatus != nil {
		if err := s.newAPIUpdateChannelStatus(ctx, base, c, id, *desiredStatus); err != nil {
			return fmt.Errorf("渠道字段已更新，但调度状态更新失败: %w", err)
		}
	}
	return nil
}

func (s *Service) newAPIUpdateChannelStatus(ctx context.Context, base string, c newAPICredential, id int64, status int) error {
	base = strings.TrimRight(base, "/")
	env, err := s.newAPIRequest(ctx, http.MethodPost, fmt.Sprintf("%s/api/channel/%d/status", base, id), c, map[string]int{"status": status})
	if err != nil {
		return err
	}
	var changed bool
	if err := json.Unmarshal(env.Data, &changed); err != nil {
		return fmt.Errorf("New API 渠道状态响应无效: %w", err)
	}
	if changed {
		return nil
	}
	check, err := s.newAPIRequest(ctx, http.MethodGet, fmt.Sprintf("%s/api/channel/%d", base, id), c, nil)
	if err != nil {
		return fmt.Errorf("New API 未确认渠道调度状态；读取远端状态失败: %w", err)
	}
	var channel struct {
		ID     int64 `json:"id"`
		Status int   `json:"status"`
	}
	if err := json.Unmarshal(check.Data, &channel); err != nil {
		return fmt.Errorf("New API 未确认渠道调度状态；远端渠道响应无效: %w", err)
	}
	if channel.ID != id || channel.Status != status {
		return fmt.Errorf("New API 未确认渠道调度状态；远端状态仍为 %d", channel.Status)
	}
	return nil
}

func (s *Service) newAPIUpdateGroup(ctx context.Context, base string, c newAPICredential, id int64, body any) error {
	updates, err := newAPIObject(body)
	if err != nil {
		return err
	}
	name, err := s.newAPIGroupName(ctx, base, c, id)
	if err != nil {
		return err
	}
	for key, value := range updates {
		switch key {
		case "rate_multiplier":
		case "name":
			if value != name {
				return errors.New("New API 分组名称同时关联用户、令牌和渠道，请在原站完成重命名后重新同步")
			}
		default:
			return fmt.Errorf("New API 不支持分组字段 %s", key)
		}
	}
	value, exists := updates["rate_multiplier"]
	if !exists {
		return nil
	}
	s.newAPIOptionsMu.Lock()
	defer s.newAPIOptionsMu.Unlock()
	options, err := s.newAPIOptions(ctx, base, c)
	if err != nil {
		return err
	}
	key := "GroupRatio"
	if _, ok := options["group_ratio_setting.group_ratio"]; ok {
		key = "group_ratio_setting.group_ratio"
	}
	raw, ok := options[key]
	if !ok {
		return errors.New("New API 分组倍率配置不存在")
	}
	ratios := map[string]float64{}
	if err := json.Unmarshal([]byte(raw), &ratios); err != nil {
		return err
	}
	number, ok := value.(float64)
	if !ok || number < 0 {
		return errors.New("分组倍率无效")
	}
	ratios[name] = number
	encoded, _ := json.Marshal(ratios)
	_, err = s.newAPIRequest(ctx, http.MethodPut, base+"/api/option/", c, map[string]any{"key": key, "value": string(encoded)})
	return err
}

type newAPIUser struct {
	ID        int64   `json:"id"`
	Username  string  `json:"username"`
	Email     string  `json:"email"`
	Role      int     `json:"role"`
	Status    int     `json:"status"`
	Quota     float64 `json:"quota"`
	UsedQuota float64 `json:"used_quota"`
	CreatedAt int64   `json:"created_at"`
	LastLogin int64   `json:"last_login_at"`
}

func newAPIUserView(user newAPIUser, quota float64) remoteUser {
	role := "user"
	if user.Role >= 100 {
		role = "root"
	} else if user.Role >= 10 {
		role = "admin"
	}
	status := "inactive"
	if user.Status == 1 {
		status = "active"
	}
	created := time.Time{}
	if user.CreatedAt > 0 {
		created = time.Unix(user.CreatedAt, 0).UTC()
	}
	return remoteUser{ID: user.ID, Username: user.Username, Email: user.Email, Role: role, Status: status, Balance: user.Quota / quota, CreatedAt: created}
}

type newAPILog struct {
	ID               int64   `json:"id"`
	UserID           int64   `json:"user_id"`
	Username         string  `json:"username"`
	Channel          int64   `json:"channel"`
	ChannelName      string  `json:"channel_name"`
	Type             int     `json:"type"`
	Quota            float64 `json:"quota"`
	PromptTokens     int64   `json:"prompt_tokens"`
	CompletionTokens int64   `json:"completion_tokens"`
	CreatedAt        int64   `json:"created_at"`
	Model            string  `json:"model_name"`
	Group            string  `json:"group"`
	UseTime          int64   `json:"use_time"`
	IsStream         bool    `json:"is_stream"`
	RequestID        string  `json:"request_id"`
	Other            string  `json:"other"`
	Content          string  `json:"content"`
	IP               string  `json:"ip"`
}

func (s *Service) newAPILogs(ctx context.Context, base string, c newAPICredential, q url.Values) (remoteUsagePage, error) {
	quota, err := s.newAPIQuota(ctx, base, c)
	if err != nil {
		return remoteUsagePage{}, err
	}
	query := newAPIPageQuery(q)
	for _, key := range []string{"username", "token_name", "group", "channel", "model_name", "request_id", "start_timestamp", "end_timestamp"} {
		if value := q.Get(key); value != "" {
			query.Set(key, value)
		}
	}
	for _, mapping := range [][2]string{{"account_id", "channel"}, {"model", "model_name"}, {"start_date", "start_timestamp"}, {"end_date", "end_timestamp"}, {"start_time", "start_timestamp"}, {"end_time", "end_timestamp"}} {
		from, to := mapping[0], mapping[1]
		if value := q.Get(from); value != "" {
			if strings.HasSuffix(to, "timestamp") {
				if date, err := time.Parse(time.RFC3339Nano, value); err == nil {
					value = strconv.FormatInt(date.Unix(), 10)
				} else if date, err := time.Parse("2006-01-02", value); err == nil {
					if from == "end_date" {
						date = date.Add(24*time.Hour - time.Second)
					}
					value = strconv.FormatInt(date.Unix(), 10)
				}
			}
			query.Set(to, value)
		}
	}
	if userID := q.Get("user_id"); userID != "" {
		id, err := strconv.ParseInt(userID, 10, 64)
		if err != nil || id <= 0 {
			return remoteUsagePage{}, errors.New("New API 用户 ID 无效")
		}
		env, err := s.newAPIRequest(ctx, http.MethodGet, fmt.Sprintf("%s/api/user/%d", base, id), c, nil)
		if err != nil {
			return remoteUsagePage{}, err
		}
		var user newAPIUser
		if err := json.Unmarshal(env.Data, &user); err != nil {
			return remoteUsagePage{}, err
		}
		if user.ID != id || user.Username == "" {
			return remoteUsagePage{}, errors.New("New API 用户不存在或缺少用户名")
		}
		query.Set("username", user.Username)
	}
	if id, err := strconv.ParseInt(q.Get("group_id"), 10, 64); err == nil && id > 0 {
		name, err := s.newAPIGroupName(ctx, base, c, id)
		if err != nil {
			return remoteUsagePage{}, err
		}
		query.Set("group", name)
	}
	if kind := q.Get("type"); kind != "" {
		query.Set("type", kind)
	}
	env, err := s.newAPIRequest(ctx, http.MethodGet, base+"/api/log/?"+query.Encode(), c, nil)
	if err != nil {
		return remoteUsagePage{}, err
	}
	var logs []newAPILog
	page, pages, err := decodeNewAPIPage(env.Data, query, &logs)
	if err != nil {
		return remoteUsagePage{}, err
	}
	out := remoteUsagePage{Items: []remoteUsage{}, Total: page.Total, Page: page.Page, PageSize: page.PageSize, Pages: pages}
	for _, log := range logs {
		if log.Type != 2 && log.Type != 5 {
			continue
		}
		status := 200
		if log.Type == 5 {
			status = 502
		}
		requestType := "sync"
		if log.IsStream {
			requestType = "stream"
		}
		var metadata struct {
			FRT                 int64   `json:"frt"`
			CacheTokens         int64   `json:"cache_tokens"`
			CacheCreationTokens int64   `json:"cache_creation_tokens"`
			GroupRatio          float64 `json:"group_ratio"`
		}
		_ = json.Unmarshal([]byte(log.Other), &metadata)
		total := log.Quota / quota
		var baseCost float64
		if metadata.GroupRatio > 0 {
			baseCost = total / metadata.GroupRatio
		} else {
			baseCost = total
		}
		out.Items = append(out.Items, remoteUsage{ID: log.ID, AccountID: log.Channel, AccountName: log.ChannelName, UserID: log.UserID, UserName: log.Username, GroupID: newAPIGroupID(log.Group), GroupName: log.Group, IPAddress: log.IP, StatusCode: status, CreatedAt: time.Unix(log.CreatedAt, 0).UTC().Format(time.RFC3339), Model: log.Model, RequestedModel: log.Model, InputTokens: log.PromptTokens, OutputTokens: log.CompletionTokens, CacheReadTokens: metadata.CacheTokens, CacheCreationTokens: metadata.CacheCreationTokens, FirstTokenMS: metadata.FRT, DurationMS: log.UseTime * 1000, RequestType: requestType, RequestID: log.RequestID, ActualCost: total, TotalCost: baseCost})
	}
	return out, nil
}

// newAPIAdmin 将现有中转站操作转换为 New API 原生管理操作。未映射的操作明确失败。
func (s *Service) newAPIAdmin(ctx context.Context, method, endpoint string, c newAPICredential, body, out any) error {
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return err
	}
	index := strings.Index(parsed.Path, "/api/v1/admin/")
	if index < 0 {
		return errors.New("New API 管理路径无效")
	}
	base := parsed.Scheme + "://" + parsed.Host + parsed.Path[:index]
	path := strings.Trim(parsed.Path[index+len("/api/v1/admin/"):], "/")
	parts := strings.Split(path, "/")
	q := parsed.Query()
	id := int64(0)
	if len(parts) > 1 {
		id, _ = strconv.ParseInt(parts[1], 10, 64)
	}
	if parts[0] == "accounts" || parts[0] == "channels" {
		if path == "accounts/upstream-billing-probe/batch" {
			return errors.New("New API 成本倍率需要从渠道的真实上游探测")
		}
		if method == http.MethodGet && parts[0] == "channels" && len(parts) > 1 && parts[1] == "model-pricing" {
			return s.newAPIDefaultModelPricing(ctx, base, c, q.Get("model"), out)
		}
		if method == http.MethodGet && len(parts) == 1 {
			query := newAPIPageQuery(q)
			env, err := s.newAPIRequest(ctx, http.MethodGet, base+"/api/channel/?"+query.Encode(), c, nil)
			if err != nil {
				return err
			}
			var channels []newAPIChannel
			page, pages, err := decodeNewAPIPage(env.Data, query, &channels)
			if err != nil {
				return err
			}
			if parts[0] == "accounts" {
				accounts := []remoteAccount{}
				for _, channel := range channels {
					accounts = append(accounts, newAPIAccount(channel))
				}
				return assignNewAPI(out, remoteAccountPage{Items: accounts, Page: page.Page, Pages: pages, PageSize: page.PageSize})
			}
			mapped := []remoteChannel{}
			for _, channel := range channels {
				account := newAPIAccount(channel)
				mapped = append(mapped, remoteChannel{ID: channel.ID, Name: channel.Name, Status: account.Status, GroupIDs: account.GroupIDs})
			}
			return assignNewAPI(out, remotePage{Items: mapped, Page: page.Page, Pages: pages, PageSize: page.PageSize})
		}
		if id <= 0 {
			return errors.New("New API 渠道 ID 无效")
		}
		if method == http.MethodGet && len(parts) > 2 && parts[2] == "models" {
			env, err := s.newAPIRequest(ctx, http.MethodGet, fmt.Sprintf("%s/api/channel/%d", base, id), c, nil)
			if err != nil {
				return err
			}
			var channel newAPIChannel
			if err := json.Unmarshal(env.Data, &channel); err != nil {
				return err
			}
			return assignNewAPI(out, newAPISplit(channel.Models))
		}
		if method == http.MethodPut || (method == http.MethodPost && len(parts) > 2 && parts[2] == "schedulable") {
			return s.newAPIUpdateChannel(ctx, base, c, id, body)
		}
		if method == http.MethodPost && len(parts) > 2 && parts[2] == "duplicate" {
			return errors.New("New API 复制渠道需要提供新密钥，请使用批量新增账号")
		}
		if method == http.MethodDelete {
			_, err := s.newAPIRequest(ctx, http.MethodDelete, fmt.Sprintf("%s/api/channel/%d", base, id), c, nil)
			return err
		}
	}
	if parts[0] == "groups" {
		if method == http.MethodGet && path == "groups/all" {
			groups, err := s.newAPIGroups(ctx, base, c)
			if err != nil {
				return err
			}
			return assignNewAPI(out, groups)
		}
		if method == http.MethodPut && path == "groups/sort-order" {
			return errors.New("New API 分组排序应保存到本地配置")
		} // 排序属于本地展示设置。
		if method == http.MethodPut && id > 0 {
			return s.newAPIUpdateGroup(ctx, base, c, id, body)
		}
		if method == http.MethodDelete {
			return s.newAPIDeleteGroup(ctx, base, c, id)
		}
	}
	if parts[0] == "usage" || parts[0] == "ops" {
		if parts[0] == "ops" {
			return s.newAPIRequestErrors(ctx, base, c, q, out)
		}
		if method != http.MethodGet {
			return errors.New("New API 日志仅支持读取")
		}
		if path == "usage/stats" {
			return s.newAPIUsageStats(ctx, base, c, q, out)
		}
		page, err := s.newAPIUsageLogs(ctx, base, c, q)
		if err != nil {
			return err
		}
		return assignNewAPI(out, page)
	}
	if path == "dashboard/users-ranking" {
		return s.newAPIRanking(ctx, base, c, q, out)
	}
	if parts[0] == "users" {
		if path == "users/batch-limits" {
			return errors.New("New API 没有按用户设置并发数和 RPM 的管理接口")
		}
		var quota float64
		if method == http.MethodGet {
			var err error
			quota, err = s.newAPIQuota(ctx, base, c)
			if err != nil {
				return err
			}
		}
		if method == http.MethodGet && len(parts) == 1 {
			query := newAPIPageQuery(q)
			target := "/api/user/"
			if search := q.Get("search"); search != "" {
				target = "/api/user/search"
				query.Set("keyword", search)
			}
			env, err := s.newAPIRequest(ctx, http.MethodGet, base+target+"?"+query.Encode(), c, nil)
			if err != nil {
				return err
			}
			var users []newAPIUser
			page, pages, err := decodeNewAPIPage(env.Data, query, &users)
			if err != nil {
				return err
			}
			mapped := []remoteUser{}
			for _, user := range users {
				mapped = append(mapped, newAPIUserView(user, quota))
			}
			return assignNewAPI(out, remoteUserPage{Items: mapped, Total: page.Total, Page: page.Page, Pages: pages, PageSize: page.PageSize})
		}
		if id <= 0 {
			return errors.New("New API 用户 ID 无效")
		}
		if method == http.MethodPut {
			updates, err := newAPIObject(body)
			if err != nil {
				return err
			}
			action := "disable"
			if updates["status"] == "active" {
				action = "enable"
			}
			return s.newAPIProtectedRequest(ctx, http.MethodPost, base, "/api/user/manage", c, map[string]any{"id": id, "action": action}, "admin.user.manage", map[string]any{"user_id": id, "action": action})
		}
		if method == http.MethodDelete {
			return s.newAPIProtectedRequest(ctx, http.MethodDelete, base, fmt.Sprintf("/api/user/%d", id), c, nil, "admin.user.delete", map[string]any{"user_id": id})
		}
		if len(parts) > 2 && parts[2] == "balance-history" {
			return s.newAPIUserBalanceHistory(ctx, base, c, id, q, out)
		}
		env, err := s.newAPIRequest(ctx, http.MethodGet, fmt.Sprintf("%s/api/user/%d", base, id), c, nil)
		if err != nil {
			return err
		}
		var user newAPIUser
		if err := json.Unmarshal(env.Data, &user); err != nil {
			return err
		}
		return assignNewAPI(out, newAPIUserView(user, quota))
	}
	return fmt.Errorf("New API 没有对应的管理操作: %s %s", method, path)
}

func (s *Service) newAPIUsageStats(ctx context.Context, base string, c newAPICredential, q url.Values, out any) error {
	result := remoteUsageStats{}
	q.Set("page_size", "100")
	q.Set("type", "2")
	for pageNumber := 1; ; pageNumber++ {
		q.Set("page", strconv.Itoa(pageNumber))
		page, err := s.newAPILogs(ctx, base, c, q)
		if err != nil {
			return err
		}
		for _, item := range page.Items {
			result.TotalTokens += item.InputTokens + item.OutputTokens
			result.TotalActualCost += item.ActualCost
			result.TotalCost += item.TotalCost
			result.TotalRequests++
			result.RequestCount++
		}
		if pageNumber >= page.Pages {
			break
		}
	}
	return assignNewAPI(out, result)
}

// newAPIUsageLogs merges consumption and failed request logs into one stable page.
func (s *Service) newAPIUsageLogs(ctx context.Context, base string, c newAPICredential, q url.Values) (remoteUsagePage, error) {
	pageValue := q.Get("page")
	if pageValue == "" {
		pageValue = "1"
	}
	requestedPage, err := strconv.Atoi(pageValue)
	if err != nil || requestedPage < 1 || requestedPage > 10000 {
		return remoteUsagePage{}, errors.New("New API 日志分页超出安全范围")
	}
	pageSize, err := strconv.Atoi(q.Get("page_size"))
	if err != nil || pageSize < 1 || pageSize > 100 {
		pageSize = 100
	}
	items := make([]remoteUsage, 0, requestedPage*pageSize*2)
	total := 0
	for _, logType := range []string{"2", "5"} {
		query := q
		query = cloneURLValues(query)
		query.Set("page_size", strconv.Itoa(pageSize))
		query.Set("type", logType)
		for number := 1; number <= requestedPage; number++ {
			query.Set("page", strconv.Itoa(number))
			page, err := s.newAPILogs(ctx, base, c, query)
			if err != nil {
				return remoteUsagePage{}, err
			}
			if number == 1 {
				total += page.Total
			}
			items = append(items, page.Items...)
			if number >= page.Pages {
				break
			}
		}
	}
	sort.SliceStable(items, func(i, j int) bool {
		left, leftErr := time.Parse(time.RFC3339Nano, items[i].CreatedAt)
		right, rightErr := time.Parse(time.RFC3339Nano, items[j].CreatedAt)
		if leftErr == nil && rightErr == nil && !left.Equal(right) {
			return left.After(right)
		}
		return items[i].ID > items[j].ID
	})
	start := (requestedPage - 1) * pageSize
	if start > len(items) {
		start = len(items)
	}
	end := start + pageSize
	if end > len(items) {
		end = len(items)
	}
	pages := (total + pageSize - 1) / pageSize
	if pages < 1 {
		pages = 1
	}
	return remoteUsagePage{Items: items[start:end], Total: total, Page: requestedPage, PageSize: pageSize, Pages: pages}, nil
}

func cloneURLValues(source url.Values) url.Values {
	clone := make(url.Values, len(source))
	for key, values := range source {
		clone[key] = append([]string(nil), values...)
	}
	return clone
}

func (s *Service) newAPIRanking(ctx context.Context, base string, c newAPICredential, q url.Values, out any) error {
	result := remoteUserRanking{Ranking: []remoteUserRankingItem{}}
	users := map[int64]remoteUserRankingItem{}
	items, err := s.newAPILogSnapshot(ctx, base, c, q, "2")
	if err != nil {
		return err
	}
	for _, item := range items {
		user := users[item.UserID]
		user.UserID = item.UserID
		user.ActualCost += item.ActualCost
		user.Tokens += item.InputTokens + item.OutputTokens
		user.Requests++
		users[item.UserID] = user
		result.TotalActualCost += item.ActualCost
		result.TotalRequests++
		result.TotalTokens += item.InputTokens + item.OutputTokens
	}
	for _, user := range users {
		result.Ranking = append(result.Ranking, user)
	}
	sort.Slice(result.Ranking, func(i, j int) bool { return result.Ranking[i].ActualCost > result.Ranking[j].ActualCost })
	return assignNewAPI(out, result)
}
