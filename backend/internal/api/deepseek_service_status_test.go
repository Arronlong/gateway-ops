package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestFetchDeepSeekPublicServiceStatusFrom(t *testing.T) {
	now := time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)
	start := now.Add(-time.Hour).Unix()
	impactStart := now.AddDate(0, 0, -2).Truncate(24 * time.Hour).Add(time.Hour).Unix()
	impactEnd := now.AddDate(0, 0, -1).Truncate(24 * time.Hour).Add(time.Hour).Unix()
	requested := map[string]bool{}
	var requestedMu sync.Mutex
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestedMu.Lock()
		requested[r.URL.Path] = true
		requestedMu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/status-page/123/summary/active":
			fmt.Fprintf(w, `{"data":{"page":{"page_id":123,"name":"DeepSeek","components":[{"component_id":"api-v4","name":"API Service"},{"component_id":"chat","section_id":"chat-section","name":"Chat Service"}],"sections":[{"section_id":"chat-section","name":"Chat"}]},"active_changes":[{"change_id":12,"type":"incident","title":"API degraded","description":"Investigating","status":"identified","affected_components":[{"component_id":"api-v4","name":"API Service","status":"degraded"}],"start_at_seconds":%d,"updates":[{"update_id":"u12","at_seconds":%d,"status":"identified","description":"Investigating","component_changes":[{"component_id":"api-v4","component_name":"API Service","status":"partial_outage"}]}]}]}}`, start, start)
		case "/api/status-page/123/summary/structure":
			if r.URL.Query().Get("start_at_from_seconds") != fmt.Sprint(now.Truncate(24*time.Hour).AddDate(0, 0, 1-statusHistoryDays).Unix()) {
				t.Errorf("unexpected history start: %s", r.URL.Query().Get("start_at_from_seconds"))
			}
			fmt.Fprintf(w, `{"data":{"section_impacts":[{"section_id":"chat-section","change_id":11,"start_at_seconds":%d,"end_at_seconds":%d,"status":"degraded"}],"section_uptimes":[{"section_id":"chat-section","uptime":99.7}],"component_impacts":[{"component_id":"api-v4","change_id":11,"start_at_seconds":%d,"end_at_seconds":%d,"status":"degraded"}],"component_uptimes":[{"component_id":"api-v4","uptime":99.4},{"component_id":"chat","uptime":99.8}]}}`, impactStart, impactEnd, impactStart, impactEnd)
		case "/api/status-page/123/change/list":
			if r.URL.Query().Get("start_at_seconds") != fmt.Sprint(now.AddDate(0, -2, 0).Unix()) {
				t.Errorf("unexpected incident history start: %s", r.URL.Query().Get("start_at_seconds"))
			}
			fmt.Fprintf(w, `{"data":{"items":[{"change_id":12,"type":"incident","title":"API degraded","description":"Investigating","status":"identified","affected_components":[{"component_id":"api-v4","name":"API Service","status":"partial_outage"}],"start_at_seconds":%d,"updates":[{"update_id":"u12","at_seconds":%d,"status":"identified","description":"Investigating","component_changes":[{"component_id":"api-v4","component_name":"API Service","status":"partial_outage"}]}]},{"change_id":11,"type":"incident","title":"Chat interruption","description":"Resolved","status":"resolved","affected_components":[{"component_id":"chat","name":"Chat Service","status":"operational"}],"start_at_seconds":%d,"close_at_seconds":%d,"updates":[{"update_id":"u11","at_seconds":%d,"status":"resolved","description":"Resolved"}]}]}}`, start, start, start-7*24*60*60, start-7*24*60*60+300, start-7*24*60*60+300)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	service := fetchDeepSeekPublicServiceStatusFrom(context.Background(), server.URL, 123, now)
	if service.ID != "deepseek" || service.Name != "DeepSeek" || service.Status.Indicator != "major" {
		t.Fatalf("unexpected service summary: %#v", service)
	}
	if len(service.Components) != 2 || service.Components[0].Status != "partial_outage" || len(service.Components[0].History) != statusHistoryDays {
		t.Fatalf("unexpected component mapping: %#v", service.Components)
	}
	if len(service.Groups) != 2 || service.Groups[0].Uptime != 99.4 || service.Groups[1].Uptime != 99.7 {
		t.Fatalf("unexpected group mapping: %#v", service.Groups)
	}
	sectionDate := time.Unix(impactStart, 0).UTC().Format("2006-01-02")
	foundSectionIncident := false
	for _, item := range service.Groups[1].History {
		if item.Date == sectionDate && len(item.Incidents) == 1 && item.Incidents[0].Name == "Chat interruption" {
			foundSectionIncident = true
		}
	}
	if !foundSectionIncident {
		t.Fatalf("expected linked incident in section history on %s", sectionDate)
	}
	day := time.Unix(impactStart, 0).UTC().Format("2006-01-02")
	foundDegradedDay := false
	for _, item := range service.Components[0].History {
		if item.Date == day && item.Status == "degraded_performance" {
			foundDegradedDay = true
		}
	}
	if !foundDegradedDay {
		t.Fatalf("expected degraded history on %s", day)
	}
	if len(service.Incidents) != 1 || service.Incidents[0].Name != "API degraded" || len(service.PastIncidents) != 1 || service.PastIncidents[0].Name != "Chat interruption" {
		t.Fatalf("unexpected incident mapping: current=%#v past=%#v", service.Incidents, service.PastIncidents)
	}
	for _, path := range []string{"/api/status-page/123/summary/active", "/api/status-page/123/summary/structure", "/api/status-page/123/change/list"} {
		requestedMu.Lock()
		if !requested[path] {
			t.Errorf("status source endpoint was not requested: %s", path)
		}
		requestedMu.Unlock()
	}
}

