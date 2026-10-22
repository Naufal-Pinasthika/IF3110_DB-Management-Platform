package migration

import (
	"context"
	"time"

	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/api"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/apperror"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/auth"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/models"
)

type Repository interface {
	ListMigrations(ctx context.Context, databaseID string, status models.MigrationStatus) ([]models.Migration, error)
	GetMigration(ctx context.Context, databaseID, migrationID string) (models.Migration, error)
	CreateMigration(ctx context.Context, migration models.Migration) (models.Migration, error)
	UpdateMigration(ctx context.Context, databaseID, migrationID string, sqlText string, now time.Time) (models.Migration, error)
	LockDatabase(ctx context.Context, databaseID string) (func() error, error)
	MarkRunning(ctx context.Context, databaseID, migrationID, executorID string, now time.Time) (models.Migration, error)
	MarkApplied(ctx context.Context, databaseID, migrationID, executorID string, now time.Time) (models.Migration, error)
	MarkFailed(ctx context.Context, databaseID, migrationID, executorID, errorSummary string, now time.Time) (models.Migration, error)
}

type AccessChecker interface {
	RequireDatabase(context.Context, auth.Principal, string, api.AccessLevel) error
}

type SQLRunner interface {
	Execute(context.Context, string, string) error
}

type Service struct {
	Repository Repository
	Access     AccessChecker
	Runner     SQLRunner
}

// ListMigrations returns linear history and draft summaries.
func (s *Service) ListMigrations(ctx context.Context, actor auth.Principal, databaseID string, status api.MigrationStatus) ([]api.MigrationSummary, error) {
	// TODO Require at least VIEW_ONLY and validate the optional status filter.
	// TODO Map target scoped models to api.MigrationSummary without changing history.
	return nil, apperror.ErrNotImplemented
}

// GetMigration returns SQL and audit details for a visible migration.
func (s *Service) GetMigration(ctx context.Context, actor auth.Principal, databaseID, migrationID string) (api.MigrationDetail, error) {
	// TODO Require at least VIEW_ONLY and look up the model using both IDs.
	// TODO Map the model to api.MigrationDetail without exposing loaded relations.
	return api.MigrationDetail{}, apperror.ErrNotImplemented
}

// CreateMigration creates a SQL draft for an editor.
func (s *Service) CreateMigration(ctx context.Context, actor auth.Principal, databaseID string, input api.CreateMigrationRequest, now time.Time) (api.MigrationCreated, error) {
	// TODO Require VIEW_EDIT and validate a nonempty name up to 200 characters and nonempty SQL.
	// TODO Build models.Migration using the principal and timestamps with DRAFT status and nil Version.
	// TODO Map the saved model to api.MigrationCreated.
	return api.MigrationCreated{}, apperror.ErrNotImplemented
}

// UpdateMigration changes SQL before execution.
func (s *Service) UpdateMigration(ctx context.Context, actor auth.Principal, databaseID, migrationID string, input api.UpdateMigrationRequest, now time.Time) (api.MigrationUpdated, error) {
	// TODO Require VIEW_EDIT and nonempty SQL and update only DRAFT records atomically.
	// TODO Reject RUNNING and APPLIED and FAILED with a conflict and keep their SQL intact.
	return api.MigrationUpdated{}, apperror.ErrNotImplemented
}

// ExecuteMigration coordinates a target transaction and persistent history.
func (s *Service) ExecuteMigration(ctx context.Context, actor auth.Principal, databaseID, migrationID string, now time.Time) (api.MigrationExecuted, error) {
	// TODO Require VIEW_EDIT and acquire the per database execution lock before claiming a draft.
	// TODO Reject execution while any unresolved RUNNING record exists for the target.
	// TODO Claim DRAFT as RUNNING atomically and execute only the returned stored SQL.
	// TODO Keep the lock until execution and final metadata updates have finished.
	// TODO Use Runner.Execute so all target statements share one transaction.
	// TODO On confirmed commit record APPLIED and assign the next successful version.
	// TODO On confirmed rollback record FAILED without a version and sanitize the error.
	// TODO If commit outcome is unknown or history recording fails keep RUNNING for investigation.
	// TODO Do not automatically retry or mark FAILED after a confirmed or uncertain commit.
	// TODO Use the actual completion time for history rather than the supplied request start time.
	return api.MigrationExecuted{}, apperror.ErrNotImplemented
}
