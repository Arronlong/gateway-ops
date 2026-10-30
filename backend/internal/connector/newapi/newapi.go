// Package newapi 实现对 NewAPI 风格上游站点的 connector，参考 docs/USER_BALANCE_GROUP_RATE_AUTH_API_CN-newapi.md。
package newapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/guliansheng/gateway-ops/internal/connector"
)

func init() {
	connector.Register(connector.TypeNewAPI, func() connector.Connector { return New() })
}

// Client NewAPI connector 实现。
type Client struct {
	http *resty.Client
}

func New() *Client {
	c := resty.New().
		SetTimeout(30*time.Second).
		SetHeader("User-Agent", "GatewayOps/0.1").
		SetHeader("Accept", "application/json")
	return &Client{http: c}
}

// newapiResp NewAPI 统一响应外壳：{ success, message, data }。
type newapiResp struct {
	Success bool            `json:"success"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

func (c *Client) GetTurnstileSiteKey(ctx context.Context, ch *connector.Channel) (string, error) {
	body, err := c.getJSON(ctx, ch, strings.TrimRight(ch.SiteURL, "/")+"/api/status", nil)
	if err != nil {
		return "", fmt.Errorf("newapi status: %w", err)
	}
	var status struct {
		TurnstileCheck   bool   `json:"turnstile_check"`
		TurnstileSiteKey string `json:"turnstile_site_key"`
	}
	if err := json.Unmarshal(body, &status); err != nil {
		return "", fmt.Errorf("newapi status decode: %w", err)
	}
	if !status.TurnstileCheck {
		return "", nil
	}
	return status.TurnstileSiteKey, nil
}

func (c *Client) Login(ctx context.Context, ch *connector.Channel) (*connector.AuthSession, error) {
	site := strings.TrimRight(ch.SiteURL, "/")
	vars := map[string]string{"username": ch.Username, "password": ch.Password, "turnstile_token": ch.TurnstileToken}
	req := c.http.R().
		SetContext(ctx).
		SetHeaders(requestHeaders(ch, nil)).
		SetBody(connector.ExpandRequestKV(ch.LoginParams, vars))
	if ch.TurnstileToken != "" {
		req.SetQueryParam("turnstile", ch.TurnstileToken)
	}

	resp, err := req.Post(site + "/api/user/login")
	if err != nil {
		return nil, fmt.Errorf("newapi login http: %w", err)
	}
	if resp.IsError() {
		return nil, fmt.Errorf("newapi login: %w", connector.HTTPStatusError(resp.StatusCode(), resp.Body()))
	}
	var wrapped newapiResp
	if err := json.Unmarshal(resp.Body(), &wrapped); err != nil {
		return nil, fmt.Errorf("newapi login decode: %w", err)
	}
	if !wrapped.Success {
		return nil, fmt.Errorf("newapi login: %s", wrapped.Message)
	}

	var data struct {
		Require2FA bool  `json:"require_2fa"`
		ID         int64 `json:"id"`
	}
	_ = json.Unmarshal(wrapped.Data, &data)
	if data.Require2FA {
		return nil, errors.New("newapi account requires 2FA; please disable it for monitoring accounts")
	}

	cookie := joinCookies(resp.Cookies())
	if cookie == "" {
		return nil, errors.New("newapi login: no session cookie returned")
	}
	if data.ID == 0 {
		// 用户 id 是后续 New-Api-User 头的必需值；缺失说明响应格式不对。
		return nil, errors.New("newapi login: missing user id in response")
	}
	// NewAPI session 默认有效期较长，保守按 7 天估算；CheckAuth 会兜底失效检测。
	return &connector.AuthSession{
		UserID:    strconv.FormatInt(data.ID, 10),
		Cookie:    cookie,
		ExpiresAt: time.Now().Add(7 * 24 * time.Hour),
	}, nil
}

func (c *Client) CheckAuth(ctx context.Context, ch *connector.Channel, session *connector.AuthSession) error {
	if session == nil || (session.Cookie == "" && len(session.Headers) == 0) {
		return errors.New("missing newapi auth credential")
	}
	_, err := c.getJSON(ctx, ch, strings.TrimRight(ch.SiteURL, "/")+"/api/user/self", session)
	return err
}

func (c *Client) GetBalance(ctx context.Context, ch *connector.Channel, session *connector.AuthSession) (*connector.BalanceResult, error) {
	site := strings.TrimRight(ch.SiteURL, "/")
	statusBody, err := c.getJSON(ctx, ch, site+"/api/status", nil)
	if err != nil {
		return nil, fmt.Errorf("newapi status: %w", err)
	}
	var status struct {
		QuotaPerUnit float64 `json:"quota_per_unit"`
	}
	if err := json.Unmarshal(statusBody, &status); err != nil {
		return nil, fmt.Errorf("newapi status decode: %w", err)
	}
	if status.QuotaPerUnit <= 0 {
		status.QuotaPerUnit = 500000
	}

	selfBody, err := c.getJSON(ctx, ch, site+"/api/user/self", session)
	if err != nil {
		return nil, fmt.Errorf("newapi self: %w", err)
	}
	var self struct {
		Quota float64 `json:"quota"`
	}
	if err := json.Unmarshal(selfBody, &self); err != nil {
		return nil, fmt.Errorf("newapi self decode: %w", err)
	}
	return &connector.BalanceResult{
		Balance:   self.Quota / status.QuotaPerUnit,
		SampledAt: time.Now(),
	}, nil
}

func (c *Client) GetRates(ctx context.Context, ch *connector.Channel, session *connector.AuthSession) ([]connector.RateResult, error) {
	body, err := c.getJSON(ctx, ch, strings.TrimRight(ch.SiteURL, "/")+"/api/user/self/groups", session)
	if err != nil {
		return nil, fmt.Errorf("newapi groups: %w", err)
	}
	// data: { "default": { "ratio": 1, "desc": "..." }, "auto": { "ratio": "自动", ... } }
	raw := map[string]struct {
		Ratio json.RawMessage `json:"ratio"`
		Desc  string          `json:"desc"`
	}{}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("newapi groups decode: %w", err)
	}
	out := make([]connector.RateResult, 0, len(raw))
	for name, v := range raw {
		var ratio float64
		if err := json.Unmarshal(v.Ratio, &ratio); err != nil {
			// "auto" 组的 ratio 是字符串 "自动"，跳过。
			continue
		}
		out = append(out, connector.RateResult{
			ModelName:   name,
			Description: v.Desc,
			Ratio:       ratio,
		})
	}
	return out, nil
}

func (c *Client) ListUsage(ctx context.Context, ch *connector.Channel, session *connector.AuthSession, query connector.UsageQuery) (*connector.UsagePage, error) {
	if session == nil || (session.Cookie == "" && len(session.Headers) == 0) {
		return nil, errors.New("missing newapi auth credential")
	}
	page := query.Page
	if page <= 0 {
		page = 1
	}
	pageSize := query.PageSize
	if pageSize <= 0 || pageSize > 100 {
		pageSize = 100
	}
	site := strings.TrimRight(ch.SiteURL, "/")
	statusBody, err := c.getJSON(ctx, ch, site+"/api/status", nil)
	if err != nil {
		return nil, fmt.Errorf("newapi usage status: %w", err)
	}
	var status struct {
		QuotaPerUnit float64 `json:"quota_per_unit"`
	}
	if err := json.Unmarshal(statusBody, &status); err != nil {
		return nil, fmt.Errorf("newapi usage status decode: %w", err)
	}
	if status.QuotaPerUnit <= 0 {
		status.QuotaPerUnit = 500000
	}

	req := c.http.R().SetContext(ctx).SetHeaders(requestHeaders(ch, session)).
		SetQueryParam("p", strconv.Itoa(page)).
		SetQueryParam("page_size", strconv.Itoa(pageSize)).
		SetQueryParam("type", "0")
	if !query.Since.IsZero() {
		req.SetQueryParam("start_timestamp", strconv.FormatInt(query.Since.Unix(), 10))
	}
	if !query.Until.IsZero() {
		req.SetQueryParam("end_timestamp", strconv.FormatInt(query.Until.Unix(), 10))
	}
	resp, err := req.Get(site + "/api/log/self")
	if err != nil {
		return nil, fmt.Errorf("newapi usage http: %w", err)
	}
	if resp.IsError() {
		return nil, fmt.Errorf("newapi usage: %w", connector.HTTPStatusError(resp.StatusCode(), resp.Body()))
	}
	var wrapped newapiResp
	if err := json.Unmarshal(resp.Body(), &wrapped); err != nil {
		return nil, fmt.Errorf("newapi usage decode: %w", err)
	}
	if !wrapped.Success {
		return nil, fmt.Errorf("newapi usage: %s", wrapped.Message)
	}
	var data struct {
		Page     int `json:"page"`
		PageSize int `json:"page_size"`
		Total    int `json:"total"`
		Items    []struct {
			ID               int    `json:"id"`
			Type             int    `json:"type"`
			CreatedAt        int64  `json:"created_at"`
			ModelName        string `json:"model_name"`
			Quota            int64  `json:"quota"`
			PromptTokens     int64  `json:"prompt_tokens"`
			CompletionTokens int64  `json:"completion_tokens"`
			UseTime          int64  `json:"use_time"`
			RequestID        string `json:"request_id"`
			Other            string `json:"other"`
		} `json:"items"`
	}
	if err := json.Unmarshal(wrapped.Data, &data); err != nil {
		return nil, fmt.Errorf("newapi usage data decode: %w", err)
	}
	pages := 1
	if data.PageSize > 0 && data.Total > 0 {
		pages = (data.Total + data.PageSize - 1) / data.PageSize
	}
	out := &connector.UsagePage{RawCount: len(data.Items), Total: int64(data.Total), Page: data.Page, PageSize: data.PageSize, Pages: pages}
	out.Records = make([]connector.UsageRecord, 0, len(data.Items))
	for _, item := range data.Items {
		if (item.Type != 0 && item.Type != 2 && item.Type != 5) || (item.Type == 0 && strings.TrimSpace(item.ModelName) == "") {
			continue
		}
		durationMS := item.UseTime * 1000
		cacheRead, cacheCreation, cacheKnown := cacheTokensFromOther(item.Other)
		firstTokenMS := firstTokenMSFromOther(item.Other)
		out.Records = append(out.Records, connector.UsageRecord{
			RemoteID: strconv.Itoa(item.ID), RequestID: item.RequestID, CreatedAt: time.Unix(item.CreatedAt, 0),
			Model: item.ModelName, FirstTokenMS: firstTokenMS, DurationMS: &durationMS, SuccessKnown: item.Type == 2 || item.Type == 5, Success: item.Type != 5,
			CacheKnown: cacheKnown, CacheReadTokens: cacheRead, CacheCreationTokens: cacheCreation,
			InputTokens: item.PromptTokens, OutputTokens: item.CompletionTokens,
			ActualCost: float64(item.Quota) / status.QuotaPerUnit, TotalCost: float64(item.Quota) / status.QuotaPerUnit,
		})
	}
	return out, nil
}

func firstTokenMSFromOther(other string) *int64 {
	if strings.TrimSpace(other) == "" {
		return nil
	}
	var metadata struct {
		FRT *int64 `json:"frt"`
	}
	if err := json.Unmarshal([]byte(other), &metadata); err != nil || metadata.FRT == nil || *metadata.FRT < 0 {
		return nil
	}
	value := *metadata.FRT
	return &value
}

func requestHeaders(ch *connector.Channel, session *connector.AuthSession) map[string]string {
	vars := map[string]string{"username": ch.Username, "password": ch.Password, "turnstile_token": ch.TurnstileToken}
	headers := make(map[string]string)
	if session != nil {
		vars["cookie"] = session.Cookie
		vars["user_id"] = session.UserID
		vars["token"] = session.AccessToken
		vars["access_token"] = session.AccessToken
		if session.Cookie != "" {
			headers["Cookie"] = session.Cookie
		}
		if session.UserID != "" {
			headers["New-Api-User"] = session.UserID
		}
		for key, value := range session.Headers {
			headers[http.CanonicalHeaderKey(key)] = value
		}
	}
	items := make([]connector.RequestKV, 0, len(ch.LoginHeaders))
	for _, item := range ch.LoginHeaders {
		unresolved := false
		for _, name := range []string{"cookie", "user_id", "token", "access_token"} {
			if strings.Contains(item.Value, "{{"+name+"}}") && vars[name] == "" {
				unresolved = true
				break
			}
		}
		if !unresolved {
			items = append(items, item)
		}
	}
	for key, value := range connector.ExpandRequestKV(items, vars) {
		headers[http.CanonicalHeaderKey(key)] = value
	}
	return headers
}

func (c *Client) getJSON(ctx context.Context, ch *connector.Channel, url string, session *connector.AuthSession) ([]byte, error) {
	req := c.http.R().SetContext(ctx).SetHeaders(requestHeaders(ch, session))
	resp, err := req.Get(url)
	if err != nil {
		return nil, err
	}
	if resp.IsError() {
		return nil, connector.HTTPStatusError(resp.StatusCode(), resp.Body())
	}
	var wrapped newapiResp
	if err := json.Unmarshal(resp.Body(), &wrapped); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	if !wrapped.Success {
		return nil, errors.New(wrapped.Message)
	}
	return wrapped.Data, nil
}

func joinCookies(cookies []*http.Cookie) string {
	if len(cookies) == 0 {
		return ""
	}
	parts := make([]string, 0, len(cookies))
	for _, c := range cookies {
		parts = append(parts, c.Name+"="+c.Value)
	}
	return strings.Join(parts, "; ")
}
