package api

import (
	"net/http"
	"sort"

	"github.com/gin-gonic/gin"
	"github.com/guliansheng/gateway-ops/internal/relay"
	"github.com/guliansheng/gateway-ops/internal/storage"
)

type channelModelCatalogItem struct {
	ChannelID uint     `json:"channel_id"`
	Models    []string `json:"models"`
}

type relayAccountModelKey struct {
	StationID uint
	AccountID int64
}

func channelModelCatalog(c *gin.Context, d *Deps) {
	if d.RelayStations == nil || d.Relay == nil {
		c.JSON(http.StatusOK, gin.H{"data": []channelModelCatalogItem{}})
		return
	}

	snapshot, err := d.RelayStations.ChannelRelayAssociationSnapshot()
	if err != nil {
		fail(c, http.StatusInternalServerError, err)
		return
	}

	stationIDs := make([]uint, 0)
	stationSeen := make(map[uint]struct{})
	for _, link := range snapshot.Links {
		if !link.Linked || link.RelayAccountExternalID <= 0 {
			continue
		}
		if _, exists := stationSeen[link.RelayStationID]; exists {
			continue
		}
		stationSeen[link.RelayStationID] = struct{}{}
		stationIDs = append(stationIDs, link.RelayStationID)
	}
	sort.Slice(stationIDs, func(i, j int) bool { return stationIDs[i] < stationIDs[j] })

	modelsByRelayAccount := make(map[relayAccountModelKey][]string)
	for _, stationID := range stationIDs {
		catalog, catalogErr := d.Relay.AccountModelCatalog(c.Request.Context(), stationID)
		if catalogErr != nil {
			if d.Log != nil {
				d.Log.Warn("load relay account model catalog for channel search", "station_id", stationID, "err", catalogErr)
			}
			continue
		}
		addRelayAccountModels(modelsByRelayAccount, stationID, catalog)
	}

	c.JSON(http.StatusOK, gin.H{"data": buildChannelModelCatalog(snapshot.Links, modelsByRelayAccount)})
}

func addRelayAccountModels(target map[relayAccountModelKey][]string, stationID uint, catalog []relay.AccountModelCatalogItem) {
	for _, item := range catalog {
		target[relayAccountModelKey{StationID: stationID, AccountID: item.AccountID}] = item.Models
	}
}

func buildChannelModelCatalog(links []storage.ChannelRelayAssociation, modelsByRelayAccount map[relayAccountModelKey][]string) []channelModelCatalogItem {
	modelsByChannel := make(map[uint]map[string]struct{})
	for _, link := range links {
		if !link.Linked || link.RelayAccountExternalID <= 0 {
			continue
		}
		models := modelsByRelayAccount[relayAccountModelKey{StationID: link.RelayStationID, AccountID: link.RelayAccountExternalID}]
		if len(models) == 0 {
			continue
		}
		modelSet := modelsByChannel[link.ChannelID]
		if modelSet == nil {
			modelSet = make(map[string]struct{}, len(models))
			modelsByChannel[link.ChannelID] = modelSet
		}
		for _, model := range models {
			if model != "" {
				modelSet[model] = struct{}{}
			}
		}
	}

	channelIDs := make([]uint, 0, len(modelsByChannel))
	for channelID := range modelsByChannel {
		channelIDs = append(channelIDs, channelID)
	}
	sort.Slice(channelIDs, func(i, j int) bool { return channelIDs[i] < channelIDs[j] })

	result := make([]channelModelCatalogItem, 0, len(channelIDs))
	for _, channelID := range channelIDs {
		models := make([]string, 0, len(modelsByChannel[channelID]))
		for model := range modelsByChannel[channelID] {
			models = append(models, model)
		}
		sort.Strings(models)
		result = append(result, channelModelCatalogItem{ChannelID: channelID, Models: models})
	}
	return result
}
