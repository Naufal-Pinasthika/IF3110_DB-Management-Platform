package migration_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/api"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/apperror"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/auth"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/contracts"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/migration"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/models"
)

type mockAccess struct {
	err error
}

func (m *mockAccess) RequireDatabase(ctx context.Context, actor auth.Principal, databaseID string, level api.AccessLevel) error {
	return m.err
}

type mockDispatcher struct {
	enqueuedRef contracts.ExecutionRef
}

func (m *mockDispatcher) Enqueue(ctx context.Context, ref contracts.ExecutionRef) (contracts.JobReceipt, error) {
	m.enqueuedRef = ref
	return contracts.JobReceipt{
		JobID:       "job-uuid-" + ref.ExecutionID,
		ExecutionID: ref.ExecutionID,
		Status:      contracts.JobPending,
	}, nil
}

type mockRunner struct {
	executeErr error
}

func (r *mockRunner) Execute(ctx context.Context, databaseID, sqlText string) error {
	return r.executeErr
}

type mockRepo struct {
	migrations  map[string]models.Migration
	approvals   map[string][]models.MigrationApproval
	groups      []models.Group
	lastApplied *models.Migration
	nextForward *models.Migration
	executions  []models.MigrationExecution
}

func newMockRepo() *mockRepo {
	return &mockRepo{
		migrations: make(map[string]models.Migration),
		approvals:  make(map[string][]models.MigrationApproval),
	}
}

func (r *mockRepo) ListMigrations(ctx context.Context, databaseID string, status models.MigrationStatus) ([]models.Migration, error) {
	var res []models.Migration
	for _, m := range r.migrations {
		if m.DatabaseID == databaseID && (status == "" || m.Status == status) {
			res = append(res, m)
		}
	}
	return res, nil
}

func (r *mockRepo) GetMigration(ctx context.Context, databaseID, migrationID string) (models.Migration, error) {
	m, ok := r.migrations[migrationID]
	if !ok || m.DatabaseID != databaseID {
		return models.Migration{}, apperror.ErrNotFound
	}
	return m, nil
}

func (r *mockRepo) GetMigrationWithApprovals(ctx context.Context, databaseID, migrationID string) (models.Migration, []models.MigrationApproval, error) {
	m, err := r.GetMigration(ctx, databaseID, migrationID)
	if err != nil {
		return m, nil, err
	}
	return m, r.approvals[migrationID], nil
}

func (r *mockRepo) CreateMigration(ctx context.Context, m models.Migration) (models.Migration, error) {
	if m.ID == "" {
		m.ID = "mig-" + m.Name
	}
	r.migrations[m.ID] = m
	return m, nil
}

func (r *mockRepo) UpdateMigrationFields(ctx context.Context, databaseID, migrationID string, updates map[string]any, now time.Time) (models.Migration, error) {
	m, ok := r.migrations[migrationID]
	if !ok {
		return m, apperror.ErrNotFound
	}
	if val, ok := updates["sql_text"].(string); ok {
		m.SQLText = val
	}
	if val, ok := updates["sql_down"].(string); ok {
		m.SQLDown = &val
	}
	r.migrations[migrationID] = m
	return m, nil
}

func (r *mockRepo) UpdateMigration(ctx context.Context, databaseID, migrationID string, sqlText string, now time.Time) (models.Migration, error) {
	return r.UpdateMigrationFields(ctx, databaseID, migrationID, map[string]any{"sql_text": sqlText}, now)
}

func (r *mockRepo) LockDatabase(ctx context.Context, databaseID string) (func() error, error) {
	return func() error { return nil }, nil
}

func (r *mockRepo) MarkRunning(ctx context.Context, databaseID, migrationID, executorID string, now time.Time) (models.Migration, error) {
	m := r.migrations[migrationID]
	m.Status = models.MigrationRunning
	m.ExecutedBy = &executorID
	r.migrations[migrationID] = m
	return m, nil
}

func (r *mockRepo) MarkApplied(ctx context.Context, databaseID, migrationID, executorID string, now time.Time) (models.Migration, error) {
	m := r.migrations[migrationID]
	m.Status = models.MigrationApplied
	ver := int64(1)
	if m.Version != nil {
		ver = *m.Version + 1
	}
	m.Version = &ver
	m.ExecutedBy = &executorID
	m.ExecutedAt = &now
	r.migrations[migrationID] = m
	return m, nil
}

func (r *mockRepo) MarkFailed(ctx context.Context, databaseID, migrationID, executorID, errorSummary string, now time.Time) (models.Migration, error) {
	m := r.migrations[migrationID]
	m.Status = models.MigrationFailed
	m.ErrorSummary = &errorSummary
	r.migrations[migrationID] = m
	return m, nil
}

func (r *mockRepo) MarkRolledBack(ctx context.Context, databaseID, migrationID string, now time.Time) (models.Migration, error) {
	m := r.migrations[migrationID]
	m.Status = models.MigrationRolledBack
	m.Version = nil
	r.migrations[migrationID] = m
	return m, nil
}

