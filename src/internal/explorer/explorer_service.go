package explorer

import (
	"context"

	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/api"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/apperror"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/auth"
)

type Repository interface {
	ListSchemas(ctx context.Context, databaseID string) ([]api.SchemaSummary, error)
	ListTables(ctx context.Context, databaseID, schema string) ([]api.TableSummary, error)
	GetTableDetail(ctx context.Context, databaseID, schema, table string) (api.TableDetail, error)
	GetTableRows(ctx context.Context, databaseID, schema, table string, limit, offset int) (api.TableRows, error)
	RunQuery(ctx context.Context, databaseID string, input api.RunQueryRequest) (api.QueryResult, error)
}

type AccessChecker interface {
	RequireDatabase(context.Context, auth.Principal, string, api.AccessLevel) error
	ListDatabases(context.Context, auth.Principal) ([]api.UserDatabase, error)
	GetDatabase(context.Context, auth.Principal, string) (api.UserDatabase, error)
}

type Service struct {
	Repository   Repository
	Access       AccessChecker
	QueryMaxRows int
}

// ListDatabases lists databases visible through group grants.
func (s *Service) ListDatabases(ctx context.Context, actor auth.Principal) ([]api.UserDatabase, error) {
	// TODO Use Access.ListDatabases so effective permissions have one implementation.
	return nil, apperror.ErrNotImplemented
}

// GetDatabase returns a visible database summary.
func (s *Service) GetDatabase(ctx context.Context, actor auth.Principal, databaseID string) (api.UserDatabase, error) {
	// TODO Use Access.GetDatabase so an admin flag never bypasses target permissions.
	return api.UserDatabase{}, apperror.ErrNotImplemented
}

// ListSchemas lists schemas after checking target access.
func (s *Service) ListSchemas(ctx context.Context, actor auth.Principal, databaseID string) ([]api.SchemaSummary, error) {
	// TODO Require at least VIEW_ONLY and read current schemas from the target repository.
	return nil, apperror.ErrNotImplemented
}

// ListTables lists tables within a visible schema.
func (s *Service) ListTables(ctx context.Context, actor auth.Principal, databaseID, schema string) ([]api.TableSummary, error) {
	// TODO Require at least VIEW_ONLY and validate the schema name before reading catalogs.
	return nil, apperror.ErrNotImplemented
}

// GetTableDetail returns live table structure.
func (s *Service) GetTableDetail(ctx context.Context, actor auth.Principal, databaseID, schema, table string) (api.TableDetail, error) {
	// TODO Require at least VIEW_ONLY and request columns and constraints and indexes.
	// TODO Keep target schema metadata live rather than copying it into platform tables.
	return api.TableDetail{}, apperror.ErrNotImplemented
}

// GetTableRows returns a bounded page of visible data.
func (s *Service) GetTableRows(ctx context.Context, actor auth.Principal, databaseID, schema, table string, limit, offset int) (api.TableRows, error) {
	// TODO Require at least VIEW_ONLY and validate schema and table names.
	// TODO Use default limit 50 and maximum 100 and reject a negative offset.
	// TODO Call the repository only after validation and permission checks.
	return api.TableRows{}, apperror.ErrNotImplemented
}

// RunQuery runs a supported read only query.
func (s *Service) RunQuery(ctx context.Context, actor auth.Principal, databaseID string, input api.RunQueryRequest) (api.QueryResult, error) {
	// TODO Require at least VIEW_ONLY and validate nonempty SQL and MaxRows against the configured cap.
	// TODO Call ValidateReadOnlySQL before using the reader transaction.
	// TODO Keep role restrictions and transaction restrictions even after SQL validation.
	return api.QueryResult{}, apperror.ErrNotImplemented
}
