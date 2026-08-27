// Package archive defines the archive (Git commit + version bump) contract.
//
// Archiving a release generates .sql files, commits them to the remote Git
// repository, bumps the project version, and writes an archive_log row
// (design §6-C).
package archive

import (
	"context"

	"sql-mgr/internal/model"
)

// ArchiveLog is an alias so callers can reference archive.ArchiveLog without
// importing the model package separately.
type ArchiveLog = model.ArchiveLog

// Archiver is the contract every archiver implementation must satisfy.
type Archiver interface {
	// Archive archives the given release: generates .sql files, commits and
	// pushes to Git (commitMsg), optionally bumps the project version
	// (versionBump), and returns the archive_log row that was written.
	Archive(ctx context.Context, releaseID int64, commitMsg string, versionBump bool) (model.ArchiveLog, error)
}
