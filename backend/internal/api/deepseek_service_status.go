package api

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	deepSeekStatusPageURL = "https://status.deepseek.com/"
	deepSeekStatusPageID  = int64(6410630422455)
)

type deepSeekStatusResponse struct {
	Data struct {
		Page          deepSeekStatusPage `json:"page"`
		ActiveChanges []deepSeekChange   `json:"active_changes"`
	} `json:"data"`
}

type deepSeekStatusPage struct {
	PageID     int64               `json:"page_id"`
	Name       string              `json:"name"`
	Components []deepSeekComponent `json:"components"`
	Sections   []deepSeekSection   `json:"sections"`
}

type deepSeekComponent struct {
	ID             string `json:"component_id"`
	SectionID      string `json:"section_id"`
	Name           string `json:"name"`
	Description    string `json:"description"`
	AvailableSince int64  `json:"available_since_seconds"`
	Order          int    `json:"order_id"`
}

type deepSeekSection struct {
	ID          string `json:"section_id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Order       int    `json:"order_id"`
}

type deepSeekStatusStructureResponse struct {
	Data deepSeekStatusStructure `json:"data"`
}

type deepSeekStatusStructure struct {
	SectionImpacts   []deepSeekStatusImpact `json:"section_impacts"`
	SectionUptimes   []deepSeekStatusUptime `json:"section_uptimes"`
	ComponentImpacts []deepSeekStatusImpact `json:"component_impacts"`
	ComponentUptimes []deepSeekStatusUptime `json:"component_uptimes"`
}

type deepSeekStatusImpact struct {
	ID        string `json:"component_id"`
	SectionID string `json:"section_id"`
	ChangeID  int64  `json:"change_id"`
	StartAt   int64  `json:"start_at_seconds"`
	EndAt     int64  `json:"end_at_seconds"`
	Status    string `json:"status"`
}

type deepSeekStatusUptime struct {
	ID        string  `json:"component_id"`
	SectionID string  `json:"section_id"`
	Uptime    float64 `json:"uptime"`
}

type deepSeekChangeListResponse struct {
	Data struct {
		Items []deepSeekChange `json:"items"`
	} `json:"data"`
}

type deepSeekChange struct {
	ID                 int64                       `json:"change_id"`
	Type               string                      `json:"type"`
	Title              string                      `json:"title"`
	Description        string                      `json:"description"`
	Status             string                      `json:"status"`
	AffectedComponents []deepSeekAffectedComponent `json:"affected_components"`
	StartAt            int64                       `json:"start_at_seconds"`
	CloseAt            int64                       `json:"close_at_seconds"`
	Updates            []deepSeekChangeUpdate      `json:"updates"`
}

type deepSeekAffectedComponent struct {
	ID        string `json:"component_id"`
	Name      string `json:"name"`
	Status    string `json:"status"`
	OldStatus string `json:"old_status"`
}

type deepSeekChangeUpdate struct {
	ID               string                    `json:"update_id"`
	At               int64                     `json:"at_seconds"`
	Status           string                    `json:"status"`
	Description      string                    `json:"description"`
	ComponentChanges []deepSeekComponentChange `json:"component_changes"`
}

type deepSeekComponentChange struct {
	ID        string `json:"component_id"`
	Name      string `json:"component_name"`
	Status    string `json:"status"`
	OldStatus string `json:"old_status"`
}

func fetchStatusTarget(ctx context.Context, target statusTarget, now time.Time) PublicServiceStatus {
	if target.id == "deepseek" {
		return fetchDeepSeekPublicServiceStatus(ctx, now)
	}
	return fetchPublicServiceStatus(ctx, target, now)
}

func fetchDeepSeekPublicServiceStatus(ctx context.Context, now time.Time) PublicServiceStatus {
	return fetchDeepSeekPublicServiceStatusFrom(ctx, strings.TrimRight(deepSeekStatusPageURL, "/"), deepSeekStatusPageID, now)
}

func fetchDeepSeekPublicServiceStatusFrom(ctx context.Context, baseURL string, pageID int64, now time.Time) PublicServiceStatus {
	service := PublicServiceStatus{
		ID: "deepseek", Name: "DeepSeek", URL: deepSeekStatusPageURL,
		HistoryURL: deepSeekStatusPageURL + "history", Components: []PublicServiceStatusComponent{},
		Groups: []PublicServiceStatusGroup{}, Incidents: []PublicServiceStatusIncident{},
		PastIncidents: []PublicServiceStatusIncident{}, ScheduledMaintenances: []PublicServiceStatusIncident{},
		Status: PublicServiceStatusSummary{Indicator: "unknown", Description: "状态未知"},
	}
	apiURL := fmt.Sprintf("%s/api/status-page/%d", strings.TrimRight(baseURL, "/"), pageID)
	var active deepSeekStatusResponse
	if err := fetchStatusPage(ctx, apiURL+"/summary/active", &active); err != nil || active.Data.Page.PageID != pageID || len(active.Data.Page.Components) == 0 || active.Data.ActiveChanges == nil {
		service.Error = "官方状态页暂时无法访问"
		return service
	}

	from := now.UTC().Truncate(24*time.Hour).AddDate(0, 0, 1-statusHistoryDays).Unix()
	to := now.Unix()
	structureURL := fmt.Sprintf("%s/summary/structure?start_at_from_seconds=%d&start_at_to_seconds=%d", apiURL, from, to)
	changesURL := fmt.Sprintf("%s/change/list?start_at_seconds=%d&end_at_seconds=%d", apiURL, now.AddDate(0, -2, 0).Unix(), to)
	var structure deepSeekStatusStructureResponse
	var changes deepSeekChangeListResponse
	var structureErr, changesErr error
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		structureErr = fetchStatusPage(ctx, structureURL, &structure)
	}()
	go func() {
		defer wg.Done()
		changesErr = fetchStatusPage(ctx, changesURL, &changes)
	}()
	wg.Wait()
	if structureErr == nil && (structure.Data.ComponentImpacts == nil || structure.Data.ComponentUptimes == nil) {
		structureErr = errors.New("missing component history data")
	}
	if changesErr == nil && changes.Data.Items == nil {
		changesErr = errors.New("missing incident history data")
	}

	allChanges := append([]deepSeekChange(nil), changes.Data.Items...)
	seenChanges := make(map[int64]int, len(allChanges))
	for index, change := range allChanges {
		seenChanges[change.ID] = index
	}
	for _, change := range active.Data.ActiveChanges {
		if index, exists := seenChanges[change.ID]; exists {
			allChanges[index] = change
		} else {
			seenChanges[change.ID] = len(allChanges)
			allChanges = append(allChanges, change)
		}
	}
	service = mapDeepSeekStatus(active.Data.Page, active.Data.ActiveChanges, structure.Data, allChanges, now)
	if structureErr != nil {
		for index := range service.Components {
			service.Components[index].History = []PublicServiceStatusDay{}
		}
		for index := range service.Groups {
			service.Groups[index].History = []PublicServiceStatusDay{}
			for componentIndex := range service.Groups[index].Components {
				service.Groups[index].Components[componentIndex].History = []PublicServiceStatusDay{}
			}
		}
	}
	if structureErr != nil || changesErr != nil {
		service.Error = "历史数据暂时不完整"
	}
	return service
}

func mapDeepSeekStatus(page deepSeekStatusPage, activeChanges []deepSeekChange, structure deepSeekStatusStructure, changes []deepSeekChange, now time.Time) PublicServiceStatus {
	now = now.UTC()
	service := PublicServiceStatus{
		ID: "deepseek", Name: "DeepSeek", URL: deepSeekStatusPageURL, HistoryURL: deepSeekStatusPageURL + "history",
		Components: make([]PublicServiceStatusComponent, 0, len(page.Components)),
		Groups:     []PublicServiceStatusGroup{}, Incidents: []PublicServiceStatusIncident{},
		PastIncidents: []PublicServiceStatusIncident{}, ScheduledMaintenances: []PublicServiceStatusIncident{},
	}
	currentStatuses := deepSeekActiveComponentStatuses(activeChanges)
	componentByID := make(map[string]PublicServiceStatusComponent, len(page.Components))
	for _, source := range page.Components {
		if source.ID == "" {
			continue
		}
		component := PublicServiceStatusComponent{
			ID: source.ID, Name: source.Name, Description: source.Description,
			Status:  currentStatuses[source.ID],
			History: buildDeepSeekHistory(source.ID, structure.ComponentImpacts, changes, now),
		}
		if component.Status == "" {
			component.Status = "operational"
		}
		if component.Name == "" {
			component.Name = source.ID
		}
		if source.AvailableSince > 0 {
			for index := range component.History {
				day, _ := time.Parse("2006-01-02", component.History[index].Date)
				if day.Add(24*time.Hour).Unix() <= source.AvailableSince {
					component.History[index].Status = "unknown"
				}
			}
		}
		service.Components = append(service.Components, component)
		componentByID[source.ID] = component
	}
	if len(service.Components) == 0 {
		service.Status = PublicServiceStatusSummary{Indicator: "unknown", Description: "状态未知"}
		service.Error = "官方状态页未返回有效服务组件"
		return service
	}
	service.Status = deepSeekOverallStatus(service.Components)
	service.Groups = buildDeepSeekGroups(page, service.Components, structure, changes, now)
	service.Incidents, service.PastIncidents, service.ScheduledMaintenances = buildDeepSeekIncidents(changes, componentByID, now)
	return service
}

func deepSeekActiveComponentStatuses(changes []deepSeekChange) map[string]string {
	result := make(map[string]string)
	for _, change := range changes {
		switch deepSeekIncidentStatus(change.Status) {
		case "resolved", "completed", "cancelled", "scheduled":
			continue
		}
		current := make(map[string]string)
		for _, component := range change.AffectedComponents {
			if component.ID != "" {
				current[component.ID] = deepSeekComponentStatus(component.Status)
			}
		}
		updates := append([]deepSeekChangeUpdate(nil), change.Updates...)
		sort.SliceStable(updates, func(i, j int) bool { return updates[i].At < updates[j].At })
		for _, update := range updates {
			for _, component := range update.ComponentChanges {
				if component.ID != "" {
					current[component.ID] = deepSeekComponentStatus(component.Status)
				}
			}
		}
		for id, status := range current {
			if previous := result[id]; previous != "" {
				result[id] = deepSeekWorseStatus(previous, status)
			} else {
				result[id] = status
			}
		}
	}
	return result
}

func deepSeekComponentStatus(status string) string {
	switch strings.ToLower(status) {
	case "operational":
		return "operational"
	case "degraded", "degraded_performance":
		return "degraded_performance"
	case "partial_outage":
		return "partial_outage"
	case "major_outage":
		return "major_outage"
	case "full_outage":
		return "full_outage"
	case "maintenance", "under_maintenance":
		return "under_maintenance"
	default:
		return "unknown"
	}
}

func deepSeekWorseStatus(left, right string) string {
	rank := map[string]int{
		"operational": 0, "under_maintenance": 1, "degraded_performance": 2,
		"partial_outage": 3, "major_outage": 4, "full_outage": 5, "unknown": 6,
	}
	if rank[right] > rank[left] {
		return right
	}
	return left
}

func deepSeekOverallStatus(components []PublicServiceStatusComponent) PublicServiceStatusSummary {
	worst := "operational"
	for _, component := range components {
		worst = deepSeekWorseStatus(worst, component.Status)
	}
	switch worst {
	case "under_maintenance":
		return PublicServiceStatusSummary{Indicator: "minor", Description: "部分服务维护中"}
	case "degraded_performance":
		return PublicServiceStatusSummary{Indicator: "minor", Description: "部分服务性能下降"}
	case "partial_outage":
		return PublicServiceStatusSummary{Indicator: "major", Description: "部分服务中断"}
	case "major_outage", "full_outage":
		return PublicServiceStatusSummary{Indicator: "critical", Description: "服务严重中断"}
	case "unknown":
		return PublicServiceStatusSummary{Indicator: "unknown", Description: "状态未知"}
	default:
		return PublicServiceStatusSummary{Indicator: "none", Description: "所有服务正常"}
	}
}

func buildDeepSeekHistory(id string, impacts []deepSeekStatusImpact, changes []deepSeekChange, now time.Time) []PublicServiceStatusDay {
	history := make([]PublicServiceStatusDay, statusHistoryDays)
	for index := range history {
		day := now.UTC().AddDate(0, 0, index-(statusHistoryDays-1))
		history[index] = PublicServiceStatusDay{Date: day.Format("2006-01-02"), Status: "operational", Incidents: []PublicServiceStatusBlip{}}
	}
	changeByID := make(map[int64]deepSeekChange, len(changes))
	for _, change := range changes {
		changeByID[change.ID] = change
	}
	for _, impact := range impacts {
		impactID := impact.ID
		if impactID == "" {
			impactID = impact.SectionID
		}
		if impactID != id || impact.StartAt <= 0 {
			continue
		}
		start := time.Unix(impact.StartAt, 0).UTC()
		end := now.UTC()
		if impact.EndAt > 0 {
			end = time.Unix(impact.EndAt, 0).UTC()
		}
		status := deepSeekComponentStatus(impact.Status)
		for index := range history {
			day, _ := time.Parse("2006-01-02", history[index].Date)
			if !start.Before(day.Add(24*time.Hour)) || !end.After(day) {
				continue
			}
			history[index].Status = deepSeekWorseStatus(history[index].Status, status)
			if change, ok := changeByID[impact.ChangeID]; ok {
				blip := PublicServiceStatusBlip{
					ID: strconv.FormatInt(change.ID, 10), Name: change.Title,
					Status: deepSeekIncidentStatus(change.Status), Impact: deepSeekIncidentImpact(change),
				}
				if !deepSeekHasBlip(history[index].Incidents, blip.ID) {
					history[index].Incidents = append(history[index].Incidents, blip)
				}
			}
		}
	}
	return history
}

func deepSeekHasBlip(blips []PublicServiceStatusBlip, id string) bool {
	for _, blip := range blips {
		if blip.ID == id {
			return true
		}
	}
	return false
}

func buildDeepSeekGroups(page deepSeekStatusPage, components []PublicServiceStatusComponent, structure deepSeekStatusStructure, changes []deepSeekChange, now time.Time) []PublicServiceStatusGroup {
	groupOrder := make(map[string]int)
	for _, component := range page.Components {
		groupOrder[component.ID] = component.Order
	}
	for _, section := range page.Sections {
		groupOrder[section.ID] = section.Order
	}
	componentByID := make(map[string]PublicServiceStatusComponent, len(components))
	for _, component := range components {
		componentByID[component.ID] = component
	}
	sectionByID := make(map[string]deepSeekSection, len(page.Sections))
	for _, section := range page.Sections {
		sectionByID[section.ID] = section
	}
	sectionUptimes := make(map[string]float64, len(structure.SectionUptimes))
	for _, uptime := range structure.SectionUptimes {
		id := uptime.ID
		if id == "" {
			id = uptime.SectionID
		}
		sectionUptimes[id] = uptime.Uptime
	}
	componentUptimes := make(map[string]float64, len(structure.ComponentUptimes))
	for _, uptime := range structure.ComponentUptimes {
		componentUptimes[uptime.ID] = uptime.Uptime
	}
	groups := make([]PublicServiceStatusGroup, 0, len(components))
	addedSections := make(map[string]bool)
	for _, source := range page.Components {
		component, exists := componentByID[source.ID]
		if !exists {
			continue
		}
		section, grouped := sectionByID[source.SectionID]
		if !grouped {
			uptime, hasUptime := componentUptimes[component.ID]
			groups = append(groups, PublicServiceStatusGroup{
				ID: component.ID, Name: component.Name, Description: component.Description,
				Status: component.Status, Uptime: uptime, UptimeAvailable: &hasUptime,
				History: component.History, Components: []PublicServiceStatusComponent{component},
			})
			continue
		}
		if addedSections[section.ID] {
			continue
		}
		addedSections[section.ID] = true
		members := make([]PublicServiceStatusComponent, 0)
		for _, member := range page.Components {
			if member.SectionID == section.ID {
				if component, ok := componentByID[member.ID]; ok {
					members = append(members, component)
				}
			}
		}
		groupStatus := "operational"
		memberHistories := make([][]PublicServiceStatusDay, 0, len(members))
		for _, member := range members {
			groupStatus = deepSeekWorseStatus(groupStatus, member.Status)
			memberHistories = append(memberHistories, member.History)
		}
		history := buildDeepSeekHistory(section.ID, structure.SectionImpacts, changes, now)
		hasSectionImpacts := false
		for _, impact := range structure.SectionImpacts {
			if impact.SectionID == section.ID {
				hasSectionImpacts = true
				break
			}
		}
		if !hasSectionImpacts {
			history = mergeDeepSeekHistories(memberHistories)
		}
		uptime, hasUptime := sectionUptimes[section.ID]
		groups = append(groups, PublicServiceStatusGroup{
			ID: section.ID, Name: section.Name, Description: section.Description,
			Status: groupStatus, Uptime: uptime, UptimeAvailable: &hasUptime, History: history, Components: members,
		})
	}
	sort.SliceStable(groups, func(i, j int) bool { return groupOrder[groups[i].ID] < groupOrder[groups[j].ID] })
	return groups
}

func mergeDeepSeekHistories(histories [][]PublicServiceStatusDay) []PublicServiceStatusDay {
	if len(histories) == 0 {
		return []PublicServiceStatusDay{}
	}
	merged := make([]PublicServiceStatusDay, len(histories[0]))
	for index, day := range histories[0] {
		merged[index] = PublicServiceStatusDay{Date: day.Date, Status: day.Status, Incidents: append([]PublicServiceStatusBlip(nil), day.Incidents...)}
	}
	for _, history := range histories[1:] {
		for index := 0; index < len(merged) && index < len(history); index++ {
			merged[index].Status = deepSeekWorseStatus(merged[index].Status, history[index].Status)
			for _, blip := range history[index].Incidents {
				if !deepSeekHasBlip(merged[index].Incidents, blip.ID) {
					merged[index].Incidents = append(merged[index].Incidents, blip)
				}
			}
		}
	}
	return merged
}

func buildDeepSeekIncidents(changes []deepSeekChange, components map[string]PublicServiceStatusComponent, now time.Time) ([]PublicServiceStatusIncident, []PublicServiceStatusIncident, []PublicServiceStatusIncident) {
	current, past, maintenance := []PublicServiceStatusIncident{}, []PublicServiceStatusIncident{}, []PublicServiceStatusIncident{}
	cutoff := now.AddDate(0, -2, 0)
	for _, change := range changes {
		incident := mapDeepSeekIncident(change, components)
		if incident.Status == "cancelled" {
			continue
		}
		if strings.EqualFold(change.Type, "maintenance") {
			if incident.Status == "resolved" || incident.Status == "completed" {
				if incident.CreatedAt != nil && !incident.CreatedAt.Before(cutoff) {
					past = append(past, incident)
				}
			} else {
				maintenance = append(maintenance, incident)
			}
			continue
		}
		if incident.Status == "resolved" || incident.Status == "completed" {
			if incident.CreatedAt != nil && !incident.CreatedAt.Before(cutoff) {
				past = append(past, incident)
			}
			continue
		}
		current = append(current, incident)
	}
	sort.SliceStable(past, func(i, j int) bool { return incidentStart(past[i]).After(incidentStart(past[j])) })
	return current, past, maintenance
}

func mapDeepSeekIncident(change deepSeekChange, components map[string]PublicServiceStatusComponent) PublicServiceStatusIncident {
	incident := PublicServiceStatusIncident{
		ID: strconv.FormatInt(change.ID, 10), Name: change.Title,
		Status: deepSeekIncidentStatus(change.Status), Impact: deepSeekIncidentImpact(change),
		CreatedAt: deepSeekUnixTime(change.StartAt), ResolvedAt: deepSeekUnixTime(change.CloseAt),
		Components: []PublicServiceIncidentComponent{}, IncidentUpdates: []PublicServiceStatusIncidentUpdate{},
	}
	componentNames := make(map[string]string)
	for _, component := range change.AffectedComponents {
		name := component.Name
		if name == "" {
			name = components[component.ID].Name
		}
		if component.ID != "" && name != "" {
			componentNames[component.ID] = name
		}
	}
	for _, update := range change.Updates {
		at := deepSeekUnixTime(update.At)
		if at != nil && (incident.UpdatedAt == nil || at.After(*incident.UpdatedAt)) {
			incident.UpdatedAt = at
		}
		mapped := PublicServiceStatusIncidentUpdate{
			Body: update.Description, Status: deepSeekIncidentStatus(update.Status),
			CreatedAt: at, UpdatedAt: at, DisplayAt: at,
			AffectedComponents: []PublicServiceStatusAffectedComponent{},
		}
		for _, component := range update.ComponentChanges {
			name := component.Name
			if name == "" {
				name = components[component.ID].Name
			}
			if component.ID != "" && name != "" {
				componentNames[component.ID] = name
				mapped.AffectedComponents = append(mapped.AffectedComponents, PublicServiceStatusAffectedComponent{
					Code: component.ID, Name: name, OldStatus: deepSeekComponentStatus(component.OldStatus),
					NewStatus: deepSeekComponentStatus(component.Status),
				})
			}
		}
		incident.IncidentUpdates = append(incident.IncidentUpdates, mapped)
	}
	for id, name := range componentNames {
		incident.Components = append(incident.Components, PublicServiceIncidentComponent{ID: id, Name: name})
	}
	sort.Slice(incident.Components, func(i, j int) bool { return incident.Components[i].Name < incident.Components[j].Name })
	if incident.ResolvedAt == nil && (incident.Status == "resolved" || incident.Status == "completed") {
		incident.ResolvedAt = incident.UpdatedAt
	}
	return incident
}

func deepSeekUnixTime(seconds int64) *time.Time {
	if seconds <= 0 || seconds > 253402300799 {
		return nil
	}
	value := time.Unix(seconds, 0).UTC()
	return &value
}

func deepSeekIncidentStatus(status string) string {
	switch strings.ToLower(status) {
	case "ongoing":
		return "in_progress"
	default:
		return strings.ToLower(status)
	}
}

func deepSeekIncidentImpact(change deepSeekChange) string {
	impact := "none"
	rank := map[string]int{"none": 0, "minor": 1, "major": 2, "critical": 3}
	applyStatus := func(status string) {
		candidate := "none"
		switch deepSeekComponentStatus(status) {
		case "degraded_performance", "under_maintenance":
			candidate = "minor"
		case "partial_outage", "major_outage":
			candidate = "major"
		case "full_outage":
			candidate = "critical"
		}
		if rank[candidate] > rank[impact] {
			impact = candidate
		}
	}
	for _, component := range change.AffectedComponents {
		applyStatus(component.Status)
	}
	for _, update := range change.Updates {
		for _, component := range update.ComponentChanges {
			applyStatus(component.Status)
		}
	}
	return impact
}
