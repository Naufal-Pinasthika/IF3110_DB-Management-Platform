package migration

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/api"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/apperror"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/store"
	pg_query "github.com/pganalyze/pg_query_go/v5"
)

// SchemaDiffer compares the current target with a complete DDL description.
// Desired DDL is materialized only in scratch schemas inside a rolled-back tx.
type SchemaDiffer struct {
	Targets *store.TargetStore
	Timeout time.Duration
}

type schemaKey struct{ Schema, Table string }
type columnShape struct {
	Type, Default, Identity, Generated string
	Nullable                           bool
}
type namedDefinition struct{ Definition, Kind string }
type tableShape struct {
	Columns     map[string]columnShape
	Constraints map[string]namedDefinition
	Indexes     map[string]string
}
type schemaShape struct {
	Schemas  map[string]bool
	Tables   map[schemaKey]*tableShape
	Excluded int
}

func quoteIdent(s string) string           { return `"` + strings.ReplaceAll(s, `"`, `""`) + `"` }
func qualified(schema, name string) string { return quoteIdent(schema) + "." + quoteIdent(name) }

func userSchema(name string) bool {
	return name != "" && name != "information_schema" && name != "pg_catalog" &&
		!strings.HasPrefix(name, "pg_toast") && !strings.HasPrefix(name, "pg_temp_") &&
		!strings.HasPrefix(name, "__schema_diff_")
}

