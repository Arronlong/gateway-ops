package storage

import "sort"

type ChannelPelicanInfo struct {
	ID                   uint        `json:"id"`
	Name                 string      `json:"name"`
	Type                 ChannelType `json:"type"`
	SiteURL              string      `json:"site_url"`
	PelicanKeyConfigured bool        `json:"pelican_key_configured"`
}

type ChannelPelicanKey struct {
	ChannelID  uint
	Ciphertext string
}

type ChannelPelicanTarget struct {
	RelayStationID    uint          `json:"relay_station_id"`
	StationName       string        `json:"station_name"`
	StationType       string        `json:"station_type"`
	AccountExternalID int64         `json:"account_external_id"`
	AccountName       string        `json:"account_name"`
	Config            PelicanConfig `json:"config"`
}

type ChannelPelicanGroup struct {
	Channel ChannelPelicanInfo     `json:"channel"`
	Targets []ChannelPelicanTarget `json:"targets"`
	Results []PelicanLibraryItem   `json:"results"`
}

func buildChannelPelicanGroups(channels []ChannelPelicanInfo, links []ChannelRelayAssociation, stations []RelayStation, configs []PelicanConfig, results []PelicanLibraryItem, historyCounts ...int) []ChannelPelicanGroup {
	historyCount := PelicanHistoryLimit
	if len(historyCounts) > 0 {
		historyCount = normalizedPelicanHistoryCount(historyCounts[0])
	}
	type accountKey struct {
		stationID uint
		accountID int64
	}
	stationTypes := make(map[uint]string)
	for _, station := range stations {
		stationTypes[station.ID] = station.Type
	}
	configByAccount := make(map[accountKey]PelicanConfig)
	for _, config := range configs {
		configByAccount[accountKey{config.RelayStationID, config.AccountExternalID}] = config
	}
	resultsByAccount := make(map[accountKey][]PelicanLibraryItem)
	for _, result := range results {
		key := accountKey{result.RelayStationID, result.AccountExternalID}
		resultsByAccount[key] = append(resultsByAccount[key], result)
	}
	groups := make([]ChannelPelicanGroup, 0, len(channels))
	for _, channel := range channels {
		group := ChannelPelicanGroup{Channel: channel, Targets: []ChannelPelicanTarget{}, Results: []PelicanLibraryItem{}}
		seen := make(map[accountKey]bool)
		for _, link := range links {
			if link.ChannelID != channel.ID || !link.Linked || link.RelayAccountExternalID <= 0 {
				continue
			}
			key := accountKey{link.RelayStationID, link.RelayAccountExternalID}
			if seen[key] {
				continue
			}
			seen[key] = true
			config, ok := configByAccount[key]
			if !ok {
				config = PelicanConfig{RelayStationID: key.stationID, AccountExternalID: key.accountID}
			}
			group.Targets = append(group.Targets, ChannelPelicanTarget{RelayStationID: key.stationID, StationName: link.RelayStationName, StationType: stationTypes[key.stationID], AccountExternalID: key.accountID, AccountName: link.RelayAccountName, Config: config})
			group.Results = append(group.Results, resultsByAccount[key]...)
		}
		sort.Slice(group.Results, func(i, j int) bool {
			left, right := group.Results[i], group.Results[j]
			if left.FinishedAt.Equal(right.FinishedAt) {
				return left.ID > right.ID
			}
			return left.FinishedAt.After(right.FinishedAt)
		})
		if len(group.Results) > historyCount {
			group.Results = group.Results[:historyCount]
		}
		groups = append(groups, group)
	}
	return groups
}

func (r *RelayStations) ChannelPelicanGallery(historyCounts ...int) ([]ChannelPelicanGroup, error) {
	historyCount := PelicanHistoryLimit
	if len(historyCounts) > 0 {
		historyCount = normalizedPelicanHistoryCount(historyCounts[0])
	}
	snapshot, err := r.ChannelRelayAssociationSnapshot()
	if err != nil {
		return nil, err
	}
	var channels []ChannelPelicanInfo
	if err := r.db.Model(&Channel{}).Select("id, name, type, site_url, (COALESCE(pelican_key_cipher, '') <> '') AS pelican_key_configured").Order("id ASC").Scan(&channels).Error; err != nil {
		return nil, err
	}
	var stations []RelayStation
	if err := r.db.Select("id, type").Find(&stations).Error; err != nil {
		return nil, err
	}
	var configs []PelicanConfig
	if err := r.db.Select("id, channel_id, relay_station_id, account_external_id, enabled, next_run_at, lease_until").Find(&configs).Error; err != nil {
		return nil, err
	}
	results := []PelicanLibraryItem{}
	err = r.db.Raw(`SELECT id, relay_station_id, account_external_id, account_name, model, reasoning_effort, trigger, status, error, duration_ms, started_at, finished_at, upstream_group_name, upstream_group_multiplier, station_name FROM (
	 SELECT results.*, stations.name AS station_name, ROW_NUMBER() OVER (PARTITION BY results.relay_station_id, results.account_external_id ORDER BY results.finished_at DESC, results.id DESC) AS row_number
	 FROM relay_pelican_results results
	 JOIN relay_stations stations ON stations.id = results.relay_station_id
	 JOIN relay_accounts accounts ON accounts.relay_station_id = results.relay_station_id AND accounts.external_id = results.account_external_id
	) ranked WHERE row_number <= ?`, historyCount).Scan(&results).Error
	if err != nil {
		return nil, err
	}
	return buildChannelPelicanGroups(channels, snapshot.Links, stations, configs, results, historyCount), nil
}

func (r *RelayStations) ChannelPelicanKeyForAccount(stationID uint, accountID int64, channelID uint) (ChannelPelicanKey, error) {
	var key ChannelPelicanKey
	err := r.db.Table("channels").
		Select("channels.id AS channel_id, channels.pelican_key_cipher AS ciphertext").
		Joins("JOIN channel_accounts ON channel_accounts.channel_id = channels.id AND channel_accounts.deleted_at IS NULL").
		Joins("JOIN channel_relay_account_links ON channel_relay_account_links.channel_account_id = channel_accounts.id").
		Where("channels.id = ? AND channels.deleted_at IS NULL AND channel_relay_account_links.relay_station_id = ? AND channel_relay_account_links.relay_account_external_id = ?", channelID, stationID, accountID).
		Take(&key).Error
	return key, err
}

func (r *RelayStations) ChannelPelicanKeysForAccount(stationID uint, accountID int64) ([]ChannelPelicanKey, error) {
	var keys []ChannelPelicanKey
	err := r.db.Table("channels").
		Select("DISTINCT channels.id AS channel_id, channels.pelican_key_cipher AS ciphertext").
		Joins("JOIN channel_accounts ON channel_accounts.channel_id = channels.id AND channel_accounts.deleted_at IS NULL").
		Joins("JOIN channel_relay_account_links ON channel_relay_account_links.channel_account_id = channel_accounts.id").
		Where("channels.deleted_at IS NULL AND channel_relay_account_links.relay_station_id = ? AND channel_relay_account_links.relay_account_external_id = ?", stationID, accountID).
		Order("channels.id ASC").
		Scan(&keys).Error
	return keys, err
}
