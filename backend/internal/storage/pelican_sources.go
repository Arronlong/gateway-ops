package storage

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

var ErrInvalidPelicanSource = errors.New("测智来源参数无效")

type PelicanSourceGroup struct {
	ID          string          `json:"id"`
	Kind        string          `json:"kind"`
	Name        string          `json:"name"`
	SiteURL     string          `json:"site_url"`
	StationID   uint            `json:"station_id"`
	StationName string          `json:"station_name"`
	ExternalID  int64           `json:"external_id"`
	Config      any             `json:"config"`
	Results     []PelicanResult `json:"results"`
}

// 返回所有已配置的号池和销售分组，包括自动测智已关闭的来源，以便在作品库直接开启。
func (r *RelayStations) PelicanSourceGallery(limit int, requested ...string) ([]PelicanSourceGroup, error) {
	limit = normalizedPelicanHistoryCount(limit)
	poolWhere, groupWhere := "1 = 1", "1 = 1"
	var poolArgs, groupArgs []any
	if len(requested) > 0 && requested[0] != "" {
		parts := strings.Split(requested[0], ":")
		if len(parts) != 3 {
			return nil, ErrInvalidPelicanSource
		}
		station, err := strconv.ParseUint(parts[1], 10, 64)
		if err != nil || station == 0 {
			return nil, ErrInvalidPelicanSource
		}
		external, err := strconv.ParseInt(parts[2], 10, 64)
		if err != nil || external <= 0 {
			return nil, ErrInvalidPelicanSource
		}
		switch parts[0] {
		case "pool":
			poolWhere = "c.relay_station_id = ? AND c.account_external_id = ?"
			poolArgs = []any{station, external}
			groupWhere = "1 = 0"
			groupArgs = nil
		case "group":
			groupWhere = "c.relay_station_id = ? AND c.group_external_id = ?"
			groupArgs = []any{station, external}
			poolWhere = "1 = 0"
			poolArgs = nil
		default:
			return nil, ErrInvalidPelicanSource
		}
	}
	var pools []struct {
		PelicanConfig
		AccountName string
		StationName string
		SiteURL     string
	}
	err := r.db.Table("relay_pelican_configs c").Select("c.*,a.name AS account_name,s.name AS station_name,s.base_url AS site_url").Joins("JOIN relay_accounts a ON a.relay_station_id=c.relay_station_id AND a.external_id=c.account_external_id").Joins("JOIN relay_stations s ON s.id=c.relay_station_id AND s.deleted_at IS NULL").Where("a.type = ?", "oauth").Where(poolWhere, poolArgs...).Order("s.id,c.account_external_id").Scan(&pools).Error
	if err != nil {
		return nil, err
	}
	result := make([]PelicanSourceGroup, 0)
	accountHistory := make(map[uint][]PelicanResult)
	for _, pool := range pools {
		history, ok := accountHistory[pool.RelayStationID]
		if !ok {
			history, err = r.PelicanGallery(pool.RelayStationID, limit)
			if err != nil {
				return nil, err
			}
			accountHistory[pool.RelayStationID] = history
		}
		items := make([]PelicanResult, 0)
		for _, item := range history {
			if item.AccountExternalID == pool.AccountExternalID {
				items = append(items, item)
			}
		}
		result = append(result, PelicanSourceGroup{ID: fmt.Sprintf("pool:%d:%d", pool.RelayStationID, pool.AccountExternalID), Kind: "pool_account", Name: pool.AccountName, SiteURL: pool.SiteURL, StationID: pool.RelayStationID, StationName: pool.StationName, ExternalID: pool.AccountExternalID, Config: pool.PelicanConfig, Results: items})
	}
	var groups []struct {
		GroupPelicanConfig
		GroupName   string
		StationName string
		SiteURL     string
	}
	err = r.db.Table("relay_group_pelican_configs c").Select("c.*,g.name AS group_name,s.name AS station_name,s.base_url AS site_url").Joins("JOIN relay_groups g ON g.relay_station_id=c.relay_station_id AND g.external_id=c.group_external_id").Joins("JOIN relay_stations s ON s.id=c.relay_station_id AND s.deleted_at IS NULL").Where(groupWhere, groupArgs...).Order("s.id,c.group_external_id").Scan(&groups).Error
	if err != nil {
		return nil, err
	}
	groupHistory := make(map[uint][]PelicanResult)
	for _, group := range groups {
		history, ok := groupHistory[group.RelayStationID]
		if !ok {
			history, err = r.GroupPelicanGallery(group.RelayStationID, limit)
			if err != nil {
				return nil, err
			}
			groupHistory[group.RelayStationID] = history
		}
		items := make([]PelicanResult, 0)
		for _, item := range history {
			if item.GroupExternalID == group.GroupExternalID {
				items = append(items, item)
			}
		}
		result = append(result, PelicanSourceGroup{ID: fmt.Sprintf("group:%d:%d", group.RelayStationID, group.GroupExternalID), Kind: "relay_group", Name: group.GroupName, SiteURL: group.SiteURL, StationID: group.RelayStationID, StationName: group.StationName, ExternalID: group.GroupExternalID, Config: group.GroupPelicanConfig, Results: items})
	}
	return result, nil
}
