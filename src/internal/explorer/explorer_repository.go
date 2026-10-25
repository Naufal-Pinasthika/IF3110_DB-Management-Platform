package explorer

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/api"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/apperror"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/store"
	"github.com/lib/pq"
)


type PostgresRepository struct {
	Targets      *store.TargetStore
	QueryTimeout time.Duration
}


var _ Repository = (*PostgresRepository)(nil)


// ListSchemas reads visible schemas from the target.
func (r *PostgresRepository) ListSchemas(ctx context.Context, databaseID string) ([]api.SchemaSummary, error) {
	db, err := r.Targets.Reader(ctx, databaseID)
	if err != nil {
		return nil, err
	}

	query := `
		SELECT schema_name 
		FROM information_schema.schemata 
		WHERE schema_name NOT IN ('information_schema', 'pg_catalog') 
		  AND schema_name NOT LIKE 'pg_toast%'
		ORDER BY schema_name;
	`
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to fetch schemas: %v", apperror.ErrInternal, err)
	}
	defer rows.Close()

	var schemas []api.SchemaSummary
	for rows.Next() {
		var s api.SchemaSummary
		if err := rows.Scan(&s.Name); err != nil {
			return nil, err
		}
		schemas = append(schemas, s)
	}
	return schemas, rows.Err()
}


// ListTables reads tables in a selected schema.
func (r *PostgresRepository) ListTables(ctx context.Context, databaseID, schema string) ([]api.TableSummary, error) {
	db, err := r.Targets.Reader(ctx, databaseID)
	if err != nil {
		return nil, err
	}

	query := `
		SELECT table_name 
		FROM information_schema.tables 
		WHERE table_schema = $1 AND table_type = 'BASE TABLE'
		ORDER BY table_name;
	`
	rows, err := db.QueryContext(ctx, query, schema)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to fetch tables: %v", apperror.ErrInternal, err)
	}
	defer rows.Close()

	var tables []api.TableSummary
	for rows.Next() {
		var t api.TableSummary
		if err := rows.Scan(&t.Name); err != nil {
			return nil, err
		}
		tables = append(tables, t)
	}
	return tables, rows.Err()
}


// GetTableDetail reads columns and constraints and indexes.
func (r *PostgresRepository) GetTableDetail(ctx context.Context, databaseID, schema, table string) (api.TableDetail, error) {
	db, err := r.Targets.Reader(ctx, databaseID)
	if err != nil {
		return api.TableDetail{}, err
	}

	detail := api.TableDetail{
		Schema:      schema,
		Table:       table,
		Columns:     []api.TableColumn{},
		Constraints: []api.TableConstraint{},
		Indexes:     []api.TableIndex{},
	}

	// 1. Columns
	colQuery := `
		SELECT column_name, data_type, is_nullable 
		FROM information_schema.columns 
		WHERE table_schema = $1 AND table_name = $2
		ORDER BY ordinal_position;
	`
	cRows, err := db.QueryContext(ctx, colQuery, schema, table)
	if err != nil {
		return detail, fmt.Errorf("%w: failed to fetch columns: %v", apperror.ErrInternal, err)
	}
	defer cRows.Close()

	for cRows.Next() {
		var col api.TableColumn
		var isNullableStr string
		if err := cRows.Scan(&col.Name, &col.DataType, &isNullableStr); err != nil {
			return detail, err
		}
		col.Nullable = (isNullableStr == "YES")
		detail.Columns = append(detail.Columns, col)
	}
	if len(detail.Columns) == 0 {
		return detail, apperror.ErrNotFound // Table doesn't exist
	}

	// 2. Constraints (simplified query)
	conQuery := `
		SELECT tc.constraint_name, tc.constraint_type, array_agg(kcu.column_name::text)
		FROM information_schema.table_constraints tc
		JOIN information_schema.key_column_usage kcu 
		  ON tc.constraint_name = kcu.constraint_name AND tc.table_schema = kcu.table_schema
		WHERE tc.table_schema = $1 AND tc.table_name = $2
		GROUP BY tc.constraint_name, tc.constraint_type;
	`
	conRows, err := db.QueryContext(ctx, conQuery, schema, table)
	if err == nil {
		defer conRows.Close()
		for conRows.Next() {
			var con api.TableConstraint
			if err := conRows.Scan(&con.Name, &con.Type, (*pq.StringArray)(&con.Columns)); err == nil {
				detail.Constraints = append(detail.Constraints, con)
			}
		}
	}

	// 3. Indexes (simplified query)
	idxQuery := `
		SELECT i.relname AS index_name, ix.indisunique, array_agg(a.attname::text)
		FROM pg_class t
		JOIN pg_index ix ON t.oid = ix.indrelid
		JOIN pg_class i ON i.oid = ix.indexrelid
		JOIN pg_attribute a ON a.attrelid = t.oid AND a.attnum = ANY(ix.indkey)
		JOIN pg_namespace n ON n.oid = t.relnamespace
		WHERE n.nspname = $1 AND t.relname = $2
		GROUP BY i.relname, ix.indisunique;
	`
	idxRows, err := db.QueryContext(ctx, idxQuery, schema, table)
	if err == nil {
		defer idxRows.Close()
		for idxRows.Next() {
			var idx api.TableIndex
			if err := idxRows.Scan(&idx.Name, &idx.IsUnique, (*pq.StringArray)(&idx.Columns)); err == nil {
				detail.Indexes = append(detail.Indexes, idx)
			}
		}
	}

	return detail, nil
}


