package config

import (
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

func TestLoadAuthDefaultsAndOverrides(t *testing.T) {
	t.Setenv("CLIGREP_AUTH_GOOGLE_CLIENT_ID", "client-id")
	t.Setenv("CLIGREP_AUTH_GOOGLE_CLIENT_SECRET", "client-secret")
	t.Setenv("CLIGREP_AUTH_GOOGLE_REDIRECT_URL", "https://example.com/api/v1/auth/google/callback")
	t.Setenv("CLIGREP_AUTH_GOOGLE_SUCCESS_URL", "https://example.com/app")
	t.Setenv("CLIGREP_AUTH_GOOGLE_FAILURE_URL", "https://example.com/login")
	t.Setenv("CLIGREP_AUTH_SESSION_TTL_HOURS", "24")
	t.Setenv("CLIGREP_AUTH_COOKIE_NAME", "custom_session")
	t.Setenv("CLIGREP_AUTH_COOKIE_SECURE", "true")
	t.Setenv("CLIGREP_AUTH_COOKIE_DOMAIN", ".example.com")
	t.Setenv("CLIGREP_AUTH_COOKIE_SAMESITE", "Strict")

	cfg := Load()

	if cfg.GoogleClientID != "client-id" {
		t.Fatalf("unexpected client id %q", cfg.GoogleClientID)
	}
	if cfg.GoogleSecret != "client-secret" {
		t.Fatalf("unexpected client secret %q", cfg.GoogleSecret)
	}
	if cfg.GoogleRedirect != "https://example.com/api/v1/auth/google/callback" {
		t.Fatalf("unexpected redirect %q", cfg.GoogleRedirect)
	}
	if cfg.AuthSuccessURL != "https://example.com/app" {
		t.Fatalf("unexpected success url %q", cfg.AuthSuccessURL)
	}
	if cfg.AuthFailureURL != "https://example.com/login" {
		t.Fatalf("unexpected failure url %q", cfg.AuthFailureURL)
	}
	if cfg.SessionTTL != 24*time.Hour {
		t.Fatalf("unexpected session ttl %s", cfg.SessionTTL)
	}
	if cfg.AuthCookieName != "custom_session" {
		t.Fatalf("unexpected cookie name %q", cfg.AuthCookieName)
	}
	if !cfg.AuthCookieSecure {
		t.Fatal("expected secure cookie")
	}
	if cfg.AuthCookieDomain != ".example.com" {
		t.Fatalf("unexpected cookie domain %q", cfg.AuthCookieDomain)
	}
	if cfg.AuthCookieSameSite != http.SameSiteStrictMode {
		t.Fatalf("unexpected same site %v", cfg.AuthCookieSameSite)
	}
}

func TestLoadAuthDefaults(t *testing.T) {
	for _, key := range []string{
		"CLIGREP_DB_HOST",
		"CLIGREP_DB_PORT",
		"CLIGREP_DB_NAME",
		"CLIGREP_DB_USER",
		"CLIGREP_DB_PASSWORD",
		"CLIGREP_AUTH_GOOGLE_CLIENT_ID",
		"CLIGREP_AUTH_GOOGLE_CLIENT_SECRET",
		"CLIGREP_AUTH_GOOGLE_REDIRECT_URL",
		"CLIGREP_AUTH_GOOGLE_SUCCESS_URL",
		"CLIGREP_AUTH_GOOGLE_FAILURE_URL",
		"CLIGREP_AUTH_SESSION_TTL_HOURS",
		"CLIGREP_AUTH_COOKIE_NAME",
		"CLIGREP_AUTH_COOKIE_SECURE",
		"CLIGREP_AUTH_COOKIE_DOMAIN",
		"CLIGREP_AUTH_COOKIE_SAMESITE",
	} {
		if err := os.Unsetenv(key); err != nil {
			t.Fatalf("unset %s: %v", key, err)
		}
	}

	cfg := Load()

	if cfg.DBHost != "" {
		t.Fatalf("expected empty db host, got %q", cfg.DBHost)
	}
	if cfg.DBPort != 0 {
		t.Fatalf("expected db port 0, got %d", cfg.DBPort)
	}
	if cfg.GoogleRedirect != "" {
		t.Fatalf("expected empty redirect url, got %q", cfg.GoogleRedirect)
	}
	if cfg.AuthSuccessURL != "" {
		t.Fatalf("expected empty success url, got %q", cfg.AuthSuccessURL)
	}
	if cfg.AuthFailureURL != "" {
		t.Fatalf("expected empty failure url, got %q", cfg.AuthFailureURL)
	}
	if cfg.AuthCookieName != "cligrep_session" {
		t.Fatalf("unexpected default cookie name %q", cfg.AuthCookieName)
	}
	if cfg.SessionTTL != 168*time.Hour {
		t.Fatalf("unexpected default session ttl %s", cfg.SessionTTL)
	}
	if cfg.AuthCookieSameSite != http.SameSiteLaxMode {
		t.Fatalf("unexpected default same site %v", cfg.AuthCookieSameSite)
	}
}

func TestValidateRequiresDatabaseConfig(t *testing.T) {
	cfg := Config{}

	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected validation error")
	}

	message := err.Error()
	for _, fragment := range []string{
		"CLIGREP_DB_HOST is required",
		"CLIGREP_DB_PORT must be a positive integer",
		"CLIGREP_DB_NAME is required",
		"CLIGREP_DB_USER is required",
		"CLIGREP_DB_PASSWORD is required",
	} {
		if !strings.Contains(message, fragment) {
			t.Fatalf("expected validation error to contain %q, got %q", fragment, message)
		}
	}
}

func TestValidateAcceptsConfiguredDatabase(t *testing.T) {
	cfg := Config{
		DBHost:     "db.example.internal",
		DBPort:     3306,
		DBName:     "app_database",
		DBUser:     "app_user",
		DBPassword: "example-password",
	}

	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected valid config, got %v", err)
	}
}
