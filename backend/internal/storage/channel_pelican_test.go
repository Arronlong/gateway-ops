package storage

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestChannelPelicanGroupsMergeDeduplicateAndLimitThirty(t *testing.T) {
	channels := []ChannelPelicanInfo{{ID: 5, Name: "Channel A"}, {ID: 6, Name: "Channel B"}, {ID: 7, Name: "Unlinked"}}
	links := []ChannelRelayAssociation{
		{ChannelID: 5, RelayStationID: 1, RelayAccountExternalID: 42, Linked: true},
		{ChannelID: 5, RelayStationID: 1, RelayAccountExternalID: 42, Linked: true},
		{ChannelID: 5, RelayStationID: 1, RelayAccountExternalID: 43, Linked: true},
		{ChannelID: 6, RelayStationID: 2, RelayAccountExternalID: 42, Linked: true},
		{ChannelID: 7, RelayStationID: 1, RelayAccountExternalID: 42, Linked: false},
	}
	results := []PelicanLibraryItem{}
	for index := 1; index <= 35; index++ {
		results = append(results, PelicanLibraryItem{PelicanResult: PelicanResult{ID: uint(index), RelayStationID: 1, AccountExternalID: int64(42 + index%2), FinishedAt: time.Unix(int64(index), 0)}})
	}
	results = append(results, PelicanLibraryItem{PelicanResult: PelicanResult{ID: 100, RelayStationID: 2, AccountExternalID: 42}})
	groups := buildChannelPelicanGroups(channels, links, []RelayStation{{ID: 1}, {ID: 2}}, []PelicanConfig{{RelayStationID: 1, AccountExternalID: 42, Enabled: true}}, results)
	if len(groups) != 3 || len(groups[0].Targets) != 2 || len(groups[0].Results) != 30 {
		t.Fatalf("unexpected groups: %#v", groups)
	}
	if groups[0].Results[0].ID != 35 || groups[0].Results[29].ID != 6 || !groups[0].Targets[0].Config.Enabled {
		t.Fatalf("incorrect order or config: %#v", groups[0])
	}
	if len(groups[1].Results) != 1 || groups[1].Results[0].ID != 100 || len(groups[2].Targets) != 0 || len(groups[2].Results) != 0 {
		t.Fatal("results leaked between station/account pairs or unlinked channels")
	}
}

func TestChannelPelicanGroupsHonorConfiguredHistoryCount(t *testing.T) {
	results := make([]PelicanLibraryItem, 6)
	for index := range results {
		results[index] = PelicanLibraryItem{PelicanResult: PelicanResult{ID: uint(index + 1), RelayStationID: 1, AccountExternalID: 42, FinishedAt: time.Unix(int64(index+1), 0)}}
	}
	groups := buildChannelPelicanGroups([]ChannelPelicanInfo{{ID: 5}}, []ChannelRelayAssociation{{ChannelID: 5, RelayStationID: 1, RelayAccountExternalID: 42, Linked: true}}, nil, nil, results, 2)
	if len(groups) != 1 || len(groups[0].Results) != 2 || groups[0].Results[0].ID != 6 {
		t.Fatalf("configured history count was ignored: %#v", groups)
	}
}

func TestChannelPelicanGroupsShareHistoryWithoutLeakingCredentials(t *testing.T) {
	channels := []ChannelPelicanInfo{{ID: 5, Name: "A"}, {ID: 6, Name: "B"}}
	links := []ChannelRelayAssociation{{ChannelID: 5, RelayStationID: 1, RelayAccountExternalID: 42, Linked: true}, {ChannelID: 6, RelayStationID: 1, RelayAccountExternalID: 42, Linked: true}}
	groups := buildChannelPelicanGroups(channels, links, []RelayStation{{ID: 1}}, nil, []PelicanLibraryItem{{PelicanResult: PelicanResult{ID: 1, RelayStationID: 1, AccountExternalID: 42}}})
	if len(groups[0].Results) != 1 || len(groups[1].Results) != 1 {
		t.Fatal("shared upstream history missing")
	}
	encoded, err := json.Marshal(groups)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{`"html"`, `"prompt"`, `"password"`, `"api_key"`, `"credentials"`} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("sensitive field in response: %s", forbidden)
		}
	}
}

func TestChannelPelicanGroupsBreakTimestampTiesByResultID(t *testing.T) {
	finished := time.Now()
	groups := buildChannelPelicanGroups([]ChannelPelicanInfo{{ID: 1}}, []ChannelRelayAssociation{{ChannelID: 1, RelayStationID: 2, RelayAccountExternalID: 42, Linked: true}}, nil, nil, []PelicanLibraryItem{
		{PelicanResult: PelicanResult{ID: 1, RelayStationID: 2, AccountExternalID: 42, FinishedAt: finished}},
		{PelicanResult: PelicanResult{ID: 2, RelayStationID: 2, AccountExternalID: 42, FinishedAt: finished}},
	})
	if groups[0].Results[0].ID != 2 {
		t.Fatal("equal timestamp order is unstable")
	}
}

func TestChannelPelicanGroupsIncludeDirectResultsWithoutRelayAssociation(t *testing.T) {
	result := PelicanLibraryItem{PelicanResult: PelicanResult{ID: 9, ChannelID: 5, AccountName: "Channel A", Status: "success"}}
	groups := buildChannelPelicanGroups(
		[]ChannelPelicanInfo{{ID: 5, Name: "Channel A", PelicanKeyConfigured: true}},
		nil,
		nil,
		nil,
		[]PelicanLibraryItem{result},
	)
	if len(groups) != 1 || len(groups[0].Targets) != 0 || len(groups[0].Results) != 1 || groups[0].Results[0].ID != 9 {
		t.Fatalf("direct channel result should not require a relay target: %#v", groups)
	}
}

func TestChannelPelicanGroupsSerializeEmptyResultsAsArrays(t *testing.T) {
	channels := []ChannelPelicanInfo{{ID: 5}, {ID: 6, PelicanKeyConfigured: true}}
	links := []ChannelRelayAssociation{{ChannelID: 5, RelayStationID: 1, RelayAccountExternalID: 42, Linked: true}}
	groups := buildChannelPelicanGroups(channels, links, []RelayStation{{ID: 1}}, nil, nil)
	encoded, err := json.Marshal(groups)
	if err != nil {
		t.Fatal(err)
	}
	var payload []struct {
		Results json.RawMessage `json:"results"`
	}
	if err := json.Unmarshal(encoded, &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload) != len(channels) {
		t.Fatalf("group count = %d, want %d", len(payload), len(channels))
	}
	for i, group := range payload {
		if string(group.Results) != "[]" {
			t.Fatalf("channel %d results = %s, want []", channels[i].ID, group.Results)
		}
	}
}
