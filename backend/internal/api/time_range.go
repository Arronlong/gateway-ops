package api

import (
	"errors"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

func parseCustomTimeRange(startRaw, endRaw string) (time.Time, time.Time, error) {
	start, err := time.Parse(time.RFC3339Nano, startRaw)
	if err != nil {
		return time.Time{}, time.Time{}, errors.New("start must be an RFC3339 timestamp")
	}
	end, err := time.Parse(time.RFC3339Nano, endRaw)
	if err != nil {
		return time.Time{}, time.Time{}, errors.New("end must be an RFC3339 timestamp")
	}
	if !end.After(start) {
		return time.Time{}, time.Time{}, errors.New("end must be later than start")
	}
	return start.UTC(), end.UTC(), nil
}

func requestedTimeWindow(c *gin.Context, defaultRange string) (time.Time, time.Time, string, error) {
	rangeName := strings.ToLower(strings.TrimSpace(c.DefaultQuery("range", defaultRange)))
	if rangeName == "custom" {
		start, end, err := parseCustomTimeRange(c.Query("start"), c.Query("end"))
		return start, end, rangeName, err
	}
	since, rangeName := operationSince(rangeName)
	return since, time.Now().UTC(), rangeName, nil
}

func requestedTimeLocation(c *gin.Context) (*time.Location, error) {
	name := strings.TrimSpace(c.Query("time_zone"))
	if name == "" {
		return time.Local, nil
	}
	location, err := time.LoadLocation(name)
	if err != nil {
		return nil, errors.New("time_zone must be a valid IANA time zone")
	}
	return location, nil
}

func customRangeEnd(rangeName string, until time.Time) time.Time {
	if rangeName == "custom" {
		return until
	}
	return time.Time{}
}
