package access

import (
	"net/http"

	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/api"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/auth"
)

type Handler struct {
	Service *Service
}

// ListDatabases handles GET /api/v1/databases.
func (h *Handler) ListDatabases(w http.ResponseWriter, r *http.Request) {
	principal, ok := auth.PrincipalFromContext(r.Context())
	if !ok {
		api.WriteAPIError(w, http.StatusUnauthorized, "UNAUTHENTICATED", "Please log in again")
		return
	}

	databases, err := h.Service.ListDatabases(r.Context(), principal)
	if err != nil {
		h.mapError(w, err)
		return
	}

	api.WriteJSON(w, api.DataResponse[[]api.UserDatabase]{Data: databases})
}

// GetDatabase handles GET /api/v1/databases/{databaseId}.
func (h *Handler) GetDatabase(w http.ResponseWriter, r *http.Request) {
	principal, ok := auth.PrincipalFromContext(r.Context())
	if !ok {
		api.WriteAPIError(w, http.StatusUnauthorized, "UNAUTHENTICATED", "Please log in again")
		return
	}

	databaseID := r.PathValue("databaseId")
	if databaseID == "" {
		api.WriteAPIError(w, http.StatusBadRequest, "INVALID_ID", "database id required")
		return
	}

	db, err := h.Service.GetDatabase(r.Context(), principal, databaseID)
	if err != nil {
		h.mapError(w, err)
		return
	}

	api.WriteJSON(w, api.DataResponse[api.UserDatabase]{Data: db})
}

func (h *Handler) mapError(w http.ResponseWriter, err error) {
	switch err {
	case ErrUnauthenticated:
		api.WriteAPIError(w, http.StatusUnauthorized, "UNAUTHENTICATED", "Please log in again")
	case ErrForbidden:
		api.WriteAPIError(w, http.StatusForbidden, "FORBIDDEN", "Access denied")
	case ErrNotFound:
		api.WriteAPIError(w, http.StatusNotFound, "NOT_FOUND", "Database not found")
	default:
		api.WriteAPIError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Internal server error")
	}
}