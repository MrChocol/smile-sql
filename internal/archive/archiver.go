// ArchiverImpl implements the Archiver interface using a *sql.DB for
// metadata persistence and a git.GitClient for repository operations
// (design §6-C).
package archive

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"sql-mgr/internal/git"
	"sql-mgr/internal/model"
)

// ArchiverImpl implements the Archiver interface.
//
// db is the metadata database, git is the Git client used to commit and
// push .sql files, and logger is an optional *log.Logger (may be nil).
type ArchiverImpl struct {
	db     *sql.DB
	git    git.GitClient
	logger *log.Logger
}

// NewArchiver returns an ArchiverImpl wired to the given DB and Git client.
func NewArchiver(db *sql.DB, gitClient git.GitClient) *ArchiverImpl {
	return &ArchiverImpl{
		db:  db,
		git: gitClient,
	}
}

// releaseWithProject holds release info joined with project info for
// archive processing.
type releaseWithProject struct {
	Release           model.SQLRelease
	ProjectCode       string
	ProjectName       string
	PomVersionCurrent string
}

// loadReleaseWithProject loads a release and its associated project info.
func (a *ArchiverImpl) loadReleaseWithProject(releaseID int64) (releaseWithProject, error) {
	var rwp releaseWithProject
	var versionID sql.NullInt64
	err := a.db.QueryRow(
		`SELECT r.id, r.project_id, r.version_id, r.title, r.status, r.created_by, r.created_at,
		        p.code, p.name, p.pom_version_current
		 FROM sql_release r
		 JOIN project p ON r.project_id = p.id
		 WHERE r.id = ?`,
		releaseID,
	).Scan(
		&rwp.Release.ID, &rwp.Release.ProjectID, &versionID,
		&rwp.Release.Title, &rwp.Release.Status, &rwp.Release.CreatedBy, &rwp.Release.CreatedAt,
		&rwp.ProjectCode, &rwp.ProjectName, &rwp.PomVersionCurrent,
	)
	if err != nil {
		return rwp, fmt.Errorf("load release %d: %w", releaseID, err)
	}
	if versionID.Valid {
		vid := versionID.Int64
		rwp.Release.VersionID = &vid
	}
	return rwp, nil
}

// loadReleaseScripts loads all scripts associated with the given release.
func (a *ArchiverImpl) loadReleaseScripts(releaseID int64) ([]model.SQLScript, error) {
	rows, err := a.db.Query(
		`SELECT s.id, s.project_id, s.title, s.description, s.content,
		        s.sql_type, s.created_by, s.status, s.created_at
		 FROM sql_script s
		 JOIN release_script rs ON rs.script_id = s.id
		 WHERE rs.release_id = ?
		 ORDER BY rs.id`,
		releaseID,
	)
	if err != nil {
		return nil, fmt.Errorf("load scripts for release %d: %w", releaseID, err)
	}
	defer rows.Close()

	var scripts []model.SQLScript
	for rows.Next() {
		var s model.SQLScript
		if err := rows.Scan(
			&s.ID, &s.ProjectID, &s.Title, &s.Description,
			&s.Content, &s.SqlType, &s.CreatedBy, &s.Status, &s.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan script: %w", err)
		}
		scripts = append(scripts, s)
	}
	return scripts, rows.Err()
}

// sanitizeFilename replaces spaces and special characters to produce a
// filesystem-safe filename component.
func sanitizeFilename(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, " ", "_")
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '_' || r == '-' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// bumpPatchVersion increments the patch component of a semver string.
// "1.0.0" becomes "1.0.1".  A non-semver or empty string defaults to "0.0.1".
func bumpPatchVersion(version string) string {
	if version == "" {
		return "0.0.1"
	}
	parts := strings.Split(version, ".")
	if len(parts) != 3 {
		return "0.0.1"
	}
	patch, err := strconv.Atoi(parts[2])
	if err != nil {
		return "0.0.1"
	}
	parts[2] = strconv.Itoa(patch + 1)
	return strings.Join(parts, ".")
}

// generateSQLFiles builds the map of relative file paths to SQL content
// (including a descriptive header block) for each script in the release.
//
// Repository layout:
//   {projectCode}/
//     archived/     — archived SQL scripts (committed on archive)
//     pending/      — pending / draft scripts (committed on script creation)
func generateSQLFiles(rwp releaseWithProject, scripts []model.SQLScript) map[string]string {
	files := make(map[string]string, len(scripts))
	archivedAt := time.Now().Format("2006-01-02 15:04:05")
	for _, s := range scripts {
		filename := fmt.Sprintf("%d_%s.sql", s.ID, sanitizeFilename(s.Title))
		relPath := fmt.Sprintf("%s/archived/%s", rwp.ProjectCode, filename)
		content := fmt.Sprintf(`-- ============================================================
-- Project: %s
-- Script: %s
-- Description: %s
-- Created By: %s
-- Created At: %s
-- Archived At: %s
-- ============================================================
%s
`,
			rwp.ProjectName, s.Title, s.Description,
			s.CreatedBy, s.CreatedAt, archivedAt, s.Content,
		)
		files[relPath] = content
	}
	return files
}

