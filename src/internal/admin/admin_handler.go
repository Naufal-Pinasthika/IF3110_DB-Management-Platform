package admin

import (
	"net/http"

	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/api"
)

type Handler struct {
	Service *Service
}

// ListUsers handles GET /api/v1/admin/users.
func (h *Handler) ListUsers(w http.ResponseWriter, r *http.Request) {
	// TODO Read the validated principal from context and required path IDs with PathValue.
	// TODO Call Service.ListUsers and keep authorization in the service.
	// TODO Return the existing API response type in api.DataResponse with status 200.
	// TODO Map service errors to the existing API error envelope.
	api.WriteAPIError(w, http.StatusNotImplemented, "NOT_IMPLEMENTED", "ListUsers is not implemented")
}

// CreateUser handles POST /api/v1/admin/users.
func (h *Handler) CreateUser(w http.ResponseWriter, r *http.Request) {
	// TODO Read the validated principal from context and required path IDs with PathValue.
	// TODO Decode api.CreateUserRequest with a body size limit and preserve optional fields.
	// TODO Call Service.CreateUser and keep authorization in the service.
	// TODO Return the existing API response type in api.DataResponse with status 201.
	// TODO Map service errors to the existing API error envelope.
	api.WriteAPIError(w, http.StatusNotImplemented, "NOT_IMPLEMENTED", "CreateUser is not implemented")
}

// UpdateUser handles PATCH /api/v1/admin/users/{userId}.
func (h *Handler) UpdateUser(w http.ResponseWriter, r *http.Request) {
	// TODO Read the validated principal from context and required path IDs with PathValue.
	// TODO Decode api.UpdateUserRequest with a body size limit and preserve optional fields.
	// TODO Call Service.UpdateUser and keep authorization in the service.
	// TODO Return the existing API response type in api.DataResponse with status 200.
	// TODO Map service errors to the existing API error envelope.
	api.WriteAPIError(w, http.StatusNotImplemented, "NOT_IMPLEMENTED", "UpdateUser is not implemented")
}

// ListGroups handles GET /api/v1/admin/groups.
func (h *Handler) ListGroups(w http.ResponseWriter, r *http.Request) {
	// TODO Read the validated principal from context and required path IDs with PathValue.
	// TODO Call Service.ListGroups and keep authorization in the service.
	// TODO Return the existing API response type in api.DataResponse with status 200.
	// TODO Map service errors to the existing API error envelope.
	api.WriteAPIError(w, http.StatusNotImplemented, "NOT_IMPLEMENTED", "ListGroups is not implemented")
}

// CreateGroup handles POST /api/v1/admin/groups.
func (h *Handler) CreateGroup(w http.ResponseWriter, r *http.Request) {
	// TODO Read the validated principal from context and required path IDs with PathValue.
	// TODO Decode api.CreateGroupRequest with a body size limit and preserve optional fields.
	// TODO Call Service.CreateGroup and keep authorization in the service.
	// TODO Return the existing API response type in api.DataResponse with status 201.
	// TODO Map service errors to the existing API error envelope.
	api.WriteAPIError(w, http.StatusNotImplemented, "NOT_IMPLEMENTED", "CreateGroup is not implemented")
}

// UpdateGroup handles PATCH /api/v1/admin/groups/{groupId}.
func (h *Handler) UpdateGroup(w http.ResponseWriter, r *http.Request) {
	// TODO Read the validated principal from context and required path IDs with PathValue.
	// TODO Decode api.UpdateGroupRequest with a body size limit and preserve optional fields.
	// TODO Call Service.UpdateGroup and keep authorization in the service.
	// TODO Return the existing API response type in api.DataResponse with status 200.
	// TODO Map service errors to the existing API error envelope.
	api.WriteAPIError(w, http.StatusNotImplemented, "NOT_IMPLEMENTED", "UpdateGroup is not implemented")
}

// DeleteGroup handles DELETE /api/v1/admin/groups/{groupId}.
func (h *Handler) DeleteGroup(w http.ResponseWriter, r *http.Request) {
	// TODO Read the validated principal from context and required path IDs with PathValue.
	// TODO Call Service.DeleteGroup and keep authorization in the service.
	// TODO Return status 204 with no response body.
	// TODO Map service errors to the existing API error envelope.
	api.WriteAPIError(w, http.StatusNotImplemented, "NOT_IMPLEMENTED", "DeleteGroup is not implemented")
}

// AddGroupMember handles PUT /api/v1/admin/groups/{groupId}/members/{userId}.
func (h *Handler) AddGroupMember(w http.ResponseWriter, r *http.Request) {
	// TODO Read the validated principal from context and required path IDs with PathValue.
	// TODO Call Service.AddGroupMember and keep authorization in the service.
	// TODO Return the existing API response type in api.DataResponse with status 200.
	// TODO Map service errors to the existing API error envelope.
	api.WriteAPIError(w, http.StatusNotImplemented, "NOT_IMPLEMENTED", "AddGroupMember is not implemented")
}

// RemoveGroupMember handles DELETE /api/v1/admin/groups/{groupId}/members/{userId}.
func (h *Handler) RemoveGroupMember(w http.ResponseWriter, r *http.Request) {
	// TODO Read the validated principal from context and required path IDs with PathValue.
	// TODO Call Service.RemoveGroupMember and keep authorization in the service.
	// TODO Return status 204 with no response body.
	// TODO Map service errors to the existing API error envelope.
	api.WriteAPIError(w, http.StatusNotImplemented, "NOT_IMPLEMENTED", "RemoveGroupMember is not implemented")
}

