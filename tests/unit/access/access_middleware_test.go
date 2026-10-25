package access

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/api"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/auth"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/models"
)

// We need to embed the real Service to test middleware, so we'll test via the actual Service with mock repo
func TestPlatformAdminMiddleware(t *testing.T) {
	mockRepo := &mockRepository{}
	svc := &Service{Repository: mockRepo}

	// Create a test handler that succeeds
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	middleware := svc.PlatformAdminMiddleware(testHandler)

	t.Run("allows platform admin", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		ctx := SetPrincipalContext(req.Context(), auth.Principal{
			User: api.LoginResponse{
				ID:              "user-1",
				Username:        "admin",
				IsPlatformAdmin: true,
			},
			SessionID: "session-123",
		})
		req = req.WithContext(ctx)

		rr := httptest.NewRecorder()
		middleware.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", rr.Code)
		}
	})

	t.Run("rejects non-platform admin", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		ctx := SetPrincipalContext(req.Context(), auth.Principal{
			User: api.LoginResponse{
				ID:              "user-1",
				Username:        "user",
				IsPlatformAdmin: false,
			},
			SessionID: "session-123",
		})
		req = req.WithContext(ctx)

		rr := httptest.NewRecorder()
		middleware.ServeHTTP(rr, req)

		if rr.Code != http.StatusForbidden {
			t.Errorf("expected 403, got %d", rr.Code)
		}
	})

	t.Run("rejects missing principal", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		// No principal in context

		rr := httptest.NewRecorder()
		middleware.ServeHTTP(rr, req)

		if rr.Code != http.StatusUnauthorized {
			t.Errorf("expected 401, got %d", rr.Code)
		}
	})
}

func TestDatabaseAccessMiddleware(t *testing.T) {
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	t.Run("allows VIEW_EDIT when required VIEW_ONLY", func(t *testing.T) {
		mockRepo := &mockRepository{
			grants: []models.GroupAccess{
				{GroupID: "group-1", DatabaseID: "db-1", AccessLevel: models.AccessViewEdit},
			},
		}
		svc := &Service{Repository: mockRepo}
		middleware := svc.DatabaseAccessMiddleware(api.AccessViewOnly)(testHandler)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/databases/db-1/schemas", nil)
		ctx := SetPrincipalContext(req.Context(), auth.Principal{
			User: api.LoginResponse{
				ID:              "user-1",
				Username:        "user",
				IsPlatformAdmin: false,
			},
			SessionID: "session-123",
		})
		req = req.WithContext(ctx)

		rr := httptest.NewRecorder()
		middleware.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", rr.Code)
		}
	})

	t.Run("allows VIEW_EDIT when required VIEW_EDIT", func(t *testing.T) {
		mockRepo := &mockRepository{
			grants: []models.GroupAccess{
				{GroupID: "group-1", DatabaseID: "db-1", AccessLevel: models.AccessViewEdit},
			},
		}
		svc := &Service{Repository: mockRepo}
		middleware := svc.DatabaseAccessMiddleware(api.AccessViewEdit)(testHandler)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/databases/db-1/schemas", nil)
		ctx := SetPrincipalContext(req.Context(), auth.Principal{
			User: api.LoginResponse{
				ID:              "user-1",
				Username:        "user",
				IsPlatformAdmin: false,
			},
			SessionID: "session-123",
		})
		req = req.WithContext(ctx)

		rr := httptest.NewRecorder()
		middleware.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", rr.Code)
		}
	})

	t.Run("rejects VIEW_ONLY when required VIEW_EDIT", func(t *testing.T) {
		mockRepo := &mockRepository{
			grants: []models.GroupAccess{
				{GroupID: "group-1", DatabaseID: "db-1", AccessLevel: models.AccessViewOnly},
			},
		}
		svc := &Service{Repository: mockRepo}
		middleware := svc.DatabaseAccessMiddleware(api.AccessViewEdit)(testHandler)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/databases/db-1/migrations", nil)
		ctx := SetPrincipalContext(req.Context(), auth.Principal{
			User: api.LoginResponse{
				ID:              "user-1",
				Username:        "user",
				IsPlatformAdmin: false,
			},
			SessionID: "session-123",
		})
		req = req.WithContext(ctx)

		rr := httptest.NewRecorder()
		middleware.ServeHTTP(rr, req)

		if rr.Code != http.StatusForbidden {
			t.Errorf("expected 403, got %d", rr.Code)
		}
	})

	t.Run("rejects missing principal", func(t *testing.T) {
		mockRepo := &mockRepository{
			grants: []models.GroupAccess{
				{GroupID: "group-1", DatabaseID: "db-1", AccessLevel: models.AccessViewEdit},
			},
		}
		svc := &Service{Repository: mockRepo}
		middleware := svc.DatabaseAccessMiddleware(api.AccessViewOnly)(testHandler)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/databases/db-1/schemas", nil)
		// No principal

		rr := httptest.NewRecorder()
		middleware.ServeHTTP(rr, req)

		if rr.Code != http.StatusUnauthorized {
			t.Errorf("expected 401, got %d", rr.Code)
		}
	})

	t.Run("rejects missing dbId", func(t *testing.T) {
		mockRepo := &mockRepository{
			grants: []models.GroupAccess{
				{GroupID: "group-1", DatabaseID: "db-1", AccessLevel: models.AccessViewEdit},
			},
		}
		svc := &Service{Repository: mockRepo}
		middleware := svc.DatabaseAccessMiddleware(api.AccessViewOnly)(testHandler)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/databases/", nil)
		ctx := SetPrincipalContext(req.Context(), auth.Principal{
			User: api.LoginResponse{
				ID:              "user-1",
				Username:        "user",
				IsPlatformAdmin: false,
			},
			SessionID: "session-123",
		})
		req = req.WithContext(ctx)

		rr := httptest.NewRecorder()
		middleware.ServeHTTP(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", rr.Code)
		}
	})
}