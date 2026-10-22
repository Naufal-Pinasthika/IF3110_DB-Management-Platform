package auth

import (
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/api"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/models"
)

// Principal comes from a validated session rather than request input.
type Principal struct {
	User      api.LoginResponse
	SessionID string `json:"-"`
}

// LoginResult carries secrets only between the service and cookie writer.
type LoginResult struct {
	User         api.LoginResponse
	Session      models.Session
	SessionToken string `json:"-"`
	CSRFToken    string `json:"-"`
}

// ValidatedSession tells middleware whether the cookie needs a new token.
type ValidatedSession struct {
	Principal    Principal
	Session      models.Session
	RotatedToken string `json:"-"`
}
