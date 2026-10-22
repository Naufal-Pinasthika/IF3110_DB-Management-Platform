package api

import (
	"net/http"

	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/health"
)

type Middleware func(http.Handler) http.Handler

func NewRouter() http.Handler {
	handlers := RouteHandlers{
		// register all the business logic function here
		Health: http.HandlerFunc(health.Health),
		// Login,
		// Me,
		// Logout,
		// ListUsers,
		// CreateUser,
		// UpdateUser,
		// ListGroups,
		// CreateGroup,
		// UpdateGroup,
		// DeleteGroup,
		// AddGroupMember,
		// RemoveGroupMember,
		// ListManagedDatabases,
		// CreateManagedDatabase,
		// GetManagedDatabase,
		// UpdateManagedDatabase,
		// DisableManagedDatabase,
		// TestDatabaseConnection,
		// SetGroupDatabaseAccess,
		// DeleteGroupDatabaseAccess,
		// ListDatabases,
		// GetDatabase,
		// ListSchemas,
		// ListTables,
		// GetTableDetail,
		// GetTableRows,
		// RunQuery,
		// ListMigrations,
		// GetMigration,
		// CreateMigration,
		// UpdateMigration,
		// ExecuteMigration,
	}

	middleware := RouteMiddleware{}

	mux := http.NewServeMux()
	RegisterRoutes(mux, handlers, middleware)
	return  mux
}
