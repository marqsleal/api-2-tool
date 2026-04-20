package config

import (
	"os"
	"strconv"
	"time"
)

type Config struct {
	Port             string
	SQLitePath       string
	OpenAPISpecPath  string
	ShutdownTimeout  time.Duration
	JobWorkerCount   int
	JobRetention     time.Duration
	LogLevel         string
	HumanLog         bool
	LogIncludeSource bool
}

func Load() Config {
	port := os.Getenv("APP_PORT")
	if port == "" {
		port = "8080"
	}

	sqlitePath := os.Getenv("SQLITE_PATH")
	if sqlitePath == "" {
		sqlitePath = "./data/tools.db"
	}

	openAPISpecPath := os.Getenv("OPENAPI_SPEC_PATH")
	if openAPISpecPath == "" {
		openAPISpecPath = "./api/openapi.yaml"
	}
	jobWorkerCount := 3
	if raw := os.Getenv("JOB_WORKER_COUNT"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			jobWorkerCount = parsed
		}
	}
	jobRetention := 7 * 24 * time.Hour
	if raw := os.Getenv("JOB_RETENTION_HOURS"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			jobRetention = time.Duration(parsed) * time.Hour
		}
	}
	logLevel := os.Getenv("LOG_LEVEL")
	if logLevel == "" {
		logLevel = "INFO"
	}
	humanLog := false
	if raw := os.Getenv("HUMAN_LOG"); raw != "" {
		if parsed, err := strconv.ParseBool(raw); err == nil {
			humanLog = parsed
		}
	}
	logIncludeSource := false
	if raw := os.Getenv("LOG_INCLUDE_SOURCE"); raw != "" {
		if parsed, err := strconv.ParseBool(raw); err == nil {
			logIncludeSource = parsed
		}
	}

	return Config{
		Port:             port,
		SQLitePath:       sqlitePath,
		OpenAPISpecPath:  openAPISpecPath,
		ShutdownTimeout:  5 * time.Second,
		JobWorkerCount:   jobWorkerCount,
		JobRetention:     jobRetention,
		LogLevel:         logLevel,
		HumanLog:         humanLog,
		LogIncludeSource: logIncludeSource,
	}
}
