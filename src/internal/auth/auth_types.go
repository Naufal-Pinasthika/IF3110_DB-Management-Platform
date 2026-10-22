package auth

import (
	"time"

	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/api"
)

// UserRecord adds the private password hash to the existing API user type.
// Handlers return User only and never serialize the whole record.
type UserRecord struct {
	User         api.AdminUser
	PasswordHash string `json:"-"`
}

// Principal comes from a validated session rather than request input.
type Principal struct {
	User      api.LoginResponse
	SessionID string `json:"-"`
}

// Session is backend state that does not exist in the public API contract.
// Raw session tokens never belong in this record.
type Session struct {
	ID                 string
	UserID             string
	TokenHash          string  `json:"-"`
	PreviousTokenHash  *string `json:"-"`
	PreviousValidUntil *time.Time
	CSRFTokenHash      string `json:"-"`
	CreatedAt          time.Time
	RotatedAt          time.Time
	LastSeenAt         time.Time
	AbsoluteExpiresAt  time.Time
	RevokedAt          *time.Time
}

// LoginResult carries secrets only between the service and cookie writer.
type LoginResult struct {
	User         api.LoginResponse
	Session      Session
	SessionToken string `json:"-"`
	CSRFToken    string `json:"-"`
}

// ValidatedSession tells middleware whether the cookie needs a new token.
type ValidatedSession struct {
	Principal    Principal
	Session      Session
	RotatedToken string `json:"-"`
}
