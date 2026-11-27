package access

import (
	"log/slog"
	"net/http"

	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/api"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/auth"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/telemetry"
)

func (s *Service) PlatformAdminMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		principal, ok := auth.PrincipalFromContext(r.Context())
		if !ok {
			telemetry.Log(r.Context(), slog.LevelWarn, "access.denied", "path", r.URL.Path, "required", "PLATFORM_ADMIN", "status", "unauthenticated")
			api.WriteAPIError(w, http.StatusUnauthorized, "UNAUTHENTICATED", "Missing authenticated principal")
			return
		}

		if err := s.RequirePlatformAdmin(r.Context(), principal); err != nil {
			telemetry.Log(r.Context(), slog.LevelWarn, "access.denied", "actor", principal.User.ID, "path", r.URL.Path, "required", "PLATFORM_ADMIN", "status", "forbidden")
			api.WriteAPIError(w, http.StatusForbidden, "FORBIDDEN", "Platform admin access required")
			return
		}

		next.ServeHTTP(w, r)
	})
}

func (s *Service) DatabaseAccessMiddleware(required api.AccessLevel) api.Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			principal, ok := auth.PrincipalFromContext(r.Context())
			if !ok {
				telemetry.Log(r.Context(), slog.LevelWarn, "access.denied", "path", r.URL.Path, "required", string(required), "status", "unauthenticated")
				api.WriteAPIError(w, http.StatusUnauthorized, "UNAUTHENTICATED", "Missing authenticated principal")
				return
			}

			dbID := extractDBIDFromPath(r.URL.Path)
			if dbID == "" {
				api.WriteAPIError(w, http.StatusBadRequest, "BAD_REQUEST", "Missing database ID")
				return
			}

			ctx := telemetry.WithDatabase(r.Context(), dbID)
			if err := s.RequireDatabase(ctx, principal, dbID, required); err != nil {
				telemetry.Log(ctx, slog.LevelWarn, "access.denied", "actor", principal.User.ID, "database_id", dbID, "path", r.URL.Path, "required", string(required), "status", "forbidden")
				api.WriteAPIError(w, http.StatusForbidden, "FORBIDDEN", "Insufficient database access")
				return
			}

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

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
