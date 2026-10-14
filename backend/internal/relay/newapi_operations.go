package relay

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

// newAPICompleteObjects 使用稳定 ID 排序读取完整列表，对不完整或重复分页返回错误。
func (s *Service) newAPICompleteObjects(ctx context.Context, base string, c newAPICredential, resource string) ([]map[string]json.RawMessage, error) {
	base = strings.TrimRight(base, "/")
	items := make([]map[string]json.RawMessage, 0)
	seen := map[int64]bool{}
	expectedTotal := -1
	for page := 1; page <= 1000; page++ {
		query := url.Values{"p": {strconv.Itoa(page)}, "page_size": {"100"}, "sort_by": {"id"}, "sort_order": {"asc"}}
		if resource == "channel" {
			query.Set("id_sort", "true")
			query.Set("tag_mode", "false")
		}
		env, err := s.newAPIRequest(ctx, http.MethodGet, base+"/api/"+resource+"/?"+query.Encode(), c, nil)
		if err != nil {
			return nil, fmt.Errorf("读取 New API %s 完整列表失败: %w", resource, err)
		}
		raw := bytes.TrimSpace(env.Data)
		var rows []map[string]json.RawMessage
		pageSize := 100
		var total *int
		if len(raw) > 0 && raw[0] == '[' {
			err = json.Unmarshal(raw, &rows)
		} else {
			var data struct {
				Items    json.RawMessage `json:"items"`
				Total    *int            `json:"total"`
				Page     int             `json:"page"`
				PageSize int             `json:"page_size"`
			}
			err = json.Unmarshal(raw, &data)
			if err == nil && (len(data.Items) == 0 || bytes.Equal(bytes.TrimSpace(data.Items), []byte("null"))) {
				err = errors.New("响应缺少分页列表")
			}
			if err == nil {
				err = json.Unmarshal(data.Items, &rows)
			}
			if data.Page != 0 && data.Page != page {
				return nil, fmt.Errorf("New API %s 分页页码异常，请重试", resource)
			}
			if data.PageSize > 0 {
				pageSize = data.PageSize
			}
			total = data.Total
		}
		if err != nil {
			return nil, fmt.Errorf("解析 New API %s 分页失败: %w", resource, err)
		}
		if total != nil {
			if *total < 0 || (expectedTotal >= 0 && expectedTotal != *total) {
				return nil, fmt.Errorf("New API %s 分页总数变化，请重试", resource)
			}
			expectedTotal = *total
		}
		for _, row := range rows {
			var id int64
			if json.Unmarshal(row["id"], &id) != nil || id <= 0 || seen[id] {
				return nil, fmt.Errorf("New API %s 分页包含无效或重复 ID，请重试", resource)
			}
			seen[id] = true
			items = append(items, row)
		}
		if expectedTotal >= 0 {
			if len(items) > expectedTotal {
				return nil, fmt.Errorf("New API %s 分页总数不一致，请重试", resource)
			}
			if len(items) == expectedTotal {
				return items, nil
			}
			if len(rows) == 0 {
				return nil, fmt.Errorf("New API %s 分页提前结束，无法安全完成操作", resource)
			}
		} else if len(rows) < pageSize {
			return items, nil
		}
	}
	return nil, fmt.Errorf("New API %s 分页超过安全上限，无法确认完整列表", resource)
}

func newAPIOperationChannel(row map[string]json.RawMessage) (newAPIChannel, error) {
	var channel newAPIChannel
	raw, err := json.Marshal(row)
	if err == nil {
		err = json.Unmarshal(raw, &channel)
	}
	if err != nil {
		return channel, fmt.Errorf("解析 New API 渠道配置失败: %w", err)
	}
	if channel.ID <= 0 {
		return channel, errors.New("New API 渠道响应缺少有效 ID")
	}
	return channel, nil
}