func (r *mockRepo) RestoreApplied(ctx context.Context, databaseID, migrationID string, now time.Time) (models.Migration, error) {
	m := r.migrations[migrationID]
	m.Status = models.MigrationApplied
	r.migrations[migrationID] = m
	return m, nil
}

func (r *mockRepo) SubmitForReview(ctx context.Context, databaseID, migrationID string, approvals []models.MigrationApproval) error {
	m := r.migrations[migrationID]
	m.Status = models.MigrationPendingReview
	r.migrations[migrationID] = m
	r.approvals[migrationID] = approvals
	return nil
}

func (r *mockRepo) SubmitRollbackReview(ctx context.Context, databaseID, migrationID string, approvals []models.MigrationApproval) error {
	m := r.migrations[migrationID]
	m.Status = models.MigrationPendingReview
	r.migrations[migrationID] = m
	r.approvals[migrationID] = approvals
	return nil
}

func (r *mockRepo) SubmitForwardReview(ctx context.Context, databaseID, migrationID string, approvals []models.MigrationApproval) error {
	m := r.migrations[migrationID]
	m.Status = models.MigrationPendingReview
	r.migrations[migrationID] = m
	r.approvals[migrationID] = approvals
	return nil
}

func (r *mockRepo) GetPendingApprovalForLeader(ctx context.Context, migrationID, leaderID string) ([]models.MigrationApproval, error) {
	var res []models.MigrationApproval
	for _, a := range r.approvals[migrationID] {
		if a.LeaderID != nil && *a.LeaderID == leaderID && a.Status == models.ApprovalPending {
			res = append(res, a)
		}
	}
	return res, nil
}

func (r *mockRepo) UpdateApprovalStatus(ctx context.Context, approvalID string, status models.ApprovalStatus, comment *string, decidedAt time.Time) error {
	for migID, list := range r.approvals {
		for i := range list {
			if list[i].ID == approvalID {
				list[i].Status = status
				list[i].Comment = comment
				list[i].DecidedAt = &decidedAt
				r.approvals[migID] = list
				return nil
			}
		}
	}
	return nil
}

func (r *mockRepo) CheckAndSetApproved(ctx context.Context, databaseID, migrationID string) (bool, error) {
	for _, a := range r.approvals[migrationID] {
		if a.Status != models.ApprovalApproved {
			return false, nil
		}
	}
	m := r.migrations[migrationID]
	m.Status = models.MigrationApproved
	r.migrations[migrationID] = m
	return true, nil
}

func (r *mockRepo) CheckAndSetRollbackApproved(ctx context.Context, databaseID, migrationID string) (bool, error) {
	for _, a := range r.approvals[migrationID] {
		if a.Status != models.ApprovalApproved {
			return false, nil
		}
	}
	m := r.migrations[migrationID]
	m.Status = models.MigrationRollbackApproved
	r.migrations[migrationID] = m
	return true, nil
}

func (r *mockRepo) RejectMigration(ctx context.Context, databaseID, migrationID string) error {
	m := r.migrations[migrationID]
	m.Status = models.MigrationRejected
	r.migrations[migrationID] = m
	return nil
}

func (r *mockRepo) RejectRollbackReview(ctx context.Context, databaseID, migrationID string) error {
	m := r.migrations[migrationID]
	m.Status = models.MigrationApplied
	r.migrations[migrationID] = m
	return nil
}

func (r *mockRepo) RejectForwardReview(ctx context.Context, databaseID, migrationID string) error {
	m := r.migrations[migrationID]
	m.Status = models.MigrationRolledBack
	r.migrations[migrationID] = m
	return nil
}

func (r *mockRepo) GetDatabaseGroupsWithLeaders(ctx context.Context, databaseID string) ([]models.Group, error) {
	return r.groups, nil
}

func (r *mockRepo) GetLastAppliedMigration(ctx context.Context, databaseID string) (*models.Migration, error) {
	return r.lastApplied, nil
}

func (r *mockRepo) GetNextForwardCandidate(ctx context.Context, databaseID string) (*models.Migration, error) {
	return r.nextForward, nil
}

func (r *mockRepo) CreateExecution(ctx context.Context, exec *models.MigrationExecution) error {
	if exec.ID == "" {
		exec.ID = fmt.Sprintf("exec-%d", len(r.executions)+1)
	}
	r.executions = append(r.executions, *exec)
	return nil
}

func (r *mockRepo) GetExecution(ctx context.Context, executionID string) (*models.MigrationExecution, error) {
	for _, e := range r.executions {
		if e.ID == executionID {
			return &e, nil
		}
	}
	return nil, apperror.ErrNotFound
}

