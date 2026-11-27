package auth

import (
	"context"
	"errors"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/models"
	"gorm.io/gorm"
	"time"
)

func (r *PostgresRepository) GormDB() *gorm.DB {
    return r.DB
}

// PostgresRepository uses GORM models and leaves HTTP mapping to the service.
type PostgresRepository struct {
	DB *gorm.DB
}

func (r *PostgresRepository) GetDB() *gorm.DB {
	return r.DB
}

func (r *PostgresRepository) FindUserByUsername(ctx context.Context, username string) (models.User, error) {
	var user models.User

	err := r.DB.WithContext(ctx).Where("username = ?", username).First(&user).Error

	if errors.Is(err, gorm.ErrRecordNotFound) {
		return models.User{}, errNotFound
	}
	if err != nil {
		return models.User{}, err
	}

	return user, nil
}

func (r *PostgresRepository) FindUserByID(ctx context.Context, userID string) (models.User, error) {
	var user models.User
	err := r.DB.WithContext(ctx).Where("id = ?", userID).First(&user).Error

	if errors.Is(err, gorm.ErrRecordNotFound) {
		return models.User{}, errNotFound
	}
	if err != nil {
		return models.User{}, err
	}

	return user, nil
}

func (r *PostgresRepository) CreateSession(ctx context.Context, session *models.Session) error {

	err := r.DB.WithContext(ctx).Omit("User").Create(session).Error
	if err != nil {
		return err
	}

	return nil
}

func (r *PostgresRepository) FindSessionByTokenHash(ctx context.Context, tokenHash string) (models.Session, error) {
	var session models.Session

	err := r.DB.WithContext(ctx).Where("token_hash = ? OR previous_token_hash = ?", tokenHash, tokenHash).First(&session).Error

	if errors.Is(err, gorm.ErrRecordNotFound) {
		return models.Session{}, errNotFound
	}
	if err != nil {
		return models.Session{}, err
	}

	return session, nil
}

func (r *PostgresRepository) TouchSession(ctx context.Context, sessionID string, now time.Time) error {
	result := r.DB.WithContext(ctx).Model(&models.Session{}).
		Where("id = ? AND revoked_at IS NULL AND absolute_expires_at > ?", sessionID, now).
		Update("last_seen_at", gorm.Expr("GREATEST(last_seen_at, ?)", now))

	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrUnauthenticated
	}

	return nil
}

func (r *PostgresRepository) RotateSession(ctx context.Context, sessionID string, expectedHash string, nextHash string, now, graceUntil time.Time) error {
	result := r.DB.WithContext(ctx).Model(&models.Session{}).
				Where("id = ? AND token_hash = ? AND revoked_at IS NULL AND absolute_expires_at > ?", sessionID, expectedHash, now).
		Updates(map[string]any{
					"token_hash" : nextHash, 
					"previous_token_hash" : expectedHash,
					"previous_valid_until" : graceUntil,
					"rotated_at" : now,
		})

	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return errRotationConflict
	}

	return nil
}

func (r *PostgresRepository) RevokeSession(ctx context.Context, sessionID string, now time.Time) error {
	err := r.DB.WithContext(ctx).Model(&models.Session{}).
		Where("id = ? AND revoked_at IS NULL", sessionID).
		Update("revoked_at", now).Error

	if err != nil {
		return err
	}

	return nil
}

func (r *PostgresRepository) RevokeUserSessions(ctx context.Context, userID string, now time.Time) error {
	err := r.DB.WithContext(ctx).Model(&models.Session{}).
			Where("user_id = ? AND revoked_at IS NULL", userID).
		Update("revoked_at", now).Error

	if err != nil {
		return err
	}

	return nil
}
