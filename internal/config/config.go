package config

import (
	"os"
	"path/filepath"
	"strconv"
	"time"
)

type Config struct {
	HTTPAddress     string
	DatabasePath    string
	BusyBoxImage    string
	PythonImage     string
	ContainerCPUs   string
	ContainerMemory string
	CommandTimeout  time.Duration
	CORSOrigin      string
}

func Load() Config {
	root, err := os.Getwd()
	if err != nil {
		root = "."
	}

	return Config{
		HTTPAddress:     getenv("CLIGREP_HTTP_ADDR", ":11802"),
		DatabasePath:    getenv("CLIGREP_DB_PATH", filepath.Join(root, "data", "cligrep.db")),
		BusyBoxImage:    getenv("CLIGREP_BUSYBOX_IMAGE", "busybox:1.36.1"),
		PythonImage:     getenv("CLIGREP_PYTHON_IMAGE", "python:3.12-slim"),
		ContainerCPUs:   getenv("CLIGREP_CONTAINER_CPUS", "0.50"),
		ContainerMemory: getenv("CLIGREP_CONTAINER_MEMORY", "128m"),
		CommandTimeout:  durationEnv("CLIGREP_COMMAND_TIMEOUT_MS", 4000),
		CORSOrigin:      getenv("CLIGREP_CORS_ORIGIN", "*"),
	}
}

func getenv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func durationEnv(key string, fallbackMS int) time.Duration {
	if value := os.Getenv(key); value != "" {
		if parsed, err := strconv.Atoi(value); err == nil && parsed > 0 {
			return time.Duration(parsed) * time.Millisecond
		}
	}
	return time.Duration(fallbackMS) * time.Millisecond
}
