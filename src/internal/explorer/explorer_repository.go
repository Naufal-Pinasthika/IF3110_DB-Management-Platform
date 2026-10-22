package explorer

import (
	"context"
	"time"

	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/api"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/apperror"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/store"
)

type PostgresRepository struct {
	Targets      *store.TargetStore
	QueryTimeout time.Duration
}

var _ Repository = (*PostgresRepository)(nil)

// ListSchemas reads visible schemas from the target.
func (r *PostgresRepository) ListSchemas(ctx context.Context, databaseID string) ([]api.SchemaSummary, error) {
	// TODO Use the reader connection and query information_schema or pg_catalog.
	// TODO Exclude internal schemas according to a documented visibility policy.
	return nil, apperror.ErrNotImplemented
}

// ListTables reads tables in a selected schema.
func (r *PostgresRepository) ListTables(ctx context.Context, databaseID, schema string) ([]api.TableSummary, error) {
	// TODO Filter live PostgreSQL catalogs by a bound schema name and reader privileges.
	return nil, apperror.ErrNotImplemented
}

// GetTableDetail reads columns and constraints and indexes.
func (r *PostgresRepository) GetTableDetail(ctx context.Context, databaseID, schema, table string) (api.TableDetail, error) {
	// TODO Query live catalogs for types and nullability and key columns and index uniqueness.
	// TODO Preserve column order within composite keys and indexes and return a missing object error.
	return api.TableDetail{}, apperror.ErrNotImplemented
}

// GetTableRows reads one page of table data.
func (r *PostgresRepository) GetTableRows(ctx context.Context, databaseID, schema, table string, limit, offset int) (api.TableRows, error) {
	// TODO Use a reader role and a read only transaction with statement timeout.
	// TODO Quote schema and table as identifiers with the chosen PostgreSQL driver.
	// TODO Bind limit and offset as values and fetch one extra row for HasMore.
	// TODO Scan dynamic columns into rows and document ordering for tables without a key.
	return api.TableRows{}, apperror.ErrNotImplemented
}

// RunQuery executes an approved query with database enforced restrictions.
func (r *PostgresRepository) RunQuery(ctx context.Context, databaseID string, input api.RunQueryRequest) (api.QueryResult, error) {
	// TODO Open a reader transaction with ReadOnly enabled and statement timeout.
	// TODO Use a role that cannot write or create temporary objects or call unsafe functions.
	// TODO Read at most the requested cap plus one row and return Truncated accurately.
	// TODO Close rows and the transaction on every path and sanitize database errors.
	return api.QueryResult{}, apperror.ErrNotImplemented
}
