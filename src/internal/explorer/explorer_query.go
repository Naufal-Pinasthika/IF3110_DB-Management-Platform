package explorer

import (
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/apperror"
)

// ValidateReadOnlySQL accepts only the documented read query subset.
func ValidateReadOnlySQL(sqlText string) error {
	// TODO Use PostgreSQL aware parsing and allow exactly one supported SELECT statement.
	// TODO Reject writes and writable CTEs and SELECT INTO and locking and unsafe functions.
	// TODO Handle comments and quoted strings without splitting on raw semicolons.
	// TODO Do not rely on a SELECT prefix check or a keyword blacklist.
	return apperror.ErrNotImplemented
}
