package storage

import (
	"net/url"
	"path"
	"sort"
	"strings"
	"time"
	"unicode"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	ChannelRelayLinkModeAuto   = "auto"
	ChannelRelayLinkModeManual = "manual"
)

type RelayAssociationAccount struct {
	ExternalID int64  `json:"external_id"`
	Name       string `json:"name"`
	BaseURL    string `json:"base_url,omitempty"`
	Identity   string `json:"identity,omitempty"`
}

type RelayAssociationStation struct {
	ID       uint                      `json:"id"`
	Name     string                    `json:"name"`
	Accounts []RelayAssociationAccount `json:"accounts"`
}

type ChannelRelayAssociation struct {
	ChannelID              uint   `json:"channel_id"`
	ChannelAccountID       uint   `json:"channel_account_id"`
	ChannelAccountUsername string `json:"channel_account_username"`
	RelayStationID         uint   `json:"relay_station_id"`
	RelayStationName       string `json:"relay_station_name"`
	RelayAccountExternalID int64  `json:"relay_account_external_id"`
	RelayAccountName       string `json:"relay_account_name,omitempty"`
	Mode                   string `json:"mode"`
	Linked                 bool   `json:"linked"`
}

type ChannelRelayAssociationSnapshot struct {
	Stations []RelayAssociationStation `json:"stations"`
	Links    []ChannelRelayAssociation `json:"links"`
}

func normalizeAssociationSitePath(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return strings.ToLower(strings.TrimRight(raw, "/"))
	}
	host := strings.ToLower(u.Hostname())
	if port := u.Port(); port != "" {
		host += ":" + port
	}
	cleanPath := path.Clean("/" + strings.TrimSpace(u.EscapedPath()))
	if cleanPath == "." || cleanPath == "/" {
		cleanPath = ""
	}
	return host + strings.TrimRight(cleanPath, "/")
}

func normalizeAssociationIdentity(raw string) string {
	return strings.ToLower(strings.TrimSpace(raw))
}

func normalizeAssociationMatchText(raw string) string {
	raw = strings.ToLower(strings.TrimSpace(raw))
	var builder strings.Builder
	for _, char := range raw {
		if unicode.IsLetter(char) || unicode.IsDigit(char) {
			builder.WriteRune(char)
		}
	}
	return builder.String()
}

func associationTextMatches(target, candidate string) bool {
	target = normalizeAssociationMatchText(target)
	candidate = normalizeAssociationMatchText(candidate)
	if target == "" || candidate == "" || len([]rune(target)) < 3 {
		return false
	}
	return target == candidate || strings.Contains(candidate, target) || strings.Contains(target, candidate)
}

func automaticRelayAccountMatchScore(channel Channel, channelAccount ChannelAccount, relayAccount RelayAccount) (int, bool) {
	if normalizeAssociationSitePath(channel.SiteURL) == "" || normalizeAssociationSitePath(channel.SiteURL) != normalizeAssociationSitePath(relayAccount.BaseURL) {
		return 0, false
	}

	score := 0
	if identity := normalizeAssociationIdentity(channelAccount.Username); identity != "" && identity == normalizeAssociationIdentity(relayAccount.Identity) {
		score += 100
	}
	if associationTextMatches(channelAccount.Username, relayAccount.Name) {
		score += 80
	}
	if associationTextMatches(channel.Name, relayAccount.Name) {
		score += 60
	}

	expectedType := normalizeAssociationMatchText(string(channel.Type))
	relayType := normalizeAssociationMatchText(relayAccount.Type)
	relayName := normalizeAssociationMatchText(relayAccount.Name)
	if expectedType != "" && relayType == expectedType {
		score += 30
	} else if expectedType != "" && strings.Contains(relayName, expectedType) {
		score += 20
	}

	return score, score > 0
}

func selectAutomaticRelayAccount(channel Channel, channelAccount ChannelAccount, relayAccounts []RelayAccount) (RelayAccount, bool) {
	var selected RelayAccount
	selectedScore := 0
	selectedCount := 0
	for _, relayAccount := range relayAccounts {
		score, ok := automaticRelayAccountMatchScore(channel, channelAccount, relayAccount)
		if !ok {
			continue
		}
		if score > selectedScore {
			selected = relayAccount
			selectedScore = score
			selectedCount = 1
			continue
		}
		if score == selectedScore {
			selectedCount++
		}
	}
	if selectedScore == 0 || selectedCount != 1 {
		return RelayAccount{}, false
	}
	return selected, true
}

