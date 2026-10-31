package api

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func startChannelPelican(c *gin.Context, d *Deps) {
	channelID, err := uintParam(c, "id")
	if err != nil {
		fail(c, http.StatusBadRequest, err)
		return
	}
	plan, err := pelicanRunPlan(c)
	if err != nil {
		fail(c, http.StatusBadRequest, err)
		return
	}
	if err := d.Relay.StartChannelPelican(c.Request.Context(), channelID, pelicanRequestedPlans(plan)...); err != nil {
		fail(c, http.StatusBadRequest, err)
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"data": gin.H{"channel_id": channelID}})
}

func channelPelicanResult(c *gin.Context, d *Deps) {
	channelID, err := uintParam(c, "id")
	if err != nil {
		fail(c, http.StatusBadRequest, err)
		return
	}
	resultID, err := uintParam(c, "result_id")
	if err != nil {
		fail(c, http.StatusBadRequest, err)
		return
	}
	result, err := d.RelayStations.ChannelPelicanResult(channelID, resultID)
	if err != nil {
		fail(c, http.StatusNotFound, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": result})
}

func deleteChannelPelicanResult(c *gin.Context, d *Deps) {
	channelID, err := uintParam(c, "id")
	if err != nil {
		fail(c, http.StatusBadRequest, err)
		return
	}
	resultID, err := uintParam(c, "result_id")
	if err != nil {
		fail(c, http.StatusBadRequest, err)
		return
	}
	if err := d.RelayStations.DeleteChannelPelicanResult(channelID, resultID); err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, gorm.ErrRecordNotFound) {
			status = http.StatusNotFound
		}
		fail(c, status, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func channelPelicanGallery(c *gin.Context, d *Deps) {
	historyCount, err := pelicanHistoryCount(d)
	if err != nil {
		fail(c, http.StatusInternalServerError, err)
		return
	}
	groups, err := d.RelayStations.ChannelPelicanGallery(historyCount)
	if err != nil {
		fail(c, http.StatusInternalServerError, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": gin.H{"groups": groups}})
}
