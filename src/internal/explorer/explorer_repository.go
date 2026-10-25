package explorer

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/models"
	"strings"
	"time"

	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/api"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/apperror"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/store"
	"github.com/lib/pq"
)

type TargetRepository struct {
	Targets      *store.TargetStore
	QueryTimeout time.Duration
}

var _ Repository = (*TargetRepository)(nil)

// ListSchemas reads visible schemas from the target.
func (r *TargetRepository) ListSchemas(ctx context.Context, databaseID string) ([]api.SchemaSummary, error) {
	config, err := r.Targets.Configuration(ctx, databaseID)
	if err != nil {
		return nil, err
	}
	if config.TargetEngine() == models.EngineMySQL {
		return r.mysqlSchemas(ctx, config)
	}
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
func (r *TargetRepository) ListTables(ctx context.Context, databaseID, schema string) ([]api.TableSummary, error) {
	config, err := r.Targets.Configuration(ctx, databaseID)
	if err != nil {
		return nil, err
	}
	if config.TargetEngine() == models.EngineMySQL {
		return r.mysqlTables(ctx, config, schema)
	}
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
func (r *TargetRepository) GetTableDetail(ctx context.Context, databaseID, schema, table string) (api.TableDetail, error) {
	config, err := r.Targets.Configuration(ctx, databaseID)
	if err != nil {
		return api.TableDetail{}, err
	}
	if config.TargetEngine() == models.EngineMySQL {
		return r.mysqlTableDetail(ctx, config, schema, table)
	}
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
func (r *TargetRepository) GetTableRows(ctx context.Context, databaseID, schema, table string, limit, offset int) (api.TableRows, error) {
	config, err := r.Targets.Configuration(ctx, databaseID)
	if err != nil {
		return api.TableRows{}, err
	}
	if config.TargetEngine() == models.EngineMySQL && schema != config.DatabaseName {
		return api.TableRows{}, apperror.ErrForbidden
	}
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
	quote, placeholders := `"`, "$1 OFFSET $2"
	if config.TargetEngine() == models.EngineMySQL {
		quote, placeholders = "`", "? OFFSET ?"
	}
	identifier := func(name string) string { return quote + strings.ReplaceAll(name, quote, quote+quote) + quote }
	query := fmt.Sprintf("SELECT * FROM %s.%s LIMIT %s", identifier(schema), identifier(table), placeholders)
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
func (r *TargetRepository) RunQuery(ctx context.Context, databaseID string, input api.RunQueryRequest) (api.QueryResult, error) {
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

// ExplainQuery runs EXPLAIN (FORMAT JSON [, ANALYZE]) on the given SQL within a ReadOnly transaction.
// For EXPLAIN ANALYZE the transaction is always rolled back so no data is modified.
func (r *TargetRepository) ExplainQuery(ctx context.Context, databaseID string, input api.ExplainQueryRequest) (api.ExplainResult, error) {
	db, err := r.Targets.Reader(ctx, databaseID)
	if err != nil {
		return api.ExplainResult{}, err
	}

	txCtx, cancel := context.WithTimeout(ctx, r.QueryTimeout)
	defer cancel()

	// Always use ReadOnly transaction – EXPLAIN ANALYZE still needs to execute the plan
	// but a ReadOnly tx prevents any data-changing side effects.
	tx, err := db.BeginTx(txCtx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return api.ExplainResult{}, fmt.Errorf("%w: %v", apperror.ErrInternal, err)
	}
	defer tx.Rollback() // always rollback – we never commit an EXPLAIN

	var explainSQL string
	if input.Analyze {
		explainSQL = "EXPLAIN (FORMAT JSON, ANALYZE, BUFFERS) " + input.SQL
	} else {
		explainSQL = "EXPLAIN (FORMAT JSON) " + input.SQL
	}

	rows, err := tx.QueryContext(txCtx, explainSQL)
	if err != nil {
		return api.ExplainResult{}, fmt.Errorf("%w: %v", apperror.ErrInvalidInput, err)
	}
	defer rows.Close()

	// PostgreSQL returns the JSON plan as a single text column with one row
	var rawJSON string
	if rows.Next() {
		if err := rows.Scan(&rawJSON); err != nil {
			return api.ExplainResult{}, fmt.Errorf("%w: failed to scan explain output: %v", apperror.ErrInternal, err)
		}
	}
	if err := rows.Err(); err != nil {
		return api.ExplainResult{}, fmt.Errorf("%w: %v", apperror.ErrInternal, err)
	}

	// rawJSON is an array of plan objects: [{"Plan": {...}, "Planning Time": 1.2, ...}]
	var rawPlans []map[string]json.RawMessage
	if err := json.Unmarshal([]byte(rawJSON), &rawPlans); err != nil {
		return api.ExplainResult{}, fmt.Errorf("%w: failed to parse explain JSON: %v", apperror.ErrInternal, err)
	}

	plans := make([]api.ExplainPlan, 0, len(rawPlans))
	for _, raw := range rawPlans {
		plan := api.ExplainPlan{Analyzed: input.Analyze}

		if planRaw, ok := raw["Plan"]; ok {
			if err := json.Unmarshal(planRaw, &plan.Plan); err != nil {
				return api.ExplainResult{}, fmt.Errorf("%w: invalid plan node: %v", apperror.ErrInternal, err)
			}
		} else {
			return api.ExplainResult{}, fmt.Errorf("%w: database returned no plan node", apperror.ErrInternal)
		}
		if pt, ok := raw["Planning Time"]; ok {
			var v float64
			if json.Unmarshal(pt, &v) == nil {
				plan.PlanningTime = &v
			}
		}
		if et, ok := raw["Execution Time"]; ok {
			var v float64
			if json.Unmarshal(et, &v) == nil {
				plan.ExecutionTime = &v
			}
		}
		plan.TotalCost = plan.Plan.TotalCost
		plans = append(plans, plan)
	}

	// Also produce a human-readable plain text EXPLAIN for reference
	rawText := buildPlanText(plans)

	return api.ExplainResult{
		Plans:    plans,
		RawText:  rawText,
		Analyzed: input.Analyze,
	}, nil
}

// ExplainSQL runs EXPLAIN on migration DML after syntax-aware validation.
// The outer transaction is always rolled back after the preview.
func (r *TargetRepository) ExplainSQL(ctx context.Context, databaseID string, sqlText string, analyze bool) (api.ExplainResult, error) {
	db, err := r.Targets.Migrator(ctx, databaseID)
	if err != nil {
		return api.ExplainResult{}, err
	}

	txCtx, cancel := context.WithTimeout(ctx, r.QueryTimeout)
	defer cancel()

	// EXPLAIN ANALYZE executes DML, so isolate it in a transaction and always roll back.
	tx, err := db.BeginTx(txCtx, nil)
	if err != nil {
		return api.ExplainResult{}, fmt.Errorf("%w: %v", apperror.ErrInternal, err)
	}
	defer tx.Rollback()

	// Build each statement's EXPLAIN individually (migration may have multiple statements)
	statements := splitStatements(sqlText)
	if len(statements) == 0 {
		return api.ExplainResult{}, fmt.Errorf("%w: no SQL statements found", apperror.ErrInvalidInput)
	}

	var allPlans []api.ExplainPlan

	for i, stmt := range statements {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}
		savepoint := fmt.Sprintf("plan_stmt_%d", i+1)
		if _, err := tx.ExecContext(txCtx, "SAVEPOINT "+savepoint); err != nil {
			return api.ExplainResult{}, fmt.Errorf("%w: could not isolate statement plan: %v", apperror.ErrInternal, err)
		}

		var explainSQL string
		if analyze {
			explainSQL = "EXPLAIN (FORMAT JSON, ANALYZE, BUFFERS) " + stmt
		} else {
			explainSQL = "EXPLAIN (FORMAT JSON) " + stmt
		}

		rows, err := tx.QueryContext(txCtx, explainSQL)
		if err != nil {
			// Some PostgreSQL commands do not have a plan (for example CREATE TABLE).
			allPlans = append(allPlans, api.ExplainPlan{Analyzed: analyze, StatementIndex: i + 1, Error: err.Error()})
			_, _ = tx.ExecContext(txCtx, "ROLLBACK TO SAVEPOINT "+savepoint)
			_, _ = tx.ExecContext(txCtx, "RELEASE SAVEPOINT "+savepoint)
			continue
		}

		var rawJSON string
		if rows.Next() {
			_ = rows.Scan(&rawJSON)
		}
		rows.Close()
		if _, err := tx.ExecContext(txCtx, "RELEASE SAVEPOINT "+savepoint); err != nil {
			return api.ExplainResult{}, fmt.Errorf("%w: could not finish statement plan: %v", apperror.ErrInternal, err)
		}

		var rawPlans []map[string]json.RawMessage
		if err := json.Unmarshal([]byte(rawJSON), &rawPlans); err != nil {
			allPlans = append(allPlans, api.ExplainPlan{Analyzed: analyze, StatementIndex: i + 1, Error: "could not parse database plan output"})
			continue
		}

		for _, raw := range rawPlans {
			plan := api.ExplainPlan{Analyzed: analyze, StatementIndex: i + 1}
			if planRaw, ok := raw["Plan"]; ok {
				if err := json.Unmarshal(planRaw, &plan.Plan); err != nil {
					plan.Error = "could not decode database plan output"
					allPlans = append(allPlans, plan)
					continue
				}
			} else {
				plan.Error = "database returned no plan node"
				allPlans = append(allPlans, plan)
				continue
			}
			if pt, ok := raw["Planning Time"]; ok {
				var v float64
				if json.Unmarshal(pt, &v) == nil {
					plan.PlanningTime = &v
				}
			}
			if et, ok := raw["Execution Time"]; ok {
				var v float64
				if json.Unmarshal(et, &v) == nil {
					plan.ExecutionTime = &v
				}
			}
			plan.TotalCost = plan.Plan.TotalCost
			allPlans = append(allPlans, plan)
		}
	}

	rawText := buildPlanText(allPlans)
	return api.ExplainResult{
		Plans:    allPlans,
		RawText:  rawText,
		Analyzed: analyze,
	}, nil
}

// splitStatements naively splits a SQL script on semicolons (good enough for migration EXPLAIN).
func splitStatements(sql string) []string {
	parts := strings.Split(sql, ";")
	var out []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// buildPlanText produces a simple text summary from parsed plan objects.
func buildPlanText(plans []api.ExplainPlan) string {
	if len(plans) == 0 {
		return ""
	}
	var sb strings.Builder
	for i, p := range plans {
		if i > 0 {
			sb.WriteString("\n\n")
		}
		if p.StatementIndex > 0 || len(plans) > 1 {
			statementIndex := p.StatementIndex
			if statementIndex == 0 {
				statementIndex = i + 1
			}
			fmt.Fprintf(&sb, "-- Statement %d\n", statementIndex)
		}
		if p.Error != "" {
			fmt.Fprintf(&sb, "Plan unavailable: %s", p.Error)
			continue
		}
		renderNode(&sb, p.Plan, 0)
		if p.PlanningTime != nil {
			fmt.Fprintf(&sb, "\nPlanning Time: %.3f ms", *p.PlanningTime)
		}
		if p.ExecutionTime != nil {
			fmt.Fprintf(&sb, "\nExecution Time: %.3f ms", *p.ExecutionTime)
		}
	}
	return sb.String()
}

func renderNode(sb *strings.Builder, node api.ExplainNode, depth int) {
	indent := strings.Repeat("  ", depth)
	arrow := "->"
	if depth == 0 {
		arrow = " "
	}

	label := node.NodeType
	if node.RelationName != "" {
		label += " on " + node.RelationName
		if node.Alias != "" && node.Alias != node.RelationName {
			label += " " + node.Alias
		}
	}
	if node.IndexName != "" {
		label += " using " + node.IndexName
	}

	cost := fmt.Sprintf("(cost=%.2f..%.2f rows=%d width=%d)", node.StartupCost, node.TotalCost, node.PlanRows, node.PlanWidth)
	line := fmt.Sprintf("%s%s %s  %s", indent, arrow, label, cost)

	if node.ActualTotalTime != nil && node.ActualRows != nil {
		actual := fmt.Sprintf("(actual time=%.3f..%.3f rows=%g loops=%d)",
			*node.ActualStartupTime, *node.ActualTotalTime, *node.ActualRows, *node.ActualLoops)
		line += "  " + actual
	}

	sb.WriteString(line + "\n")
	for _, child := range node.Plans {
		renderNode(sb, child, depth+1)
	}
}
