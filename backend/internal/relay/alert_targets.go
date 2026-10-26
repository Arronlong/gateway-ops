package relay

import (
	"context"
	"crypto/sha256"
	"fmt"
	"time"

	"github.com/guliansheng/gateway-ops/internal/storage"
)

func (s *Service) AlertUserSnapshots(ctx context.Context, stationID uint) ([]storage.RelayUserSnapshot, error) {
	station, err := s.stations.FindByID(stationID)
	if err != nil {
		return nil, err
	}
	credential, err := s.cipher.Decrypt(station.APIKeyCipher)
	if err != nil {
		return nil, fmt.Errorf("decrypt admin API key: %w", err)
	}
	s.reportMu.Lock()
	revision := s.userReportRevision[stationID]
	s.reportMu.Unlock()
	key := fmt.Sprintf("alert-users:%d:%d:%x", stationID, revision, sha256.Sum256([]byte(station.BaseURL+"\x00"+station.APIKeyCipher)))
	value, err := s.cachedReport(ctx, key, time.Minute, func(loadCtx context.Context) (any, error) {
		users, err := s.fetchUsers(loadCtx, station.BaseURL, credential, "")
		if err != nil {
			return nil, err
		}
		return mapAlertUserSnapshots(users, time.Now().UTC()), nil
	})
	if err != nil {
		return nil, err
	}
	return value.([]storage.RelayUserSnapshot), nil
}

func mapAlertUserSnapshots(users []remoteUser, observedAt time.Time) []storage.RelayUserSnapshot {
	result := make([]storage.RelayUserSnapshot, 0, len(users))
	for _, user := range users {
		if user.ID <= 0 || user.DeletedAt != nil {
			continue
		}
		result = append(result, storage.RelayUserSnapshot{
			ID: user.ID, Username: user.Username, Email: user.Email, Role: user.Role,
			Balance: user.Balance, Concurrency: user.Concurrency, RPMLimit: user.RPMLimit,
			Status: user.Status, LastUsedAt: user.LastUsedAt, CreatedAt: user.CreatedAt, ObservedAt: observedAt,
		})
	}
	return result
}
