package explorer

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/api"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/apperror"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/models"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/store"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/telemetry"
	"github.com/lib/pq"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

func startQueryTrace(ctx context.Context, databaseID, operation, query string, params ...any) (context.Context, func(rows int64, err error)) {
	ctx = telemetry.WithDatabase(ctx, databaseID)
	ctx, span := otel.Tracer("platform/explorer").Start(ctx, "db.query")
	span.SetAttributes(
		attribute.String("database.id", databaseID),
		attribute.String("db.operation.name", operation),
		attribute.Bool("db.read_only", true),
	)
	redactedParams := telemetry.RedactParams(params...)
	started := time.Now()
	telemetry.Log(ctx, slog.LevelInfo, "sql.started",
		"operation", operation,
		"read_only", true,
		"query", query,
		"parameters", redactedParams,
		"status", "started",
	)
	return ctx, func(rows int64, err error) {
		durMS := float64(time.Since(started)) / float64(time.Millisecond)
		status := "success"
		level := slog.LevelInfo
		errMsg := ""
		sqlState := ""
		if err != nil {
			status = "failed"
			level = slog.LevelError
			errMsg = telemetry.SanitizeValue(err.Error())
			sqlState = telemetry.SQLState(err)
			span.RecordError(err)
			span.SetStatus(codes.Error, "read-only query failed")
		}
		span.SetAttributes(attribute.Int64("db.rows", rows))
		span.End()
		telemetry.Log(ctx, level, "sql.finished",
			"operation", operation,
			"read_only", true,
			"query", query,
			"parameters", redactedParams,
			"duration_ms", durMS,
			"rows", rows,
			"status", status,
			"error", errMsg,
			"sqlstate", sqlState,
		)
	}
}

func rollbackReadOnlyTx(ctx context.Context, tx *sql.Tx, operation string) {
	err := tx.Rollback()
	if err != nil && err != sql.ErrTxDone {
		telemetry.Log(ctx, slog.LevelWarn, "sql.transaction_rollback",
			"operation", operation,
			"status", "failed",
			"error", telemetry.SanitizeValue(err.Error()),
			"sqlstate", telemetry.SQLState(err),
		)
		return
	}
	telemetry.Log(ctx, slog.LevelInfo, "sql.transaction_rollback",
		"operation", operation,
		"status", "success",
	)
}

type TargetRepository struct {
	Targets      *store.TargetStore
	QueryTimeout time.Duration
}

