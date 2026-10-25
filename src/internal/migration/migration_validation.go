package migration

import (
	"fmt"
	"strings"

	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/apperror"
	pg_query "github.com/pganalyze/pg_query_go/v5"
)

// validateDMLSQL uses PostgreSQL's parser and permits only row-query/DML
// statements. In particular, schema-changing statements such as CREATE,
// ALTER, and DROP are rejected regardless of case, comments, or formatting.
func validateDMLSQL(sqlText string) error {
	if strings.TrimSpace(sqlText) == "" {
		return fmt.Errorf("%w: SQL text cannot be empty", apperror.ErrInvalidInput)
	}
	parsed, err := pg_query.Parse(sqlText)
	if err != nil {
		return fmt.Errorf("%w: invalid SQL syntax: %v", apperror.ErrInvalidInput, err)
	}
	if len(parsed.Stmts) == 0 {
		return fmt.Errorf("%w: no SQL statements found", apperror.ErrInvalidInput)
	}
	for _, raw := range parsed.Stmts {
		stmt := raw.Stmt
		if selectStmt := stmt.GetSelectStmt(); selectStmt != nil {
			if selectStmt.GetIntoClause() != nil {
				return fmt.Errorf("%w: SELECT INTO is forbidden because it creates a table", apperror.ErrForbidden)
			}
			continue
		}
		if stmt.GetInsertStmt() != nil || stmt.GetUpdateStmt() != nil || stmt.GetDeleteStmt() != nil || stmt.GetMergeStmt() != nil {
			continue
		}
		return fmt.Errorf("%w: only SELECT and DML statements are permitted; DDL and other commands are forbidden", apperror.ErrForbidden)
	}
	return nil
}
