// Package sub2api 实现 sub2api 风格上游站点的 connector，参考 docs/USER_BALANCE_GROUP_RATE_AUTH_API_CN-sub2api.md。
package sub2api

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
	connector.Register(connector.TypeSub2API, func() connector.Connector { return New() })
}

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

// sub2Resp sub2api 统一响应外壳：{ code, message, data }。code 0 = 成功。
type sub2Resp struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

func (c *Client) GetTurnstileSiteKey(ctx context.Context, ch *connector.Channel) (string, error) {
	body, err := c.getJSON(ctx, ch, strings.TrimRight(ch.SiteURL, "/")+"/api/v1/settings/public", nil)
	if err != nil {
		return "", fmt.Errorf("sub2api public settings: %w", err)
	}
	var settings struct {
		TurnstileEnabled bool   `json:"turnstile_enabled"`
		TurnstileSiteKey string `json:"turnstile_site_key"`
	}
	if err := json.Unmarshal(body, &settings); err != nil {
		return "", fmt.Errorf("sub2api public settings decode: %w", err)
	}
	if !settings.TurnstileEnabled {
		return "", nil
	}
	return settings.TurnstileSiteKey, nil
}

func (c *Client) Login(ctx context.Context, ch *connector.Channel) (*connector.AuthSession, error) {
	site := strings.TrimRight(ch.SiteURL, "/")
	vars := map[string]string{"username": ch.Username, "password": ch.Password, "turnstile_token": ch.TurnstileToken}
	body := connector.ExpandRequestKV(ch.LoginParams, vars)

	resp, err := c.http.R().
		SetContext(ctx).
		SetHeaders(requestHeaders(ch, nil)).
		SetBody(body).
		Post(site + "/api/v1/auth/login")
	if err != nil {
		return nil, fmt.Errorf("sub2api login http: %w", err)
	}
	if resp.IsError() {
		return nil, fmt.Errorf("sub2api login: %w", connector.HTTPStatusError(resp.StatusCode(), resp.Body()))
	}
	var wrapped sub2Resp
	if err := json.Unmarshal(resp.Body(), &wrapped); err != nil {
		return nil, fmt.Errorf("sub2api login decode: %w", err)
	}
	if wrapped.Code != 0 {
		return nil, fmt.Errorf("sub2api login: %s", wrapped.Message)
	}

	var data struct {
		Requires2FA  bool   `json:"requires_2fa"`
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int64  `json:"expires_in"`
	}
	if err := json.Unmarshal(wrapped.Data, &data); err != nil {
		return nil, fmt.Errorf("sub2api login data: %w", err)
	}
	if data.Requires2FA {
		return nil, errors.New("sub2api account requires 2FA; please disable it for monitoring accounts")
	}
	if data.AccessToken == "" {
		return nil, errors.New("sub2api login: empty access_token")
	}

	expires := time.Now().Add(time.Duration(data.ExpiresIn) * time.Second)
	if data.ExpiresIn <= 0 {
		expires = time.Now().Add(time.Hour)
	}
	return &connector.AuthSession{
		AccessToken:  data.AccessToken,
		RefreshToken: data.RefreshToken,
		ExpiresAt:    expires,
	}, nil
}

func (c *Client) RefreshToken(ctx context.Context, ch *connector.Channel, refreshToken string) (*connector.AuthSession, error) {
	site := strings.TrimRight(ch.SiteURL, "/")
	resp, err := c.http.R().
		SetContext(ctx).
		SetHeader("Content-Type", "application/json").
		SetHeaders(requestHeaders(ch, nil)).
		SetBody(map[string]string{"refresh_token": refreshToken}).
		Post(site + "/api/v1/auth/refresh")
	if err != nil {
		return nil, fmt.Errorf("sub2api refresh http: %w", err)
	}
	if resp.IsError() {
		return nil, fmt.Errorf("sub2api refresh: %w", connector.HTTPStatusError(resp.StatusCode(), resp.Body()))
	}
	var wrapped sub2Resp
	if err := json.Unmarshal(resp.Body(), &wrapped); err != nil {
		return nil, fmt.Errorf("sub2api refresh decode: %w", err)
	}
	if wrapped.Code != 0 {
		return nil, fmt.Errorf("sub2api refresh: %s", wrapped.Message)
	}
	var data struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int64  `json:"expires_in"`
	}
	if err := json.Unmarshal(wrapped.Data, &data); err != nil {
		return nil, fmt.Errorf("sub2api refresh data: %w", err)
	}
	if data.AccessToken == "" {
		return nil, errors.New("sub2api refresh: empty access_token returned")
	}
	if data.RefreshToken == "" {
		data.RefreshToken = refreshToken
	}
	expires := time.Now().Add(time.Duration(data.ExpiresIn) * time.Second)
	if data.ExpiresIn <= 0 {
		expires = time.Now().Add(time.Hour)
	}
	return &connector.AuthSession{AccessToken: data.AccessToken, RefreshToken: data.RefreshToken, ExpiresAt: expires}, nil
}

