// Package web wires together the HTTP layer: session/auth, template
// rendering, and the Server that holds shared dependencies.
//
// Sprint 0 provides: Deps, CurrentUser, Server (with Routes + render),
// and login/auth middleware.  Module-specific handlers are added by
// each workflow agent.
package web

import (
	"context"
	"database/sql"

	"sql-mgr/internal/archive"
	"sql-mgr/internal/crypto"
	"sql-mgr/internal/executor"
	"sql-mgr/internal/git"
	"sql-mgr/internal/model"
)

// Deps bundles all shared dependencies that HTTP handlers need.
// During Sprint 0, Executor/Git/Archiver may be nil — handlers should
// nil-check before calling them.
type Deps struct {
	DB       *sql.DB
	Crypto   crypto.Crypto
	Executor executor.Executor // Sprint 0: may be nil
	Git      git.GitClient     // Sprint 0: may be nil
	Archiver archive.Archiver  // Sprint 0: may be nil
}

// contextKey is an unexported type used for storing the current user
// in a context without collision.
type contextKey string

const userCtxKey contextKey = "currentUser"

// CurrentUser extracts the authenticated user from the request context.
// Returns (zero User, false) when no user is set (e.g. on public routes).
func CurrentUser(ctx context.Context) (model.User, bool) {
	u, ok := ctx.Value(userCtxKey).(model.User)
	return u, ok
}

// withUser returns a new context that carries the given user so that
// downstream handlers can retrieve it via CurrentUser.
func withUser(ctx context.Context, u model.User) context.Context {
	return context.WithValue(ctx, userCtxKey, u)
}
