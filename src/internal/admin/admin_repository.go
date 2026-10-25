package admin

import (
	"context"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/models"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/store"
)

// PostgresRepository uses GORM models and leaves HTTP mapping to the service.
type PostgresRepository struct {
	DB *gorm.DB
}

var _ Repository = (*PostgresRepository)(nil)
var _ store.DatabaseRepository = (*PostgresRepository)(nil)

// ListUsers lists users without password hashes.
func (r *PostgresRepository) ListUsers(ctx context.Context) ([]models.User, error) {
	var users []models.User
	err := r.DB.WithContext(ctx).
		Select("id", "username", "is_platform_admin", "is_active", "created_at").
		Order("created_at").
		Find(&users).Error
	return users, err
}

// GetUser loads an account for a partial update.
func (r *PostgresRepository) GetUser(ctx context.Context, userID string) (models.User, error) {
	var user models.User
	err := r.DB.WithContext(ctx).
		Where("id = ?", userID).
		First(&user).Error
	return user, err
}

// CreateUser persists a user with an already hashed password.
func (r *PostgresRepository) CreateUser(ctx context.Context, user models.User) (models.User, error) {
	err := r.DB.WithContext(ctx).Create(&user).Error
	return user, err
}

// UpdateUser updates an account and revokes sessions when required.
func (r *PostgresRepository) UpdateUser(ctx context.Context, user models.User, revokeSessions bool, now time.Time) (models.User, error) {
	err := r.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Update user using map to allow false values
		updates := map[string]interface{}{
			"username":          user.Username,
			"password_hash":     user.PasswordHash,
			"is_platform_admin": user.IsPlatformAdmin,
			"is_active":         user.IsActive,
		}
		if err := tx.Model(&models.User{}).Where("id = ?", user.ID).Updates(updates).Error; err != nil {
			return err
		}

		if revokeSessions {
			if err := tx.Model(&models.Session{}).
				Where("user_id = ? AND revoked_at IS NULL", user.ID).
				Updates(map[string]interface{}{
					"revoked_at":        now,
					"previous_token_hash": gorm.Expr("token_hash"),
					"previous_valid_until": now.Add(30 * time.Second), // grace period
				}).Error; err != nil {
				return err
			}
		}

		return nil
	})

	if err != nil {
		return models.User{}, err
	}

	// Reload to return updated user
	return r.GetUser(ctx, user.ID)
}

// ListGroups lists groups.
func (r *PostgresRepository) ListGroups(ctx context.Context) ([]models.Group, error) {
	var groups []models.Group
	err := r.DB.WithContext(ctx).
		Order("name").
		Find(&groups).Error
	return groups, err
}

// CreateGroup creates a uniquely named group.
func (r *PostgresRepository) CreateGroup(ctx context.Context, name string) (models.Group, error) {
	group := models.Group{Name: name}
	err := r.DB.WithContext(ctx).Create(&group).Error
	return group, err
}

// UpdateGroup renames a group.
func (r *PostgresRepository) UpdateGroup(ctx context.Context, groupID, name string) (models.Group, error) {
	err := r.DB.WithContext(ctx).
		Model(&models.Group{}).
		Where("id = ?", groupID).
		Update("name", name).Error
	if err != nil {
		return models.Group{}, err
	}
	var group models.Group
	err = r.DB.WithContext(ctx).Where("id = ?", groupID).First(&group).Error
	return group, err
}

// DeleteGroup removes a group and its related grants and memberships.
func (r *PostgresRepository) DeleteGroup(ctx context.Context, groupID string, now time.Time) error {
	return r.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&models.Session{}).
			Where("user_id IN (SELECT user_id FROM user_groups WHERE group_id = ?) AND revoked_at IS NULL", groupID).
			Updates(map[string]interface{}{
				"revoked_at":            now,
				"previous_token_hash":   gorm.Expr("token_hash"),
				"previous_valid_until":  now.Add(30 * time.Second),
			}).Error; err != nil {
			return err
		}

		// Delete group - FK cascades will handle user_groups and group_access
		if err := tx.Delete(&models.Group{}, "id = ?", groupID).Error; err != nil {
			return err
		}

		return nil
	})
}

// AddGroupMember adds a membership without duplicates.
func (r *PostgresRepository) AddGroupMember(ctx context.Context, groupID, userID string, now time.Time) (models.UserGroup, error) {
	var ug models.UserGroup
	err := r.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.First(&models.Group{}, "id = ?", groupID).Error; err != nil {
			return err
		}
		if err := tx.First(&models.User{}, "id = ?", userID).Error; err != nil {
			return err
		}

		ug = models.UserGroup{UserID: userID, GroupID: groupID}
		if err := tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "user_id"}, {Name: "group_id"}},
			DoNothing: true,
		}).Create(&ug).Error; err != nil {
			return err
		}

		if err := tx.Model(&models.Session{}).
			Where("user_id = ? AND revoked_at IS NULL", userID).
			Updates(map[string]interface{}{
				"revoked_at":            now,
				"previous_token_hash":   gorm.Expr("token_hash"),
				"previous_valid_until":  now.Add(30 * time.Second),
			}).Error; err != nil {
			return err
		}

		return nil
	})
	return ug, err
}

