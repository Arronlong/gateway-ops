package relay

import (
	"encoding/json"
	"regexp"
	"strings"
)

var testSecretPattern = regexp.MustCompile(`(?i)(bearer\s+|(?:x-api-key|api[_-]?key|access_token|refresh_token|password)["\\\s:=]+)([^"\\\s,}]+)`)

func redactTestText(text string, secrets ...string) string {
	plain := func(value string) string {
		for _, secret := range secrets {
			if secret != "" {
				value = strings.ReplaceAll(value, secret, "[REDACTED]")
			}
		}
		return testSecretPattern.ReplaceAllString(value, "${1}[REDACTED]")
	}
	var redactValue func(any, int) any
	var redactText func(string, int) string
	redactValue = func(value any, depth int) any {
		if depth > 16 {
			return "[REDACTED]"
		}
		switch decoded := value.(type) {
		case map[string]any:
			for key, child := range decoded {
				normalized := strings.NewReplacer("-", "", "_", "", " ", "").Replace(strings.ToLower(key))
				switch normalized {
				case "apikey", "xapikey", "authorization", "proxyauthorization", "password", "accesstoken", "refreshtoken", "token", "secret", "clientsecret", "credentials", "cookie", "setcookie":
					decoded[key] = "[REDACTED]"
				default:
					decoded[key] = redactValue(child, depth+1)
				}
			}
			return decoded
		case []any:
			for index, child := range decoded {
				decoded[index] = redactValue(child, depth+1)
			}
			return decoded
		case string:
			return redactText(decoded, depth+1)
		default:
			return value
		}
	}
	redactText = func(value string, depth int) string {
		if depth > 16 {
			return "[REDACTED]"
		}
		var decoded any
		decoder := json.NewDecoder(strings.NewReader(value))
		decoder.UseNumber()
		if json.Valid([]byte(value)) && decoder.Decode(&decoded) == nil {
			if encoded, err := json.Marshal(redactValue(decoded, depth+1)); err == nil {
				return string(encoded)
			}
		}
		lines := strings.Split(value, "\n")
		for index, line := range lines {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "data:") {
				lines[index] = "data: " + redactText(strings.TrimSpace(strings.TrimPrefix(trimmed, "data:")), depth+1)
			} else {
				lines[index] = plain(line)
			}
		}
		return strings.Join(lines, "\n")
	}
	redacted := redactText(text, 0)
	runes := []rune(redacted)
	if len(runes) > 8192 {
		redacted = string(runes[:8192]) + "\n输出过长，已截断"
	}
	return redacted
}
