package api

import (
	"errors"
	"github.com/gin-gonic/gin"
	"github.com/guliansheng/gateway-ops/internal/relay"
	"github.com/guliansheng/gateway-ops/internal/storage"
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
	results, err := d.RelayStations.PelicanGallery(stationID)
	if err != nil {
		fail(c, 500, err)
		return
	}
	c.JSON(200, gin.H{"data": gin.H{"configs": configs, "results": results}})
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
	config, err := d.Relay.SavePelicanConfig(stationID, accountID, storage.PelicanConfig{Enabled: input.Enabled})
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
	config, err := d.Relay.StartPelican(c.Request.Context(), stationID, accountID)
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
