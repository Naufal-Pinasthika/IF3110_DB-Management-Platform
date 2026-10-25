package app

import (
	"net/http"

	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/api"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/auth"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/config"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/health"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/web"
)

// NewRouter composes the production API, middleware, and embedded frontend.
func NewRouter(repo auth.Repository, cfg config.Config) (http.Handler, error) {
	service, err := auth.NewService(repo, cfg.Session)
	if err != nil {
		return nil, err
	}
	handler := &auth.Handler{Service: service, PublicOrigin: cfg.PublicOrigin}
	apiRouter := api.NewRouter(api.RouteHandlers{
		Health: http.HandlerFunc(health.Health),
		Login:  http.HandlerFunc(handler.Login),
		Me:     http.HandlerFunc(handler.Me),
		Logout: http.HandlerFunc(handler.Logout),
	}, api.RouteMiddleware{
		Session:   handler.SessionMiddleware,
		CSRF:      handler.CSRFMiddleware,
		LoginCSRF: handler.LoginCSRFMiddleware,
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
