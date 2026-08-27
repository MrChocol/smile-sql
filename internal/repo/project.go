// Package repo -- project repository (workflow B: config management).
//
// ProjectRepo manages CRUD for project records.
package repo

import (
	"database/sql"

	"sql-mgr/internal/model"
)

// ProjectRepo provides data access for project records.
type ProjectRepo struct {
	db *sql.DB
}

// NewProjectRepo returns a ProjectRepo backed by the given *sql.DB.
func NewProjectRepo(db *sql.DB) *ProjectRepo {
	return &ProjectRepo{db: db}
}

// List returns all projects ordered by ID.
func (r *ProjectRepo) List() ([]model.Project, error) {
	rows, err := r.db.Query(
		`SELECT id, code, name, owner_team, pom_version_current, created_at FROM project ORDER BY id`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var projects []model.Project
	for rows.Next() {
		var p model.Project
		if err := rows.Scan(&p.ID, &p.Code, &p.Name, &p.OwnerTeam, &p.PomVersionCurrent, &p.CreatedAt); err != nil {
			return nil, err
		}
		projects = append(projects, p)
	}
	return projects, rows.Err()
}

// Create inserts a new project and returns its ID.
func (r *ProjectRepo) Create(p model.Project) (int64, error) {
	res, err := r.db.Exec(
		`INSERT INTO project (code, name, owner_team, pom_version_current) VALUES (?, ?, ?, ?)`,
		p.Code, p.Name, p.OwnerTeam, p.PomVersionCurrent,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// Get returns a single project by ID.
func (r *ProjectRepo) Get(id int64) (model.Project, error) {
	var p model.Project
	err := r.db.QueryRow(
		`SELECT id, code, name, owner_team, pom_version_current, created_at FROM project WHERE id = ?`,
		id,
	).Scan(&p.ID, &p.Code, &p.Name, &p.OwnerTeam, &p.PomVersionCurrent, &p.CreatedAt)
	return p, err
}

// Update modifies an existing project identified by p.ID.
func (r *ProjectRepo) Update(p model.Project) error {
	_, err := r.db.Exec(
		`UPDATE project SET code = ?, name = ?, owner_team = ?, pom_version_current = ? WHERE id = ?`,
		p.Code, p.Name, p.OwnerTeam, p.PomVersionCurrent, p.ID,
	)
	return err
}

// Delete removes a project by ID.
func (r *ProjectRepo) Delete(id int64) error {
	_, err := r.db.Exec(`DELETE FROM project WHERE id = ?`, id)
	return err
}
