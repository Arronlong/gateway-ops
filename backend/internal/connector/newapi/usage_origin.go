package newapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/guliansheng/gateway-ops/internal/connector"
)

func (c *Client) UsageHistoryOrigin(ctx context.Context, ch *connector.Channel, session *connector.AuthSession) (time.Time, error) {
	body, err := c.getJSON(ctx, ch, strings.TrimRight(ch.SiteURL, "/")+"/api/user/self", session)
	if err != nil {
		return time.Time{}, err
	}
	var user struct {
		CreatedAt   int64 `json:"created_at"`
		CreatedTime int64 `json:"created_time"`
	}
	if err := json.Unmarshal(body, &user); err != nil {
		return time.Time{}, err
	}
	if user.CreatedAt > 0 {
		return time.Unix(user.CreatedAt, 0).UTC(), nil
	}
	if user.CreatedTime > 0 {
		return time.Unix(user.CreatedTime, 0).UTC(), nil
	}
	release, err := connector.BeginUsageRequest(ctx, ch.SiteURL)
	if err != nil {
		return time.Time{}, err
	}
	defer release()
	resp, err := c.http.R().SetContext(ch.RequestContext(ctx)).SetHeaders(requestHeaders(ch, session)).SetQueryParam("p", "1").SetQueryParam("page_size", "100").Get(strings.TrimRight(ch.SiteURL, "/") + "/api/token/")
	if err != nil {
		return time.Time{}, err
	}
	if resp.IsError() {
		connector.ObserveUsageResponse(ctx, ch.SiteURL, resp.StatusCode(), resp.Header().Get("Retry-After"))
		if resp.StatusCode() == http.StatusNotFound || resp.StatusCode() == http.StatusMethodNotAllowed {
			return time.Time{}, nil
		}
		return time.Time{}, connector.HTTPStatusError(resp.StatusCode(), resp.Body())
	}
	var wrapped newapiResp
	if err := json.Unmarshal(resp.Body(), &wrapped); err != nil {
		return time.Time{}, err
	}
	if !wrapped.Success {
		return time.Time{}, nil
	}
	var data struct {
		Items []struct {
			CreatedTime int64 `json:"created_time"`
		} `json:"items"`
	}
	if err := json.Unmarshal(wrapped.Data, &data); err != nil {
		return time.Time{}, nil
	}
	var oldest time.Time
	for _, token := range data.Items {
		stamp := time.Unix(token.CreatedTime, 0).UTC()
		if token.CreatedTime > 0 && (oldest.IsZero() || stamp.Before(oldest)) {
			oldest = stamp
		}
	}
	return oldest, nil
}
