package channel

import (
	"context"
	"errors"
	"strings"

	"github.com/guliansheng/gateway-ops/internal/connector"
	"github.com/guliansheng/gateway-ops/internal/storage"
)

// RefreshChannelToken 仅供渠道维护任务主动续期，不与测智调用关联。
func (s *Service) RefreshChannelToken(ctx context.Context, c *storage.Channel) (bool, error) {
	if c.Type != storage.ChannelTypeSub2API || c.CredentialMode != storage.CredentialModeToken {
		return false, nil
	}
	session, err := s.buildSessionFromToken(c)
	if err != nil {
		return false, err
	}
	if strings.TrimSpace(session.RefreshToken) == "" {
		return false, errors.New("missing Sub2API refresh_token")
	}
	resolved, err := s.Resolve(ctx, c)
	if err != nil {
		return false, err
	}
	conn, err := connector.For(connector.ChannelType(c.Type))
	if err != nil {
		return false, err
	}
	sessionKey := c.ID
	if c.AuthSessionKey != 0 {
		sessionKey = c.AuthSessionKey
	}
	_, err = s.refreshSub2APIToken(ctx, c, resolved, conn, sessionKey, session, true)
	if err != nil {
		s.setCredentialError(c, err.Error())
		return false, err
	}
	s.setCredentialError(c, "")
	return true, nil
}
