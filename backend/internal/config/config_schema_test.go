package config

import "testing"

func TestSchemaEnvironment(t *testing.T) {
	t.Setenv("GATEWAYOPS_DATABASE_SCHEMA", "gatewayops_pelican_test_0123456789ab")
	cfg, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Database.ToStorageConfig().SearchPath != "gatewayops_pelican_test_0123456789ab" {
		t.Fatalf("schema env not loaded: %q", cfg.Database.SearchPath)
	}
}
