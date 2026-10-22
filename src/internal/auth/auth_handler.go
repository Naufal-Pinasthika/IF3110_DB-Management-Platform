package auth

import (
	"net/http"

	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/api"
)

type Handler struct {
	Service *Service
}

// Login handles POST /api/v1/auth/login.
func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	// TODO Decode api.LoginRequest with a body size limit and call Service.Login.
	// TODO Set an HttpOnly and SameSite cookie with Secure on production HTTPS.
	// TODO Return only result.User using api.DataResponse with status 200.
	// TODO Deliver the separate CSRF token through the chosen form or header mechanism.
	// TODO Map service errors to the existing API error envelope.
	api.WriteAPIError(w, http.StatusNotImplemented, "NOT_IMPLEMENTED", "Login is not implemented")
}

// Me handles GET /api/v1/auth/me.
func (h *Handler) Me(w http.ResponseWriter, r *http.Request) {
	// TODO Read the validated principal from context and call Service.Me.
	// TODO Return api.LoginResponse inside api.DataResponse with status 200.
	// TODO Map service errors to the existing API error envelope.
	api.WriteAPIError(w, http.StatusNotImplemented, "NOT_IMPLEMENTED", "Me is not implemented")
}

// Logout handles POST /api/v1/auth/logout.
func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	// TODO Require the validated session and CSRF guard then call Service.Logout.
	// TODO Expire the cookie using the same name and path and security attributes.
	// TODO Return api.LogoutResult inside api.DataResponse with status 200.
	// TODO Map service errors to the existing API error envelope.
	api.WriteAPIError(w, http.StatusNotImplemented, "NOT_IMPLEMENTED", "Logout is not implemented")
}
