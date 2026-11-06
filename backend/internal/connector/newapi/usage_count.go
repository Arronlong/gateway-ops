package newapi

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/guliansheng/gateway-ops/internal/connector"
)

func usageStartTimestamp(value time.Time) int64 {
	stamp := value.Unix()
	if value.Nanosecond() > 0 {
		stamp++
	}
	return stamp
}

func (c *Client) CountUsage(ctx context.Context, ch *connector.Channel, session *connector.AuthSession, query connector.UsageQuery) (int64, error) {
	var count int64
	for _, kind := range []int{2, 5} {
		release, err := connector.BeginUsageRequest(ctx, ch.SiteURL)
		if err != nil {
			return 0, err
		}
		req := c.http.R().SetContext(ch.RequestContext(ctx)).SetHeaders(requestHeaders(ch, session)).SetQueryParam("p", "1").SetQueryParam("page_size", "1").SetQueryParam("type", strconv.Itoa(kind))
		if !query.Since.IsZero() {
			req.SetQueryParam("start_timestamp", strconv.FormatInt(usageStartTimestamp(query.Since), 10))
		}
		if !query.Until.IsZero() {
			req.SetQueryParam("end_timestamp", strconv.FormatInt(query.Until.Add(-time.Nanosecond).Unix(), 10))
		}
		resp, err := req.Get(strings.TrimRight(ch.SiteURL, "/") + "/api/log/self")
		if resp != nil {
			connector.ObserveUsageResponse(ctx, ch.SiteURL, resp.StatusCode(), resp.Header().Get("Retry-After"))
		}
		release()
		if err != nil {
			return 0, err
		}
		if resp.IsError() {
			return 0, connector.HTTPStatusError(resp.StatusCode(), resp.Body())
		}
		var wrapped newapiResp
		if err := json.Unmarshal(resp.Body(), &wrapped); err != nil {
			return 0, err
		}
		if !wrapped.Success {
			return 0, fmt.Errorf("newapi usage count: %s", wrapped.Message)
		}
		var data struct {
			Total    *int64     `json:"total"`
			PageSize int        `json:"page_size"`
			Items    []usageLog `json:"items"`
		}
		if err := json.Unmarshal(wrapped.Data, &data); err != nil {
			return 0, err
		}
		if data.Total == nil || *data.Total < int64(len(data.Items)) || (len(data.Items) > 0 && data.PageSize < len(data.Items)) || (data.Total != nil && *data.Total > 0 && len(data.Items) == 0) {
			return 0, fmt.Errorf("upstream did not return a reliable request total")
		}
		if *data.Total == 2000 {
			return 0, fmt.Errorf("\u4e0a\u6e38 total \u4e3a 2000\uff0c\u53ef\u80fd\u5df2\u622a\u65ad\uff1b\u8bf7\u91cd\u65b0\u62c9\u53d6\u5386\u53f2\u8fdb\u884c\u5206\u6bb5\u6838\u5bf9")
		}
		for _, item := range data.Items {
			if item.Type != kind {
				return 0, fmt.Errorf("upstream ignored the call type filter")
			}
		}
		count += *data.Total
	}
	return count, nil
}
