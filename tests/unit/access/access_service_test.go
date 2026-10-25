package access

import (
	"context"
	"errors"
	"testing"

	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/api"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/auth"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/models"
)

func TestRequirePlatformAdmin(t *testing.T) {
	mockRepo := &mockRepository{}
	svc := &Service{Repository: mockRepo}

	ctx := context.Background()

	t.Run("success when user is platform admin", func(t *testing.T) {
		principal := makePrincipal("user-1", true)
		err := svc.RequirePlatformAdmin(ctx, principal)
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})

	t.Run("forbidden when user is not platform admin", func(t *testing.T) {
		principal := makePrincipal("user-1", false)
		err := svc.RequirePlatformAdmin(ctx, principal)
		if err == nil {
			t.Error("expected forbidden error, got nil")
		}
	})

	t.Run("unauthenticated when principal is empty", func(t *testing.T) {
		principal := auth.Principal{}
		err := svc.RequirePlatformAdmin(ctx, principal)
		if err == nil {
			t.Error("expected unauthenticated error, got nil")
		}
	})
}

func TestEffectiveAccess(t *testing.T) {
	ctx := context.Background()

	t.Run("VIEW_EDIT when any grant is VIEW_EDIT", func(t *testing.T) {
		mockRepo := &mockRepository{
			grants: []models.GroupAccess{
				makeGrant("group-1", "db-1", models.AccessViewOnly),
				makeGrant("group-2", "db-1", models.AccessViewEdit),
			},
		}
		svc := &Service{Repository: mockRepo}
		principal := makePrincipal("user-1", false)

		level, err := svc.EffectiveAccess(ctx, principal, "db-1")
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if level != api.AccessViewEdit {
			t.Errorf("expected VIEW_EDIT, got %s", level)
		}
	})

	t.Run("VIEW_ONLY when all grants are VIEW_ONLY", func(t *testing.T) {
		mockRepo := &mockRepository{
			grants: []models.GroupAccess{
				makeGrant("group-1", "db-1", models.AccessViewOnly),
				makeGrant("group-2", "db-1", models.AccessViewOnly),
			},
		}
		svc := &Service{Repository: mockRepo}
		principal := makePrincipal("user-1", false)

		level, err := svc.EffectiveAccess(ctx, principal, "db-1")
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if level != api.AccessViewOnly {
			t.Errorf("expected VIEW_ONLY, got %s", level)
		}
	})

	t.Run("forbidden when no grants", func(t *testing.T) {
		mockRepo := &mockRepository{
			grants: []models.GroupAccess{},
		}
		svc := &Service{Repository: mockRepo}
		principal := makePrincipal("user-1", false)

		level, err := svc.EffectiveAccess(ctx, principal, "db-1")
		if err == nil {
			t.Error("expected forbidden error, got nil")
		}
		if level != "" {
			t.Errorf("expected empty level, got %s", level)
		}
	})

	t.Run("forbidden when user is platform admin but no grant", func(t *testing.T) {
		mockRepo := &mockRepository{
			grants: []models.GroupAccess{},
		}
		svc := &Service{Repository: mockRepo}
		principal := makePrincipal("user-1", true) // platform admin but no grant

		level, err := svc.EffectiveAccess(ctx, principal, "db-1")
		if err == nil {
			t.Error("expected forbidden error for platform admin without grant, got nil")
		}
		if level != "" {
			t.Errorf("expected empty level, got %s", level)
		}
	})

	t.Run("unauthenticated when principal empty", func(t *testing.T) {
		mockRepo := &mockRepository{}
		svc := &Service{Repository: mockRepo}
		principal := auth.Principal{}

		level, err := svc.EffectiveAccess(ctx, principal, "db-1")
		if err == nil {
			t.Error("expected unauthenticated error, got nil")
		}
		if level != "" {
			t.Errorf("expected empty level, got %s", level)
		}
	})

	t.Run("repository error propagated", func(t *testing.T) {
		mockRepo := &mockRepository{
			grantsErr: errors.New("db error"),
		}
		svc := &Service{Repository: mockRepo}
		principal := makePrincipal("user-1", false)

		level, err := svc.EffectiveAccess(ctx, principal, "db-1")
		if err == nil {
			t.Error("expected error, got nil")
		}
		if level != "" {
			t.Errorf("expected empty level, got %s", level)
		}
	})
}

