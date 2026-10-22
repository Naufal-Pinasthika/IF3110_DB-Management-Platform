package admin

import (
	"context"
	"time"

	"gorm.io/gorm"

	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/apperror"
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
	// TODO Query models.User with WithContext and select only public user fields.
	return nil, apperror.ErrNotImplemented
}

// GetUser loads an account for a partial update.
func (r *PostgresRepository) GetUser(ctx context.Context, userID string) (models.User, error) {
	// TODO Read the user and its password hash without serializing the record.
	return models.User{}, apperror.ErrNotImplemented
}

// CreateUser persists a user with an already hashed password.
func (r *PostgresRepository) CreateUser(ctx context.Context, user models.User) (models.User, error) {
	// TODO Insert a unique username and password hash and active state.
	// TODO Map unique violations to a conflict rather than leaking SQL errors.
	return models.User{}, apperror.ErrNotImplemented
}

// UpdateUser updates an account and revokes sessions when required.
func (r *PostgresRepository) UpdateUser(ctx context.Context, user models.User, revokeSessions bool, now time.Time) (models.User, error) {
	// TODO Use a GORM transaction for user changes and required session revocations.
	// TODO Use explicit selected fields or a map so false is not skipped.
	// TODO Use row locking or another conflict check to prevent lost partial updates.
	return models.User{}, apperror.ErrNotImplemented
}

// ListGroups lists groups.
func (r *PostgresRepository) ListGroups(ctx context.Context) ([]models.Group, error) {
	// TODO Select group IDs and names using a stable order.
	return nil, apperror.ErrNotImplemented
}

// CreateGroup creates a uniquely named group.
func (r *PostgresRepository) CreateGroup(ctx context.Context, name string) (models.Group, error) {
	// TODO Insert the group and translate duplicate names to a conflict.
	return models.Group{}, apperror.ErrNotImplemented
}

// UpdateGroup renames a group.
func (r *PostgresRepository) UpdateGroup(ctx context.Context, groupID, name string) (models.Group, error) {
	// TODO Update the specified group and report missing IDs and duplicate names.
	return models.Group{}, apperror.ErrNotImplemented
}

// DeleteGroup removes a group and its related grants and memberships.
func (r *PostgresRepository) DeleteGroup(ctx context.Context, groupID string, now time.Time) error {
	// TODO Revoke affected member sessions and delete the group in one metadata transaction.
	// TODO Use existing foreign key cascades without deleting users or migration history.
	return apperror.ErrNotImplemented
}

// AddGroupMember adds a membership without duplicates.
func (r *PostgresRepository) AddGroupMember(ctx context.Context, groupID, userID string, now time.Time) (models.UserGroup, error) {
	// TODO Check both IDs and insert the membership idempotently.
	// TODO Revoke affected user sessions in the same transaction when membership changes.
	return models.UserGroup{}, apperror.ErrNotImplemented
}

// RemoveGroupMember removes one membership.
func (r *PostgresRepository) RemoveGroupMember(ctx context.Context, groupID, userID string, now time.Time) error {
	// TODO Delete only this pair and revoke affected user sessions in the same transaction.
	// TODO Preserve all other group memberships.
	return apperror.ErrNotImplemented
}

// ListDatabases lists registered targets without secrets.
func (r *PostgresRepository) ListDatabases(ctx context.Context) ([]models.ManagedDatabase, error) {
	// TODO Select summary fields including disabled targets for administration.
	return nil, apperror.ErrNotImplemented
}

// GetDatabase loads a target with encrypted credentials for internal use.
func (r *PostgresRepository) GetDatabase(ctx context.Context, databaseID string) (models.ManagedDatabase, error) {
	// TODO Read current settings and encrypted credentials and report missing IDs.
	// TODO Map the model to a safe API response in the service.
	return models.ManagedDatabase{}, apperror.ErrNotImplemented
}

// CreateDatabase stores a target with encrypted credentials.
func (r *PostgresRepository) CreateDatabase(ctx context.Context, database models.ManagedDatabase) (models.ManagedDatabase, error) {
	// TODO Insert connection fields and ciphertext and return only summary fields.
	return models.ManagedDatabase{}, apperror.ErrNotImplemented
}

// UpdateDatabase updates registered target settings.
func (r *PostgresRepository) UpdateDatabase(ctx context.Context, database models.ManagedDatabase) (models.ManagedDatabase, error) {
	// TODO Update selected model fields with a map so false values are preserved.
	// TODO Prevent lost updates with a transaction or an explicit conflict strategy.
	return models.ManagedDatabase{}, apperror.ErrNotImplemented
}

// DisableDatabase disables a target without deleting its history.
func (r *PostgresRepository) DisableDatabase(ctx context.Context, databaseID string) error {
	// TODO Set is_enabled to false and keep its grants and migration records.
	return apperror.ErrNotImplemented
}

// SetGroupDatabaseAccess upserts one group grant.
func (r *PostgresRepository) SetGroupDatabaseAccess(ctx context.Context, grant models.GroupAccess, now time.Time) (models.GroupAccess, error) {
	// TODO Check both foreign keys and upsert the allowed access level.
	// TODO Use clause.OnConflict with both primary key columns instead of Save.
	// TODO Revoke affected group member sessions in the same transaction on a change.
	return models.GroupAccess{}, apperror.ErrNotImplemented
}

// DeleteGroupDatabaseAccess revokes one group grant.
func (r *PostgresRepository) DeleteGroupDatabaseAccess(ctx context.Context, groupID, databaseID string, now time.Time) error {
	// TODO Delete the pair and revoke affected group member sessions atomically.
	// TODO Leave other groups and their grants unchanged.
	return apperror.ErrNotImplemented
}
