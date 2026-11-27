package migration

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/api"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/access"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/apperror"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/auth"
)

type Handler struct {
	Service *Service
}

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

func (h *Handler) CreateMigration(w http.ResponseWriter, r *http.Request) {
	dbID := r.PathValue("dbId")
	principal, _ := auth.PrincipalFromContext(r.Context())

	r.Body = http.MaxBytesReader(w, r.Body, 1024*1024)
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

func (h *Handler) SubmitReview(w http.ResponseWriter, r *http.Request) {
	dbID := r.PathValue("dbId")
	migrationID := r.PathValue("migrationId")
	principal, _ := auth.PrincipalFromContext(r.Context())

	detail, err := h.Service.SubmitReview(r.Context(), principal, dbID, migrationID, time.Now())
	if err != nil {
		writeError(w, err)
		return
	}

	api.WriteDataResponse(w, http.StatusOK, detail)
}

func (h *Handler) Approve(w http.ResponseWriter, r *http.Request) {
	dbID := r.PathValue("dbId")
	migrationID := r.PathValue("migrationId")
	principal, _ := auth.PrincipalFromContext(r.Context())

	var req api.DecideApprovalRequest
	_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 64*1024)).Decode(&req)

	detail, err := h.Service.ApproveMigration(r.Context(), principal, dbID, migrationID, req, time.Now())
	if err != nil {
		writeError(w, err)
		return
	}

	api.WriteDataResponse(w, http.StatusOK, detail)
}

func (h *Handler) Reject(w http.ResponseWriter, r *http.Request) {
	dbID := r.PathValue("dbId")
	migrationID := r.PathValue("migrationId")
	principal, _ := auth.PrincipalFromContext(r.Context())

	var req api.DecideApprovalRequest
	_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 64*1024)).Decode(&req)

	detail, err := h.Service.RejectMigration(r.Context(), principal, dbID, migrationID, req, time.Now())
	if err != nil {
		writeError(w, err)
		return
	}

	api.WriteDataResponse(w, http.StatusOK, detail)
}

func (h *Handler) Rollback(w http.ResponseWriter, r *http.Request) {
	dbID := r.PathValue("dbId")
	migrationID := r.PathValue("migrationId")
	principal, _ := auth.PrincipalFromContext(r.Context())

	executed, err := h.Service.RollbackMigration(r.Context(), principal, dbID, migrationID, time.Now())
	if err != nil {
		writeError(w, err)
		return
	}

	api.WriteDataResponse(w, http.StatusOK, executed)
}

func (h *Handler) Forward(w http.ResponseWriter, r *http.Request) {
	dbID := r.PathValue("dbId")
	migrationID := r.PathValue("migrationId")
	principal, _ := auth.PrincipalFromContext(r.Context())

	executed, err := h.Service.ForwardMigration(r.Context(), principal, dbID, migrationID, time.Now())
	if err != nil {
		writeError(w, err)
		return
	}

	api.WriteDataResponse(w, http.StatusOK, executed)
}

func (h *Handler) ListHistory(w http.ResponseWriter, r *http.Request) {
	dbID := r.PathValue("dbId")
	principal, _ := auth.PrincipalFromContext(r.Context())
	hideRollback := r.URL.Query().Get("hide_rollback") == "true" || r.URL.Query().Get("hide_rollback") == "1"

	history, err := h.Service.ListMigrationHistory(r.Context(), principal, dbID, hideRollback)
	if err != nil {
		writeError(w, err)
		return
	}

	api.WriteDataResponse(w, http.StatusOK, history)
}

func (h *Handler) ExplainMigration(w http.ResponseWriter, r *http.Request) {
	dbID, migrationID := r.PathValue("dbId"), r.PathValue("migrationId")
	principal, _ := auth.PrincipalFromContext(r.Context())
	var req struct {
		Analyze bool `json:"analyze"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024*1024)).Decode(&req); err != nil {
		api.WriteAPIError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid JSON body")
		return
	}
	result, err := h.Service.ExplainMigration(r.Context(), principal, dbID, migrationID, req.Analyze)
	if err != nil {
		writeError(w, err)
		return
	}
	api.WriteDataResponse(w, http.StatusOK, result)
}

func (h *Handler) CompareSchema(w http.ResponseWriter, r *http.Request) {
	principal, _ := auth.PrincipalFromContext(r.Context())
	r.Body = http.MaxBytesReader(w, r.Body, 1024*1024)
	var req api.SchemaDiffRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.WriteAPIError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid schema DDL request")
		return
	}
	result, err := h.Service.CompareSchema(r.Context(), principal, r.PathValue("dbId"), req)
	if err != nil {
		writeError(w, err)
		return
	}
	api.WriteDataResponse(w, http.StatusOK, result)
}
func writeError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, apperror.ErrTransactionOutcomeUnknown):
		api.WriteAPIError(w, http.StatusConflict, "MIGRATION_OUTCOME_UNKNOWN", "Execution outcome is unknown. Inspect the target before retrying; this migration remains RUNNING.")
	case errors.Is(err, apperror.ErrNotFound), errors.Is(err, access.ErrNotFound):
		api.WriteAPIError(w, http.StatusNotFound, "NOT_FOUND", "Resource not found")

	case errors.Is(err, apperror.ErrConflict):
		api.WriteAPIError(w, http.StatusConflict, "CONFLICT", err.Error())

	case errors.Is(err, apperror.ErrInvalidInput):
		api.WriteAPIError(w, http.StatusBadRequest, "BAD_REQUEST", err.Error())

	case errors.Is(err, apperror.ErrForbidden):
		api.WriteAPIError(w, http.StatusForbidden, "FORBIDDEN", err.Error())

	case errors.Is(err, access.ErrForbidden):
		api.WriteAPIError(w, http.StatusForbidden, "FORBIDDEN", "Insufficient database access")

	case errors.Is(err, access.ErrUnauthenticated), errors.Is(err, auth.ErrUnauthenticated):
		api.WriteAPIError(w, http.StatusUnauthorized, "UNAUTHENTICATED", "Missing or invalid token")

	default:
		api.WriteAPIError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
	}
}
