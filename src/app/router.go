package app

import (
	"net/http"
	"time"

	"gorm.io/gorm"

	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/api"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/access"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/admin"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/auth"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/config"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/explorer"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/health"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/migration"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/store"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/web"
)

type dbProvider interface {
	GetDB() *gorm.DB
}

// NewRouter composes the production API, middleware, and embedded frontend.
func NewRouter(repo auth.Repository, cfg config.Config) (http.Handler, error) {
	service, err := auth.NewService(repo, cfg.Session)
	if err != nil {
		return nil, err
	}
	authHandler := &auth.Handler{Service: service, PublicOrigin: cfg.PublicOrigin}

	// Session middleware wrapper that injects Principal into access context
	sessionMiddleware := func(next http.Handler) http.Handler {
		return authHandler.SessionMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if principal, ok := auth.PrincipalFromContext(r.Context()); ok {
				r = r.WithContext(auth.SetPrincipalContext(r.Context(), principal))
			}
			next.ServeHTTP(w, r)
		}))
	}

	routeHandlers := api.RouteHandlers{
		Health: http.HandlerFunc(health.Health),
		Login:  http.HandlerFunc(authHandler.Login),
		Me:     http.HandlerFunc(authHandler.Me),
		Logout: http.HandlerFunc(authHandler.Logout),
	}
	routeMiddleware := api.RouteMiddleware{
		Session:   sessionMiddleware,
		CSRF:      authHandler.CSRFMiddleware,
		LoginCSRF: authHandler.LoginCSRFMiddleware,
	}

	var db *gorm.DB
	if pgRepo, ok := repo.(*auth.PostgresRepository); ok && pgRepo != nil {
		db = pgRepo.DB
	} else if provider, ok := repo.(dbProvider); ok && provider != nil {
		db = provider.GetDB()
	}

	if db != nil {
		encKey := cfg.DBCredentialEncryptionKey
		if len(encKey) != 32 {
			encKey = make([]byte, 32)
		}
		cipher := &store.Cipher{Key: encKey}

		adminRepo := &admin.PostgresRepository{DB: db}
		targetStore := &store.TargetStore{
			Databases: adminRepo,
			Cipher:    cipher,
		}

		accessRepo := &access.PostgresRepository{DB: db}
		accessService := &access.Service{Repository: accessRepo}

		adminService := &admin.Service{
			Repository:   adminRepo,
			Access:       accessService,
			Cipher:       cipher,
			Connections:  targetStore,
			HashPassword: auth.HashPassword,
		}
		adminHandler := &admin.Handler{Service: adminService}

		queryTimeout := cfg.QueryTimeout
		if queryTimeout <= 0 {
			queryTimeout = 5 * time.Second
		}
		queryMaxRows := cfg.QueryMaxRows
		if queryMaxRows <= 0 {
			queryMaxRows = 100
		}

		explorerRepo := &explorer.TargetRepository{
			Targets:      targetStore,
			QueryTimeout: queryTimeout,
		}
		explorerService := &explorer.Service{
			Repository:   explorerRepo,
			Access:       accessService,
			QueryMaxRows: queryMaxRows,
		}
		explorerHandler := &explorer.Handler{Service: explorerService}

		migrationRepo := &migration.PostgresRepository{DB: db}
		migrationRunner := &migration.Runner{
			Targets: targetStore,
			Timeout: queryTimeout,
		}
		schemaDiffer := &migration.SchemaDiffer{Targets: targetStore, Timeout: queryTimeout}
		migrationService := &migration.Service{
			Repository: migrationRepo,
			Access:     accessService,
			Runner:     migrationRunner,
			Explainer:  explorerRepo,
			SchemaDiff: schemaDiffer,
		}
		migrationHandler := &migration.Handler{Service: migrationService}

		// Admin users
		routeHandlers.ListUsers = http.HandlerFunc(adminHandler.ListUsers)
		routeHandlers.CreateUser = http.HandlerFunc(adminHandler.CreateUser)
		routeHandlers.UpdateUser = http.HandlerFunc(adminHandler.UpdateUser)

		routeHandlers.ListAccessSnapshot = http.HandlerFunc(adminHandler.ListAccessSnapshot)
		routeHandlers.TestConnectionDraft = http.HandlerFunc(adminHandler.TestConnectionDraft)

		// Admin groups and membership
		routeHandlers.ListGroups = http.HandlerFunc(adminHandler.ListGroups)
		routeHandlers.CreateGroup = http.HandlerFunc(adminHandler.CreateGroup)
		routeHandlers.UpdateGroup = http.HandlerFunc(adminHandler.UpdateGroup)
		routeHandlers.DeleteGroup = http.HandlerFunc(adminHandler.DeleteGroup)
		routeHandlers.AddGroupMember = http.HandlerFunc(adminHandler.AddGroupMember)
		routeHandlers.RemoveGroupMember = http.HandlerFunc(adminHandler.RemoveGroupMember)

		// Admin managed databases
		routeHandlers.ListManagedDatabases = http.HandlerFunc(adminHandler.ListManagedDatabases)
		routeHandlers.CreateManagedDatabase = http.HandlerFunc(adminHandler.CreateManagedDatabase)
		routeHandlers.GetManagedDatabase = http.HandlerFunc(adminHandler.GetManagedDatabase)
		routeHandlers.UpdateManagedDatabase = http.HandlerFunc(adminHandler.UpdateManagedDatabase)
		routeHandlers.DisableManagedDatabase = http.HandlerFunc(adminHandler.DisableManagedDatabase)
		routeHandlers.DeleteManagedDatabase = http.HandlerFunc(adminHandler.DeleteManagedDatabase)
		routeHandlers.TestDatabaseConnection = http.HandlerFunc(adminHandler.TestDatabaseConnection)

		// Admin database grants
		routeHandlers.SetGroupDatabaseAccess = http.HandlerFunc(adminHandler.SetGroupDatabaseAccess)
		routeHandlers.DeleteGroupDatabaseAccess = http.HandlerFunc(adminHandler.DeleteGroupDatabaseAccess)

		// Database explorer
		routeHandlers.ListDatabases = http.HandlerFunc(explorerHandler.ListDatabases)
		routeHandlers.GetDatabase = http.HandlerFunc(explorerHandler.GetDatabase)
		routeHandlers.ListSchemas = http.HandlerFunc(explorerHandler.ListSchemas)
		routeHandlers.ListTables = http.HandlerFunc(explorerHandler.ListTables)
		routeHandlers.GetTableDetail = http.HandlerFunc(explorerHandler.GetTableDetail)
		routeHandlers.GetTableRows = http.HandlerFunc(explorerHandler.GetTableRows)
		routeHandlers.RunQuery = http.HandlerFunc(explorerHandler.RunQuery)
		routeHandlers.ExplainQuery = http.HandlerFunc(explorerHandler.ExplainQuery)

		// Migrations
		routeHandlers.ListMigrations = http.HandlerFunc(migrationHandler.ListMigrations)
		routeHandlers.GetMigration = http.HandlerFunc(migrationHandler.GetMigration)
		routeHandlers.CreateMigration = http.HandlerFunc(migrationHandler.CreateMigration)
		routeHandlers.UpdateMigration = http.HandlerFunc(migrationHandler.UpdateMigration)
		routeHandlers.ExecuteMigration = http.HandlerFunc(migrationHandler.ExecuteMigration)
		routeHandlers.ExplainMigration = http.HandlerFunc(migrationHandler.ExplainMigration)
		routeHandlers.CompareSchema = http.HandlerFunc(migrationHandler.CompareSchema)

		// Middlewares
		routeMiddleware.PlatformAdmin = accessService.PlatformAdminMiddleware
		routeMiddleware.DatabaseAccess = accessService.DatabaseAccessMiddleware
	}

	apiRouter := api.NewRouter(routeHandlers, routeMiddleware)
	pages, err := web.New()
	if err != nil {
		return nil, err
	}
	mux := http.NewServeMux()
	mux.Handle("/api/", apiRouter)
	mux.Handle("/health", apiRouter)
	mux.Handle("/", pages)
	return mux, nil
}