// RemoveGroupMember removes one membership.
func (r *PostgresRepository) RemoveGroupMember(ctx context.Context, groupID, userID string, now time.Time) error {
	return r.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Delete the membership
		if err := tx.Where("user_id = ? AND group_id = ?", userID, groupID).
			Delete(&models.UserGroup{}).Error; err != nil {
			return err
		}

		// Revoke affected user sessions
		if err := tx.Model(&models.Session{}).
			Where("user_id = ? AND revoked_at IS NULL", userID).
			Updates(map[string]interface{}{
				"revoked_at":            now,
				"previous_token_hash":   gorm.Expr("token_hash"),
				"previous_valid_until":  now.Add(30 * time.Second),
			}).Error; err != nil {
			return err
		}

		return nil
	})
}

// ListDatabases lists registered targets without secrets.
func (r *PostgresRepository) ListDatabases(ctx context.Context) ([]models.ManagedDatabase, error) {
	var dbs []models.ManagedDatabase
	err := r.DB.WithContext(ctx).
		Select("id", "label", "host", "port", "database_name", "ssl_mode", "reader_username", "reader_password_enc", "migrator_username", "migrator_password_enc", "is_enabled").
		Order("label").
		Find(&dbs).Error
	return dbs, err
}

// GetDatabase loads a target with encrypted credentials for internal use.
func (r *PostgresRepository) GetDatabase(ctx context.Context, databaseID string) (models.ManagedDatabase, error) {
	var db models.ManagedDatabase
	err := r.DB.WithContext(ctx).
		Where("id = ?", databaseID).
		First(&db).Error
	return db, err
}

// CreateDatabase stores a target with encrypted credentials.
func (r *PostgresRepository) CreateDatabase(ctx context.Context, database models.ManagedDatabase) (models.ManagedDatabase, error) {
	err := r.DB.WithContext(ctx).Create(&database).Error
	return database, err
}

// UpdateDatabase updates registered target settings.
func (r *PostgresRepository) UpdateDatabase(ctx context.Context, database models.ManagedDatabase) (models.ManagedDatabase, error) {
	updates := map[string]interface{}{
		"label":                   database.Label,
		"host":                    database.Host,
		"port":                    database.Port,
		"database_name":           database.DatabaseName,
		"ssl_mode":                database.SSLMode,
		"reader_username":         database.ReaderUsername,
		"reader_password_enc":     database.ReaderPasswordEnc,
		"migrator_username":       database.MigratorUsername,
		"migrator_password_enc":   database.MigratorPasswordEnc,
		"is_enabled":              database.IsEnabled,
	}

	err := r.DB.WithContext(ctx).
		Model(&models.ManagedDatabase{}).
		Where("id = ?", database.ID).
		Updates(updates).Error
	if err != nil {
		return models.ManagedDatabase{}, err
	}

	return r.GetDatabase(ctx, database.ID)
}

// DisableDatabase disables a target without deleting its history.
func (r *PostgresRepository) DisableDatabase(ctx context.Context, databaseID string) error {
	return r.DB.WithContext(ctx).
		Model(&models.ManagedDatabase{}).
		Where("id = ?", databaseID).
		Update("is_enabled", false).Error
}

// SetGroupDatabaseAccess upserts one group grant.
func (r *PostgresRepository) SetGroupDatabaseAccess(ctx context.Context, grant models.GroupAccess, now time.Time) (models.GroupAccess, error) {
	var result models.GroupAccess
	err := r.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Check foreign keys
		if err := tx.First(&models.Group{}, "id = ?", grant.GroupID).Error; err != nil {
			return err
		}
		if err := tx.First(&models.ManagedDatabase{}, "id = ?", grant.DatabaseID).Error; err != nil {
			return err
		}

		// Upsert using ON CONFLICT
		if err := tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "group_id"}, {Name: "database_id"}},
			DoUpdates: clause.AssignmentColumns([]string{"access_level"}),
		}).Create(&grant).Error; err != nil {
			return err
		}

		// Revoke affected group member sessions
		if err := tx.Model(&models.Session{}).
			Where("user_id IN (SELECT user_id FROM user_groups WHERE group_id = ?) AND revoked_at IS NULL", grant.GroupID).
			Updates(map[string]interface{}{
				"revoked_at":            now,
				"previous_token_hash":   gorm.Expr("token_hash"),
				"previous_valid_until":  now.Add(30 * time.Second),
			}).Error; err != nil {
			return err
		}

		result = grant
		return nil
	})
	return result, err
}

// DeleteGroupDatabaseAccess revokes one group grant.
func (r *PostgresRepository) DeleteGroupDatabaseAccess(ctx context.Context, groupID, databaseID string, now time.Time) error {
	return r.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Delete the grant
		if err := tx.Where("group_id = ? AND database_id = ?", groupID, databaseID).
			Delete(&models.GroupAccess{}).Error; err != nil {
			return err
		}

		// Revoke affected group member sessions
		if err := tx.Model(&models.Session{}).
			Where("user_id IN (SELECT user_id FROM user_groups WHERE group_id = ?) AND revoked_at IS NULL", groupID).
			Updates(map[string]interface{}{
				"revoked_at":            now,
				"previous_token_hash":   gorm.Expr("token_hash"),
				"previous_valid_until":  now.Add(30 * time.Second),
			}).Error; err != nil {
			return err
		}

		return nil
	})
}
