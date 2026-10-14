package relay

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// 分组测试只使用该分组的模型调用令牌，管理令牌始终留在管理接口中。
func (s *Service) newAPIGroupGatewayKey(ctx context.Context, base string, c newAPICredential, groupID int64) (string, error) {
	group, err := s.newAPIGroupName(ctx, base, c, groupID)
	if err != nil {
		return "", err
	}
	selfGroup := ""
	selfLoaded := false
	for number := 1; number <= 100000; number++ {
		query := url.Values{"p": {strconv.Itoa(number)}, "page_size": {"100"}}
		env, err := s.newAPIRequest(ctx, http.MethodGet, base+"/api/token/?"+query.Encode(), c, nil)
		if err != nil {
			return "", fmt.Errorf("读取分组调用令牌失败: %w", err)
		}
		var tokens []struct {
			ID         int64    `json:"id"`
			Group      string   `json:"group"`
			AutoGroups []string `json:"auto_groups"`
			Status     int      `json:"status"`
			Expired    int64    `json:"expired_time"`
			Remain     float64  `json:"remain_quota"`
			Unlimited  bool     `json:"unlimited_quota"`
			Key        string   `json:"key"`
		}
		page, pages, err := decodeNewAPIPage(env.Data, query, &tokens)
		if err != nil {
			return "", err
		}
		for _, token := range tokens {
			if token.Status != 1 || (token.Expired > 0 && token.Expired <= time.Now().Unix()) || (!token.Unlimited && token.Remain <= 0) {
				continue
			}
			tokenGroup := token.Group
			if tokenGroup == "" {
				if !selfLoaded {
					self, err := s.newAPIRequest(ctx, http.MethodGet, base+"/api/user/self", c, nil)
					if err != nil {
						return "", err
					}
					var user struct {
						Group string `json:"group"`
					}
					if err := json.Unmarshal(self.Data, &user); err != nil {
						return "", err
					}
					selfGroup, selfLoaded = user.Group, true
				}
				tokenGroup = selfGroup
			}
			groupMatches := tokenGroup == group
			if tokenGroup == "auto" && len(token.AutoGroups) == 1 && token.AutoGroups[0] == group {
				groupMatches = true
			}
			if !groupMatches {
				continue
			}
			key := strings.TrimSpace(token.Key)
			// 新版本列表返回脱敏值，必须调用专用接口取得完整密钥。
			if key == "" || strings.Contains(key, "*") || !strings.HasPrefix(key, "sk-") {
				full, err := s.newAPIRequest(ctx, http.MethodPost, fmt.Sprintf("%s/api/token/%d/key", base, token.ID), c, nil)
				if err != nil {
					return "", fmt.Errorf("读取完整调用令牌失败: %w", err)
				}
				var data struct {
					Key string `json:"key"`
				}
				if err := json.Unmarshal(full.Data, &data); err != nil {
					return "", err
				}
				key = strings.TrimSpace(data.Key)
			}
			if key == "" || strings.Contains(key, "*") {
				continue
			}
			if c.GatewayKey != "" && strings.TrimPrefix(c.GatewayKey, "sk-") != strings.TrimPrefix(key, "sk-") {
				continue
			}
			return key, nil
		}
		if page.Page >= pages || len(tokens) == 0 {
			break
		}
		if number == 100000 {
			return "", errors.New("New API 调用令牌超过安全分页上限，无法确认分组令牌")
		}
	}
	return "", errors.New("该分组没有当前管理员可用的模型调用令牌，请在 New API 中创建并绑定分组；已配置的调用令牌也必须属于该分组")
}
