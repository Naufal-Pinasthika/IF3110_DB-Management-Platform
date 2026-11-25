package migration

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/api"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/apperror"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/auth"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/contracts"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/jobs"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/models"
)

type AccessChecker interface {
	RequireDatabase(ctx context.Context, actor auth.Principal, databaseID string, level api.AccessLevel) error
}

type SQLRunner interface {
	Execute(ctx context.Context, databaseID, sqlText string) error
}

type SQLExplainer interface {
	ExplainSQL(ctx context.Context, databaseID, sqlText string, analyze bool) (api.ExplainResult, error)
}

type SchemaComparator interface {
	Compare(ctx context.Context, databaseID, desiredDDL string) (api.SchemaDiffResult, error)
}

type Service struct {
	Repository Repository
	Access     AccessChecker
	Runner     SQLRunner
	Explainer  SQLExplainer
	SchemaDiff SchemaComparator
	Dispatcher Dispatcher
}

func (s *Service) requirePostgresTarget(ctx context.Context, actor auth.Principal, databaseID string) error {
	provider, ok := s.Access.(interface {
		GetDatabase(context.Context, auth.Principal, string) (api.UserDatabase, error)
	})
	if !ok {
		return nil
	}
	database, err := provider.GetDatabase(ctx, actor, databaseID)
	if err != nil {
		return err
	}
	if database.Engine == models.EngineMySQL {
		return fmt.Errorf("%w: query plan and schema diff analysis currently support PostgreSQL only", apperror.ErrInvalidInput)
	}
	return nil
}

// CompareSchema checks a complete desired DDL description against the target.
func (s *Service) CompareSchema(ctx context.Context, actor auth.Principal, databaseID string, input api.SchemaDiffRequest) (api.SchemaDiffResult, error) {
	if err := s.Access.RequireDatabase(ctx, actor, databaseID, api.AccessViewEdit); err != nil {
		return api.SchemaDiffResult{}, err
	}
	if err := s.requirePostgresTarget(ctx, actor, databaseID); err != nil {
		return api.SchemaDiffResult{}, err
	}
	if s.SchemaDiff == nil {
		return api.SchemaDiffResult{}, fmt.Errorf("%w: schema comparison is unavailable", apperror.ErrInternal)
	}
	return s.SchemaDiff.Compare(ctx, databaseID, input.DesiredDDL)
}

// ExplainMigration previews a draft without committing any statement effects.
func (s *Service) ExplainMigration(ctx context.Context, actor auth.Principal, databaseID, migrationID string, analyze bool) (api.ExplainResult, error) {
	if err := s.Access.RequireDatabase(ctx, actor, databaseID, api.AccessViewEdit); err != nil {
		return api.ExplainResult{}, err
	}
	if err := s.requirePostgresTarget(ctx, actor, databaseID); err != nil {
		return api.ExplainResult{}, err
	}
	m, err := s.Repository.GetMigration(ctx, databaseID, migrationID)
	if err != nil {
		return api.ExplainResult{}, err
	}
	if m.Status != models.MigrationDraft && m.Status != models.MigrationRejected {
		return api.ExplainResult{}, apperror.ErrConflict
	}
	if s.Explainer == nil {
		return api.ExplainResult{}, fmt.Errorf("%w: migration plan analysis is unavailable", apperror.ErrInternal)
	}
	return s.Explainer.ExplainSQL(ctx, databaseID, m.SQLText, analyze)
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
			ID:             m.ID,
			Name:           m.Name,
			Version:        m.Version,
			Status:         m.Status,
			MigrationType:  m.MigrationType,
			IsIrreversible: m.IsIrreversible,
			ExecutedBy:     m.ExecutedBy,
			ExecutedAt:     m.ExecutedAt,
			HasDown:        m.SQLDown != nil && *m.SQLDown != "",
		})
	}

	return summaries, nil
}