// ListManagedDatabases handles GET /api/v1/admin/databases.
func (h *Handler) ListManagedDatabases(w http.ResponseWriter, r *http.Request) {
	// TODO Read the validated principal from context and required path IDs with PathValue.
	// TODO Call Service.ListManagedDatabases and keep authorization in the service.
	// TODO Return the existing API response type in api.DataResponse with status 200.
	// TODO Map service errors to the existing API error envelope.
	api.WriteAPIError(w, http.StatusNotImplemented, "NOT_IMPLEMENTED", "ListManagedDatabases is not implemented")
}

// CreateManagedDatabase handles POST /api/v1/admin/databases.
func (h *Handler) CreateManagedDatabase(w http.ResponseWriter, r *http.Request) {
	// TODO Read the validated principal from context and required path IDs with PathValue.
	// TODO Decode api.CreateDatabaseRequest with a body size limit and preserve optional fields.
	// TODO Call Service.CreateManagedDatabase and keep authorization in the service.
	// TODO Return the existing API response type in api.DataResponse with status 201.
	// TODO Map service errors to the existing API error envelope.
	api.WriteAPIError(w, http.StatusNotImplemented, "NOT_IMPLEMENTED", "CreateManagedDatabase is not implemented")
}

// GetManagedDatabase handles GET /api/v1/admin/databases/{dbId}.
func (h *Handler) GetManagedDatabase(w http.ResponseWriter, r *http.Request) {
	// TODO Read the validated principal from context and required path IDs with PathValue.
	// TODO Call Service.GetManagedDatabase and keep authorization in the service.
	// TODO Return the existing API response type in api.DataResponse with status 200.
	// TODO Map service errors to the existing API error envelope.
	api.WriteAPIError(w, http.StatusNotImplemented, "NOT_IMPLEMENTED", "GetManagedDatabase is not implemented")
}

// UpdateManagedDatabase handles PATCH /api/v1/admin/databases/{dbId}.
func (h *Handler) UpdateManagedDatabase(w http.ResponseWriter, r *http.Request) {
	// TODO Read the validated principal from context and required path IDs with PathValue.
	// TODO Decode api.UpdateDatabaseRequest with a body size limit and preserve optional fields.
	// TODO Call Service.UpdateManagedDatabase and keep authorization in the service.
	// TODO Return the existing API response type in api.DataResponse with status 200.
	// TODO Map service errors to the existing API error envelope.
	api.WriteAPIError(w, http.StatusNotImplemented, "NOT_IMPLEMENTED", "UpdateManagedDatabase is not implemented")
}

// DisableManagedDatabase handles DELETE /api/v1/admin/databases/{dbId}.
func (h *Handler) DisableManagedDatabase(w http.ResponseWriter, r *http.Request) {
	// TODO Read the validated principal from context and required path IDs with PathValue.
	// TODO Call Service.DisableManagedDatabase and keep authorization in the service.
	// TODO Return status 204 with no response body.
	// TODO Map service errors to the existing API error envelope.
	api.WriteAPIError(w, http.StatusNotImplemented, "NOT_IMPLEMENTED", "DisableManagedDatabase is not implemented")
}

// TestDatabaseConnection handles POST /api/v1/admin/databases/{dbId}/test-connection.
func (h *Handler) TestDatabaseConnection(w http.ResponseWriter, r *http.Request) {
	// TODO Read the validated principal from context and required path IDs with PathValue.
	// TODO Call Service.TestDatabaseConnection and keep authorization in the service.
	// TODO Return the existing API response type in api.DataResponse with status 200.
	// TODO Map service errors to the existing API error envelope.
	api.WriteAPIError(w, http.StatusNotImplemented, "NOT_IMPLEMENTED", "TestDatabaseConnection is not implemented")
}

// SetGroupDatabaseAccess handles PUT /api/v1/admin/groups/{groupId}/databases/{dbId}/access.
func (h *Handler) SetGroupDatabaseAccess(w http.ResponseWriter, r *http.Request) {
	// TODO Read the validated principal from context and required path IDs with PathValue.
	// TODO Decode api.SetGroupAccessRequest with a body size limit and preserve optional fields.
	// TODO Call Service.SetGroupDatabaseAccess and keep authorization in the service.
	// TODO Return the existing API response type in api.DataResponse with status 200.
	// TODO Map service errors to the existing API error envelope.
	api.WriteAPIError(w, http.StatusNotImplemented, "NOT_IMPLEMENTED", "SetGroupDatabaseAccess is not implemented")
}

// DeleteGroupDatabaseAccess handles DELETE /api/v1/admin/groups/{groupId}/databases/{dbId}/access.
func (h *Handler) DeleteGroupDatabaseAccess(w http.ResponseWriter, r *http.Request) {
	// TODO Read the validated principal from context and required path IDs with PathValue.
	// TODO Call Service.DeleteGroupDatabaseAccess and keep authorization in the service.
	// TODO Return status 204 with no response body.
	// TODO Map service errors to the existing API error envelope.
	api.WriteAPIError(w, http.StatusNotImplemented, "NOT_IMPLEMENTED", "DeleteGroupDatabaseAccess is not implemented")
}
