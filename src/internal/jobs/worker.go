package jobs

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	randv2 "math/rand/v2"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/contracts"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/telemetry"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

type Worker struct {
	Repository *PostgresRepository
	Executor   Executor
	Options    Options
	pool       *sql.DB
	id         string
	cancel     context.CancelFunc
	done       chan struct{}
}

func NewWorker(repository *PostgresRepository, executor Executor, options Options) (*Worker, error) {
	if repository == nil || repository.DB == nil || executor == nil {
		return nil, ErrUnavailable
	}
	
	if err := options.Validate(); err != nil {
		return nil, err
	}

	pool, err := repository.DB.DB()
	if err != nil {
		return nil, err
	}

	if max := pool.Stats().MaxOpenConnections; max > 0 && max < options.Concurrency*2+2 {
		return nil, fmt.Errorf("worker platform pool needs at least %d connections", options.Concurrency*2+2)
	}

	host, _ := os.Hostname()
	var nonce [8]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return nil, err
	}

	return &Worker{Repository: repository, Executor: executor, Options: options,
		pool: pool, id: host + "-" + hex.EncodeToString(nonce[:])}, nil
}

func (w *Worker) Start(ctx context.Context) {
	runCtx, cancel := context.WithCancel(ctx)
	w.cancel = cancel
	w.done = make(chan struct{})
	go func() {
		defer close(w.done)
		if err := w.Run(runCtx); err != nil && !errors.Is(err, context.Canceled) {
			telemetry.Log(runCtx, slog.LevelError, "worker.run_failed", "worker_id", w.id, "error", err.Error())
		}
	}()
}

