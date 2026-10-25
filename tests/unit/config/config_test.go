package config_test

import (
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/config"
	"strings"
	"testing"
	"time"
)

func configEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{"APP_ADDR", "PUBLIC_ORIGIN", "SESSION_COOKIE_SECURE", "SESSION_IDLE_TIMEOUT", "SESSION_ABSOLUTE_TIMEOUT", "SESSION_ROTATION_INTERVAL", "SESSION_GRACE_PERIOD"} {
		t.Setenv(key, "")
	}
	t.Setenv("PLATFORM_DATABASE_URL", "postgres://user:***@localhost/platform_db")
	t.Setenv("DB_CREDENTIAL_ENCRYPTION_KEY", "testYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWE=") // base64 of 32 'a' bytes
}

func TestLoadDefaults(t *testing.T) {
	configEnv(t)
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AppAddr != ":8080" || cfg.PublicOrigin != "http://localhost:28080" {
		t.Fatalf("invalid HTTP defaults: %+v", cfg)
	}
	if cfg.Session.CookieName != "sid" || cfg.Session.CookieSecure {
		t.Fatal("invalid local cookie settings")
	}
	if cfg.Session.IdleTimeout != 30*time.Minute || cfg.Session.AbsoluteTimeout != 24*time.Hour || cfg.Session.RotationInterval != 15*time.Minute || cfg.Session.GracePeriod != 30*time.Second {
		t.Fatalf("invalid session defaults: %+v", cfg.Session)
	}
}

func TestLoadRejectsInvalidSettings(t *testing.T) {
	for _, tc := range []struct{ key, value string }{
		{"PLATFORM_DATABASE_URL", ""},
		{"PUBLIC_ORIGIN", "invalid"},
		{"PUBLIC_ORIGIN", "http://localhost/path"},
		{"PUBLIC_ORIGIN", "https://example.com"}, // HTTPS requires Secure cookies.
		{"SESSION_COOKIE_SECURE", "invalid"},
		{"SESSION_IDLE_TIMEOUT", "invalid"},
		{"SESSION_IDLE_TIMEOUT", "0s"},
		{"SESSION_GRACE_PERIOD", "15m"},
		{"SESSION_ROTATION_INTERVAL", "24h"},
	} {
		t.Run(tc.key+"="+tc.value, func(t *testing.T) {
			configEnv(t)
			t.Setenv(tc.key, tc.value)
			_, err := config.Load()
			if err == nil {
				t.Fatal("invalid settings accepted")
			}
			if strings.Contains(err.Error(), "secret") {
				t.Fatal("configuration error exposed credentials")
			}
		})
	}
}

func TestLoadHTTPSAndSessionOverrides(t *testing.T) {
	configEnv(t)
	t.Setenv("PUBLIC_ORIGIN", "https://example.com")
	t.Setenv("SESSION_COOKIE_SECURE", "true")
	t.Setenv("SESSION_IDLE_TIMEOUT", "10m")
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Session.CookieSecure || cfg.Session.IdleTimeout != 10*time.Minute {
		t.Fatal("session overrides ignored")
	}
}
