package relay

import "encoding/json"

func (usage *remoteUsage) UnmarshalJSON(data []byte) error {
	type alias remoteUsage
	var decoded alias
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	*usage = remoteUsage(decoded)
	if raw, ok := fields["cache_read_tokens"]; ok && string(raw) != "null" {
		var tokens int64
		usage.CacheKnown = json.Unmarshal(raw, &tokens) == nil && tokens >= 0
	}
	return nil
}
