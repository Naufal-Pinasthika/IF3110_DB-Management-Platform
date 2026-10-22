package admin

import (
	"context"
	"database/sql"
	"time"

	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/api"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/apperror"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/auth"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/store"
)

type PostgresRepository struct {
	DB *sql.DB
}

var _ Repository = (*PostgresRepository)(nil)
var _ store.DatabaseRepository = (*PostgresRepository)(nil)

// ListUsers lists users without password hashes.
func (r *PostgresRepository) ListUsers(ctx context.Context) ([]api.AdminUser, error) {
	// TODO Select only the fields in api.AdminUser.
	return nil, apperror.ErrNotImplemented
}

// GetUser loads an account for a partial update.
func (r *PostgresRepository) GetUser(ctx context.Context, userID string) (auth.UserRecord, error) {
	// TODO Read the user and its password hash without serializing the record.
	return auth.UserRecord{}, apperror.ErrNotImplemented
}

// CreateUser persists a user with an already hashed password.
func (r *PostgresRepository) CreateUser(ctx context.Context, user auth.UserRecord) (api.AdminUser, error) {
	// TODO Insert a unique username and password hash and active state.
	// TODO Map unique violations to a conflict rather than leaking SQL errors.
	return api.AdminUser{}, apperror.ErrNotImplemented
}

// UpdateUser updates an account and revokes sessions when required.
func (r *PostgresRepository) UpdateUser(ctx context.Context, user auth.UserRecord, revokeSessions bool, now time.Time) (api.AdminUser, error) {
	// TODO Update the account and required session revocations in one metadata transaction.
	// TODO Use row locking or another conflict check to prevent lost partial updates.
	return api.AdminUser{}, apperror.ErrNotImplemented
}

// ListGroups lists groups.
func (r *PostgresRepository) ListGroups(ctx context.Context) ([]api.Group, error) {
	// TODO Select group IDs and names using a stable order.
	return nil, apperror.ErrNotImplemented
}

// CreateGroup creates a uniquely named group.
func (r *PostgresRepository) CreateGroup(ctx context.Context, name string) (api.Group, error) {
	// TODO Insert the group and translate duplicate names to a conflict.
	return api.Group{}, apperror.ErrNotImplemented
}

// UpdateGroup renames a group.
func (r *PostgresRepository) UpdateGroup(ctx context.Context, groupID, name string) (api.Group, error) {
	// TODO Update the specified group and report missing IDs and duplicate names.
	return api.Group{}, apperror.ErrNotImplemented
}

// DeleteGroup removes a group and its related grants and memberships.
func (r *PostgresRepository) DeleteGroup(ctx context.Context, groupID string, now time.Time) error {
	// TODO Revoke affected member sessions and delete the group in one metadata transaction.
	// TODO Use existing foreign key cascades without deleting users or migration history.
	return apperror.ErrNotImplemented
}

// AddGroupMember adds a membership without duplicates.
func (r *PostgresRepository) AddGroupMember(ctx context.Context, groupID, userID string, now time.Time) (api.GroupMember, error) {
	// TODO Check both IDs and insert the membership idempotently.
	// TODO Revoke affected user sessions in the same transaction when membership changes.
	return api.GroupMember{}, apperror.ErrNotImplemented
}

// RemoveGroupMember removes one membership.
func (r *PostgresRepository) RemoveGroupMember(ctx context.Context, groupID, userID string, now time.Time) error {
	// TODO Delete only this pair and revoke affected user sessions in the same transaction.
	// TODO Preserve all other group memberships.
	return apperror.ErrNotImplemented
}

// ListDatabases lists registered targets without secrets.
func (r *PostgresRepository) ListDatabases(ctx context.Context) ([]api.ManagedDatabaseSummary, error) {
	// TODO Select summary fields including disabled targets for administration.
	return nil, apperror.ErrNotImplemented
}

// GetDatabase loads a target with encrypted credentials for internal use.
func (r *PostgresRepository) GetDatabase(ctx context.Context, databaseID string) (store.DatabaseRecord, error) {
	// TODO Read current settings and encrypted credentials and report missing IDs.
	// TODO Never return this full record from an HTTP handler.
	return store.DatabaseRecord{}, apperror.ErrNotImplemented
}

// CreateDatabase stores a target with encrypted credentials.
func (r *PostgresRepository) CreateDatabase(ctx context.Context, database store.DatabaseRecord) (api.ManagedDatabaseSummary, error) {
	// TODO Insert connection fields and ciphertext and return only summary fields.
	return api.ManagedDatabaseSummary{}, apperror.ErrNotImplemented
}

// UpdateDatabase updates registered target settings.
func (r *PostgresRepository) UpdateDatabase(ctx context.Context, database store.DatabaseRecord) (api.ManagedDatabaseSummary, error) {
	// TODO Persist merged settings and ciphertext without storing request passwords.
	// TODO Prevent lost updates with a transaction or an explicit conflict strategy.
	return api.ManagedDatabaseSummary{}, apperror.ErrNotImplemented
}

// DisableDatabase disables a target without deleting its history.
func (r *PostgresRepository) DisableDatabase(ctx context.Context, databaseID string) error {
	// TODO Set is_enabled to false and keep its grants and migration records.
	return apperror.ErrNotImplemented
}

// SetGroupDatabaseAccess upserts one group grant.
func (r *PostgresRepository) SetGroupDatabaseAccess(ctx context.Context, grant api.GroupDatabaseAccess, now time.Time) (api.GroupDatabaseAccess, error) {
	// TODO Check both foreign keys and upsert the allowed access level.
	// TODO Revoke affected group member sessions in the same transaction on a change.
	return api.GroupDatabaseAccess{}, apperror.ErrNotImplemented
}

// DeleteGroupDatabaseAccess revokes one group grant.
func (r *PostgresRepository) DeleteGroupDatabaseAccess(ctx context.Context, groupID, databaseID string, now time.Time) error {
	// TODO Delete the pair and revoke affected group member sessions atomically.
	// TODO Leave other groups and their grants unchanged.
	return apperror.ErrNotImplemented
}
