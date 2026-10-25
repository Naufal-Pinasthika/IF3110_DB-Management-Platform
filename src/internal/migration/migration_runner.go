package migration

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/apperror"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/models"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/sqlguard"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/store"
	"github.com/go-sql-driver/mysql"
	pgquery "github.com/pganalyze/pg_query_go/v5"
)

type Runner struct {
	Targets *store.TargetStore
	Timeout time.Duration
}

var _ SQLRunner = (*Runner)(nil)

func (r *Runner) Execute(ctx context.Context, databaseID, sqlText string) error {
	config, err := r.Targets.Configuration(ctx, databaseID)
	if err != nil {
		return err
	}
	statements := []string{sqlText}
	var plan sqlguard.MySQLMigration
	if config.TargetEngine() == models.EngineMySQL {
		plan, err = sqlguard.ParseMySQLMigration(sqlText, config.DatabaseName)
		if err != nil {
			return err
		}
		statements = plan.Statements
	} else {
		// Parse transaction commands, including END/ABORT, instead of searching comments/literals.
		parsed, err := pgquery.Parse(sqlText)
		if err != nil || len(parsed.Stmts) == 0 {
			return fmt.Errorf("%w: invalid migration SQL", apperror.ErrInvalidInput)
		}
		for _, raw := range parsed.Stmts {
			if raw.Stmt.GetTransactionStmt() != nil {
				return fmt.Errorf("%w: manual transaction control is forbidden", apperror.ErrInvalidInput)
			}
		}
	}
	db, err := r.Targets.Migrator(ctx, databaseID)
	if err != nil {
		return fmt.Errorf("migrator connection failed: %w", err)
	}
	ctx, cancel := context.WithTimeout(ctx, r.Timeout)
	defer cancel()
	if plan.DDL {
		return executeMySQLDDL(ctx, db, statements[0])
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()
	if config.TargetEngine() == models.EngineMySQL {
		// Nontransactional tables would silently violate the rollback guarantee.
		for _, table := range plan.Tables {
			var engine sql.NullString
			err := tx.QueryRowContext(ctx, "SELECT engine FROM information_schema.tables WHERE table_schema=? AND table_name=?", config.DatabaseName, table).Scan(&engine)
			if err != nil || !engine.Valid || !strings.EqualFold(engine.String, "InnoDB") {
				return fmt.Errorf("%w: DML migrations require existing InnoDB tables", apperror.ErrInvalidInput)
			}
		}
	}
	for _, statement := range statements {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			if rbErr := tx.Rollback(); rbErr != nil {
				return fmt.Errorf("%w: execution failed and rollback was not confirmed", apperror.ErrTransactionOutcomeUnknown)
			}
			return fmt.Errorf("%w: migration failed and safely rolled back: %v", apperror.ErrInvalidInput, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("%w: target commit was not confirmed", apperror.ErrTransactionOutcomeUnknown)
	}
	return nil
}
func executeMySQLDDL(ctx context.Context, db *sql.DB, statement string) error {
	conn, err := db.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	if _, err := conn.ExecContext(ctx, "SET SESSION default_storage_engine = 'InnoDB'"); err != nil {
		return err
	}
	// One DDL statement, outside a transaction: MySQL implicitly commits schema changes.
	if _, err := conn.ExecContext(ctx, statement); err != nil {
		var serverError *mysql.MySQLError
		if errors.As(err, &serverError) {
			return fmt.Errorf("%w: MySQL schema change failed; inspect the schema before retrying (DDL is not transactionally rolled back): %v", apperror.ErrInvalidInput, err)
		}
		return fmt.Errorf("%w: MySQL schema change was not confirmed; inspect the target before retrying", apperror.ErrTransactionOutcomeUnknown)
	}
	return nil
}