// Archive implements the Archiver interface.
//
// The flow:
//  1. Load the release (joined with project) by ID.
//  2. Load all scripts associated with the release.
//  3. Generate .sql files with header blocks.
//  4. Read the current project version.
//  5. Begin a DB transaction; if versionBump, bump the version inside it.
//  6. Call git.CommitAndPush to commit and push the .sql files.
//     If this fails the transaction is rolled back.
//  7. Insert an archive_log row.
//  8. Update the release status to 'archived'.
//  9. Update each script status to 'archived'.
//  10. Commit the transaction and return the archive_log entry.
func (a *ArchiverImpl) Archive(ctx context.Context, releaseID int64, commitMsg string, versionBump bool) (model.ArchiveLog, error) {
	if a.git == nil {
		return model.ArchiveLog{}, fmt.Errorf("archiver: git client is not configured")
	}

	// 1. Load the release with project info.
	rwp, err := a.loadReleaseWithProject(releaseID)
	if err != nil {
		return model.ArchiveLog{}, err
	}

	// 2. Load all scripts associated with the release.
	scripts, err := a.loadReleaseScripts(releaseID)
	if err != nil {
		return model.ArchiveLog{}, err
	}
	if len(scripts) == 0 {
		return model.ArchiveLog{}, fmt.Errorf("archive release %d: no scripts associated", releaseID)
	}

	// 3. Generate .sql files with header blocks.
	files := generateSQLFiles(rwp, scripts)

	// 4. Get the current project version.
	var versionFrom string
	err = a.db.QueryRow(
		`SELECT version FROM project_version WHERE project_id = ? AND is_current = 1`,
		rwp.Release.ProjectID,
	).Scan(&versionFrom)
	if err != nil && err != sql.ErrNoRows {
		return model.ArchiveLog{}, fmt.Errorf("load current version for project %d: %w", rwp.Release.ProjectID, err)
	}

	// Compute the new version if a bump is requested.
	versionTo := versionFrom
	if versionBump {
		versionTo = bumpPatchVersion(versionFrom)
	}

	// 5. Start a DB transaction for version bump, archive_log insert,
	//    and status updates.  Git push happens inside; if it fails the
	//    transaction is rolled back.
	tx, err := a.db.Begin()
	if err != nil {
		return model.ArchiveLog{}, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback() // no-op after Commit; rolls back on early return

	// 5a. Version bump (inside transaction).
	if versionBump {
		if _, err := tx.Exec(
			`UPDATE project_version SET is_current = 0 WHERE project_id = ?`,
			rwp.Release.ProjectID,
		); err != nil {
			return model.ArchiveLog{}, fmt.Errorf("unset current version: %w", err)
		}
		if _, err := tx.Exec(
			`INSERT INTO project_version (project_id, version, is_current, source) VALUES (?, ?, 1, ?)`,
			rwp.Release.ProjectID, versionTo, model.VersionSourceAutoInc,
		); err != nil {
			return model.ArchiveLog{}, fmt.Errorf("insert new version: %w", err)
		}
		if _, err := tx.Exec(
			`UPDATE project SET pom_version_current = ? WHERE id = ?`,
			versionTo, rwp.Release.ProjectID,
		); err != nil {
			return model.ArchiveLog{}, fmt.Errorf("update project pom_version_current: %w", err)
		}
	}

	// 6. Call git to commit and push the .sql files.
	commitHash, err := a.git.CommitAndPush(ctx, files, commitMsg)
	if err != nil {
		return model.ArchiveLog{}, fmt.Errorf("git commit and push: %w", err)
	}

	// 7. Insert an archive_log row.
	archivedBy := "system"
	res, err := tx.Exec(
		`INSERT INTO archive_log (release_id, project_id, version_from, version_to, git_commit_hash, commit_message, archived_by)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		releaseID, rwp.Release.ProjectID, versionFrom, versionTo,
		commitHash, commitMsg, archivedBy,
	)
	if err != nil {
		return model.ArchiveLog{}, fmt.Errorf("insert archive_log: %w", err)
	}
	logID, err := res.LastInsertId()
	if err != nil {
		return model.ArchiveLog{}, fmt.Errorf("archive_log last insert id: %w", err)
	}

	// 8. Update release status to 'archived'.
	if _, err := tx.Exec(
		`UPDATE sql_release SET status = ? WHERE id = ?`,
		model.ReleaseStatusArchived, releaseID,
	); err != nil {
		return model.ArchiveLog{}, fmt.Errorf("update release status: %w", err)
	}

	// 9. Update each script status to 'archived'.
	for _, s := range scripts {
		if _, err := tx.Exec(
			`UPDATE sql_script SET status = ? WHERE id = ?`,
			model.ScriptStatusArchived, s.ID,
		); err != nil {
			return model.ArchiveLog{}, fmt.Errorf("update script %d status: %w", s.ID, err)
		}
	}

	// 10. Commit the transaction.
	if err := tx.Commit(); err != nil {
		return model.ArchiveLog{}, fmt.Errorf("commit tx: %w", err)
	}

	// 11. Return the archive_log entry.
	return model.ArchiveLog{
		ID:            logID,
		ReleaseID:     releaseID,
		ProjectID:     rwp.Release.ProjectID,
		VersionFrom:   versionFrom,
		VersionTo:     versionTo,
		GitCommitHash: commitHash,
		CommitMessage: commitMsg,
		ArchivedBy:    archivedBy,
		ArchivedAt:    time.Now().Format("2006-01-02 15:04:05"),
	}, nil
}