func parseDesiredDDL(sqlText string) (*pg_query.ParseResult, map[string]string, error) {
	if strings.TrimSpace(sqlText) == "" {
		return nil, nil, fmt.Errorf("%w: desired schema DDL cannot be empty", apperror.ErrInvalidInput)
	}
	parsed, err := pg_query.Parse(sqlText)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: invalid schema DDL: %v", apperror.ErrInvalidInput, err)
	}
	if len(parsed.Stmts) == 0 {
		return nil, nil, fmt.Errorf("%w: desired schema DDL cannot be empty", apperror.ErrInvalidInput)
	}

	names := map[string]bool{}
	addName := func(name string) error {
		if name == "" {
			name = "public"
		}
		if !userSchema(name) {
			return fmt.Errorf("%w: system schema %q cannot be part of a desired schema", apperror.ErrInvalidInput, name)
		}
		names[name] = true
		return nil
	}
	for _, raw := range parsed.Stmts {
		n := raw.Stmt
		if stmt := n.GetCreateSchemaStmt(); stmt != nil {
			if stmt.Authrole != nil || len(stmt.SchemaElts) > 0 {
				return nil, nil, fmt.Errorf("%w: CREATE SCHEMA AUTHORIZATION and inline schema objects are unsupported", apperror.ErrInvalidInput)
			}
			if err := addName(stmt.Schemaname); err != nil {
				return nil, nil, err
			}
			continue
		}
		if stmt := n.GetCreateStmt(); stmt != nil {
			if stmt.Relation == nil || stmt.Relation.Catalogname != "" || stmt.OfTypename != nil || stmt.Partbound != nil || stmt.Partspec != nil || len(stmt.InhRelations) > 0 || len(stmt.Options) > 0 || stmt.Tablespacename != "" {
				return nil, nil, fmt.Errorf("%w: this CREATE TABLE form is outside schema diff support", apperror.ErrInvalidInput)
			}
			if stmt.IfNotExists {
				return nil, nil, fmt.Errorf("%w: omit IF NOT EXISTS from desired schema DDL", apperror.ErrInvalidInput)
			}
			if err := addName(stmt.Relation.Schemaname); err != nil {
				return nil, nil, err
			}
			for _, elt := range stmt.TableElts {
				col := elt.GetColumnDef()
				if col != nil {
					if isSerialType(col.TypeName) {
						return nil, nil, fmt.Errorf("%w: use GENERATED ... AS IDENTITY instead of SERIAL so the generated migration includes the sequence", apperror.ErrInvalidInput)
					}
					for _, cn := range col.Constraints {
						constraint := cn.GetConstraint()
						if err := validateDesiredConstraint(constraint); err != nil {
							return nil, nil, err
						}
						if constraint.Pktable != nil {
							if constraint.Pktable.Catalogname != "" {
								return nil, nil, fmt.Errorf("%w: cross-database foreign keys are unsupported", apperror.ErrInvalidInput)
							}
							if err := addName(constraint.Pktable.Schemaname); err != nil {
								return nil, nil, err
							}
						}
					}
					continue
				}
				if constraint := elt.GetConstraint(); constraint != nil {
					if err := validateDesiredConstraint(constraint); err != nil {
						return nil, nil, err
					}
					if constraint.Pktable != nil {
						if constraint.Pktable.Catalogname != "" {
							return nil, nil, fmt.Errorf("%w: cross-database foreign keys are unsupported", apperror.ErrInvalidInput)
						}
						if err := addName(constraint.Pktable.Schemaname); err != nil {
							return nil, nil, err
						}
					}
					continue
				}
				return nil, nil, fmt.Errorf("%w: unsupported table element", apperror.ErrInvalidInput)
			}
			for _, cn := range stmt.Constraints {
				constraint := cn.GetConstraint()
				if err := validateDesiredConstraint(constraint); err != nil {
					return nil, nil, err
				}
				if constraint.Pktable != nil {
					if constraint.Pktable.Catalogname != "" {
						return nil, nil, fmt.Errorf("%w: cross-database foreign keys are unsupported", apperror.ErrInvalidInput)
					}
					if err := addName(constraint.Pktable.Schemaname); err != nil {
						return nil, nil, err
					}
				}
			}
			continue
		}
		if stmt := n.GetIndexStmt(); stmt != nil {
			if stmt.Relation == nil || stmt.Relation.Catalogname != "" || stmt.Concurrent || stmt.TableSpace != "" || stmt.Isconstraint {
				return nil, nil, fmt.Errorf("%w: this CREATE INDEX form is outside schema diff support", apperror.ErrInvalidInput)
			}
			if err := addName(stmt.Relation.Schemaname); err != nil {
				return nil, nil, err
			}
			continue
		}
		return nil, nil, fmt.Errorf("%w: only CREATE SCHEMA, CREATE TABLE, and CREATE INDEX are accepted as a complete desired schema", apperror.ErrInvalidInput)
	}
	if len(names) == 0 {
		names["public"] = true
	}

	var random [5]byte
	if _, err := rand.Read(random[:]); err != nil {
		return nil, nil, fmt.Errorf("%w: could not prepare schema comparison", apperror.ErrInternal)
	}
	prefix := "__schema_diff_" + hex.EncodeToString(random[:])
	ordered := make([]string, 0, len(names))
	for name := range names {
		ordered = append(ordered, name)
	}
	sort.Strings(ordered)
	mapping := make(map[string]string, len(ordered))
	for i, name := range ordered {
		mapping[name] = fmt.Sprintf("%s_%d", prefix, i)
	}

	// Rewrite relation names into isolated scratch schemas. CREATE SCHEMA
	// statements become namespace declarations and are issued ourselves.
	filtered := &pg_query.ParseResult{}
	for _, raw := range parsed.Stmts {
		n := raw.Stmt
		if schemaStmt := n.GetCreateSchemaStmt(); schemaStmt != nil {
			continue
		}
		if stmt := n.GetCreateStmt(); stmt != nil {
			rewriteRangeVar(stmt.Relation, mapping)
			for _, elt := range stmt.TableElts {
				if col := elt.GetColumnDef(); col != nil {
					for _, cn := range col.Constraints {
						rewriteConstraint(cn.GetConstraint(), mapping)
					}
				} else {
					rewriteConstraint(elt.GetConstraint(), mapping)
				}
			}
			for _, cn := range stmt.Constraints {
				rewriteConstraint(cn.GetConstraint(), mapping)
			}
		} else if stmt := n.GetIndexStmt(); stmt != nil {
			rewriteRangeVar(stmt.Relation, mapping)
		}
		filtered.Stmts = append(filtered.Stmts, raw)
	}
	return filtered, mapping, nil
}

func isSerialType(typeName *pg_query.TypeName) bool {
	if typeName == nil || len(typeName.Names) == 0 {
		return false
	}
	last := typeName.Names[len(typeName.Names)-1].GetString_()
	if last == nil {
		return false
	}
	switch strings.ToLower(last.Sval) {
	case "serial", "smallserial", "bigserial":
		return true
	default:
		return false
	}
}