var _ Repository = (*TargetRepository)(nil)

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
	qCtx, finish := startQueryTrace(ctx, databaseID, "explorer.list_schemas", strings.TrimSpace(query))
	rows, err := db.QueryContext(qCtx, query)
	if err != nil {
		finish(0, err)
		return nil, fmt.Errorf("%w: failed to fetch schemas: %v", apperror.ErrInternal, err)
	}
	defer rows.Close()

	var schemas []api.SchemaSummary
	for rows.Next() {
		var s api.SchemaSummary
		if err := rows.Scan(&s.Name); err != nil {
			finish(int64(len(schemas)), err)
			return nil, err
		}
		schemas = append(schemas, s)
	}
	err = rows.Err()
	finish(int64(len(schemas)), err)
	return schemas, err
}

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
	qCtx, finish := startQueryTrace(ctx, databaseID, "explorer.list_tables", strings.TrimSpace(query), schema)
	rows, err := db.QueryContext(qCtx, query, schema)
	if err != nil {
		finish(0, err)
		return nil, fmt.Errorf("%w: failed to fetch tables: %v", apperror.ErrInternal, err)
	}
	defer rows.Close()

	var tables []api.TableSummary
	for rows.Next() {
		var t api.TableSummary
		if err := rows.Scan(&t.Name); err != nil {
			finish(int64(len(tables)), err)
			return nil, err
		}
		tables = append(tables, t)
	}
	err = rows.Err()
	finish(int64(len(tables)), err)
	return tables, err
}

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

	colQuery := `
		SELECT column_name, data_type, is_nullable 
		FROM information_schema.columns 
		WHERE table_schema = $1 AND table_name = $2
		ORDER BY ordinal_position;
	`
	colCtx, finishCols := startQueryTrace(ctx, databaseID, "explorer.table_columns", strings.TrimSpace(colQuery), schema, table)
	cRows, err := db.QueryContext(colCtx, colQuery, schema, table)
	if err != nil {
		finishCols(0, err)
		return detail, fmt.Errorf("%w: failed to fetch columns: %v", apperror.ErrInternal, err)
	}
	defer cRows.Close()

	for cRows.Next() {
		var col api.TableColumn
		var isNullableStr string
		if err := cRows.Scan(&col.Name, &col.DataType, &isNullableStr); err != nil {
			finishCols(int64(len(detail.Columns)), err)
			return detail, err
		}
		col.Nullable = (isNullableStr == "YES")
		detail.Columns = append(detail.Columns, col)
	}
	finishCols(int64(len(detail.Columns)), cRows.Err())
	if len(detail.Columns) == 0 {
		return detail, apperror.ErrNotFound
	}

	conQuery := `
		SELECT tc.constraint_name, tc.constraint_type, array_agg(kcu.column_name::text)
		FROM information_schema.table_constraints tc
		JOIN information_schema.key_column_usage kcu 
		  ON tc.constraint_name = kcu.constraint_name AND tc.table_schema = kcu.table_schema
		WHERE tc.table_schema = $1 AND tc.table_name = $2
		GROUP BY tc.constraint_name, tc.constraint_type;
	`
	conCtx, finishCons := startQueryTrace(ctx, databaseID, "explorer.table_constraints", strings.TrimSpace(conQuery), schema, table)
	conRows, err := db.QueryContext(conCtx, conQuery, schema, table)
	if err == nil {
		defer conRows.Close()
		for conRows.Next() {
			var con api.TableConstraint
			if err := conRows.Scan(&con.Name, &con.Type, (*pq.StringArray)(&con.Columns)); err == nil {
				detail.Constraints = append(detail.Constraints, con)
			}
		}
		finishCons(int64(len(detail.Constraints)), conRows.Err())
	} else {
		finishCons(0, err)
	}

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
	idxCtx, finishIdx := startQueryTrace(ctx, databaseID, "explorer.table_indexes", strings.TrimSpace(idxQuery), schema, table)
	idxRows, err := db.QueryContext(idxCtx, idxQuery, schema, table)
	if err == nil {
		defer idxRows.Close()
		for idxRows.Next() {
			var idx api.TableIndex
			if err := idxRows.Scan(&idx.Name, &idx.IsUnique, (*pq.StringArray)(&idx.Columns)); err == nil {
				detail.Indexes = append(detail.Indexes, idx)
			}
		}
		finishIdx(int64(len(detail.Indexes)), idxRows.Err())
	} else {
		finishIdx(0, err)
	}

	return detail, nil
}

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

	tx, err := db.BeginTx(txCtx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return api.TableRows{}, fmt.Errorf("%w: failed to start transaction: %v", apperror.ErrInternal, err)
	}
	defer rollbackReadOnlyTx(ctx, tx, "explorer.get_table_rows")

	quote, placeholders := `"`, "$1 OFFSET $2"
	if config.TargetEngine() == models.EngineMySQL {
		quote, placeholders = "`", "? OFFSET ?"
	}
	identifier := func(name string) string { return quote + strings.ReplaceAll(name, quote, quote+quote) + quote }
	query := fmt.Sprintf("SELECT * FROM %s.%s LIMIT %s", identifier(schema), identifier(table), placeholders)
	qCtx, finish := startQueryTrace(txCtx, databaseID, "explorer.get_table_rows", query, limit+1, offset)
	rows, err := tx.QueryContext(qCtx, query, limit+1, offset)
	if err != nil {
		finish(0, err)
		return api.TableRows{}, fmt.Errorf("%w: query failed: %v", apperror.ErrInvalidInput, err)
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		finish(0, err)
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
			finish(int64(len(result.Rows)), err)
			return api.TableRows{}, err
		}

		for i, val := range vals {
			if b, ok := val.([]byte); ok {
				vals[i] = string(b)
			}
		}
		result.Rows = append(result.Rows, vals)
	}

	err = rows.Err()
	finish(int64(len(result.Rows)), err)
	return result, err
}

