package alert

import (
	"time"

	"github.com/guliansheng/gateway-ops/internal/storage"
)

func userInactivityObservation(user storage.RelayUserSnapshot, now time.Time) Observation {
	observation := Observation{ObservedAt: user.ObservedAt.UTC()}
	if user.ObservedAt.IsZero() || user.ObservedAt.After(now) || !fresh(user.ObservedAt, now, 3*time.Minute) {
		observation.Reason = "用户使用快照缺失或已过期"
		return observation
	}
	if !user.LastUsedAtSupported {
		observation.Reason = "当前中转站不提供用户最后使用时间"
		return observation
	}
	lastUsed := user.CreatedAt
	if user.LastUsedAt != nil {
		lastUsed = *user.LastUsedAt
	}
	if lastUsed.IsZero() || lastUsed.After(user.ObservedAt) {
		observation.Reason = "用户最后使用时间或创建时间缺失或无效"
		return observation
	}
	observation.Value = user.ObservedAt.Sub(lastUsed).Hours() / 24
	observation.Valid = true
	return observation
}
