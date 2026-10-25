package explorer

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/api"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/apperror"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/models"
)

func (r *TargetRepository) mysqlSchemas(ctx context.Context, config models.ManagedDatabase) ([]api.SchemaSummary, error) {
	if _, err := r.Targets.Reader(ctx, config.ID); err != nil {
		return nil, err
	}
	return []api.SchemaSummary{{Name: config.DatabaseName}}, nil
}

func (r *TargetRepository) mysqlTables(ctx context.Context, config models.ManagedDatabase, schema string) ([]api.TableSummary, error) {
	if schema != config.DatabaseName {
		return nil, apperror.ErrForbidden
	}
	db, err := r.Targets.Reader(ctx, config.ID)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, r.QueryTimeout)
	defer cancel()
	rows, err := db.QueryContext(ctx, `SELECT table_name FROM information_schema.tables WHERE table_schema = ? AND table_type = 'BASE TABLE' ORDER BY table_name`, schema)
	if err != nil {
		return nil, fmt.Errorf("%w: could not read MySQL tables", apperror.ErrInternal)
	}
	defer rows.Close()
	tables := []api.TableSummary{}
	for rows.Next() {
		var table api.TableSummary
		if err := rows.Scan(&table.Name); err != nil {
			return nil, err
		}
		tables = append(tables, table)
	}
	return tables, rows.Err()
}

func (r *TargetRepository) mysqlTableDetail(ctx context.Context, config models.ManagedDatabase, schema, table string) (api.TableDetail, error) {
	detail := api.TableDetail{Schema: schema, Table: table, Columns: []api.TableColumn{}, Constraints: []api.TableConstraint{}, Indexes: []api.TableIndex{}}
	if schema != config.DatabaseName {
		return detail, apperror.ErrForbidden
	}
	db, err := r.Targets.Reader(ctx, config.ID)
	if err != nil {
		return detail, err
	}
	ctx, cancel := context.WithTimeout(ctx, r.QueryTimeout)
	defer cancel()
	columns, err := db.QueryContext(ctx, `SELECT column_name, column_type, is_nullable FROM information_schema.columns WHERE table_schema = ? AND table_name = ? ORDER BY ordinal_position`, schema, table)
	if err != nil {
		return detail, err
	}
	for columns.Next() {
		var col api.TableColumn
		var nullable string
		if err := columns.Scan(&col.Name, &col.DataType, &nullable); err != nil {
			columns.Close()
			return detail, err
		}
		col.Nullable = nullable == "YES"
		detail.Columns = append(detail.Columns, col)
	}
	err = columns.Err()
	columns.Close()
	if err != nil {
		return detail, err
	}
	if len(detail.Columns) == 0 {
		return detail, apperror.ErrNotFound
	}

	// LEFT JOIN includes CHECK constraints, which have no key_column_usage row.
	constraints, err := db.QueryContext(ctx, `SELECT tc.constraint_name, tc.constraint_type, kcu.column_name
		FROM information_schema.table_constraints tc
		LEFT JOIN information_schema.key_column_usage kcu ON tc.constraint_schema=kcu.constraint_schema AND tc.table_name=kcu.table_name AND tc.constraint_name=kcu.constraint_name
		WHERE tc.table_schema=? AND tc.table_name=? ORDER BY tc.constraint_name, kcu.ordinal_position`, schema, table)
	if err != nil {
		return detail, err
	}
	for constraints.Next() {
		var name, kind string
		var column sql.NullString
		if err := constraints.Scan(&name, &kind, &column); err != nil {
			constraints.Close()
			return detail, err
		}
		if len(detail.Constraints) == 0 || detail.Constraints[len(detail.Constraints)-1].Name != name {
			detail.Constraints = append(detail.Constraints, api.TableConstraint{Name: name, Type: kind, Columns: []string{}})
		}
		if column.Valid {
			i := len(detail.Constraints) - 1
			detail.Constraints[i].Columns = append(detail.Constraints[i].Columns, column.String)
		}
	}
	err = constraints.Err()
	constraints.Close()
	if err != nil {
		return detail, err
	}

	indexes, err := db.QueryContext(ctx, `SELECT index_name, non_unique, COALESCE(column_name, expression, '') FROM information_schema.statistics WHERE table_schema=? AND table_name=? ORDER BY index_name, seq_in_index`, schema, table)
	if err != nil {
		return detail, err
	}
	defer indexes.Close()
	for indexes.Next() {
		var name, column string
		var nonUnique int
		if err := indexes.Scan(&name, &nonUnique, &column); err != nil {
			return detail, err
		}
		if len(detail.Indexes) == 0 || detail.Indexes[len(detail.Indexes)-1].Name != name {
			detail.Indexes = append(detail.Indexes, api.TableIndex{Name: name, IsUnique: nonUnique == 0, Columns: []string{}})
		}
		i := len(detail.Indexes) - 1
		detail.Indexes[i].Columns = append(detail.Indexes[i].Columns, column)
	}
	return detail, indexes.Err()
}
