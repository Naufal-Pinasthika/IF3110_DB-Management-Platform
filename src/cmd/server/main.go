package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/app"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/auth"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/config"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/store"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/telemetry"
)

func main() {
	shutdown := telemetry.Init("platform-api")
	if err := run(); err != nil {
		slog.Error("server.stopped", "error", "startup or serving failed")
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = shutdown(ctx)
		os.Exit(1)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = shutdown(ctx)
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	db, err := store.OpenPlatform(ctx, cfg.PlatformDatabaseURL)
	if err != nil {
		return err
	}
	pool, err := db.DB()
	if err != nil {
		return err
	}
	defer pool.Close()
	router, err := app.NewRouter(&auth.PostgresRepository{DB: db}, cfg)
	if err != nil {
		return err
	}
	server := &http.Server{
		Addr:              cfg.AppAddr,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	slog.Info("server.listening", "address", cfg.AppAddr)
	finished := make(chan error, 1)
	go func() { finished <- server.ListenAndServe() }()
	select {
	case err := <-finished:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			_ = server.Close()
			return err
		}
		if err := <-finished; !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	}
	return nil
}
