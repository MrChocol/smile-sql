// Package repo — execution repository (workflow D: execution engine).
//
// ExecutionRepo manages CRUD for execution_record rows — the single source of
// truth for every script pushed against a datasource.
package repo

import (
	"database/sql"
	"fmt"
	"strings"

	"sql-mgr/internal/model"
)

// ExecutionRepo provides data access for execution_record rows.
type ExecutionRepo struct {
	db *sql.DB
}

// NewExecutionRepo returns an ExecutionRepo backed by the given *sql.DB.
func NewExecutionRepo(db *sql.DB) *ExecutionRepo {
	return &ExecutionRepo{db: db}
}

// Create inserts a new execution record and returns its ID.
func (r *ExecutionRepo) Create(rec *model.ExecutionRecord) (int64, error) {
	res, err := r.db.Exec(
		`INSERT INTO execution_record
		   (release_id, script_id, datasource_id, environment_id,
		    status, mark_method, result, executed_by, marked_by, error)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		rec.ReleaseID, rec.ScriptID, rec.DatasourceID, rec.EnvironmentID,
		rec.Status, rec.MarkMethod, rec.Result, rec.ExecutedBy,
		rec.MarkedBy, rec.Error,
	)
	if err != nil {
		return 0, fmt.Errorf("execution create: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("execution last insert id: %w", err)
	}
	return id, nil
}

// Get returns a single execution record by its ID.
func (r *ExecutionRepo) Get(id int64) (model.ExecutionRecord, error) {
	var rec model.ExecutionRecord
	var executedAt, markedAt sql.NullString
	err := r.db.QueryRow(
		`SELECT id, release_id, script_id, datasource_id, environment_id,
		        status, mark_method, result, executed_by, executed_at,
		        marked_by, marked_at, error
		 FROM execution_record WHERE id = ?`,
		id,
	).Scan(
		&rec.ID, &rec.ReleaseID, &rec.ScriptID, &rec.DatasourceID,
		&rec.EnvironmentID, &rec.Status, &rec.MarkMethod, &rec.Result,
		&rec.ExecutedBy, &executedAt, &rec.MarkedBy, &markedAt, &rec.Error,
	)
	if err != nil {
		return rec, fmt.Errorf("execution get %d: %w", id, err)
	}
	if executedAt.Valid {
		rec.ExecutedAt = &executedAt.String
	}
	if markedAt.Valid {
		rec.MarkedAt = &markedAt.String
	}
	return rec, nil
}

// ListByRelease returns all execution records for the given release,
// ordered by ID ascending.
func (r *ExecutionRepo) ListByRelease(releaseID int64) ([]model.ExecutionRecord, error) {
	rows, err := r.db.Query(
		`SELECT id, release_id, script_id, datasource_id, environment_id,
		        status, mark_method, result, executed_by, executed_at,
		        marked_by, marked_at, error
		 FROM execution_record
		 WHERE release_id = ?
		 ORDER BY id`,
		releaseID,
	)
	if err != nil {
		return nil, fmt.Errorf("execution list by release %d: %w", releaseID, err)
	}
	defer rows.Close()
	return scanExecutionRows(rows)
}

// ListByReleaseAndDatasource returns execution records matching both the
// given release and datasource, ordered by ID ascending.
func (r *ExecutionRepo) ListByReleaseAndDatasource(releaseID, datasourceID int64) ([]model.ExecutionRecord, error) {
	rows, err := r.db.Query(
		`SELECT id, release_id, script_id, datasource_id, environment_id,
		        status, mark_method, result, executed_by, executed_at,
		        marked_by, marked_at, error
		 FROM execution_record
		 WHERE release_id = ? AND datasource_id = ?
		 ORDER BY id`,
		releaseID, datasourceID,
	)
	if err != nil {
		return nil, fmt.Errorf("execution list by release %d and datasource %d: %w", releaseID, datasourceID, err)
	}
	defer rows.Close()
	return scanExecutionRows(rows)
}

// ListWithFilter returns execution records matching the optional filter
// criteria.  ProjectID is resolved via a JOIN on sql_release.  When all
// filter fields are nil every record is returned.
func (r *ExecutionRepo) ListWithFilter(filter model.ExecutionFilter) ([]model.ExecutionRecord, error) {
	var sb strings.Builder
	sb.WriteString(
		`SELECT er.id, er.release_id, er.script_id, er.datasource_id, er.environment_id,
		        er.status, er.mark_method, er.result, er.executed_by, er.executed_at,
		        er.marked_by, er.marked_at, er.error
		 FROM execution_record er
		 JOIN sql_release r ON er.release_id = r.id
		 WHERE 1=1`,
	)
	var args []any
	if filter.ProjectID != nil {
		sb.WriteString(` AND r.project_id = ?`)
		args = append(args, *filter.ProjectID)
	}
	if filter.EnvironmentID != nil {
		sb.WriteString(` AND er.environment_id = ?`)
		args = append(args, *filter.EnvironmentID)
	}
	if filter.Status != nil {
		sb.WriteString(` AND er.status = ?`)
		args = append(args, *filter.Status)
	}
	sb.WriteString(` ORDER BY er.id`)

	rows, err := r.db.Query(sb.String(), args...)
	if err != nil {
		return nil, fmt.Errorf("execution list with filter: %w", err)
	}
	defer rows.Close()
	return scanExecutionRows(rows)
}

// UpdateResult sets the status, result text, and error for an execution
// record, and stamps executed_at with the current time.
func (r *ExecutionRepo) UpdateResult(id int64, status, result, errStr string) error {
	_, err := r.db.Exec(
		`UPDATE execution_record
		 SET status = ?, result = ?, error = ?, executed_at = datetime('now')
		 WHERE id = ?`,
		status, result, errStr, id,
	)
	if err != nil {
		return fmt.Errorf("execution update result %d: %w", id, err)
	}
	return nil
}

// MarkManual sets the status and mark_method='manual' for an execution
// record, recording who performed the manual mark and when.
func (r *ExecutionRepo) MarkManual(id int64, status, markedBy string) error {
	_, err := r.db.Exec(
		`UPDATE execution_record
		 SET status = ?, mark_method = 'manual', marked_by = ?, marked_at = datetime('now')
		 WHERE id = ?`,
		status, markedBy, id,
	)
	if err != nil {
		return fmt.Errorf("execution mark manual %d: %w", id, err)
	}
	return nil
}

// Delete removes an execution record by its ID.
func (r *ExecutionRepo) Delete(id int64) error {
	_, err := r.db.Exec(`DELETE FROM execution_record WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("execution delete %d: %w", id, err)
	}
	return nil
}

// scanExecutionRows iterates rows and returns a slice of ExecutionRecord.
// It is shared by every List* method to avoid duplicated scan logic.
func scanExecutionRows(rows *sql.Rows) ([]model.ExecutionRecord, error) {
	var records []model.ExecutionRecord
	for rows.Next() {
		var rec model.ExecutionRecord
		var executedAt, markedAt sql.NullString
		if err := rows.Scan(
			&rec.ID, &rec.ReleaseID, &rec.ScriptID, &rec.DatasourceID,
			&rec.EnvironmentID, &rec.Status, &rec.MarkMethod, &rec.Result,
			&rec.ExecutedBy, &executedAt, &rec.MarkedBy, &markedAt, &rec.Error,
		); err != nil {
			return nil, fmt.Errorf("execution scan: %w", err)
		}
		if executedAt.Valid {
			rec.ExecutedAt = &executedAt.String
		}
		if markedAt.Valid {
			rec.MarkedAt = &markedAt.String
		}
		records = append(records, rec)
	}
	return records, rows.Err()
}
