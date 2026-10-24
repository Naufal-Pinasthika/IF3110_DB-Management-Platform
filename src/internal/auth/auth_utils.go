package auth

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/api"
)

var (
	ErrInvalidPasswordlength= errors.New("password must contain 1-72 bytes")
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrUnauthenticated = errors.New("invalid session")
	ErrCSRF = errors.New("invalid csrf token")
	errNotFound = errors.New("data record not found")
	errRotationConflict = errors.New("session rotation conflict")	
)

// collection of helper function for auth servies
func WriteAuthError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrInvalidCredentials):
		api.WriteAPIError(w, http.StatusUnauthorized,"INVALID_CREDENTIALS", "Username or password is incorrect")

	case errors.Is(err, ErrUnauthenticated):
		api.WriteAPIError(w, http.StatusUnauthorized,"UNAUTHENTICATED", "Please log in again")

	case errors.Is(err, ErrCSRF):
		api.WriteAPIError(w, http.StatusForbidden,"CSRF_INVALID", "Request verification failed")

	case errors.Is(err, errRotationConflict):
		api.WriteAPIError(w, http.StatusServiceUnavailable,"SESSION_BUSY", "Please retry the request")

	default:
		api.WriteAPIError(w, http.StatusInternalServerError,"INTERNAL_ERROR", "An internal error occurred")
	}
}

func WriteJSON(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(data)
}