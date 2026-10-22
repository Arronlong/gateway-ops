package storage

import (
	"strings"
	"testing"
)

func TestDBConfigSearchPathIsIsolatedAndDefaultsUnchanged(t *testing.T) {
	base := DBConfig{Host: "127.0.0.1", Port: 5432, User: "test", Password: "secret", Name: "gatewayops", SSLMode: "disable", Timezone: "Asia/Shanghai"}
	wantDefault := "host=127.0.0.1 port=5432 user=test password=secret dbname=gatewayops sslmode=disable TimeZone=Asia/Shanghai"
	if got := base.DSN(); got != wantDefault {
		t.Fatalf("default DSN changed: %q", got)
	}
	base.SearchPath = "gatewayops_pelican_test_a1b2"
	got := base.DSN()
	if !strings.HasSuffix(got, "search_path=gatewayops_pelican_test_a1b2") {
		t.Fatalf("isolated schema missing from DSN: %q", got)
	}
	base.SearchPath = "public,other"
	if strings.Contains(base.DSN(), "search_path=") {
		t.Fatal("invalid search_path was accepted")
	}
}
