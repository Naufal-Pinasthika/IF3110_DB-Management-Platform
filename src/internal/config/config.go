package config

import (
	"encoding/base64"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"time"

	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/apperror"
)

type Config struct {
	AppAddr                   string
	PublicOrigin              string
	PlatformDatabaseURL       string `json:"-"`
	DBCredentialEncryptionKey []byte `json:"-"`
	Session                   SessionConfig
	QueryTimeout              time.Duration
	QueryMaxRows              int
}

type SessionConfig struct {
	CookieName       string
	CookieSecure     bool
	IdleTimeout      time.Duration
	AbsoluteTimeout  time.Duration
	RotationInterval time.Duration
	GracePeriod      time.Duration
}

// Load reads environment settings without logging secrets.
func Load() (Config, error) {
	cfg := Config{
		AppAddr:             envOrDefault("APP_ADDR", ":8080"),
		PublicOrigin:        envOrDefault("PUBLIC_ORIGIN", "http://localhost:28080"),
		PlatformDatabaseURL: os.Getenv("PLATFORM_DATABASE_URL"),
		Session:             SessionConfig{CookieName: "sid"},
		QueryTimeout:        5 * time.Second,
		QueryMaxRows:        100,
	}
	var err error
	cfg.Session.CookieSecure, err = strconv.ParseBool(envOrDefault("SESSION_COOKIE_SECURE", "false"))
	if err != nil {
		return Config{}, fmt.Errorf("SESSION_COOKIE_SECURE must be a boolean")
	}
	for _, setting := range []struct {
		name     string
		fallback string
		target   *time.Duration
	}{
		{"SESSION_IDLE_TIMEOUT", "30m", &cfg.Session.IdleTimeout},
		{"SESSION_ABSOLUTE_TIMEOUT", "24h", &cfg.Session.AbsoluteTimeout},
		{"SESSION_ROTATION_INTERVAL", "15m", &cfg.Session.RotationInterval},
		{"SESSION_GRACE_PERIOD", "30s", &cfg.Session.GracePeriod},
	} {
		*setting.target, err = time.ParseDuration(envOrDefault(setting.name, setting.fallback))
		if err != nil {
			return Config{}, fmt.Errorf("%s must be a duration", setting.name)
		}
	}

	// Database credential encryption key (base64-encoded 32-byte key for AES-256-GCM)
	encKeyB64 := os.Getenv("DB_CREDENTIAL_ENCRYPTION_KEY")
	if encKeyB64 == "" {
		return Config{}, apperror.New("DB_CREDENTIAL_ENCRYPTION_KEY is required")
	}
	encKey, err := base64.StdEncoding.DecodeString(encKeyB64)
	if err != nil {
		return Config{}, apperror.New("DB_CREDENTIAL_ENCRYPTION_KEY must be valid base64")
	}
	if len(encKey) != 32 {
		return Config{}, apperror.New("DB_CREDENTIAL_ENCRYPTION_KEY must decode to 32 bytes (256 bits)")
	}
	cfg.DBCredentialEncryptionKey = encKey

	return cfg, cfg.Validate()
}

// Validate rejects invalid settings before serving requests.
func (c Config) Validate() error {
	if c.AppAddr == "" || c.PlatformDatabaseURL == "" {
		return fmt.Errorf("APP_ADDR and PLATFORM_DATABASE_URL are required")
	}
	origin, err := url.Parse(c.PublicOrigin)
	if err != nil || (origin.Scheme != "http" && origin.Scheme != "https") || origin.Hostname() == "" || origin.User != nil || origin.Path != "" || origin.RawQuery != "" || origin.Fragment != "" {
		return fmt.Errorf("PUBLIC_ORIGIN must be an HTTP origin without a path")
	}
	if origin.Scheme == "https" && !c.Session.CookieSecure {
		return fmt.Errorf("SESSION_COOKIE_SECURE must be true for HTTPS")
	}
	if len(c.DBCredentialEncryptionKey) != 32 {
		return fmt.Errorf("DB_CREDENTIAL_ENCRYPTION_KEY must be 32 bytes")
	}
	s := c.Session
	if s.CookieName == "" || s.IdleTimeout <= 0 || s.GracePeriod <= 0 || s.RotationInterval <= s.GracePeriod || s.AbsoluteTimeout <= s.RotationInterval {
		return fmt.Errorf("session durations must be positive with grace < rotation < absolute")
	}
	return nil
}

func envOrDefault(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
