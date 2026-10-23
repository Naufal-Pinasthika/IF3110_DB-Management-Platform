package auth

import (
	"context"
	"errors"
	"time"

	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/api"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/apperror"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/config"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/models"
)

// define repository func interface
type Repository interface {
	FindUserByUsername(ctx context.Context, username string) (models.User, error)
	FindUserByID(ctx context.Context, userID string) (models.User, error)
	CreateSession(ctx context.Context, session models.Session) error
	FindSessionByTokenHash(ctx context.Context, tokenHash string) (models.Session, error)
	TouchSession(ctx context.Context, sessionID string, now time.Time) error
	RotateSession(ctx context.Context, sessionID, expectedHash string, nextHash string, now time.Time, graceUntil time.Time) error
	RevokeSession(ctx context.Context, sessionID string, now time.Time) error
	RevokeUserSessions(ctx context.Context, userID string, now time.Time) error
}

type Service struct {
	Repository Repository
	Policy config.SessionConfig
	tempPasswordHash string
}

// generate api.LoginResponse
func userResponse(user models.User) api.LoginResponse{
	return api.LoginResponse{
		ID: user.ID,
		Username: user.Username,
		IsPlatformAdmin: user.IsPlatformAdmin,
	}
}

// login an account and creates a fresh session
func (s *Service) Login(ctx context.Context, input api.LoginRequest, now time.Time) (LoginResult, error) {
	// validation
	if input.Username == "" || len(input.Username) > 64 || len(input.Password) == 0 || len(input.Password) > 72 {
		return LoginResult{}, ErrInvalidCredentials
	}

	// find user via query
	user, err := s.Repository.FindUserByUsername(ctx, input.Username)
	if errors.Is(err, errNotFound) {
		// still verifies to prevent timing attack
		// https://security.stackexchange.com/questions/239054/timing-attacks-in-password-hash-comparisons
		_ = VerifyPassword(input.Password, s.tempPasswordHash)
		return  LoginResult{}, ErrInvalidCredentials
	}
	if err != nil {
		return LoginResult{}, err
	}

	// verifies its password relative to its hash
	passwordValid := VerifyPassword(input.Password, user.PasswordHash)
	if passwordValid != nil || !user.IsActive {
		return LoginResult{}, ErrInvalidCredentials
	}

	// grant token session
	sessionToken, err := newToken()
	if err != nil {
		return LoginResult{}, err
	}

	csrfToken, err := newToken()
	if err != nil {
		return LoginResult{}, err
	}

	// inject session to db
	session := models.Session{
		UserID: user.ID,
		TokenHash: tokenHash(sessionToken),
		CSRFTokenHash: tokenHash(csrfToken),
		CreatedAt: now,
		RotatedAt: now,
		LastSeenAt: now,
		AbsoluteExpiresAt: now.Add(s.Policy.AbsoluteTimeout),
	}

	errCreateSession := s.Repository.CreateSession(ctx, session)
	if errCreateSession != nil {
		return LoginResult{}, errCreateSession
	}

	// return LoginResult successfully
	return LoginResult{
		User: userResponse(user),
		Session: session,
		SessionToken: sessionToken,
		CSRFToken: csrfToken,
	}, nil
}

// 	returns current authenticated user.
func (s *Service) Me(ctx context.Context, actor Principal) (api.LoginResponse, error) {
	// Use validated principal and confirm that its user is still active.
	user, err := s.Repository.FindUserByID(ctx, actor.User.ID)
	if errors.Is(err, errNotFound) {
		return api.LoginResponse{}, ErrUnauthenticated
	}
	if err != nil {
		return api.LoginResponse{}, err
	}
	if !user.IsActive {
		return api.LoginResponse{}, ErrUnauthenticated
	}

	// success
	return userResponse(user), nil
}

// ValidateSession validates cookie state on every protected request.
func (s *Service) ValidateSession(ctx context.Context, token string, now time.Time) (ValidatedSession, error) {
	// TODO Hash the token and find a current or previous token match.
	// TODO Reject revoked or idle or absolute expired sessions and inactive users.
	// TODO Accept a previous token only within grace and before absolute expiration.
	// TODO Load current user flags and check expiry before updating last activity.
	// TODO Rotate when due and return a new cookie token without extending absolute life.
	// TODO Prevent concurrent rotations from replacing each other or extending grace.
	return ValidatedSession{}, apperror.ErrNotImplemented
}

// RotateSession replaces a token while preserving the original absolute expiration.
func (s *Service) RotateSession(ctx context.Context, session models.Session, now time.Time) (string, error) {
	// TODO Generate a new random token and save its hash with the previous hash.
	// TODO Update rotated time and grace atomically with a conditional update or lock.
	// TODO Do not rotate again just because a request used the previous token.
	// TODO Return the raw token only so middleware can update the cookie.
	return "", apperror.ErrNotImplemented
}

// Logout revokes a session on the server.
func (s *Service) Logout(ctx context.Context, actor Principal, now time.Time) error {
	// TODO Revoke the validated session before the handler expires its cookie.
	// TODO Make an already revoked session harmless without reviving it.
	return apperror.ErrNotImplemented
}

// RevokeUserSessions invalidates all sessions after an account access change.
func (s *Service) RevokeUserSessions(ctx context.Context, userID string, now time.Time) error {
	// TODO Revoke all active sessions for the user on the backend.
	// TODO Require a fresh login and new token after a privilege increase.
	return apperror.ErrNotImplemented
}

// ValidateCSRF checks a request token against its session.
func (s *Service) ValidateCSRF(ctx context.Context, session models.Session, token string) error {
	// TODO Reject missing tokens and compare token hashes in constant time.
	// TODO Use a separate CSRF token rather than exposing the session cookie value.
	return apperror.ErrNotImplemented
}
