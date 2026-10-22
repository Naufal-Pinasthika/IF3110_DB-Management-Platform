package auth

import (
	"context"
	"net/http"

	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/api"
)

// PrincipalFromContext reads identity written by session middleware.
func PrincipalFromContext(ctx context.Context) (Principal, bool) {
	// TODO Use a private typed context key shared with SessionMiddleware.
	// TODO Return false when no validated principal exists and callers must deny access.
	return Principal{}, false
}

// SessionMiddleware validates a cookie before calling the protected handler.
func (h *Handler) SessionMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// TODO Read the configured cookie and call ValidateSession with the current time.
		// TODO Return 401 for invalid sessions and update the cookie after rotation.
		// TODO Put the validated principal and session in context then call next.
		api.WriteAPIError(w, http.StatusNotImplemented, "NOT_IMPLEMENTED", "SessionMiddleware is not implemented")
	})
}

// CSRFMiddleware protects authenticated requests that change state.
func (h *Handler) CSRFMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// TODO Read the validated session and X-CSRF-Token then call ValidateCSRF.
		// TODO Return 403 for a mismatch and call next only after successful validation.
		// TODO Protect POST queries as required by the existing route definition.
		api.WriteAPIError(w, http.StatusNotImplemented, "NOT_IMPLEMENTED", "CSRFMiddleware is not implemented")
	})
}

// LoginCSRFMiddleware protects login before an authenticated session exists.
func (h *Handler) LoginCSRFMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// TODO Choose anonymous form tokens or strict Origin validation with a documented fallback.
		// TODO Reject invalid requests before login without requiring an authenticated session.
		// TODO Set RouteMiddleware.LoginCSRF since nil currently skips this protection.
		api.WriteAPIError(w, http.StatusNotImplemented, "NOT_IMPLEMENTED", "LoginCSRFMiddleware is not implemented")
	})
}
