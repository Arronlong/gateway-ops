package relay

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var newAPIReportingStatusPattern = regexp.MustCompile(`(?i)\bstatus(?:[_ ]code)?\s*[=:]\s*([1-5][0-9]{2})\b`)

func (s *Service) newAPIRequestErrors(ctx context.Context, base string, c newAPICredential, q url.Values, out any) error {
	base = strings.TrimRight(base, "/")
	query, err := s.newAPIReportingLogQuery(ctx, base, c, q)
	if err != nil {
		return err
	}
	query.Set("type", "5")
	logs, page, pages, err := s.newAPIReportingLogs(ctx, base, c, query)
	if err != nil {
		return err
	}
	result := remoteRequestErrorPage{Items: []remoteRequestError{}, Total: page.Total, Page: page.Page, PageSize: page.PageSize, Pages: pages}
	for _, log := range logs {
		if log.Type != 5 {
			continue
		}
		var metadata struct {
			StatusCode      json.RawMessage `json:"status_code"`
			ErrorType       string          `json:"error_type"`
			ErrorCode       json.RawMessage `json:"error_code"`
			ErrorOwner      string          `json:"error_owner"`
			ErrorPhase      string          `json:"error_phase"`
			ClientRequestID string          `json:"client_request_id"`
			UpstreamModel   string          `json:"upstream_model_name"`
			FRT             *int64          `json:"frt"`
		}
		_ = json.Unmarshal([]byte(log.Other), &metadata)
		status, _ := strconv.Atoi(newAPIReportingJSONText(metadata.StatusCode))
		if status < 100 || status >= 600 {
			status = http.StatusBadGateway
			if match := newAPIReportingStatusPattern.FindStringSubmatch(log.Content); len(match) > 1 {
				status, _ = strconv.Atoi(match[1])
			}
		}
		kind := newAPIReportingJSONText(metadata.ErrorCode)
		if kind == "" {
			kind = metadata.ErrorType
		}
		owner, phase := metadata.ErrorOwner, metadata.ErrorPhase
		if owner == "" {
			owner = "provider"
		}
		if phase == "" {
			phase = "upstream"
		}
		duration := log.UseTime * 1000
		result.Items = append(result.Items, remoteRequestError{
			RequestID: log.RequestID, ClientRequestID: metadata.ClientRequestID,
			CreatedAt:  time.Unix(log.CreatedAt, 0).UTC().Format(time.RFC3339),
			ErrorPhase: phase, Phase: phase, Type: kind, Message: log.Content, ErrorOwner: owner,
			StatusCode: status, Platform: "newapi", Model: log.Model, RequestedModel: log.Model,
			UpstreamModel: metadata.UpstreamModel, GroupID: newAPIGroupID(log.Group),
			DurationMS: &duration, TimeToFirstTokenMS: metadata.FRT,
		})
	}
	return assignNewAPI(out, result)
}

func (s *Service) newAPIDefaultModelPricing(ctx context.Context, base string, c newAPICredential, model string, out any) error {
	env, err := s.newAPIRequest(ctx, http.MethodGet, strings.TrimRight(base, "/")+"/api/pricing", c, nil)
	if err != nil {
		return err
	}
	var items []struct {
		Model      string   `json:"model_name"`
		Provider   string   `json:"owner_by"`
		QuotaType  int      `json:"quota_type"`
		Ratio      float64  `json:"model_ratio"`
		Price      float64  `json:"model_price"`
		Completion float64  `json:"completion_ratio"`
		Cache      *float64 `json:"cache_ratio"`
		Creation   *float64 `json:"create_cache_ratio"`
		Image      *float64 `json:"image_ratio"`
	}
	if err := json.Unmarshal(env.Data, &items); err != nil {
		return err
	}
	result := remoteDefaultModelPricing{}
	for _, item := range items {
		if item.Model != model {
			continue
		}
		result.Found, result.Provider = true, item.Provider
		if item.QuotaType == 1 {
			result.BillingMode, result.PerRequestPrice = "per_request", &item.Price
		} else {
			// PublicModelPricing 按每个 token 保存价格，前端统一换算为每 1M tokens 展示。
			input := item.Ratio * 2 / 1_000_000
			output := input * item.Completion
			result.BillingMode, result.InputPrice, result.OutputPrice = "token", &input, &output
			scale := func(ratio *float64) *float64 {
				if ratio == nil {
					return nil
				}
				price := input * *ratio
				return &price
			}
			result.CacheReadPrice = scale(item.Cache)
			result.CacheWritePrice = scale(item.Creation)
			result.ImageInputPrice = scale(item.Image)
		}
		break
	}
	return assignNewAPI(out, result)
}

