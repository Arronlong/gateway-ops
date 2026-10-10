package api

import (
	"testing"

	"github.com/guliansheng/gateway-ops/internal/storage"
)

func TestValidateSystemSettingsIframeConfiguration(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		settings storage.UISettings
		wantErr  bool
	}{
		{
			name: "allow all",
			settings: storage.UISettings{
				IframeEmbeddingEnabled: true,
				IframeEmbeddingMode:    storage.IframeEmbeddingModeAll,
			},
		},
		{
			name: "valid allowlist",
			settings: storage.UISettings{
				IframeEmbeddingEnabled:   true,
				IframeEmbeddingMode:      storage.IframeEmbeddingModeAllowlist,
				IframeEmbeddingAllowlist: []string{"example.com", "*.example.com", "**.devai.**", "a*abc.com", "devai_cn.xxx.cn"},
			},
		},
		{
			name: "invalid mode",
			settings: storage.UISettings{
				IframeEmbeddingEnabled: true,
				IframeEmbeddingMode:    "invalid",
			},
			wantErr: true,
		},
		{
			name: "invalid allowlist pattern",
			settings: storage.UISettings{
				IframeEmbeddingEnabled:   true,
				IframeEmbeddingMode:      storage.IframeEmbeddingModeAllowlist,
				IframeEmbeddingAllowlist: []string{"https://example.com"},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := validateSystemSettings(tt.settings)
			if (err != nil) != tt.wantErr {
				t.Fatalf("validateSystemSettings() error = %v, wantErr %t", err, tt.wantErr)
			}
		})
	}
}
