package migration

import (
	"net/http"

	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/api"
)

type Handler struct {
	Service *Service
}

// ListMigrations handles GET /api/v1/databases/{dbId}/migrations.
func (h *Handler) ListMigrations(w http.ResponseWriter, r *http.Request) {
	// TODO Read principal and dbId and the optional status filter.
	// TODO Call Service.ListMigrations and return api.MigrationSummary items in api.DataResponse with status 200.
	// TODO Map service errors to the existing API error envelope.
	api.WriteAPIError(w, http.StatusNotImplemented, "NOT_IMPLEMENTED", "ListMigrations is not implemented")
}

// GetMigration handles GET /api/v1/databases/{dbId}/migrations/{migrationId}.
func (h *Handler) GetMigration(w http.ResponseWriter, r *http.Request) {
	// TODO Read principal and both IDs then call Service.GetMigration.
	// TODO Return api.MigrationDetail in api.DataResponse with status 200.
	// TODO Map service errors to the existing API error envelope.
	api.WriteAPIError(w, http.StatusNotImplemented, "NOT_IMPLEMENTED", "GetMigration is not implemented")
}

// CreateMigration handles POST /api/v1/databases/{dbId}/migrations.
func (h *Handler) CreateMigration(w http.ResponseWriter, r *http.Request) {
	// TODO Read principal and dbId and decode api.CreateMigrationRequest with a body size limit.
	// TODO Call Service.CreateMigration and return api.MigrationCreated in api.DataResponse with status 201.
	// TODO Map service errors to the existing API error envelope.
	api.WriteAPIError(w, http.StatusNotImplemented, "NOT_IMPLEMENTED", "CreateMigration is not implemented")
}

// UpdateMigration handles PATCH /api/v1/databases/{dbId}/migrations/{migrationId}.
func (h *Handler) UpdateMigration(w http.ResponseWriter, r *http.Request) {
	// TODO Read principal and both IDs and decode api.UpdateMigrationRequest.
	// TODO Call Service.UpdateMigration and return api.MigrationUpdated with status 200.
	// TODO Use api.DataResponse and map a non draft state to status 409.
	// TODO Map service errors to the existing API error envelope.
	api.WriteAPIError(w, http.StatusNotImplemented, "NOT_IMPLEMENTED", "UpdateMigration is not implemented")
}

// ExecuteMigration handles POST /api/v1/databases/{dbId}/migrations/{migrationId}/execute.
func (h *Handler) ExecuteMigration(w http.ResponseWriter, r *http.Request) {
	// TODO Read principal and both IDs and execute stored SQL rather than SQL from the request.
	// TODO Call Service.ExecuteMigration and return api.MigrationExecuted with status 200.
	// TODO Use api.DataResponse and report uncertain execution without claiming rollback.
	// TODO Map service errors to the existing API error envelope.
	api.WriteAPIError(w, http.StatusNotImplemented, "NOT_IMPLEMENTED", "ExecuteMigration is not implemented")
}