func (c *Client) CheckAuth(ctx context.Context, ch *connector.Channel, session *connector.AuthSession) error {
	if session == nil || session.AccessToken == "" {
		return errors.New("missing sub2api access_token")
	}
	_, err := c.getJSON(ctx, ch, strings.TrimRight(ch.SiteURL, "/")+"/api/v1/auth/me", session)
	return err
}

func (c *Client) GetBalance(ctx context.Context, ch *connector.Channel, session *connector.AuthSession) (*connector.BalanceResult, error) {
	body, err := c.getJSON(ctx, ch, strings.TrimRight(ch.SiteURL, "/")+"/api/v1/auth/me", session)
	if err != nil {
		return nil, fmt.Errorf("sub2api me: %w", err)
	}
	var me struct {
		Balance float64 `json:"balance"`
	}
	if err := json.Unmarshal(body, &me); err != nil {
		return nil, fmt.Errorf("sub2api me decode: %w", err)
	}
	return &connector.BalanceResult{
		Balance:   me.Balance,
		SampledAt: time.Now(),
	}, nil
}

func (c *Client) GetRates(ctx context.Context, ch *connector.Channel, session *connector.AuthSession) ([]connector.RateResult, error) {
	site := strings.TrimRight(ch.SiteURL, "/")

	availBody, err := c.getJSON(ctx, ch, site+"/api/v1/groups/available", session)
	if err != nil {
		return nil, fmt.Errorf("sub2api groups available: %w", err)
	}
	var groups []struct {
		ID             uint64  `json:"id"`
		Name           string  `json:"name"`
		Description    string  `json:"description"`
		RateMultiplier float64 `json:"rate_multiplier"`
	}
	if err := json.Unmarshal(availBody, &groups); err != nil {
		return nil, fmt.Errorf("sub2api groups available decode: %w", err)
	}

	overrides := map[string]float64{}
	if ratesBody, err := c.getJSON(ctx, ch, site+"/api/v1/groups/rates", session); err == nil {
		_ = json.Unmarshal(ratesBody, &overrides)
	}

	out := make([]connector.RateResult, 0, len(groups))
	for _, g := range groups {
		rate := g.RateMultiplier
		if v, ok := overrides[strconv.FormatUint(g.ID, 10)]; ok {
			rate = v
		}
		out = append(out, connector.RateResult{
			ModelName:   g.Name,
			Description: g.Description,
			Ratio:       rate,
		})
	}
	return out, nil
}

