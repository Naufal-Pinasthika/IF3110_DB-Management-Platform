package access_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/api"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/auth"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/models"
)

// nextHandler records whether the wrapped handler was reached.
func nextHandler(reached *bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*reached = true
		w.WriteHeader(http.StatusOK)
	})
}

func TestPlatformAdminMiddleware(t *testing.T) {
	svc := newService(&mockRepository{})

	t.Run("missing principal unauthorized", func(t *testing.T) {
		reached := false
		mw := svc.PlatformAdminMiddleware(nextHandler(&reached))
		req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/users", nil)
		rr := httptest.NewRecorder()
		mw.ServeHTTP(rr, req)
		if rr.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", rr.Code)
		}
		if reached {
			t.Fatal("next handler reached, want blocked")
		}
	})

	t.Run("regular user forbidden", func(t *testing.T) {
		reached := false
		mw := svc.PlatformAdminMiddleware(nextHandler(&reached))
		req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/users", nil)
		ctx := auth.SetPrincipalContext(req.Context(), makePrincipal("user-1", false))
		req = req.WithContext(ctx)
		rr := httptest.NewRecorder()
		mw.ServeHTTP(rr, req)
		if rr.Code != http.StatusForbidden {
			t.Fatalf("status = %d, want 403", rr.Code)
		}
		if reached {
			t.Fatal("next handler reached, want blocked")
		}
	})

	t.Run("platform admin allowed", func(t *testing.T) {
		reached := false
		mw := svc.PlatformAdminMiddleware(nextHandler(&reached))
		req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/users", nil)
		ctx := auth.SetPrincipalContext(req.Context(), makePrincipal("user-1", true))
		req = req.WithContext(ctx)
		rr := httptest.NewRecorder()
		mw.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rr.Code)
		}
		if !reached {
			t.Fatal("next handler not reached, want allowed")
		}
	})
}

func TestDatabaseAccessMiddleware(t *testing.T) {
	t.Run("missing principal unauthorized", func(t *testing.T) {
		svc := newService(&mockRepository{})
		reached := false
		mw := svc.DatabaseAccessMiddleware(api.AccessViewOnly)(nextHandler(&reached))
		req := httptest.NewRequest(http.MethodGet, "/api/v1/databases/db-1/schemas", nil)
		rr := httptest.NewRecorder()
		mw.ServeHTTP(rr, req)
		if rr.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", rr.Code)
		}
		if reached {
			t.Fatal("next handler reached, want blocked")
		}
	})

	t.Run("missing database id bad request", func(t *testing.T) {
		svc := newService(&mockRepository{})
		reached := false
		mw := svc.DatabaseAccessMiddleware(api.AccessViewOnly)(nextHandler(&reached))
		req := httptest.NewRequest(http.MethodGet, "/api/v1/databases/", nil)
		ctx := auth.SetPrincipalContext(req.Context(), makePrincipal("user-1", false))
		req = req.WithContext(ctx)
		rr := httptest.NewRecorder()
		mw.ServeHTTP(rr, req)
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", rr.Code)
		}
		if reached {
			t.Fatal("next handler reached, want blocked")
		}
	})

	t.Run("insufficient access forbidden", func(t *testing.T) {
		svc := newService(&mockRepository{
			grants: []models.GroupAccess{makeGrant("group-1", "db-1", models.AccessViewOnly)},
		})
		reached := false
		mw := svc.DatabaseAccessMiddleware(api.AccessViewEdit)(nextHandler(&reached))
		req := httptest.NewRequest(http.MethodGet, "/api/v1/databases/db-1/migrations", nil)
		ctx := auth.SetPrincipalContext(req.Context(), makePrincipal("user-1", false))
		req = req.WithContext(ctx)
		rr := httptest.NewRecorder()
		mw.ServeHTTP(rr, req)
		if rr.Code != http.StatusForbidden {
			t.Fatalf("status = %d, want 403", rr.Code)
		}
		if reached {
			t.Fatal("next handler reached, want blocked")
		}
	})

	t.Run("sufficient access allowed", func(t *testing.T) {
		svc := newService(&mockRepository{
			grants: []models.GroupAccess{makeGrant("group-1", "db-1", models.AccessViewEdit)},
		})
		reached := false
		mw := svc.DatabaseAccessMiddleware(api.AccessViewEdit)(nextHandler(&reached))
		req := httptest.NewRequest(http.MethodGet, "/api/v1/databases/db-1/migrations", nil)
		ctx := auth.SetPrincipalContext(req.Context(), makePrincipal("user-1", false))
		req = req.WithContext(ctx)
		rr := httptest.NewRecorder()
		mw.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rr.Code)
		}
		if !reached {
			t.Fatal("next handler not reached, want allowed")
		}
	})
}

func TestPrincipalContextRoundTrip(t *testing.T) {
	principal := makePrincipal("user-1", true)
	ctx := auth.SetPrincipalContext(context.Background(), principal)
	got, ok := auth.PrincipalFromContext(ctx)
	if !ok {
		t.Fatal("principal missing from context")
	}
	if got.User.ID != "user-1" || !got.User.IsPlatformAdmin {
		t.Fatalf("principal = %+v, want user-1 admin", got)
	}
}

func TestPrincipalFromContextMissing(t *testing.T) {
	if _, ok := auth.PrincipalFromContext(context.Background()); ok {
		t.Fatal("expected no principal, got one")
	}
}
