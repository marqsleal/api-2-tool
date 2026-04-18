package config

import "testing"

func TestLoadDefaultsAndEnv(t *testing.T) {
	t.Setenv("APP_PORT", "")
	t.Setenv("SQLITE_PATH", "")
	t.Setenv("OPENAPI_SPEC_PATH", "")
	cfg := Load()
	if cfg.Port != "8080" || cfg.SQLitePath != "./data/tools.db" || cfg.OpenAPISpecPath != "./api/openapi.yaml" {
		t.Fatalf("unexpected defaults: %+v", cfg)
	}

	t.Setenv("APP_PORT", "9090")
	t.Setenv("SQLITE_PATH", "/tmp/x.db")
	t.Setenv("OPENAPI_SPEC_PATH", "/tmp/o.yaml")
	cfg = Load()
	if cfg.Port != "9090" || cfg.SQLitePath != "/tmp/x.db" || cfg.OpenAPISpecPath != "/tmp/o.yaml" {
		t.Fatalf("unexpected env config: %+v", cfg)
	}
}
