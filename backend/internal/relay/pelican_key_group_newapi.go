package relay

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
)

// New API 使用用户会话查询令牌，列表脱敏时必须再读取完整 Key 做精确匹配。
func (t *pelicanTarget) readNewAPIKeyGroup(ctx context.Context, site string, headers http.Header) pelicanKeyGroup {
	if !validPelicanKeySite(site) {
		return pelicanKeyGroup{}
	}
	client := t.client
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	read := func(method, path string, out any) bool {
		req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(site, "/")+path, nil)
		if err != nil {
			return false
		}
		req.Header = headers.Clone()
		if req.Header == nil {
			req.Header = make(http.Header)
		}
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
		var envelope struct {
			Success bool            `json:"success"`
			Data    json.RawMessage `json:"data"`
		}
		return json.Unmarshal(body, &envelope) == nil && envelope.Success && json.Unmarshal(envelope.Data, out) == nil
	}
	type apiToken struct {
		ID         int64    `json:"id"`
		Key        string   `json:"key"`
		Group      string   `json:"group"`
		AutoGroups []string `json:"auto_groups"`
	}
	normalize := func(key string) string { return strings.TrimPrefix(strings.TrimSpace(key), "sk-") }
	actual := normalize(t.token)
	if actual == "" {
		return pelicanKeyGroup{}
	}
	fullReads := 0
	for page := 1; page <= 10; page++ {
		var raw json.RawMessage
		if !read(http.MethodGet, "/api/token/?p="+strconv.Itoa(page)+"&page_size=100", &raw) {
			break
		}
		var list struct {
			Items []apiToken `json:"items"`
			Total int        `json:"total"`
		}
		legacy := len(raw) > 0 && raw[0] == byte(91)
		if legacy {
			if json.Unmarshal(raw, &list.Items) != nil {
				return pelicanKeyGroup{}
			}
		} else if json.Unmarshal(raw, &list) != nil {
			return pelicanKeyGroup{}
		}
		for _, token := range list.Items {
			key := normalize(token.Key)
			if strings.Contains(key, "*") {
				pattern := "^" + strings.ReplaceAll(regexp.QuoteMeta(key), `\*`, ".*") + "$"
				matches, _ := regexp.MatchString(pattern, actual)
				if !matches || token.ID <= 0 || fullReads >= 5 {
					continue
				}
				fullReads++
				var full struct {
					Key string `json:"key"`
				}
				if !read(http.MethodPost, "/api/token/"+strconv.FormatInt(token.ID, 10)+"/key", &full) {
					continue
				}
				key = normalize(full.Key)
			}
			if key != actual {
				continue
			}
			group := strings.TrimSpace(token.Group)
			if group == "" {
				var self struct {
					Group string `json:"group"`
				}
				if !read(http.MethodGet, "/api/user/self", &self) {
					return pelicanKeyGroup{}
				}
				group = strings.TrimSpace(self.Group)
			}
			if group == "auto" && len(token.AutoGroups) == 1 {
				group = token.AutoGroups[0]
			}
			if group == "" || len([]rune(group)) > 256 {
				return pelicanKeyGroup{}
			}
			info := pelicanKeyGroup{Name: t.redact(group)}
			if group == "auto" {
				return info
			}
			// auto 等组允许字符串倍率，因此先单独解码匹配分组，避免污染其他组。
			var values map[string]json.RawMessage
			if read(http.MethodGet, "/api/user/self/groups", &values) {
				var value struct {
					Ratio *float64 `json:"ratio"`
				}
				if raw, ok := values[group]; ok && json.Unmarshal(raw, &value) == nil && validPelicanMultiplier(value.Ratio) {
					info.Multiplier = value.Ratio
				}
			}
			return info
		}
		if len(list.Items) == 0 || legacy || (list.Total > 0 && page*100 >= list.Total) || (list.Total == 0 && len(list.Items) < 100) {
			break
		}
	}
	return pelicanKeyGroup{}
}
