package config

import "testing"

func TestLoadDefaultsAndEnv(t *testing.T) {
	t.Setenv("APP_PORT", "")
	t.Setenv("SQLITE_PATH", "")
	t.Setenv("OPENAPI_SPEC_PATH", "")
	t.Setenv("JOB_WORKER_COUNT", "")
	t.Setenv("JOB_RETENTION_HOURS", "")
	cfg := Load()
	if cfg.Port != "8080" || cfg.SQLitePath != "./data/tools.db" || cfg.OpenAPISpecPath != "./api/openapi.yaml" || cfg.JobWorkerCount != 3 {
		t.Fatalf("unexpected defaults: %+v", cfg)
	}
	if cfg.JobRetention.Hours() != 168 {
		t.Fatalf("unexpected default retention: %+v", cfg)
	}

	t.Setenv("APP_PORT", "9090")
	t.Setenv("SQLITE_PATH", "/tmp/x.db")
	t.Setenv("OPENAPI_SPEC_PATH", "/tmp/o.yaml")
	t.Setenv("JOB_WORKER_COUNT", "5")
	t.Setenv("JOB_RETENTION_HOURS", "24")
	cfg = Load()
	if cfg.Port != "9090" || cfg.SQLitePath != "/tmp/x.db" || cfg.OpenAPISpecPath != "/tmp/o.yaml" || cfg.JobWorkerCount != 5 {
		t.Fatalf("unexpected env config: %+v", cfg)
	}
	if cfg.JobRetention.Hours() != 24 {
		t.Fatalf("unexpected env retention: %+v", cfg)
	}
}
