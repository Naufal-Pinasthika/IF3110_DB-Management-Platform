package api

import "net/http"

type RouteHandlers struct {
	Health http.Handler
	Login http.Handler
	Me http.Handler
	Logout http.Handler
	ListUsers http.Handler
	CreateUser http.Handler
	UpdateUser http.Handler
	ListGroups http.Handler
	CreateGroup http.Handler
	UpdateGroup http.Handler
	DeleteGroup http.Handler
	AddGroupMember http.Handler
	RemoveGroupMember http.Handler
	ListManagedDatabases http.Handler
	CreateManagedDatabase http.Handler
	GetManagedDatabase http.Handler
	UpdateManagedDatabase http.Handler
	DisableManagedDatabase http.Handler
	TestDatabaseConnection http.Handler
	SetGroupDatabaseAccess http.Handler
	DeleteGroupDatabaseAccess http.Handler
	ListDatabases http.Handler
	GetDatabase http.Handler
	ListSchemas http.Handler
	ListTables http.Handler
	GetTableDetail http.Handler
	GetTableRows http.Handler
	RunQuery http.Handler
	ListMigrations http.Handler
	GetMigration http.Handler
	CreateMigration http.Handler
	UpdateMigration http.Handler
	ExecuteMigration http.Handler
}

type RouteMiddleware struct {
	Session Middleware
	PlatformAdmin Middleware
	CSRF Middleware
	LoginCSRF Middleware
	DatabaseAccess func(AccessLevel) Middleware
}

func registerRoute(mux *http.ServeMux, method, path string, handler http.Handler, protected ...Middleware) {
	// placeholder as long as all the handler havent been imlemented yet
	if handler == nil {
		handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			WriteAPIError(w, http.StatusNotImplemented, "NOT_IMPLEMENTED", "Endpoint handler is not configured")
		})
	}

	// check every middleware configured if its valid (csrf, session, admin, etc.)
	for i := len(protected) - 1; i >= 0; i-- {
		if protected[i] == nil {
			handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				WriteAPIError(w, http.StatusServiceUnavailable, "MIDDLEWARE_NOT_CONFIGURED", "Required route middleware is not configured")
			})
			break
		}
		handler = protected[i](handler)
	}

	mux.Handle(method+" "+path, handler)
}

func RegisterRoutes(mux *http.ServeMux, handlers RouteHandlers, middleware RouteMiddleware) {
	session := middleware.Session
	admin := middleware.PlatformAdmin
	csrf := middleware.CSRF
	databaseView := databaseAccessMiddleware(middleware, AccessViewOnly)
	databaseEdit := databaseAccessMiddleware(middleware, AccessViewEdit)

	// Health
	registerRoute(mux, "GET", "/health", handlers.Health)

	// Auth
	if middleware.LoginCSRF == nil {
		registerRoute(mux, "POST", "/api/v1/auth/login", handlers.Login)
	} else {
		registerRoute(mux, "POST", "/api/v1/auth/login", handlers.Login, middleware.LoginCSRF)
	}
	registerRoute(mux, "GET", "/api/v1/auth/me", handlers.Me, session)
	registerRoute(mux, "POST", "/api/v1/auth/logout", handlers.Logout, session, csrf)

	// Admin users
	registerRoute(mux, "GET", "/api/v1/admin/users", handlers.ListUsers, session, admin)
	registerRoute(mux, "POST", "/api/v1/admin/users", handlers.CreateUser, session, admin, csrf)
	registerRoute(mux, "PATCH", "/api/v1/admin/users/{userId}", handlers.UpdateUser, session, admin, csrf)

	// Admin groups and membership
	registerRoute(mux, "GET", "/api/v1/admin/groups", handlers.ListGroups, session, admin)
	registerRoute(mux, "POST", "/api/v1/admin/groups", handlers.CreateGroup, session, admin, csrf)
	registerRoute(mux, "PATCH", "/api/v1/admin/groups/{groupId}", handlers.UpdateGroup, session, admin, csrf)
	registerRoute(mux, "DELETE", "/api/v1/admin/groups/{groupId}", handlers.DeleteGroup, session, admin, csrf)
	registerRoute(mux, "PUT", "/api/v1/admin/groups/{groupId}/members/{userId}", handlers.AddGroupMember, session, admin, csrf)
	registerRoute(mux, "DELETE", "/api/v1/admin/groups/{groupId}/members/{userId}", handlers.RemoveGroupMember, session, admin, csrf)

	// Admin managed databases
	registerRoute(mux, "GET", "/api/v1/admin/databases", handlers.ListManagedDatabases, session, admin)
	registerRoute(mux, "POST", "/api/v1/admin/databases", handlers.CreateManagedDatabase, session, admin, csrf)
	registerRoute(mux, "GET", "/api/v1/admin/databases/{dbId}", handlers.GetManagedDatabase, session, admin)
	registerRoute(mux, "PATCH", "/api/v1/admin/databases/{dbId}", handlers.UpdateManagedDatabase, session, admin, csrf)
	registerRoute(mux, "DELETE", "/api/v1/admin/databases/{dbId}", handlers.DisableManagedDatabase, session, admin, csrf)
	registerRoute(mux, "POST", "/api/v1/admin/databases/{dbId}/test-connection", handlers.TestDatabaseConnection, session, admin, csrf)

	// Admin database grants
	registerRoute(mux, "PUT", "/api/v1/admin/groups/{groupId}/databases/{dbId}/access", handlers.SetGroupDatabaseAccess, session, admin, csrf)
	registerRoute(mux, "DELETE", "/api/v1/admin/groups/{groupId}/databases/{dbId}/access", handlers.DeleteGroupDatabaseAccess, session, admin, csrf)

	// Database explorer
	registerRoute(mux, "GET", "/api/v1/databases", handlers.ListDatabases, session)
	registerRoute(mux, "GET", "/api/v1/databases/{dbId}", handlers.GetDatabase, session, databaseView)
	registerRoute(mux, "GET", "/api/v1/databases/{dbId}/schemas", handlers.ListSchemas, session, databaseView)
	registerRoute(mux, "GET", "/api/v1/databases/{dbId}/schemas/{schema}/tables", handlers.ListTables, session, databaseView)
	registerRoute(mux, "GET", "/api/v1/databases/{dbId}/schemas/{schema}/tables/{table}", handlers.GetTableDetail, session, databaseView)
	registerRoute(mux, "GET", "/api/v1/databases/{dbId}/schemas/{schema}/tables/{table}/rows", handlers.GetTableRows, session, databaseView)

	// Read only query endpoint
	registerRoute(mux, "POST", "/api/v1/databases/{dbId}/queries", handlers.RunQuery, session, databaseView, csrf)

	// Migrations
	registerRoute(mux, "GET", "/api/v1/databases/{dbId}/migrations", handlers.ListMigrations, session, databaseView)
	registerRoute(mux, "GET", "/api/v1/databases/{dbId}/migrations/{migrationId}", handlers.GetMigration, session, databaseView)
	registerRoute(mux, "POST", "/api/v1/databases/{dbId}/migrations", handlers.CreateMigration, session, databaseEdit, csrf)
	registerRoute(mux, "PATCH", "/api/v1/databases/{dbId}/migrations/{migrationId}", handlers.UpdateMigration, session, databaseEdit, csrf)
	registerRoute(mux, "POST", "/api/v1/databases/{dbId}/migrations/{migrationId}/execute", handlers.ExecuteMigration, session, databaseEdit, csrf)
}
