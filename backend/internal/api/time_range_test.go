package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func testTimeRangeContext(query string) *gin.Context {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/?"+query, nil)
	return c
}

func TestParseCustomTimeRangeUsesHalfOpenUTCWindow(t *testing.T) {
	start, end, err := parseCustomTimeRange("2026-10-21T16:00:00Z", "2026-10-24T16:00:00Z")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := start.Format(time.RFC3339), "2026-10-21T16:00:00Z"; got != want {
		t.Fatalf("start = %s, want %s", got, want)
	}
	if got, want := end.Format(time.RFC3339), "2026-10-24T16:00:00Z"; got != want {
		t.Fatalf("end = %s, want %s", got, want)
	}
}

func TestParseCustomTimeRangeRejectsMissingMalformedAndReversedBounds(t *testing.T) {
	for _, test := range []struct {
		name  string
		start string
		end   string
	}{
		{name: "missing start", end: "2026-10-24T16:00:00Z"},
		{name: "missing end", start: "2026-10-21T16:00:00Z"},
		{name: "malformed offset", start: "2026-10-21T16:00:00", end: "2026-10-24T16:00:00Z"},
		{name: "reversed", start: "2026-10-24T16:00:00Z", end: "2026-10-21T16:00:00Z"},
		{name: "empty", start: "2026-10-21T16:00:00Z", end: "2026-10-21T16:00:00Z"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, _, err := parseCustomTimeRange(test.start, test.end); err == nil {
				t.Fatal("expected invalid range error")
			}
		})
	}
}

func TestRequestedTimeWindowReadsCustomBounds(t *testing.T) {
	c := testTimeRangeContext("range=custom&start=2026-10-21T16%3A00%3A00Z&end=2026-10-24T16%3A00%3A00Z")
	since, until, rangeName, err := requestedTimeWindow(c, "today")
	if err != nil {
		t.Fatal(err)
	}
	if rangeName != "custom" || since.Format(time.RFC3339) != "2026-10-21T16:00:00Z" || until.Format(time.RFC3339) != "2026-10-24T16:00:00Z" {
		t.Fatalf("window = %s, %s, %s", rangeName, since, until)
	}
}

func TestRequestedTimeLocationValidatesIANAZone(t *testing.T) {
	location, err := requestedTimeLocation(testTimeRangeContext("time_zone=Asia%2FShanghai"))
	if err != nil {
		t.Fatal(err)
	}
	if location.String() != "Asia/Shanghai" {
		t.Fatalf("location = %s, want Asia/Shanghai", location)
	}
	if _, err := requestedTimeLocation(testTimeRangeContext("time_zone=Not%2FAZone")); err == nil {
		t.Fatal("expected invalid IANA time zone error")
	}
}

func TestStatisticsHandlersRejectIncompleteCustomWindow(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for name, handler := range map[string]func(*gin.Context, *Deps){
		"dashboard":           dashboardSummary,
		"balance trend":       dashboardBalanceTrend,
		"channel summary":     channelSummary,
		"channel metrics":     channelMetrics,
		"channel latency":     channelLatencyBuckets,
		"channel consumption": channelConsumptionBuckets,
		"operation summary":   operationsSummary,
		"ledger":              listOperationLedger,
		"relay daily":         relayStationDailyConsumption,
		"relay usage":         relayStationUsage,
		"relay recent":        relayStationRecentUsage,
		"relay users":         relayStationUsers,
		"relay latency":       relayStationLatencyBuckets,
		"relay consumption":   relayStationConsumptionBuckets,
		"public monitor":      publicRelayGroupMonitor,
	} {
		t.Run(name, func(t *testing.T) {
			response := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(response)
			c.Params = gin.Params{{Key: "id", Value: "1"}}
			c.Request = httptest.NewRequest(http.MethodGet, "/?range=custom&start=2026-10-21T16%3A00%3A00Z", nil)
			handler(c, nil)
			if response.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400", response.Code)
			}
		})
	}
}
