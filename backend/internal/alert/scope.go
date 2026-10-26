package alert

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

type scopeValueKind uint8

const (
	scopeText scopeValueKind = iota
	scopeNumber
	scopeBoolean
	scopeTime
)

var scopeFieldKinds = map[TargetType]map[string]scopeValueKind{
	TargetChannel: {
		"name": scopeText, "type": scopeText, "monitor_enabled": scopeBoolean,
		"balance_mode": scopeText, "balance": scopeNumber, "score": scopeNumber,
		"cache_hit_rate": scopeNumber, "model": scopeText,
	},
	TargetRelayAccount: {
		"station_name": scopeText, "station_type": scopeText, "name": scopeText,
		"identity": scopeText, "platform": scopeText, "type": scopeText, "status": scopeText,
		"schedulable": scopeBoolean, "account_plan": scopeText, "model_type": scopeText,
		"expires_at": scopeTime, "last_used_at": scopeTime,
	},
	TargetRelayUser: {
		"station_name": scopeText, "station_type": scopeText, "username": scopeText,
		"email": scopeText, "role": scopeText, "status": scopeText, "balance": scopeNumber,
		"concurrency": scopeNumber, "rpm_limit": scopeNumber, "last_used_at": scopeTime,
		"created_at": scopeTime,
	},
}

func validateScope(target TargetType, scope ScopeConfig) error {
	fields, ok := scopeFieldKinds[target]
	if !ok {
		return fmt.Errorf("unsupported alert target type: %s", target)
	}
	if len(scope.Filters) > 20 {
		return errors.New("对象筛选条件不能超过 20 条")
	}
	if scope.Logic != "" && scope.Logic != "all" && scope.Logic != "any" {
		return errors.New("对象筛选组合方式无效")
	}
	for _, filter := range scope.Filters {
		kind, ok := fields[filter.Field]
		if !ok {
			return fmt.Errorf("对象筛选字段不支持: %s", filter.Field)
		}
		if len(filter.Value) == 0 || len([]rune(filter.Value)) > 256 {
			return errors.New("对象筛选值不能为空且不能超过 256 字")
		}
		if !validScopeOperator(kind, filter.Operator) {
			return fmt.Errorf("对象筛选操作符与字段不匹配: %s", filter.Operator)
		}
		if err := validateScopeValue(kind, filter.Value); err != nil {
			return fmt.Errorf("对象筛选值无效: %w", err)
		}
	}
	return nil
}

func validScopeOperator(kind scopeValueKind, operator string) bool {
	switch operator {
	case "eq", "ne":
		return true
	case "contains", "not_contains":
		return kind == scopeText
	case "gt", "gte", "lt", "lte":
		return kind == scopeNumber || kind == scopeTime
	default:
		return false
	}
}

func validateScopeValue(kind scopeValueKind, value string) error {
	switch kind {
	case scopeNumber:
		number, err := strconv.ParseFloat(value, 64)
		if err != nil || math.IsNaN(number) || math.IsInf(number, 0) {
			return errors.New("必须是有限数字")
		}
	case scopeBoolean:
		if _, err := strconv.ParseBool(value); err != nil {
			return errors.New("必须是 true 或 false")
		}
	case scopeTime:
		if _, err := time.Parse(time.RFC3339, value); err != nil {
			return errors.New("时间必须是含时区的 RFC3339 格式")
		}
	}
	return nil
}

func scopeMatches(target TargetType, scope ScopeConfig, attributes map[string]string) bool {
	if len(scope.Filters) == 0 {
		return true
	}
	if _, ok := scopeFieldKinds[target]; !ok {
		return false
	}
	any := scope.Logic == "any"
	for _, filter := range scope.Filters {
		actual, exists := attributes[filter.Field]
		matched := false
		if exists && filter.Field == "model" {
			if strings.TrimSpace(actual) != "" {
				negative := filter.Operator == "ne" || filter.Operator == "not_contains"
				matchOperator := filter.Operator
				if filter.Operator == "ne" {
					matchOperator = "eq"
				} else if filter.Operator == "not_contains" {
					matchOperator = "contains"
				}
				for _, model := range strings.Split(actual, "\n") {
					if matchesScopeValue(scopeFieldKinds[target][filter.Field], matchOperator, model, filter.Value) {
						matched = true
						break
					}
				}
				if negative {
					matched = !matched
				}
			}
		} else if exists {
			matched = matchesScopeValue(scopeFieldKinds[target][filter.Field], filter.Operator, actual, filter.Value)
		}
		if any && matched {
			return true
		}
		if !any && !matched {
			return false
		}
	}
	return !any
}

func matchesScopeValue(kind scopeValueKind, operator, actual, expected string) bool {
	switch kind {
	case scopeText:
		actual, expected = strings.ToLower(strings.TrimSpace(actual)), strings.ToLower(strings.TrimSpace(expected))
		switch operator {
		case "eq":
			return actual == expected
		case "ne":
			return actual != expected
		case "contains":
			return strings.Contains(actual, expected)
		case "not_contains":
			return !strings.Contains(actual, expected)
		}
	case scopeNumber:
		left, leftErr := strconv.ParseFloat(actual, 64)
		right, rightErr := strconv.ParseFloat(expected, 64)
		if leftErr != nil || rightErr != nil || math.IsNaN(left) || math.IsInf(left, 0) || math.IsNaN(right) || math.IsInf(right, 0) {
			return false
		}
		switch operator {
		case "eq":
			return left == right
		case "ne":
			return left != right
		case "gt":
			return left > right
		case "gte":
			return left >= right
		case "lt":
			return left < right
		case "lte":
			return left <= right
		}
	case scopeBoolean:
		left, leftErr := strconv.ParseBool(actual)
		right, rightErr := strconv.ParseBool(expected)
		if leftErr != nil || rightErr != nil {
			return false
		}
		if operator == "eq" {
			return left == right
		}
		return operator == "ne" && left != right
	case scopeTime:
		left, leftErr := time.Parse(time.RFC3339, actual)
		right, rightErr := time.Parse(time.RFC3339, expected)
		if leftErr != nil || rightErr != nil {
			return false
		}
		switch operator {
		case "eq":
			return left.Equal(right)
		case "ne":
			return !left.Equal(right)
		case "gt":
			return left.After(right)
		case "gte":
			return left.Equal(right) || left.After(right)
		case "lt":
			return left.Before(right)
		case "lte":
			return left.Equal(right) || left.Before(right)
		}
	}
	return false
}