func (r *mockRepo) UpdateExecution(ctx context.Context, exec *models.MigrationExecution) error {
	for i := range r.executions {
		if r.executions[i].ID == exec.ID {
			r.executions[i] = *exec
			return nil
		}
	}
	return nil
}

func (r *mockRepo) ListExecutions(ctx context.Context, databaseID string, hideRollback bool) ([]models.MigrationExecution, error) {
	if !hideRollback {
		return r.executions, nil
	}
	cancelled := make(map[string]bool)
	rollbacks := make(map[string]bool)
	for _, e := range r.executions {
		if e.ActionType == "ROLLBACK" && e.Status == "SUCCEEDED" && e.RolledBackExecutionID != nil {
			cancelled[*e.RolledBackExecutionID] = true
			rollbacks[e.ID] = true
		}
	}
	var res []models.MigrationExecution
	for _, e := range r.executions {
		if !cancelled[e.ID] && !rollbacks[e.ID] {
			res = append(res, e)
		}
	}
	return res, nil
}

func makePrincipal(id string) auth.Principal {
	return auth.Principal{
		User: api.LoginResponse{ID: id, Username: "user-" + id},
	}
}

func TestMigrationWorkflowApprovals(t *testing.T) {
	ctx := context.Background()
	repo := newMockRepo()
	dispatcher := &mockDispatcher{}
	svc := &migration.Service{
		Repository: repo,
		Access:     &mockAccess{},
		Dispatcher: dispatcher,
	}

	dbID := "db-1"
	leader1 := "leader-1"
	leader2 := "leader-2"
	repo.groups = []models.Group{
		{ID: "g-1", Name: "Engineering", LeaderID: &leader1},
		{ID: "g-2", Name: "Data", LeaderID: &leader2},
	}

	downSQL := "DROP TABLE users;"
	m, err := repo.CreateMigration(ctx, models.Migration{
		DatabaseID: dbID,
		Name:       "create_users",
		SQLText:    "CREATE TABLE users (id int);",
		SQLDown:    &downSQL,
		CreatedBy:  leader1,
		Status:     models.MigrationDraft,
	})
	if err != nil {
		t.Fatal(err)
	}

	t.Run("Submit review stays PENDING_REVIEW when a group has no leader yet", func(t *testing.T) {
		noLeadRepo := newMockRepo()
		noLeadSvc := &migration.Service{Repository: noLeadRepo, Access: &mockAccess{}, Dispatcher: dispatcher}
		noLeadRepo.groups = []models.Group{
			{ID: "g-1", Name: "Engineering", LeaderID: &leader1},
			{ID: "g-2", Name: "Data", LeaderID: nil},
		}
		mn, _ := noLeadRepo.CreateMigration(ctx, models.Migration{
			DatabaseID: dbID, Name: "pending_no_leader", SQLText: "SELECT 1;", CreatedBy: leader1, Status: models.MigrationDraft,
		})
		detail, err := noLeadSvc.SubmitReview(ctx, makePrincipal(leader1), dbID, mn.ID, time.Now())
		if err != nil {
			t.Fatalf("expected submission to stay PENDING_REVIEW, got error: %v", err)
		}
		if detail.Status != models.MigrationPendingReview {
			t.Fatalf("expected PENDING_REVIEW, got %s", detail.Status)
		}
		if len(noLeadRepo.approvals[mn.ID]) != 2 || noLeadRepo.approvals[mn.ID][1].LeaderID != nil {
			t.Fatalf("expected nil LeaderID pending row for unassigned group, got %+v", noLeadRepo.approvals[mn.ID])
		}
		noLeadRepo.approvals[mn.ID][0].ID = "app-nl-1"
		noLeadRepo.approvals[mn.ID][1].ID = "app-nl-2"
		afterFirst, err := noLeadSvc.ApproveMigration(ctx, makePrincipal(leader1), dbID, mn.ID, api.DecideApprovalRequest{}, time.Now())
		if err != nil {
			t.Fatalf("leader1 approve failed: %v", err)
		}
		if afterFirst.Status != models.MigrationPendingReview {
			t.Fatalf("expected PENDING_REVIEW while unassigned group still pending, got %s", afterFirst.Status)
		}
	})

	t.Run("Submit review creates pending approvals for all group leaders", func(t *testing.T) {
		detail, err := svc.SubmitReview(ctx, makePrincipal(leader1), dbID, m.ID, time.Now())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if detail.Status != models.MigrationPendingReview {
			t.Fatalf("expected PENDING_REVIEW, got %s", detail.Status)
		}
		if len(repo.approvals[m.ID]) != 2 {
			t.Fatalf("expected 2 approvals, got %d", len(repo.approvals[m.ID]))
		}
	})

	t.Run("Non-leader cannot approve", func(t *testing.T) {
		_, err := svc.ApproveMigration(ctx, makePrincipal("random-user"), dbID, m.ID, api.DecideApprovalRequest{}, time.Now())
		if !errors.Is(err, apperror.ErrForbidden) {
			t.Fatalf("expected forbidden, got %v", err)
		}
	})

	t.Run("First leader approves -> status remains PENDING_REVIEW", func(t *testing.T) {
		repo.approvals[m.ID][0].ID = "app-1"
		repo.approvals[m.ID][1].ID = "app-2"
		detail, err := svc.ApproveMigration(ctx, makePrincipal(leader1), dbID, m.ID, api.DecideApprovalRequest{}, time.Now())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if detail.Status != models.MigrationPendingReview {
			t.Fatalf("expected PENDING_REVIEW until all leaders approve, got %s", detail.Status)
		}
	})

	t.Run("Second leader approves -> status becomes APPROVED", func(t *testing.T) {
		detail, err := svc.ApproveMigration(ctx, makePrincipal(leader2), dbID, m.ID, api.DecideApprovalRequest{}, time.Now())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if detail.Status != models.MigrationApproved {
			t.Fatalf("expected APPROVED after all leaders approve, got %s", detail.Status)
		}
	})

	t.Run("Execute approved migration enqueues async job", func(t *testing.T) {
		res, err := svc.ExecuteMigration(ctx, makePrincipal(leader1), dbID, m.ID, time.Now())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.JobID == "" {
			t.Fatal("expected async job_id in execution response")
		}
		if dispatcher.enqueuedRef.ExecutionID == "" {
			t.Fatal("expected execution dispatched to queue")
		}
	})

	t.Run("Leader rejection transitions status to REJECTED", func(t *testing.T) {
		m2, _ := repo.CreateMigration(ctx, models.Migration{
			DatabaseID: dbID,
			Name:       "add_orders",
			SQLText:    "CREATE TABLE orders (id int);",
			CreatedBy:  leader1,
			Status:     models.MigrationDraft,
		})
		_, _ = svc.SubmitReview(ctx, makePrincipal(leader1), dbID, m2.ID, time.Now())
		repo.approvals[m2.ID][0].ID = "app-21"
		repo.approvals[m2.ID][1].ID = "app-22"

		comment := "Schema needs index"
		detail, err := svc.RejectMigration(ctx, makePrincipal(leader2), dbID, m2.ID, api.DecideApprovalRequest{Comment: &comment}, time.Now())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if detail.Status != models.MigrationRejected {
			t.Fatalf("expected REJECTED, got %s", detail.Status)
		}

		// Update rejected draft and resubmit
		newSQL := "CREATE TABLE orders (id int PRIMARY KEY);"
		_, err = svc.UpdateMigration(ctx, makePrincipal(leader1), dbID, m2.ID, api.UpdateMigrationRequest{SQLText: &newSQL}, time.Now())
		if err != nil {
			t.Fatalf("failed to update rejected migration: %v", err)
		}

		resubmitted, err := svc.SubmitReview(ctx, makePrincipal(leader1), dbID, m2.ID, time.Now())
		if err != nil || resubmitted.Status != models.MigrationPendingReview {
			t.Fatalf("expected PENDING_REVIEW on resubmit, got %v (err: %v)", resubmitted.Status, err)
		}
	})

	t.Run("Already-approved leader cannot approve again", func(t *testing.T) {
		m3, _ := repo.CreateMigration(ctx, models.Migration{
			DatabaseID: dbID,
			Name:       "add_audit_col",
			SQLText:    "ALTER TABLE users ADD COLUMN updated_at TIMESTAMPTZ;",
			CreatedBy:  leader1,
			Status:     models.MigrationDraft,
		})
		_, _ = svc.SubmitReview(ctx, makePrincipal(leader1), dbID, m3.ID, time.Now())
		repo.approvals[m3.ID][0].ID = "app-31"
		repo.approvals[m3.ID][1].ID = "app-32"

		if _, err := svc.ApproveMigration(ctx, makePrincipal(leader1), dbID, m3.ID, api.DecideApprovalRequest{}, time.Now()); err != nil {
			t.Fatalf("first approval failed: %v", err)
		}
		if _, err := svc.ApproveMigration(ctx, makePrincipal(leader1), dbID, m3.ID, api.DecideApprovalRequest{}, time.Now()); !errors.Is(err, apperror.ErrForbidden) {
			t.Fatalf("duplicate approval error = %v, want ErrForbidden", err)
		}
	})

	t.Run("Single user leading multiple groups approves all their groups at once", func(t *testing.T) {
		sharedRepo := newMockRepo()
		sharedSvc := &migration.Service{Repository: sharedRepo, Access: &mockAccess{}, Dispatcher: dispatcher}
		sharedRepo.groups = []models.Group{
			{ID: "g-a", Name: "Team A", LeaderID: &leader1},
			{ID: "g-b", Name: "Team B", LeaderID: &leader1},
		}
		ms, _ := sharedRepo.CreateMigration(ctx, models.Migration{
			DatabaseID: dbID, Name: "shared_leader_mig", SQLText: "SELECT 1;", CreatedBy: leader1, Status: models.MigrationDraft,
		})
		_, err := sharedSvc.SubmitReview(ctx, makePrincipal(leader1), dbID, ms.ID, time.Now())
		if err != nil {
			t.Fatalf("submit failed: %v", err)
		}
		sharedRepo.approvals[ms.ID][0].ID = "app-s1"
		sharedRepo.approvals[ms.ID][1].ID = "app-s2"

		detail, err := sharedSvc.ApproveMigration(ctx, makePrincipal(leader1), dbID, ms.ID, api.DecideApprovalRequest{}, time.Now())
		if err != nil {
			t.Fatalf("approve failed: %v", err)
		}
		if detail.Status != models.MigrationApproved {
			t.Fatalf("status = %s, want APPROVED", detail.Status)
		}
	})

	t.Run("ExecuteMigration revalidates newly-added group access after approval", func(t *testing.T) {
		revalRepo := newMockRepo()
		revalSvc := &migration.Service{Repository: revalRepo, Access: &mockAccess{}, Dispatcher: dispatcher}
		revalRepo.groups = []models.Group{
			{ID: "g-1", Name: "Engineering", LeaderID: &leader1},
		}
		mr, _ := revalRepo.CreateMigration(ctx, models.Migration{
			DatabaseID: dbID, Name: "reval_mig", SQLText: "SELECT 1;", CreatedBy: leader1, Status: models.MigrationDraft,
		})
		_, _ = revalSvc.SubmitReview(ctx, makePrincipal(leader1), dbID, mr.ID, time.Now())
		revalRepo.approvals[mr.ID][0].ID = "app-r1"
		_, err := revalSvc.ApproveMigration(ctx, makePrincipal(leader1), dbID, mr.ID, api.DecideApprovalRequest{}, time.Now())
		if err != nil {
			t.Fatalf("approve failed: %v", err)
		}

		// A second group is granted access to the database after approval
		revalRepo.groups = append(revalRepo.groups, models.Group{ID: "g-2", Name: "Analytics", LeaderID: &leader2})
		_, err = revalSvc.ExecuteMigration(ctx, makePrincipal(leader1), dbID, mr.ID, time.Now())
		if !errors.Is(err, apperror.ErrConflict) {
			t.Fatalf("execute with unapproved new group: err = %v, want ErrConflict", err)
		}
	})
}

