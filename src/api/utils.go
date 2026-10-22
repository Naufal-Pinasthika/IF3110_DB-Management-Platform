package api

import (
	"encoding/json"
	"net/http"
)

func databaseAccessMiddleware(middleware RouteMiddleware, access AccessLevel) Middleware {
	if middleware.DatabaseAccess == nil {
		return nil
	}
	return middleware.DatabaseAccess(access)
}

func WriteAPIError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(ErrorResponse{
		Error: APIError{Code: code, Message: message},
	})
}