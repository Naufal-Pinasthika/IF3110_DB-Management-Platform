// Package config reads process settings and validates them before startup.
package config

import (
	"time"

	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/apperror"
)

type Config struct {
	AppAddr                   string
	PlatformDatabaseURL       string `json:"-"`
	DBCredentialEncryptionKey []byte `json:"-"`
	Session                   SessionConfig
	QueryTimeout              time.Duration
	QueryMaxRows              int
}

// SessionConfig holds team policy rather than values required by the assignment.
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
	// TODO Read APP_ADDR and PLATFORM_DATABASE_URL from the environment.
	// TODO Read DB_CREDENTIAL_ENCRYPTION_KEY and decode the chosen key format.
	// TODO Parse SESSION_COOKIE_SECURE and all four SESSION duration settings.
	// TODO Choose and document a stable cookie name such as sid.
	// TODO Document suggested defaults of 30m idle and 24h absolute lifetime.
	// TODO Document suggested rotation of 15m and grace period of 30s.
	// TODO Set query timeout and row limits then call Validate.
	return Config{}, apperror.ErrNotImplemented
}

// Validate rejects invalid settings before serving requests.
func (c Config) Validate() error {
	// TODO Require an address and metadata DSN and a valid encryption key.
	// TODO Require positive durations and grace below rotation below absolute.
	// TODO Require positive query limits and Secure cookies on production HTTPS.
	// TODO Report setting names without including secret values in errors.
	return apperror.ErrNotImplemented
}
