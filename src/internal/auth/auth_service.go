package auth

import (
	"context"
	"crypto/subtle"
	"errors"
	"time"

	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/api"
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

// this section mostly helper methods

// returns api.LoginResponse
func userResponse(user models.User) api.LoginResponse {
	return api.LoginResponse{
		ID: user.ID,
		Username: user.Username,
		IsPlatformAdmin: user.IsPlatformAdmin,
	}
}

// returns now + gracePeriod, unless already capped by session.AbsoluteExpiresAt
func (s *Service) graceUntil (session models.Session, now time.Time) time.Time {
	until := now.Add(s.Policy.GracePeriod)

	if until.After(session.AbsoluteExpiresAt) {
		return session.AbsoluteExpiresAt
	}
	return  until
}


// --------------------------------------------
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
	// length base64 char token validation
	if len(token) != 43 {
		return ValidatedSession{}, ErrUnauthenticated
	}
	// hash token and find a current or previous token match
	hash := tokenHash(token)

	// concurrency control on hash token validation to prevent race condition
	for attempt := 0; attempt < 2; attempt++ {
		session, err := s.Repository.FindSessionByTokenHash(ctx, hash)
		if errors.Is(err, errNotFound) {
			return ValidatedSession{}, ErrUnauthenticated
		}
		if err != nil {
			return ValidatedSession{}, err
		}

		// check if session revokes, expired, or idle timeout
		if session.RevokedAt != nil || !now.Before(session.AbsoluteExpiresAt) || now.Before(session.LastSeenAt.Add(s.Policy.IdleTimeout)) {
			return ValidatedSession{}, ErrUnauthenticated
		}

		// check if current hash session is the same as session stored in db
		// can be current session token (isCurrToken) / previous token hash (isPrevToken)
		isCurrToken := hash == session.TokenHash

		isPrevToken := session.PreviousTokenHash != nil && session.PreviousValidUntil != nil && hash == *session.PreviousTokenHash && now.Before(*session.PreviousValidUntil)

		if !isCurrToken && !isPrevToken {
			return ValidatedSession{}, ErrUnauthenticated
		}

		// find user from session (since session attached its user id)
		user, err := s.Repository.FindUserByID(ctx, session.UserID)
		if errors.Is(err, errNotFound) {
			return ValidatedSession{}, ErrUnauthenticated
		}
		if err != nil {
			return ValidatedSession{}, err
		}
		if !user.IsActive {
			return ValidatedSession{}, ErrUnauthenticated
		}

		var rotatedToken string

		// check if its overdue to token rotation
		rotationDue := !now.Before(session.RotatedAt.Add(s.Policy.RotationInterval))

		// (apply only to isCurrToken and the token is rotationDue)
		// if theres a 2 request (even at a short period amount of time, as long its timestamp different)
		// this block below will not be executed (as it will counted as isPrevToken)
		if isCurrToken && rotationDue {
			rotatedToken, err := s.RotateSession(ctx, session, now)
			if errors.Is(err, errRotationConflict) {
				continue // avoid race condition if theres 2 token requested simultaneously (same timestamp)
			}
			if err != nil {
				return ValidatedSession{}, err
			}

			// gives old token into PreviousTokenHash 
			// also add PreviousValidUntil relatives to its added grace period
			previousHash := session.TokenHash
			graceUntill := s.graceUntil(session, now)

			session.PreviousTokenHash = &previousHash
			session.PreviousValidUntil = &graceUntill

			// provides a new token
			session.TokenHash = tokenHash(rotatedToken)
			session.RotatedAt = now
		}

		// update session
		errTouchSession := s.Repository.TouchSession(ctx, session.ID, now)
		if errTouchSession != nil {
			return ValidatedSession{}, errTouchSession
		}

		if now.After(session.LastSeenAt){
			session.LastSeenAt = now
		}

		// return success
		return ValidatedSession{
			Principal: Principal{
				User: userResponse(user),
				SessionID: session.ID,
			},
			Session: session,
			RotatedToken: rotatedToken,
		}, nil
	}

	// if race condition still keeps happening after more than 2 attems, just return conflict
	return ValidatedSession{}, errRotationConflict
}

// replaces a token while preserving the original absolute expiration
// this func is basically abstraction wrapper from the db query func RotateSession() on auth_repository
func (s *Service) RotateSession(ctx context.Context, session models.Session, now time.Time) (string, error) {
	// generate a new random token and save its hash with the previous hash
	token, err := newToken()
	if err != nil  {
		return "", err
	}

	// update rotated time and grace atomically with a conditional update or lock
	// uses RotateSession() from repository
	errRotateSession := s.Repository.RotateSession(ctx, session.ID, session.TokenHash, tokenHash(token), now, s.graceUntil(session, now))
	if errRotateSession != nil {
		return "", errRotateSession
	}

	return token, nil
}

// revoke a session on the server
// basically a wrapper from RevokeSession() on auth_repository 
func (s *Service) Logout(ctx context.Context, actor Principal, now time.Time) error {
	return s.Repository.RevokeSession(ctx, actor.SessionID, now)
}

// invalidates all sessions after an account access change
// basically a wrapper from RevokeUserSession() on auth_repository
func (s *Service) RevokeUserSessions(ctx context.Context, userID string, now time.Time) error {
	return s.Repository.RevokeUserSessions(ctx, userID, now)
}

// checkscsrd token
func (s *Service) ValidateCSRF(ctx context.Context, session models.Session, token string) error {
	if len(token) != 43 {
		return ErrCSRF
	}

	actual := tokenHash(token)

	if subtle.ConstantTimeCompare([]byte(actual), []byte(session.CSRFTokenHash)) != 1 {
		return ErrCSRF
	}

	return nil
}
