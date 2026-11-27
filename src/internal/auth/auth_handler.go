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

func (h *Handler) setSessionCookie(w http.ResponseWriter, value string, expires time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:     h.Service.Policy.CookieName,
		Value:    value,
		Path:     "/",
		Expires:  expires,
		HttpOnly: true,
		Secure:   h.Service.Policy.CookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
}

func (h *Handler) setCSRFCookie(w http.ResponseWriter, value string, expires time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:     "csrf_token",
		Value:    value,
		Path:     "/",
		Expires:  expires,
		HttpOnly: false,
		Secure:   h.Service.Policy.CookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
}

func (h * Handler) clearCookie(w http.ResponseWriter) {
	for _, name := range []string{h.Service.Policy.CookieName, "csrf_token"} {
		http.SetCookie(w, &http.Cookie{
			Name: name,
			Value: "",
			Path: "/",
			MaxAge: -1,
			Expires: time.Unix(0,0),
			HttpOnly: name == h.Service.Policy.CookieName, 
			Secure: h.Service.Policy.CookieSecure,
			SameSite: http.SameSiteLaxMode,
		})
	}
}

func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")

	r.Body = http.MaxBytesReader(w, r.Body, 8<<10)
	
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	var input api.LoginRequest
	errDecode := decoder.Decode(&input)
	if errDecode != nil {
		api.WriteAPIError(w, http.StatusBadRequest, "INVALID_BODY", "Invalid login request")
		return
	}

	errDecode = decoder.Decode(&struct{}{})
	if errDecode!= io.EOF {
		api.WriteAPIError(w, http.StatusBadRequest, "INVALID_BODY", "Expected one JSON object")
		return
	}

	result, err := h.Service.Login(r.Context(), input, time.Now().UTC())
	if err != nil {
		WriteAuthError(w, err)
		return
	}

	h.setSessionCookie(w, result.SessionToken, result.Session.AbsoluteExpiresAt)
	h.setCSRFCookie(w, result.CSRFToken, result.Session.AbsoluteExpiresAt)

	WriteJSON(w, api.DataResponse[api.LoginResponse]{Data: result.User})
}

func (h *Handler) Me(w http.ResponseWriter, r *http.Request) {
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

func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
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

	h.clearCookie(w)
	
	WriteJSON(w, api.DataResponse[api.LogoutResult]{
		Data: api.LogoutResult{Success: true},
	})
}
