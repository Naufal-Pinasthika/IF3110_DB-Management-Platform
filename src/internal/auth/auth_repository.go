package auth

import (
	"context"
	"time"
	"gorm.io/gorm"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/models"
)

// PostgresRepository uses GORM models and leaves HTTP mapping to the service.
type PostgresRepository struct {
	DB *gorm.DB
}

// FindUserByUsername loads an account for password verification.
func (r *PostgresRepository) FindUserByUsername(ctx context.Context, username string) (models.User, error) {
	var user models.User

	// Query WHERE username into DB via gorm & pass it to &user
	err := r.DB.WithContext(ctx).Where("username = ?", username).First(&user).Error

	// return empty model and err if failed
	if err != nil {
		return models.User{}, errNotFound
	}

	return user, nil
}

// FindUserByID loads current account flags for session checks.
func (r *PostgresRepository) FindUserByID(ctx context.Context, userID string) (models.User, error) {
	var user models.User
	// Query WHERE id into DB via gorm & pass it to &user
	err := r.DB.WithContext(ctx).Where("id = ?", userID).First(&user).Error

	// return empty model and err if failed
	if err != nil {
		return  models.User{}, errNotFound
	}

	return user, nil
}

// CreateSession persists a new session using hashes only.
func (r *PostgresRepository) CreateSession(ctx context.Context, session *models.Session) error {
	// create atomic Session without creating duplicate User table (FK) using .Omit()
	// https://stackoverflow.com/questions/65363974/
	
	// psql will generate Session uuid automatically
	err := r.DB.WithContext(ctx).Omit("User").Create(session).Error
	if err != nil {
		return err
	}

	// CreateSession process succeed
	return nil
}

// Dinds either token generation for later policy checks
func (r *PostgresRepository) FindSessionByTokenHash(ctx context.Context, tokenHash string) (models.Session, error) {
	var session models.Session

	// Query WHERE token_hash or previous_token_hash 
	err := r.DB.WithContext(ctx).Where("token_hash = ? OR previous_token_hash = ?", tokenHash, tokenHash).First(&session).Error

	// return empty model and err if failed
	if err != nil {
		return  models.Session{}, errNotFound
	}

	return session, nil
}

// updates activity after validation
func (r *PostgresRepository) TouchSession(ctx context.Context, sessionID string, now time.Time) error {
	// Update last_seen_at for an active session (SessionID present & absolute_expires_at > now)
	result := r.DB.WithContext(ctx).Model(&models.Session{}).
				Where("id = ? and absolute_expires_at > ?", sessionID, now).
				// prevent race condition using GREATEST() expr (ensure the most recent is updated)
				Update("last_seen_at", gorm.Expr("GREATEST(last_seen_at, ?)", now))
	
	// return error if failed
	if result.Error != nil {
		return result.Error
	}
	// return error if no rows affected (because session have been revoked/expired)
	if result.RowsAffected != 1 {
		return ErrUnauthenticated
	}

    // TouchSession process succeed
	return nil	
}

// atomically replaces the active token hash
func (r *PostgresRepository) RotateSession(ctx context.Context, sessionID string, expectedHash string, nextHash string, now, graceUntil time.Time) error {
	// Query WHERE SessionID present, token_hash = expectedHash, revoked_at NULL, absolute_expires > now (session still up)
	result := r.DB.WithContext(ctx).Model(&models.Session{}).
				Where("id = ? AND token_hash = ? AND revoked_at IS NULL AND absolute_expires > ?", sessionID, expectedHash, now).
				// update token_hash, previous_token_hash, previous_valid_untul, rotated_at
				Updates(map[string]any{
					"token_hash" : nextHash, 
					"previous_token_hash" : expectedHash,
					"previous_valid_until" : graceUntil,
					"rotated_at" : now,
				})
				
	// return error if failed
	if result.Error != nil {
		return result.Error
	}
	// return error if no rows affected (because session have been revoked/expired)
	if result.RowsAffected != 1 {
		return ErrUnauthenticated
	}	

	// RotateSession process succeed
	return nil
}

// invalidates one session
func (r *PostgresRepository) RevokeSession(ctx context.Context, sessionID string, now time.Time) error {
	// Query WHERE username into DB via gorm & pass it to &user
	err := r.DB.WithContext(ctx).Model(&models.Session{}).
			Where("id = ? AND revoked_at IS NULL", sessionID).
			Update("revoked_at", now).Error

	if err != nil {
		return err
	}

	// RevokeSession process succeed
	return nil
}

// same as RevokeSession but for user_id
func (r *PostgresRepository) RevokeUserSessions(ctx context.Context, userID string, now time.Time) error {
	// Query WHERE username into DB via gorm & pass it to &user
	err := r.DB.WithContext(ctx).Model(&models.Session{}).
			Where("id = ? AND revoked_at IS NULL", userID).
			Update("revoked_at", now).Error

	if err != nil {
		return err
	}

	// RevokeUserSession process succeed
	return nil
}
