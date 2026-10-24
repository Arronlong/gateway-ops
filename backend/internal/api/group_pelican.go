package api

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/guliansheng/gateway-ops/internal/relay"
	"github.com/guliansheng/gateway-ops/internal/storage"
)

func groupPelicanIDs(c *gin.Context) (uint, int64, error) {
	station, err := uintParam(c, "id")
	if err != nil {
		return 0, 0, err
	}
	group, err := strconv.ParseInt(c.Param("external_id"), 10, 64)
	if err != nil || group <= 0 {
		return 0, 0, errors.New("分组 ID 无效")
	}
	return station, group, nil
}

func pelicanPublicPageEnabled(d *Deps) (bool, error) {
	if d.Settings == nil {
		return true, nil
	}
	value, found, err := d.Settings.Get(storage.SettingPelicanPageEnabled)
	if err != nil {
		return false, err
	}
	if !found {
		return true, nil
	}
	return strconv.ParseBool(value)
}
func groupPelicanGallery(c *gin.Context, d *Deps, public bool) {
	station, err := uintParam(c, "id")
	if err != nil {
		fail(c, 400, err)
		return
	}
	if _, err = d.RelayStations.FindByID(station); err != nil {
		fail(c, 404, err)
		return
	}
	limit, err := pelicanHistoryCount(d)
	if err != nil {
		fail(c, 500, err)
		return
	}
	if value := c.Query("limit"); value != "" {
		requested, e := strconv.Atoi(value)
		if e != nil || requested < 1 || requested > storage.PelicanHistoryLimitMax {
			fail(c, 400, errors.New("作品数量无效"))
			return
		}
		if requested < limit {
			limit = requested
		}
	}
	if public && limit > 3 {
		limit = 3
	}
	if public {
		enabled, e := pelicanPublicPageEnabled(d)
		if e != nil {
			fail(c, 500, e)
			return
		}
		if !enabled {
			c.JSON(200, gin.H{"data": gin.H{"page_enabled": false, "configs": []storage.GroupPelicanConfig{}, "results": []storage.PelicanResult{}}})
			return
		}
	}
	results, err := d.RelayStations.GroupPelicanGallery(station, limit, public)
	if err != nil {
		fail(c, 500, err)
		return
	}
	configs := make([]storage.GroupPelicanConfig, 0)
	if public {
		filtered := make([]storage.PelicanResult, 0)
		for _, result := range results {
			if result.Status == "success" {
				result.Error = ""
				filtered = append(filtered, result)
			}
		}
		results = filtered
	} else {
		configs, err = d.RelayStations.ListGroupPelicanConfigs(station)
		if err != nil {
			fail(c, 500, err)
			return
		}
	}
	c.JSON(200, gin.H{"data": gin.H{"page_enabled": true, "configs": configs, "results": results}})
}
func saveGroupPelican(c *gin.Context, d *Deps) {
	station, group, err := groupPelicanIDs(c)
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
	config, err := d.Relay.SaveGroupPelicanConfig(station, group, input.Enabled)
	if err != nil {
		fail(c, 400, err)
		return
	}
	c.JSON(200, gin.H{"data": config})
}
func startGroupPelican(c *gin.Context, d *Deps) {
	station, group, err := groupPelicanIDs(c)
	if err != nil {
		fail(c, 400, err)
		return
	}
	plan, err := pelicanRunPlan(c)
	if err != nil {
		fail(c, http.StatusBadRequest, err)
		return
	}
	config, err := d.Relay.StartGroupPelican(c.Request.Context(), station, group, pelicanRequestedPlans(plan)...)
	if err != nil {
		status := 400
		if errors.Is(err, relay.ErrPelicanRunning) {
			status = 409
		}
		fail(c, status, err)
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"data": config})
}
func groupPelicanResult(c *gin.Context, d *Deps, public bool) {
	station, group, err := groupPelicanIDs(c)
	if err != nil {
		fail(c, 400, err)
		return
	}
	id, err := uintParam(c, "result_id")
	if err != nil {
		fail(c, 400, err)
		return
	}
	if public {
		if err = d.RelayStations.PublicPelicanGroup(station, group); err != nil {
			fail(c, 404, errors.New("作品不存在"))
			return
		}
		if d.Settings != nil {
			enabled, e := pelicanPublicPageEnabled(d)
			if e != nil {
				fail(c, 500, e)
				return
			}
			if !enabled {
				fail(c, 404, errors.New("测智页面已隐藏"))
				return
			}
		}
	} else if _, err = d.RelayStations.FindByID(station); err != nil {
		fail(c, 404, err)
		return
	}
	result, err := d.RelayStations.GroupPelicanResult(station, group, id)
	if err != nil {
		fail(c, 404, err)
		return
	}
	if public {
		if result.Status != "success" {
			fail(c, 404, errors.New("作品不存在"))
			return
		}
		result.Prompt = ""
		result.Error = ""
	}
	c.JSON(200, gin.H{"data": result})
}
func deleteGroupPelicanResult(c *gin.Context, d *Deps) {
	station, group, err := groupPelicanIDs(c)
	if err != nil {
		fail(c, 400, err)
		return
	}
	id, err := uintParam(c, "result_id")
	if err != nil {
		fail(c, 400, err)
		return
	}
	if _, err = d.RelayStations.FindByID(station); err != nil {
		fail(c, 404, err)
		return
	}
	if err = d.RelayStations.DeleteGroupPelicanResult(station, group, id); err != nil {
		fail(c, 404, err)
		return
	}
	c.JSON(200, gin.H{"data": gin.H{"deleted": true}})
}
func pelicanSources(c *gin.Context, d *Deps) {
	limit, err := pelicanHistoryCount(d)
	if err != nil {
		fail(c, 500, err)
		return
	}
	groups, err := d.RelayStations.PelicanSourceGallery(limit, c.Query("source_id"))
	if err != nil {
		if errors.Is(err, storage.ErrInvalidPelicanSource) {
			fail(c, 400, err)
			return
		}
		fail(c, 500, err)
		return
	}
	c.JSON(200, gin.H{"data": gin.H{"groups": groups}})
}