func (w *Worker) Stop(ctx context.Context) error {
	if w.cancel != nil {
		w.cancel()
	}
	if w.done != nil {
		select {
		case <-w.done:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

func (w *Worker) Run(ctx context.Context) error {
	if err := w.Repository.Ready(ctx); err != nil {
		return fmt.Errorf("migration jobs schema is unavailable: %w", err)
	}

	telemetry.Log(ctx, slog.LevelInfo, "worker.started", "worker_id", w.id, "concurrency", w.Options.Concurrency)
	var workers sync.WaitGroup

	for range w.Options.Concurrency {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for ctx.Err() == nil {
				didWork, err := w.take(ctx)
				if err != nil && ctx.Err() == nil {
					telemetry.Log(ctx, slog.LevelError, "worker.poll_failed", "worker_id", w.id, "error_code", "QUEUE_UNAVAILABLE")
				}
				if !didWork || err != nil {
					if !wait(ctx, w.Options.PollInterval) {
						return
					}
				}
			}
		}()
	}

	workers.Wait()
	telemetry.Log(context.Background(), slog.LevelInfo, "worker.stopped", "worker_id", w.id)
	return nil
}

func wait(ctx context.Context, delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

// claims and runs next ready job
func (w *Worker) take(ctx context.Context) (bool, error) {
	candidates, err := w.Repository.Candidates(ctx, 64)
	if err != nil {
		return false, err
	}

	for _, candidate := range candidates {
		if ctx.Err() != nil {
			return false, ctx.Err()
		}

		carrier := make(map[string]string)
		if err := json.Unmarshal(candidate.TraceJSON, &carrier); err != nil {
			carrier = nil
		}
		
		jobCtx := telemetry.Extract(ctx, carrier)
		jobCtx = telemetry.Correlate(jobCtx, candidate.ActorID, candidate.DatabaseID, candidate.ID, candidate.ExecutionID)
		jobCtx, span := otel.Tracer("platform/jobs").Start(jobCtx, "queue.process", trace.WithSpanKind(trace.SpanKindConsumer),
			trace.WithAttributes(attribute.String("job.id", candidate.ID), attribute.String("execution.id", candidate.ExecutionID), attribute.String("database.id", candidate.DatabaseID), attribute.String("actor.id", candidate.ActorID)))
		lockCtx, cancel := context.WithTimeout(jobCtx, w.Options.HeartbeatInterval)
		lock, err := tryDatabaseLock(lockCtx, w.pool, candidate.DatabaseID)
		cancel()
		if err != nil {
			span.SetStatus(codes.Error, "lock failed")
			span.End()
			return false, err
		}
		if lock == nil {
			span.End()
			continue
		}
		claimCtx, claimCancel := context.WithTimeout(jobCtx, w.Options.HeartbeatInterval)
		job, err := w.Repository.Claim(claimCtx, candidate.ID, w.id, w.Options.LeaseDuration)
		claimCancel()
		if err != nil {
			lock.release()
			span.End()
			if errors.Is(err, ErrClaimLost) {
				continue
			}
			return false, err
		}
		w.process(jobCtx, job, lock)
		span.End()
		return true, nil
	}
	return false, nil
}

func (w *Worker) process(ctx context.Context, job Job, lock *databaseLock) {
	defer lock.release()
	inspect := job.NeedsInspection || job.AttemptCount >= job.MaxAttempts
	opCtx, cancel := context.WithTimeout(ctx, w.Options.ExecutionTimeout)
	defer cancel()
	beginCtx, beginCancel := context.WithTimeout(opCtx, w.Options.HeartbeatInterval)
	err := w.Repository.BeginExecution(beginCtx, &job)
	beginCancel()
	if err != nil {
		telemetry.Log(ctx, slog.LevelError, "job.claim_failed", "job_id", job.ID, "database_id", job.DatabaseID, "error_code", "CLAIM_UNAVAILABLE")
		return
	}
	opCtx = telemetry.WithAttempt(opCtx, job.AttemptCount)
	if job.AttemptCount == 1 && !job.CreatedAt.IsZero() {
		telemetry.ObserveQueueWait(time.Since(job.CreatedAt))
	}
	kind := "execute"
	if inspect {
		kind = "inspect"
	}
	telemetry.Log(ctx, slog.LevelInfo, "job.attempt_started", "job_id", job.ID, "execution_id", job.ExecutionID,
		"database_id", job.DatabaseID, "attempt", job.AttemptCount, "operation", kind)
	stop := make(chan struct{})
	monitorDone := make(chan error, 1)
	go func() { monitorDone <- w.monitor(opCtx, cancel, stop, job, lock) }()
	callCtx, span := otel.Tracer("platform/jobs").Start(opCtx, "migration."+kind)
	result, callErr := w.call(callCtx, job.Ref(), inspect)
	close(stop)
	monitorErr := <-monitorDone
	if monitorErr != nil {
		callErr = monitorErr
	}
	if callErr != nil {
		span.SetStatus(codes.Error, "execution outcome requires inspection")
	}
	span.End()
	done := w.decide(job, result, callErr, inspect)
	finishCtx, finishCancel := context.WithTimeout(context.WithoutCancel(ctx), w.Options.HeartbeatInterval)
	defer finishCancel()
	if err := w.Repository.Finish(finishCtx, job, done); err != nil {
		telemetry.Log(ctx, slog.LevelError, "job.finish_unconfirmed", "job_id", job.ID, "execution_id", job.ExecutionID,
			"database_id", job.DatabaseID, "error_code", "FINISH_UNCONFIRMED")
		return
	}
	if done.Status == contracts.JobRetryWait {
		telemetry.IncJobRetry(string(done.Result.RetryAdvice))
	} else {
		telemetry.IncJobFinished(string(done.Status))
	}
	telemetry.Log(opCtx, slog.LevelInfo, "job.attempt_finished", "job_id", job.ID, "execution_id", job.ExecutionID,
		"database_id", job.DatabaseID, "status", done.Status, "outcome", done.Result.State,
		"attempt", job.AttemptCount, "retry_advice", done.Result.RetryAdvice,
		"committed_batches", done.Result.CommittedBatches, "committed_rows", done.Result.CommittedRows,
		"database_blocked", done.Result.BlocksDatabase, "error_code", done.Result.ErrorCode)
}

func (w *Worker) call(ctx context.Context, ref contracts.ExecutionRef, inspect bool) (result contracts.ExecutionResult, err error) {
	defer func() {
		if recover() != nil {
			err = errors.New("executor panicked; reconcile before retry")
		}
	}()
	if inspect {
		return w.Executor.Inspect(ctx, ref)
	}
	return w.Executor.Execute(ctx, ref)
}

func (w *Worker) monitor(ctx context.Context, cancel context.CancelFunc, stop <-chan struct{}, job Job, lock *databaseLock) error {
	ticker := time.NewTicker(w.Options.HeartbeatInterval)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return nil
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			checkCtx, done := context.WithTimeout(ctx, w.Options.HeartbeatInterval)
			err := lock.alive(checkCtx)
			if err == nil {
				err = w.Repository.Heartbeat(checkCtx, job, w.Options.LeaseDuration)
			}
			done()
			if err != nil {
				cancel()
				telemetry.Log(ctx, slog.LevelError, "job.ownership_lost", "job_id", job.ID, "database_id", job.DatabaseID)
				return ErrClaimLost
			}
			if reader, ok := w.Executor.(ProgressReader); ok {
				progressCtx, progressDone := context.WithTimeout(ctx, w.Options.HeartbeatInterval)
				progress, readErr := reader.ReadProgress(progressCtx, job.Ref())
				if readErr == nil {
					readErr = w.Repository.UpdateProgress(progressCtx, job, progress)
				}
				progressDone()
				if readErr != nil {
					telemetry.Log(ctx, slog.LevelWarn, "job.progress_unavailable", "job_id", job.ID, "database_id", job.DatabaseID)
				}
			}
		}
	}
}

// resolves job state and retry policy
func (w *Worker) decide(job Job, result contracts.ExecutionResult, err error, inspecting bool) completion {
	valid := result.CommittedBatches >= 0 && result.CommittedRows >= 0
	switch result.State {
	case contracts.ExecutionSucceeded, contracts.ExecutionFailed, contracts.ExecutionPartial, contracts.ExecutionPartiallyReverted, contracts.ExecutionUnknown:
	default:
		valid = false
	}
	switch result.RetryAdvice {
	case contracts.RetryStop, contracts.RetrySafe, contracts.RetryResume, contracts.RetryReconcile:
	default:
		valid = false
	}
	if err != nil || !valid {
		result = contracts.ExecutionResult{State: contracts.ExecutionUnknown, RetryAdvice: contracts.RetryReconcile,
			BlocksDatabase: true, CommittedBatches: job.CommittedBatches, CommittedRows: job.CommittedRows, ErrorCode: "OUTCOME_UNCONFIRMED"}
	}
	if result.State == contracts.ExecutionPartial || result.State == contracts.ExecutionPartiallyReverted {
		result.BlocksDatabase = true
	}
	if result.State == contracts.ExecutionUnknown {
		result.BlocksDatabase = true
		if result.RetryAdvice != contracts.RetryStop {
			result.RetryAdvice = contracts.RetryReconcile
		}
	}
	if result.RetryAdvice == contracts.RetryReconcile {
		result.BlocksDatabase = true
	}
	if len(result.ErrorCode) > 80 || strings.IndexFunc(result.ErrorCode, func(ch rune) bool {
		return !(ch >= 'A' && ch <= 'Z' || ch >= '0' && ch <= '9' || ch == '_')
	}) >= 0 {
		result.ErrorCode = "EXECUTION_ERROR"
	}
	done := completion{Result: result, Status: contracts.JobFailed}
	if result.State == contracts.ExecutionSucceeded && !result.BlocksDatabase {
		done.Status = contracts.JobSucceeded
		return done
	}
	if result.BlocksDatabase {
		done.Status = contracts.JobBlocked
		done.Inspect = true
	}
	switch result.RetryAdvice {
	case contracts.RetryReconcile:
		done.Inspect = true
		if !inspecting || job.AttemptCount < job.MaxAttempts {
			done.Status = contracts.JobRetryWait
		}
	case contracts.RetrySafe, contracts.RetryResume:
		if job.AttemptCount < job.MaxAttempts {
			done.Status = contracts.JobRetryWait
			done.Inspect = false
		}
	}
	if done.Status == contracts.JobRetryWait {
		done.Delay = w.backoff(job.AttemptCount)
	}
	return done
}

// computes jittered exponential retry delay
func (w *Worker) backoff(attempt int) time.Duration {
	delay := w.Options.RetryBaseDelay
	for i := 1; i < attempt && delay < w.Options.RetryMaxDelay; i++ {
		if delay > w.Options.RetryMaxDelay/2 {
			delay = w.Options.RetryMaxDelay
			break
		}
		delay *= 2
	}
	if delay > w.Options.RetryMaxDelay {
		delay = w.Options.RetryMaxDelay
	}
	return delay/2 + time.Duration(randv2.Int64N(int64(delay-delay/2)))
}
