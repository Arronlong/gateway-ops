package relay

import "github.com/guliansheng/gateway-ops/internal/storage"

// CredentialMetadata 只包含编辑界面可读取的非敏感配置。
type CredentialMetadata struct {
	AuthMode             string `json:"auth_mode,omitempty"`
	Username             string `json:"username,omitempty"`
	AdminUserID          string `json:"admin_user_id,omitempty"`
	GatewayKeyConfigured bool   `json:"gateway_key_configured"`
}

func (s *Service) CredentialMetadata(station *storage.RelayStation) CredentialMetadata {
	if station.Type != relayStationNewAPI {
		return CredentialMetadata{}
	}
	credential, err := s.decryptRelayCredential(station)
	if err != nil {
		return CredentialMetadata{}
	}
	mode := credential.AuthMode
	if mode == "" {
		switch {
		case credential.Username != "":
			mode = "password"
		case credential.Token != "":
			mode = "access_token"
		case credential.Cookie != "":
			mode = "cookie"
		}
	}
	return CredentialMetadata{AuthMode: mode, Username: credential.Username, AdminUserID: credential.UserID, GatewayKeyConfigured: credential.GatewayKey != ""}
}