func (r *RelayStations) ReconcileChannelRelayAccountLinks() error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		var channels []Channel
		if err := tx.Find(&channels).Error; err != nil {
			return err
		}
		channelByID := make(map[uint]Channel, len(channels))
		for _, channel := range channels {
			channelByID[channel.ID] = channel
		}

		var channelAccounts []ChannelAccount
		if err := tx.Find(&channelAccounts).Error; err != nil {
			return err
		}
		channelAccountByID := make(map[uint]ChannelAccount, len(channelAccounts))
		for _, account := range channelAccounts {
			channelAccountByID[account.ID] = account
		}

		var stations []RelayStation
		if err := tx.Order("id ASC").Find(&stations).Error; err != nil {
			return err
		}
		stationByID := make(map[uint]RelayStation, len(stations))
		for _, station := range stations {
			stationByID[station.ID] = station
		}

		var relayAccounts []RelayAccount
		if err := tx.Order("relay_station_id ASC, external_id ASC").Find(&relayAccounts).Error; err != nil {
			return err
		}
		relayByStation := make(map[uint][]RelayAccount)
		relayExists := make(map[string]struct{}, len(relayAccounts))
		for _, account := range relayAccounts {
			relayByStation[account.RelayStationID] = append(relayByStation[account.RelayStationID], account)
			relayExists[automaticBindingKey(account.RelayStationID, account.ExternalID)] = struct{}{}
		}

		var existing []ChannelRelayAccountLink
		if err := tx.Find(&existing).Error; err != nil {
			return err
		}
		existingByPair := make(map[string]ChannelRelayAccountLink, len(existing))
		for _, link := range existing {
			if _, ok := channelAccountByID[link.ChannelAccountID]; !ok {
				if err := tx.Delete(&link).Error; err != nil {
					return err
				}
				continue
			}
			if _, ok := stationByID[link.RelayStationID]; !ok {
				if err := tx.Delete(&link).Error; err != nil {
					return err
				}
				continue
			}
			if link.RelayAccountExternalID > 0 {
				if _, ok := relayExists[automaticBindingKey(link.RelayStationID, link.RelayAccountExternalID)]; !ok {
					if err := tx.Delete(&link).Error; err != nil {
						return err
					}
					continue
				}
			}
			existingByPair[automaticBindingKey(uint(link.ChannelAccountID), int64(link.RelayStationID))] = link
		}

		for _, channelAccount := range channelAccounts {
			channel, channelExists := channelByID[channelAccount.ChannelID]
			if !channelExists {
				continue
			}
			for _, station := range stations {
				pairKey := automaticBindingKey(channelAccount.ID, int64(station.ID))
				current, hasCurrent := existingByPair[pairKey]
				if hasCurrent && current.Mode == ChannelRelayLinkModeManual {
					continue
				}

				candidate, matched := selectAutomaticRelayAccount(channel, channelAccount, relayByStation[station.ID])
				if !matched {
					if hasCurrent && current.Mode == ChannelRelayLinkModeAuto {
						if err := tx.Delete(&current).Error; err != nil {
							return err
						}
					}
					continue
				}

				link := ChannelRelayAccountLink{
					ChannelAccountID:       channelAccount.ID,
					RelayStationID:         station.ID,
					RelayAccountExternalID: candidate.ExternalID,
					Mode:                   ChannelRelayLinkModeAuto,
				}
				if err := tx.Clauses(clause.OnConflict{
					Columns:   []clause.Column{{Name: "channel_account_id"}, {Name: "relay_station_id"}},
					DoUpdates: clause.AssignmentColumns([]string{"relay_account_external_id", "mode", "updated_at"}),
				}).Create(&link).Error; err != nil {
					return err
				}
			}
		}
		return nil
	})
}

