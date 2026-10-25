package explorer

import (
	"context"
	"fmt"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/models"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/sqlguard"
	"strings"

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

type QueryExplainer interface {
	ExplainQuery(ctx context.Context, databaseID string, input api.ExplainQueryRequest) (api.ExplainResult, error)
}

// ExplainQuery returns a plan for one read-only query.
func (s *Service) ExplainQuery(ctx context.Context, actor auth.Principal, databaseID string, input api.ExplainQueryRequest) (api.ExplainResult, error) {
	if err := s.Access.RequireDatabase(ctx, actor, databaseID, api.AccessViewOnly); err != nil {
		return api.ExplainResult{}, err
	}
	if strings.TrimSpace(input.SQL) == "" {
		return api.ExplainResult{}, fmt.Errorf("%w: sql text cannot be empty", apperror.ErrInvalidInput)
	}
	database, err := s.Access.GetDatabase(ctx, actor, databaseID)
	if err != nil {
		return api.ExplainResult{}, err
	}
	if database.Engine == models.EngineMySQL {
		return api.ExplainResult{}, fmt.Errorf("%w: query plan analysis currently supports PostgreSQL only", apperror.ErrInvalidInput)
	}
	if err := ValidateReadOnlySQL(input.SQL); err != nil {
		return api.ExplainResult{}, err
	}
	explainer, ok := s.Repository.(QueryExplainer)
	if !ok {
		return api.ExplainResult{}, fmt.Errorf("%w: query plan analysis is unavailable", apperror.ErrInternal)
	}
	return explainer.ExplainQuery(ctx, databaseID, input)
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
	return s.Access.ListDatabases(ctx, actor)
}

// GetDatabase returns a visible database summary.
func (s *Service) GetDatabase(ctx context.Context, actor auth.Principal, databaseID string) (api.UserDatabase, error) {
	return s.Access.GetDatabase(ctx, actor, databaseID)
}

// ListSchemas lists schemas after checking target access.
func (s *Service) ListSchemas(ctx context.Context, actor auth.Principal, databaseID string) ([]api.SchemaSummary, error) {
	if err := s.Access.RequireDatabase(ctx, actor, databaseID, api.AccessViewOnly); err != nil {
		return nil, err
	}
	return s.Repository.ListSchemas(ctx, databaseID)
}

// ListTables lists tables within a visible schema.
func (s *Service) ListTables(ctx context.Context, actor auth.Principal, databaseID, schema string) ([]api.TableSummary, error) {
	if err := s.Access.RequireDatabase(ctx, actor, databaseID, api.AccessViewOnly); err != nil {
		return nil, err
	}
	if strings.TrimSpace(schema) == "" {
		return nil, fmt.Errorf("%w: schema cannot be empty", apperror.ErrInvalidInput)
	}
	return s.Repository.ListTables(ctx, databaseID, schema)
}

// GetTableDetail returns live table structure.
func (s *Service) GetTableDetail(ctx context.Context, actor auth.Principal, databaseID, schema, table string) (api.TableDetail, error) {
	if err := s.Access.RequireDatabase(ctx, actor, databaseID, api.AccessViewOnly); err != nil {
		return api.TableDetail{}, err
	}
	if strings.TrimSpace(schema) == "" || strings.TrimSpace(table) == "" {
		return api.TableDetail{}, fmt.Errorf("%w: schema and table cannot be empty", apperror.ErrInvalidInput)
	}
	return s.Repository.GetTableDetail(ctx, databaseID, schema, table)
}

// GetTableRows returns a bounded page of visible data.
func (s *Service) GetTableRows(ctx context.Context, actor auth.Principal, databaseID, schema, table string, limit, offset int) (api.TableRows, error) {
	if err := s.Access.RequireDatabase(ctx, actor, databaseID, api.AccessViewOnly); err != nil {
		return api.TableRows{}, err
	}
	if strings.TrimSpace(schema) == "" || strings.TrimSpace(table) == "" {
		return api.TableRows{}, fmt.Errorf("%w: schema and table cannot be empty", apperror.ErrInvalidInput)
	}

	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100 // Hard cap on rows to prevent memory exhaustion
	}
	if offset < 0 {
		return api.TableRows{}, fmt.Errorf("%w: offset cannot be negative", apperror.ErrInvalidInput)
	}

	return s.Repository.GetTableRows(ctx, databaseID, schema, table, limit, offset)
}

// RunQuery runs a supported read only query.
func (s *Service) RunQuery(ctx context.Context, actor auth.Principal, databaseID string, input api.RunQueryRequest) (api.QueryResult, error) {
	if err := s.Access.RequireDatabase(ctx, actor, databaseID, api.AccessViewOnly); err != nil {
		return api.QueryResult{}, err
	}
	if strings.TrimSpace(input.SQL) == "" {
		return api.QueryResult{}, fmt.Errorf("%w: sql text cannot be empty", apperror.ErrInvalidInput)
	}

	// Determine MaxRows based on client request and platform constraints
	if input.MaxRows <= 0 {
		input.MaxRows = s.QueryMaxRows
	} else if input.MaxRows > s.QueryMaxRows {
		input.MaxRows = s.QueryMaxRows
	}

	// 1. Static validation against AST rules
	database, err := s.Access.GetDatabase(ctx, actor, databaseID)
	if err != nil {
		return api.QueryResult{}, err
	}
	if database.Engine == models.EngineMySQL {
		err = sqlguard.ValidateMySQLQuery(input.SQL, database.DatabaseName)
	} else {
		err = ValidateReadOnlySQL(input.SQL)
	}
	if err != nil {
		return api.QueryResult{}, err
	}

	// 2. Execution wrapped in ReadOnly Tx
	return s.Repository.RunQuery(ctx, databaseID, input)
}
