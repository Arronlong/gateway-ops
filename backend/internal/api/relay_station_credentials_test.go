package api

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/guliansheng/gateway-ops/internal/crypto"
	"github.com/guliansheng/gateway-ops/internal/relay"
	"github.com/guliansheng/gateway-ops/internal/storage"
)

func TestRelayStationSummaryKeepsNewAPICredentialsPrivate(t *testing.T) {
	cipher, err := crypto.NewCipher("test-secret")
	if err != nil {
		t.Fatal(err)
	}
	raw := `gatewayops:newapi:{"auth_mode":"access_token","token":"management-secret","user_id":"7","gateway_key":"gateway-secret"}`
	encrypted, err := cipher.Encrypt(raw)
	if err != nil {
		t.Fatal(err)
	}
	station := storage.RelayStation{ID: 1, Type: "newapi", Name: "New API", APIKeyCipher: encrypted}
	summary := stationSummaryWithCredentials(&Deps{Relay: relay.NewService(nil, cipher)}, &station)
	if summary.Type != "newapi" || summary.AuthMode != "access_token" || summary.AdminUserID != "7" || !summary.GatewayKeyConfigured {
		t.Fatalf("summary=%#v", summary)
	}
	encoded, err := json.Marshal(summary)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"management-secret", "gateway-secret", encrypted, "api_key_cipher", "admin_token", "admin_cookie", "password"} {
		if strings.Contains(string(encoded), secret) {
			t.Fatalf("summary leaked %q", secret)
		}
	}
	if got := stationSummary(&storage.RelayStation{ID: 2}).Type; got != "sub2api" {
		t.Fatalf("legacy type=%q", got)
	}
}
