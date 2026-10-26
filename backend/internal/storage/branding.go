package storage

import (
	"bytes"
	"encoding/base64"
	"errors"
	"image/png"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	SettingProductName  = "ui.branding.product_name"
	SettingProductIcon  = "ui.branding.product_icon"
	DefaultProductName  = "GatewayOps"
	MaxProductIconBytes = 512 << 10
)

type BrandingSettings struct {
	ProductName string `json:"product_name"`
	ProductIcon string `json:"product_icon"`
}

func NormalizeProductName(name string) string {
	if name = strings.TrimSpace(name); name == "" {
		return DefaultProductName
	}
	return name
}

func ValidateBranding(name, icon string) error {
	if !utf8.ValidString(name) || utf8.RuneCountInString(name) > 40 || strings.ContainsFunc(name, unicode.IsControl) {
		return errors.New("产品名称最多 40 个字符，不能包含控制字符")
	}
	if icon == "" {
		return nil
	}
	const prefix = "data:image/png;base64,"
	if !strings.HasPrefix(icon, prefix) || len(icon) > len(prefix)+base64.StdEncoding.EncodedLen(MaxProductIconBytes) {
		return errors.New("产品图标必须是最大 512 KiB 的 PNG 图片")
	}
	data, err := base64.StdEncoding.Strict().DecodeString(strings.TrimPrefix(icon, prefix))
	if err != nil || len(data) > MaxProductIconBytes {
		return errors.New("产品图标编码无效")
	}
	config, err := png.DecodeConfig(bytes.NewReader(data))
	if err != nil || config.Width < 1 || config.Height < 1 || config.Width > 128 || config.Height > 128 {
		return errors.New("产品图标尺寸必须在 1 到 128 像素之间")
	}
	if _, err := png.Decode(bytes.NewReader(data)); err != nil {
		return errors.New("产品图标内容损坏")
	}
	return nil
}

func (s *Settings) BrandingSettings() (BrandingSettings, error) {
	settings := BrandingSettings{ProductName: DefaultProductName}
	for _, field := range []struct {
		key    string
		target *string
	}{{SettingProductName, &settings.ProductName}, {SettingProductIcon, &settings.ProductIcon}} {
		value, found, err := s.Get(field.key)
		if err != nil {
			return settings, err
		}
		if found {
			*field.target = value
		}
	}
	settings.ProductName = NormalizeProductName(settings.ProductName)
	return settings, nil
}
