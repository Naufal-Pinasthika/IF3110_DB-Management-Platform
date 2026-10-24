package auth

import (
	"encoding/json"
	"io"
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
// /api/v1/auth/login
func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	// no-store since this is auth handling
	w.Header().Set("Cache-Control", "no-store")

	// limit request body only 8kb
	r.Body = http.MaxBytesReader(w, r.Body, 8<<10)
	
	// decode api.LoginRequest with a body size limit and call Service.Login
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields() // add strict rule when unmarshling json to struct to prevent malicious additional input fields (ex: adding is_admin)

	var input api.LoginRequest
	errDecode := decoder.Decode(&input)
	if errDecode != nil {
		api.WriteAPIError(w, http.StatusBadRequest, "INVALID_BODY", "Invalid login request")
		return
	}

	// make sure json request body is properly validated untill eof
	errDecode = decoder.Decode(&struct{}{})
	if errDecode!= io.EOF {
		api.WriteAPIError(w, http.StatusBadRequest, "INVALID_BODY", "Expected one JSON object")
		return
	}

	// pass the credentials into service
	result, err := h.Service.Login(r.Context(), input, time.Now().UTC())
	if err != nil {
		WriteAuthError(w, err)
		return
	}



	// Set  HttpOnly and SameSite (csrf) cookie with Secure on production (HTTPS)
	h.setCookie(w, h.Service.Policy.CookieName, result.SessionToken, true, result.Session.AbsoluteExpiresAt)
	h.setCookie(w, "csrf_token", result.CSRFToken, false, result.Session.AbsoluteExpiresAt)

	// DataResponse uses Generic Wrapper [T any] {Data T}, where T is LoginResponse
	// since LoginResponse.User exist, we pass ONLY the User field into schema (LoginResponse.User = Data)
	// this is because Login() only need to rply with User response pattern
	// expect alot of this pattern will exist accross auth handler
	WriteJSON(w, api.DataResponse[api.LoginResponse]{Data: result.User})
}

// /api/v1/auth/me
func (h *Handler) Me(w http.ResponseWriter, r *http.Request) {
	// read the validated principal from context and call Service.Me
	principal, ok := PrincipalFromContext(r.Context())
	if !ok {
		WriteAuthError(w, ErrUnauthenticated)
		return
	}

	user, err := h.Service.Me(r.Context(), principal)
	if err != nil {
		WriteAuthError(w, err)
		return
	}

	WriteJSON(w, api.DataResponse[api.LoginResponse]{Data: user})
}

// POST /api/v1/auth/logout
func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	// validate session and CSRF guard then call Service.Logout
	principal, ok := PrincipalFromContext(r.Context())
	if !ok {
		WriteAuthError(w, ErrUnauthenticated)
		return
	}

	err := h.Service.Logout(r.Context(), principal, time.Now().UTC())
	if err != nil {
		WriteAuthError(w, err)
		return
	}

	// clear cookies 
	h.clearCookie(w)
	
	WriteJSON(w, api.DataResponse[api.LogoutResult]{
		Data: api.LogoutResult{Success: true},
	})
}
