package migration

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/api"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/apperror"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/auth"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/contracts"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/jobs"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/models"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/telemetry"
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
		action := app.Action
		if action == "" {
			action = "MIGRATE"
		}
		var leaderID, leaderUsername string
		if app.LeaderID != nil {
			leaderID = *app.LeaderID
		}
		if app.Leader != nil {
			leaderUsername = app.Leader.Username
		}
		approvalItems = append(approvalItems, api.MigrationApprovalItem{
			ID:             app.ID,
			GroupID:        app.GroupID,
			GroupName:      app.Group.Name,
			LeaderID:       leaderID,
			LeaderUsername: leaderUsername,
			Status:         string(app.Status),
			Action:         action,
			Comment:        app.Comment,
			DecidedAt:      app.DecidedAt,
		})
	}

	lastApplied, _ := s.Repository.GetLastAppliedMigration(ctx, databaseID)
	reversibleTop := lastApplied != nil && lastApplied.ID == migrationID &&
		!m.IsIrreversible && m.SQLDown != nil && *m.SQLDown != ""
	canRequestRollback := reversibleTop && m.Status == models.MigrationApplied
	canRollback := reversibleTop && m.Status == models.MigrationRollbackApproved

	nextForward, _ := s.Repository.GetNextForwardCandidate(ctx, databaseID)
	canRequestForward := nextForward != nil && nextForward.ID == migrationID &&
		m.Status == models.MigrationRolledBack
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
		CanRequestRollback: canRequestRollback,
		CanRequestForward:  canRequestForward,
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

	telemetry.Log(ctx, slog.LevelInfo, "migration.created",
		"actor", actor.User.ID,
		"database_id", databaseID,
		"migration_id", saved.ID,
		"migration_name", saved.Name,
		"migration_type", saved.MigrationType,
		"is_irreversible", saved.IsIrreversible,
		"status", string(saved.Status),
	)

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

func (s *Service) buildPendingApprovals(ctx context.Context, databaseID, migrationID, action string, now time.Time) ([]models.MigrationApproval, error) {
	groups, err := s.Repository.GetDatabaseGroupsWithLeaders(ctx, databaseID)
	if err != nil {
		return nil, err
	}
	if len(groups) == 0 {
		return nil, fmt.Errorf("%w: no groups have access to this database", apperror.ErrConflict)
	}

	approvals := make([]models.MigrationApproval, 0, len(groups))
	for _, g := range groups {
		var leaderID *string
		if g.LeaderID != nil && *g.LeaderID != "" {
			id := *g.LeaderID
			leaderID = &id
		}
		approvals = append(approvals, models.MigrationApproval{
			MigrationID: migrationID,
			GroupID:     g.ID,
			LeaderID:    leaderID,
			Status:      models.ApprovalPending,
			Action:      action,
			CreatedAt:   now,
		})
	}
	return approvals, nil
}

func (s *Service) validateApprovedByCurrentGroups(ctx context.Context, databaseID string, approvals []models.MigrationApproval, requiredAction string) error {
	groups, err := s.Repository.GetDatabaseGroupsWithLeaders(ctx, databaseID)
	if err != nil {
		return err
	}
	if len(groups) == 0 {
		return fmt.Errorf("%w: no groups have access to this database", apperror.ErrConflict)
	}
	approvedLeaders := make(map[string]string, len(approvals))
	for _, a := range approvals {
		if a.Status != models.ApprovalApproved || a.LeaderID == nil {
			continue
		}
		if requiredAction != "" {
			if a.Action != requiredAction {
				continue
			}
		} else if a.Action == "ROLLBACK" {
			continue
		}
		approvedLeaders[a.GroupID] = *a.LeaderID
	}
	for _, g := range groups {
		if g.LeaderID == nil || *g.LeaderID == "" {
			return fmt.Errorf("%w: group %q has no designated leader", apperror.ErrConflict, g.Name)
		}
		approvedLeader, ok := approvedLeaders[g.ID]
		if !ok || approvedLeader != *g.LeaderID {
			return fmt.Errorf("%w: missing approval from current leader of group %q", apperror.ErrConflict, g.Name)
		}
	}
	return nil
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

	approvals, err := s.buildPendingApprovals(ctx, databaseID, migrationID, "MIGRATE", now)
	if err != nil {
		return api.MigrationDetail{}, err
	}

	if err := s.Repository.SubmitForReview(ctx, databaseID, migrationID, approvals); err != nil {
		return api.MigrationDetail{}, err
	}

	telemetry.Log(ctx, slog.LevelInfo, "migration.review_submitted",
		"actor", actor.User.ID,
		"database_id", databaseID,
		"migration_id", migrationID,
		"required_approvals", len(approvals),
		"status", string(models.MigrationPendingReview),
	)

	return s.GetMigration(ctx, actor, databaseID, migrationID)
}

