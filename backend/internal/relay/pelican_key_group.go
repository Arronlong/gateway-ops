package relay

import (
	"context"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/guliansheng/gateway-ops/internal/connector"
	"github.com/guliansheng/gateway-ops/internal/storage"
)

type pelicanKeyGroup struct {
	Name       string
	Multiplier *float64
}

func validPelicanMultiplier(value *float64) bool {
	return value != nil && *value >= 0 && !math.IsNaN(*value) && !math.IsInf(*value, 0)
}

func (s *Service) pelicanKeyGroup(ctx context.Context, target *pelicanTarget, stationID uint, accountID int64) pelicanKeyGroup {
	if s.channels == nil || target.oauth {
		return pelicanKeyGroup{}
	}
	// 代理站点需依次读取令牌、完整 Key 和分组，预算覆盖这三次只读查询。
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	sessions, err := s.stations.PelicanKeySessions(ctx, stationID, accountID)
	if err != nil {
		return pelicanKeyGroup{}
	}
	for _, session := range sessions {
		token, err := s.cipher.Decrypt(session.AccessTokenCipher)
		if err != nil {
			continue
		}
		var info pelicanKeyGroup
		if session.Type == storage.ChannelTypeNewAPI {
			headers := s.pelicanNewAPIHeaders(session, token)
			if len(headers) == 0 {
				continue
			}
			info = target.readNewAPIKeyGroup(ctx, session.SiteURL, headers)
		} else {
			if token == "" && session.CredentialMode == storage.CredentialModeToken {
				raw, err := s.cipher.Decrypt(session.CredentialCipher)
				if err != nil {
					continue
				}
				var credential struct {
					AccessToken string `json:"access_token"`
				}
				if json.Unmarshal([]byte(raw), &credential) != nil {
					continue
				}
				token = credential.AccessToken
			}
			if token == "" {
				continue
			}
			info = target.readKeyGroup(ctx, session.SiteURL, token)
		}
		if info.Name != "" {
			return info
		}
	}
	return pelicanKeyGroup{}
}

func (s *Service) pelicanNewAPIHeaders(source storage.PelicanKeySession, token string) http.Header {
	cookie, err := s.cipher.Decrypt(source.CookieCipher)
	if err != nil {
		return nil
	}
	userID := source.UserID
	headers := make(http.Header)
	if source.CredentialMode == storage.CredentialModeToken {
		raw, err := s.cipher.Decrypt(source.CredentialCipher)
		if err != nil {
			return nil
		}
		var credential struct {
			AuthType string                `json:"auth_type"`
			Cookie   string                `json:"cookie"`
			UserID   string                `json:"user_id"`
			Token    string                `json:"token"`
			Headers  []connector.RequestKV `json:"headers"`
		}
		if json.Unmarshal([]byte(raw), &credential) != nil {
			return nil
		}
		userID, cookie, token = credential.UserID, credential.Cookie, credential.Token
		if credential.AuthType == "access_token" && credential.Headers != nil {
			for key, value := range connector.ExpandRequestKV(credential.Headers, map[string]string{"token": token, "user_id": userID}) {
				headers.Set(key, value)
			}
		}
	}
	if headers.Get("Authorization") == "" && token != "" {
		headers.Set("Authorization", "Bearer "+token)
	}
	if cookie != "" {
		headers.Set("Cookie", cookie)
	}
	if userID != "" {
		headers.Set("New-Api-User", userID)
	}
	if headers.Get("Authorization") == "" && headers.Get("Cookie") == "" {
		return nil
	}
	return headers
}

// 分组仅接受完整 Key 的精确匹配；未知信息不影响作品生成，也不推测默认分组。
func (t *pelicanTarget) readKeyGroup(ctx context.Context, site, session string) pelicanKeyGroup {
	if !validPelicanKeySite(site) {
		return pelicanKeyGroup{}
	}
	client := t.client
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	read := func(path, token string, wrapped bool, out any) bool {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(site, "/")+path, nil)
		if err != nil {
			return false
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Accept", "application/json")
		response, err := client.Do(req)
		if err != nil {
			return false
		}
		defer response.Body.Close()
		if response.StatusCode != http.StatusOK {
			return false
		}
		body, err := io.ReadAll(io.LimitReader(response.Body, 2<<20+1))
		if err != nil || len(body) > 2<<20 {
			return false
		}
		if wrapped {
			var envelope struct {
				Code *int            `json:"code"`
				Data json.RawMessage `json:"data"`
			}
			if json.Unmarshal(body, &envelope) != nil || envelope.Code == nil || *envelope.Code != 0 {
				return false
			}
			body = envelope.Data
		}
		return json.Unmarshal(body, out) == nil
	}
	for page := 1; page <= 10; page++ {
		var keys struct {
			Items []struct {
				Key      string  `json:"key"`
				GroupID  *int64  `json:"group_id"`
				GroupIDs []int64 `json:"group_ids"`
				Group    *struct {
					ID             int64    `json:"id"`
					Name           string   `json:"name"`
					RateMultiplier *float64 `json:"rate_multiplier"`
				} `json:"group"`
			} `json:"items"`
			Pages int `json:"pages"`
		}
		if !read("/api/v1/keys?page="+strconv.Itoa(page)+"&page_size=100", session, true, &keys) {
			break
		}
		for _, key := range keys.Items {
			if key.Key != t.token {
				continue
			}
			if len(key.GroupIDs) > 1 || key.Group == nil || key.Group.ID <= 0 || (key.GroupID != nil && *key.GroupID != key.Group.ID) || (len(key.GroupIDs) == 1 && key.GroupIDs[0] != key.Group.ID) {
				return pelicanKeyGroup{}
			}
			info := pelicanKeyGroup{Name: strings.ReplaceAll(t.redact(strings.TrimSpace(key.Group.Name)), session, "[redacted]")}
			if len([]rune(info.Name)) > 256 {
				return pelicanKeyGroup{}
			}
			if info.Name == "" {
				return info
			}
			var billing struct {
				Object   string   `json:"object"`
				Resolved *float64 `json:"resolved_rate_multiplier"`
			}
			if read("/v1/sub2api/billing", t.token, false, &billing) && billing.Object == "sub2api.key_billing" && validPelicanMultiplier(billing.Resolved) {
				info.Multiplier = billing.Resolved
			} else {
				var rates map[string]float64
				if read("/api/v1/groups/rates", session, true, &rates) && rates != nil {
					info.Multiplier = key.Group.RateMultiplier
					if rate, ok := rates[strconv.FormatInt(key.Group.ID, 10)]; ok {
						info.Multiplier = &rate
					}
					if !validPelicanMultiplier(info.Multiplier) {
						info.Multiplier = nil
					}
				}
			}
			return info
		}
		if len(keys.Items) == 0 || (keys.Pages > 0 && page >= keys.Pages) || (keys.Pages == 0 && len(keys.Items) < 100) {
			break
		}
	}
	return pelicanKeyGroup{}
}

// 凭据只发送到其关联渠道保存的地址；生成地址可以是该站点的另一个受信代理入口。
func validPelicanKeySite(site string) bool {
	base, err := url.Parse(strings.TrimRight(site, "/"))
	return err == nil && base.Host != "" && (base.Scheme == "http" || base.Scheme == "https") && base.User == nil && base.RawQuery == "" && base.Fragment == ""
}
