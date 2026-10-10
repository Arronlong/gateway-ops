package security

import "testing"

func TestHostGlobMatchesExpectedHosts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		pattern string
		host    string
		want    bool
	}{
		{name: "all domains", pattern: "*", host: "anything.example", want: true},
		{name: "exact", pattern: "example.com", host: "example.com", want: true},
		{name: "exact excludes subdomain", pattern: "example.com", host: "a.example.com", want: false},
		{name: "single level wildcard", pattern: "*.example.com", host: "a.example.com", want: true},
		{name: "single level excludes multiple", pattern: "*.example.com", host: "a.b.example.com", want: false},
		{name: "single level excludes root", pattern: "*.example.com", host: "example.com", want: false},
		{name: "multi level one", pattern: "**.example.com", host: "a.example.com", want: true},
		{name: "multi level many", pattern: "**.example.com", host: "a.b.example.com", want: true},
		{name: "multi level excludes root", pattern: "**.example.com", host: "example.com", want: false},
		{name: "label substring wildcard", pattern: "a*abc.com", host: "a123abc.com", want: true},
		{name: "label substring wildcard zero chars", pattern: "a*abc.com", host: "aabc.com", want: true},
		{name: "label wildcard does not cross dot", pattern: "a*abc.com", host: "a.fooabc.com", want: false},
		{name: "wildcards around exact label", pattern: "*.devai.*", host: "a.devai.com", want: true},
		{name: "wildcards around exact label other suffix", pattern: "*.devai.*", host: "x.devai.net", want: true},
		{name: "single labels do not absorb multiple", pattern: "*.devai.*", host: "a.b.devai.co.uk", want: false},
		{name: "multi labels around exact label", pattern: "**.devai.**", host: "a.b.devai.c.d", want: true},
		{name: "multi labels around exact label short", pattern: "**.devai.**", host: "a.devai.com", want: true},
		{name: "exact label not substring", pattern: "**.devai.**", host: "mydevai.com", want: false},
		{name: "case insensitive", pattern: "*.Example.COM", host: "A.example.com", want: true},
		{name: "host trailing dot", pattern: "example.com", host: "example.com.", want: true},
		{name: "internal hostname with underscore", pattern: "devai_cn.xxx.cn", host: "devai_cn.xxx.cn", want: true},
		{name: "underscore with wildcard", pattern: "devai_*.xxx.cn", host: "devai_cn.xxx.cn", want: true},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := MatchHostPattern(tt.pattern, tt.host)
			if err != nil {
				t.Fatalf("MatchHostPattern(%q, %q) error: %v", tt.pattern, tt.host, err)
			}
			if got != tt.want {
				t.Fatalf("MatchHostPattern(%q, %q) = %t, want %t", tt.pattern, tt.host, got, tt.want)
			}
		})
	}
}

func TestHostGlobRejectsInvalidPatterns(t *testing.T) {
	t.Parallel()

	for _, pattern := range []string{
		"",
		"https://example.com",
		"example.com/path",
		"example.com:8443",
		"a..example.com",
		"ab**cd.example.com",
	} {
		pattern := pattern
		t.Run(pattern, func(t *testing.T) {
			t.Parallel()
			if err := ValidateHostPattern(pattern); err == nil {
				t.Fatalf("ValidateHostPattern(%q) = nil, want error", pattern)
			}
		})
	}
}

func TestHostGlobMatchAny(t *testing.T) {
	t.Parallel()

	got, err := MatchAnyHostPattern([]string{"example.com", "**.devai.**", "a*abc.com"}, "foo.bar.devai.co.uk")
	if err != nil {
		t.Fatal(err)
	}
	if !got {
		t.Fatal("MatchAnyHostPattern() = false, want true")
	}
}
