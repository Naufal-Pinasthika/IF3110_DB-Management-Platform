package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"time"

	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/api"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/auth"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/config"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/health"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/store"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/web"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	db, err := store.OpenPlatform(context.Background(), cfg.PlatformDatabaseURL)
	if err != nil {
		return err
	}
	pool, err := db.DB()
	if err != nil {
		return err
	}
	defer pool.Close()
	router, err := newRouter(&auth.PostgresRepository{DB: db}, cfg)
	if err != nil {
		return err
	}
	server := &http.Server{
		Addr:              cfg.AppAddr,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	log.Printf("HTTP server listening on %s", cfg.AppAddr)
	if err := server.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// Keep composition here: auth already imports api for request and response types.
func newRouter(repo auth.Repository, cfg config.Config) (http.Handler, error) {
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
