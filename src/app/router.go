package app

import (
	"net/http"

	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/api"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/access"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/admin"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/auth"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/config"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/health"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/web"
	"gorm.io/gorm"
)

// NewRouter composes the production API, middleware, and embedded frontend.
func NewRouter(repo auth.Repository, cfg config.Config) (http.Handler, error) {
	service, err := auth.NewService(repo, cfg.Session)
	if err != nil {
		return nil, err
	}
	authHandler := &auth.Handler{Service: service, PublicOrigin: cfg.PublicOrigin}

	var db *gorm.DB
	if pr, ok := repo.(*auth.PostgresRepository); ok {
		db = pr.DB
	}

		accessService := access.NewService(nil)

		// Admin service
		adminRepo := admin.PostgresRepository{
			DB:           db,
			GracePeriod:  cfg.Session.GracePeriod,
		}
		adminService := &admin.Service{
			Repository:   &adminRepo,
			Access:       accessService,
			Cipher:       nil,
			Connections:  nil,
			HashPassword: auth.HashPassword,
		}
		adminHandler := &admin.Handler{Service: adminService}

		apiRouter := api.NewRouter(api.RouteHandlers{
			Health:         http.HandlerFunc(health.Health),
			Login:          http.HandlerFunc(authHandler.Login),
			Me:             http.HandlerFunc(authHandler.Me),
			Logout:         http.HandlerFunc(authHandler.Logout),
			ListUsers:      http.HandlerFunc(adminHandler.ListUsers),
			CreateUser:     http.HandlerFunc(adminHandler.CreateUser),
			UpdateUser:     http.HandlerFunc(adminHandler.UpdateUser),
			ListGroups:     http.HandlerFunc(adminHandler.ListGroups),
			CreateGroup:    http.HandlerFunc(adminHandler.CreateGroup),
			UpdateGroup:    http.HandlerFunc(adminHandler.UpdateGroup),
			DeleteGroup:    http.HandlerFunc(adminHandler.DeleteGroup),
			AddGroupMember:         http.HandlerFunc(adminHandler.AddGroupMember),
			RemoveGroupMember:      http.HandlerFunc(adminHandler.RemoveGroupMember),
			ListManagedDatabases:   http.HandlerFunc(adminHandler.ListManagedDatabases),
			CreateManagedDatabase:  http.HandlerFunc(adminHandler.CreateManagedDatabase),
			GetManagedDatabase:     http.HandlerFunc(adminHandler.GetManagedDatabase),
			UpdateManagedDatabase:  http.HandlerFunc(adminHandler.UpdateManagedDatabase),
			DisableManagedDatabase: http.HandlerFunc(adminHandler.DisableManagedDatabase),
			TestDatabaseConnection: http.HandlerFunc(adminHandler.TestDatabaseConnection),
			SetGroupDatabaseAccess: http.HandlerFunc(adminHandler.SetGroupDatabaseAccess),
			DeleteGroupDatabaseAccess: http.HandlerFunc(adminHandler.DeleteGroupDatabaseAccess),
			// Explorer / migration routes are not implemented yet — return 501 via api.NewRouter
			ListDatabases: nil,
			GetDatabase:   nil,
			ListSchemas:   nil,
			ListTables:    nil,
			GetTableDetail: nil,
			GetTableRows:  nil,
			RunQuery:      nil,
			ListMigrations: nil,
			GetMigration:  nil,
			CreateMigration: nil,
			UpdateMigration: nil,
			ExecuteMigration: nil,
		}, api.RouteMiddleware{
			Session:       authHandler.SessionMiddleware,
			PlatformAdmin: accessService.PlatformAdminMiddleware,
			CSRF:          authHandler.CSRFMiddleware,
			LoginCSRF:     authHandler.LoginCSRFMiddleware,
			DatabaseAccess: func(level api.AccessLevel) api.Middleware {
				// Not implemented yet — return nil so api.NewRouter returns MIDDLEWARE_NOT_CONFIGURED (503)
				return nil
			},
		})
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