func (c *Client) ListUsage(ctx context.Context, ch *connector.Channel, session *connector.AuthSession, query connector.UsageQuery) (*connector.UsagePage, error) {
	if session == nil || session.AccessToken == "" {
		return nil, errors.New("missing sub2api access_token")
	}
	page := query.Page
	if page <= 0 {
		page = 1
	}
	pageSize := query.PageSize
	if pageSize <= 0 || pageSize > 1000 {
		pageSize = 100
	}
	req := c.http.R().SetContext(ctx).SetHeaders(requestHeaders(ch, session)).
		SetQueryParam("page", strconv.Itoa(page)).
		SetQueryParam("page_size", strconv.Itoa(pageSize)).
		SetQueryParam("sort_by", "created_at").
		SetQueryParam("sort_order", "desc").
		SetQueryParam("timezone", "UTC")
	if !query.Since.IsZero() {
		req.SetQueryParam("start_date", query.Since.UTC().Format("2006-01-02"))
	}
	if !query.Until.IsZero() {
		end := query.Until.UTC()
		if end.After(time.Time{}) {
			end = end.Add(-time.Nanosecond)
		}
		req.SetQueryParam("end_date", end.Format("2006-01-02"))
	}
	resp, err := req.Get(strings.TrimRight(ch.SiteURL, "/") + "/api/v1/usage")
	if err != nil {
		return nil, fmt.Errorf("sub2api usage http: %w", err)
	}
	if resp.IsError() {
		return nil, fmt.Errorf("sub2api usage: %w", connector.HTTPStatusError(resp.StatusCode(), resp.Body()))
	}
	var wrapped sub2Resp
	if err := json.Unmarshal(resp.Body(), &wrapped); err != nil {
		return nil, fmt.Errorf("sub2api usage decode: %w", err)
	}
	if wrapped.Code != 0 {
		return nil, fmt.Errorf("sub2api usage: %s", wrapped.Message)
	}
	var data struct {
		Items []struct {
			ID                  int64   `json:"id"`
			RequestID           string  `json:"request_id"`
			Model               string  `json:"model"`
			InputTokens         int64   `json:"input_tokens"`
			OutputTokens        int64   `json:"output_tokens"`
			CacheReadTokens     *int64  `json:"cache_read_tokens"`
			CacheCreationTokens int64   `json:"cache_creation_tokens"`
			TotalCost           float64 `json:"total_cost"`
			ActualCost          float64 `json:"actual_cost"`
			DurationMS          *int64  `json:"duration_ms"`
			FirstTokenMS        *int64  `json:"first_token_ms"`
			CreatedAt           string  `json:"created_at"`
		} `json:"items"`
		Total    int64 `json:"total"`
		Page     int   `json:"page"`
		PageSize int   `json:"page_size"`
		Pages    int   `json:"pages"`
	}
	if err := json.Unmarshal(wrapped.Data, &data); err != nil {
		return nil, fmt.Errorf("sub2api usage data decode: %w", err)
	}
	out := &connector.UsagePage{Total: data.Total, Page: data.Page, PageSize: data.PageSize, Pages: data.Pages}
	out.Records = make([]connector.UsageRecord, 0, len(data.Items))
	for _, item := range data.Items {
		createdAt, err := time.Parse(time.RFC3339, item.CreatedAt)
		if err != nil {
			continue
		}
		out.Records = append(out.Records, connector.UsageRecord{
			RemoteID: strconv.FormatInt(item.ID, 10), RequestID: item.RequestID, CreatedAt: createdAt,
			Model: item.Model, FirstTokenMS: item.FirstTokenMS, DurationMS: item.DurationMS,
			SuccessKnown: false, Success: true, InputTokens: item.InputTokens, OutputTokens: item.OutputTokens,
			CacheKnown: item.CacheReadTokens != nil, CacheReadTokens: cacheTokenValue(item.CacheReadTokens), CacheCreationTokens: item.CacheCreationTokens,
			ActualCost: item.ActualCost, TotalCost: item.TotalCost,
		})
	}
	return out, nil
}

func requestHeaders(ch *connector.Channel, session *connector.AuthSession) map[string]string {
	vars := map[string]string{"username": ch.Username, "password": ch.Password, "turnstile_token": ch.TurnstileToken}
	headers := make(map[string]string)
	if session != nil && session.AccessToken != "" {
		vars["access_token"] = session.AccessToken
		vars["token"] = session.AccessToken
		headers["Authorization"] = "Bearer " + session.AccessToken
	}
	items := make([]connector.RequestKV, 0, len(ch.LoginHeaders))
	for _, item := range ch.LoginHeaders {
		if (session == nil || session.AccessToken == "") && (strings.Contains(item.Value, "{{access_token}}") || strings.Contains(item.Value, "{{token}}")) {
			continue
		}
		items = append(items, item)
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
	var wrapped sub2Resp
	if err := json.Unmarshal(resp.Body(), &wrapped); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	if wrapped.Code != 0 {
		return nil, errors.New(wrapped.Message)
	}
	return wrapped.Data, nil
}
