package config

import (
	"net/http"
	"os"
	"strconv"
	"time"
)

type Config struct {
	HTTPAddress        string
	DBHost             string
	DBPort             int
	DBName             string
	DBUser             string
	DBPassword         string
	BusyBoxImage       string
	PythonImage        string
	ContainerCPUs      string
	ContainerMemory    string
	CommandTimeout     time.Duration
	CORSOrigin         string
	GoogleClientID     string
	GoogleSecret       string
	GoogleRedirect     string
	AuthSuccessURL     string
	AuthFailureURL     string
	SessionTTL         time.Duration
	AuthCookieName     string
	AuthCookieSecure   bool
	AuthCookieDomain   string
	AuthCookieSameSite http.SameSite
}

func Load() Config {
	return Config{
		HTTPAddress:        getenv("CLIGREP_HTTP_ADDR", ":11802"),
		DBHost:             getenv("CLIGREP_DB_HOST", "13.212.113.109"),
		DBPort:             intEnv("CLIGREP_DB_PORT", 3306),
		DBName:             getenv("CLIGREP_DB_NAME", "cligrep"),
		DBUser:             getenv("CLIGREP_DB_USER", "cligrep"),
		DBPassword:         getenv("CLIGREP_DB_PASSWORD", "cligrep0@123"),
		BusyBoxImage:       getenv("CLIGREP_BUSYBOX_IMAGE", "busybox:1.36.1"),
		PythonImage:        getenv("CLIGREP_PYTHON_IMAGE", "python:3.12-slim"),
		ContainerCPUs:      getenv("CLIGREP_CONTAINER_CPUS", "0.50"),
		ContainerMemory:    getenv("CLIGREP_CONTAINER_MEMORY", "128m"),
		CommandTimeout:     durationEnv("CLIGREP_COMMAND_TIMEOUT_MS", 4000),
		CORSOrigin:         getenv("CLIGREP_CORS_ORIGIN", "http://127.0.0.1:11801,http://localhost:11801,http://127.0.0.1:5173,http://localhost:5173"),
		GoogleClientID:     getenv("CLIGREP_AUTH_GOOGLE_CLIENT_ID", ""),
		GoogleSecret:       getenv("CLIGREP_AUTH_GOOGLE_CLIENT_SECRET", ""),
		GoogleRedirect:     getenv("CLIGREP_AUTH_GOOGLE_REDIRECT_URL", "http://127.0.0.1:11802/api/v1/auth/google/callback"),
		AuthSuccessURL:     getenv("CLIGREP_AUTH_GOOGLE_SUCCESS_URL", "http://127.0.0.1:11801/"),
		AuthFailureURL:     getenv("CLIGREP_AUTH_GOOGLE_FAILURE_URL", "http://127.0.0.1:11801/login?error=google_oauth"),
		SessionTTL:         time.Duration(intEnv("CLIGREP_AUTH_SESSION_TTL_HOURS", 168)) * time.Hour,
		AuthCookieName:     getenv("CLIGREP_AUTH_COOKIE_NAME", "cligrep_session"),
		AuthCookieSecure:   boolEnv("CLIGREP_AUTH_COOKIE_SECURE", false),
		AuthCookieDomain:   getenv("CLIGREP_AUTH_COOKIE_DOMAIN", ""),
		AuthCookieSameSite: sameSiteEnv("CLIGREP_AUTH_COOKIE_SAMESITE", http.SameSiteLaxMode),
	}
}

func getenv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func intEnv(key string, fallback int) int {
	if value := os.Getenv(key); value != "" {
		if parsed, err := strconv.Atoi(value); err == nil && parsed > 0 {
			return parsed
		}
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

func boolEnv(key string, fallback bool) bool {
	if value := os.Getenv(key); value != "" {
		parsed, err := strconv.ParseBool(value)
		if err == nil {
			return parsed
		}
	}
	return fallback
}

func sameSiteEnv(key string, fallback http.SameSite) http.SameSite {
	switch getenv(key, "") {
	case "strict", "Strict", "STRICT":
		return http.SameSiteStrictMode
	case "none", "None", "NONE":
		return http.SameSiteNoneMode
	case "lax", "Lax", "LAX":
		return http.SameSiteLaxMode
	case "":
		return fallback
	default:
		return fallback
	}
}
