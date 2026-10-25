package admin

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/api"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/apperror"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/auth"
)

type Handler struct {
	Service *Service
}

// ListUsers handles GET /api/v1/admin/users.
func (h *Handler) ListUsers(w http.ResponseWriter, r *http.Request) {
	principal, ok := auth.PrincipalFromContext(r.Context())
	if !ok {
		api.WriteAPIError(w, http.StatusUnauthorized, "UNAUTHENTICATED", "Please log in again")
		return
	}

	users, err := h.Service.ListUsers(r.Context(), principal)
	if err != nil {
		h.mapError(w, err)
		return
	}

	api.WriteJSON(w, api.DataResponse[[]api.AdminUser]{Data: users})
}

// CreateUser handles POST /api/v1/admin/users.
func (h *Handler) CreateUser(w http.ResponseWriter, r *http.Request) {
	principal, ok := auth.PrincipalFromContext(r.Context())
	if !ok {
		api.WriteAPIError(w, http.StatusUnauthorized, "UNAUTHENTICATED", "Please log in again")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 8<<10)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	var input api.CreateUserRequest
	if err := decoder.Decode(&input); err != nil {
		api.WriteAPIError(w, http.StatusBadRequest, "INVALID_BODY", "Invalid request body")
		return
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		api.WriteAPIError(w, http.StatusBadRequest, "INVALID_BODY", "Expected one JSON object")
		return
	}

	user, err := h.Service.CreateUser(r.Context(), principal, input)
	if err != nil {
		h.mapError(w, err)
		return
	}

	w.WriteHeader(http.StatusCreated)
	api.WriteJSON(w, api.DataResponse[api.AdminUser]{Data: user})
}

// UpdateUser handles PATCH /api/v1/admin/users/{userId}.
func (h *Handler) UpdateUser(w http.ResponseWriter, r *http.Request) {
	principal, ok := auth.PrincipalFromContext(r.Context())
	if !ok {
		api.WriteAPIError(w, http.StatusUnauthorized, "UNAUTHENTICATED", "Please log in again")
		return
	}

	userID := r.PathValue("userId")
	if userID == "" {
		api.WriteAPIError(w, http.StatusBadRequest, "INVALID_ID", "user id required")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 8<<10)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	var input api.UpdateUserRequest
	if err := decoder.Decode(&input); err != nil {
		api.WriteAPIError(w, http.StatusBadRequest, "INVALID_BODY", "Invalid request body")
		return
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		api.WriteAPIError(w, http.StatusBadRequest, "INVALID_BODY", "Expected one JSON object")
		return
	}

	user, err := h.Service.UpdateUser(r.Context(), principal, userID, input, time.Now().UTC())
	if err != nil {
		h.mapError(w, err)
		return
	}

	api.WriteJSON(w, api.DataResponse[api.AdminUser]{Data: user})
}

// ListGroups handles GET /api/v1/admin/groups.
func (h *Handler) ListGroups(w http.ResponseWriter, r *http.Request) {
	principal, ok := auth.PrincipalFromContext(r.Context())
	if !ok {
		api.WriteAPIError(w, http.StatusUnauthorized, "UNAUTHENTICATED", "Please log in again")
		return
	}

	groups, err := h.Service.ListGroups(r.Context(), principal)
	if err != nil {
		h.mapError(w, err)
		return
	}

	api.WriteJSON(w, api.DataResponse[[]api.Group]{Data: groups})
}

// CreateGroup handles POST /api/v1/admin/groups.
func (h *Handler) CreateGroup(w http.ResponseWriter, r *http.Request) {
	principal, ok := auth.PrincipalFromContext(r.Context())
	if !ok {
		api.WriteAPIError(w, http.StatusUnauthorized, "UNAUTHENTICATED", "Please log in again")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 8<<10)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	var input api.CreateGroupRequest
	if err := decoder.Decode(&input); err != nil {
		api.WriteAPIError(w, http.StatusBadRequest, "INVALID_BODY", "Invalid request body")
		return
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		api.WriteAPIError(w, http.StatusBadRequest, "INVALID_BODY", "Expected one JSON object")
		return
	}

	group, err := h.Service.CreateGroup(r.Context(), principal, input)
	if err != nil {
		h.mapError(w, err)
		return
	}

	w.WriteHeader(http.StatusCreated)
	api.WriteJSON(w, api.DataResponse[api.Group]{Data: group})
}