// newAPICloneAccount 一次写入目标配置，不会对搜索到的同名旧渠道执行修改。
func (s *Service) newAPICloneAccount(ctx context.Context, base string, c newAPICredential, sourceID int64, requested BatchCloneAccount) (*remoteAccount, error) {
	if sourceID <= 0 {
		return nil, errors.New("New API 源渠道 ID 无效")
	}
	requested.Name = strings.TrimSpace(requested.Name)
	requested.APIKey = strings.TrimSpace(requested.APIKey)
	if requested.APIKey == "" {
		return nil, errors.New("新渠道 API Key 不能为空")
	}
	base = strings.TrimRight(base, "/")
	env, err := s.newAPIRequest(ctx, http.MethodGet, fmt.Sprintf("%s/api/channel/%d", base, sourceID), c, nil)
	if err != nil {
		return nil, fmt.Errorf("读取 New API 源渠道失败: %w", err)
	}
	var source map[string]json.RawMessage
	if err := json.Unmarshal(env.Data, &source); err != nil {
		return nil, fmt.Errorf("解析 New API 源渠道失败: %w", err)
	}
	original, err := newAPIOperationChannel(source)
	if err != nil {
		return nil, err
	}
	if original.ID != sourceID {
		return nil, errors.New("New API 源渠道 ID 与请求不一致")
	}
	if requested.Name == "" {
		requested.Name = original.Name
	}
	if requested.Name == "" {
		return nil, errors.New("New API 源渠道名称为空，无法继承")
	}
	requestedBase := requested.BaseURL
	if strings.TrimSpace(requestedBase) == "" {
		requestedBase = original.BaseURL
	}
	targetBase, err := normalizeBaseURL(requestedBase)
	if err != nil {
		return nil, err
	}
	before, err := s.newAPICompleteObjects(ctx, base, c, "channel")
	if err != nil {
		return nil, err
	}
	existing := map[int64]bool{}
	for _, row := range before {
		var id int64
		_ = json.Unmarshal(row["id"], &id)
		existing[id] = true
	}
	if !existing[sourceID] {
		return nil, errors.New("New API 源渠道已不在完整列表中，请刷新后重试")
	}
	// 保留未知的非敏感配置；清除旧凭据、主键和运行状态，让上游初始化新渠道。
	for _, key := range []string{"id", "key", "keys", "api_key", "token", "access_token", "refresh_token", "password", "secret", "created_time", "test_time", "response_time", "balance", "balance_updated_time", "used_quota", "channel_info"} {
		delete(source, key)
	}
	if rawInfo := source["other_info"]; len(rawInfo) > 0 && string(rawInfo) != "null" {
		var encodedInfo string
		if err := json.Unmarshal(rawInfo, &encodedInfo); err != nil {
			return nil, fmt.Errorf("解析 New API 渠道附加信息失败: %w", err)
		}
		info := map[string]json.RawMessage{}
		if strings.TrimSpace(encodedInfo) != "" {
			if err := json.Unmarshal([]byte(encodedInfo), &info); err != nil {
				return nil, fmt.Errorf("解析 New API 渠道附加信息失败: %w", err)
			}
		}
		delete(info, "status_reason")
		delete(info, "status_time")
		if len(info) == 0 {
			delete(source, "other_info")
		} else {
			cleanInfo, err := json.Marshal(info)
			if err != nil {
				return nil, fmt.Errorf("编码 New API 渠道附加信息失败: %w", err)
			}
			source["other_info"], _ = json.Marshal(string(cleanInfo))
		}
	}
	source["name"], _ = json.Marshal(requested.Name)
	source["key"], _ = json.Marshal(requested.APIKey)
	source["base_url"], _ = json.Marshal(targetBase)
	env, err = s.newAPIRequest(ctx, http.MethodPost, base+"/api/channel/", c, map[string]any{"mode": "single", "channel": source})
	if err != nil {
		return nil, fmt.Errorf("创建 New API 渠道失败: %w", err)
	}
	after, err := s.newAPICompleteObjects(ctx, base, c, "channel")
	if err != nil {
		return nil, fmt.Errorf("New API 已返回创建成功，但无法确认新渠道；请在原站核对，不要重复创建: %w", err)
	}
	var candidates []remoteAccount
	for _, row := range after {
		channel, err := newAPIOperationChannel(row)
		if err != nil {
			return nil, err
		}
		if existing[channel.ID] {
			continue
		}
		if channel.Name == requested.Name && strings.TrimRight(channel.BaseURL, "/") == targetBase && channel.Type == original.Type && channel.Models == original.Models && channel.Group == original.Group {
			candidates = append(candidates, newAPIAccount(channel))
		}
	}
	if len(candidates) != 1 {
		return nil, fmt.Errorf("New API 已返回创建成功，但匹配到 %d 个新渠道，无法安全确认 ID；请在原站核对，不要重复创建", len(candidates))
	}
	// 即使上游扩展返回 ID，也不信任它指向已存在的渠道。
	if len(env.Data) > 0 && string(env.Data) != "null" {
		var response struct {
			ID int64 `json:"id"`
		}
		if json.Unmarshal(env.Data, &response) == nil && response.ID > 0 && response.ID != candidates[0].ID {
			return nil, errors.New("New API 创建响应 ID 与新渠道不一致；请在原站核对，不要重复创建")
		}
	}
	return &candidates[0], nil
}

