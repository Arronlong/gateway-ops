package monitor

import (
	"context"
	"time"

	"github.com/guliansheng/gateway-ops/internal/storage"
	"github.com/guliansheng/gateway-ops/internal/tasklog"
)

func (s *Service) RefreshChannelTokens(ctx context.Context) SyncAllResult {
	result := SyncAllResult{}
	channels, err := s.channels.ListMonitorEnabled()
	if err != nil {
		tasklog.Add(ctx, "ERROR", "\u8bfb\u53d6\u6e20\u9053\u5217\u8868\u5931\u8d25\uff1a%v", err)
		result.Failed++
		return result
	}
	for _, parent := range channels {
		if ctx.Err() != nil {
			break
		}
		if parent.Type != storage.ChannelTypeSub2API {
			continue
		}
		accounts, err := s.channels.ListAccounts(parent.ID)
		if err != nil {
			tasklog.Add(ctx, "ERROR", "\u6e20\u9053 #%d \u8bfb\u53d6\u8d26\u53f7\u5931\u8d25\uff1a%v", parent.ID, err)
			result.Failed++
			continue
		}
		targets := []*storage.Channel{}
		if len(accounts) == 0 {
			targets = append(targets, &parent)
		} else {
			for _, account := range accounts {
				targets = append(targets, s.channelSvc.AccountChannel(&parent, account))
			}
		}
		for _, target := range targets {
			if ctx.Err() != nil {
				break
			}
			if target.CredentialMode != storage.CredentialModeToken {
				continue
			}
			result.Total++
			accountCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
			_, err := s.channelSvc.RefreshChannelToken(accountCtx, target)
			cancel()
			if err != nil {
				result.Failed++
				tasklog.Add(ctx, "ERROR", "\u6e20\u9053 #%d %s\uff08%s\uff09Token \u5237\u65b0\u5931\u8d25\uff1a%v", parent.ID, parent.Name, target.Username, err)
			} else {
				result.Succeeded++
				tasklog.Add(ctx, "INFO", "\u6e20\u9053 #%d %s\uff08%s\uff09Token \u5df2\u5237\u65b0", parent.ID, parent.Name, target.Username)
			}
		}
	}
	return result
}
