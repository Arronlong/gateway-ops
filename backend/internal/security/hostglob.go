package security

import (
	"errors"
	"fmt"
	"strings"
)

var errInvalidHostPattern = errors.New("invalid host pattern")

func ValidateHostPattern(pattern string) error {
	pattern = normalizePattern(pattern)
	if pattern == "" {
		return fmt.Errorf("%w: empty pattern", errInvalidHostPattern)
	}
	if pattern == "*" {
		return nil
	}
	if strings.Contains(pattern, "://") || strings.ContainsAny(pattern, "/:") {
		return fmt.Errorf("%w: scheme, path, and port are not allowed", errInvalidHostPattern)
	}

	labels := strings.Split(pattern, ".")
	for _, label := range labels {
		if label == "" {
			return fmt.Errorf("%w: empty hostname label", errInvalidHostPattern)
		}
		if label == "**" {
			continue
		}
		if strings.Contains(label, "**") {
			return fmt.Errorf("%w: ** must occupy a complete hostname label", errInvalidHostPattern)
		}
		for _, r := range label {
			if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '*' {
				continue
			}
			return fmt.Errorf("%w: unsupported character %q", errInvalidHostPattern, r)
		}
	}
	return nil
}

func MatchHostPattern(pattern, host string) (bool, error) {
	if err := ValidateHostPattern(pattern); err != nil {
		return false, err
	}

	pattern = normalizePattern(pattern)
	host = normalizeHost(host)
	if host == "" || strings.ContainsAny(host, "/:") {
		return false, nil
	}
	if pattern == "*" {
		return true, nil
	}

	patternLabels := strings.Split(pattern, ".")
	hostLabels := strings.Split(host, ".")
	for _, label := range hostLabels {
		if label == "" {
			return false, nil
		}
	}

	type state struct {
		patternIndex int
		hostIndex    int
	}
	memo := make(map[state]bool)
	seen := make(map[state]bool)

	var match func(int, int) bool
	match = func(patternIndex, hostIndex int) bool {
		current := state{patternIndex: patternIndex, hostIndex: hostIndex}
		if seen[current] {
			return memo[current]
		}
		seen[current] = true

		if patternIndex == len(patternLabels) {
			memo[current] = hostIndex == len(hostLabels)
			return memo[current]
		}
		if hostIndex >= len(hostLabels) {
			memo[current] = false
			return false
		}

		labelPattern := patternLabels[patternIndex]
		if labelPattern == "**" {
			for nextHost := hostIndex + 1; nextHost <= len(hostLabels); nextHost++ {
				if match(patternIndex+1, nextHost) {
					memo[current] = true
					return true
				}
			}
			memo[current] = false
			return false
		}

		if !matchHostLabel(labelPattern, hostLabels[hostIndex]) {
			memo[current] = false
			return false
		}
		memo[current] = match(patternIndex+1, hostIndex+1)
		return memo[current]
	}

	return match(0, 0), nil
}

func MatchAnyHostPattern(patterns []string, host string) (bool, error) {
	for _, pattern := range patterns {
		matched, err := MatchHostPattern(pattern, host)
		if err != nil {
			return false, err
		}
		if matched {
			return true, nil
		}
	}
	return false, nil
}

func normalizePattern(pattern string) string {
	return strings.ToLower(strings.TrimSpace(pattern))
}

func normalizeHost(host string) string {
	host = strings.ToLower(strings.TrimSpace(host))
	return strings.TrimSuffix(host, ".")
}

func matchHostLabel(pattern, value string) bool {
	patternIndex := 0
	valueIndex := 0
	starIndex := -1
	starMatch := 0

	for valueIndex < len(value) {
		if patternIndex < len(pattern) && pattern[patternIndex] == value[valueIndex] {
			patternIndex++
			valueIndex++
			continue
		}
		if patternIndex < len(pattern) && pattern[patternIndex] == '*' {
			starIndex = patternIndex
			patternIndex++
			starMatch = valueIndex
			continue
		}
		if starIndex >= 0 {
			patternIndex = starIndex + 1
			starMatch++
			valueIndex = starMatch
			continue
		}
		return false
	}

	for patternIndex < len(pattern) && pattern[patternIndex] == '*' {
		patternIndex++
	}
	return patternIndex == len(pattern)
}
