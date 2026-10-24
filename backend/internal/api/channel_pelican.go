package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

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