// SubmitRollbackReview requests leader approval before rolling back an applied migration.
func (s *Service) SubmitRollbackReview(ctx context.Context, actor auth.Principal, databaseID, migrationID string, now time.Time) (api.MigrationDetail, error) {
	if err := s.Access.RequireDatabase(ctx, actor, databaseID, api.AccessViewEdit); err != nil {
		return api.MigrationDetail{}, err
	}

	lastApplied, err := s.Repository.GetLastAppliedMigration(ctx, databaseID)
	if err != nil {
		return api.MigrationDetail{}, err
	}
	if lastApplied == nil || lastApplied.ID != migrationID || lastApplied.Status != models.MigrationApplied {
		return api.MigrationDetail{}, fmt.Errorf("%w: only the latest applied migration can be submitted for rollback review", apperror.ErrConflict)
	}
	if lastApplied.IsIrreversible {
		reason := "irreversible migration cannot be rolled back"
		if lastApplied.IrreversibleReason != nil && *lastApplied.IrreversibleReason != "" {
			reason = *lastApplied.IrreversibleReason
		}
		return api.MigrationDetail{}, fmt.Errorf("%w: %s", apperror.ErrConflict, reason)
	}
	if lastApplied.SQLDown == nil || *lastApplied.SQLDown == "" {
		return api.MigrationDetail{}, fmt.Errorf("%w: migration has no SQL down defined", apperror.ErrConflict)
	}

	approvals, err := s.buildPendingApprovals(ctx, databaseID, migrationID, "ROLLBACK", now)
	if err != nil {
		return api.MigrationDetail{}, err
	}

	if err := s.Repository.SubmitRollbackReview(ctx, databaseID, migrationID, approvals); err != nil {
		return api.MigrationDetail{}, err
	}

	telemetry.Log(ctx, slog.LevelInfo, "migration.rollback_review_submitted",
		"actor", actor.User.ID,
		"database_id", databaseID,
		"migration_id", migrationID,
		"required_approvals", len(approvals),
		"status", string(models.MigrationPendingReview),
	)

	return s.GetMigration(ctx, actor, databaseID, migrationID)
}

