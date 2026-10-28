package channel

import (
	"errors"
	"strings"

	"github.com/guliansheng/gateway-ops/internal/crypto"
	"github.com/guliansheng/gateway-ops/internal/storage"
)

func encryptPelicanAPIKey(cipher *crypto.Cipher, value string) (string, error) {
	value = strings.TrimSpace(value)
	if len(value) > 4096 {
		return "", errors.New("测智 API Key 不能超过 4096 字节")
	}
	if value == "" {
		return "", nil
	}
	if cipher == nil {
		return "", errors.New("测智 API Key 加密器不可用")
	}
	return cipher.Encrypt(value)
}

func applyPelicanAPIKey(cipher *crypto.Cipher, channel *storage.Channel, value *string) error {
	if value == nil {
		return nil
	}
	encrypted, err := encryptPelicanAPIKey(cipher, *value)
	if err != nil {
		return err
	}
	channel.PelicanKeyCipher = encrypted
	return nil
}
