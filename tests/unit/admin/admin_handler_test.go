package admin_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/access"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/admin"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/auth"
)

// TestMapErrorForbidden ensures a rejected access check maps the access
// package's forbidden sentinel to 403 instead of falling through to 500.
func TestMapErrorForbidden(t *testing.T) {
	svc := &admin.Service{
		Repository: &mockRepository{},
		Access:     &mockAccess{err: access.ErrForbidden},
	}
	handler := &admin.Handler{Service: svc}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/users", nil)
	ctx := auth.SetValidatedPrincipalContext(req.Context(), makePrincipal("user-1", false))
	req = req.WithContext(ctx)
	rr := httptest.NewRecorder()

	handler.ListUsers(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rr.Code)
	}
}

// TestMapErrorUnauthenticated ensures the access package's unauthenticated
// sentinel maps to 401.
func TestMapErrorUnauthenticated(t *testing.T) {
	svc := &admin.Service{
		Repository: &mockRepository{},
		Access:     &mockAccess{err: access.ErrUnauthenticated},
	}
	handler := &admin.Handler{Service: svc}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/users", nil)
	ctx := auth.SetValidatedPrincipalContext(req.Context(), makePrincipal("user-1", false))
	req = req.WithContext(ctx)
	rr := httptest.NewRecorder()

	handler.ListUsers(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rr.Code)
	}
}
