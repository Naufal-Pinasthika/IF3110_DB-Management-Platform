package access

import (
	"context"
	"database/sql"

	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/api"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/apperror"
)

type PostgresRepository struct {
	DB *sql.DB
}

var _ Repository = (*PostgresRepository)(nil)

// ListUserGrants loads grants across all current groups.
func (r *PostgresRepository) ListUserGrants(ctx context.Context, userID, databaseID string) ([]api.AccessLevel, error) {
	// TODO Join users and user_groups and group_access and managed_databases.
	// TODO Return grants only for an active user and enabled database.
	return nil, apperror.ErrNotImplemented
}

// ListGrantedDatabases lists distinct enabled targets with at least one group grant.
func (r *PostgresRepository) ListGrantedDatabases(ctx context.Context, userID string) ([]api.ManagedDatabaseSummary, error) {
	// TODO Use current memberships and grants without an implicit platform admin bypass.
	// TODO Leave effective permission calculation in the service.
	return nil, apperror.ErrNotImplemented
}

// GetDatabaseSummary loads safe target metadata.
func (r *PostgresRepository) GetDatabaseSummary(ctx context.Context, databaseID string) (api.ManagedDatabaseSummary, error) {
	// TODO Select public summary fields and report missing or disabled targets.
	return api.ManagedDatabaseSummary{}, apperror.ErrNotImplemented
}