func TestRollbackAndHideRollback(t *testing.T) {
	ctx := context.Background()
	repo := newMockRepo()
	dispatcher := &mockDispatcher{}
	svc := &migration.Service{
		Repository: repo,
		Access:     &mockAccess{},
		Dispatcher: dispatcher,
	}

	dbID := "db-1"
	leader1 := "leader-1"
	leader2 := "leader-2"
	repo.groups = []models.Group{
		{ID: "g-1", Name: "Engineering", LeaderID: &leader1},
		{ID: "g-2", Name: "Data", LeaderID: &leader2},
	}

	downSQL := "DROP TABLE t1;"
	ver := int64(1)
	m := models.Migration{
		ID:         "mig-1",
		DatabaseID: dbID,
		Name:       "m1",
		SQLText:    "CREATE TABLE t1 (id int);",
		SQLDown:    &downSQL,
		Status:     models.MigrationApplied,
		Version:    &ver,
	}
	repo.migrations[m.ID] = m
	repo.lastApplied = &m

	t.Run("Cannot rollback irreversible migration", func(t *testing.T) {
		m.IsIrreversible = true
		reason := "data truncated"
		m.IrreversibleReason = &reason
		repo.migrations[m.ID] = m
		repo.lastApplied = &m

		if _, err := svc.SubmitRollbackReview(ctx, makePrincipal(leader1), dbID, m.ID, time.Now()); err == nil {
			t.Fatal("expected rollback review to be rejected for irreversible migration")
		}
		if _, err := svc.RollbackMigration(ctx, makePrincipal(leader1), dbID, m.ID, time.Now()); err == nil {
			t.Fatal("expected rollback to be rejected for irreversible migration")
		}
		m.IsIrreversible = false
		m.IrreversibleReason = nil
		repo.migrations[m.ID] = m
		repo.lastApplied = &m
	})

	t.Run("Direct rollback without leader approval is rejected", func(t *testing.T) {
		_, err := svc.RollbackMigration(ctx, makePrincipal(leader1), dbID, m.ID, time.Now())
		if !errors.Is(err, apperror.ErrConflict) {
			t.Fatalf("expected ErrConflict when rollback is not approved, got %v", err)
		}
	})

	t.Run("Rollback review rejection restores APPLIED status", func(t *testing.T) {
		detail, err := svc.SubmitRollbackReview(ctx, makePrincipal(leader1), dbID, m.ID, time.Now())
		if err != nil {
			t.Fatalf("submit rollback review failed: %v", err)
		}
		if detail.Status != models.MigrationPendingReview {
			t.Fatalf("expected PENDING_REVIEW, got %s", detail.Status)
		}
		repo.approvals[m.ID][0].ID = "rb-app-1"
		repo.approvals[m.ID][1].ID = "rb-app-2"

		comment := "Not safe during peak hours"
		rejected, err := svc.RejectMigration(ctx, makePrincipal(leader2), dbID, m.ID, api.DecideApprovalRequest{Comment: &comment}, time.Now())
		if err != nil {
			t.Fatalf("reject rollback failed: %v", err)
		}
		if rejected.Status != models.MigrationApplied {
			t.Fatalf("expected APPLIED after rollback rejection, got %s", rejected.Status)
		}
	})

	t.Run("Rollback requires all leader approvals then creates execution referencing cancelled forward", func(t *testing.T) {
		repo.executions = []models.MigrationExecution{
			{ID: "E1", MigrationID: m.ID, DatabaseID: dbID, ActionType: "FORWARD", Status: "SUCCEEDED"},
		}
		_, err := svc.SubmitRollbackReview(ctx, makePrincipal(leader1), dbID, m.ID, time.Now())
		if err != nil {
			t.Fatalf("submit rollback review failed: %v", err)
		}
		repo.approvals[m.ID][0].ID = "rb-app-3"
		repo.approvals[m.ID][1].ID = "rb-app-4"

		detail1, err := svc.ApproveMigration(ctx, makePrincipal(leader1), dbID, m.ID, api.DecideApprovalRequest{}, time.Now())
		if err != nil || detail1.Status != models.MigrationPendingReview {
			t.Fatalf("expected PENDING_REVIEW after first approval, got %v (err: %v)", detail1.Status, err)
		}
		detail2, err := svc.ApproveMigration(ctx, makePrincipal(leader2), dbID, m.ID, api.DecideApprovalRequest{}, time.Now())
		if err != nil || detail2.Status != models.MigrationRollbackApproved {
			t.Fatalf("expected ROLLBACK_APPROVED after all approvals, got %v (err: %v)", detail2.Status, err)
		}
		if !detail2.CanRollback {
			t.Fatal("expected CanRollback to be true after ROLLBACK_APPROVED")
		}

		res, err := svc.RollbackMigration(ctx, makePrincipal(leader1), dbID, m.ID, time.Now())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.JobID == "" {
			t.Fatal("expected job ID")
		}

		lastExec := repo.executions[len(repo.executions)-1]
		if lastExec.ActionType != "ROLLBACK" {
			t.Fatalf("expected ROLLBACK action, got %s", lastExec.ActionType)
		}
		if lastExec.RolledBackExecutionID == nil || *lastExec.RolledBackExecutionID != "E1" {
			t.Fatalf("expected rolled back execution E1, got %v", lastExec.RolledBackExecutionID)
		}
	})

	t.Run("Forward review on ROLLED_BACK requires leader approval", func(t *testing.T) {
		_, _ = repo.MarkRolledBack(ctx, dbID, m.ID, time.Now())
		cur := repo.migrations[m.ID]
		repo.lastApplied = nil
		repo.nextForward = &cur

		if _, err := svc.ForwardMigration(ctx, makePrincipal(leader1), dbID, m.ID, time.Now()); !errors.Is(err, apperror.ErrConflict) {
			t.Fatalf("expected ErrConflict forwarding unapproved ROLLED_BACK migration, got %v", err)
		}

		detail, err := svc.SubmitForwardReview(ctx, makePrincipal(leader1), dbID, m.ID, time.Now())
		if err != nil || detail.Status != models.MigrationPendingReview {
			t.Fatalf("expected PENDING_REVIEW for forward, got %v (err: %v)", detail.Status, err)
		}
		repo.approvals[m.ID][0].ID = "fw-app-1"
		repo.approvals[m.ID][1].ID = "fw-app-2"

		_, err = svc.ApproveMigration(ctx, makePrincipal(leader1), dbID, m.ID, api.DecideApprovalRequest{}, time.Now())
		if err != nil {
			t.Fatalf("leader1 forward approve failed: %v", err)
		}
		approvedDetail, err := svc.ApproveMigration(ctx, makePrincipal(leader2), dbID, m.ID, api.DecideApprovalRequest{}, time.Now())
		if err != nil || approvedDetail.Status != models.MigrationApproved {
			t.Fatalf("expected APPROVED after forward approvals, got %v (err: %v)", approvedDetail.Status, err)
		}
		cur = repo.migrations[m.ID]
		repo.nextForward = &cur

		fwdRes, err := svc.ForwardMigration(ctx, makePrincipal(leader1), dbID, m.ID, time.Now())
		if err != nil {
			t.Fatalf("forward after approval failed: %v", err)
		}
		if fwdRes.JobID == "" {
			t.Fatal("expected job ID on forward execution")
		}
	})

	t.Run("Hide rollback filters out cancelled pair", func(t *testing.T) {
		e1Cancelled := "E2"
		repo.executions = []models.MigrationExecution{
			{ID: "E1", MigrationID: "m1", DatabaseID: dbID, ActionType: "FORWARD", Status: "SUCCEEDED"},
			{ID: "E2", MigrationID: "m2", DatabaseID: dbID, ActionType: "FORWARD", Status: "SUCCEEDED"},
			{ID: "E3", MigrationID: "m2", DatabaseID: dbID, ActionType: "ROLLBACK", Status: "SUCCEEDED", RolledBackExecutionID: &e1Cancelled},
			{ID: "E4", MigrationID: "m2", DatabaseID: dbID, ActionType: "FORWARD", Status: "SUCCEEDED"},
			{ID: "E5", MigrationID: "m3", DatabaseID: dbID, ActionType: "FORWARD", Status: "SUCCEEDED"},
		}

		all, err := svc.ListMigrationHistory(ctx, makePrincipal("user-1"), dbID, false)
		if err != nil || len(all) != 5 {
			t.Fatalf("expected 5 history items without filter, got %d (err: %v)", len(all), err)
		}

		hidden, err := svc.ListMigrationHistory(ctx, makePrincipal("user-1"), dbID, true)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(hidden) != 3 {
			t.Fatalf("expected 3 history items with hide_rollback (E1, E4, E5), got %d", len(hidden))
		}
		for _, item := range hidden {
			if item.ID == "E2" || item.ID == "E3" {
				t.Fatalf("found cancelled/rollback item %s in filtered history", item.ID)
			}
		}
	})

	t.Run("Failed rollback attempts remain visible with hide_rollback", func(t *testing.T) {
		e1Cancelled := "E2"
		repo.executions = []models.MigrationExecution{
			{ID: "E1", MigrationID: "m1", DatabaseID: dbID, ActionType: "FORWARD", Status: "SUCCEEDED"},
			{ID: "E2", MigrationID: "m2", DatabaseID: dbID, ActionType: "FORWARD", Status: "SUCCEEDED"},
			{ID: "E3-FAIL", MigrationID: "m2", DatabaseID: dbID, ActionType: "ROLLBACK", Status: "FAILED", RolledBackExecutionID: &e1Cancelled},
		}

		hidden, err := svc.ListMigrationHistory(ctx, makePrincipal("user-1"), dbID, true)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(hidden) != 3 {
			t.Fatalf("expected failed rollback to remain visible, got %d items", len(hidden))
		}
	})
}

