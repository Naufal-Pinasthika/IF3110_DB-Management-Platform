package auth

import (
	"context"
	"crypto/subtle"
	"errors"
	"log/slog"
	"time"

	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/api"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/config"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/models"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/telemetry"
)

// define repository func interface
type Repository interface {
	FindUserByUsername(ctx context.Context, username string) (models.User, error)
	FindUserByID(ctx context.Context, userID string) (models.User, error)
	CreateSession(ctx context.Context, session *models.Session) error
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

func NewService(repo Repository, policy config.SessionConfig) (*Service, error) {
	dummyHash, err := HashPassword("dummy-password-for-timing")
	if err != nil {
		return nil, err
	}
	return &Service{Repository: repo, Policy: policy, tempPasswordHash: dummyHash}, nil
}

func userResponse(user models.User) api.LoginResponse {
	return api.LoginResponse{
		ID: user.ID,
		Username: user.Username,
		IsPlatformAdmin: user.IsPlatformAdmin,
	}
}

// caps grace period by absolute expiry
func (s *Service) graceUntil (session models.Session, now time.Time) time.Time {
	until := now.Add(s.Policy.GracePeriod)

	if until.After(session.AbsoluteExpiresAt) {
		return session.AbsoluteExpiresAt
	}
	return  until
}

func (s *Service) Login(ctx context.Context, input api.LoginRequest, now time.Time) (LoginResult, error) {
	if input.Username == "" || len(input.Username) > 64 || len(input.Password) == 0 || len(input.Password) > 72 {
		return LoginResult{}, ErrInvalidCredentials
	}

	user, err := s.Repository.FindUserByUsername(ctx, input.Username)
	if errors.Is(err, errNotFound) {
		_ = VerifyPassword(input.Password, s.tempPasswordHash)
		telemetry.Log(ctx, slog.LevelWarn, "auth.login_failed", "username", input.Username, "status", "failed", "reason", "invalid_credentials")
		return  LoginResult{}, ErrInvalidCredentials
	}
	if err != nil {
		telemetry.Log(ctx, slog.LevelError, "auth.login_failed", "username", input.Username, "status", "error", "reason", "repository_error")
		return LoginResult{}, err
	}

	passwordValid := VerifyPassword(input.Password, user.PasswordHash)
	if passwordValid != nil || !user.IsActive {
		reason := "invalid_credentials"
		if passwordValid == nil && !user.IsActive {
			reason = "inactive_account"
		}
		telemetry.Log(ctx, slog.LevelWarn, "auth.login_failed", "actor", user.ID, "username", user.Username, "status", "failed", "reason", reason)
		return LoginResult{}, ErrInvalidCredentials
	}

	sessionToken, err := newToken()
	if err != nil {
		return LoginResult{}, err
	}

	csrfToken, err := newToken()
	if err != nil {
		return LoginResult{}, err
	}

	session := models.Session{
		UserID: user.ID,
		TokenHash: tokenHash(sessionToken),
		CSRFTokenHash: tokenHash(csrfToken),
		CreatedAt: now,
		RotatedAt: now,
		LastSeenAt: now,
		AbsoluteExpiresAt: now.Add(s.Policy.AbsoluteTimeout),
	}

	errCreateSession := s.Repository.CreateSession(ctx, &session)
	if errCreateSession != nil {
		telemetry.Log(ctx, slog.LevelError, "auth.login_failed", "actor", user.ID, "status", "error", "reason", "session_create_failed")
		return LoginResult{}, errCreateSession
	}

	telemetry.Log(ctx, slog.LevelInfo, "auth.login_succeeded", "actor", user.ID, "username", user.Username, "status", "success")

	return LoginResult{
		User: userResponse(user),
		Session: session,
		SessionToken: sessionToken,
		CSRFToken: csrfToken,
	}, nil
}

func (s *Service) Me(ctx context.Context, actor Principal) (api.LoginResponse, error) {
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

	return userResponse(user), nil
}

func (s *Service) ValidateSession(ctx context.Context, token string, now time.Time) (ValidatedSession, error) {
	if len(token) != 43 {
		return ValidatedSession{}, ErrUnauthenticated
	}
	hash := tokenHash(token)

	for attempt := 0; attempt < 2; attempt++ {
		session, err := s.Repository.FindSessionByTokenHash(ctx, hash)
		if errors.Is(err, errNotFound) {
			return ValidatedSession{}, ErrUnauthenticated
		}
		if err != nil {
			return ValidatedSession{}, err
		}

		if session.RevokedAt != nil || !now.Before(session.AbsoluteExpiresAt) || !now.Before(session.LastSeenAt.Add(s.Policy.IdleTimeout)) {
			return ValidatedSession{}, ErrUnauthenticated
		}

		isCurrToken := hash == session.TokenHash

		isPrevToken := session.PreviousTokenHash != nil && session.PreviousValidUntil != nil && hash == *session.PreviousTokenHash && now.Before(*session.PreviousValidUntil)

		if !isCurrToken && !isPrevToken {
			return ValidatedSession{}, ErrUnauthenticated
		}

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

		rotationDue := !now.Before(session.RotatedAt.Add(s.Policy.RotationInterval))

		if isCurrToken && rotationDue {
			rotatedToken, err = s.RotateSession(ctx, session, now)
			if errors.Is(err, errRotationConflict) {
				continue
			}
			if err != nil {
				return ValidatedSession{}, err
			}

			previousHash := session.TokenHash
			graceUntill := s.graceUntil(session, now)

			session.PreviousTokenHash = &previousHash
			session.PreviousValidUntil = &graceUntill

			session.TokenHash = tokenHash(rotatedToken)
			session.RotatedAt = now
		}

		errTouchSession := s.Repository.TouchSession(ctx, session.ID, now)
		if errTouchSession != nil {
			return ValidatedSession{}, errTouchSession
		}

		if now.After(session.LastSeenAt){
			session.LastSeenAt = now
		}

		return ValidatedSession{
			Principal: Principal{
				User: userResponse(user),
				SessionID: session.ID,
			},
			Session: session,
			RotatedToken: rotatedToken,
		}, nil
	}

	return ValidatedSession{}, errRotationConflict
}

func (s *Service) RotateSession(ctx context.Context, session models.Session, now time.Time) (string, error) {
	token, err := newToken()
	if err != nil  {
		return "", err
	}

	errRotateSession := s.Repository.RotateSession(ctx, session.ID, session.TokenHash, tokenHash(token), now, s.graceUntil(session, now))
	if errRotateSession != nil {
		return "", errRotateSession
	}

	return token, nil
}

func (s *Service) Logout(ctx context.Context, actor Principal, now time.Time) error {
	err := s.Repository.RevokeSession(ctx, actor.SessionID, now)
	if err != nil {
		telemetry.Log(ctx, slog.LevelError, "auth.logout_failed", "actor", actor.User.ID, "status", "failed")
		return err
	}
	telemetry.Log(ctx, slog.LevelInfo, "auth.logout_succeeded", "actor", actor.User.ID, "status", "success")
	return nil
}

func (s *Service) RevokeUserSessions(ctx context.Context, userID string, now time.Time) error {
	return s.Repository.RevokeUserSessions(ctx, userID, now)
}

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
