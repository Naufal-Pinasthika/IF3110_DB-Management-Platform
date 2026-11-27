package jobs

import (
	"context"
	"errors"
	"log/slog"
	"strings"

	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/api"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/apperror"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/auth"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/contracts"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/telemetry"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"gorm.io/gorm"
)

type AccessChecker interface {
	RequireDatabase(context.Context, auth.Principal, string, api.AccessLevel) error
}

type Service struct {
	Repository *PostgresRepository
	Registry   ExecutionRegistry
	Access     AccessChecker
	Options    Options
}

// binds queue and registry to tx
func (s *Service) WithTransaction(tx *gorm.DB, registry ExecutionRegistry) *Service {
	copy := *s
	copy.Repository = &PostgresRepository{DB: tx}
	copy.Registry = registry
	return &copy
}

func validID(id string) bool {
	if len(id) != 36 {
		return false
	}
	for i, ch := range id {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			if ch != '-' {
				return false
			}
			continue
		}
		if !strings.ContainsRune("0123456789abcdefABCDEF", ch) {
			return false
		}
	}
	return id != "00000000-0000-0000-0000-000000000000"
}

func normalizeRef(ref contracts.ExecutionRef) contracts.ExecutionRef {
	ref.ExecutionID = strings.ToLower(ref.ExecutionID)
	ref.DatabaseID = strings.ToLower(ref.DatabaseID)
	return ref
}

func (s *Service) Enqueue(ctx context.Context, ref contracts.ExecutionRef) (receipt contracts.JobReceipt, err error) {
	ctx, span := otel.Tracer("platform/jobs").Start(ctx, "queue.enqueue", trace.WithSpanKind(trace.SpanKindProducer))
	defer span.End()
	if !validID(ref.ExecutionID) || !validID(ref.DatabaseID) {
		return receipt, apperror.ErrInvalidInput
	}
	ref = normalizeRef(ref)
	span.SetAttributes(attribute.String("execution.id", ref.ExecutionID), attribute.String("database.id", ref.DatabaseID))
	existing, lookupErr := s.Repository.GetByExecution(ctx, ref.ExecutionID)
	if lookupErr == nil {
		if existing.DatabaseID != ref.DatabaseID {
			return receipt, apperror.ErrConflict
		}
		return existing.Receipt(), nil
	}
	if !errors.Is(lookupErr, apperror.ErrNotFound) {
		return receipt, lookupErr
	}
	if s.Registry == nil {
		return receipt, ErrUnavailable
	}
	metadata, err := s.Registry.ValidateExecution(ctx, ref)
	if err != nil {
		return receipt, err
	}
	if !validID(metadata.ActorID) {
		return receipt, apperror.ErrInvalidInput
	}
	metadata.ActorID = strings.ToLower(metadata.ActorID)
	ctx = telemetry.Correlate(ctx, metadata.ActorID, ref.DatabaseID, "", ref.ExecutionID)
	span.SetAttributes(attribute.String("actor.id", metadata.ActorID))
	if metadata.ReplaceBlockedExecutionID != "" {
		if !validID(metadata.ReplaceBlockedExecutionID) || strings.EqualFold(metadata.ReplaceBlockedExecutionID, ref.ExecutionID) {
			return receipt, apperror.ErrInvalidInput
		}
		metadata.ReplaceBlockedExecutionID = strings.ToLower(metadata.ReplaceBlockedExecutionID)
	}
	if err := s.Options.Validate(); err != nil {
		return receipt, err
	}
	job, err := s.Repository.Enqueue(ctx, ref, metadata, s.Options.MaxAttempts, telemetry.Carrier(ctx))
	if err != nil {
		span.SetStatus(codes.Error, "enqueue failed")
		return receipt, err
	}
	span.SetAttributes(attribute.String("job.id", job.ID))
	telemetry.IncJobEnqueued()
	telemetry.Log(ctx, slog.LevelInfo, "job.enqueued", "job_id", job.ID, "execution_id", job.ExecutionID,
		"database_id", job.DatabaseID, "status", job.Status)
	return job.Receipt(), nil
}

func (s *Service) require(ctx context.Context, actor auth.Principal, databaseID string, level api.AccessLevel) error {
	if !validID(databaseID) {
		return apperror.ErrInvalidInput
	}
	if s.Access == nil {
		return ErrUnavailable
	}
	return s.Access.RequireDatabase(ctx, actor, databaseID, level)
}

func (s *Service) Get(ctx context.Context, actor auth.Principal, databaseID, jobID string) (Job, error) {
	ctx = telemetry.Correlate(ctx, actor.User.ID, databaseID, jobID, "")
	if err := s.require(ctx, actor, databaseID, api.AccessViewOnly); err != nil {
		return Job{}, err
	}
	if !validID(jobID) {
		return Job{}, apperror.ErrInvalidInput
	}
	return s.Repository.Get(ctx, databaseID, jobID)
}

func (s *Service) List(ctx context.Context, actor auth.Principal, databaseID string, options ListOptions) (Page, error) {
	ctx = telemetry.Correlate(ctx, actor.User.ID, databaseID, "", "")
	if err := s.require(ctx, actor, databaseID, api.AccessViewOnly); err != nil {
		return Page{}, err
	}
	if options.Limit == 0 {
		options.Limit = 50
	}
	if options.Limit < 1 || options.Limit > 100 || options.Before < 0 {
		return Page{}, apperror.ErrInvalidInput
	}
	switch options.Status {
	case "", contracts.JobPending, contracts.JobRunning, contracts.JobRetryWait, contracts.JobSucceeded, contracts.JobFailed, contracts.JobBlocked:
	default:
		return Page{}, apperror.ErrInvalidInput
	}
	options.Limit++
	rows, err := s.Repository.List(ctx, databaseID, options)
	if err != nil {
		return Page{}, err
	}
	page := Page{Jobs: rows}
	if len(rows) == options.Limit {
		page.Jobs = rows[:len(rows)-1]
		page.NextBefore = page.Jobs[len(page.Jobs)-1].Sequence
	}
	return page, nil
}
