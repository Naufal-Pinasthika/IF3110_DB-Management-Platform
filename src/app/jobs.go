package app

import (
	"fmt"
	"time"

	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/access"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/admin"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/config"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/explorer"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/jobs"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/migration"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/store"
	"gorm.io/gorm"
)

type JobsIntegration struct{ Registry jobs.ExecutionRegistry }

func jobOptions(cfg config.Config) jobs.Options {
	if cfg.Jobs == (config.JobsConfig{}) {
		return jobs.DefaultOptions()
	}
	return cfg.Jobs
}

func NewJobService(db *gorm.DB, cfg config.Config, registry jobs.ExecutionRegistry) *jobs.Service {
	return &jobs.Service{Repository: &jobs.PostgresRepository{DB: db}, Registry: registry,
		Access: &access.Service{Repository: &access.PostgresRepository{DB: db}}, Options: jobOptions(cfg)}
}

// injection point for a migration executor
func NewJobsWorker(db *gorm.DB, cfg config.Config, executor jobs.Executor) (*jobs.Worker, error) {
	return jobs.NewWorker(&jobs.PostgresRepository{DB: db}, executor, jobOptions(cfg))
}

func newMigrationService(db *gorm.DB, cfg config.Config, targetStore *store.TargetStore, accessService *access.Service, explorerRepo *explorer.TargetRepository) *migration.Service {
	queryTimeout := cfg.QueryTimeout
	if queryTimeout <= 0 {
		queryTimeout = 5 * time.Second
	}
	service := &migration.Service{
		Repository: &migration.PostgresRepository{DB: db}, Access: accessService,
		Runner:     &migration.Runner{Targets: targetStore, Timeout: jobOptions(cfg).ExecutionTimeout},
		Explainer:  explorerRepo,
		SchemaDiff: &migration.SchemaDiffer{Targets: targetStore, Timeout: queryTimeout},
	}
	return service
}


func NewMigrationWorker(db *gorm.DB, cfg config.Config) (*jobs.Worker, error) {
	options := jobOptions(cfg)

	accessService := &access.Service{Repository: &access.PostgresRepository{DB: db}}
	targets := &store.TargetStore{Databases: &admin.PostgresRepository{DB: db}, Cipher: &store.Cipher{Key: cfg.DBCredentialEncryptionKey}}

	explorerRepo := &explorer.TargetRepository{Targets: targets, QueryTimeout: cfg.QueryTimeout}
	service := newMigrationService(db, cfg, targets, accessService, explorerRepo)

	executor, ok := any(service).(jobs.Executor)
	if !ok {
		return nil, fmt.Errorf("%w: migration.Service must implement jobs.Executor (Execute and Inspect)", jobs.ErrUnavailable)
	}
	registry, ok := any(service).(jobs.ExecutionRegistry)
	if !ok {
		return nil, fmt.Errorf("%w: migration.Service must implement jobs.ExecutionRegistry (ValidateExecution)", jobs.ErrUnavailable)
	}

	service.Dispatcher = NewJobService(db, cfg, registry)
	pool, err := db.DB()

	if err != nil {
		return nil, err
	}

	pool.SetMaxOpenConns(options.Concurrency*2 + 4)
	pool.SetMaxIdleConns(options.Concurrency + 2)
	
	return NewJobsWorker(db, cfg, executor)
}
