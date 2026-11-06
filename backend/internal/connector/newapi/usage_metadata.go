package newapi

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/guliansheng/gateway-ops/internal/connector"
)

type usageMetadata struct {
	quota   float64
	expires time.Time
}

func (c *Client) usageQuotaPerUnit(ctx context.Context, ch *connector.Channel) (float64, error) {
	site := strings.TrimRight(ch.SiteURL, "/")
	if value, ok := c.usageMetadata.Load(site); ok {
		entry := value.(usageMetadata)
		if time.Now().Before(entry.expires) {
			return entry.quota, nil
		}
	}
	body, err := c.getJSON(ctx, ch, site+"/api/status", nil)
	if err != nil {
		return 0, fmt.Errorf("newapi usage status: %w", err)
	}
	var status struct {
		QuotaPerUnit float64 `json:"quota_per_unit"`
	}
	if err := json.Unmarshal(body, &status); err != nil {
		return 0, fmt.Errorf("newapi usage status decode: %w", err)
	}
	if status.QuotaPerUnit <= 0 {
		status.QuotaPerUnit = 500000
	}
	c.usageMetadata.Store(site, usageMetadata{quota: status.QuotaPerUnit, expires: time.Now().Add(10 * time.Minute)})
	return status.QuotaPerUnit, nil
}