func TestRequireDatabase(t *testing.T) {
	ctx := context.Background()

	t.Run("success when effective equals required VIEW_ONLY", func(t *testing.T) {
		mockRepo := &mockRepository{
			grants: []models.GroupAccess{
				makeGrant("group-1", "db-1", models.AccessViewOnly),
			},
		}
		svc := &Service{Repository: mockRepo}
		principal := makePrincipal("user-1", false)

		err := svc.RequireDatabase(ctx, principal, "db-1", api.AccessViewOnly)
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})

	t.Run("success when effective VIEW_EDIT and required VIEW_ONLY", func(t *testing.T) {
		mockRepo := &mockRepository{
			grants: []models.GroupAccess{
				makeGrant("group-1", "db-1", models.AccessViewEdit),
			},
		}
		svc := &Service{Repository: mockRepo}
		principal := makePrincipal("user-1", false)

		err := svc.RequireDatabase(ctx, principal, "db-1", api.AccessViewOnly)
		if err != nil {
			t.Errorf("expected no error (VIEW_EDIT includes VIEW_ONLY), got %v", err)
		}
	})

	t.Run("success when effective VIEW_EDIT and required VIEW_EDIT", func(t *testing.T) {
		mockRepo := &mockRepository{
			grants: []models.GroupAccess{
				makeGrant("group-1", "db-1", models.AccessViewEdit),
			},
		}
		svc := &Service{Repository: mockRepo}
		principal := makePrincipal("user-1", false)

		err := svc.RequireDatabase(ctx, principal, "db-1", api.AccessViewEdit)
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})

	t.Run("forbidden when effective VIEW_ONLY but required VIEW_EDIT", func(t *testing.T) {
		mockRepo := &mockRepository{
			grants: []models.GroupAccess{
				makeGrant("group-1", "db-1", models.AccessViewOnly),
			},
		}
		svc := &Service{Repository: mockRepo}
		principal := makePrincipal("user-1", false)

		err := svc.RequireDatabase(ctx, principal, "db-1", api.AccessViewEdit)
		if err == nil {
			t.Error("expected forbidden error, got nil")
		}
	})

	t.Run("forbidden when no grants", func(t *testing.T) {
		mockRepo := &mockRepository{
			grants: []models.GroupAccess{},
		}
		svc := &Service{Repository: mockRepo}
		principal := makePrincipal("user-1", false)

		err := svc.RequireDatabase(ctx, principal, "db-1", api.AccessViewOnly)
		if err == nil {
			t.Error("expected forbidden error, got nil")
		}
	})

	t.Run("forbidden when required level is invalid", func(t *testing.T) {
		mockRepo := &mockRepository{
			grants: []models.GroupAccess{
				makeGrant("group-1", "db-1", models.AccessViewEdit),
			},
		}
		svc := &Service{Repository: mockRepo}
		principal := makePrincipal("user-1", false)

		err := svc.RequireDatabase(ctx, principal, "db-1", "INVALID_LEVEL")
		if err == nil {
			t.Error("expected forbidden error for invalid level, got nil")
		}
	})
}

