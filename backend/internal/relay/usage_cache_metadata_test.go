package relay

import (
	"encoding/json"
	"testing"
	"time"
)

func TestRelayUsageCacheAvailabilitySurvivesPersistence(t *testing.T) {
	for _, tc := range []struct {
		raw   string
		known bool
	}{{`{"cache_read_tokens":0}`, true}, {`{}`, false}, {`{"cache_read_tokens":null}`, false}, {`{"cache_read_tokens":12}`, true}} {
		var usage remoteUsage
		if err := json.Unmarshal([]byte(tc.raw), &usage); err != nil {
			t.Fatal(err)
		}
		usage.ID = 1
		usage.CreatedAt = time.Now().UTC().Format(time.RFC3339Nano)
		event, ok := usageEventFromRemote(7, usage, time.Now())
		if !ok {
			t.Fatal("invalid fixture")
		}
		sample, ok := latencySampleFromUsage(remoteUsageFromEvent(event))
		if !ok || sample.CacheKnown != tc.known {
			t.Fatalf("%s => %#v", tc.raw, sample)
		}
	}
}