func validateDesiredConstraint(c *pg_query.Constraint) error {
	if c == nil {
		return fmt.Errorf("%w: unsupported table constraint", apperror.ErrInvalidInput)
	}
	switch c.Contype {
	case pg_query.ConstrType_CONSTR_NULL, pg_query.ConstrType_CONSTR_NOTNULL, pg_query.ConstrType_CONSTR_DEFAULT,
		pg_query.ConstrType_CONSTR_CHECK, pg_query.ConstrType_CONSTR_PRIMARY, pg_query.ConstrType_CONSTR_UNIQUE,
		pg_query.ConstrType_CONSTR_FOREIGN:
		return nil
	default:
		return fmt.Errorf("%w: unsupported table constraint type", apperror.ErrInvalidInput)
	}
}

func rewriteRangeVar(rv *pg_query.RangeVar, mapping map[string]string) {
	if rv == nil {
		return
	}
	if rv.Schemaname == "" {
		rv.Schemaname = "public"
	}
	if scratch, ok := mapping[rv.Schemaname]; ok {
		rv.Schemaname = scratch
	}
}

func rewriteConstraint(c *pg_query.Constraint, mapping map[string]string) {
	if c != nil && c.Pktable != nil {
		rewriteRangeVar(c.Pktable, mapping)
	}
}

func schemaAliases(mapping map[string]string) map[string]string {
	reverse := make(map[string]string, len(mapping))
	for original, scratch := range mapping {
		reverse[scratch] = original
	}
	return reverse
}

func rewriteSchemaText(text string, aliases map[string]string) string {
	for scratch, original := range aliases {
		text = strings.ReplaceAll(text, quoteIdent(scratch)+".", quoteIdent(original)+".")
		text = strings.ReplaceAll(text, scratch+".", quoteIdent(original)+".")
	}
	return text
}

func schemaDiffContext(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return context.WithTimeout(ctx, timeout)
}

// Compare returns a diff against the declared desired schema without changing
// the target. All DDL runs in a transaction that is always rolled back.
func (d *SchemaDiffer) Compare(ctx context.Context, databaseID, desiredDDL string) (api.SchemaDiffResult, error) {
	parsed, mapping, err := parseDesiredDDL(desiredDDL)
	if err != nil {
		return api.SchemaDiffResult{}, err
	}
	db, err := d.Targets.Migrator(ctx, databaseID)
	if err != nil {
		return api.SchemaDiffResult{}, err
	}
	txCtx, cancel := schemaDiffContext(ctx, d.Timeout)
	defer cancel()
	tx, err := db.BeginTx(txCtx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return api.SchemaDiffResult{}, fmt.Errorf("%w: could not start schema comparison: %v", apperror.ErrInternal, err)
	}
	rolledBack := false
	defer func() {
		if !rolledBack {
			_ = tx.Rollback()
		}
	}()

	current, err := readSchemaShape(txCtx, tx, nil)
	if err != nil {
		return api.SchemaDiffResult{}, err
	}
	orderedScratch := make([]string, 0, len(mapping))
	for _, scratch := range mapping {
		orderedScratch = append(orderedScratch, scratch)
	}
	sort.Strings(orderedScratch)
	quotedPath := make([]string, 0, len(orderedScratch)+1)
	for _, scratch := range orderedScratch {
		if _, err := tx.ExecContext(txCtx, "CREATE SCHEMA "+quoteIdent(scratch)); err != nil {
			return api.SchemaDiffResult{}, fmt.Errorf("%w: could not prepare desired schema: %v", apperror.ErrInvalidInput, err)
		}
		quotedPath = append(quotedPath, quoteIdent(scratch))
	}
	quotedPath = append(quotedPath, "public")
	if _, err := tx.ExecContext(txCtx, "SET LOCAL search_path TO "+strings.Join(quotedPath, ", ")); err != nil {
		return api.SchemaDiffResult{}, fmt.Errorf("%w: could not set comparison search path: %v", apperror.ErrInternal, err)
	}
	desiredSQL, err := pg_query.Deparse(parsed)
	if err != nil {
		return api.SchemaDiffResult{}, fmt.Errorf("%w: could not normalize desired DDL: %v", apperror.ErrInvalidInput, err)
	}
	statements, err := pg_query.SplitWithParser(desiredSQL, true)
	if err != nil {
		return api.SchemaDiffResult{}, fmt.Errorf("%w: could not split desired DDL: %v", apperror.ErrInvalidInput, err)
	}
	for _, statement := range statements {
		if _, err := tx.ExecContext(txCtx, statement); err != nil {
			return api.SchemaDiffResult{}, fmt.Errorf("%w: desired schema DDL could not be applied in the isolated comparison: %v", apperror.ErrInvalidInput, err)
		}
	}
	aliases := schemaAliases(mapping)
	desired, err := readSchemaShape(txCtx, tx, aliases)
	if err != nil {
		return api.SchemaDiffResult{}, err
	}
	result := compareSchemaShapes(current, desired)
	if err := tx.Rollback(); err != nil {
		return api.SchemaDiffResult{}, fmt.Errorf("%w: could not roll back schema comparison: %v", apperror.ErrInternal, err)
	}
	rolledBack = true
	return result, nil
}

