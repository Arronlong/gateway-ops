package api

import (
	"github.com/gin-gonic/gin"
	"net/http"
)

func relayLatestTestRuns(c *gin.Context, d *Deps) {
	runs, err := d.RelayStations.LatestTestRuns()
	if err != nil {
		fail(c, http.StatusInternalServerError, err)
		return
	}
	for i := range runs {
		for j := range runs[i].Results {
			runs[i].Results[j].Output = ""
		}
	}
	c.JSON(http.StatusOK, gin.H{"data": runs})
}
func relayStationTestRuns(c *gin.Context, d *Deps) {
	id, err := uintParam(c, "id")
	if err != nil {
		fail(c, http.StatusBadRequest, err)
		return
	}
	if _, err = d.RelayStations.FindByID(id); err != nil {
		fail(c, http.StatusNotFound, err)
		return
	}
	runs, err := d.RelayStations.ListTestRuns(id)
	if err != nil {
		fail(c, http.StatusInternalServerError, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": runs})
}
