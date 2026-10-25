package access_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/api"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/access"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/apperror"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/auth"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/models"
)

func newService(repo access.Repository) *access.Service {
	return &access.Service{Repository: repo}
}

func TestRequirePlatformAdmin(t *testing.T) {
	ctx := context.Background()
	svc := newService(&mockRepository{})

	tests := []struct {
		name      string
		principal auth.Principal
		wantErr   error
	}{
		{"platform admin allowed", makePrincipal("user-1", true), nil},
		{"regular user forbidden", makePrincipal("user-1", false), access.ErrForbidden},
		{"missing identity unauthenticated", auth.Principal{}, access.ErrUnauthenticated},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := svc.RequirePlatformAdmin(ctx, tc.principal)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("error = %v, want %v", err, tc.wantErr)
			}
		})
	}
}

func TestEffectiveAccess(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name      string
		principal auth.Principal
		grants    []models.GroupAccess
		grantsErr error
		wantLevel api.AccessLevel
		wantErr   error
	}{
		{
			name:      "missing identity unauthenticated",
			principal: auth.Principal{},
			wantErr:   access.ErrUnauthenticated,
		},
		{
			name:      "no grants forbidden",
			principal: makePrincipal("user-1", false),
			grants:    []models.GroupAccess{},
			wantErr:   access.ErrForbidden,
		},
		{
			name:      "view only when all grants view only",
			principal: makePrincipal("user-1", false),
			grants: []models.GroupAccess{
				makeGrant("group-1", "db-1", models.AccessViewOnly),
				makeGrant("group-2", "db-1", models.AccessViewOnly),
			},
			wantLevel: api.AccessViewOnly,
		},
		{
			name:      "view edit wins over view only",
			principal: makePrincipal("user-1", false),
			grants: []models.GroupAccess{
				makeGrant("group-1", "db-1", models.AccessViewOnly),
				makeGrant("group-2", "db-1", models.AccessViewEdit),
			},
			wantLevel: api.AccessViewEdit,
		},
		{
			name:      "repository error propagated",
			principal: makePrincipal("user-1", false),
			grantsErr: errUnavailable,
			wantErr:   errUnavailable,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := &mockRepository{grants: tc.grants, grantsErr: tc.grantsErr}
			svc := newService(repo)
			level, err := svc.EffectiveAccess(ctx, tc.principal, "db-1")
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("error = %v, want %v", err, tc.wantErr)
			}
			if err == nil && level != tc.wantLevel {
				t.Fatalf("level = %q, want %q", level, tc.wantLevel)
			}
		})
	}
}

func TestRequireDatabase(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name      string
		principal auth.Principal
		grants    []models.GroupAccess
		required  api.AccessLevel
		wantErr   error
	}{
		{
			name:      "missing identity unauthenticated",
			principal: auth.Principal{},
			required:  api.AccessViewOnly,
			wantErr:   access.ErrUnauthenticated,
		},
		{
			name:      "invalid required level forbidden",
			principal: makePrincipal("user-1", false),
			required:  "WRITE",
			wantErr:   access.ErrForbidden,
		},
		{
			name:      "view only satisfied by view only",
			principal: makePrincipal("user-1", false),
			grants:    []models.GroupAccess{makeGrant("group-1", "db-1", models.AccessViewOnly)},
			required:  api.AccessViewOnly,
		},
		{
			name:      "view only satisfied by view edit",
			principal: makePrincipal("user-1", false),
			grants:    []models.GroupAccess{makeGrant("group-1", "db-1", models.AccessViewEdit)},
			required:  api.AccessViewOnly,
		},
		{
			name:      "view edit satisfied by view edit",
			principal: makePrincipal("user-1", false),
			grants:    []models.GroupAccess{makeGrant("group-1", "db-1", models.AccessViewEdit)},
			required:  api.AccessViewEdit,
		},
		{
			name:      "view edit rejected by view only",
			principal: makePrincipal("user-1", false),
			grants:    []models.GroupAccess{makeGrant("group-1", "db-1", models.AccessViewOnly)},
			required:  api.AccessViewEdit,
			wantErr:   access.ErrForbidden,
		},
		{
			name:      "no grants forbidden",
			principal: makePrincipal("user-1", false),
			grants:    []models.GroupAccess{},
			required:  api.AccessViewOnly,
			wantErr:   access.ErrForbidden,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := &mockRepository{grants: tc.grants}
			svc := newService(repo)
			err := svc.RequireDatabase(ctx, tc.principal, "db-1", tc.required)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("error = %v, want %v", err, tc.wantErr)
			}
		})
	}
}

