// Package git defines the Git-integration contract.
//
// The platform pushes generated .sql files to an internal Git repository
// during the archive step (design §6-C). Sprint 0 only defines the
// interface; a real implementation will be added in a later sprint.
package git

import "context"

// GitClient is the contract for committing and pushing SQL files to the
// remote Git repository.
type GitClient interface {
	// CommitAndPush stages the given files (map of relative-path → content),
	// creates a single commit with msg, pushes to the configured remote, and
	// returns the resulting commit hash.
	CommitAndPush(ctx context.Context, files map[string]string, msg string) (hash string, err error)

	// InitRepo initialises the remote repository connection.  It tries to
	// clone the remote; if the remote is empty, it initialises a local repo,
	// creates a README, and pushes as the initial commit.  Returns nil on
	// success.
	InitRepo(ctx context.Context) error
}
