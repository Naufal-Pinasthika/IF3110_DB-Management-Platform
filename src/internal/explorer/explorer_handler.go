package explorer

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/api"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/access"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/apperror"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/auth"
)

type Handler struct {
	Service *Service
}

func writeError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, apperror.ErrNotFound), errors.Is(err, access.ErrNotFound):
		api.WriteAPIError(w, http.StatusNotFound, "NOT_FOUND", err.Error())
	case errors.Is(err, apperror.ErrForbidden), errors.Is(err, access.ErrForbidden):
		api.WriteAPIError(w, http.StatusForbidden, "FORBIDDEN", err.Error())
	case errors.Is(err, apperror.ErrInvalidInput):
		api.WriteAPIError(w, http.StatusBadRequest, "BAD_REQUEST", err.Error())
	case errors.Is(err, apperror.ErrConflict):
		api.WriteAPIError(w, http.StatusConflict, "CONFLICT", err.Error())
	case errors.Is(err, access.ErrUnauthenticated), errors.Is(err, auth.ErrUnauthenticated):
		api.WriteAPIError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Missing or invalid token")
	default:
		api.WriteAPIError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
	}
}

func (h *Handler) ListDatabases(w http.ResponseWriter, r *http.Request) {
	principal, ok := auth.PrincipalFromContext(r.Context())
	if !ok {
		api.WriteAPIError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Missing or invalid token")
		return
	}

	dbs, err := h.Service.ListDatabases(r.Context(), principal)
	if err != nil {
		writeError(w, err)
		return
	}

	api.WriteDataResponse(w, http.StatusOK, dbs)
}

func (h *Handler) GetDatabase(w http.ResponseWriter, r *http.Request) {
	principal, ok := auth.PrincipalFromContext(r.Context())
	if !ok {
		api.WriteAPIError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Missing or invalid token")
		return
	}
	dbID := r.PathValue("dbId")

	db, err := h.Service.GetDatabase(r.Context(), principal, dbID)
	if err != nil {
		writeError(w, err)
		return
	}

	api.WriteDataResponse(w, http.StatusOK, db)
}

func (h *Handler) ListSchemas(w http.ResponseWriter, r *http.Request) {
	principal, ok := auth.PrincipalFromContext(r.Context())
	if !ok {
		api.WriteAPIError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Missing or invalid token")
		return
	}
	dbID := r.PathValue("dbId")

	schemas, err := h.Service.ListSchemas(r.Context(), principal, dbID)
	if err != nil {
		writeError(w, err)
		return
	}

	api.WriteDataResponse(w, http.StatusOK, schemas)
}

func (h *Handler) ListTables(w http.ResponseWriter, r *http.Request) {
	principal, ok := auth.PrincipalFromContext(r.Context())
	if !ok {
		api.WriteAPIError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Missing or invalid token")
		return
	}
	dbID := r.PathValue("dbId")
	schema := r.PathValue("schema")

	tables, err := h.Service.ListTables(r.Context(), principal, dbID, schema)
	if err != nil {
		writeError(w, err)
		return
	}

	api.WriteDataResponse(w, http.StatusOK, tables)
}

func (h *Handler) GetTableDetail(w http.ResponseWriter, r *http.Request) {
	principal, ok := auth.PrincipalFromContext(r.Context())
	if !ok {
		api.WriteAPIError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Missing or invalid token")
		return
	}
	dbID := r.PathValue("dbId")
	schema := r.PathValue("schema")
	table := r.PathValue("table")

	detail, err := h.Service.GetTableDetail(r.Context(), principal, dbID, schema, table)
	if err != nil {
		writeError(w, err)
		return
	}

	api.WriteDataResponse(w, http.StatusOK, detail)
}

func (h *Handler) GetTableRows(w http.ResponseWriter, r *http.Request) {
	principal, ok := auth.PrincipalFromContext(r.Context())
	if !ok {
		api.WriteAPIError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Missing or invalid token")
		return
	}

	dbID := r.PathValue("dbId")
	schema := r.PathValue("schema")
	table := r.PathValue("table")

	limitStr := r.URL.Query().Get("limit")
	offsetStr := r.URL.Query().Get("offset")

	limit := 50
	var err error
	if limitStr != "" {
		if limit, err = strconv.Atoi(limitStr); err != nil {
			api.WriteAPIError(w, http.StatusBadRequest, "BAD_REQUEST", "Malformed limit parameter")
			return
		}
	}

	offset := 0
	if offsetStr != "" {
		if offset, err = strconv.Atoi(offsetStr); err != nil {
			api.WriteAPIError(w, http.StatusBadRequest, "BAD_REQUEST", "Malformed offset parameter")
			return
		}
	}

	rows, err := h.Service.GetTableRows(r.Context(), principal, dbID, schema, table, limit, offset)
	if err != nil {
		writeError(w, err)
		return
	}

	api.WriteDataResponse(w, http.StatusOK, rows)
}

func (h *Handler) RunQuery(w http.ResponseWriter, r *http.Request) {
	principal, ok := auth.PrincipalFromContext(r.Context())
	if !ok {
		api.WriteAPIError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Missing or invalid token")
		return
	}
	dbID := r.PathValue("dbId")

	r.Body = http.MaxBytesReader(w, r.Body, 1024*1024)
	var req api.RunQueryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.WriteAPIError(w, http.StatusBadRequest, "BAD_REQUEST", "Invalid request body")
		return
	}

	result, err := h.Service.RunQuery(r.Context(), principal, dbID, req)
	if err != nil {
		writeError(w, err)
		return
	}

	api.WriteDataResponse(w, http.StatusOK, result)
}

func (h *Handler) ExplainQuery(w http.ResponseWriter, r *http.Request) {
	principal, ok := auth.PrincipalFromContext(r.Context())
	if !ok { api.WriteAPIError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Missing or invalid token"); return }
	r.Body = http.MaxBytesReader(w, r.Body, 1024*1024)
	var req api.ExplainQueryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil { api.WriteAPIError(w, http.StatusBadRequest, "BAD_REQUEST", "Invalid request body"); return }
	result, err := h.Service.ExplainQuery(r.Context(), principal, r.PathValue("dbId"), req)
	if err != nil { writeError(w, err); return }
	api.WriteDataResponse(w, http.StatusOK, result)
}