func (s *Service) newAPITestAccount(ctx context.Context, base string, c newAPICredential, id int64, model, mode string) (AccountTestResult, error) {
	if id <= 0 {
		return AccountTestResult{}, errors.New("New API 渠道 ID 无效")
	}
	stream := false
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "", "regular", "non-stream", "non_stream", "nonstream", "standard", "false":
	case "stream", "true":
		stream = true
	default:
		return AccountTestResult{}, fmt.Errorf("New API 测试模式无效: %s", mode)
	}
	query := url.Values{"model": {model}, "stream": {strconv.FormatBool(stream)}}
	endpoint := fmt.Sprintf("%s/api/channel/test/%d?%s", strings.TrimRight(base, "/"), id, query.Encode())
	started := time.Now()
	env, err := s.newAPIRequest(ctx, http.MethodGet, endpoint, c, nil)
	result := AccountTestResult{DurationMS: time.Since(started).Milliseconds()}
	// 输出保留上游逻辑状态和错误消息，耗时使用本次实际请求时长。
	secrets := newAPIManagementSecrets(s, base, c)
	message := sanitizeGatewayMessage(env.Message, secrets...)
	output, _ := json.Marshal(map[string]any{"success": env.Success, "message": message})
	result.Output = string(output)
	if err != nil {
		fields := strings.Fields(err.Error())
		if len(fields) >= 3 && fields[0] == "New" && fields[1] == "API" {
			result.StatusCode, _ = strconv.Atoi(strings.TrimSuffix(fields[2], ":"))
		}
		if result.StatusCode == 0 && !env.Success {
			result.StatusCode = http.StatusOK
		}
		return result, errors.New(sanitizeGatewayMessage(err.Error(), secrets...))
	}
	result.StatusCode = http.StatusOK
	if !env.Success {
		if message == "" {
			message = "上游未提供错误详情"
		}
		return result, fmt.Errorf("New API 渠道测试失败: %s", message)
	}
	return result, nil
}

func newAPIOperationUsesGroup(row map[string]json.RawMessage, name string) (bool, error) {
	var group string
	if len(row["group"]) == 0 {
		return false, errors.New("New API 引用配置缺少分组字段，无法安全删除分组")
	}
	if err := json.Unmarshal(row["group"], &group); err != nil {
		return false, fmt.Errorf("解析 New API 分组引用失败: %w", err)
	}
	for _, value := range newAPISplit(group) {
		if value == name {
			return true, nil
		}
	}
	// 自动分组令牌也可以通过候选列表引用目标分组。
	if group == "auto" {
		var groups []string
		if len(row["auto_groups"]) == 0 || string(row["auto_groups"]) == "null" {
			return true, nil
		}
		if err := json.Unmarshal(row["auto_groups"], &groups); err != nil {
			return false, fmt.Errorf("解析 New API 令牌自动分组失败: %w", err)
		}
		if len(groups) == 0 {
			return true, nil
		}
		for _, value := range groups {
			if value == name {
				return true, nil
			}
		}
	}
	return false, nil
}

func (s *Service) newAPIDeleteGroup(ctx context.Context, base string, c newAPICredential, id int64) error {
	if id <= 0 {
		return errors.New("New API 分组 ID 无效")
	}
	if id == newAPIGroupID("default") {
		return errors.New("New API default 默认分组不能删除")
	}
	base = strings.TrimRight(base, "/")
	name, err := s.newAPIGroupName(ctx, base, c, id)
	if err != nil {
		return err
	}
	if name == "default" {
		return errors.New("New API default 默认分组不能删除")
	}
	for _, resource := range []struct{ path, label, action string }{
		{"channel", "渠道", "请先将渠道迁移到其他分组"},
		{"user", "用户", "请先将用户迁移到其他分组"},
		{"token", "当前管理员令牌", "请先修改或删除令牌的分组配置"},
	} {
		rows, err := s.newAPICompleteObjects(ctx, base, c, resource.path)
		if err != nil {
			return fmt.Errorf("无法确认%s引用，已取消删除分组: %w", resource.label, err)
		}
		for _, row := range rows {
			used, err := newAPIOperationUsesGroup(row, name)
			if err != nil {
				return err
			}
			if used {
				return fmt.Errorf("New API 分组 %s 仍被%s ID=%s 引用，%s后再删除", name, resource.label, string(row["id"]), resource.action)
			}
		}
	}
	s.newAPIOptionsMu.Lock()
	defer s.newAPIOptionsMu.Unlock()
	options, err := s.newAPIOptions(ctx, base, c)
	if err != nil {
		return fmt.Errorf("读取 New API 完整分组倍率失败: %w", err)
	}
	key := ""
	for _, candidate := range []string{"GroupRatio", "group_ratio_setting.group_ratio"} {
		if _, ok := options[candidate]; ok {
			key = candidate
			break
		}
	}
	if key == "" {
		return errors.New("New API 完整 GroupRatio 配置不可见，请使用具有全局设置权限的管理员凭据")
	}
	// 使用原始 JSON 数值保留其他分组的数值精度。
	var ratios map[string]json.RawMessage
	if err := json.Unmarshal([]byte(options[key]), &ratios); err != nil {
		return fmt.Errorf("解析 New API 完整 GroupRatio 失败: %w", err)
	}
	if _, ok := ratios[name]; !ok {
		return fmt.Errorf("New API GroupRatio 中不存在分组 %s，请刷新后重试", name)
	}
	delete(ratios, name)
	raw, err := json.Marshal(ratios)
	if err != nil {
		return fmt.Errorf("编码 New API GroupRatio 失败: %w", err)
	}
	_, err = s.newAPIRequest(ctx, http.MethodPut, base+"/api/option/", c, map[string]string{"key": key, "value": string(raw)})
	if err != nil {
		return fmt.Errorf("删除 New API 分组配置失败: %w", err)
	}
	return nil
}

