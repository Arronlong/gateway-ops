package relay

import "testing"

func TestNewAPICacheKnownPreservesZeroReadToken(t *testing.T) {
	cases := []struct {
		other string
		want  bool
	}{
		{`{"cache_tokens":0,"cache_creation_tokens":128}`, true},
		{`{"cache_read_tokens":0}`, true},
		{`{"cache_creation_tokens":128}`, false},
		{`{"cache_tokens":-1}`, false},
		{`{}`, false},
	}
	for _, tc := range cases {
		if got := newAPICacheKnown(tc.other); got != tc.want {
			t.Errorf("newAPICacheKnown(%s) = %v, want %v", tc.other, got, tc.want)
		}
	}
}
