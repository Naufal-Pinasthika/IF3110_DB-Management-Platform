package migration

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/api"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/apperror"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/auth"
)


type Handler struct {
	Service *Service
}



// ListMigrations handles GET /api/v1/databases/{dbId}/migrations.
func (h *Handler) ListMigrations(w http.ResponseWriter, r *http.Request) {
	dbID := r.PathValue("dbId")
	status := api.MigrationStatus(r.URL.Query().Get("status"))
	principal, _ := auth.PrincipalFromContext(r.Context())

	summaries, err := h.Service.ListMigrations(r.Context(), principal, dbID, status)
	if err != nil {
		writeError(w, err)
		return
	}

	api.WriteDataResponse(w, http.StatusOK, summaries)
}


// GetMigration handles GET /api/v1/databases/{dbId}/migrations/{migrationId}.
func (h *Handler) GetMigration(w http.ResponseWriter, r *http.Request) {
	dbID := r.PathValue("dbId")
	migrationID := r.PathValue("migrationId")
	principal, _ := auth.PrincipalFromContext(r.Context())

	detail, err := h.Service.GetMigration(r.Context(), principal, dbID, migrationID)
	if err != nil {
		writeError(w, err)
		return
	}

	api.WriteDataResponse(w, http.StatusOK, detail)
}


// CreateMigration handles POST /api/v1/databases/{dbId}/migrations.
func (h *Handler) CreateMigration(w http.ResponseWriter, r *http.Request) {
	dbID := r.PathValue("dbId")
	principal, _ := auth.PrincipalFromContext(r.Context())

	r.Body = http.MaxBytesReader(w, r.Body, 1024*1024) // 1MB body limit
	var req api.CreateMigrationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.WriteAPIError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid JSON body")
		return
	}

	created, err := h.Service.CreateMigration(r.Context(), principal, dbID, req, time.Now())
	if err != nil {
		writeError(w, err)
		return
	}
	
	api.WriteDataResponse(w, http.StatusCreated, created)
}


// UpdateMigration handles PATCH /api/v1/databases/{dbId}/migrations/{migrationId}.
func (h *Handler) UpdateMigration(w http.ResponseWriter, r *http.Request) {
	dbID := r.PathValue("dbId")
	migrationID := r.PathValue("migrationId")
	principal, _ := auth.PrincipalFromContext(r.Context())

	r.Body = http.MaxBytesReader(w, r.Body, 1024*1024)
	var req api.UpdateMigrationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.WriteAPIError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid JSON body")
		return
	}

	updated, err := h.Service.UpdateMigration(r.Context(), principal, dbID, migrationID, req, time.Now())
	if err != nil {
		writeError(w, err)
		return
	}

	api.WriteDataResponse(w, http.StatusOK, updated)
}


// ExecuteMigration handles POST /api/v1/databases/{dbId}/migrations/{migrationId}/execute.
func (h *Handler) ExecuteMigration(w http.ResponseWriter, r *http.Request) {
	dbID := r.PathValue("dbId")
	migrationID := r.PathValue("migrationId")
	principal, _ := auth.PrincipalFromContext(r.Context())

	executed, err := h.Service.ExecuteMigration(r.Context(), principal, dbID, migrationID, time.Now())
	if err != nil {
		writeError(w, err)
		return
	}

	api.WriteDataResponse(w, http.StatusOK, executed)
}


func writeError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, apperror.ErrNotFound):
		api.WriteAPIError(w, http.StatusNotFound, "NOT_FOUND", "Resource not found")

	case errors.Is(err, apperror.ErrConflict):
		api.WriteAPIError(w, http.StatusConflict, "CONFLICT", "State conflict or already running")

	case errors.Is(err, apperror.ErrInvalidInput):
		api.WriteAPIError(w, http.StatusBadRequest, "BAD_REQUEST", err.Error())

	case errors.Is(err, apperror.ErrForbidden):
		api.WriteAPIError(w, http.StatusForbidden, "FORBIDDEN", "Insufficient database access")

	default:
		api.WriteAPIError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
	}
}