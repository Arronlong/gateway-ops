package storage

import (
	"testing"
	"time"
)

func TestGroupChannelUsageModelsKeepsChannelsSeparateAndOrdered(t *testing.T) {
	rows := []channelUsageModelRow{
		{ChannelID: 1, Model: "claude-3-7-sonnet"},
		{ChannelID: 1, Model: "gpt-5.6-sol"},
		{ChannelID: 2, Model: "gemini-2.5-pro"},
	}

	got := groupChannelUsageModels(rows)
	if len(got[1]) != 2 || got[1][0] != "claude-3-7-sonnet" || got[1][1] != "gpt-5.6-sol" {
		t.Fatalf("channel 1 models = %#v", got[1])
	}
	if len(got[2]) != 1 || got[2][0] != "gemini-2.5-pro" {
		t.Fatalf("channel 2 models = %#v", got[2])
	}
}

func TestChannelUsageSamplesExposeChannelAccountID(t *testing.T) {
	events := []ChannelUsageEvent{{ChannelAccountID: 42, OccurredAt: time.Unix(1, 0)}}
	samples := channelUsageSamples(events)
	if len(samples) != 1 || samples[0].ChannelAccountID != 42 {
		t.Fatalf("samples = %#v", samples)
	}
}
