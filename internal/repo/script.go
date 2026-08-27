// Package repo — script repository (workflow C).
//
// ScriptRepo manages CRUD for sql_script records.
package repo

import (
	"database/sql"
	"fmt"

	"sql-mgr/internal/model"
)

// ScriptRepo provides data access for sql_script records.
type ScriptRepo struct {
	db *sql.DB
}

// NewScriptRepo returns a ScriptRepo backed by the given *sql.DB.
func NewScriptRepo(db *sql.DB) *ScriptRepo {
	return &ScriptRepo{db: db}
}

// List returns scripts ordered by newest first.  When projectID > 0 the
// result is filtered to that project; otherwise every script is returned.
func (r *ScriptRepo) List(projectID int64) ([]model.SQLScript, error) {
	q := `SELECT id, project_id, title, description, content, sql_type, created_by, status, created_at FROM sql_script`
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
		return nil, fmt.Errorf("script list: %w", err)
	}
	defer rows.Close()

	var scripts []model.SQLScript
	for rows.Next() {
		var s model.SQLScript
		if err := rows.Scan(
			&s.ID, &s.ProjectID, &s.Title, &s.Description,
			&s.Content, &s.SqlType, &s.CreatedBy, &s.Status, &s.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("script scan: %w", err)
		}
		scripts = append(scripts, s)
	}
	return scripts, rows.Err()
}

// Create inserts a new script and returns its ID.
func (r *ScriptRepo) Create(s *model.SQLScript) (int64, error) {
	res, err := r.db.Exec(
		`INSERT INTO sql_script (project_id, title, description, content, sql_type, created_by, status)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		s.ProjectID, s.Title, s.Description, s.Content,
		s.SqlType, s.CreatedBy, s.Status,
	)
	if err != nil {
		return 0, fmt.Errorf("script create: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("script last insert id: %w", err)
	}
	return id, nil
}

// Get returns a single script by its ID.
func (r *ScriptRepo) Get(id int64) (model.SQLScript, error) {
	var s model.SQLScript
	err := r.db.QueryRow(
		`SELECT id, project_id, title, description, content, sql_type, created_by, status, created_at
		 FROM sql_script WHERE id = ?`,
		id,
	).Scan(
		&s.ID, &s.ProjectID, &s.Title, &s.Description,
		&s.Content, &s.SqlType, &s.CreatedBy, &s.Status, &s.CreatedAt,
	)
	if err != nil {
		return s, fmt.Errorf("script get %d: %w", id, err)
	}
	return s, nil
}

// Update modifies an existing script identified by s.ID.
func (r *ScriptRepo) Update(s *model.SQLScript) error {
	_, err := r.db.Exec(
		`UPDATE sql_script
		 SET project_id = ?, title = ?, description = ?, content = ?, sql_type = ?, created_by = ?, status = ?
		 WHERE id = ?`,
		s.ProjectID, s.Title, s.Description, s.Content,
		s.SqlType, s.CreatedBy, s.Status, s.ID,
	)
	if err != nil {
		return fmt.Errorf("script update %d: %w", s.ID, err)
	}
	return nil
}

// Delete removes a script by its ID.
func (r *ScriptRepo) Delete(id int64) error {
	_, err := r.db.Exec(`DELETE FROM sql_script WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("script delete %d: %w", id, err)
	}
	return nil
}
