// Package repo -- environment repository (workflow B: config management).
//
// EnvironmentRepo manages CRUD for environment records.
package repo

import (
	"database/sql"

	"sql-mgr/internal/model"
)

// EnvironmentRepo provides data access for environment records.
type EnvironmentRepo struct {
	db *sql.DB
}

// NewEnvironmentRepo returns an EnvironmentRepo backed by the given *sql.DB.
func NewEnvironmentRepo(db *sql.DB) *EnvironmentRepo {
	return &EnvironmentRepo{db: db}
}

// List returns all environments ordered by promote_order then ID.
func (r *EnvironmentRepo) List() ([]model.Environment, error) {
	rows, err := r.db.Query(
		`SELECT id, code, name, promote_order FROM environment ORDER BY promote_order, id`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var envs []model.Environment
	for rows.Next() {
		var e model.Environment
		if err := rows.Scan(&e.ID, &e.Code, &e.Name, &e.PromoteOrder); err != nil {
			return nil, err
		}
		envs = append(envs, e)
	}
	return envs, rows.Err()
}

// Create inserts a new environment and returns its ID.
func (r *EnvironmentRepo) Create(e model.Environment) (int64, error) {
	res, err := r.db.Exec(
		`INSERT INTO environment (code, name, promote_order) VALUES (?, ?, ?)`,
		e.Code, e.Name, e.PromoteOrder,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// Get returns a single environment by ID.
func (r *EnvironmentRepo) Get(id int64) (model.Environment, error) {
	var e model.Environment
	err := r.db.QueryRow(
		`SELECT id, code, name, promote_order FROM environment WHERE id = ?`,
		id,
	).Scan(&e.ID, &e.Code, &e.Name, &e.PromoteOrder)
	return e, err
}

// Update modifies an existing environment identified by e.ID.
func (r *EnvironmentRepo) Update(e model.Environment) error {
	_, err := r.db.Exec(
		`UPDATE environment SET code = ?, name = ?, promote_order = ? WHERE id = ?`,
		e.Code, e.Name, e.PromoteOrder, e.ID,
	)
	return err
}

// Delete removes an environment by ID.
func (r *EnvironmentRepo) Delete(id int64) error {
	_, err := r.db.Exec(`DELETE FROM environment WHERE id = ?`, id)
	return err
}