// GetTableRows reads one page of table data.
func (r *PostgresRepository) GetTableRows(ctx context.Context, databaseID, schema, table string, limit, offset int) (api.TableRows, error) {
	db, err := r.Targets.Reader(ctx, databaseID)
	if err != nil {
		return api.TableRows{}, err
	}

	txCtx, cancel := context.WithTimeout(ctx, r.QueryTimeout)
	defer cancel()

	// Using ReadOnly transaction
	tx, err := db.BeginTx(txCtx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return api.TableRows{}, fmt.Errorf("%w: failed to start transaction: %v", apperror.ErrInternal, err)
	}
	defer tx.Rollback()

	// Quote identifiers and fetch limit+1 to determine HasMore
	query := fmt.Sprintf(`SELECT * FROM %q.%q LIMIT $1 OFFSET $2`, schema, table)
	rows, err := tx.QueryContext(txCtx, query, limit+1, offset)
	if err != nil {
		return api.TableRows{}, fmt.Errorf("%w: query failed: %v", apperror.ErrInvalidInput, err)
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return api.TableRows{}, err
	}

	result := api.TableRows{
		Columns: cols,
		Rows:    [][]any{},
		Limit:   limit,
		Offset:  offset,
		HasMore: false,
	}

	count := 0
	for rows.Next() {
		count++
		if count > limit {
			result.HasMore = true
			break
		}

		vals := make([]any, len(cols))
		valPtrs := make([]any, len(cols))
		for i := range vals {
			valPtrs[i] = &vals[i]
		}

		if err := rows.Scan(valPtrs...); err != nil {
			return api.TableRows{}, err
		}

		// Convert bytes from db into strings so JSON encoding works properly
		for i, val := range vals {
			if b, ok := val.([]byte); ok {
				vals[i] = string(b)
			}
		}
		result.Rows = append(result.Rows, vals)
	}

	return result, rows.Err()
}


// RunQuery executes an approved query with database enforced restrictions.
func (r *PostgresRepository) RunQuery(ctx context.Context, databaseID string, input api.RunQueryRequest) (api.QueryResult, error) {
	db, err := r.Targets.Reader(ctx, databaseID)
	if err != nil {
		return api.QueryResult{}, err
	}

	txCtx, cancel := context.WithTimeout(ctx, r.QueryTimeout)
	defer cancel()

	// Strict ReadOnly transaction serves as the ultimate barrier against modifications
	tx, err := db.BeginTx(txCtx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return api.QueryResult{}, fmt.Errorf("%w: %v", apperror.ErrInternal, err)
	}
	defer tx.Rollback()

	// Execute user query
	rows, err := tx.QueryContext(txCtx, input.SQL)
	if err != nil {
		return api.QueryResult{}, fmt.Errorf("%w: %v", apperror.ErrInvalidInput, err)
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return api.QueryResult{}, err
	}

	result := api.QueryResult{
		Columns:   cols,
		Rows:      [][]any{},
		RowCount:  0,
		Truncated: false,
	}

	for rows.Next() {
		if result.RowCount >= input.MaxRows {
			result.Truncated = true
			break
		}

		vals := make([]any, len(cols))
		valPtrs := make([]any, len(cols))
		for i := range vals {
			valPtrs[i] = &vals[i]
		}

		if err := rows.Scan(valPtrs...); err != nil {
			return api.QueryResult{}, err
		}

		for i, val := range vals {
			if b, ok := val.([]byte); ok {
				vals[i] = string(b)
			}
		}
		result.Rows = append(result.Rows, vals)
		result.RowCount++
	}

	return result, rows.Err()
}