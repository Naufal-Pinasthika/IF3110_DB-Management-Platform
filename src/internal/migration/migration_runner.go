package migration

import (
	"context"
	"time"

	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/apperror"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/store"
)

type Runner struct {
	Targets *store.TargetStore
	Timeout time.Duration
}

var _ SQLRunner = (*Runner)(nil)

// Execute runs every migration statement in one target transaction.
func (r *Runner) Execute(ctx context.Context, databaseID, sqlText string) error {
	// TODO Open a migrator connection only after the service has checked VIEW_EDIT.
	// TODO Reject user transaction control and statements incompatible with a transaction.
	// TODO Reject CREATE INDEX CONCURRENTLY and database creation for this milestone.
	// TODO Use PostgreSQL aware parsing rather than splitting strings on semicolons.
	// TODO Begin one transaction and set a timeout and execute every supported statement.
	// TODO Roll back all changes on failure and commit only after every statement succeeds.
	// TODO Return nil only after COMMIT succeeds and return normal errors after confirmed rollback.
	// TODO Wrap ErrTransactionOutcomeUnknown if commit or rollback cannot be confirmed.
	// TODO Never change metadata status here because target and metadata use separate databases.
	return apperror.ErrNotImplemented
}
