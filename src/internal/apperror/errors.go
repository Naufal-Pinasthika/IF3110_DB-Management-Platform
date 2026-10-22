package apperror

import "errors"

// ErrNotImplemented marks unfinished logic so a stub cannot report success.
var ErrNotImplemented = errors.New("business logic is not implemented")

// ErrTransactionOutcomeUnknown means the target may have committed.
// Keep the migration RUNNING and investigate before trying another migration.
var ErrTransactionOutcomeUnknown = errors.New("target transaction outcome is unknown")
