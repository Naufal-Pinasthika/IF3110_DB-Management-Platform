package auth

import (
	"net/http"
	"time"

	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/api"
)

type Handler struct {
	Service *Service
	PublicOrigin string
}

// ---------------------------------------------------------------------------------------------------------------
// cookies function thats bascially wrapper from http function 
// https://medium.com/@adrilukman73/the-simple-guide-to-cookies-in-golang-create-read-and-delete-a5397732b04b

// will set-cookies to client browser via http response header 
func (h * Handler) setCookie(w http.ResponseWriter, name string, value string, httpOnly bool, expires time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name: name,
		Value: value,
		Path: "/", // valid for any place in this web
		Expires: expires,
		HttpOnly: httpOnly, // flag security for js
		Secure: h.Service.Policy.CookieSecure, // https
		SameSite: http.SameSiteLaxMode, // prevent csrf
	})
}

// revoke cookies thats available (in this case h.Service.Policy.CookieName and csrf_token)
// we use MaxAge: -1 to instruct the browser to delete the cookies immediately, and set value to ""
func (h * Handler) clearCookie(w http.ResponseWriter) {
	// uses for loop range, range returns idx, value
	// since we dont need any idx, use blank identifier (_)
	for _, name := range []string{h.Service.Policy.CookieName, "csrf_token"} {
		http.SetCookie(w, &http.Cookie{
			Name: name,
			Value: "",
			Path: "/", // valid for any place in this web
			MaxAge: -1,
			Expires: time.Unix(0,0), // since we also enforce expiration, we set this to 00:00:00 UTC 1 Jan 1970 https://en.wikipedia.org/wiki/Unix_time
			HttpOnly: name == h.Service.Policy.CookieName, 
			Secure: h.Service.Policy.CookieSecure, // https
			SameSite: http.SameSiteLaxMode, // prevent csrf			
		})
	}
}

// ---------------------------------------------------------------------------------------------------------------
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
