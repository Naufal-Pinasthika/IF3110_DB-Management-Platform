package api

import (
	"time"

	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/models"
)

// General attr
type DataResponse[T any] struct {
	Data T `json:"data"`
}

type ErrorResponse struct {
	Error APIError `json:"error"`
}

type APIError struct {
	Code string `json:"code"`
	Message string `json:"message"`
}

type AccessLevel = models.AccessLevel
const (
	AccessViewOnly = models.AccessViewOnly
	AccessViewEdit = models.AccessViewEdit
)

type MigrationStatus = models.MigrationStatus
const (
	MigrationDraft = models.MigrationDraft
	MigrationRunning = models.MigrationRunning
	MigrationApplied = models.MigrationApplied
	MigrationFailed = models.MigrationFailed
)

// ------------------------------------------------------------
// POST /api/v1/auth/login request body
type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// POST /api/v1/auth/login response body
// GET /api/v1/auth/me response body
type LoginResponse struct {
	ID string  `json:"id"`
	Username string `json:"username"`
	IsPlatformAdmin bool `json:"is_platform_admin"`
}

// POST /api/v1/auth/logout response body (no need for request body)
type LogoutResult struct {
	Success bool `json:"success"`
}

// ------------------------------------------------------------
// POST /api/v1/admin/users request body
type CreateUserRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	IsPlatformAdmin bool `json:"is_platform_admin"`
}

// PATCH /api/v1/admin/users/{userId} request body
type UpdateUserRequest struct {
	Username *string `json:"username,omitempty"`
	Password *string `json:"password,omitempty"`
	IsActive *bool `json:"is_active,omitempty"`
}

// POST /api/v1/admin/users response body
// PATCH /api/v1/admin/users/{userId} response body
// GET /api/v1/admin/users response body
type AdminUser struct {
	ID string  `json:"id"`
	Username string `json:"username"`
	IsPlatformAdmin bool `json:"is_platform_admin"`
	IsActive bool `json:"is_active"`
}

// ------------------------------------------------------------
// POST /api/v1/admin/groups
type CreateGroupRequest struct {
	Name string `json:"name"`
}

// PATCH /api/v1/admin/groups/{groupId}
type UpdateGroupRequest struct {
	Name *string `json:"name,omitempty"`
}

// POST /api/v1/admin/groups response body (Group)
// PATCH /api/v1/admin/groups/{groupId} response body (Group)
// GET /api/v1/admin/groups response body ([]Group)
type Group struct {
	ID string `json:"id"`
	Name string `json:"name"`
}

// PUT /api/v1/admin/groups/{groupId}/members/{userId} response body
type GroupMember struct {
	GroupID string `json:"group_id"`
	UserID string `json:"user_id"`
}

// DELETE /api/v1/admin/groups/{groupId}/members/{userId} dont have any request/response body

// PUT /api/v1/admin/groups/{groupId}/databases/{dbId}/access request body
type SetGroupAccessRequest struct {
	AccessLevel AccessLevel `json:"access_level"`
}

// PUT /api/v1/admin/groups/{groupId}/databases/{dbId}/access response body
type GroupDatabaseAccess struct {
	GroupID string `json:"group_id"`
	DatabaseID string `json:"database_id"`
	AccessLevel AccessLevel `json:"access_level"`
}

// DELETE /api/v1/admin/groups/{groupId}/databases/{dbId}/access dont have any request/response body

// DELETE /api/v1/admin/groups/{groupId} dont have any request/response body

// ------------------------------------------------------------
// POST /api/v1/admin/databases request body
type CreateDatabaseRequest struct {
	Label string `json:"label"`
	Host string `json:"host"`
	Port int `json:"port"`
	DatabaseName string `json:"database_name"`
	SSLMode string `json:"ssl_mode"`
	ReaderUsername string `json:"reader_username"`
	ReaderPassword string `json:"reader_password"`
	MigratorUsername string `json:"migrator_username"`
	MigratorPassword string `json:"migrator_password"`
}

// PATCH /api/v1/admin/databases/{dbId} request body
type UpdateDatabaseRequest struct {
	Label *string `json:"label,omitempty"`
	Host *string `json:"host,omitempty"`
	Port *int `json:"port,omitempty"`
	DatabaseName *string `json:"database_name,omitempty"`
	SSLMode *string `json:"ssl_mode,omitempty"`
	ReaderUsername *string `json:"reader_username,omitempty"`
	ReaderPassword *string `json:"reader_password,omitempty"`
	MigratorUsername *string `json:"migrator_username,omitempty"`
	MigratorPassword *string `json:"migrator_password,omitempty"`
	IsEnabled *bool `json:"is_enabled,omitempty"`
}

// GET /api/v1/admin/databases response body ([]ManagedDatabaseSummary)
// POST /api/v1/admin/databases response body (ManagedDatabaseSummary)
// PATCH /api/v1/admin/databases/{dbId} response body (ManagedDatabaseSummary)
type ManagedDatabaseSummary struct {
	ID string `json:"id"`
	Label string `json:"label"`
	DatabaseName string `json:"database_name"`
	IsEnabled bool `json:"is_enabled"`
}

