package access

import (
	"net/http"

	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/api"
)

// PlatformAdminMiddleware checks platform management permission after session validation.
func (s *Service) PlatformAdminMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// TODO Get the principal with auth.PrincipalFromContext and reject a missing identity.
		// TODO Call RequirePlatformAdmin and return 403 on denial before calling next.
		api.WriteAPIError(w, http.StatusNotImplemented, "NOT_IMPLEMENTED", "PlatformAdminMiddleware is not implemented")
	})
}

// DatabaseAccessMiddleware creates the guard expected by RouteMiddleware.DatabaseAccess.
func (s *Service) DatabaseAccessMiddleware(required api.AccessLevel) api.Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// TODO Get a validated principal and the dbId path value.
			// TODO Call RequireDatabase and deny failures before calling next.
			// TODO Keep service checks too so direct service calls cannot skip policy.
			api.WriteAPIError(w, http.StatusNotImplemented, "NOT_IMPLEMENTED", "DatabaseAccessMiddleware is not implemented")
		})
	}
}
