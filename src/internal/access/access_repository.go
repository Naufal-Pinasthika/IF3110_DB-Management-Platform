package access

import (
	"context"

	"gorm.io/gorm"

	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/apperror"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/models"
)

type PostgresRepository struct {
	DB *gorm.DB
}

var _ Repository = (*PostgresRepository)(nil)

// loads grants across all current groups for a specific user and database.
func (r *PostgresRepository) ListUserGrants(ctx context.Context, userID, databaseID string) ([]models.GroupAccess, error) {
	var grants []models.GroupAccess

	err := r.DB.WithContext(ctx).
		Model(&models.GroupAccess{}).
		Select("group_access.group_id, group_access.database_id, group_access.access_level").
		Joins("JOIN user_groups ON user_groups.group_id = group_access.group_id").
		Joins("JOIN users ON users.id = user_groups.user_id").
		Joins("JOIN managed_databases ON managed_databases.id = group_access.database_id").
		Where("users.id = ? AND users.is_active = ?", userID, true).
		Where("managed_databases.id = ? AND managed_databases.is_enabled = ?", databaseID, true).
		Find(&grants).Error

	if err != nil {
		return nil, err
	}

	return grants, nil
}

// lists distinct enabled targets with at least one group grant for the user.
func (r *PostgresRepository) ListGrantedDatabases(ctx context.Context, userID string) ([]models.ManagedDatabase, error) {
	var databases []models.ManagedDatabase

	err := r.DB.WithContext(ctx).
		Model(&models.ManagedDatabase{}).
		Select("DISTINCT managed_databases.id, managed_databases.engine, managed_databases.label, managed_databases.host, managed_databases.port, managed_databases.database_name, managed_databases.ssl_mode, managed_databases.reader_username, managed_databases.reader_password_enc, managed_databases.migrator_username, managed_databases.migrator_password_enc, managed_databases.is_enabled").
		Joins("JOIN group_access ON group_access.database_id = managed_databases.id").
		Joins("JOIN user_groups ON user_groups.group_id = group_access.group_id").
		Joins("JOIN users ON users.id = user_groups.user_id").
		Where("users.id = ? AND users.is_active = ?", userID, true).
		Where("managed_databases.is_enabled = ?", true).
		Find(&databases).Error

	if err != nil {
		return nil, err
	}

	return databases, nil
}

// lists every enabled target for platform administrators.
func (r *PostgresRepository) ListEnabledDatabases(ctx context.Context) ([]models.ManagedDatabase, error) {
	var databases []models.ManagedDatabase
	err := r.DB.WithContext(ctx).
		Model(&models.ManagedDatabase{}).
		Select("id, label, database_name, is_enabled").
		Where("is_enabled = ?", true).
		Order("label, id").
		Find(&databases).Error
	return databases, err
}

// loads safe target metadata (public fields only).
func (r *PostgresRepository) GetDatabaseSummary(ctx context.Context, databaseID string) (models.ManagedDatabase, error) {
	var db models.ManagedDatabase

	err := r.DB.WithContext(ctx).
		Where("id = ? AND is_enabled = ?", databaseID, true).
		First(&db).Error

	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return models.ManagedDatabase{}, apperror.ErrNotImplemented
		}
		return models.ManagedDatabase{}, err
	}

	return db, nil
}
