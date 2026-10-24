package storage

import "context"

// PelicanKeySession 只用于后台查询实际 Key 的分组，不向前端返回登录凭据。
type PelicanKeySession struct {
	SiteURL           string
	Type              ChannelType
	CredentialMode    CredentialMode
	CredentialCipher  string `json:"-"`
	AccessTokenCipher string `json:"-"`
	CookieCipher      string `json:"-"`
	UserID            string
}

func (r *RelayStations) PelicanKeySessions(ctx context.Context, stationID uint, accountID int64) ([]PelicanKeySession, error) {
	var sessions []PelicanKeySession
	err := r.db.WithContext(ctx).Raw(`SELECT channels.site_url, sessions.access_token_cipher, channels.type, sessions.cookie_cipher, sessions.user_id,
	 CASE WHEN accounts.is_primary THEN channels.credential_mode ELSE accounts.credential_mode END AS credential_mode,
	 CASE WHEN accounts.is_primary THEN channels.password_cipher ELSE accounts.password_cipher END AS credential_cipher
	 FROM channel_relay_account_links links
	 JOIN channel_accounts accounts ON accounts.id = links.channel_account_id AND accounts.deleted_at IS NULL
	 JOIN channels ON channels.id = accounts.channel_id AND channels.deleted_at IS NULL
	 LEFT JOIN auth_sessions sessions ON sessions.channel_id = CASE WHEN accounts.is_primary THEN accounts.channel_id ELSE accounts.id + ? END
	 AND (sessions.expires_at IS NULL OR sessions.expires_at > CURRENT_TIMESTAMP)
	 WHERE links.relay_station_id = ? AND links.relay_account_external_id = ?
	 AND channels.type IN (?, ?)
	 ORDER BY accounts.is_primary DESC, accounts.id LIMIT 8`, AdditionalAccountSessionOffset, stationID, accountID, string(ChannelTypeSub2API), string(ChannelTypeNewAPI)).Scan(&sessions).Error
	return sessions, err
}