func TestListDatabases(t *testing.T) {
	ctx := context.Background()

	t.Run("missing identity unauthenticated", func(t *testing.T) {
		svc := newService(&mockRepository{})
		dbs, err := svc.ListDatabases(ctx, auth.Principal{})
		if !errors.Is(err, access.ErrUnauthenticated) {
			t.Fatalf("error = %v, want %v", err, access.ErrUnauthenticated)
		}
		if dbs != nil {
			t.Fatalf("databases = %v, want nil", dbs)
		}
	})

	t.Run("repository error propagated", func(t *testing.T) {
		repo := &mockRepository{databasesErr: errUnavailable}
		svc := newService(repo)
		_, err := svc.ListDatabases(ctx, makePrincipal("user-1", false))
		if !errors.Is(err, errUnavailable) {
			t.Fatalf("error = %v, want %v", err, errUnavailable)
		}
	})

	t.Run("returns granted databases with effective access", func(t *testing.T) {
		repo := &mockRepository{
			databases: []models.ManagedDatabase{
				makeDatabase("db-1", "One", "db1"),
				makeDatabase("db-2", "Two", "db2"),
				makeDatabase("db-3", "Three", "db3"),
			},
			grants: []models.GroupAccess{
				makeGrant("group-1", "db-1", models.AccessViewEdit),
				makeGrant("group-1", "db-2", models.AccessViewOnly),
				// db-3 has no grant for this user
			},
		}
		svc := newService(repo)
		dbs, err := svc.ListDatabases(ctx, makePrincipal("user-1", false))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(dbs) != 2 {
			t.Fatalf("len(databases) = %d, want 2", len(dbs))
		}
		if dbs[0].ID != "db-1" || dbs[0].EffectiveAccess != api.AccessViewEdit {
			t.Fatalf("db-1 = %+v, want VIEW_EDIT", dbs[0])
		}
		if dbs[1].ID != "db-2" || dbs[1].EffectiveAccess != api.AccessViewOnly {
			t.Fatalf("db-2 = %+v, want VIEW_ONLY", dbs[1])
		}
	})
}

func TestGetDatabase(t *testing.T) {
	ctx := context.Background()

	t.Run("missing identity unauthenticated", func(t *testing.T) {
		svc := newService(&mockRepository{})
		_, err := svc.GetDatabase(ctx, auth.Principal{}, "db-1")
		if !errors.Is(err, access.ErrUnauthenticated) {
			t.Fatalf("error = %v, want %v", err, access.ErrUnauthenticated)
		}
	})

	t.Run("no grant forbidden", func(t *testing.T) {
		repo := &mockRepository{
			dbSummary: makeDatabase("db-1", "One", "db1"),
			grants:    []models.GroupAccess{},
		}
		svc := newService(repo)
		_, err := svc.GetDatabase(ctx, makePrincipal("user-1", false), "db-1")
		if !errors.Is(err, access.ErrForbidden) {
			t.Fatalf("error = %v, want %v", err, access.ErrForbidden)
		}
	})

	t.Run("not implemented summary maps to not found", func(t *testing.T) {
		repo := &mockRepository{
			summaryErr: apperror.ErrNotImplemented,
			grants:     []models.GroupAccess{makeGrant("group-1", "db-1", models.AccessViewOnly)},
		}
		svc := newService(repo)
		_, err := svc.GetDatabase(ctx, makePrincipal("user-1", false), "db-1")
		if !errors.Is(err, access.ErrNotFound) {
			t.Fatalf("error = %v, want %v", err, access.ErrNotFound)
		}
	})

	t.Run("summary repository error propagated", func(t *testing.T) {
		repo := &mockRepository{
			summaryErr: errUnavailable,
			grants:     []models.GroupAccess{makeGrant("group-1", "db-1", models.AccessViewOnly)},
		}
		svc := newService(repo)
		_, err := svc.GetDatabase(ctx, makePrincipal("user-1", false), "db-1")
		if !errors.Is(err, errUnavailable) {
			t.Fatalf("error = %v, want %v", err, errUnavailable)
		}
	})

	t.Run("returns summary with effective access", func(t *testing.T) {
		repo := &mockRepository{
			dbSummary: makeDatabase("db-1", "One", "db1"),
			grants:    []models.GroupAccess{makeGrant("group-1", "db-1", models.AccessViewEdit)},
		}
		svc := newService(repo)
		db, err := svc.GetDatabase(ctx, makePrincipal("user-1", false), "db-1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if db.ID != "db-1" || db.Label != "One" || db.DatabaseName != "db1" {
			t.Fatalf("db = %+v, want db-1/One/db1", db)
		}
		if db.EffectiveAccess != api.AccessViewEdit {
			t.Fatalf("effective access = %q, want VIEW_EDIT", db.EffectiveAccess)
		}
	})
}
