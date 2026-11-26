package main

import (
	"context"
	"errors"
	"fmt"
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
		slog.Error("server.stopped", "error", err.Error())
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
		return fmt.Errorf("config.Load failed: %w", err)
	}
	db, err := store.OpenPlatform(ctx, cfg.PlatformDatabaseURL)
	if err != nil {
		return fmt.Errorf("store.OpenPlatform failed: %w", err)
	}
	pool, err := db.DB()
	if err != nil {
		return fmt.Errorf("db.DB pool failed: %w", err)
	}
	defer pool.Close()
	router, err := app.NewRouter(&auth.PostgresRepository{DB: db}, cfg)
	if err != nil {
		return fmt.Errorf("app.NewRouter failed: %w", err)
	}
	worker, err := app.NewMigrationWorker(db, cfg)
	if err != nil {
		slog.Warn("warning: migration worker initialization", "error", err)
	} else if worker != nil {
		workerCtx, cancelWorker := context.WithCancel(context.Background())
		defer cancelWorker()
		worker.Start(workerCtx)
		defer worker.Stop(context.Background())
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