// GetMigration returns SQL, approvals, and execution eligibility for a visible migration.
func (s *Service) GetMigration(ctx context.Context, actor auth.Principal, databaseID, migrationID string) (api.MigrationDetail, error) {
	if err := s.Access.RequireDatabase(ctx, actor, databaseID, api.AccessViewOnly); err != nil {
		return api.MigrationDetail{}, err
	}

	m, approvals, err := s.Repository.GetMigrationWithApprovals(ctx, databaseID, migrationID)
	if err != nil {
		return api.MigrationDetail{}, err
	}

	approvalItems := make([]api.MigrationApprovalItem, 0, len(approvals))
	for _, app := range approvals {
		approvalItems = append(approvalItems, api.MigrationApprovalItem{
			ID:             app.ID,
			GroupID:        app.GroupID,
			GroupName:      app.Group.Name,
			LeaderID:       app.LeaderID,
			LeaderUsername: app.Leader.Username,
			Status:         string(app.Status),
			Comment:        app.Comment,
			DecidedAt:      app.DecidedAt,
		})
	}

	lastApplied, _ := s.Repository.GetLastAppliedMigration(ctx, databaseID)
	canRollback := lastApplied != nil && lastApplied.ID == migrationID &&
		!m.IsIrreversible && m.SQLDown != nil && *m.SQLDown != "" &&
		m.Status == models.MigrationApplied

	nextForward, _ := s.Repository.GetNextForwardCandidate(ctx, databaseID)
	canForward := nextForward != nil && nextForward.ID == migrationID &&
		m.Status == models.MigrationApproved

	return api.MigrationDetail{
		ID:                 m.ID,
		DatabaseID:         m.DatabaseID,
		Name:               m.Name,
		SQLText:            m.SQLText,
		SQLDown:            m.SQLDown,
		IsIrreversible:     m.IsIrreversible,
		IrreversibleReason: m.IrreversibleReason,
		MigrationType:      m.MigrationType,
		BatchAttribute:     m.BatchAttribute,
		BatchSize:          m.BatchSize,
		Status:             m.Status,
		Version:            m.Version,
		CreatedBy:          m.CreatedBy,
		ExecutedBy:         m.ExecutedBy,
		CreatedAt:          m.CreatedAt,
		UpdatedAt:          m.UpdatedAt,
		ExecutedAt:         m.ExecutedAt,
		ErrorSummary:       m.ErrorSummary,
		Approvals:          approvalItems,
		CanRollback:        canRollback,
		CanForward:         canForward,
	}, nil
}

