// Package repo -- project_version repository (workflow B: config management).
//
// VersionRepo manages CRUD for project_version records.
package repo

import (
	"database/sql"
	"fmt"

	"sql-mgr/internal/model"
)

// VersionRepo provides data access for project_version records.
type VersionRepo struct {
	db *sql.DB
}

// NewVersionRepo returns a VersionRepo backed by the given *sql.DB.
func NewVersionRepo(db *sql.DB) *VersionRepo {
	return &VersionRepo{db: db}
}

// ListByProject returns all versions for the given project, ordered by ID.
func (r *VersionRepo) ListByProject(projectID int64) ([]model.ProjectVersion, error) {
	rows, err := r.db.Query(
		`SELECT id, project_id, version, is_current, source, created_at FROM project_version WHERE project_id = ? ORDER BY id`,
		projectID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var versions []model.ProjectVersion
	for rows.Next() {
		var v model.ProjectVersion
		var isCurrent int
		if err := rows.Scan(&v.ID, &v.ProjectID, &v.Version, &isCurrent, &v.Source, &v.CreatedAt); err != nil {
			return nil, err
		}
		v.IsCurrent = isCurrent != 0
		versions = append(versions, v)
	}
	return versions, rows.Err()
}

// ListAll returns all versions across all projects, ordered by ID.
func (r *VersionRepo) ListAll() ([]model.ProjectVersion, error) {
	rows, err := r.db.Query(
		`SELECT id, project_id, version, is_current, source, created_at FROM project_version ORDER BY id`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var versions []model.ProjectVersion
	for rows.Next() {
		var v model.ProjectVersion
		var isCurrent int
		if err := rows.Scan(&v.ID, &v.ProjectID, &v.Version, &isCurrent, &v.Source, &v.CreatedAt); err != nil {
			return nil, err
		}
		v.IsCurrent = isCurrent != 0
		versions = append(versions, v)
	}
	return versions, rows.Err()
}

// Create inserts a new project version and returns its ID.
func (r *VersionRepo) Create(v model.ProjectVersion) (int64, error) {
	isCurrent := 0
	if v.IsCurrent {
		isCurrent = 1
	}
	res, err := r.db.Exec(
		`INSERT INTO project_version (project_id, version, is_current, source) VALUES (?, ?, ?, ?)`,
		v.ProjectID, v.Version, isCurrent, v.Source,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// SetCurrent atomically unsets is_current on all versions of the given
// project, then sets is_current=1 on the specified version.
func (r *VersionRepo) SetCurrent(versionID, projectID int64) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}

	if _, err = tx.Exec(`UPDATE project_version SET is_current = 0 WHERE project_id = ?`, projectID); err != nil {
		tx.Rollback()
		return fmt.Errorf("unset current versions: %w", err)
	}
	if _, err = tx.Exec(`UPDATE project_version SET is_current = 1 WHERE id = ?`, versionID); err != nil {
		tx.Rollback()
		return fmt.Errorf("set current version: %w", err)
	}

	return tx.Commit()
}
