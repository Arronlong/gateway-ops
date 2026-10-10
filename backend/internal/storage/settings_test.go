package storage

import "testing"

func TestUISettingsOperationsManagementDefaultsEnabled(t *testing.T) {
	t.Parallel()

	got := defaultUISettings()
	if !got.OperationsManagementEnabled {
		t.Fatal("operations management must be enabled by default")
	}
}

func TestUISettingsValuesSerializesOperationsManagement(t *testing.T) {
	t.Parallel()

	settings := defaultUISettings()
	settings.OperationsManagementEnabled = false

	values, err := uiSettingsValues(settings)
	if err != nil {
		t.Fatal(err)
	}
	if got := values[SettingUIOperationsManagementEnabled]; got != "false" {
		t.Fatalf("operations management value = %q, want false", got)
	}
}

func TestUISettingsIframeDefaults(t *testing.T) {
	t.Parallel()

	got := defaultUISettings()
	if got.IframeEmbeddingEnabled {
		t.Fatal("iframe embedding must be disabled by default")
	}
	if got.IframeEmbeddingMode != IframeEmbeddingModeAllowlist {
		t.Fatalf("iframe embedding mode = %q, want %q", got.IframeEmbeddingMode, IframeEmbeddingModeAllowlist)
	}
	if len(got.IframeEmbeddingAllowlist) != 0 {
		t.Fatalf("iframe embedding allowlist = %#v, want empty", got.IframeEmbeddingAllowlist)
	}
}

func TestUISettingsValuesSerializesIframeAllowlist(t *testing.T) {
	t.Parallel()

	settings := defaultUISettings()
	settings.IframeEmbeddingEnabled = true
	settings.IframeEmbeddingMode = IframeEmbeddingModeAllowlist
	settings.IframeEmbeddingAllowlist = []string{"*.example.com", "a*abc.com"}

	values, err := uiSettingsValues(settings)
	if err != nil {
		t.Fatal(err)
	}
	if got := values[SettingSecurityIframeEnabled]; got != "true" {
		t.Fatalf("enabled value = %q, want true", got)
	}
	if got := values[SettingSecurityIframeMode]; got != IframeEmbeddingModeAllowlist {
		t.Fatalf("mode value = %q, want %q", got, IframeEmbeddingModeAllowlist)
	}
	if got := values[SettingSecurityIframeAllowlist]; got != `["*.example.com","a*abc.com"]` {
		t.Fatalf("allowlist value = %q", got)
	}
}

func TestDecodeStringSliceSetting(t *testing.T) {
	t.Parallel()

	got, err := decodeStringSliceSetting(`["*.example.com","a*abc.com"]`)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != "*.example.com" || got[1] != "a*abc.com" {
		t.Fatalf("decoded allowlist = %#v", got)
	}
}
