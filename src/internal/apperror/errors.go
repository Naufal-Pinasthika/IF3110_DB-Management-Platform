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

// ErrNotFound indicates the requested resource does not exist.
var ErrNotFound = errors.New("resource not found")

// ErrConflict indicates a state mismatch, such as trying to execute an already applied migration
// or concurrently modifying the same resource.
var ErrConflict = errors.New("resource state conflict or already modified")

// ErrInvalidInput indicates the client provided malformed, incomplete, or unsafe data.
var ErrInvalidInput = errors.New("invalid input provided")

// ErrForbidden indicates the authenticated user lacks the required access level, 
// or the target resource (like a database) is disabled.
var ErrForbidden = errors.New("access forbidden or insufficient privileges")

// ErrInternal represents an unexpected platform or infrastructural failure 
// (e.g., encryption failures, connection pool errors, or database lock failures).
var ErrInternal = errors.New("internal server error")

// returned when ciphertext is malformed or authentication fails.
var ErrInvalidCiphertext = errors.New("invalid ciphertext")


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