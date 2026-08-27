// Package repo -- archive_log repository (workflow E: archive/Git integration).
//
// ArchiveRepo manages CRUD for archive_log records.
package repo

import (
	"database/sql"
	"fmt"

	"sql-mgr/internal/model"
)

// ArchiveRepo provides data access for archive_log records.
type ArchiveRepo struct {
	db *sql.DB
}

// NewArchiveRepo returns an ArchiveRepo backed by the given *sql.DB.
func NewArchiveRepo(db *sql.DB) *ArchiveRepo {
	return &ArchiveRepo{db: db}
}

// Create inserts a new archive_log entry and returns its ID.
func (r *ArchiveRepo) Create(log *model.ArchiveLog) (int64, error) {
	res, err := r.db.Exec(
		`INSERT INTO archive_log (release_id, project_id, version_from, version_to, git_commit_hash, commit_message, archived_by)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		log.ReleaseID, log.ProjectID, log.VersionFrom, log.VersionTo,
		log.GitCommitHash, log.CommitMessage, log.ArchivedBy,
	)
	if err != nil {
		return 0, fmt.Errorf("archive_log create: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("archive_log last insert id: %w", err)
	}
	return id, nil
}

// Get returns a single archive_log entry by its ID.
func (r *ArchiveRepo) Get(id int64) (model.ArchiveLog, error) {
	var log model.ArchiveLog
	err := r.db.QueryRow(
		`SELECT id, release_id, project_id, version_from, version_to,
		        git_commit_hash, commit_message, archived_by, archived_at
		 FROM archive_log WHERE id = ?`,
		id,
	).Scan(
		&log.ID, &log.ReleaseID, &log.ProjectID, &log.VersionFrom, &log.VersionTo,
		&log.GitCommitHash, &log.CommitMessage, &log.ArchivedBy, &log.ArchivedAt,
	)
	if err != nil {
		return log, fmt.Errorf("archive_log get %d: %w", id, err)
	}
	return log, nil
}

// List returns all archive_log entries ordered by ID descending (newest first).
func (r *ArchiveRepo) List() ([]model.ArchiveLog, error) {
	rows, err := r.db.Query(
		`SELECT id, release_id, project_id, version_from, version_to,
		        git_commit_hash, commit_message, archived_by, archived_at
		 FROM archive_log ORDER BY id DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("archive_log list: %w", err)
	}
	defer rows.Close()

	var logs []model.ArchiveLog
	for rows.Next() {
		var log model.ArchiveLog
		if err := rows.Scan(
			&log.ID, &log.ReleaseID, &log.ProjectID, &log.VersionFrom, &log.VersionTo,
			&log.GitCommitHash, &log.CommitMessage, &log.ArchivedBy, &log.ArchivedAt,
		); err != nil {
			return nil, fmt.Errorf("archive_log scan: %w", err)
		}
		logs = append(logs, log)
	}
	return logs, rows.Err()
}

// ListByProject returns all archive_log entries for the given project,
// ordered by ID descending (newest first).
func (r *ArchiveRepo) ListByProject(projectID int64) ([]model.ArchiveLog, error) {
	rows, err := r.db.Query(
		`SELECT id, release_id, project_id, version_from, version_to,
		        git_commit_hash, commit_message, archived_by, archived_at
		 FROM archive_log WHERE project_id = ? ORDER BY id DESC`,
		projectID,
	)
	if err != nil {
		return nil, fmt.Errorf("archive_log list by project %d: %w", projectID, err)
	}
	defer rows.Close()

	var logs []model.ArchiveLog
	for rows.Next() {
		var log model.ArchiveLog
		if err := rows.Scan(
			&log.ID, &log.ReleaseID, &log.ProjectID, &log.VersionFrom, &log.VersionTo,
			&log.GitCommitHash, &log.CommitMessage, &log.ArchivedBy, &log.ArchivedAt,
		); err != nil {
			return nil, fmt.Errorf("archive_log scan: %w", err)
		}
		logs = append(logs, log)
	}
	return logs, rows.Err()
}