// SubmitForwardReview requests leader approval before re-applying a rolled-back migration in linear order.
func (s *Service) SubmitForwardReview(ctx context.Context, actor auth.Principal, databaseID, migrationID string, now time.Time) (api.MigrationDetail, error) {
	if err := s.Access.RequireDatabase(ctx, actor, databaseID, api.AccessViewEdit); err != nil {
		return api.MigrationDetail{}, err
	}

	m, err := s.Repository.GetMigration(ctx, databaseID, migrationID)
	if err != nil {
		return api.MigrationDetail{}, err
	}
	if m.Status != models.MigrationRolledBack {
		return api.MigrationDetail{}, fmt.Errorf("%w: migration must be ROLLED_BACK to submit for forward review", apperror.ErrConflict)
	}

	nextCandidate, err := s.Repository.GetNextForwardCandidate(ctx, databaseID)
	if err != nil {
		return api.MigrationDetail{}, err
	}
	if nextCandidate == nil || nextCandidate.ID != migrationID {
		return api.MigrationDetail{}, fmt.Errorf("%w: forward review must follow linear migration order", apperror.ErrConflict)
	}

	approvals, err := s.buildPendingApprovals(ctx, databaseID, migrationID, "FORWARD", now)
	if err != nil {
		return api.MigrationDetail{}, err
	}

	if err := s.Repository.SubmitForwardReview(ctx, databaseID, migrationID, approvals); err != nil {
		return api.MigrationDetail{}, err
	}

	telemetry.Log(ctx, slog.LevelInfo, "migration.forward_review_submitted",
		"actor", actor.User.ID,
		"database_id", databaseID,
		"migration_id", migrationID,
		"required_approvals", len(approvals),
		"status", string(models.MigrationPendingReview),
	)

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

	reviewAction := approvals[0].Action
	if reviewAction == "" {
		reviewAction = "MIGRATE"
	}

	for _, app := range approvals {
		if err := s.Repository.UpdateApprovalStatus(ctx, app.ID, models.ApprovalApproved, input.Comment, now); err != nil {
			return api.MigrationDetail{}, err
		}
	}

	var approvedAll bool
	if reviewAction == "ROLLBACK" {
		approvedAll, _ = s.Repository.CheckAndSetRollbackApproved(ctx, databaseID, migrationID)
	} else {
		approvedAll, _ = s.Repository.CheckAndSetApproved(ctx, databaseID, migrationID)
	}

	telemetry.Log(ctx, slog.LevelInfo, "migration.approved",
		"actor", actor.User.ID,
		"database_id", databaseID,
		"migration_id", migrationID,
		"action", reviewAction,
		"approved_groups", len(approvals),
		"fully_approved", approvedAll,
		"status", "approved",
	)

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

	reviewAction := approvals[0].Action
	if reviewAction == "" {
		reviewAction = "MIGRATE"
	}

	for _, app := range approvals {
		_ = s.Repository.UpdateApprovalStatus(ctx, app.ID, models.ApprovalRejected, input.Comment, now)
	}

	var rejectErr error
	var resultingStatus models.MigrationStatus
	switch reviewAction {
	case "ROLLBACK":
		rejectErr = s.Repository.RejectRollbackReview(ctx, databaseID, migrationID)
		resultingStatus = models.MigrationApplied
	case "FORWARD":
		rejectErr = s.Repository.RejectForwardReview(ctx, databaseID, migrationID)
		resultingStatus = models.MigrationRolledBack
	default:
		rejectErr = s.Repository.RejectMigration(ctx, databaseID, migrationID)
		resultingStatus = models.MigrationRejected
	}
	if rejectErr != nil {
		return api.MigrationDetail{}, rejectErr
	}

	telemetry.Log(ctx, slog.LevelInfo, "migration.rejected",
		"actor", actor.User.ID,
		"database_id", databaseID,
		"migration_id", migrationID,
		"action", reviewAction,
		"status", string(resultingStatus),
	)

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
	if err := s.validateApprovedByCurrentGroups(ctx, databaseID, approvals, ""); err != nil {
		return api.MigrationExecuted{}, err
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

	telemetry.Log(ctx, slog.LevelInfo, "migration.queued",
		"actor", actor.User.ID,
		"database_id", databaseID,
		"migration_id", m.ID,
		"execution_id", exec.ID,
		"job_id", receipt.JobID,
		"action_type", exec.ActionType,
		"status", string(receipt.Status),
	)

	return api.MigrationExecuted{
		ID:          m.ID,
		Status:      models.MigrationRunning,
		ExecutedBy:  &actor.User.ID,
		JobID:       receipt.JobID,
		ExecutionID: exec.ID,
	}, nil
}

// RollbackMigration triggers an asynchronous rollback of the latest active migration after leader approval.
func (s *Service) RollbackMigration(ctx context.Context, actor auth.Principal, databaseID, migrationID string, now time.Time) (api.MigrationExecuted, error) {
	if err := s.Access.RequireDatabase(ctx, actor, databaseID, api.AccessViewEdit); err != nil {
		return api.MigrationExecuted{}, err
	}

	m, approvals, err := s.Repository.GetMigrationWithApprovals(ctx, databaseID, migrationID)
	if err != nil {
		return api.MigrationExecuted{}, err
	}
	if m.Status != models.MigrationRollbackApproved {
		return api.MigrationExecuted{}, fmt.Errorf("%w: rollback must be approved by group leaders before execution", apperror.ErrConflict)
	}

	lastApplied, err := s.Repository.GetLastAppliedMigration(ctx, databaseID)
	if err != nil {
		return api.MigrationExecuted{}, err
	}
	if lastApplied == nil || lastApplied.ID != migrationID {
		return api.MigrationExecuted{}, fmt.Errorf("%w: only the latest active applied migration can be rolled back", apperror.ErrConflict)
	}
	if m.IsIrreversible {
		reason := "irreversible migration cannot be rolled back"
		if m.IrreversibleReason != nil && *m.IrreversibleReason != "" {
			reason = *m.IrreversibleReason
		}
		return api.MigrationExecuted{}, fmt.Errorf("%w: %s", apperror.ErrConflict, reason)
	}
	if m.SQLDown == nil || *m.SQLDown == "" {
		return api.MigrationExecuted{}, fmt.Errorf("%w: migration has no SQL down defined", apperror.ErrConflict)
	}

	// Revalidate that every group currently granted access to this database has a leader and approved this rollback.
	if err := s.validateApprovedByCurrentGroups(ctx, databaseID, approvals, "ROLLBACK"); err != nil {
		return api.MigrationExecuted{}, err
	}

	executions, err := s.Repository.ListExecutions(ctx, databaseID, false)
	var cancelledExecID *string
	if err == nil {
		for i := len(executions) - 1; i >= 0; i-- {
			if executions[i].MigrationID == migrationID && (executions[i].ActionType == "FORWARD" || executions[i].ActionType == "RETRY") && executions[i].Status == "SUCCEEDED" {
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
		ExecutedSQL:           *m.SQLDown,
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

	_, _ = s.Repository.MarkRunning(ctx, databaseID, migrationID, actor.User.ID, now)

	telemetry.Log(ctx, slog.LevelInfo, "migration.rollback_queued",
		"actor", actor.User.ID,
		"database_id", databaseID,
		"migration_id", migrationID,
		"execution_id", exec.ID,
		"job_id", receipt.JobID,
		"action_type", exec.ActionType,
		"status", string(receipt.Status),
	)

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

// ResumeExecution enqueues a retry/resume job from the last committed batch checkpoint.
func (s *Service) ResumeExecution(ctx context.Context, actor auth.Principal, databaseID, executionID string, now time.Time) (api.MigrationExecuted, error) {
	if err := s.Access.RequireDatabase(ctx, actor, databaseID, api.AccessViewEdit); err != nil {
		return api.MigrationExecuted{}, err
	}

	exec, err := s.Repository.GetExecution(ctx, executionID)
	if err != nil {
		return api.MigrationExecuted{}, err
	}
	if exec.DatabaseID != databaseID {
		return api.MigrationExecuted{}, apperror.ErrNotFound
	}
	if exec.Status != "PARTIAL" && exec.Status != "PARTIALLY_REVERTED" {
		return api.MigrationExecuted{}, fmt.Errorf("%w: only PARTIAL or PARTIALLY_REVERTED executions can be resumed", apperror.ErrConflict)
	}

	m, approvals, err := s.Repository.GetMigrationWithApprovals(ctx, databaseID, exec.MigrationID)
	if err != nil {
		return api.MigrationExecuted{}, err
	}
	if m.MigrationType != "BATCH" {
		return api.MigrationExecuted{}, fmt.Errorf("%w: resume from checkpoint is only available for BATCH migrations", apperror.ErrConflict)
	}

	requiredAction := ""
	if exec.ActionType == "ROLLBACK" {
		requiredAction = "ROLLBACK"
	}
	if err := s.validateApprovedByCurrentGroups(ctx, databaseID, approvals, requiredAction); err != nil {
		return api.MigrationExecuted{}, err
	}

	actionType := "RETRY"
	if exec.Status == "PARTIALLY_REVERTED" {
		if exec.ActionType == "ROLLBACK" {
			actionType = "ROLLBACK"
		} else {
			actionType = "REVERT_BATCH"
		}
	}

	prevID := exec.ID
	newExec := models.MigrationExecution{
		MigrationID:           exec.MigrationID,
		DatabaseID:            databaseID,
		ActionType:            actionType,
		RolledBackExecutionID: &prevID,
		Status:                "RUNNING",
		ExecutedSQL:           exec.ExecutedSQL,
		ExecutedBy:            actor.User.ID,
		StartedAt:             now,
		CommittedBatches:      exec.CommittedBatches,
		CommittedRows:         exec.CommittedRows,
		LastCursorValue:       exec.LastCursorValue,
	}
	if err := s.Repository.CreateExecution(ctx, &newExec); err != nil {
		return api.MigrationExecuted{}, err
	}

	if s.Dispatcher == nil {
		return api.MigrationExecuted{}, fmt.Errorf("%w: queue dispatcher not configured", apperror.ErrInternal)
	}

	receipt, err := s.Dispatcher.Enqueue(ctx, contracts.ExecutionRef{
		ExecutionID: newExec.ID,
		DatabaseID:  databaseID,
	})
	if err != nil {
		return api.MigrationExecuted{}, err
	}

	_, _ = s.Repository.MarkRunning(ctx, databaseID, m.ID, actor.User.ID, now)

	telemetry.Log(ctx, slog.LevelInfo, "migration.resume_queued",
		"actor", actor.User.ID,
		"database_id", databaseID,
		"migration_id", m.ID,
		"execution_id", newExec.ID,
		"previous_execution_id", exec.ID,
		"job_id", receipt.JobID,
		"action_type", newExec.ActionType,
		"status", string(receipt.Status),
	)

	return api.MigrationExecuted{
		ID:          m.ID,
		Status:      models.MigrationRunning,
		ExecutedBy:  &actor.User.ID,
		JobID:       receipt.JobID,
		ExecutionID: newExec.ID,
	}, nil
}

// CancelBatch creates an asynchronous REVERT_BATCH compensation job for a PARTIAL forward batch execution.
func (s *Service) CancelBatch(ctx context.Context, actor auth.Principal, databaseID, executionID string, now time.Time) (api.MigrationExecuted, error) {
	if err := s.Access.RequireDatabase(ctx, actor, databaseID, api.AccessViewEdit); err != nil {
		return api.MigrationExecuted{}, err
	}

	exec, err := s.Repository.GetExecution(ctx, executionID)
	if err != nil {
		return api.MigrationExecuted{}, err
	}
	if exec.DatabaseID != databaseID {
		return api.MigrationExecuted{}, apperror.ErrNotFound
	}
	if exec.Status != "PARTIAL" || (exec.ActionType != "FORWARD" && exec.ActionType != "RETRY") {
		return api.MigrationExecuted{}, fmt.Errorf("%w: only PARTIAL forward/retry batch executions can be compensated", apperror.ErrConflict)
	}

	m, approvals, err := s.Repository.GetMigrationWithApprovals(ctx, databaseID, exec.MigrationID)
	if err != nil {
		return api.MigrationExecuted{}, err
	}
	if m.MigrationType != "BATCH" {
		return api.MigrationExecuted{}, fmt.Errorf("%w: batch compensation is only available for BATCH migrations", apperror.ErrConflict)
	}
	if m.IsIrreversible || m.SQLDown == nil || *m.SQLDown == "" {
		return api.MigrationExecuted{}, fmt.Errorf("%w: compensation is unsafe without approved SQL down; create a corrective migration instead", apperror.ErrConflict)
	}

	if err := s.validateApprovedByCurrentGroups(ctx, databaseID, approvals, ""); err != nil {
		return api.MigrationExecuted{}, err
	}

	prevID := exec.ID
	newExec := models.MigrationExecution{
		MigrationID:           exec.MigrationID,
		DatabaseID:            databaseID,
		ActionType:            "REVERT_BATCH",
		RolledBackExecutionID: &prevID,
		Status:                "RUNNING",
		ExecutedSQL:           *m.SQLDown,
		ExecutedBy:            actor.User.ID,
		StartedAt:             now,
		LastCursorValue:       exec.LastCursorValue,
	}
	if err := s.Repository.CreateExecution(ctx, &newExec); err != nil {
		return api.MigrationExecuted{}, err
	}

	if s.Dispatcher == nil {
		return api.MigrationExecuted{}, fmt.Errorf("%w: queue dispatcher not configured", apperror.ErrInternal)
	}

	receipt, err := s.Dispatcher.Enqueue(ctx, contracts.ExecutionRef{
		ExecutionID: newExec.ID,
		DatabaseID:  databaseID,
	})
	if err != nil {
		return api.MigrationExecuted{}, err
	}

	_, _ = s.Repository.MarkRunning(ctx, databaseID, m.ID, actor.User.ID, now)

	telemetry.Log(ctx, slog.LevelInfo, "migration.cancel_batch_queued",
		"actor", actor.User.ID,
		"database_id", databaseID,
		"migration_id", m.ID,
		"execution_id", newExec.ID,
		"partial_execution_id", exec.ID,
		"job_id", receipt.JobID,
		"action_type", newExec.ActionType,
		"status", string(receipt.Status),
	)

	return api.MigrationExecuted{
		ID:          m.ID,
		Status:      models.MigrationRunning,
		ExecutedBy:  &actor.User.ID,
		JobID:       receipt.JobID,
		ExecutionID: newExec.ID,
	}, nil
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
			LastCursorValue:       e.LastCursorValue,
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
			prev, prevErr := s.Repository.GetExecution(ctx, *exec.RolledBackExecutionID)
			// Only hand off blocked database gate when the referenced execution is NOT already SUCCEEDED.
			if prevErr != nil || (prev != nil && prev.Status != "SUCCEEDED") {
				metadata.ReplaceBlockedExecutionID = *exec.RolledBackExecutionID
			}
		}
		return metadata, nil
	}
	m, err := s.Repository.GetMigration(ctx, ref.DatabaseID, ref.ExecutionID)
	if err != nil {
		return jobs.ExecutionMetadata{}, err
	}
	return jobs.ExecutionMetadata{ActorID: m.CreatedBy}, nil
}

func (s *Service) resolveCheckpointExecutionID(ctx context.Context, exec *models.MigrationExecution) string {
	if exec == nil {
		return ""
	}
	cur := exec
	for range 20 {
		if cur.ActionType == "FORWARD" || cur.RolledBackExecutionID == nil || *cur.RolledBackExecutionID == "" {
			return cur.ID
		}
		prev, err := s.Repository.GetExecution(ctx, *cur.RolledBackExecutionID)
		if err != nil || prev == nil {
			return cur.ID
		}
		cur = prev
	}
	return cur.ID
}

// ReadProgress satisfies jobs.ProgressReader for real-time batch progress monitoring.
func (s *Service) ReadProgress(ctx context.Context, ref contracts.ExecutionRef) (jobs.Progress, error) {
	exec, err := s.Repository.GetExecution(ctx, ref.ExecutionID)
	checkpointExecID := ref.ExecutionID
	if err == nil && exec != nil {
		checkpointExecID = s.resolveCheckpointExecutionID(ctx, exec)
	}
	if runner, ok := s.Runner.(*Runner); ok {
		batches, rows, _, rErr := runner.InspectCheckpoints(ctx, ref.DatabaseID, checkpointExecID)
		if rErr == nil && (batches > 0 || rows > 0) {
			return jobs.Progress{CommittedBatches: batches, CommittedRows: rows}, nil
		}
	}
	if err == nil && exec != nil {
		return jobs.Progress{CommittedBatches: exec.CommittedBatches, CommittedRows: exec.CommittedRows}, nil
	}
	return jobs.Progress{}, err
}

// Execute satisfies jobs.Executor.
func (s *Service) Execute(ctx context.Context, ref contracts.ExecutionRef) (res contracts.ExecutionResult, retErr error) {
	started := time.Now()
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

	actorID := m.CreatedBy
	actionType := "FORWARD"
	executedSQL := m.SQLText
	var version int64
	if m.Version != nil {
		version = *m.Version
	}
	if exec != nil {
		if exec.ExecutedBy != "" {
			actorID = exec.ExecutedBy
		}
		if exec.ActionType != "" {
			actionType = exec.ActionType
		}
		if exec.ExecutedSQL != "" {
			executedSQL = exec.ExecutedSQL
		} else if actionType == "ROLLBACK" && m.SQLDown != nil {
			executedSQL = *m.SQLDown
		}
		if exec.Version != nil {
			version = *exec.Version
		}
	}

	mode := "regular"
	if m.MigrationType == "BATCH" {
		mode = "batch"
	}

	ctx = telemetry.Correlate(ctx, actorID, ref.DatabaseID, "", ref.ExecutionID)
	ctx = telemetry.WithMigration(ctx, m.ID)

	telemetry.Log(ctx, slog.LevelInfo, "migration.execution_started",
		"migration_id", m.ID,
		"execution_id", ref.ExecutionID,
		"version", version,
		"actor", actorID,
		"action_type", actionType,
		"mode", mode,
		"query", executedSQL,
		"status", "started",
	)

	defer func() {
		dur := time.Since(started)
		status := strings.ToLower(string(res.State))
		if status == "" {
			status = "failed"
		}
		telemetry.ObserveMigrationDuration(mode, actionType, status, dur)
		level := slog.LevelInfo
		errMsg := ""
		sqlState := ""
		if retErr != nil || res.State != contracts.ExecutionSucceeded {
			level = slog.LevelError
			if retErr != nil {
				errMsg = telemetry.SanitizeValue(retErr.Error())
				sqlState = telemetry.SQLState(retErr)
			} else if res.ErrorCode != "" {
				errMsg = res.ErrorCode
			}
		}
		telemetry.Log(ctx, level, "migration.execution_finished",
			"migration_id", m.ID,
			"execution_id", ref.ExecutionID,
			"version", version,
			"actor", actorID,
			"action_type", actionType,
			"mode", mode,
			"query", executedSQL,
			"status", status,
			"duration_ms", float64(dur)/float64(time.Millisecond),
			"committed_batches", res.CommittedBatches,
			"committed_rows", res.CommittedRows,
			"error", errMsg,
			"sqlstate", sqlState,
		)
	}()

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

	if exec != nil && (exec.ActionType == "ROLLBACK" || exec.ActionType == "REVERT_BATCH") {
		downSQL := exec.ExecutedSQL
		if downSQL == "" && m.SQLDown != nil {
			downSQL = *m.SQLDown
		}

		if isBatch {
			runner, ok := s.Runner.(*Runner)
			if !ok {
				return contracts.ExecutionResult{State: contracts.ExecutionFailed, RetryAdvice: contracts.RetryStop}, fmt.Errorf("batch runner not available")
			}
			checkpointExecID := s.resolveCheckpointExecutionID(ctx, exec)
			baseBatches := exec.CommittedBatches
			baseRows := exec.CommittedRows
			revertedBatches, revertedRows, bErr := runner.RevertBatch(ctx, ref.DatabaseID, checkpointExecID, downSQL, batchAttr, batchSize, func(b, r int64, cursor string) {
				exec.CommittedBatches = baseBatches + b
				exec.CommittedRows = baseRows + r
				exec.LastCursorValue = &cursor
				_ = s.Repository.UpdateExecution(ctx, exec)
			})

			now = time.Now()
			exec.CommittedBatches = baseBatches + revertedBatches
			exec.CommittedRows = baseRows + revertedRows
			exec.FinishedAt = &now
			if bErr != nil {
				exec.Status = "PARTIALLY_REVERTED"
				errSummary := bErr.Error()
				exec.ErrorSummary = &errSummary
				_ = s.Repository.UpdateExecution(ctx, exec)
				return contracts.ExecutionResult{
					State:            contracts.ExecutionPartiallyReverted,
					CommittedBatches: exec.CommittedBatches,
					CommittedRows:    exec.CommittedRows,
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
				RetryAdvice:      contracts.RetryStop,
				CommittedBatches: exec.CommittedBatches,
				CommittedRows:    exec.CommittedRows,
			}, nil
		}

		execErr := s.Runner.Execute(ctx, ref.DatabaseID, downSQL)
		now = time.Now()
		exec.FinishedAt = &now
		if execErr != nil {
			exec.Status = "FAILED"
			errSummary := execErr.Error()
			exec.ErrorSummary = &errSummary
			_ = s.Repository.UpdateExecution(ctx, exec)
			if exec.ActionType == "ROLLBACK" {
				_, _ = s.Repository.RestoreApplied(ctx, ref.DatabaseID, m.ID, now)
			} else {
				_, _ = s.Repository.MarkFailed(ctx, ref.DatabaseID, m.ID, actorID, execErr.Error(), now)
			}
			return contracts.ExecutionResult{State: contracts.ExecutionFailed, RetryAdvice: contracts.RetryStop, ErrorCode: execErr.Error()}, execErr
		}

		exec.Status = "SUCCEEDED"
		_ = s.Repository.UpdateExecution(ctx, exec)
		_, _ = s.Repository.MarkRolledBack(ctx, ref.DatabaseID, m.ID, now)

		return contracts.ExecutionResult{State: contracts.ExecutionSucceeded, RetryAdvice: contracts.RetryStop}, nil
	}

	// FORWARD / RETRY execution
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

		checkpointExecID := ref.ExecutionID
		if exec != nil {
			checkpointExecID = s.resolveCheckpointExecutionID(ctx, exec)
		}

		committedBatches, committedRows, lastCursor, bErr := runner.ExecuteBatch(ctx, ref.DatabaseID, checkpointExecID, sqlUp, batchAttr, batchSize, startCursor, func(b, r int64, cursor string) {
			if exec != nil {
				exec.CommittedBatches = b
				exec.CommittedRows = r
				exec.LastCursorValue = &cursor
				_ = s.Repository.UpdateExecution(ctx, exec)
			}
		})

		now = time.Now()
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

		_, _ = s.Repository.MarkApplied(ctx, ref.DatabaseID, m.ID, actorID, now)
		return contracts.ExecutionResult{
			State:            contracts.ExecutionSucceeded,
			RetryAdvice:      contracts.RetryStop,
			CommittedBatches: committedBatches,
			CommittedRows:    committedRows,
		}, nil
	}

	execErr := s.Runner.Execute(ctx, ref.DatabaseID, sqlUp)
	now = time.Now()
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
		_, _ = s.Repository.MarkFailed(ctx, ref.DatabaseID, m.ID, actorID, execErr.Error(), now)
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
	_, _ = s.Repository.MarkApplied(ctx, ref.DatabaseID, m.ID, actorID, now)

	return contracts.ExecutionResult{
		State:       contracts.ExecutionSucceeded,
		RetryAdvice: contracts.RetryStop,
	}, nil
}

// Inspect satisfies jobs.Executor.
func (s *Service) Inspect(ctx context.Context, ref contracts.ExecutionRef) (contracts.ExecutionResult, error) {
	exec, execErr := s.Repository.GetExecution(ctx, ref.ExecutionID)
	if execErr == nil && exec != nil {
		switch exec.Status {
		case "SUCCEEDED":
			return contracts.ExecutionResult{
				State:            contracts.ExecutionSucceeded,
				RetryAdvice:      contracts.RetryStop,
				CommittedBatches: exec.CommittedBatches,
				CommittedRows:    exec.CommittedRows,
			}, nil
		case "FAILED":
			return contracts.ExecutionResult{
				State:            contracts.ExecutionFailed,
				RetryAdvice:      contracts.RetryStop,
				CommittedBatches: exec.CommittedBatches,
				CommittedRows:    exec.CommittedRows,
			}, nil
		case "PARTIALLY_REVERTED":
			return contracts.ExecutionResult{
				State:            contracts.ExecutionPartiallyReverted,
				CommittedBatches: exec.CommittedBatches,
				CommittedRows:    exec.CommittedRows,
				RetryAdvice:      contracts.RetryResume,
				BlocksDatabase:   true,
			}, nil
		case "PARTIAL":
			return contracts.ExecutionResult{
				State:            contracts.ExecutionPartial,
				CommittedBatches: exec.CommittedBatches,
				CommittedRows:    exec.CommittedRows,
				RetryAdvice:      contracts.RetryResume,
				BlocksDatabase:   true,
			}, nil
		}
	}

	checkpointExecID := ref.ExecutionID
	if execErr == nil && exec != nil {
		checkpointExecID = s.resolveCheckpointExecutionID(ctx, exec)
	}
	runner, ok := s.Runner.(*Runner)
	if ok {
		batches, rows, _, err := runner.InspectCheckpoints(ctx, ref.DatabaseID, checkpointExecID)
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
		return contracts.ExecutionResult{State: contracts.ExecutionSucceeded, RetryAdvice: contracts.RetryStop}, nil
	}

	return contracts.ExecutionResult{State: contracts.ExecutionUnknown, RetryAdvice: contracts.RetryReconcile, BlocksDatabase: true}, nil
}