func readSchemaShape(ctx context.Context, tx *sql.Tx, aliases map[string]string) (schemaShape, error) {
	shape := schemaShape{Schemas: map[string]bool{}, Tables: map[schemaKey]*tableShape{}}
	rows, err := tx.QueryContext(ctx, `SELECT nspname FROM pg_namespace WHERE nspname NOT IN ('pg_catalog','information_schema') AND nspname NOT LIKE 'pg_toast%' AND nspname NOT LIKE 'pg_temp_%' ORDER BY nspname`)
	if err != nil {
		return shape, fmt.Errorf("%w: could not inspect schemas: %v", apperror.ErrInternal, err)
	}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			rows.Close()
			return shape, err
		}
		if aliases != nil {
			original, ok := aliases[name]
			if !ok {
				continue
			}
			name = original
		} else if !userSchema(name) {
			continue
		}
		shape.Schemas[name] = true
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return shape, err
	}
	rows.Close()

	tableRows, err := tx.QueryContext(ctx, `SELECT n.nspname,c.relname,c.relkind FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE c.relkind IN ('r','p') AND NOT c.relispartition AND n.nspname NOT IN ('pg_catalog','information_schema') AND n.nspname NOT LIKE 'pg_toast%' AND n.nspname NOT LIKE 'pg_temp_%' ORDER BY n.nspname,c.relname`)
	if err != nil {
		return shape, fmt.Errorf("%w: could not inspect tables: %v", apperror.ErrInternal, err)
	}
	type tableID struct{ schema, table string }
	var ids []tableID
	for tableRows.Next() {
		var schema, table string
		var kind string
		if err := tableRows.Scan(&schema, &table, &kind); err != nil {
			tableRows.Close()
			return shape, err
		}
		if aliases != nil {
			original, ok := aliases[schema]
			if !ok {
				continue
			}
			schema = original
		} else if !userSchema(schema) {
			continue
		}
		shape.Tables[schemaKey{schema, table}] = &tableShape{Columns: map[string]columnShape{}, Constraints: map[string]namedDefinition{}, Indexes: map[string]string{}}
		ids = append(ids, tableID{schema, table})
	}
	if err := tableRows.Err(); err != nil {
		tableRows.Close()
		return shape, err
	}
	tableRows.Close()

	for _, id := range ids {
		dbSchema := id.schema
		if aliases != nil {
			for scratch, original := range aliases {
				if original == id.schema {
					dbSchema = scratch
					break
				}
			}
		}
		key := schemaKey{id.schema, id.table}
		table := shape.Tables[key]
		colRows, err := tx.QueryContext(ctx, `SELECT a.attname,format_type(a.atttypid,a.atttypmod),NOT a.attnotnull,pg_get_expr(ad.adbin,ad.adrelid),a.attidentity,a.attgenerated FROM pg_attribute a JOIN pg_class c ON c.oid=a.attrelid JOIN pg_namespace n ON n.oid=c.relnamespace LEFT JOIN pg_attrdef ad ON ad.adrelid=c.oid AND ad.adnum=a.attnum WHERE n.nspname=$1 AND c.relname=$2 AND a.attnum>0 AND NOT a.attisdropped ORDER BY a.attnum`, dbSchema, id.table)
		if err != nil {
			return shape, fmt.Errorf("%w: could not inspect columns: %v", apperror.ErrInternal, err)
		}
		for colRows.Next() {
			var name, typ, identity, generated string
			var nullable bool
			var def sql.NullString
			if err := colRows.Scan(&name, &typ, &nullable, &def, &identity, &generated); err != nil {
				colRows.Close()
				return shape, err
			}
			defaultExpr := ""
			if def.Valid {
				defaultExpr = def.String
			}
			if aliases != nil {
				defaultExpr = rewriteSchemaText(defaultExpr, aliases)
			}
			table.Columns[name] = columnShape{Type: typ, Nullable: nullable, Default: defaultExpr, Identity: identity, Generated: generated}
		}
		if err := colRows.Err(); err != nil {
			colRows.Close()
			return shape, err
		}
		colRows.Close()

		conRows, err := tx.QueryContext(ctx, `SELECT conname,contype,pg_get_constraintdef(oid,true) FROM pg_constraint WHERE conrelid=(SELECT c.oid FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname=$1 AND c.relname=$2) AND contype IN ('p','u','f','c','x') ORDER BY conname`, dbSchema, id.table)
		if err != nil {
			return shape, fmt.Errorf("%w: could not inspect constraints: %v", apperror.ErrInternal, err)
		}
		for conRows.Next() {
			var name, kind, def string
			if err := conRows.Scan(&name, &kind, &def); err != nil {
				conRows.Close()
				return shape, err
			}
			if aliases != nil {
				def = rewriteSchemaText(def, aliases)
			}
			table.Constraints[name] = namedDefinition{Definition: def, Kind: kind}
		}
		if err := conRows.Err(); err != nil {
			conRows.Close()
			return shape, err
		}
		conRows.Close()

		idxRows, err := tx.QueryContext(ctx, `SELECT i.relname,pg_get_indexdef(i.oid) FROM pg_index ix JOIN pg_class t ON t.oid=ix.indrelid JOIN pg_namespace n ON n.oid=t.relnamespace JOIN pg_class i ON i.oid=ix.indexrelid WHERE n.nspname=$1 AND t.relname=$2 AND NOT EXISTS (SELECT 1 FROM pg_constraint con WHERE con.conindid=i.oid) ORDER BY i.relname`, dbSchema, id.table)
		if err != nil {
			return shape, fmt.Errorf("%w: could not inspect indexes: %v", apperror.ErrInternal, err)
		}
		for idxRows.Next() {
			var name, def string
			if err := idxRows.Scan(&name, &def); err != nil {
				idxRows.Close()
				return shape, err
			}
			if aliases != nil {
				def = rewriteSchemaText(def, aliases)
			}
			table.Indexes[name] = def
		}
		if err := idxRows.Err(); err != nil {
			idxRows.Close()
			return shape, err
		}
		idxRows.Close()
	}
	if aliases == nil {
		var excluded int
		err := tx.QueryRowContext(ctx, `SELECT count(*) FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE (c.relkind NOT IN ('r','p','i','I','t') OR c.relispartition) AND n.nspname NOT IN ('pg_catalog','information_schema') AND n.nspname NOT LIKE 'pg_toast%' AND n.nspname NOT LIKE 'pg_temp_%'`).Scan(&excluded)
		if err != nil {
			return shape, fmt.Errorf("%w: could not count excluded objects: %v", apperror.ErrInternal, err)
		}
		shape.Excluded = excluded
	}
	return shape, nil
}

