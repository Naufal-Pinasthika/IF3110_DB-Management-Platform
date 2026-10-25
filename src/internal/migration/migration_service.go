package migration

import (
	"context"
	"strings"
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
	RequireDatabase(ctx context.Context, actor auth.Principal, databaseID string, level api.AccessLevel) error
}


type SQLRunner interface {
	Execute(ctx context.Context, databaseID, sqlText string) error
}


type Service struct {
	Repository Repository
	Access     AccessChecker
	Runner     SQLRunner
}


// ListMigrations returns linear history and draft summaries.
func (s *Service) ListMigrations(ctx context.Context, actor auth.Principal, databaseID string, status api.MigrationStatus) ([]api.MigrationSummary, error) {
	if err := s.Access.RequireDatabase(ctx, actor, databaseID, api.AccessViewOnly); err != nil {
		return nil, err
	}

	migrations, err := s.Repository.ListMigrations(ctx, databaseID, models.MigrationStatus(status))
	if err != nil {
		return nil, err
	}

	summaries := make([]api.MigrationSummary, 0, len(migrations))
	for _, m := range migrations {
		summaries = append(summaries, api.MigrationSummary{
			ID:         m.ID,
			Name:       m.Name,
			Version:    m.Version,
			Status:     m.Status,
			ExecutedBy: m.ExecutedBy,
			ExecutedAt: m.ExecutedAt,
		})
	}

	return summaries, nil
}


// GetMigration returns SQL and audit details for a visible migration.
func (s *Service) GetMigration(ctx context.Context, actor auth.Principal, databaseID, migrationID string) (api.MigrationDetail, error) {
	if err := s.Access.RequireDatabase(ctx, actor, databaseID, api.AccessViewOnly); err != nil {
		return api.MigrationDetail{}, err
	}

	m, err := s.Repository.GetMigration(ctx, databaseID, migrationID)
	if err != nil {
		return api.MigrationDetail{}, err
	}

	return api.MigrationDetail{
		ID:           m.ID,
		DatabaseID:   m.DatabaseID,
		Name:         m.Name,
		SQLText:      m.SQLText,
		Status:       m.Status,
		Version:      m.Version,
		CreatedBy:    m.CreatedBy,
		ExecutedBy:   m.ExecutedBy,
		CreatedAt:    m.CreatedAt,
		UpdatedAt:    m.UpdatedAt,
		ExecutedAt:   m.ExecutedAt,
		ErrorSummary: m.ErrorSummary,
	}, nil
}


// CreateMigration creates a SQL draft for an editor.
func (s *Service) CreateMigration(ctx context.Context, actor auth.Principal, databaseID string, input api.CreateMigrationRequest, now time.Time) (api.MigrationCreated, error) {
	if err := s.Access.RequireDatabase(ctx, actor, databaseID, api.AccessViewEdit); err != nil {
		return api.MigrationCreated{}, err
	}

	if input.Name == "" || len(input.Name) > 200 || input.SQLText == "" {
		return api.MigrationCreated{}, apperror.ErrInvalidInput
	}

	m := models.Migration{
		DatabaseID: databaseID,
		Name:       input.Name,
		SQLText:    input.SQLText,
		CreatedBy:  actor.User.ID,
		CreatedAt:  now,
		UpdatedAt:  now,
	}

	saved, err := s.Repository.CreateMigration(ctx, m)
	if err != nil {
		return api.MigrationCreated{}, err
	}

	return api.MigrationCreated{
		ID:         saved.ID,
		DatabaseID: saved.DatabaseID,
		Name:       saved.Name,
		Status:     saved.Status,
		Version:    saved.Version,
	}, nil
}


// UpdateMigration changes SQL before execution.
func (s *Service) UpdateMigration(ctx context.Context, actor auth.Principal, databaseID, migrationID string, input api.UpdateMigrationRequest, now time.Time) (api.MigrationUpdated, error) {
	if err := s.Access.RequireDatabase(ctx, actor, databaseID, api.AccessViewEdit); err != nil {
		return api.MigrationUpdated{}, err
	}

	if input.SQLText == "" {
		return api.MigrationUpdated{}, apperror.ErrInvalidInput
	}

	updated, err := s.Repository.UpdateMigration(ctx, databaseID, migrationID, input.SQLText, now)
	if err != nil {
		return api.MigrationUpdated{}, err
	}

	return api.MigrationUpdated{
		ID:      updated.ID,
		Status:  updated.Status,
		Version: updated.Version,
	}, nil
}


// ExecuteMigration coordinates a target transaction and persistent history.
func (s *Service) ExecuteMigration(ctx context.Context, actor auth.Principal, databaseID, migrationID string, now time.Time) (api.MigrationExecuted, error) {
	if err := s.Access.RequireDatabase(ctx, actor, databaseID, api.AccessViewEdit); err != nil {
		return api.MigrationExecuted{}, err
	}

	release, err := s.Repository.LockDatabase(ctx, databaseID)
	if err != nil {
		return api.MigrationExecuted{}, apperror.ErrInternal
	}
	defer release()

	running, err := s.Repository.MarkRunning(ctx, databaseID, migrationID, actor.User.ID, now)
	if err != nil {
		return api.MigrationExecuted{}, err
	}

	execErr := s.Runner.Execute(ctx, databaseID, running.SQLText)
	completionTime := time.Now()

	if execErr != nil {
		errSummary := execErr.Error()
		if idx := strings.Index(errSummary, "pq: "); idx != -1 {
			errSummary = errSummary[idx:]
		} else if strings.HasPrefix(errSummary, "invalid input provided: ") {
			errSummary = strings.TrimPrefix(errSummary, "invalid input provided: ")
		}
		_, failErr := s.Repository.MarkFailed(ctx, databaseID, migrationID, actor.User.ID, errSummary, completionTime)
		if failErr != nil {
			return api.MigrationExecuted{}, failErr
		}
		return api.MigrationExecuted{}, execErr
	}

	applied, err := s.Repository.MarkApplied(ctx, databaseID, migrationID, actor.User.ID, completionTime)
	if err != nil {
		return api.MigrationExecuted{}, err
	}

	return api.MigrationExecuted{
		ID:         applied.ID,
		Status:     applied.Status,
		Version:    applied.Version,
		ExecutedBy: applied.ExecutedBy,
		ExecutedAt: applied.ExecutedAt,
	}, nil
}