func (r *TargetRepository) RunQuery(ctx context.Context, databaseID string, input api.RunQueryRequest) (api.QueryResult, error) {
	db, err := r.Targets.Reader(ctx, databaseID)
	if err != nil {
		return api.QueryResult{}, err
	}

	txCtx, cancel := context.WithTimeout(ctx, r.QueryTimeout)
	defer cancel()

	tx, err := db.BeginTx(txCtx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return api.QueryResult{}, fmt.Errorf("%w: %v", apperror.ErrInternal, err)
	}
	defer rollbackReadOnlyTx(ctx, tx, "explorer.run_query")

	qCtx, finish := startQueryTrace(txCtx, databaseID, "explorer.run_query", input.SQL)
	rows, err := tx.QueryContext(qCtx, input.SQL)
	if err != nil {
		finish(0, err)
		return api.QueryResult{}, fmt.Errorf("%w: %v", apperror.ErrInvalidInput, err)
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		finish(0, err)
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
			finish(int64(result.RowCount), err)
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

	err = rows.Err()
	finish(int64(result.RowCount), err)
	return result, err
}

func (r *TargetRepository) ExplainQuery(ctx context.Context, databaseID string, input api.ExplainQueryRequest) (api.ExplainResult, error) {
	db, err := r.Targets.Reader(ctx, databaseID)
	if err != nil {
		return api.ExplainResult{}, err
	}

	txCtx, cancel := context.WithTimeout(ctx, r.QueryTimeout)
	defer cancel()

	tx, err := db.BeginTx(txCtx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return api.ExplainResult{}, fmt.Errorf("%w: %v", apperror.ErrInternal, err)
	}
	defer rollbackReadOnlyTx(ctx, tx, "explorer.explain_query")

	var explainSQL string
	if input.Analyze {
		explainSQL = "EXPLAIN (FORMAT JSON, ANALYZE, BUFFERS) " + input.SQL
	} else {
		explainSQL = "EXPLAIN (FORMAT JSON) " + input.SQL
	}

	qCtx, finish := startQueryTrace(txCtx, databaseID, "explorer.explain_query", explainSQL)
	rows, err := tx.QueryContext(qCtx, explainSQL)
	if err != nil {
		finish(0, err)
		return api.ExplainResult{}, fmt.Errorf("%w: %v", apperror.ErrInvalidInput, err)
	}
	defer rows.Close()

	var rawJSON string
	if rows.Next() {
		if err := rows.Scan(&rawJSON); err != nil {
			finish(0, err)
			return api.ExplainResult{}, fmt.Errorf("%w: failed to scan explain output: %v", apperror.ErrInternal, err)
		}
	}
	if err := rows.Err(); err != nil {
		finish(0, err)
		return api.ExplainResult{}, fmt.Errorf("%w: %v", apperror.ErrInternal, err)
	}
	finish(1, nil)

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

	rawText := buildPlanText(plans)

	return api.ExplainResult{
		Plans:    plans,
		RawText:  rawText,
		Analyzed: input.Analyze,
	}, nil
}

func (r *TargetRepository) ExplainSQL(ctx context.Context, databaseID string, sqlText string, analyze bool) (res api.ExplainResult, retErr error) {
	db, err := r.Targets.Migrator(ctx, databaseID)
	if err != nil {
		return api.ExplainResult{}, err
	}

	txCtx, cancel := context.WithTimeout(ctx, r.QueryTimeout)
	defer cancel()

	tx, err := db.BeginTx(txCtx, nil)
	if err != nil {
		return api.ExplainResult{}, fmt.Errorf("%w: %v", apperror.ErrInternal, err)
	}
	defer rollbackReadOnlyTx(ctx, tx, "explorer.explain_migration")

	statements := splitStatements(sqlText)
	if len(statements) == 0 {
		return api.ExplainResult{}, fmt.Errorf("%w: no SQL statements found", apperror.ErrInvalidInput)
	}

	qCtx, finishExplain := startQueryTrace(txCtx, databaseID, "explorer.explain_migration", sqlText)
	defer func() { finishExplain(int64(len(statements)), retErr) }()
	txCtx = qCtx

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

		if !isExplainableStmt(stmt) {
			start := time.Now()
			_, execErr := tx.ExecContext(txCtx, stmt)
			dur := float64(time.Since(start).Microseconds()) / 1000.0
			_, _ = tx.ExecContext(txCtx, "ROLLBACK TO SAVEPOINT "+savepoint)
			_, _ = tx.ExecContext(txCtx, "RELEASE SAVEPOINT "+savepoint)

			if execErr != nil {
				allPlans = append(allPlans, api.ExplainPlan{
					Analyzed:       analyze,
					StatementIndex: i + 1,
					Error:          execErr.Error(),
				})
			} else {
				allPlans = append(allPlans, api.ExplainPlan{
					Analyzed:       analyze,
					StatementIndex: i + 1,
					ExecutionTime:  &dur,
					Plan: api.ExplainNode{
						NodeType:     "DDL Execution (previewed & rolled back)",
						RelationName: summarizeDDL(stmt),
					},
				})
			}
			continue
		}

		var explainSQL string
		if analyze {
			explainSQL = "EXPLAIN (FORMAT JSON, ANALYZE, BUFFERS) " + stmt
		} else {
			explainSQL = "EXPLAIN (FORMAT JSON) " + stmt
		}

		rows, err := tx.QueryContext(txCtx, explainSQL)
		if err != nil {
			start := time.Now()
			_, execErr := tx.ExecContext(txCtx, stmt)
			dur := float64(time.Since(start).Microseconds()) / 1000.0
			_, _ = tx.ExecContext(txCtx, "ROLLBACK TO SAVEPOINT "+savepoint)
			_, _ = tx.ExecContext(txCtx, "RELEASE SAVEPOINT "+savepoint)

			if execErr == nil {
				allPlans = append(allPlans, api.ExplainPlan{
					Analyzed:       analyze,
					StatementIndex: i + 1,
					ExecutionTime:  &dur,
					Plan: api.ExplainNode{
						NodeType:     "DDL Execution (previewed & rolled back)",
						RelationName: summarizeDDL(stmt),
					},
				})
			} else {
				allPlans = append(allPlans, api.ExplainPlan{Analyzed: analyze, StatementIndex: i + 1, Error: execErr.Error()})
			}
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

func isExplainableStmt(stmt string) bool {
	s := strings.TrimSpace(stmt)
	for {
		if strings.HasPrefix(s, "--") {
			idx := strings.Index(s, "\n")
			if idx == -1 {
				return false
			}
			s = strings.TrimSpace(s[idx+1:])
			continue
		}
		if strings.HasPrefix(s, "/*") {
			idx := strings.Index(s, "*/")
			if idx == -1 {
				return false
			}
			s = strings.TrimSpace(s[idx+2:])
			continue
		}
		break
	}
	fields := strings.Fields(s)
	if len(fields) == 0 {
		return false
	}
	firstWord := strings.ToUpper(fields[0])
	switch firstWord {
	case "SELECT", "INSERT", "UPDATE", "DELETE", "MERGE", "VALUES", "WITH", "DECLARE", "EXECUTE":
		return true
	default:
		return false
	}
}

func summarizeDDL(stmt string) string {
	s := strings.TrimSpace(stmt)
	lines := strings.Split(s, "\n")
	if len(lines) > 0 {
		firstLine := strings.TrimSpace(lines[0])
		firstLine = strings.TrimSuffix(firstLine, "(")
		firstLine = strings.TrimSpace(firstLine)
		if len(firstLine) > 60 {
			return firstLine[:57] + "..."
		}
		return firstLine
	}
	if len(s) > 60 {
		return s[:57] + "..."
	}
	return s
}

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
