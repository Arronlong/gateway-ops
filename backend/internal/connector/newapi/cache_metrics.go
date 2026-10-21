package newapi

import "encoding/json"

func cacheTokensFromOther(other string) (read, creation int64, known bool) {
	var data map[string]json.RawMessage
	if json.Unmarshal([]byte(other), &data) != nil {
		return
	}
	readValue := func(key string) (int64, bool) {
		raw, ok := data[key]
		if !ok || string(raw) == "null" {
			return 0, false
		}
		var value int64
		if json.Unmarshal(raw, &value) != nil || value < 0 {
			return 0, false
		}
		return value, true
	}
	read, known = readValue("cache_tokens")
	if !known {
		read, known = readValue("cache_read_tokens")
	}
	if total, ok := readValue("cache_creation_tokens"); ok {
		creation = total
	} else {
		five, _ := readValue("cache_creation_tokens_5m")
		hour, _ := readValue("cache_creation_tokens_1h")
		creation = five + hour
	}
	return
}