func TestBatchResumeAndCancel(t *testing.T) {
	ctx := context.Background()
	repo := newMockRepo()
	dispatcher := &mockDispatcher{}
	svc := &migration.Service{
		Repository: repo,
		Access:     &mockAccess{},
		Dispatcher: dispatcher,
	}

	dbID := "db-1"
	leader1 := "leader-1"
	repo.groups = []models.Group{
		{ID: "g-1", Name: "Engineering", LeaderID: &leader1},
	}

	downSQL := "UPDATE users SET flag = 0 WHERE id <= {{CURSOR}} LIMIT {{BATCH_SIZE}};"
	batchAttr := "id"
	batchSize := 500
	mb := models.Migration{
		ID:             "mig-batch-1",
		DatabaseID:     dbID,
		Name:           "batch_backfill",
		SQLText:        "UPDATE users SET flag = 1 WHERE id > {{CURSOR}} LIMIT {{BATCH_SIZE}};",
		SQLDown:        &downSQL,
		MigrationType:  "BATCH",
		BatchAttribute: &batchAttr,
		BatchSize:      &batchSize,
		Status:         models.MigrationRunning,
		CreatedBy:      leader1,
	}
	repo.migrations[mb.ID] = mb
	repo.approvals[mb.ID] = []models.MigrationApproval{
		{ID: "app-b1", MigrationID: mb.ID, GroupID: "g-1", LeaderID: &leader1, Status: models.ApprovalApproved, Action: "MIGRATE"},
	}

	cursor := "1500"
	repo.executions = []models.MigrationExecution{
		{
			ID:               "exec-partial-1",
			MigrationID:      mb.ID,
			DatabaseID:       dbID,
			ActionType:       "FORWARD",
			Status:           "PARTIAL",
			ExecutedSQL:      mb.SQLText,
			ExecutedBy:       leader1,
			CommittedBatches: 3,
			CommittedRows:    1500,
			LastCursorValue:  &cursor,
		},
	}

	t.Run("ResumeExecution queues RETRY from checkpoint", func(t *testing.T) {
		res, err := svc.ResumeExecution(ctx, makePrincipal(leader1), dbID, "exec-partial-1", time.Now())
		if err != nil {
			t.Fatalf("resume execution failed: %v", err)
		}
		if res.JobID == "" {
			t.Fatal("expected job ID")
		}
		latest := repo.executions[len(repo.executions)-1]
		if latest.ActionType != "RETRY" {
			t.Fatalf("expected RETRY action, got %s", latest.ActionType)
		}
		if latest.RolledBackExecutionID == nil || *latest.RolledBackExecutionID != "exec-partial-1" {
			t.Fatalf("expected parent exec-partial-1, got %v", latest.RolledBackExecutionID)
		}
		if latest.LastCursorValue == nil || *latest.LastCursorValue != "1500" {
			t.Fatalf("expected cursor 1500 preserved, got %v", latest.LastCursorValue)
		}
	})

	t.Run("CancelBatch queues REVERT_BATCH compensation", func(t *testing.T) {
		res, err := svc.CancelBatch(ctx, makePrincipal(leader1), dbID, "exec-partial-1", time.Now())
		if err != nil {
			t.Fatalf("cancel batch failed: %v", err)
		}
		if res.JobID == "" {
			t.Fatal("expected job ID")
		}
		latest := repo.executions[len(repo.executions)-1]
		if latest.ActionType != "REVERT_BATCH" {
			t.Fatalf("expected REVERT_BATCH action, got %s", latest.ActionType)
		}
		if latest.ExecutedSQL != downSQL {
			t.Fatalf("expected SQLDown on compensation execution, got %s", latest.ExecutedSQL)
		}
	})

	t.Run("CancelBatch rejects irreversible batch migration", func(t *testing.T) {
		mCopy := repo.migrations[mb.ID]
		mCopy.IsIrreversible = true
		repo.migrations[mb.ID] = mCopy
		defer func() {
			mCopy.IsIrreversible = false
			repo.migrations[mb.ID] = mCopy
		}()

		if _, err := svc.CancelBatch(ctx, makePrincipal(leader1), dbID, "exec-partial-1", time.Now()); !errors.Is(err, apperror.ErrConflict) {
			t.Fatalf("expected ErrConflict for irreversible cancel-batch, got %v", err)
		}
	})

	t.Run("ResumeExecution on PARTIALLY_REVERTED continues REVERT_BATCH", func(t *testing.T) {
		repo.executions = append(repo.executions, models.MigrationExecution{
			ID:                    "exec-part-rev-1",
			MigrationID:           mb.ID,
			DatabaseID:            dbID,
			ActionType:            "REVERT_BATCH",
			RolledBackExecutionID: strPtr("exec-partial-1"),
			Status:                "PARTIALLY_REVERTED",
			ExecutedSQL:           downSQL,
			ExecutedBy:            leader1,
			CommittedBatches:      1,
			CommittedRows:         500,
		})
		_, err := svc.ResumeExecution(ctx, makePrincipal(leader1), dbID, "exec-part-rev-1", time.Now())
		if err != nil {
			t.Fatalf("resume partially reverted failed: %v", err)
		}
		latest := repo.executions[len(repo.executions)-1]
		if latest.ActionType != "REVERT_BATCH" {
			t.Fatalf("expected REVERT_BATCH action on resume, got %s", latest.ActionType)
		}
	})
}

