// Package sqlguard validates the supported MySQL SQL subset before execution.
package sqlguard

import (
	"fmt"
	"strings"

	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/apperror"
	"github.com/xwb1989/sqlparser"
)

type MySQLMigration struct {
	Statements []string
	DDL        bool
	Tables     []string
}

func invalid(message string) error { return fmt.Errorf("%w: %s", apperror.ErrInvalidInput, message) }

func mysqlStatements(sql string) ([]string, error) {
	// Executable comments must not change the SQL that was validated.
	if strings.Contains(sql, "/*!") || strings.Contains(strings.ToUpper(sql), "/*M!") {
		return nil, invalid("executable SQL comments are not supported")
	}
	pieces, err := sqlparser.SplitStatementToPieces(sql)
	if err != nil {
		return nil, invalid("invalid MySQL SQL")
	}
	var statements []string
	for _, piece := range pieces {
		if len(mysqlTokens(piece)) > 0 {
			statements = append(statements, piece)
		}
	}
	if len(statements) == 0 {
		return nil, invalid("SQL cannot be empty")
	}
	return statements, nil
}

// mysqlTokens skips comments and retains decoded identifiers/strings for DDL checks.
func mysqlTokens(sql string) []string {
	tokenizer := sqlparser.NewStringTokenizer(sql)
	var tokens []string
	for {
		token, value := tokenizer.Scan()
		if token == 0 {
			break
		}
		if token == sqlparser.COMMENT {
			continue
		}
		if len(value) == 0 {
			value = []byte{byte(token)}
		}
		tokens = append(tokens, string(value))
	}
	return tokens
}

func checkTables(statement sqlparser.Statement, database string, readOnly bool) ([]string, error) {
	var tables []string
	err := sqlparser.Walk(func(node sqlparser.SQLNode) (bool, error) {
		switch node := node.(type) {
		case sqlparser.TableName:
			if !node.Qualifier.IsEmpty() && node.Qualifier.String() != database {
				return false, invalid("SQL must use the configured database only")
			}
			if name := node.Name.String(); name != "" && name != "dual" {
				tables = append(tables, name)
			}
		case *sqlparser.Select:
			if readOnly && node.Lock != "" {
				return false, invalid("locking SELECT is not supported")
			}
		case *sqlparser.Union:
			if readOnly && node.Lock != "" {
				return false, invalid("locking SELECT is not supported")
			}
		}
		return true, nil
	}, statement)
	return tables, err
}

func ValidateMySQLQuery(sql, database string) error {
	statements, err := mysqlStatements(sql)
	if err != nil {
		return err
	}
	if len(statements) != 1 {
		return invalid("query must contain exactly one SELECT")
	}
	statement, err := sqlparser.ParseStrictDDL(statements[0])
	if err != nil {
		return invalid("unsupported MySQL SELECT syntax")
	}
	if _, ok := statement.(sqlparser.SelectStatement); !ok {
		return invalid("only SELECT statements are permitted")
	}
	_, err = checkTables(statement, database, true)
	return err
}

// ParseMySQLMigration rejects mixed DDL/DML before any target changes occur.
// MySQL DDL is intentionally limited to one table/index statement per migration.
func ParseMySQLMigration(sql, database string) (MySQLMigration, error) {
	plan := MySQLMigration{}
	statements, err := mysqlStatements(sql)
	if err != nil {
		return plan, err
	}
	plan.Statements = statements
	for _, text := range statements {
		tokens := mysqlTokens(text)
		first := strings.ToUpper(tokens[0])
		if first == "CREATE" || first == "ALTER" || first == "DROP" {
			if len(statements) != 1 {
				return plan, invalid("MySQL schema changes require one statement per migration; do not mix DDL and DML")
			}
			if len(tokens) < 3 {
				return plan, invalid("incomplete schema statement")
			}
			kind := strings.ToUpper(tokens[1])
			if kind == "UNIQUE" && first == "CREATE" && len(tokens) > 3 {
				kind = strings.ToUpper(tokens[2])
			}
			if kind != "TABLE" && !((first == "CREATE" || first == "DROP") && kind == "INDEX") {
				return plan, invalid("only table and index schema changes are supported")
			}
			for i, token := range tokens {
				if token == "." && i > 0 && tokens[i-1] != database {
					return plan, invalid("schema changes must use the configured database only")
				}
				upper := strings.ToUpper(token)
				if upper == "SELECT" || upper == "DIRECTORY" || upper == "TABLESPACE" || upper == "TEMPORARY" {
					return plan, invalid("unsupported schema statement")
				}
				if upper == "ENGINE" {
					j := i + 1
					if j < len(tokens) && tokens[j] == "=" {
						j++
					}
					if j >= len(tokens) || !strings.EqualFold(tokens[j], "InnoDB") {
						return plan, invalid("only InnoDB tables are supported")
					}
				}
			}
			plan.DDL = true
			continue
		}
		statement, err := sqlparser.ParseStrictDDL(text)
		if err != nil {
			return plan, invalid("unsupported MySQL migration syntax")
		}
		switch statement.(type) {
		case *sqlparser.Insert, *sqlparser.Update, *sqlparser.Delete:
		default:
			return plan, invalid("use INSERT, UPDATE, DELETE, or one table/index schema change; transaction control is managed by the platform")
		}
		tables, err := checkTables(statement, database, false)
		if err != nil {
			return plan, err
		}
		plan.Tables = append(plan.Tables, tables...)
	}
	return plan, nil
}
