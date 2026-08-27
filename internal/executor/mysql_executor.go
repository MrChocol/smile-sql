// Package executor — MySQL execution engine (workflow D).
//
// MySQLExecutor implements the Executor interface by connecting to a target
// MySQL datasource (via go-sql-driver/mysql), running the script content,
// and returning an auto-judged Result.
package executor

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql" // register the mysql driver

	"sql-mgr/internal/crypto"
	"sql-mgr/internal/model"
)

// defaultTimeout is the maximum duration a single script may run.
const defaultTimeout = 30 * time.Second

// MySQLExecutor connects to a target MySQL datasource, runs a script, and
// returns an auto-judged Result.
type MySQLExecutor struct {
	crypto crypto.Crypto
}

// NewMySQLExecutor returns a MySQLExecutor that uses c to decrypt
// datasource passwords before connecting.
func NewMySQLExecutor(c crypto.Crypto) *MySQLExecutor {
	return &MySQLExecutor{crypto: c}
}

// Ensure MySQLExecutor satisfies the Executor interface at compile time.
var _ Executor = (*MySQLExecutor)(nil)

// Execute connects to the target MySQL datasource described by ds, runs
// the script content, and returns a Result.
//
// On success it returns Result{Status: ExecStatusSuccess, Mark: MarkMethodAuto}.
// On a failed SQL execution it returns
// Result{Status: ExecStatusFailed, Err: "...", Mark: MarkMethodAuto} with a nil error.
// A non-nil error indicates the engine itself failed (e.g. context cancelled).
func (e *MySQLExecutor) Execute(ctx context.Context, script model.SQLScript, ds model.Datasource) (Result, error) {
	// Decrypt the datasource password.
	password, err := e.crypto.Decrypt(ds.PasswordEnc)
	if err != nil {
		return Result{
			Status: model.ExecStatusFailed,
			Err:    fmt.Sprintf("解密密码失败: %v", err),
			Mark:   model.MarkMethodAuto,
		}, nil
	}

	// Build the MySQL DSN: username:password@tcp(host:port)/db_name?parseTime=true&multiStatements=true
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?parseTime=true&multiStatements=true",
		ds.Username, password, ds.Host, ds.Port, ds.DBName)

	// Apply a timeout to the execution context.
	execCtx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	// Open a fresh connection to the target datasource (NOT the platform's
	// SQLite metadata DB).  Each execution opens and closes its own pool.
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return Result{
			Status: model.ExecStatusFailed,
			Err:    fmt.Sprintf("打开数据库连接失败: %v", err),
			Mark:   model.MarkMethodAuto,
		}, nil
	}
	defer db.Close()

	// Ping to verify connectivity before running the script.
	if err := db.PingContext(execCtx); err != nil {
		return Result{
			Status: model.ExecStatusFailed,
			Err:    fmt.Sprintf("连接数据库失败: %v", err),
			Mark:   model.MarkMethodAuto,
		}, nil
	}

	// Split the script into individual statements and execute them one by
	// one so that we can report which statement failed.
	stmts := splitSQLStatements(script.Content)
	if len(stmts) == 0 {
		return Result{
			Status: model.ExecStatusFailed,
			Err:    "脚本内容为空",
			Mark:   model.MarkMethodAuto,
		}, nil
	}

	for i, stmt := range stmts {
		if _, err := db.ExecContext(execCtx, stmt); err != nil {
			return Result{
				Status: model.ExecStatusFailed,
				Err:    fmt.Sprintf("第 %d 条语句执行失败: %v\n语句: %s", i+1, err, truncate(stmt, 200)),
				Mark:   model.MarkMethodAuto,
			}, nil
		}
	}

	return Result{
		Status: model.ExecStatusSuccess,
		Mark:   model.MarkMethodAuto,
	}, nil
}

// splitSQLStatements splits a multi-statement SQL script into individual
// statements.  It handles:
//   - Single-line -- comments
//   - Multi-line /* */ comments
//   - String literals (single and double quoted)
//   - Semicolons inside the above are ignored
func splitSQLStatements(content string) []string {
	var stmts []string
	var buf strings.Builder
	inSingle := false
	inDouble := false
	inLineComment := false
	inBlockComment := false

	runes := []rune(content)
	for i := 0; i < len(runes); i++ {
		r := runes[i]

		if inLineComment {
			buf.WriteRune(r)
			if r == '\n' {
				inLineComment = false
			}
			continue
		}
		if inBlockComment {
			buf.WriteRune(r)
			if r == '*' && i+1 < len(runes) && runes[i+1] == '/' {
				buf.WriteRune('/')
				i++
				inBlockComment = false
			}
			continue
		}
		if inSingle {
			buf.WriteRune(r)
			if r == '\\' && i+1 < len(runes) {
				buf.WriteRune(runes[i+1])
				i++
				continue
			}
			if r == '\'' {
				inSingle = false
			}
			continue
		}
		if inDouble {
			buf.WriteRune(r)
			if r == '\\' && i+1 < len(runes) {
				buf.WriteRune(runes[i+1])
				i++
				continue
			}
			if r == '"' {
				inDouble = false
			}
			continue
		}

		// Check for comment starts
		if r == '-' && i+1 < len(runes) && runes[i+1] == '-' {
			buf.WriteRune(r)
			buf.WriteRune('-')
			i++
			inLineComment = true
			continue
		}
		if r == '/' && i+1 < len(runes) && runes[i+1] == '*' {
			buf.WriteRune(r)
			buf.WriteRune('*')
			i++
			inBlockComment = true
			continue
		}

		if r == '\'' {
			inSingle = true
			buf.WriteRune(r)
			continue
		}
		if r == '"' {
			inDouble = true
			buf.WriteRune(r)
			continue
		}

		if r == ';' {
			stmt := strings.TrimSpace(buf.String())
			if stmt != "" {
				stmts = append(stmts, stmt)
			}
			buf.Reset()
			continue
		}

		buf.WriteRune(r)
	}

	// Last statement without trailing semicolon
	last := strings.TrimSpace(buf.String())
	if last != "" {
		stmts = append(stmts, last)
	}

	return stmts
}

// truncate shortens s to maxLen characters, appending "..." if truncated.
func truncate(s string, maxLen int) string {
	s = strings.TrimSpace(s)
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// TestConnection attempts to open a connection to ds and ping the server.
// The password parameter is the plain-text password (decrypted by caller).
// Returns nil on success, or a human-readable error on failure.
func (e *MySQLExecutor) TestConnection(ctx context.Context, ds model.Datasource, password string) error {
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?parseTime=true",
		ds.Username, password, ds.Host, ds.Port, ds.DBName)

	execCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return fmt.Errorf("打开连接失败: %w", err)
	}
	defer db.Close()

	if err := db.PingContext(execCtx); err != nil {
		return fmt.Errorf("连接失败: %v", err)
	}
	return nil
}
