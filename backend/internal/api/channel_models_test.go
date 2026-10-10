package api

import (
	"reflect"
	"testing"

	"github.com/guliansheng/gateway-ops/internal/storage"
)

func TestBuildChannelModelCatalogMergesLinkedAccounts(t *testing.T) {
	links := []storage.ChannelRelayAssociation{
		{ChannelID: 2, RelayStationID: 1, RelayAccountExternalID: 20, Linked: true},
		{ChannelID: 1, RelayStationID: 1, RelayAccountExternalID: 10, Linked: true},
		{ChannelID: 1, RelayStationID: 2, RelayAccountExternalID: 30, Linked: true},
		{ChannelID: 3, RelayStationID: 1, RelayAccountExternalID: 40, Linked: false},
	}
	models := map[relayAccountModelKey][]string{
		{StationID: 1, AccountID: 10}: {"gpt-5.6-sol", "claude-4"},
		{StationID: 2, AccountID: 30}: {"gpt-5.6-sol", "gemini-3-pro"},
		{StationID: 1, AccountID: 20}: {"gpt-5.6-sol"},
	}

	got := buildChannelModelCatalog(links, models)
	want := []channelModelCatalogItem{
		{ChannelID: 1, Models: []string{"claude-4", "gemini-3-pro", "gpt-5.6-sol"}},
		{ChannelID: 2, Models: []string{"gpt-5.6-sol"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("catalog = %#v, want %#v", got, want)
	}
}
