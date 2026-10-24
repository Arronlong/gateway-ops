package storage

import "strconv"

const (
	SettingChannelTokenRefreshEnabled  = "sync.channel.token_refresh.enabled"
	SettingChannelTokenRefreshInterval = "sync.channel.token_refresh.interval_minutes"
)

func (s *Settings) TokenRefreshSettings() (bool, int, error) {
	enabled, interval := false, 30
	if value, found, err := s.Get(SettingChannelTokenRefreshEnabled); err != nil {
		return false, interval, err
	} else if found {
		enabled, err = strconv.ParseBool(value)
		if err != nil {
			return false, interval, err
		}
	}
	if value, found, err := s.Get(SettingChannelTokenRefreshInterval); err != nil {
		return enabled, interval, err
	} else if found {
		interval, err = strconv.Atoi(value)
		if err != nil {
			return enabled, 30, err
		}
		if interval < 1 {
			interval = 30
		}
	}
	return enabled, interval, nil
}