// GET /api/v1/admin/databases/{dbId} response body
type ManagedDatabaseDetail struct {
	ID string `json:"id"`
	Label string `json:"label"`
	Host string `json:"host"`
	Port int `json:"port"`
	DatabaseName string `json:"database_name"`
	SSLMode string `json:"ssl_mode"`
	ReaderUsername string `json:"reader_username"`
	MigratorUsername string `json:"migrator_username"`
	IsEnabled bool `json:"is_enabled"`
}

// DELETE /api/v1/admin/databases/{dbId} dont have any request/response body

// POST /api/v1/admin/databases/{dbId}/test-connection
type TestConnectionResult struct {
    Connected bool   `json:"connected"`
    Message   string `json:"message,omitempty"`
}

// ------------------------------------------------------------
// GET /api/v1/databases response body ([]UserDatabase)
// GET /api/v1/databases/{dbId} (UserDatabase)
type UserDatabase struct {
    ID string `json:"id"`
    Label string `json:"label"`
    DatabaseName string `json:"database_name"`
    EffectiveAccess AccessLevel `json:"effective_access"`
}

// GET /api/v1/databases/{dbId}/schemas response body ([]SchemaSummary)
type SchemaSummary struct {
    Name string `json:"name"`
}

// GET /api/v1/databases/{dbId}/schemas/{schema}/tables ([]TableSummary)
type TableSummary struct {
    Name string `json:"name"`
}

type TableColumn struct {
    Name string `json:"name"`
    DataType string `json:"data_type"`
    Nullable bool   `json:"nullable"`
}
type TableConstraint struct {
    Name string   `json:"name"`
    Type string   `json:"type"`
    Columns []string `json:"columns"`
}
type TableIndex struct {
    Name string   `json:"name"`
    Columns []string `json:"columns"`
    IsUnique bool     `json:"is_unique"`
}

// GET /api/v1/databases/{dbId}/schemas/{schema}/tables/{table} response body
type TableDetail struct {
    Schema string `json:"schema"`
    Table string `json:"table"`
    Columns []TableColumn `json:"columns"`
    Constraints []TableConstraint `json:"constraints"`
    Indexes []TableIndex `json:"indexes"`
}

// GET /api/v1/databases/{dbId}/schemas/{schema}/tables/{table}/rows response body
type TableRows struct {
    Columns []string `json:"columns"`
    Rows [][]any  `json:"rows"`
    Limit int `json:"limit"`
    Offset int `json:"offset"`
    HasMore bool `json:"has_more"`
}

// ------------------------------------------------------------
// POST /api/v1/databases/{dbId}/queries request body
type RunQueryRequest struct {
    SQL string `json:"sql"`
    MaxRows int `json:"max_rows"`
}

// POST /api/v1/databases/{dbId}/queries response body
type QueryResult struct {
    Columns []string `json:"columns"`
    Rows [][]any `json:"rows"`
    RowCount int `json:"row_count"`
    Truncated bool `json:"truncated"`
}

// ------------------------------------------------------------
// POST /api/v1/databases/{dbId}/migrations request body
type CreateMigrationRequest struct {
    Name    string `json:"name"`
    SQLText string `json:"sql_text"`
}

// POST /api/v1/databases/{dbId}/migrations response body
type MigrationCreated struct {
	ID string `json:"id"`
	DatabaseID string `json:"database_id"`
	Name string `json:"name"`
	Status MigrationStatus `json:"status"`
	Version *int64 `json:"version"`
}

// PATCH /api/v1/databases/{dbId}/migrations/{migrationId} request body
type UpdateMigrationRequest struct {
    SQLText string `json:"sql_text"`
}

// PATCH /api/v1/databases/{dbId}/migrations/{migrationId} response body
type MigrationUpdated struct {
    ID string `json:"id"`
    Status MigrationStatus `json:"status"`
    Version *int64 `json:"version"`
}

// POST /api/v1/databases/{dbId}/migrations/{migrationId}/execute response body
type MigrationExecuted struct {
    ID string `json:"id"`
    Status MigrationStatus `json:"status"`
    Version *int64 `json:"version"`
    ExecutedBy *string `json:"executed_by"`
    ExecutedAt *time.Time `json:"executed_at"`
}

// GET /api/v1/databases/{dbId}/migrations response body ([]MigrationSummary)
type MigrationSummary struct {
    ID string `json:"id"`
    Name string `json:"name"`
    Version *int64 `json:"version"`
    Status MigrationStatus `json:"status"`
    ExecutedBy *string `json:"executed_by"`
    ExecutedAt *time.Time `json:"executed_at"`
}

// GET /api/v1/databases/{dbId}/migrations/{migrationId} response body
type MigrationDetail struct {
    ID string `json:"id"`
    DatabaseID string `json:"database_id"`
    Name string `json:"name"`
    SQLText string `json:"sql_text"`
    Status MigrationStatus `json:"status"`
    Version *int64 `json:"version"`
    CreatedBy string `json:"created_by"`
    ExecutedBy *string `json:"executed_by"`
    CreatedAt time.Time `json:"created_at"`
    UpdatedAt time.Time `json:"updated_at"`
    ExecutedAt *time.Time `json:"executed_at"`
    ErrorSummary *string `json:"error_summary"`
}

