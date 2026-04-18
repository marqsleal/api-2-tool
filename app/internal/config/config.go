package config

import (
	"os"
	"time"
)

type Config struct {
	Port            string
	SQLitePath      string
	OpenAPISpecPath string
	ShutdownTimeout time.Duration
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

	return Config{
		Port:            port,
		SQLitePath:      sqlitePath,
		OpenAPISpecPath: openAPISpecPath,
		ShutdownTimeout: 5 * time.Second,
	}
}