func (s *Service) newAPIUserBalanceHistory(ctx context.Context, base string, c newAPICredential, id int64, q url.Values, out any) error {
	base = strings.TrimRight(base, "/")
	user, err := s.newAPIReportingUser(ctx, base, c, id)
	if err != nil {
		return err
	}
	quota, err := s.newAPIQuota(ctx, base, c)
	if err != nil {
		return err
	}
	query := newAPIReportingPageQuery(q)
	query.Set("username", user.Username)
	query.Set("type", "1")
	logs, page, pages, err := s.newAPIReportingLogs(ctx, base, c, query)
	if err != nil {
		return err
	}
	result := UserBalanceHistory{User: newAPIUserView(user, quota), Items: []UserBalanceHistoryItem{}, Page: page.Page, Pages: pages, Total: page.Total}
	var totalQuota float64
	include := func(log newAPILog) bool {
		return log.Type == 1 && log.UserID == id
	}
	for _, log := range logs {
		if !include(log) {
			continue
		}
		totalQuota += log.Quota
		result.Items = append(result.Items, UserBalanceHistoryItem{
			ID: log.ID, Type: "recharge", Value: log.Quota / quota, Notes: log.Content,
			CreatedAt: time.Unix(log.CreatedAt, 0).UTC(),
		})
	}
	query.Set("page_size", strconv.Itoa(page.PageSize))
	for pageNumber := 1; pageNumber <= pages; pageNumber++ {
		if pageNumber == page.Page {
			continue
		}
		query.Set("p", strconv.Itoa(pageNumber))
		logs, _, _, err := s.newAPIReportingLogs(ctx, base, c, query)
		if err != nil {
			return err
		}
		if len(logs) == 0 && page.Total > 0 {
			return errors.New("New API recharge history has an incomplete page")
		}
		for _, log := range logs {
			if include(log) {
				totalQuota += log.Quota
			}
		}
	}
	result.TotalRecharged = totalQuota / quota
	return assignNewAPI(out, result)
}

func newAPIReportingPageQuery(q url.Values) url.Values {
	query := newAPIPageQuery(q)
	page, err := strconv.Atoi(query.Get("p"))
	if err != nil || page < 1 {
		query.Set("p", "1")
	}
	size, err := strconv.Atoi(query.Get("page_size"))
	if err != nil || size < 1 || size > 100 {
		query.Set("page_size", "100")
	}
	return query
}

func (s *Service) newAPIReportingLogs(ctx context.Context, base string, c newAPICredential, query url.Values) ([]newAPILog, newAPIPage, int, error) {
	env, err := s.newAPIRequest(ctx, http.MethodGet, base+"/api/log/?"+query.Encode(), c, nil)
	if err != nil {
		return nil, newAPIPage{}, 0, err
	}
	var logs []newAPILog
	page, pages, err := decodeNewAPIPage(env.Data, query, &logs)
	return logs, page, pages, err
}

func (s *Service) newAPIReportingUser(ctx context.Context, base string, c newAPICredential, id int64) (newAPIUser, error) {
	if id <= 0 {
		return newAPIUser{}, errors.New("New API user ID must be positive")
	}
	env, err := s.newAPIRequest(ctx, http.MethodGet, fmt.Sprintf("%s/api/user/%d", base, id), c, nil)
	if err != nil {
		return newAPIUser{}, err
	}
	var user newAPIUser
	if err := json.Unmarshal(env.Data, &user); err != nil {
		return newAPIUser{}, err
	}
	if user.ID != id || strings.TrimSpace(user.Username) == "" {
		return newAPIUser{}, errors.New("New API user does not exist or has no username")
	}
	return user, nil
}

func (s *Service) newAPIReportingLogQuery(ctx context.Context, base string, c newAPICredential, q url.Values) (url.Values, error) {
	query := newAPIReportingPageQuery(q)
	for _, key := range []string{"channel", "model_name", "username", "token_name", "group", "request_id", "upstream_request_id", "start_timestamp", "end_timestamp"} {
		if value := q.Get(key); value != "" {
			query.Set(key, value)
		}
	}
	for _, mapping := range [][2]string{{"account_id", "channel"}, {"model", "model_name"}, {"start_date", "start_timestamp"}, {"end_date", "end_timestamp"}, {"start_time", "start_timestamp"}, {"end_time", "end_timestamp"}} {
		value := q.Get(mapping[0])
		if value == "" {
			continue
		}
		if strings.HasSuffix(mapping[1], "timestamp") {
			if date, err := time.Parse(time.RFC3339Nano, value); err == nil {
				value = strconv.FormatInt(date.Unix(), 10)
			} else if date, err := time.Parse("2006-01-02", value); err == nil {
				if mapping[0] == "end_date" {
					date = date.Add(24*time.Hour - time.Second)
				}
				value = strconv.FormatInt(date.Unix(), 10)
			} else if _, err := strconv.ParseInt(value, 10, 64); err != nil {
				return nil, fmt.Errorf("New API invalid %s: %w", mapping[0], err)
			}
		}
		query.Set(mapping[1], value)
	}
	if value := q.Get("user_id"); value != "" {
		id, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return nil, err
		}
		user, err := s.newAPIReportingUser(ctx, base, c, id)
		if err != nil {
			return nil, err
		}
		query.Set("username", user.Username)
	}
	if value := q.Get("group_id"); value != "" {
		id, err := strconv.ParseInt(value, 10, 64)
		if err != nil || id <= 0 {
			return nil, errors.New("New API invalid group ID")
		}
		name, err := s.newAPIGroupName(ctx, base, c, id)
		if err != nil {
			return nil, err
		}
		query.Set("group", name)
	}
	return query, nil
}

func newAPIReportingJSONText(raw json.RawMessage) string {
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return text
	}
	var number json.Number
	if json.Unmarshal(raw, &number) == nil {
		return number.String()
	}
	return ""
}
