// Package repo -- datasource repository (workflow B: config management).
//
// DatasourceRepo manages CRUD for datasource records.  The password_enc
// field is stored as-is; encryption is the caller's responsibility.
package repo

import (
	"database/sql"

	"sql-mgr/internal/model"
)

// DatasourceRepo provides data access for datasource records.
type DatasourceRepo struct {
	db *sql.DB
}

// NewDatasourceRepo returns a DatasourceRepo backed by the given *sql.DB.
func NewDatasourceRepo(db *sql.DB) *DatasourceRepo {
	return &DatasourceRepo{db: db}
}

// List returns datasources optionally filtered by projectID and/or
// environmentID.  A value of 0 for either parameter means "no filter".
func (r *DatasourceRepo) List(projectID, environmentID int64) ([]model.Datasource, error) {
	query := `SELECT id, project_id, environment_id, name, db_type, host, port, db_name, username, password_enc, enabled, created_at FROM datasource WHERE 1=1`
	args := []any{}
	if projectID > 0 {
		query += ` AND project_id = ?`
		args = append(args, projectID)
	}
	if environmentID > 0 {
		query += ` AND environment_id = ?`
		args = append(args, environmentID)
	}
	query += ` ORDER BY id`

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var dss []model.Datasource
	for rows.Next() {
		var d model.Datasource
		var enabled int
		if err := rows.Scan(&d.ID, &d.ProjectID, &d.EnvironmentID, &d.Name, &d.DBType, &d.Host, &d.Port, &d.DBName, &d.Username, &d.PasswordEnc, &enabled, &d.CreatedAt); err != nil {
			return nil, err
		}
		d.Enabled = enabled != 0
		dss = append(dss, d)
	}
	return dss, rows.Err()
}

// Create inserts a new datasource and returns its ID.
func (r *DatasourceRepo) Create(d model.Datasource) (int64, error) {
	enabled := 0
	if d.Enabled {
		enabled = 1
	}
	res, err := r.db.Exec(
		`INSERT INTO datasource (project_id, environment_id, name, db_type, host, port, db_name, username, password_enc, enabled) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		d.ProjectID, d.EnvironmentID, d.Name, d.DBType, d.Host, d.Port, d.DBName, d.Username, d.PasswordEnc, enabled,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// Get returns a single datasource by ID.
func (r *DatasourceRepo) Get(id int64) (model.Datasource, error) {
	var d model.Datasource
	var enabled int
	err := r.db.QueryRow(
		`SELECT id, project_id, environment_id, name, db_type, host, port, db_name, username, password_enc, enabled, created_at FROM datasource WHERE id = ?`,
		id,
	).Scan(&d.ID, &d.ProjectID, &d.EnvironmentID, &d.Name, &d.DBType, &d.Host, &d.Port, &d.DBName, &d.Username, &d.PasswordEnc, &enabled, &d.CreatedAt)
	if err != nil {
		return d, err
	}
	d.Enabled = enabled != 0
	return d, nil
}

// Update modifies an existing datasource identified by d.ID.
func (r *DatasourceRepo) Update(d model.Datasource) error {
	enabled := 0
	if d.Enabled {
		enabled = 1
	}
	_, err := r.db.Exec(
		`UPDATE datasource SET project_id = ?, environment_id = ?, name = ?, db_type = ?, host = ?, port = ?, db_name = ?, username = ?, password_enc = ?, enabled = ? WHERE id = ?`,
		d.ProjectID, d.EnvironmentID, d.Name, d.DBType, d.Host, d.Port, d.DBName, d.Username, d.PasswordEnc, enabled, d.ID,
	)
	return err
}

// Delete removes a datasource by ID.
func (r *DatasourceRepo) Delete(id int64) error {
	_, err := r.db.Exec(`DELETE FROM datasource WHERE id = ?`, id)
	return err
}
