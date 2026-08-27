// Package repo — release repository (workflow C).
//
// ReleaseRepo manages CRUD for sql_release records and the release_script
// many-to-many association table.
package repo

import (
	"database/sql"
	"fmt"

	"sql-mgr/internal/model"
)

// ReleaseRepo provides data access for sql_release and release_script records.
type ReleaseRepo struct {
	db *sql.DB
}

// NewReleaseRepo returns a ReleaseRepo backed by the given *sql.DB.
func NewReleaseRepo(db *sql.DB) *ReleaseRepo {
	return &ReleaseRepo{db: db}
}

// List returns releases ordered by newest first.  When projectID > 0 the
// result is filtered to that project; otherwise every release is returned.
func (r *ReleaseRepo) List(projectID int64) ([]model.SQLRelease, error) {
	q := `SELECT id, project_id, version_id, title, status, created_by, created_at FROM sql_release`
	var rows *sql.Rows
	var err error
	if projectID > 0 {
		q += ` WHERE project_id = ? ORDER BY id DESC`
		rows, err = r.db.Query(q, projectID)
	} else {
		q += ` ORDER BY id DESC`
		rows, err = r.db.Query(q)
	}
	if err != nil {
		return nil, fmt.Errorf("release list: %w", err)
	}
	defer rows.Close()

	var releases []model.SQLRelease
	for rows.Next() {
		var rel model.SQLRelease
		var versionID sql.NullInt64
		if err := rows.Scan(
			&rel.ID, &rel.ProjectID, &versionID,
			&rel.Title, &rel.Status, &rel.CreatedBy, &rel.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("release scan: %w", err)
		}
		if versionID.Valid {
			vid := versionID.Int64
			rel.VersionID = &vid
		}
		releases = append(releases, rel)
	}
	return releases, rows.Err()
}

// Create inserts a new release and returns its ID.
func (r *ReleaseRepo) Create(rel *model.SQLRelease) (int64, error) {
	// version_id is nullable — pass nil when VersionID is unset.
	var versionArg any
	if rel.VersionID != nil {
		versionArg = *rel.VersionID
	}
	res, err := r.db.Exec(
		`INSERT INTO sql_release (project_id, version_id, title, status, created_by)
		 VALUES (?, ?, ?, ?, ?)`,
		rel.ProjectID, versionArg, rel.Title, rel.Status, rel.CreatedBy,
	)
	if err != nil {
		return 0, fmt.Errorf("release create: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("release last insert id: %w", err)
	}
	return id, nil
}

// Get returns a single release by its ID.
func (r *ReleaseRepo) Get(id int64) (model.SQLRelease, error) {
	var rel model.SQLRelease
	var versionID sql.NullInt64
	err := r.db.QueryRow(
		`SELECT id, project_id, version_id, title, status, created_by, created_at
		 FROM sql_release WHERE id = ?`,
		id,
	).Scan(
		&rel.ID, &rel.ProjectID, &versionID,
		&rel.Title, &rel.Status, &rel.CreatedBy, &rel.CreatedAt,
	)
	if err != nil {
		return rel, fmt.Errorf("release get %d: %w", id, err)
	}
	if versionID.Valid {
		vid := versionID.Int64
		rel.VersionID = &vid
	}
	return rel, nil
}

// Update modifies an existing release identified by rel.ID.
func (r *ReleaseRepo) Update(rel *model.SQLRelease) error {
	var versionArg any
	if rel.VersionID != nil {
		versionArg = *rel.VersionID
	}
	_, err := r.db.Exec(
		`UPDATE sql_release
		 SET project_id = ?, version_id = ?, title = ?, status = ?
		 WHERE id = ?`,
		rel.ProjectID, versionArg, rel.Title, rel.Status, rel.ID,
	)
	if err != nil {
		return fmt.Errorf("release update %d: %w", rel.ID, err)
	}
	return nil
}

// Delete removes a release by its ID, along with all associated script links.
func (r *ReleaseRepo) Delete(id int64) error {
	// First delete associated script links
	_, err := r.db.Exec(`DELETE FROM release_script WHERE release_id = ?`, id)
	if err != nil {
		return fmt.Errorf("release delete scripts %d: %w", id, err)
	}
	_, err = r.db.Exec(`DELETE FROM sql_release WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("release delete %d: %w", id, err)
	}
	return nil
}

// AddScript links a script to a release by inserting a row into release_script.
func (r *ReleaseRepo) AddScript(releaseID, scriptID int64) error {
	_, err := r.db.Exec(
		`INSERT INTO release_script (release_id, script_id) VALUES (?, ?)`,
		releaseID, scriptID,
	)
	if err != nil {
		return fmt.Errorf("add script to release %d: %w", releaseID, err)
	}
	return nil
}

// RemoveScript unlinks a script from a release by deleting the release_script row.
func (r *ReleaseRepo) RemoveScript(releaseID, scriptID int64) error {
	_, err := r.db.Exec(
		`DELETE FROM release_script WHERE release_id = ? AND script_id = ?`,
		releaseID, scriptID,
	)
	if err != nil {
		return fmt.Errorf("remove script from release %d: %w", releaseID, err)
	}
	return nil
}

// ClearAllScripts removes all scripts from a release at once.
func (r *ReleaseRepo) ClearAllScripts(releaseID int64) (int64, error) {
	res, err := r.db.Exec(
		`DELETE FROM release_script WHERE release_id = ?`,
		releaseID,
	)
	if err != nil {
		return 0, fmt.Errorf("clear all scripts from release %d: %w", releaseID, err)
	}
	rows, _ := res.RowsAffected()
	return rows, nil
}

// ListScripts returns all scripts associated with the given release, ordered
// by their association insertion order (oldest first).
func (r *ReleaseRepo) ListScripts(releaseID int64) ([]model.SQLScript, error) {
	rows, err := r.db.Query(
		`SELECT s.id, s.project_id, s.title, s.description, s.content,
		        s.sql_type, s.created_by, s.status, s.created_at
		 FROM sql_script s
		 JOIN release_script rs ON rs.script_id = s.id
		 WHERE rs.release_id = ?
		 ORDER BY rs.id`,
		releaseID,
	)
	if err != nil {
		return nil, fmt.Errorf("list scripts for release %d: %w", releaseID, err)
	}
	defer rows.Close()

	var scripts []model.SQLScript
	for rows.Next() {
		var s model.SQLScript
		if err := rows.Scan(
			&s.ID, &s.ProjectID, &s.Title, &s.Description,
			&s.Content, &s.SqlType, &s.CreatedBy, &s.Status, &s.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("release script scan: %w", err)
		}
		scripts = append(scripts, s)
	}
	return scripts, rows.Err()
}
