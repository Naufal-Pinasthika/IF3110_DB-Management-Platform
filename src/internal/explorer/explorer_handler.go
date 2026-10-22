package explorer

import (
	"net/http"

	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/api"
)

type Handler struct {
	Service *Service
}

// ListDatabases handles GET /api/v1/databases.
func (h *Handler) ListDatabases(w http.ResponseWriter, r *http.Request) {
	// TODO Read the validated principal and call Service.ListDatabases.
	// TODO Return api.UserDatabase items in api.DataResponse with status 200.
	// TODO Map service errors to the existing API error envelope.
	api.WriteAPIError(w, http.StatusNotImplemented, "NOT_IMPLEMENTED", "ListDatabases is not implemented")
}

// GetDatabase handles GET /api/v1/databases/{dbId}.
func (h *Handler) GetDatabase(w http.ResponseWriter, r *http.Request) {
	// TODO Read the validated principal and dbId then call Service.GetDatabase.
	// TODO Return api.UserDatabase in api.DataResponse with status 200.
	// TODO Map service errors to the existing API error envelope.
	api.WriteAPIError(w, http.StatusNotImplemented, "NOT_IMPLEMENTED", "GetDatabase is not implemented")
}

// ListSchemas handles GET /api/v1/databases/{dbId}/schemas.
func (h *Handler) ListSchemas(w http.ResponseWriter, r *http.Request) {
	// TODO Read principal and dbId then call Service.ListSchemas.
	// TODO Return api.SchemaSummary items in api.DataResponse with status 200.
	// TODO Map service errors to the existing API error envelope.
	api.WriteAPIError(w, http.StatusNotImplemented, "NOT_IMPLEMENTED", "ListSchemas is not implemented")
}

// ListTables handles GET /api/v1/databases/{dbId}/schemas/{schema}/tables.
func (h *Handler) ListTables(w http.ResponseWriter, r *http.Request) {
	// TODO Read principal and dbId and schema with PathValue then call Service.ListTables.
	// TODO Return api.TableSummary items in api.DataResponse with status 200.
	// TODO Map service errors to the existing API error envelope.
	api.WriteAPIError(w, http.StatusNotImplemented, "NOT_IMPLEMENTED", "ListTables is not implemented")
}

// GetTableDetail handles GET /api/v1/databases/{dbId}/schemas/{schema}/tables/{table}.
func (h *Handler) GetTableDetail(w http.ResponseWriter, r *http.Request) {
	// TODO Read principal and all three path values then call Service.GetTableDetail.
	// TODO Return api.TableDetail in api.DataResponse with status 200.
	// TODO Map service errors to the existing API error envelope.
	api.WriteAPIError(w, http.StatusNotImplemented, "NOT_IMPLEMENTED", "GetTableDetail is not implemented")
}

// GetTableRows handles GET /api/v1/databases/{dbId}/schemas/{schema}/tables/{table}/rows.
func (h *Handler) GetTableRows(w http.ResponseWriter, r *http.Request) {
	// TODO Read principal and path values and parse limit and offset as integers.
	// TODO Apply the default for an absent limit and reject malformed pagination with 400.
	// TODO Call Service.GetTableRows and return api.TableRows in api.DataResponse with status 200.
	// TODO Map service errors to the existing API error envelope.
	api.WriteAPIError(w, http.StatusNotImplemented, "NOT_IMPLEMENTED", "GetTableRows is not implemented")
}

// RunQuery handles POST /api/v1/databases/{dbId}/queries.
func (h *Handler) RunQuery(w http.ResponseWriter, r *http.Request) {
	// TODO Read principal and dbId and decode api.RunQueryRequest with a body size limit.
	// TODO Call Service.RunQuery after session and CSRF validation.
	// TODO Return api.QueryResult in api.DataResponse with status 200.
	// TODO Map service errors to the existing API error envelope.
	api.WriteAPIError(w, http.StatusNotImplemented, "NOT_IMPLEMENTED", "RunQuery is not implemented")
}
