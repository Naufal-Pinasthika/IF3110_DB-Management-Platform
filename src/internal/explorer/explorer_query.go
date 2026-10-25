package explorer

import (
	"fmt"

	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/apperror"
	pg_query "github.com/pganalyze/pg_query_go/v5"
)


// ValidateReadOnlySQL parses the AST and ensures exactly one safe SELECT statement.
func ValidateReadOnlySQL(sqlText string) error {
	if sqlText == "" {
		return fmt.Errorf("%w: sql text cannot be empty", apperror.ErrInvalidInput)
	}


	// 1. Use PostgreSQL aware parsing (creates an AST)
	result, err := pg_query.Parse(sqlText)
	if err != nil {
		return fmt.Errorf("%w: invalid sql syntax: %v", apperror.ErrInvalidInput, err)
	}


	// 2. Allow exactly one statement
	if len(result.Stmts) != 1 {
		return fmt.Errorf("%w: query must contain exactly one statement", apperror.ErrInvalidInput)
	}
	stmt := result.Stmts[0].Stmt


	// 3. Ensure it is a SELECT statement
	selStmt := stmt.GetSelectStmt()
	if selStmt == nil {
		return fmt.Errorf("%w: only SELECT statements are permitted", apperror.ErrForbidden)
	}


	// 4. Reject SELECT INTO (which creates tables/inserts data)
	if selStmt.GetIntoClause() != nil {
		return fmt.Errorf("%w: SELECT INTO is not permitted", apperror.ErrForbidden)
	}


	// 5. Reject Row locking clauses like FOR UPDATE or FOR SHARE
	if len(selStmt.GetLockingClause()) > 0 {
		return fmt.Errorf("%w: row locking clauses (FOR UPDATE/SHARE) are not permitted", apperror.ErrForbidden)
	}


	// 6. Ensure CTEs (WITH clauses) do not contain data-modifying statements (INSERT/UPDATE/DELETE)
	if withClause := selStmt.GetWithClause(); withClause != nil {
		for _, cteNode := range withClause.GetCtes() {
			cte := cteNode.GetCommonTableExpr()
			if cte.GetCtequery().GetSelectStmt() == nil {
				return fmt.Errorf("%w: data-modifying CTEs are not permitted", apperror.ErrForbidden)
			}
		}
	}

	return nil
}