// compareSchemaShapes produces deterministic diff entries and reviewed SQL.
func compareSchemaShapes(current, desired schemaShape) api.SchemaDiffResult {
	result := api.SchemaDiffResult{Changes: []api.SchemaDiffChange{}, ExcludedObjectCount: current.Excluded}
	var createSchemas, dropDependencies, dropColumns, alterColumns, dropTables, createTables, addColumns, addConstraints, createIndexes []string
	for _, schema := range sortedBoolKeys(desired.Schemas) {
		if !current.Schemas[schema] {
			result.Changes = append(result.Changes, api.SchemaDiffChange{Kind: "schema", Action: "added", Schema: schema, Name: schema, After: "Schema namespace"})
			createSchemas = append(createSchemas, "CREATE SCHEMA "+quoteIdent(schema)+";")
		}
	}
	keys := map[schemaKey]bool{}
	for key := range current.Tables {
		keys[key] = true
	}
	for key := range desired.Tables {
		keys[key] = true
	}
	ordered := make([]schemaKey, 0, len(keys))
	for key := range keys {
		ordered = append(ordered, key)
	}
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].Schema == ordered[j].Schema {
			return ordered[i].Table < ordered[j].Table
		}
		return ordered[i].Schema < ordered[j].Schema
	})
	for _, key := range ordered {
		old, had := current.Tables[key]
		want, has := desired.Tables[key]
		tableName := qualified(key.Schema, key.Table)
		if had && !has {
			result.Changes = append(result.Changes, api.SchemaDiffChange{Kind: "table", Action: "removed", Schema: key.Schema, Table: key.Table, Name: key.Table, Before: tableSummary(old)})
			for _, name := range sortedKeys(old.Constraints) {
				if old.Constraints[name].Kind == "f" {
					dropDependencies = append(dropDependencies, "ALTER TABLE "+tableName+" DROP CONSTRAINT "+quoteIdent(name)+";")
				}
			}
			dropTables = append(dropTables, "DROP TABLE "+tableName+";")
			continue
		}
		if !had && has {
			result.Changes = append(result.Changes, api.SchemaDiffChange{Kind: "table", Action: "added", Schema: key.Schema, Table: key.Table, Name: key.Table, After: tableSummary(want)})
			createTables = append(createTables, createTableSQL(key, want))
			for _, name := range sortedKeys(want.Indexes) {
				createIndexes = append(createIndexes, want.Indexes[name]+";")
			}
			for _, name := range sortedKeys(want.Constraints) {
				addConstraints = append(addConstraints, "ALTER TABLE "+tableName+" ADD CONSTRAINT "+quoteIdent(name)+" "+want.Constraints[name].Definition+";")
			}
			continue
		}
		if !had {
			continue
		}
		for _, name := range sortedKeys(old.Constraints) {
			wantDef, exists := want.Constraints[name]
			if !exists || wantDef != old.Constraints[name] {
				result.Changes = append(result.Changes, api.SchemaDiffChange{Kind: "constraint", Action: actionFor(exists), Schema: key.Schema, Table: key.Table, Name: name, Before: old.Constraints[name].Definition, After: wantDef.Definition})
				dropDependencies = append(dropDependencies, "ALTER TABLE "+tableName+" DROP CONSTRAINT "+quoteIdent(name)+";")
			}
		}
		for _, name := range sortedKeys(want.Constraints) {
			oldDef, exists := old.Constraints[name]
			if !exists || oldDef != want.Constraints[name] {
				addConstraints = append(addConstraints, "ALTER TABLE "+tableName+" ADD CONSTRAINT "+quoteIdent(name)+" "+want.Constraints[name].Definition+";")
			}
		}
		for _, name := range sortedKeys(old.Indexes) {
			wantDef, exists := want.Indexes[name]
			if !exists || wantDef != old.Indexes[name] {
				result.Changes = append(result.Changes, api.SchemaDiffChange{Kind: "index", Action: actionFor(exists), Schema: key.Schema, Table: key.Table, Name: name, Before: old.Indexes[name], After: wantDef})
				dropDependencies = append(dropDependencies, "DROP INDEX "+qualified(key.Schema, name)+";")
			}
		}
		for _, name := range sortedKeys(want.Indexes) {
			if old.Indexes[name] != want.Indexes[name] {
				createIndexes = append(createIndexes, want.Indexes[name]+";")
			}
		}
		columnNames := map[string]bool{}
		for n := range old.Columns {
			columnNames[n] = true
		}
		for n := range want.Columns {
			columnNames[n] = true
		}
		for _, name := range sortedBoolKeys(columnNames) {
			before, hadCol := old.Columns[name]
			after, hasCol := want.Columns[name]
			colQ := quoteIdent(name)
			switch {
			case hadCol && !hasCol:
				result.Changes = append(result.Changes, api.SchemaDiffChange{Kind: "column", Action: "removed", Schema: key.Schema, Table: key.Table, Name: name, Before: formatColumn(before)})
				dropColumns = append(dropColumns, "ALTER TABLE "+tableName+" DROP COLUMN "+colQ+";")
			case !hadCol && hasCol:
				result.Changes = append(result.Changes, api.SchemaDiffChange{Kind: "column", Action: "added", Schema: key.Schema, Table: key.Table, Name: name, After: formatColumn(after)})
				addColumns = append(addColumns, "ALTER TABLE "+tableName+" ADD COLUMN "+columnSQL(name, after)+";")
			case hadCol && hasCol && before != after:
				result.Changes = append(result.Changes, api.SchemaDiffChange{Kind: "column", Action: "modified", Schema: key.Schema, Table: key.Table, Name: name, Before: formatColumn(before), After: formatColumn(after)})
				if before.Identity != after.Identity || before.Generated != after.Generated {
					dropColumns = append(dropColumns, "ALTER TABLE "+tableName+" DROP COLUMN "+colQ+";")
					addColumns = append(addColumns, "ALTER TABLE "+tableName+" ADD COLUMN "+columnSQL(name, after)+";")
				} else {
					if before.Default != after.Default && before.Default != "" {
						alterColumns = append(alterColumns, "ALTER TABLE "+tableName+" ALTER COLUMN "+colQ+" DROP DEFAULT;")
					}
					if before.Type != after.Type {
						alterColumns = append(alterColumns, "ALTER TABLE "+tableName+" ALTER COLUMN "+colQ+" TYPE "+after.Type+";")
					}
					if before.Nullable != after.Nullable {
						if after.Nullable {
							alterColumns = append(alterColumns, "ALTER TABLE "+tableName+" ALTER COLUMN "+colQ+" DROP NOT NULL;")
						} else {
							alterColumns = append(alterColumns, "ALTER TABLE "+tableName+" ALTER COLUMN "+colQ+" SET NOT NULL;")
						}
					}
					if before.Default != after.Default && after.Default != "" {
						alterColumns = append(alterColumns, "ALTER TABLE "+tableName+" ALTER COLUMN "+colQ+" SET DEFAULT "+after.Default+";")
					}
				}
			}
		}
	}
	result.HasChanges = len(result.Changes) > 0
	parts := [][]string{createSchemas, dropDependencies, dropColumns, alterColumns, dropTables, createTables, addColumns, addConstraints, createIndexes}
	var sqlParts []string
	for _, group := range parts {
		if len(group) > 0 {
			sqlParts = append(sqlParts, strings.Join(group, "\n"))
		}
	}
	if len(sqlParts) > 0 {
		result.GeneratedSQL = strings.Join(sqlParts, "\n\n") + "\n"
	}
	return result
}

