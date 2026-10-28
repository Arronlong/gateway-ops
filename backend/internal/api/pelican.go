package api

import (
	"errors"
	"github.com/gin-gonic/gin"
	"github.com/guliansheng/gateway-ops/internal/relay"
	"github.com/guliansheng/gateway-ops/internal/storage"
	"io"
	"net/http"
	"strconv"

	"gorm.io/gorm"
)

func pelicanIDs(c *gin.Context) (uint, int64, error) {
	stationID, err := uintParam(c, "id")
	if err != nil {
		return 0, 0, err
	}
	accountID, err := strconv.ParseInt(c.Param("external_id"), 10, 64)
	if err != nil || accountID <= 0 {
		return 0, 0, errors.New("账号 ID 无效")
	}
	return stationID, accountID, nil
}

func pelicanChannelID(c *gin.Context) (uint, error) {
	raw := c.Query("channel_id")
	if raw == "" {
		return 0, nil
	}
	parsed, err := strconv.ParseUint(raw, 10, 32)
	if err != nil || parsed == 0 {
		return 0, errors.New("监控渠道 ID 无效")
	}
	return uint(parsed), nil
}

func pelicanRunPlan(c *gin.Context) (*storage.PelicanTestPlan, error) {
	if c.Request.Body == nil || c.Request.Body == http.NoBody {
		return nil, nil
	}
	var plan storage.PelicanTestPlan
	if err := c.ShouldBindJSON(&plan); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, nil
		}
		return nil, err
	}
	return &plan, nil
}

func pelicanRequestedPlans(plan *storage.PelicanTestPlan) []storage.PelicanTestPlan {
	if plan == nil {
		return nil
	}
	return []storage.PelicanTestPlan{*plan}
}

func pelicanHistoryCount(d *Deps) (int, error) {
	if d.Settings == nil {
		return storage.PelicanHistoryLimit, nil
	}
	return d.Settings.PelicanHistoryCount()
}

func relayPelicanGallery(c *gin.Context, d *Deps) {
	stationID, err := uintParam(c, "id")
	if err != nil {
		fail(c, 400, err)
		return
	}
	if _, err = d.RelayStations.FindByID(stationID); err != nil {
		fail(c, 404, err)
		return
	}
	configs, err := d.RelayStations.ListPelicanConfigs(stationID)
	if err != nil {
		fail(c, 500, err)
		return
	}
	historyCount, err := pelicanHistoryCount(d)
	if err != nil {
		fail(c, 500, err)
		return
	}
	var results []storage.PelicanResult
	if c.Query("account_type") == "oauth" {
		results, err = d.RelayStations.PoolPelicanGallery(stationID, 3)
	} else {
		results, err = d.RelayStations.PelicanGallery(stationID, historyCount)
	}
	if err != nil {
		fail(c, 500, err)
		return
	}
	c.JSON(200, gin.H{"data": gin.H{"configs": configs, "results": results}})
}

func pelicanLibrary(c *gin.Context, d *Deps) {
	historyCount, err := pelicanHistoryCount(d)
	if err != nil {
		fail(c, 500, err)
		return
	}
	filter := storage.PelicanLibraryFilter{Status: c.DefaultQuery("status", "all"), Search: c.Query("search"), Page: 1, PageSize: 60, HistoryCount: historyCount}
	if value := c.Query("page"); value != "" {
		page, err := strconv.Atoi(value)
		if err != nil || page < 1 {
			fail(c, http.StatusBadRequest, errors.New("invalid page"))
			return
		}
		filter.Page = page
	}
	if value := c.Query("page_size"); value != "" {
		pageSize, err := strconv.Atoi(value)
		if err != nil || pageSize < 1 || pageSize > 100 {
			fail(c, http.StatusBadRequest, errors.New("page_size must be between 1 and 100"))
			return
		}
		filter.PageSize = pageSize
	}
	if value := c.Query("station_id"); value != "" {
		stationID, err := strconv.ParseUint(value, 10, 64)
		if err != nil || stationID == 0 {
			fail(c, http.StatusBadRequest, errors.New("invalid station_id"))
			return
		}
		filter.StationID = uint(stationID)
	}
	if filter.Status != "all" && filter.Status != "success" && filter.Status != "failed" {
		fail(c, http.StatusBadRequest, errors.New("status must be all, success, or failed"))
		return
	}
	if len([]rune(filter.Search)) > 100 {
		fail(c, http.StatusBadRequest, errors.New("search must be at most 100 characters"))
		return
	}
	page, err := d.RelayStations.PelicanLibrary(filter)
	if err != nil {
		fail(c, http.StatusInternalServerError, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": page})
}

func saveRelayPelicanConfig(c *gin.Context, d *Deps) {
	stationID, accountID, err := pelicanIDs(c)
	if err != nil {
		fail(c, 400, err)
		return
	}
	var input struct {
		Enabled bool `json:"enabled"`
	}
	if err = c.ShouldBindJSON(&input); err != nil {
		fail(c, 400, err)
		return
	}
	channelID, err := pelicanChannelID(c)
	if err != nil {
		fail(c, http.StatusBadRequest, err)
		return
	}
	config, err := d.Relay.SavePelicanConfigForChannel(stationID, accountID, channelID, storage.PelicanConfig{Enabled: input.Enabled})
	if err != nil {
		fail(c, 400, err)
		return
	}
	c.JSON(200, gin.H{"data": config})
}
func startRelayPelican(c *gin.Context, d *Deps) {
	stationID, accountID, err := pelicanIDs(c)
	if err != nil {
		fail(c, 400, err)
		return
	}
	plan, err := pelicanRunPlan(c)
	if err != nil {
		fail(c, http.StatusBadRequest, err)
		return
	}
	channelID, err := pelicanChannelID(c)
	if err != nil {
		fail(c, http.StatusBadRequest, err)
		return
	}
	config, err := d.Relay.StartPelicanForChannel(c.Request.Context(), stationID, accountID, channelID, pelicanRequestedPlans(plan)...)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, relay.ErrPelicanRunning) {
			status = http.StatusConflict
		}
		fail(c, status, err)
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"data": config})
}
func relayPelicanResult(c *gin.Context, d *Deps) {
	stationID, accountID, err := pelicanIDs(c)
	if err != nil {
		fail(c, 400, err)
		return
	}
	resultID, err := uintParam(c, "result_id")
	if err != nil {
		fail(c, 400, err)
		return
	}
	if _, err = d.RelayStations.FindByID(stationID); err != nil {
		fail(c, 404, err)
		return
	}
	result, err := d.RelayStations.PelicanResult(stationID, accountID, resultID)
	if err != nil {
		fail(c, 404, err)
		return
	}
	c.JSON(200, gin.H{"data": result})
}

func deleteRelayPelicanResult(c *gin.Context, d *Deps) {
	stationID, accountID, err := pelicanIDs(c)
	if err != nil || stationID == 0 {
		if err == nil {
			err = errors.New("invalid station ID")
		}
		fail(c, http.StatusBadRequest, err)
		return
	}
	resultID, err := uintParam(c, "result_id")
	if err != nil || resultID == 0 {
		if err == nil {
			err = errors.New("invalid result ID")
		}
		fail(c, http.StatusBadRequest, err)
		return
	}
	if err := d.RelayStations.DeletePelicanResult(stationID, accountID, resultID); err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, gorm.ErrRecordNotFound) {
			status = http.StatusNotFound
		}
		fail(c, status, err)
		return
	}
	c.Status(http.StatusNoContent)
}
