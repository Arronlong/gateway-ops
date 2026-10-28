package channel

import (
	"testing"

	appcrypto "github.com/guliansheng/gateway-ops/internal/crypto"
	"github.com/guliansheng/gateway-ops/internal/storage"
)

func TestApplyPelicanAPIKeyEncryptsPreservesAndClears(t *testing.T) {
	cipher, err := appcrypto.NewCipher("test-pelican-key-secret")
	if err != nil {
		t.Fatal(err)
	}
	channel := storage.Channel{}
	key := "upstream-secret"
	if err := applyPelicanAPIKey(cipher, &channel, &key); err != nil {
		t.Fatal(err)
	}
	if channel.PelicanKeyCipher == "" || channel.PelicanKeyCipher == key {
		t.Fatalf("key was not encrypted: %q", channel.PelicanKeyCipher)
	}
	plain, err := cipher.Decrypt(channel.PelicanKeyCipher)
	if err != nil || plain != key {
		t.Fatalf("decrypted key=%q err=%v", plain, err)
	}
	if err := applyPelicanAPIKey(cipher, &channel, nil); err != nil {
		t.Fatal(err)
	}
	if channel.PelicanKeyCipher == "" {
		t.Fatal("omitted key update cleared the configured key")
	}
	empty := ""
	if err := applyPelicanAPIKey(cipher, &channel, &empty); err != nil {
		t.Fatal(err)
	}
	if channel.PelicanKeyCipher != "" {
		t.Fatal("empty key update did not clear the configured key")
	}
}