func TestFetchDeepSeekPublicServiceStatusFromMarksRequiredFailureUnknown(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	service := fetchDeepSeekPublicServiceStatusFrom(context.Background(), server.URL, 123, time.Now().UTC())
	if service.Status.Indicator != "unknown" || service.Error == "" {
		t.Fatalf("expected unknown status and an error, got %#v", service)
	}
}

func TestDeepSeekPartialHistoryFailureDoesNotInventHealthyHistory(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/summary/active") {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"data":{"page":{"page_id":123,"components":[{"component_id":"api","name":"API"}]},"active_changes":[]}}`)
			return
		}
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()
	service := fetchDeepSeekPublicServiceStatusFrom(context.Background(), server.URL, 123, time.Now().UTC())
	if service.Status.Indicator != "none" || service.Error == "" {
		t.Fatalf("expected available current status and history warning: %#v", service)
	}
	if len(service.Components[0].History) != 0 || len(service.Groups[0].History) != 0 {
		t.Fatal("failed history endpoint must not produce healthy history")
	}
	if service.UpdatedAt != nil {
		t.Fatal("fetch time must not be presented as provider update time")
	}
}

func TestDeepSeekActiveChangeWinsOverStaleHistory(t *testing.T) {
	now := time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/summary/active"):
			fmt.Fprintf(w, `{"data":{"page":{"page_id":123,"components":[{"component_id":"api","name":"API"}]},"active_changes":[{"change_id":1,"type":"incident","title":"Current outage","status":"investigating","start_at_seconds":%d,"affected_components":[{"component_id":"api","status":"full_outage"}]}]}}`, now.Add(-time.Hour).Unix())
		case strings.HasSuffix(r.URL.Path, "/change/list"):
			fmt.Fprintf(w, `{"data":{"items":[{"change_id":1,"type":"incident","title":"Stale resolved copy","status":"resolved","start_at_seconds":%d,"close_at_seconds":%d}]}}`, now.Add(-time.Hour).Unix(), now.Unix())
		default:
			fmt.Fprint(w, `{"data":{"component_impacts":[],"component_uptimes":[{"component_id":"api","uptime":100}]}}`)
		}
	}))
	defer server.Close()
	service := fetchDeepSeekPublicServiceStatusFrom(context.Background(), server.URL, 123, now)
	if len(service.Incidents) != 1 || service.Incidents[0].Name != "Current outage" || len(service.PastIncidents) != 0 {
		t.Fatalf("expected active source to win, got current=%#v past=%#v", service.Incidents, service.PastIncidents)
	}
}

func TestDeepSeekCurrentMaintenanceBoundaries(t *testing.T) {
	for _, status := range []string{"scheduled", "completed", "cancelled"} {
		t.Run(status, func(t *testing.T) {
			statuses := deepSeekActiveComponentStatuses([]deepSeekChange{{
				Type: "maintenance", Status: status,
				AffectedComponents: []deepSeekAffectedComponent{{ID: "api", Status: "maintenance"}},
			}})
			if current := statuses["api"]; current != "" && current != "operational" {
				t.Fatalf("%s maintenance must not affect current status: %s", status, current)
			}
		})
	}
	statuses := deepSeekActiveComponentStatuses([]deepSeekChange{{
		Type: "maintenance", Status: "ongoing",
		AffectedComponents: []deepSeekAffectedComponent{{ID: "api", Status: "maintenance"}},
	}})
	if statuses["api"] != "under_maintenance" {
		t.Fatalf("expected active maintenance, got %#v", statuses)
	}
}

