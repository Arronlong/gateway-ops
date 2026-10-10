package api

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/guliansheng/gateway-ops/internal/relay"
)

var allowedLatencyBucketMinutes = map[int]struct{}{
	5: {}, 10: {}, 30: {}, 60: {}, 180: {}, 360: {}, 720: {},
}

func parseLatencyBucketSpec(mode, countRaw, minutesRaw string, defaultCount int) (relay.LatencyBucketSpec, error) {
	mode = strings.TrimSpace(strings.ToLower(mode))
	if mode == "" || mode == "auto" {
		count := defaultCount
		if strings.TrimSpace(countRaw) != "" {
			parsed, err := strconv.Atoi(countRaw)
			if err != nil || parsed <= 0 || parsed > 240 {
				return relay.LatencyBucketSpec{}, fmt.Errorf("bucket_count 必须是 1 到 240 的整数")
			}
			count = parsed
		}
		if count <= 0 || count > 240 {
			return relay.LatencyBucketSpec{}, fmt.Errorf("默认 bucket_count 无效")
		}
		return relay.LatencyBucketSpec{Count: count}, nil
	}
	if mode != "fixed" {
		return relay.LatencyBucketSpec{}, fmt.Errorf("bucket_mode 仅支持 auto 或 fixed")
	}
	minutes, err := strconv.Atoi(strings.TrimSpace(minutesRaw))
	if err != nil {
		return relay.LatencyBucketSpec{}, fmt.Errorf("bucket_minutes 必须是整数")
	}
	if _, ok := allowedLatencyBucketMinutes[minutes]; !ok {
		return relay.LatencyBucketSpec{}, fmt.Errorf("bucket_minutes 仅支持 5、10、30、60、180、360、720")
	}
	return relay.LatencyBucketSpec{Size: time.Duration(minutes) * time.Minute}, nil
}

func latencyBucketSpecFromQuery(c *gin.Context, defaultCount int) (relay.LatencyBucketSpec, error) {
	return parseLatencyBucketSpec(
		c.DefaultQuery("bucket_mode", "auto"),
		c.Query("bucket_count"),
		c.Query("bucket_minutes"),
		defaultCount,
	)
}