func (s *Service) newAPIGroupModels(ctx context.Context, base string, c newAPICredential, id int64) ([]string, error) {
	if id <= 0 {
		return nil, errors.New("New API 分组 ID 无效")
	}
	base = strings.TrimRight(base, "/")
	name, err := s.newAPIGroupName(ctx, base, c, id)
	if err != nil {
		return nil, err
	}
	channels, err := s.newAPICompleteObjects(ctx, base, c, "channel")
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	for _, row := range channels {
		channel, err := newAPIOperationChannel(row)
		if err != nil {
			return nil, err
		}
		if channel.Status != 1 {
			continue
		}
		used, err := newAPIOperationUsesGroup(row, name)
		if err != nil {
			return nil, err
		}
		if !used {
			continue
		}
		for _, model := range newAPISplit(channel.Models) {
			seen[model] = true
		}
	}
	models := make([]string, 0, len(seen))
	for model := range seen {
		models = append(models, model)
	}
	sort.Strings(models)
	return models, nil
}

func newAPIManagementSecrets(s *Service, base string, c newAPICredential) []string {
	secrets := []string{c.Password, c.Token, strings.TrimPrefix(c.Token, "Bearer "), c.GatewayKey}
	appendCookie := func(cookie string) {
		if cookie == "" {
			return
		}
		secrets = append(secrets, cookie)
		for _, part := range strings.Split(cookie, ";") {
			if _, value, ok := strings.Cut(strings.TrimSpace(part), "="); ok && value != "" {
				secrets = append(secrets, value)
			}
		}
	}
	appendCookie(c.Cookie)
	if c.Username != "" {
		entry := s.newAPISessionEntry(base, c)
		entry.mu.Lock()
		secrets = append(secrets, entry.session.Token)
		appendCookie(entry.session.Cookie)
		entry.mu.Unlock()
	}
	return secrets
}

func cloneRemoteAccountExtra(extra map[string]any) (map[string]any, error) {
	copy := map[string]any{}
	if len(extra) == 0 {
		return copy, nil
	}
	raw, err := json.Marshal(extra)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(raw, &copy); err != nil {
		return nil, err
	}
	return copy, nil
}

func (s *Service) newAPIProbeAccounts(ctx context.Context, base string, c newAPICredential, accounts []remoteAccount) ([]remoteAccount, error) {
	result := make([]remoteAccount, len(accounts))
	copy(result, accounts)
	for i := range result {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		account := &result[i]
		account.RateMultiplier = nil
		extra, err := cloneRemoteAccountExtra(account.Extra)
		if err != nil {
			return nil, fmt.Errorf("复制 New API 账号扩展数据失败: %w", err)
		}
		account.Extra = extra
		now := time.Now().UTC().Format(time.RFC3339Nano)
		if !strings.EqualFold(strings.TrimSpace(account.Type), "apikey") || account.ID <= 0 {
			account.Extra["upstream_billing_probe"] = map[string]any{
				"status": "manual_required", "error": "该账号类型不支持自动成本探测，请通过成本关联或手工倍率配置成本", "received_at": now,
			}
			continue
		}
		account.Extra["upstream_billing_probe"] = map[string]any{
			"status":      "manual_required",
			"error":       "New API 使用一次性安全验证保护渠道密钥，后台不会读取或复用该密钥；成本倍率请通过成本关联或手工倍率配置",
			"received_at": now,
		}
	}
	return result, nil
}
