package access

import (
	"context"
	"net/http"

	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/api"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/auth"
)

// contextKey is a private type for context keys to avoid collisions.
type contextKey string

const (
	// principalContextKey is the key used to store/retrieve Principal from request context.
	// This must match the key used in auth.SessionMiddleware.
	principalContextKey contextKey = "principal"
)

// PrincipalFromContext reads the Principal from request context.
// Returns zero Principal and false if not found.
func PrincipalFromContext(ctx context.Context) (auth.Principal, bool) {
	principal, ok := ctx.Value(principalContextKey).(auth.Principal)
	return principal, ok
}

// SetPrincipalContext stores the Principal in request context.
func SetPrincipalContext(ctx context.Context, principal auth.Principal) context.Context {
	return context.WithValue(ctx, principalContextKey, principal)
}

// PlatformAdminMiddleware checks platform management permission after session validation.
func (s *Service) PlatformAdminMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		principal, ok := PrincipalFromContext(r.Context())
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

// DatabaseAccessMiddleware creates the guard expected by RouteMiddleware.DatabaseAccess.
func (s *Service) DatabaseAccessMiddleware(required api.AccessLevel) api.Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			principal, ok := PrincipalFromContext(r.Context())
			if !ok {
				api.WriteAPIError(w, http.StatusUnauthorized, "UNAUTHENTICATED", "Missing authenticated principal")
				return
			}

			// Extract dbId from path values (Go 1.22+)
			dbID := r.PathValue("dbId")
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