func (r *RelayStations) ChannelRelayAssociationSnapshot() (*ChannelRelayAssociationSnapshot, error) {
	if err := r.ReconcileChannelRelayAccountLinks(); err != nil {
		return nil, err
	}
	var stations []RelayStation
	if err := r.db.Order("id ASC").Find(&stations).Error; err != nil {
		return nil, err
	}
	var relayAccounts []RelayAccount
	if err := r.db.Order("relay_station_id ASC, name ASC, external_id ASC").Find(&relayAccounts).Error; err != nil {
		return nil, err
	}
	accountsByStation := make(map[uint][]RelayAssociationAccount)
	for _, account := range relayAccounts {
		accountsByStation[account.RelayStationID] = append(accountsByStation[account.RelayStationID], RelayAssociationAccount{
			ExternalID: account.ExternalID,
			Name:       account.Name,
			BaseURL:    account.BaseURL,
			Identity:   account.Identity,
		})
	}
	stationViews := make([]RelayAssociationStation, 0, len(stations))
	stationName := make(map[uint]string, len(stations))
	for _, station := range stations {
		stationName[station.ID] = station.Name
		stationViews = append(stationViews, RelayAssociationStation{
			ID: station.ID, Name: station.Name, Accounts: accountsByStation[station.ID],
		})
	}

	var links []ChannelRelayAccountLink
	if err := r.db.Order("channel_account_id ASC, relay_station_id ASC").Find(&links).Error; err != nil {
		return nil, err
	}
	var channelAccounts []ChannelAccount
	if err := r.db.Find(&channelAccounts).Error; err != nil {
		return nil, err
	}
	accountByID := make(map[uint]ChannelAccount, len(channelAccounts))
	for _, account := range channelAccounts {
		accountByID[account.ID] = account
	}
	relayName := make(map[string]string, len(relayAccounts))
	for _, account := range relayAccounts {
		relayName[automaticBindingKey(account.RelayStationID, account.ExternalID)] = account.Name
	}
	linkViews := make([]ChannelRelayAssociation, 0, len(links))
	for _, link := range links {
		account, ok := accountByID[link.ChannelAccountID]
		if !ok {
			continue
		}
		linkViews = append(linkViews, ChannelRelayAssociation{
			ChannelID:              account.ChannelID,
			ChannelAccountID:       account.ID,
			ChannelAccountUsername: account.Username,
			RelayStationID:         link.RelayStationID,
			RelayStationName:       stationName[link.RelayStationID],
			RelayAccountExternalID: link.RelayAccountExternalID,
			RelayAccountName:       relayName[automaticBindingKey(link.RelayStationID, link.RelayAccountExternalID)],
			Mode:                   link.Mode,
			Linked:                 link.RelayAccountExternalID > 0,
		})
	}
	sort.Slice(linkViews, func(i, j int) bool {
		if linkViews[i].ChannelID != linkViews[j].ChannelID {
			return linkViews[i].ChannelID < linkViews[j].ChannelID
		}
		if linkViews[i].ChannelAccountID != linkViews[j].ChannelAccountID {
			return linkViews[i].ChannelAccountID < linkViews[j].ChannelAccountID
		}
		return linkViews[i].RelayStationID < linkViews[j].RelayStationID
	})
	return &ChannelRelayAssociationSnapshot{Stations: stationViews, Links: linkViews}, nil
}

func (r *RelayStations) SetChannelRelayAccountLink(channelID, channelAccountID, stationID uint, relayAccountExternalID *int64, mode string) error {
	if mode != ChannelRelayLinkModeAuto && mode != ChannelRelayLinkModeManual {
		return gorm.ErrInvalidValue
	}
	return r.db.Transaction(func(tx *gorm.DB) error {
		var channelAccount ChannelAccount
		if err := tx.Where("id = ? AND channel_id = ?", channelAccountID, channelID).First(&channelAccount).Error; err != nil {
			return err
		}
		var station RelayStation
		if err := tx.First(&station, stationID).Error; err != nil {
			return err
		}

		if mode == ChannelRelayLinkModeAuto {
			if err := tx.Where("channel_account_id = ? AND relay_station_id = ?", channelAccountID, stationID).
				Delete(&ChannelRelayAccountLink{}).Error; err != nil {
				return err
			}
			return nil
		}

		externalID := int64(0)
		if relayAccountExternalID != nil {
			externalID = *relayAccountExternalID
		}
		if externalID > 0 {
			var relayAccount RelayAccount
			if err := tx.Where("relay_station_id = ? AND external_id = ?", stationID, externalID).First(&relayAccount).Error; err != nil {
				return err
			}
			var occupied int64
			if err := tx.Model(&ChannelRelayAccountLink{}).
				Where("relay_station_id = ? AND relay_account_external_id = ? AND channel_account_id <> ?", stationID, externalID, channelAccountID).
				Count(&occupied).Error; err != nil {
				return err
			}
			if occupied > 0 {
				return gorm.ErrDuplicatedKey
			}
		}
		now := time.Now()
		link := ChannelRelayAccountLink{
			ChannelAccountID:       channelAccountID,
			RelayStationID:         stationID,
			RelayAccountExternalID: externalID,
			Mode:                   ChannelRelayLinkModeManual,
			UpdatedAt:              now,
		}
		return tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "channel_account_id"}, {Name: "relay_station_id"}},
			DoUpdates: clause.AssignmentColumns([]string{"relay_account_external_id", "mode", "updated_at"}),
		}).Create(&link).Error
	})
}