// CreateMigration creates a SQL draft.
func (s *Service) CreateMigration(ctx context.Context, actor auth.Principal, databaseID string, input api.CreateMigrationRequest, now time.Time) (api.MigrationCreated, error) {
	if err := s.Access.RequireDatabase(ctx, actor, databaseID, api.AccessViewEdit); err != nil {
		return api.MigrationCreated{}, err
	}

	if input.Name == "" || len(input.Name) > 200 || input.SQLText == "" {
		return api.MigrationCreated{}, apperror.ErrInvalidInput
	}

	migType := input.MigrationType
	if migType == "" {
		migType = "REGULAR"
	}
	if migType != "REGULAR" && migType != "BATCH" {
		return api.MigrationCreated{}, fmt.Errorf("%w: invalid migration_type", apperror.ErrInvalidInput)
	}

	if input.IsIrreversible && (input.IrreversibleReason == nil || *input.IrreversibleReason == "") {
		return api.MigrationCreated{}, fmt.Errorf("%w: irreversible migrations require an irreversible_reason", apperror.ErrInvalidInput)
	}

	m := models.Migration{
		DatabaseID:         databaseID,
		Name:               input.Name,
		SQLText:            input.SQLText,
		SQLDown:            input.SQLDown,
		IsIrreversible:     input.IsIrreversible,
		IrreversibleReason: input.IrreversibleReason,
		MigrationType:      migType,
		BatchAttribute:     input.BatchAttribute,
		BatchSize:          input.BatchSize,
		CreatedBy:          actor.User.ID,
		CreatedAt:          now,
		UpdatedAt:          now,
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

// UpdateMigration changes SQL and parameters before review.
func (s *Service) UpdateMigration(ctx context.Context, actor auth.Principal, databaseID, migrationID string, input api.UpdateMigrationRequest, now time.Time) (api.MigrationUpdated, error) {
	if err := s.Access.RequireDatabase(ctx, actor, databaseID, api.AccessViewEdit); err != nil {
		return api.MigrationUpdated{}, err
	}

	updates := make(map[string]any)
	if input.SQLText != nil {
		if *input.SQLText == "" {
			return api.MigrationUpdated{}, apperror.ErrInvalidInput
		}
		updates["sql_text"] = *input.SQLText
	}
	if input.SQLDown != nil {
		updates["sql_down"] = *input.SQLDown
	}
	if input.IsIrreversible != nil {
		updates["is_irreversible"] = *input.IsIrreversible
	}
	if input.IrreversibleReason != nil {
		updates["irreversible_reason"] = *input.IrreversibleReason
	}
	if input.BatchAttribute != nil {
		updates["batch_attribute"] = *input.BatchAttribute
	}
	if input.BatchSize != nil {
		updates["batch_size"] = *input.BatchSize
	}

	updated, err := s.Repository.UpdateMigrationFields(ctx, databaseID, migrationID, updates, now)
	if err != nil {
		return api.MigrationUpdated{}, err
	}

	return api.MigrationUpdated{
		ID:      updated.ID,
		Status:  updated.Status,
		Version: updated.Version,
	}, nil
}

// SubmitReview requests approval from all group leaders with database access.
func (s *Service) SubmitReview(ctx context.Context, actor auth.Principal, databaseID, migrationID string, now time.Time) (api.MigrationDetail, error) {
	if err := s.Access.RequireDatabase(ctx, actor, databaseID, api.AccessViewEdit); err != nil {
		return api.MigrationDetail{}, err
	}

	m, err := s.Repository.GetMigration(ctx, databaseID, migrationID)
	if err != nil {
		return api.MigrationDetail{}, err
	}
	if m.Status != models.MigrationDraft && m.Status != models.MigrationRejected {
		return api.MigrationDetail{}, fmt.Errorf("%w: migration must be DRAFT or REJECTED to submit for review", apperror.ErrConflict)
	}

	groups, err := s.Repository.GetDatabaseGroupsWithLeaders(ctx, databaseID)
	if err != nil {
		return api.MigrationDetail{}, err
	}
	if len(groups) == 0 {
		return api.MigrationDetail{}, fmt.Errorf("%w: no groups have access to this database", apperror.ErrConflict)
	}

	var approvals []models.MigrationApproval
	for _, g := range groups {
		if g.LeaderID == nil || *g.LeaderID == "" {
			return api.MigrationDetail{}, fmt.Errorf("%w: group %q has no designated leader", apperror.ErrConflict, g.Name)
		}
		approvals = append(approvals, models.MigrationApproval{
			MigrationID: migrationID,
			GroupID:     g.ID,
			LeaderID:    *g.LeaderID,
			Status:      models.ApprovalPending,
			CreatedAt:   now,
		})
	}

	if err := s.Repository.SubmitForReview(ctx, databaseID, migrationID, approvals); err != nil {
		return api.MigrationDetail{}, err
	}

	return s.GetMigration(ctx, actor, databaseID, migrationID)
}

// ApproveMigration allows a group leader to approve a pending review.
func (s *Service) ApproveMigration(ctx context.Context, actor auth.Principal, databaseID, migrationID string, input api.DecideApprovalRequest, now time.Time) (api.MigrationDetail, error) {
	if err := s.Access.RequireDatabase(ctx, actor, databaseID, api.AccessViewOnly); err != nil {
		return api.MigrationDetail{}, err
	}

	m, err := s.Repository.GetMigration(ctx, databaseID, migrationID)
	if err != nil {
		return api.MigrationDetail{}, err
	}
	if m.Status != models.MigrationPendingReview {
		return api.MigrationDetail{}, fmt.Errorf("%w: migration is not pending review", apperror.ErrConflict)
	}

	approvals, err := s.Repository.GetPendingApprovalForLeader(ctx, migrationID, actor.User.ID)
	if err != nil {
		return api.MigrationDetail{}, err
	}
	if len(approvals) == 0 {
		return api.MigrationDetail{}, fmt.Errorf("%w: caller is not a group leader with pending approval for this migration", apperror.ErrForbidden)
	}

	for _, app := range approvals {
		if err := s.Repository.UpdateApprovalStatus(ctx, app.ID, models.ApprovalApproved, input.Comment, now); err != nil {
			return api.MigrationDetail{}, err
		}
	}

	_, _ = s.Repository.CheckAndSetApproved(ctx, databaseID, migrationID)

	return s.GetMigration(ctx, actor, databaseID, migrationID)
}

// RejectMigration allows a group leader to reject a pending review.
func (s *Service) RejectMigration(ctx context.Context, actor auth.Principal, databaseID, migrationID string, input api.DecideApprovalRequest, now time.Time) (api.MigrationDetail, error) {
	if err := s.Access.RequireDatabase(ctx, actor, databaseID, api.AccessViewOnly); err != nil {
		return api.MigrationDetail{}, err
	}

	m, err := s.Repository.GetMigration(ctx, databaseID, migrationID)
	if err != nil {
		return api.MigrationDetail{}, err
	}
	if m.Status != models.MigrationPendingReview {
		return api.MigrationDetail{}, fmt.Errorf("%w: migration is not pending review", apperror.ErrConflict)
	}

	approvals, err := s.Repository.GetPendingApprovalForLeader(ctx, migrationID, actor.User.ID)
	if err != nil {
		return api.MigrationDetail{}, err
	}
	if len(approvals) == 0 {
		return api.MigrationDetail{}, fmt.Errorf("%w: caller is not an assigned group leader for this migration", apperror.ErrForbidden)
	}

	for _, app := range approvals {
		_ = s.Repository.UpdateApprovalStatus(ctx, app.ID, models.ApprovalRejected, input.Comment, now)
	}

	if err := s.Repository.RejectMigration(ctx, databaseID, migrationID); err != nil {
		return api.MigrationDetail{}, err
	}

	return s.GetMigration(ctx, actor, databaseID, migrationID)
}

// ExecuteMigration publishes the forward execution job asynchronously.
func (s *Service) ExecuteMigration(ctx context.Context, actor auth.Principal, databaseID, migrationID string, now time.Time) (api.MigrationExecuted, error) {
	if err := s.Access.RequireDatabase(ctx, actor, databaseID, api.AccessViewEdit); err != nil {
		return api.MigrationExecuted{}, err
	}

	m, approvals, err := s.Repository.GetMigrationWithApprovals(ctx, databaseID, migrationID)
	if err != nil {
		return api.MigrationExecuted{}, err
	}
	if m.Status != models.MigrationApproved {
		return api.MigrationExecuted{}, fmt.Errorf("%w: migration must be APPROVED before execution", apperror.ErrConflict)
	}

	// Revalidate that every group currently granted access to this database has a leader and approved this migration.
	groups, err := s.Repository.GetDatabaseGroupsWithLeaders(ctx, databaseID)
	if err != nil {
		return api.MigrationExecuted{}, err
	}
	if len(groups) == 0 {
		return api.MigrationExecuted{}, fmt.Errorf("%w: no groups have access to this database", apperror.ErrConflict)
	}
	approvedGroups := make(map[string]bool, len(approvals))
	for _, a := range approvals {
		if a.Status == models.ApprovalApproved {
			approvedGroups[a.GroupID] = true
		}
	}
	for _, g := range groups {
		if g.LeaderID == nil || *g.LeaderID == "" {
			return api.MigrationExecuted{}, fmt.Errorf("%w: group %q has no designated leader", apperror.ErrConflict, g.Name)
		}
		if !approvedGroups[g.ID] {
			return api.MigrationExecuted{}, fmt.Errorf("%w: missing approval from leader of group %q", apperror.ErrConflict, g.Name)
		}
	}

	exec := models.MigrationExecution{
		MigrationID: m.ID,
		DatabaseID:  databaseID,
		ActionType:  "FORWARD",
		Status:      "RUNNING",
		ExecutedSQL: m.SQLText,
		ExecutedBy:  actor.User.ID,
		StartedAt:   now,
	}
	if err := s.Repository.CreateExecution(ctx, &exec); err != nil {
		return api.MigrationExecuted{}, err
	}

	if s.Dispatcher == nil {
		return api.MigrationExecuted{}, fmt.Errorf("%w: queue dispatcher not configured", apperror.ErrInternal)
	}

	receipt, err := s.Dispatcher.Enqueue(ctx, contracts.ExecutionRef{
		ExecutionID: exec.ID,
		DatabaseID:  databaseID,
	})
	if err != nil {
		return api.MigrationExecuted{}, err
	}

	_, _ = s.Repository.MarkRunning(ctx, databaseID, migrationID, actor.User.ID, now)

	return api.MigrationExecuted{
		ID:          m.ID,
		Status:      models.MigrationRunning,
		ExecutedBy:  &actor.User.ID,
		JobID:       receipt.JobID,
		ExecutionID: exec.ID,
	}, nil
}

// RollbackMigration triggers an asynchronous rollback of the latest active migration.
func (s *Service) RollbackMigration(ctx context.Context, actor auth.Principal, databaseID, migrationID string, now time.Time) (api.MigrationExecuted, error) {
	if err := s.Access.RequireDatabase(ctx, actor, databaseID, api.AccessViewEdit); err != nil {
		return api.MigrationExecuted{}, err
	}

	lastApplied, err := s.Repository.GetLastAppliedMigration(ctx, databaseID)
	if err != nil {
		return api.MigrationExecuted{}, err
	}
	if lastApplied == nil || lastApplied.ID != migrationID {
		return api.MigrationExecuted{}, fmt.Errorf("%w: only the latest active applied migration can be rolled back", apperror.ErrConflict)
	}
	if lastApplied.IsIrreversible {
		reason := "irreversible migration cannot be rolled back"
		if lastApplied.IrreversibleReason != nil && *lastApplied.IrreversibleReason != "" {
			reason = *lastApplied.IrreversibleReason
		}
		return api.MigrationExecuted{}, fmt.Errorf("%w: %s", apperror.ErrConflict, reason)
	}
	if lastApplied.SQLDown == nil || *lastApplied.SQLDown == "" {
		return api.MigrationExecuted{}, fmt.Errorf("%w: migration has no SQL down defined", apperror.ErrConflict)
	}

	executions, err := s.Repository.ListExecutions(ctx, databaseID, false)
	var cancelledExecID *string
	if err == nil {
		for i := len(executions) - 1; i >= 0; i-- {
			if executions[i].MigrationID == migrationID && executions[i].ActionType == "FORWARD" && executions[i].Status == "SUCCEEDED" {
				id := executions[i].ID
				cancelledExecID = &id
				break
			}
		}
	}

	exec := models.MigrationExecution{
		MigrationID:           migrationID,
		DatabaseID:            databaseID,
		ActionType:            "ROLLBACK",
		RolledBackExecutionID: cancelledExecID,
		Status:                "RUNNING",
		ExecutedSQL:           *lastApplied.SQLDown,
		ExecutedBy:            actor.User.ID,
		StartedAt:             now,
	}
	if err := s.Repository.CreateExecution(ctx, &exec); err != nil {
		return api.MigrationExecuted{}, err
	}

	if s.Dispatcher == nil {
		return api.MigrationExecuted{}, fmt.Errorf("%w: queue dispatcher not configured", apperror.ErrInternal)
	}

	receipt, err := s.Dispatcher.Enqueue(ctx, contracts.ExecutionRef{
		ExecutionID: exec.ID,
		DatabaseID:  databaseID,
	})
	if err != nil {
		return api.MigrationExecuted{}, err
	}

	return api.MigrationExecuted{
		ID:          migrationID,
		Status:      models.MigrationRunning,
		ExecutedBy:  &actor.User.ID,
		JobID:       receipt.JobID,
		ExecutionID: exec.ID,
	}, nil
}

// ForwardMigration triggers forward execution of an approved migration in linear sequence.
func (s *Service) ForwardMigration(ctx context.Context, actor auth.Principal, databaseID, migrationID string, now time.Time) (api.MigrationExecuted, error) {
	if err := s.Access.RequireDatabase(ctx, actor, databaseID, api.AccessViewEdit); err != nil {
		return api.MigrationExecuted{}, err
	}

	nextCandidate, err := s.Repository.GetNextForwardCandidate(ctx, databaseID)
	if err != nil {
		return api.MigrationExecuted{}, err
	}
	if nextCandidate == nil || nextCandidate.ID != migrationID {
		return api.MigrationExecuted{}, fmt.Errorf("%w: forward must follow linear migration order", apperror.ErrConflict)
	}

	return s.ExecuteMigration(ctx, actor, databaseID, migrationID, now)
}

// ListMigrationHistory returns all execution attempts, optionally hiding rolled back pairs.
func (s *Service) ListMigrationHistory(ctx context.Context, actor auth.Principal, databaseID string, hideRollback bool) ([]api.MigrationExecutionSummary, error) {
	if err := s.Access.RequireDatabase(ctx, actor, databaseID, api.AccessViewOnly); err != nil {
		return nil, err
	}

	executions, err := s.Repository.ListExecutions(ctx, databaseID, hideRollback)
	if err != nil {
		return nil, err
	}

	result := make([]api.MigrationExecutionSummary, 0, len(executions))
	for _, e := range executions {
		result = append(result, api.MigrationExecutionSummary{
			ID:                    e.ID,
			MigrationID:           e.MigrationID,
			MigrationName:         e.Migration.Name,
			DatabaseID:            e.DatabaseID,
			ActionType:            e.ActionType,
			RolledBackExecutionID: e.RolledBackExecutionID,
			Status:                e.Status,
			ExecutedSQL:           e.ExecutedSQL,
			ExecutedBy:            e.ExecutedBy,
			ExecutedByUsername:    e.Executor.Username,
			StartedAt:             e.StartedAt,
			FinishedAt:            e.FinishedAt,
			Version:               e.Version,
			ErrorSummary:          e.ErrorSummary,
			CommittedBatches:      e.CommittedBatches,
			CommittedRows:         e.CommittedRows,
		})
	}
	return result, nil
}

// ValidateExecution satisfies jobs.ExecutionRegistry.
func (s *Service) ValidateExecution(ctx context.Context, ref contracts.ExecutionRef) (jobs.ExecutionMetadata, error) {
	exec, err := s.Repository.GetExecution(ctx, ref.ExecutionID)
	if err == nil {
		metadata := jobs.ExecutionMetadata{ActorID: exec.ExecutedBy}
		if exec.RolledBackExecutionID != nil && *exec.RolledBackExecutionID != "" {
			metadata.ReplaceBlockedExecutionID = *exec.RolledBackExecutionID
		}
		return metadata, nil
	}
	m, err := s.Repository.GetMigration(ctx, ref.DatabaseID, ref.ExecutionID)
	if err != nil {
		return jobs.ExecutionMetadata{}, err
	}
	return jobs.ExecutionMetadata{ActorID: m.CreatedBy}, nil
}

// ReadProgress satisfies jobs.ProgressReader for real-time batch progress monitoring.
func (s *Service) ReadProgress(ctx context.Context, ref contracts.ExecutionRef) (jobs.Progress, error) {
	if runner, ok := s.Runner.(*Runner); ok {
		batches, rows, _, err := runner.InspectCheckpoints(ctx, ref.DatabaseID, ref.ExecutionID)
		if err == nil && (batches > 0 || rows > 0) {
			return jobs.Progress{CommittedBatches: batches, CommittedRows: rows}, nil
		}
	}
	exec, err := s.Repository.GetExecution(ctx, ref.ExecutionID)
	if err == nil {
		return jobs.Progress{CommittedBatches: exec.CommittedBatches, CommittedRows: exec.CommittedRows}, nil
	}
	return jobs.Progress{}, err
}

// Execute satisfies jobs.Executor.
func (s *Service) Execute(ctx context.Context, ref contracts.ExecutionRef) (contracts.ExecutionResult, error) {
	exec, err := s.Repository.GetExecution(ctx, ref.ExecutionID)
	var m models.Migration
	if err == nil {
		m, _ = s.Repository.GetMigration(ctx, ref.DatabaseID, exec.MigrationID)
	} else {
		m, err = s.Repository.GetMigration(ctx, ref.DatabaseID, ref.ExecutionID)
		if err != nil {
			return contracts.ExecutionResult{State: contracts.ExecutionFailed, RetryAdvice: contracts.RetryStop, ErrorCode: err.Error()}, err
		}
	}

	isBatch := m.MigrationType == "BATCH"
	batchAttr := "id"
	if m.BatchAttribute != nil && *m.BatchAttribute != "" {
		batchAttr = *m.BatchAttribute
	}
	batchSize := 1000
	if m.BatchSize != nil && *m.BatchSize > 0 {
		batchSize = *m.BatchSize
	}

	now := time.Now()

	if exec != nil && exec.ActionType == "ROLLBACK" {
		downSQL := exec.ExecutedSQL
		if downSQL == "" && m.SQLDown != nil {
			downSQL = *m.SQLDown
		}

		if isBatch {
			runner, ok := s.Runner.(*Runner)
			if !ok {
				return contracts.ExecutionResult{State: contracts.ExecutionFailed, RetryAdvice: contracts.RetryStop}, fmt.Errorf("batch runner not available")
			}
			revertedBatches, revertedRows, bErr := runner.RevertBatch(ctx, ref.DatabaseID, ref.ExecutionID, downSQL, batchAttr, func(b, r int64, cursor string) {
				exec.CommittedBatches = b
				exec.CommittedRows = r
				_ = s.Repository.UpdateExecution(ctx, exec)
			})

			exec.FinishedAt = &now
			if bErr != nil {
				exec.Status = "PARTIALLY_REVERTED"
				errSummary := bErr.Error()
				exec.ErrorSummary = &errSummary
				_ = s.Repository.UpdateExecution(ctx, exec)
				return contracts.ExecutionResult{
					State:            contracts.ExecutionPartiallyReverted,
					CommittedBatches: revertedBatches,
					CommittedRows:    revertedRows,
					RetryAdvice:      contracts.RetryResume,
					BlocksDatabase:   true,
					ErrorCode:        bErr.Error(),
				}, bErr
			}

			exec.Status = "SUCCEEDED"
			_ = s.Repository.UpdateExecution(ctx, exec)

			_, _ = s.Repository.MarkRolledBack(ctx, ref.DatabaseID, m.ID, now)

			return contracts.ExecutionResult{
				State:            contracts.ExecutionSucceeded,
				CommittedBatches: revertedBatches,
				CommittedRows:    revertedRows,
			}, nil
		}

		execErr := s.Runner.Execute(ctx, ref.DatabaseID, downSQL)
		exec.FinishedAt = &now
		if execErr != nil {
			exec.Status = "FAILED"
			errSummary := execErr.Error()
			exec.ErrorSummary = &errSummary
			_ = s.Repository.UpdateExecution(ctx, exec)
			return contracts.ExecutionResult{State: contracts.ExecutionFailed, RetryAdvice: contracts.RetryStop, ErrorCode: execErr.Error()}, execErr
		}

		exec.Status = "SUCCEEDED"
		_ = s.Repository.UpdateExecution(ctx, exec)
		_, _ = s.Repository.MarkRolledBack(ctx, ref.DatabaseID, m.ID, now)

		return contracts.ExecutionResult{State: contracts.ExecutionSucceeded}, nil
	}

	// FORWARD execution
	sqlUp := m.SQLText
	if exec != nil && exec.ExecutedSQL != "" {
		sqlUp = exec.ExecutedSQL
	}

	if isBatch {
		runner, ok := s.Runner.(*Runner)
		if !ok {
			return contracts.ExecutionResult{State: contracts.ExecutionFailed, RetryAdvice: contracts.RetryStop}, fmt.Errorf("batch runner not available")
		}

		startCursor := ""
		if exec != nil && exec.LastCursorValue != nil {
			startCursor = *exec.LastCursorValue
		}

		committedBatches, committedRows, lastCursor, bErr := runner.ExecuteBatch(ctx, ref.DatabaseID, ref.ExecutionID, sqlUp, batchAttr, batchSize, startCursor, func(b, r int64, cursor string) {
			if exec != nil {
				exec.CommittedBatches = b
				exec.CommittedRows = r
				exec.LastCursorValue = &cursor
				_ = s.Repository.UpdateExecution(ctx, exec)
			}
		})

		if exec != nil {
			exec.CommittedBatches = committedBatches
			exec.CommittedRows = committedRows
			exec.LastCursorValue = &lastCursor
			exec.FinishedAt = &now
		}

		if bErr != nil {
			if exec != nil {
				exec.Status = "PARTIAL"
				errSummary := bErr.Error()
				exec.ErrorSummary = &errSummary
				_ = s.Repository.UpdateExecution(ctx, exec)
			}
			return contracts.ExecutionResult{
				State:            contracts.ExecutionPartial,
				CommittedBatches: committedBatches,
				CommittedRows:    committedRows,
				RetryAdvice:      contracts.RetryResume,
				BlocksDatabase:   true,
				ErrorCode:        bErr.Error(),
			}, bErr
		}

		if exec != nil {
			exec.Status = "SUCCEEDED"
			_ = s.Repository.UpdateExecution(ctx, exec)
		}

		_, _ = s.Repository.MarkApplied(ctx, ref.DatabaseID, m.ID, m.CreatedBy, now)
		return contracts.ExecutionResult{
			State:            contracts.ExecutionSucceeded,
			CommittedBatches: committedBatches,
			CommittedRows:    committedRows,
		}, nil
	}

	execErr := s.Runner.Execute(ctx, ref.DatabaseID, sqlUp)
	if exec != nil {
		exec.FinishedAt = &now
	}

	if execErr != nil {
		if errors.Is(execErr, apperror.ErrTransactionOutcomeUnknown) {
			if exec != nil {
				exec.Status = "UNKNOWN"
				errSummary := execErr.Error()
				exec.ErrorSummary = &errSummary
				_ = s.Repository.UpdateExecution(ctx, exec)
			}
			return contracts.ExecutionResult{
				State:          contracts.ExecutionUnknown,
				RetryAdvice:    contracts.RetryReconcile,
				BlocksDatabase: true,
				ErrorCode:      execErr.Error(),
			}, execErr
		}

		if exec != nil {
			exec.Status = "FAILED"
			errSummary := execErr.Error()
			exec.ErrorSummary = &errSummary
			_ = s.Repository.UpdateExecution(ctx, exec)
		}
		_, _ = s.Repository.MarkFailed(ctx, ref.DatabaseID, m.ID, m.CreatedBy, execErr.Error(), now)
		return contracts.ExecutionResult{
			State:       contracts.ExecutionFailed,
			RetryAdvice: contracts.RetryStop,
			ErrorCode:   execErr.Error(),
		}, execErr
	}

	if exec != nil {
		exec.Status = "SUCCEEDED"
		_ = s.Repository.UpdateExecution(ctx, exec)
	}
	_, _ = s.Repository.MarkApplied(ctx, ref.DatabaseID, m.ID, m.CreatedBy, now)

	return contracts.ExecutionResult{
		State: contracts.ExecutionSucceeded,
	}, nil
}

// Inspect satisfies jobs.Executor.
func (s *Service) Inspect(ctx context.Context, ref contracts.ExecutionRef) (contracts.ExecutionResult, error) {
	runner, ok := s.Runner.(*Runner)
	if ok {
		batches, rows, _, err := runner.InspectCheckpoints(ctx, ref.DatabaseID, ref.ExecutionID)
		if err == nil && batches > 0 {
			return contracts.ExecutionResult{
				State:            contracts.ExecutionPartial,
				CommittedBatches: batches,
				CommittedRows:    rows,
				RetryAdvice:      contracts.RetryResume,
				BlocksDatabase:   true,
			}, nil
		}
	}

	m, err := s.Repository.GetMigration(ctx, ref.DatabaseID, ref.ExecutionID)
	if err == nil && m.Status == models.MigrationApplied {
		return contracts.ExecutionResult{State: contracts.ExecutionSucceeded}, nil
	}

	return contracts.ExecutionResult{State: contracts.ExecutionUnknown, BlocksDatabase: true}, nil
}
