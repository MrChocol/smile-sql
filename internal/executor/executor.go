// Package executor defines the SQL execution-engine contract.
//
// The executor connects to a target datasource, runs a SQL script, and
// returns a Result whose Status/Mark fields the platform writes into
// execution_record (design §6-B).
package executor

import (
	"context"

	"sql-mgr/internal/model"
)

// Result captures the outcome of executing one script against one datasource.
//
//   Status — one of model.ExecStatusPending / ExecStatusSuccess / ExecStatusFailed
//   Err     — human-readable error text (empty when Status == success)
//   Mark    — model.MarkMethodAuto (auto-judged by the engine)
type Result struct {
	Status string
	Err    string
	Mark   string
}

// Executor is the contract every execution engine must satisfy.
type Executor interface {
	// Execute runs script against ds and returns a Result.
	// A non-nil error indicates the engine itself failed (e.g. context
	// cancelled); a failed SQL execution is represented by
	// Result{Status: ExecStatusFailed, Err: "..."} with a nil error.
	Execute(ctx context.Context, script model.SQLScript, ds model.Datasource) (Result, error)

	// TestConnection attempts to open a connection to ds and ping.
	// The password parameter is the plain-text password (decrypted by
	// the caller).  Returns nil on success, or a human-readable error.
	TestConnection(ctx context.Context, ds model.Datasource, password string) error
}