// UpdateGroup handles PATCH /api/v1/admin/groups/{groupId}.
func (h *Handler) UpdateGroup(w http.ResponseWriter, r *http.Request) {
	principal, ok := auth.PrincipalFromContext(r.Context())
	if !ok {
		api.WriteAPIError(w, http.StatusUnauthorized, "UNAUTHENTICATED", "Please log in again")
		return
	}

	groupID := r.PathValue("groupId")
	if groupID == "" {
		api.WriteAPIError(w, http.StatusBadRequest, "INVALID_ID", "group id required")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 8<<10)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	var input api.UpdateGroupRequest
	if err := decoder.Decode(&input); err != nil {
		api.WriteAPIError(w, http.StatusBadRequest, "INVALID_BODY", "Invalid request body")
		return
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		api.WriteAPIError(w, http.StatusBadRequest, "INVALID_BODY", "Expected one JSON object")
		return
	}

	group, err := h.Service.UpdateGroup(r.Context(), principal, groupID, input)
	if err != nil {
		h.mapError(w, err)
		return
	}

	api.WriteJSON(w, api.DataResponse[api.Group]{Data: group})
}

// DeleteGroup handles DELETE /api/v1/admin/groups/{groupId}.
func (h *Handler) DeleteGroup(w http.ResponseWriter, r *http.Request) {
	principal, ok := auth.PrincipalFromContext(r.Context())
	if !ok {
		api.WriteAPIError(w, http.StatusUnauthorized, "UNAUTHENTICATED", "Please log in again")
		return
	}

	groupID := r.PathValue("groupId")
	if groupID == "" {
		api.WriteAPIError(w, http.StatusBadRequest, "INVALID_ID", "group id required")
		return
	}

	err := h.Service.DeleteGroup(r.Context(), principal, groupID, time.Now().UTC())
	if err != nil {
		h.mapError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
	}

	// mapError maps service errors to API error responses.
	func (h *Handler) mapError(w http.ResponseWriter, err error) {
		switch {
		case errors.Is(err, auth.ErrUnauthenticated):
			api.WriteAPIError(w, http.StatusUnauthorized, "UNAUTHENTICATED", "Please log in again")
		case errors.Is(err, apperror.ErrForbidden):
			api.WriteAPIError(w, http.StatusForbidden, "FORBIDDEN", "Platform admin access required")
		case errors.Is(err, apperror.ErrNotFound):
			api.WriteAPIError(w, http.StatusNotFound, "NOT_FOUND", err.Error())
		case errors.Is(err, apperror.ErrConflict):
			api.WriteAPIError(w, http.StatusConflict, "CONFLICT", err.Error())
		case errors.Is(err, apperror.ErrInvalidInput):
			api.WriteAPIError(w, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		default:
			api.WriteAPIError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "An internal error occurred")
		}
	}

	// AddGroupMember handles PUT /api/v1/admin/groups/{groupId}/members/{userId}.
	func (h *Handler) AddGroupMember(w http.ResponseWriter, r *http.Request) {
		principal, ok := auth.PrincipalFromContext(r.Context())
		if !ok {
			api.WriteAPIError(w, http.StatusUnauthorized, "UNAUTHENTICATED", "Please log in again")
			return
		}

		groupID := r.PathValue("groupId")
		userID := r.PathValue("userId")
		if groupID == "" || userID == "" {
			api.WriteAPIError(w, http.StatusBadRequest, "INVALID_ID", "group id and user id required")
			return
		}

		member, err := h.Service.AddGroupMember(r.Context(), principal, groupID, userID, time.Now().UTC())
			if err != nil {
				h.mapError(w, err)
				return
			}

			api.WriteJSON(w, api.DataResponse[api.GroupMember]{Data: member})
	}

	// RemoveGroupMember handles DELETE /api/v1/admin/groups/{groupId}/members/{userId}.
	func (h *Handler) RemoveGroupMember(w http.ResponseWriter, r *http.Request) {
		principal, ok := auth.PrincipalFromContext(r.Context())
		if !ok {
			api.WriteAPIError(w, http.StatusUnauthorized, "UNAUTHENTICATED", "Please log in again")
			return
		}

		groupID := r.PathValue("groupId")
		userID := r.PathValue("userId")
		if groupID == "" || userID == "" {
			api.WriteAPIError(w, http.StatusBadRequest, "INVALID_ID", "group id and user id required")
			return
		}

		err := h.Service.RemoveGroupMember(r.Context(), principal, groupID, userID, time.Now().UTC())
		if err != nil {
			h.mapError(w, err)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}

	// ListManagedDatabases handles GET /api/v1/admin/databases.
	func (h *Handler) ListManagedDatabases(w http.ResponseWriter, r *http.Request) {
		principal, ok := auth.PrincipalFromContext(r.Context())
		if !ok {
			api.WriteAPIError(w, http.StatusUnauthorized, "UNAUTHENTICATED", "Please log in again")
			return
		}

		dbs, err := h.Service.ListManagedDatabases(r.Context(), principal)
		if err != nil {
			h.mapError(w, err)
			return
		}

		api.WriteJSON(w, api.DataResponse[[]api.ManagedDatabaseSummary]{Data: dbs})
	}

	// CreateManagedDatabase handles POST /api/v1/admin/databases.
	func (h *Handler) CreateManagedDatabase(w http.ResponseWriter, r *http.Request) {
		principal, ok := auth.PrincipalFromContext(r.Context())
		if !ok {
			api.WriteAPIError(w, http.StatusUnauthorized, "UNAUTHENTICATED", "Please log in again")
			return
		}

		r.Body = http.MaxBytesReader(w, r.Body, 8<<10)
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()

		var input api.CreateDatabaseRequest
		if err := decoder.Decode(&input); err != nil {
			api.WriteAPIError(w, http.StatusBadRequest, "INVALID_BODY", "Invalid request body")
			return
		}
		if err := decoder.Decode(&struct{}{}); err != io.EOF {
			api.WriteAPIError(w, http.StatusBadRequest, "INVALID_BODY", "Expected one JSON object")
			return
		}

		db, err := h.Service.CreateManagedDatabase(r.Context(), principal, input)
		if err != nil {
			h.mapError(w, err)
			return
		}

		w.WriteHeader(http.StatusCreated)
		api.WriteJSON(w, api.DataResponse[api.ManagedDatabaseSummary]{Data: db})
	}

	// GetManagedDatabase handles GET /api/v1/admin/databases/{dbId}.
	func (h *Handler) GetManagedDatabase(w http.ResponseWriter, r *http.Request) {
		principal, ok := auth.PrincipalFromContext(r.Context())
		if !ok {
			api.WriteAPIError(w, http.StatusUnauthorized, "UNAUTHENTICATED", "Please log in again")
			return
		}

		dbID := r.PathValue("dbId")
		if dbID == "" {
			api.WriteAPIError(w, http.StatusBadRequest, "INVALID_ID", "database id required")
			return
		}

		db, err := h.Service.GetManagedDatabase(r.Context(), principal, dbID)
		if err != nil {
			h.mapError(w, err)
			return
		}

		api.WriteJSON(w, api.DataResponse[api.ManagedDatabaseDetail]{Data: db})
	}

	// UpdateManagedDatabase handles PATCH /api/v1/admin/databases/{dbId}.
	func (h *Handler) UpdateManagedDatabase(w http.ResponseWriter, r *http.Request) {
		principal, ok := auth.PrincipalFromContext(r.Context())
		if !ok {
			api.WriteAPIError(w, http.StatusUnauthorized, "UNAUTHENTICATED", "Please log in again")
			return
		}

		dbID := r.PathValue("dbId")
		if dbID == "" {
			api.WriteAPIError(w, http.StatusBadRequest, "INVALID_ID", "database id required")
			return
		}

		r.Body = http.MaxBytesReader(w, r.Body, 8<<10)
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()

		var input api.UpdateDatabaseRequest
		if err := decoder.Decode(&input); err != nil {
			api.WriteAPIError(w, http.StatusBadRequest, "INVALID_BODY", "Invalid request body")
			return
		}
		if err := decoder.Decode(&struct{}{}); err != io.EOF {
			api.WriteAPIError(w, http.StatusBadRequest, "INVALID_BODY", "Expected one JSON object")
			return
		}

		db, err := h.Service.UpdateManagedDatabase(r.Context(), principal, dbID, input)
		if err != nil {
			h.mapError(w, err)
			return
		}

		api.WriteJSON(w, api.DataResponse[api.ManagedDatabaseSummary]{Data: db})
	}

	// DisableManagedDatabase handles DELETE /api/v1/admin/databases/{dbId}.
	func (h *Handler) DisableManagedDatabase(w http.ResponseWriter, r *http.Request) {
		principal, ok := auth.PrincipalFromContext(r.Context())
		if !ok {
			api.WriteAPIError(w, http.StatusUnauthorized, "UNAUTHENTICATED", "Please log in again")
			return
		}

		dbID := r.PathValue("dbId")
		if dbID == "" {
			api.WriteAPIError(w, http.StatusBadRequest, "INVALID_ID", "database id required")
			return
		}

		err := h.Service.DisableManagedDatabase(r.Context(), principal, dbID)
		if err != nil {
			h.mapError(w, err)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}

	// TestDatabaseConnection handles POST /api/v1/admin/databases/{dbId}/test-connection.
	func (h *Handler) TestDatabaseConnection(w http.ResponseWriter, r *http.Request) {
		principal, ok := auth.PrincipalFromContext(r.Context())
		if !ok {
			api.WriteAPIError(w, http.StatusUnauthorized, "UNAUTHENTICATED", "Please log in again")
			return
		}

		dbID := r.PathValue("dbId")
		if dbID == "" {
			api.WriteAPIError(w, http.StatusBadRequest, "INVALID_ID", "database id required")
			return
		}

		result, err := h.Service.TestDatabaseConnection(r.Context(), principal, dbID)
		if err != nil {
			h.mapError(w, err)
			return
		}

		api.WriteJSON(w, api.DataResponse[api.TestConnectionResult]{Data: result})
	}

	// SetGroupDatabaseAccess handles PUT /api/v1/admin/groups/{groupId}/databases/{dbId}/access.
	func (h *Handler) SetGroupDatabaseAccess(w http.ResponseWriter, r *http.Request) {
		principal, ok := auth.PrincipalFromContext(r.Context())
		if !ok {
			api.WriteAPIError(w, http.StatusUnauthorized, "UNAUTHENTICATED", "Please log in again")
			return
		}

		groupID := r.PathValue("groupId")
		dbID := r.PathValue("dbId")
		if groupID == "" || dbID == "" {
			api.WriteAPIError(w, http.StatusBadRequest, "INVALID_ID", "group id and database id required")
			return
		}

		r.Body = http.MaxBytesReader(w, r.Body, 8<<10)
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()

		var input api.SetGroupAccessRequest
		if err := decoder.Decode(&input); err != nil {
			api.WriteAPIError(w, http.StatusBadRequest, "INVALID_BODY", "Invalid request body")
			return
		}
		if err := decoder.Decode(&struct{}{}); err != io.EOF {
			api.WriteAPIError(w, http.StatusBadRequest, "INVALID_BODY", "Expected one JSON object")
			return
		}

		access, err := h.Service.SetGroupDatabaseAccess(r.Context(), principal, groupID, dbID, input, time.Now().UTC())
		if err != nil {
			h.mapError(w, err)
			return
		}

		api.WriteJSON(w, api.DataResponse[api.GroupDatabaseAccess]{Data: access})
	}

	// DeleteGroupDatabaseAccess handles DELETE /api/v1/admin/groups/{groupId}/databases/{dbId}/access.
	func (h *Handler) DeleteGroupDatabaseAccess(w http.ResponseWriter, r *http.Request) {
		principal, ok := auth.PrincipalFromContext(r.Context())
		if !ok {
			api.WriteAPIError(w, http.StatusUnauthorized, "UNAUTHENTICATED", "Please log in again")
			return
		}

		groupID := r.PathValue("groupId")
		dbID := r.PathValue("dbId")
		if groupID == "" || dbID == "" {
			api.WriteAPIError(w, http.StatusBadRequest, "INVALID_ID", "group id and database id required")
			return
		}

		err := h.Service.DeleteGroupDatabaseAccess(r.Context(), principal, groupID, dbID, time.Now().UTC())
		if err != nil {
			h.mapError(w, err)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
