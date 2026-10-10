package storage

import "testing"

func TestSelectAutomaticRelayAccountUsesChannelAccountIdentifier(t *testing.T) {
	channel := Channel{
		Name:    "板栗API",
		Type:    ChannelTypeSub2API,
		SiteURL: "https://banliapi.top",
	}
	channelAccount := ChannelAccount{Username: "板栗API"}
	relayAccounts := []RelayAccount{
		{
			ExternalID: 41678,
			Name:       "【板栗API-1】https://banliapi.top",
			BaseURL:    "https://banliapi.top",
		},
		{
			ExternalID: 41725,
			Name:       "【板栗API】sub2api-https://banliapi.top",
			BaseURL:    "https://banliapi.top",
		},
	}

	got, ok := selectAutomaticRelayAccount(channel, channelAccount, relayAccounts)
	if !ok {
		t.Fatal("expected a unique automatic relay account match")
	}
	if got.ExternalID != 41725 {
		t.Fatalf("expected sub2api relay account 41725, got %d", got.ExternalID)
	}
}

func TestSelectAutomaticRelayAccountUsesChannelNameWhenIdentifierIsNotRelayIdentity(t *testing.T) {
	channel := Channel{
		Name:    "板栗API",
		Type:    ChannelTypeSub2API,
		SiteURL: "https://banliapi.top",
	}
	channelAccount := ChannelAccount{Username: "arianaonroy@mr.email"}
	relayAccounts := []RelayAccount{
		{
			ExternalID: 41678,
			Name:       "【板栗API-1】https://banliapi.top",
			BaseURL:    "https://banliapi.top",
		},
		{
			ExternalID: 41725,
			Name:       "【板栗API】sub2api-https://banliapi.top",
			BaseURL:    "https://banliapi.top",
		},
	}

	got, ok := selectAutomaticRelayAccount(channel, channelAccount, relayAccounts)
	if !ok {
		t.Fatal("expected the channel name and type to disambiguate the relay account")
	}
	if got.ExternalID != 41725 {
		t.Fatalf("expected sub2api relay account 41725, got %d", got.ExternalID)
	}
}

func TestSelectAutomaticRelayAccountDoesNotUseAmbiguousURLOnly(t *testing.T) {
	channel := Channel{
		Name:    "未命名渠道",
		Type:    ChannelTypeSub2API,
		SiteURL: "https://same.example.com",
	}
	channelAccount := ChannelAccount{Username: "unrelated@example.com"}
	relayAccounts := []RelayAccount{
		{ExternalID: 1, Name: "上游一", BaseURL: "https://same.example.com"},
		{ExternalID: 2, Name: "上游二", BaseURL: "https://same.example.com"},
	}

	if _, ok := selectAutomaticRelayAccount(channel, channelAccount, relayAccounts); ok {
		t.Fatal("expected ambiguous URL-only candidates to remain unlinked")
	}
}
