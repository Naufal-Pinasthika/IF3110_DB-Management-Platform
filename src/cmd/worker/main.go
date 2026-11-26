package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/app"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/config"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/jobs"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/store"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/telemetry"
)

func main() { os.Exit(run()) }

func run() int {
	shutdown := telemetry.Init("platform-worker")

	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = shutdown(ctx)
	}()

	cfg, err := config.Load()
	if err != nil {
		slog.Error("worker.configuration_failed", "error", err.Error())
		return 1
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	db, err := store.OpenPlatform(ctx, cfg.PlatformDatabaseURL)
	if err != nil {
		slog.Error("worker.database_unavailable")
		return 1
	}

	pool, err := db.DB()
	if err != nil {
		slog.Error("worker.pool_unavailable")
		return 1
	}

	telemetry.RegisterDBPool("platform", pool)
	telemetry.RegisterJobStatusCounter((&jobs.PostgresRepository{DB: db}).CountByStatus)

	if metricsAddr := os.Getenv("WORKER_METRICS_ADDR"); metricsAddr != "" {
		mux := http.NewServeMux()
		mux.Handle("GET /metrics", telemetry.MetricsHandler())
		metricsServer := &http.Server{
			Addr:              metricsAddr,
			Handler:           mux,
			ReadHeaderTimeout: 5 * time.Second,
		}
		go func() {
			slog.Info("worker.metrics_listening", "address", metricsAddr)
			if err := metricsServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				slog.Error("worker.metrics_failed", "error", err.Error())
			}
		}()
		defer func() {
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = metricsServer.Shutdown(shutdownCtx)
		}()
	}

	worker, err := app.NewMigrationWorker(db, cfg)
	if err != nil {
		_ = pool.Close()
		slog.Error("worker.integration_unavailable", "error", err.Error())
		return 1
	}

	finished := make(chan error, 1)
	go func() { finished <- worker.Run(ctx) }()

	select {
	case err := <-finished:
		_ = pool.Close()
		if err != nil {
			slog.Error("worker.start_failed", "error", "queue schema or database unavailable")
			return 1
		}
		return 0
	case <-ctx.Done():
		timer := time.NewTimer(cfg.Jobs.ShutdownTimeout)
		defer timer.Stop()
		select {
		case <-finished:
			_ = pool.Close()
			return 0
		case <-timer.C:
			// Do not return lock connections to a pool while an executor might still run
			slog.Error("worker.shutdown_timed_out", "status", "reconciliation_required")
			return 1
		}
	}
}
