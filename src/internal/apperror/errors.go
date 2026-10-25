package apperror

import (
	"errors"
	"fmt"
)

// ErrNotImplemented marks unfinished logic so a stub cannot report success.
var ErrNotImplemented = errors.New("business logic is not implemented")

// ErrTransactionOutcomeUnknown means the target may have committed.
// Keep the migration RUNNING and investigate before trying another migration.
var ErrTransactionOutcomeUnknown = errors.New("target transaction outcome is unknown")

// New creates a new error with the given message.
func New(message string) error {
	return errors.New(message)
}

// Wrap wraps an error with additional context.
func Wrap(err error, message string) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", message, err)
}