func strPtr(s string) *string { return &s }

func TestProgressAndRegistry(t *testing.T) {
	ctx := context.Background()
	repo := newMockRepo()
	runner := &mockRunner{}
	svc := &migration.Service{
		Repository: repo,
		Access:     &mockAccess{},
		Runner:     runner,
	}

	dbID := "db-1"
	execID := "exec-test-1"
	rolledBack := "exec-old-1"

	repo.executions = []models.MigrationExecution{
		{
			ID:                    execID,
			DatabaseID:            dbID,
			MigrationID:           "mig-1",
			ActionType:            "ROLLBACK",
			RolledBackExecutionID: &rolledBack,
			ExecutedBy:            "user-1",
			CommittedBatches:      5,
			CommittedRows:         5000,
		},
	}

	t.Run("ValidateExecution populates ReplaceBlockedExecutionID for rollback handoff", func(t *testing.T) {
		meta, err := svc.ValidateExecution(ctx, contracts.ExecutionRef{ExecutionID: execID, DatabaseID: dbID})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if meta.ActorID != "user-1" {
			t.Fatalf("expected actor user-1, got %s", meta.ActorID)
		}
		if meta.ReplaceBlockedExecutionID != rolledBack {
			t.Fatalf("expected replace blocked execution %s, got %s", rolledBack, meta.ReplaceBlockedExecutionID)
		}
	})

	t.Run("ReadProgress returns committed batches and rows", func(t *testing.T) {
		progress, err := svc.ReadProgress(ctx, contracts.ExecutionRef{ExecutionID: execID, DatabaseID: dbID})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if progress.CommittedBatches != 5 || progress.CommittedRows != 5000 {
			t.Fatalf("expected 5 batches and 5000 rows, got %+v", progress)
		}
	})
}
