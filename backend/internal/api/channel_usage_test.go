package api

import "testing"

func TestPercentilePtrCalculatesP95(t *testing.T) {
	values := make([]int64, 20)
	for i := range values {
		values[i] = int64(i + 1)
	}

	got := percentilePtr(values, 0.95)
	if got == nil || *got != 19 {
		t.Fatalf("P95 = %v, want 19", got)
	}
}
