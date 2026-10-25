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


// ListDatabases handles GET /api/v1/databases.
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


// GetDatabase handles GET /api/v1/databases/{dbId}.
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


// ListSchemas handles GET /api/v1/databases/{dbId}/schemas.
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


// ListTables handles GET /api/v1/databases/{dbId}/schemas/{schema}/tables.
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


// GetTableDetail handles GET /api/v1/databases/{dbId}/schemas/{schema}/tables/{table}.
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


// GetTableRows handles GET /api/v1/databases/{dbId}/schemas/{schema}/tables/{table}/rows.
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

	limit := 50 // default limit
	var err error
	if limitStr != "" {
		if limit, err = strconv.Atoi(limitStr); err != nil {
			api.WriteAPIError(w, http.StatusBadRequest, "BAD_REQUEST", "Malformed limit parameter")
			return
		}
	}

	offset := 0 // default offset
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


// RunQuery handles POST /api/v1/databases/{dbId}/queries.
func (h *Handler) RunQuery(w http.ResponseWriter, r *http.Request) {
	principal, ok := auth.PrincipalFromContext(r.Context())
	if !ok {
		api.WriteAPIError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Missing or invalid token")
		return
	}
	dbID := r.PathValue("dbId")

	r.Body = http.MaxBytesReader(w, r.Body, 1024*1024) // 1MB body limit for queries
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