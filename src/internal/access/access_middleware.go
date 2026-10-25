package access

import (
	"net/http"

	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/api"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/auth"
)

// checks platform management permission after session validation.
func (s *Service) PlatformAdminMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		principal, ok := auth.PrincipalFromContext(r.Context())
		if !ok {
			api.WriteAPIError(w, http.StatusUnauthorized, "UNAUTHENTICATED", "Missing authenticated principal")
			return
		}

		if err := s.RequirePlatformAdmin(r.Context(), principal); err != nil {
			api.WriteAPIError(w, http.StatusForbidden, "FORBIDDEN", "Platform admin access required")
			return
		}

		next.ServeHTTP(w, r)
	})
}

// creates the guard expected by RouteMiddleware.DatabaseAccess.
func (s *Service) DatabaseAccessMiddleware(required api.AccessLevel) api.Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			principal, ok := auth.PrincipalFromContext(r.Context())
			if !ok {
				api.WriteAPIError(w, http.StatusUnauthorized, "UNAUTHENTICATED", "Missing authenticated principal")
				return
			}

			// Extract dbId
			dbID := extractDBIDFromPath(r.URL.Path)
			if dbID == "" {
				api.WriteAPIError(w, http.StatusBadRequest, "BAD_REQUEST", "Missing database ID")
				return
			}

			if err := s.RequireDatabase(r.Context(), principal, dbID, required); err != nil {
				api.WriteAPIError(w, http.StatusForbidden, "FORBIDDEN", "Insufficient database access")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// extracts the database ID from a path like /api/v1/databases/{dbId}/...
func extractDBIDFromPath(path string) string {
	prefix := "/api/v1/databases/"
	idx := findSubstring(path, prefix)
	if idx == -1 {
		return ""
	}
	start := idx + len(prefix)
	if start >= len(path) {
		return ""
	}
	// Find next slash
	end := start
	for end < len(path) && path[end] != '/' {
		end++
	}
	if end == start {
		return ""
	}
	return path[start:end]
}

func findSubstring(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