func TestDeepSeekOfficialLiveStatus(t *testing.T) {
	outputPath := os.Getenv("GATEWAYOPS_STATUS_LIVE_OUTPUT")
	if outputPath == "" {
		t.Skip("set GATEWAYOPS_STATUS_LIVE_OUTPUT to verify public status sources")
	}
	ctx, cancel := context.WithTimeout(context.Background(), statusFetchTimeout)
	defer cancel()
	now := time.Now().UTC()
	service := fetchDeepSeekPublicServiceStatus(ctx, now)
	if service.Error != "" || len(service.Components) == 0 || len(service.Groups) == 0 {
		t.Fatalf("live DeepSeek source unavailable: error=%q components=%d groups=%d", service.Error, len(service.Components), len(service.Groups))
	}
	for _, component := range service.Components {
		if len(component.History) != statusHistoryDays {
			t.Fatalf("unexpected live history length for %s: %d", component.ID, len(component.History))
		}
	}
	view := PublicServiceStatusView{UpdatedAt: now, Days: statusHistoryDays, Services: []PublicServiceStatus{service}}
	data, err := json.Marshal(map[string]any{"data": view})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(outputPath, data, 0600); err != nil {
		t.Fatal(err)
	}
	t.Logf("verified official source: components=%d groups=%d past_incidents=%d", len(service.Components), len(service.Groups), len(service.PastIncidents))
}

func TestDeepSeekUnknownStateAndNewComponents(t *testing.T) {
	if got := deepSeekComponentStatus("new_unknown_status"); got != "unknown" {
		t.Fatalf("unknown provider state mapped to %q", got)
	}
	now := time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)
	var page deepSeekStatusPage
	if err := json.Unmarshal([]byte(fmt.Sprintf(`{"components":[{"component_id":"new-api","name":"New API","available_since_seconds":%d}]}`, now.Add(-24*time.Hour).Unix())), &page); err != nil {
		t.Fatal(err)
	}
	service := mapDeepSeekStatus(page, nil, deepSeekStatusStructure{}, nil, now)
	if service.Components[0].History[0].Status != "unknown" {
		t.Fatal("history before component launch must not be reported as operational")
	}
}

func TestDeepSeekInvalidTimestampsRemainJSONSerializable(t *testing.T) {
	incident := mapDeepSeekIncident(deepSeekChange{
		ID: 1, Type: "incident", Title: "Invalid upstream timestamps", Status: "resolved",
		StartAt: 1792488015000, CloseAt: 1792488015000,
		Updates: []deepSeekChangeUpdate{{At: 1792488015000, Description: "Invalid timestamp"}},
	}, nil)
	if _, err := json.Marshal(incident); err != nil {
		t.Fatalf("upstream milliseconds must not break the merged JSON response: %v", err)
	}
}

func TestDeepSeekSectionHistoryFallbackIsPerSection(t *testing.T) {
	now := time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)
	page := deepSeekStatusPage{
		Components: []deepSeekComponent{{ID: "a", SectionID: "A", Name: "A"}, {ID: "b", SectionID: "B", Name: "B"}},
		Sections:   []deepSeekSection{{ID: "A", Name: "Group A"}, {ID: "B", Name: "Group B"}},
	}
	structure := deepSeekStatusStructure{
		SectionImpacts:   []deepSeekStatusImpact{{SectionID: "A", StartAt: now.Add(-time.Hour).Unix(), EndAt: now.Unix(), Status: "degraded"}},
		ComponentImpacts: []deepSeekStatusImpact{{ID: "b", StartAt: now.Add(-time.Hour).Unix(), EndAt: now.Unix(), Status: "full_outage"}},
	}
	service := mapDeepSeekStatus(page, nil, structure, nil, now)
	if got := service.Groups[1].History[statusHistoryDays-1].Status; got != "full_outage" {
		t.Fatalf("group B must include its own component history, got %q", got)
	}
}

func TestDeepSeekMissingUptimeDoesNotBecomeZero(t *testing.T) {
	now := time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)
	page := deepSeekStatusPage{
		Components: []deepSeekComponent{{ID: "a", SectionID: "group", Name: "A"}, {ID: "b", SectionID: "group", Name: "B"}},
		Sections:   []deepSeekSection{{ID: "group", Name: "Group"}},
	}
	service := mapDeepSeekStatus(page, nil, deepSeekStatusStructure{ComponentUptimes: []deepSeekStatusUptime{{ID: "a", Uptime: 100}}}, nil, now)
	if service.Groups[0].UptimeAvailable == nil || *service.Groups[0].UptimeAvailable {
		t.Fatalf("missing uptime must be explicitly unavailable, got %#v", service.Groups[0])
	}
}