func actionFor(exists bool) string {
	if exists {
		return "modified"
	}
	return "added"
}
func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
func sortedBoolKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
func formatColumn(c columnShape) string {
	parts := []string{c.Type}
	if c.Identity == "a" {
		parts = append(parts, "GENERATED ALWAYS AS IDENTITY")
	} else if c.Identity == "d" {
		parts = append(parts, "GENERATED BY DEFAULT AS IDENTITY")
	}
	if c.Generated != "" && c.Default != "" {
		parts = append(parts, "GENERATED ALWAYS AS ("+c.Default+") STORED")
	} else if c.Default != "" && c.Identity == "" {
		parts = append(parts, "DEFAULT "+c.Default)
	}
	if !c.Nullable {
		parts = append(parts, "NOT NULL")
	}
	return strings.Join(parts, " ")
}
func columnSQL(name string, c columnShape) string { return quoteIdent(name) + " " + formatColumn(c) }
func tableSummary(t *tableShape) string {
	return fmt.Sprintf("%d columns, %d constraints, %d indexes", len(t.Columns), len(t.Constraints), len(t.Indexes))
}
func createTableSQL(key schemaKey, t *tableShape) string {
	var lines []string
	for _, name := range sortedKeys(t.Columns) {
		lines = append(lines, "  "+columnSQL(name, t.Columns[name]))
	}
	return "CREATE TABLE " + qualified(key.Schema, key.Table) + " (\n" + strings.Join(lines, ",\n") + "\n);"
}
