package api

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/guliansheng/gateway-ops/internal/storage"
	"gorm.io/gorm"
)

func registerPublicRelayStations(g *gin.RouterGroup, d *Deps) {
	g.GET("/relay-stations", func(c *gin.Context) {
		publicRelayStations(c, d)
	})
	g.GET("/relay-stations/:id/group-monitor", func(c *gin.Context) {
		publicRelayGroupMonitor(c, d)
	})
}

type publicRelayStationOption struct {
	ID   uint   `json:"id"`
	Name string `json:"name"`
}

func publicRelayStationOptions(stations []storage.RelayStation) []publicRelayStationOption {
	options := make([]publicRelayStationOption, 0, len(stations))
	for _, station := range stations {
		options = append(options, publicRelayStationOption{
			ID:   station.ID,
			Name: station.Name,
		})
	}
	return options
}

func publicRelayStations(c *gin.Context, d *Deps) {
	stations, err := d.RelayStations.List()
	if err != nil {
		fail(c, http.StatusInternalServerError, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": publicRelayStationOptions(stations)})
}

func publicRelayGroupMonitor(c *gin.Context, d *Deps) {
	stationID, err := uintParam(c, "id")
	if err != nil || stationID == 0 {
		fail(c, http.StatusBadRequest, errors.New("无效的中转站 ID"))
		return
	}
	rangeName := strings.ToLower(strings.TrimSpace(c.DefaultQuery("range", "24h")))
	if rangeName == "" {
		rangeName = "24h"
	}
	switch rangeName {
	case "today", "24h", "7d", "30d":
	default:
		fail(c, http.StatusBadRequest, errors.New("时间范围仅支持 today、24h、7d 或 30d"))
		return
	}
	view, err := d.Relay.PublicGroupMonitorForRange(c.Request.Context(), stationID, rangeName)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			fail(c, http.StatusNotFound, errors.New("中转站不存在"))
			return
		}
		fail(c, http.StatusBadGateway, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": view})
}
