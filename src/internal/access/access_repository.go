package access

import (
	"context"

	"gorm.io/gorm"

	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/apperror"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/models"
)

// PostgresRepository uses GORM models and leaves HTTP mapping to the service.
type PostgresRepository struct {
	DB *gorm.DB
}

var _ Repository = (*PostgresRepository)(nil)

// ListUserGrants loads grants across all current groups.
func (r *PostgresRepository) ListUserGrants(ctx context.Context, userID, databaseID string) ([]models.GroupAccess, error) {
	// TODO Query models.GroupAccess with explicit GORM joins for users and memberships and targets.
	// TODO Return grants only for an active user and enabled database.
	return nil, apperror.ErrNotImplemented
}

// ListGrantedDatabases lists distinct enabled targets with at least one group grant.
func (r *PostgresRepository) ListGrantedDatabases(ctx context.Context, userID string) ([]models.ManagedDatabase, error) {
	// TODO Use current memberships and grants without an implicit platform admin bypass.
	// TODO Leave effective permission calculation in the service.
	return nil, apperror.ErrNotImplemented
}

// GetDatabaseSummary loads safe target metadata.
func (r *PostgresRepository) GetDatabaseSummary(ctx context.Context, databaseID string) (models.ManagedDatabase, error) {
	// TODO Select public fields into models.ManagedDatabase and report missing or disabled targets.
	return models.ManagedDatabase{}, apperror.ErrNotImplemented
}
