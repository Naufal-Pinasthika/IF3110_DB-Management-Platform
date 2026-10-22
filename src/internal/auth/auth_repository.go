package auth

import (
	"context"
	"time"

	"gorm.io/gorm"

	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/apperror"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/models"
)

// PostgresRepository uses GORM models and leaves HTTP mapping to the service.
type PostgresRepository struct {
	DB *gorm.DB
}

var _ Repository = (*PostgresRepository)(nil)

// FindUserByUsername loads an account for password verification.
func (r *PostgresRepository) FindUserByUsername(ctx context.Context, username string) (models.User, error) {
	// TODO Query models.User with WithContext and a bound username.
	// TODO Map gorm.ErrRecordNotFound to a domain error.
	return models.User{}, apperror.ErrNotImplemented
}

// FindUserByID loads current account flags for session checks.
func (r *PostgresRepository) FindUserByID(ctx context.Context, userID string) (models.User, error) {
	// TODO Read users by ID on each validation so inactive accounts lose access.
	return models.User{}, apperror.ErrNotImplemented
}

// CreateSession persists a new session using hashes only.
func (r *PostgresRepository) CreateSession(ctx context.Context, session models.Session) error {
	// TODO Create models.Session with hashes only and the supplied timestamps.
	return apperror.ErrNotImplemented
}

// FindSessionByTokenHash finds either token generation for later policy checks.
func (r *PostgresRepository) FindSessionByTokenHash(ctx context.Context, tokenHash string) (models.Session, error) {
	// TODO Look up token_hash or previous_token_hash and preserve nullable fields.
	// TODO Let the service check revocation and all expiration rules.
	return models.Session{}, apperror.ErrNotImplemented
}

// TouchSession updates activity after validation.
func (r *PostgresRepository) TouchSession(ctx context.Context, sessionID string, now time.Time) error {
	// TODO Update last_seen_at for an active session without moving time backwards.
	// TODO Never change absolute_expires_at.
	return apperror.ErrNotImplemented
}

// RotateSession atomically replaces the active token hash.
func (r *PostgresRepository) RotateSession(ctx context.Context, sessionID, expectedHash, nextHash string, now, graceUntil time.Time) error {
	// TODO Use expectedHash in a conditional update or lock the session row.
	// TODO Move the old hash to previous and save grace and rotated time.
	// TODO Report a conflict if another request already rotated or revoked the session.
	return apperror.ErrNotImplemented
}

// RevokeSession invalidates one session.
func (r *PostgresRepository) RevokeSession(ctx context.Context, sessionID string, now time.Time) error {
	// TODO Set revoked_at without deleting audit state or exposing token values.
	return apperror.ErrNotImplemented
}

// RevokeUserSessions invalidates every active session for a user.
func (r *PostgresRepository) RevokeUserSessions(ctx context.Context, userID string, now time.Time) error {
	// TODO Update revoked_at for all matching active sessions.
	return apperror.ErrNotImplemented
}