func TestListDatabases(t *testing.T) {
	ctx := context.Background()

	t.Run("returns databases with effective access", func(t *testing.T) {
		mockRepo := &mockRepository{
			databases: []models.ManagedDatabase{
				makeDatabase("db-1", "Database 1", "db1"),
				makeDatabase("db-2", "Database 2", "db2"),
			},
			grants: []models.GroupAccess{
				makeGrant("group-1", "db-1", models.AccessViewEdit),
				makeGrant("group-1", "db-2", models.AccessViewOnly),
			},
		}
		svc := &Service{Repository: mockRepo}
		principal := makePrincipal("user-1", false)

		dbs, err := svc.ListDatabases(ctx, principal)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if len(dbs) != 2 {
			t.Errorf("expected 2 databases, got %d", len(dbs))
		}
		if dbs[0].EffectiveAccess != api.AccessViewEdit {
			t.Errorf("expected db-1 VIEW_EDIT, got %s", dbs[0].EffectiveAccess)
		}
		if dbs[1].EffectiveAccess != api.AccessViewOnly {
			t.Errorf("expected db-2 VIEW_ONLY, got %s", dbs[1].EffectiveAccess)
		}
	})

	t.Run("skips databases where user has no grant", func(t *testing.T) {
		mockRepo := &mockRepository{
			databases: []models.ManagedDatabase{
				makeDatabase("db-1", "Database 1", "db1"),
				makeDatabase("db-2", "Database 2", "db2"),
			},
			grants: []models.GroupAccess{
				makeGrant("group-1", "db-1", models.AccessViewOnly),
				// no grant for db-2
			},
		}
		svc := &Service{Repository: mockRepo}
		principal := makePrincipal("user-1", false)

		dbs, err := svc.ListDatabases(ctx, principal)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if len(dbs) != 1 {
			t.Errorf("expected 1 database (db-2 skipped), got %d", len(dbs))
		}
		if dbs[0].ID != "db-1" {
			t.Errorf("expected db-1, got %s", dbs[0].ID)
		}
	})

	t.Run("unauthenticated when principal empty", func(t *testing.T) {
		mockRepo := &mockRepository{}
		svc := &Service{Repository: mockRepo}
		principal := auth.Principal{}

		dbs, err := svc.ListDatabases(ctx, principal)
		if err == nil {
			t.Error("expected unauthenticated error, got nil")
		}
		if dbs != nil {
			t.Errorf("expected nil databases, got %v", dbs)
		}
	})
}

func TestGetDatabase(t *testing.T) {
	ctx := context.Background()

	t.Run("returns database with effective access", func(t *testing.T) {
		mockRepo := &mockRepository{
			dbSummary: makeDatabase("db-1", "Database 1", "db1"),
			grants: []models.GroupAccess{
				makeGrant("group-1", "db-1", models.AccessViewEdit),
			},
		}
		svc := &Service{Repository: mockRepo}
		principal := makePrincipal("user-1", false)

		db, err := svc.GetDatabase(ctx, principal, "db-1")
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if db.ID != "db-1" {
			t.Errorf("expected db-1, got %s", db.ID)
		}
		if db.EffectiveAccess != api.AccessViewEdit {
			t.Errorf("expected VIEW_EDIT, got %s", db.EffectiveAccess)
		}
	})

	t.Run("not found when database disabled", func(t *testing.T) {
		mockRepo := &mockRepository{
			summaryErr: ErrNotFound, // using our error var
			grants: []models.GroupAccess{
				makeGrant("group-1", "db-1", models.AccessViewOnly),
			},
		}
		svc := &Service{Repository: mockRepo}
		principal := makePrincipal("user-1", false)

		db, err := svc.GetDatabase(ctx, principal, "db-1")
		if err == nil {
			t.Error("expected not found error, got nil")
		}
		if db.ID != "" {
			t.Errorf("expected empty database, got %v", db)
		}
	})

	t.Run("forbidden when no grant", func(t *testing.T) {
		mockRepo := &mockRepository{
			dbSummary: makeDatabase("db-1", "Database 1", "db1"),
			grants:    []models.GroupAccess{},
		}
		svc := &Service{Repository: mockRepo}
		principal := makePrincipal("user-1", false)

		db, err := svc.GetDatabase(ctx, principal, "db-1")
		if err == nil {
			t.Error("expected forbidden error, got nil")
		}
		if db.ID != "" {
			t.Errorf("expected empty database, got %v", db)
		}
	})
}