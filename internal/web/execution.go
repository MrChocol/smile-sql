// Package web — execution handlers (workflow D: execution engine).
//
// This file implements the execution record list, detail, push (execute),
// and manual-mark handlers, plus the RegisterExecution route registration
// that wires every execution route onto the mux.
package web

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"sql-mgr/internal/model"
	"sql-mgr/internal/repo"
)

// --------------------------------------------------------------------------- //
// View data structs
// --------------------------------------------------------------------------- //

// executionWithNames extends ExecutionRecord with resolved display names
// (project, script title, datasource name, environment name) obtained via
// JOINs at query time.
type executionWithNames struct {
	model.ExecutionRecord
	ProjectName     string
	ScriptTitle     string
	DatasourceName  string
	EnvironmentName string
}

// executionListView holds data for the execution list page.
type executionListView struct {
	Records         []executionWithNames
	Projects        []model.Project
	Environments    []model.Environment
	Filter          model.ExecutionFilter
	FilterProjectID int64
	FilterEnvID     int64
	FilterStatus    string
	FilterReleaseID int64
}

// executionDetailView holds data for the execution detail page.
type executionDetailView struct {
	Record executionWithNames
}

// --------------------------------------------------------------------------- //
// Helpers
// --------------------------------------------------------------------------- //

// loadEnvironmentsForForms queries the environment table and returns all
// environments ordered by promote_order, suitable for dropdown lists.
func (s *Server) loadEnvironmentsForForms() ([]model.Environment, error) {
	er := repo.NewEnvironmentRepo(s.deps.DB)
	return er.List()
}

// queryExecutionsWithNames runs the execution JOIN query and returns
// enriched rows with display names resolved.
func (s *Server) queryExecutionsWithNames(query string, args ...any) ([]executionWithNames, error) {
	rows, err := s.deps.DB.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("query executions: %w", err)
	}
	defer rows.Close()

	var result []executionWithNames
	for rows.Next() {
		var ewn executionWithNames
		var executedAt, markedAt sql.NullString
		if err := rows.Scan(
			&ewn.ID, &ewn.ReleaseID, &ewn.ScriptID, &ewn.DatasourceID,
			&ewn.EnvironmentID, &ewn.Status, &ewn.MarkMethod, &ewn.Result,
			&ewn.ExecutedBy, &executedAt, &ewn.MarkedBy, &markedAt, &ewn.Error,
			&ewn.ProjectName, &ewn.ScriptTitle,
			&ewn.DatasourceName, &ewn.EnvironmentName,
		); err != nil {
			return nil, fmt.Errorf("scan execution: %w", err)
		}
		if executedAt.Valid {
			s := executedAt.String
			ewn.ExecutedAt = &s
		}
		if markedAt.Valid {
			s := markedAt.String
			ewn.MarkedAt = &s
		}
		result = append(result, ewn)
	}
	return result, rows.Err()
}

// getExecutionWithNames returns a single execution record with all display
// names resolved via JOINs.
func (s *Server) getExecutionWithNames(id int64) (executionWithNames, error) {
	var ewn executionWithNames
	var executedAt, markedAt sql.NullString
	err := s.deps.DB.QueryRow(
		`SELECT er.id, er.release_id, er.script_id, er.datasource_id, er.environment_id,
		        er.status, er.mark_method, er.result, er.executed_by, er.executed_at,
		        er.marked_by, er.marked_at, er.error,
		        p.name, s.title, d.name, e.name
		 FROM execution_record er
		 JOIN sql_release r ON er.release_id = r.id
		 JOIN project p ON r.project_id = p.id
		 JOIN sql_script s ON er.script_id = s.id
		 JOIN datasource d ON er.datasource_id = d.id
		 JOIN environment e ON er.environment_id = e.id
		 WHERE er.id = ?`,
		id,
	).Scan(
		&ewn.ID, &ewn.ReleaseID, &ewn.ScriptID, &ewn.DatasourceID,
		&ewn.EnvironmentID, &ewn.Status, &ewn.MarkMethod, &ewn.Result,
		&ewn.ExecutedBy, &executedAt, &ewn.MarkedBy, &markedAt, &ewn.Error,
		&ewn.ProjectName, &ewn.ScriptTitle,
		&ewn.DatasourceName, &ewn.EnvironmentName,
	)
	if err != nil {
		return ewn, fmt.Errorf("get execution %d: %w", id, err)
	}
	if executedAt.Valid {
		s := executedAt.String
		ewn.ExecutedAt = &s
	}
	if markedAt.Valid {
		s := markedAt.String
		ewn.MarkedAt = &s
	}
	return ewn, nil
}

// parseScriptIDs parses a comma-separated list of script IDs into a slice of int64.
// Returns an empty slice if the input is empty.
func parseScriptIDs(raw string) ([]int64, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	parts := strings.Split(raw, ",")
	ids := make([]int64, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		id, err := strconv.ParseInt(p, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("无效的脚本ID: %s", p)
		}
		ids = append(ids, id)
	}
	return ids, nil
}

// filterScriptsByIDs filters the given script slice to only include scripts
// whose IDs are in the provided idSet. The result preserves the original order.
// It also validates that every requested ID exists in the script list (to
// prevent executing scripts that don't belong to the release).
func filterScriptsByIDs(scripts []model.SQLScript, ids []int64) ([]model.SQLScript, error) {
	// Build a set of valid script IDs from the release.
	validSet := make(map[int64]bool, len(scripts))
	for _, s := range scripts {
		validSet[s.ID] = true
	}

	// Validate all requested IDs belong to the release.
	for _, id := range ids {
		if !validSet[id] {
			return nil, fmt.Errorf("脚本 %d 不属于该发布", id)
		}
	}

	// Filter scripts preserving original order.
	idSet := make(map[int64]bool, len(ids))
	for _, id := range ids {
		idSet[id] = true
	}
	var filtered []model.SQLScript
	for _, s := range scripts {
		if idSet[s.ID] {
			filtered = append(filtered, s)
		}
	}
	return filtered, nil
}

// --------------------------------------------------------------------------- //
// Handlers — Executions
// --------------------------------------------------------------------------- //

// listExecutions handles GET /executions.  Optional query parameters
// ?project=X&env=Y&status=Z&release=R filter the list.
func (s *Server) listExecutions(w http.ResponseWriter, r *http.Request) {
	projectID, _ := strconv.ParseInt(r.URL.Query().Get("project"), 10, 64)
	envID, _ := strconv.ParseInt(r.URL.Query().Get("env"), 10, 64)
	status := r.URL.Query().Get("status")
	releaseID, _ := strconv.ParseInt(r.URL.Query().Get("release"), 10, 64)

	query := `SELECT er.id, er.release_id, er.script_id, er.datasource_id, er.environment_id,
	                 er.status, er.mark_method, er.result, er.executed_by, er.executed_at,
	                 er.marked_by, er.marked_at, er.error,
	                 p.name, s.title, d.name, e.name
	          FROM execution_record er
	          JOIN sql_release r ON er.release_id = r.id
	          JOIN project p ON r.project_id = p.id
	          JOIN sql_script s ON er.script_id = s.id
	          JOIN datasource d ON er.datasource_id = d.id
	          JOIN environment e ON er.environment_id = e.id
	          WHERE 1=1`
	var args []any
	if projectID > 0 {
		query += ` AND r.project_id = ?`
		args = append(args, projectID)
	}
	if envID > 0 {
		query += ` AND er.environment_id = ?`
		args = append(args, envID)
	}
	if status != "" {
		query += ` AND er.status = ?`
		args = append(args, status)
	}
	if releaseID > 0 {
		query += ` AND er.release_id = ?`
		args = append(args, releaseID)
	}
	query += ` ORDER BY er.id DESC`

	records, err := s.queryExecutionsWithNames(query, args...)
	if err != nil {
		s.render(w, r, "execution.html", nil, "加载执行记录失败: "+err.Error())
		return
	}

	projects, perr := s.loadProjectsForForms()
	if perr != nil {
		s.render(w, r, "execution.html", nil, "加载项目失败: "+perr.Error())
		return
	}

	envs, eerr := s.loadEnvironmentsForForms()
	if eerr != nil {
		s.render(w, r, "execution.html", nil, "加载环境失败: "+eerr.Error())
		return
	}

	s.render(w, r, "execution.html", executionListView{
		Records:         records,
		Projects:        projects,
		Environments:    envs,
		FilterProjectID: projectID,
		FilterEnvID:     envID,
		FilterStatus:    status,
		FilterReleaseID: releaseID,
	}, "")
}

// executionDetail handles GET /executions/{id} — shows a single execution
// record with all JOINed display names.
func (s *Server) executionDetail(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id == 0 {
		http.NotFound(w, r)
		return
	}

	ewn, err := s.getExecutionWithNames(id)
	if err != nil {
		s.render(w, r, "execution.html", nil, "执行记录不存在或加载失败: "+err.Error())
		return
	}

	s.render(w, r, "execution_detail.html", executionDetailView{Record: ewn}, "")
}

// pushExecute handles POST /executions/push — the core "push" action.
//
// Takes release_id and datasource_id form params.  For the given release,
// looks up all associated scripts (via release_script JOIN), and for each
// script creates an execution_record (status=pending), then calls
// s.deps.Executor.Execute() for each, updating the result (auto-judge).
// If s.deps.Executor is nil, renders an error flash.
func (s *Server) pushExecute(w http.ResponseWriter, r *http.Request) {
	if s.deps.Executor == nil {
		s.render(w, r, "execution.html", nil, "执行引擎未配置")
		return
	}

	releaseID, err := strconv.ParseInt(r.FormValue("release_id"), 10, 64)
	if err != nil || releaseID == 0 {
		http.Redirect(w, r, "/executions", http.StatusSeeOther)
		return
	}

	datasourceID, err := strconv.ParseInt(r.FormValue("datasource_id"), 10, 64)
	if err != nil || datasourceID == 0 {
		http.Redirect(w, r, "/executions?release="+strconv.FormatInt(releaseID, 10), http.StatusSeeOther)
		return
	}

	// Load the target datasource (for environment_id and for execution).
	dsRepo := repo.NewDatasourceRepo(s.deps.DB)
	ds, err := dsRepo.Get(datasourceID)
	if err != nil {
		s.render(w, r, "execution.html", nil, "数据源不存在: "+err.Error())
		return
	}

	// Load all scripts associated with the release.
	releaseRepo := repo.NewReleaseRepo(s.deps.DB)
	scripts, err := releaseRepo.ListScripts(releaseID)
	if err != nil {
		s.render(w, r, "execution.html", nil, "加载发布脚本失败: "+err.Error())
		return
	}
	if len(scripts) == 0 {
		s.render(w, r, "execution.html", nil, "发布下没有关联的脚本，无法执行")
		return
	}

	// Resolve the current user for executed_by.
	var executedBy string
	if u, ok := CurrentUser(r.Context()); ok {
		executedBy = u.Username
	}

	execRepo := repo.NewExecutionRepo(s.deps.DB)
	scriptRepo := repo.NewScriptRepo(s.deps.DB)

	allSuccess := true

	// For each script: create a pending record, execute, then update result.
	for _, script := range scripts {
		rec := &model.ExecutionRecord{
			ReleaseID:     releaseID,
			ScriptID:      script.ID,
			DatasourceID:  datasourceID,
			EnvironmentID: ds.EnvironmentID,
			Status:        model.ExecStatusPending,
			MarkMethod:    model.MarkMethodAuto,
			ExecutedBy:    executedBy,
		}

		recID, err := execRepo.Create(rec)
		if err != nil {
			// Skip this script but continue with the next.
			allSuccess = false
			continue
		}

		// Execute the script against the target datasource.
		result, execErr := s.deps.Executor.Execute(r.Context(), script, ds)
		if execErr != nil {
			// Engine-level failure (context cancelled, etc.).
			_ = execRepo.UpdateResult(recID, model.ExecStatusFailed, "",
				fmt.Sprintf("引擎错误: %v", execErr))
			allSuccess = false
		} else {
			// SQL-level result: success or failed.
			_ = execRepo.UpdateResult(recID, result.Status, "", result.Err)
			if result.Status == model.ExecStatusSuccess {
				// Update script status to "executed" on success.
				script.Status = model.ScriptStatusExecuted
				_ = scriptRepo.Update(&script)
			} else {
				allSuccess = false
			}
		}
	}

	// If all scripts executed successfully, update the release status
	// to "executing" to reflect that execution has been performed.
	if allSuccess {
		_, _ = s.deps.DB.Exec(`UPDATE sql_release SET status = ? WHERE id = ?`,
			model.ReleaseStatusExecuting, releaseID)
	}

	// Redirect to the execution list filtered by this release.
	http.Redirect(w, r, "/executions?release="+strconv.FormatInt(releaseID, 10), http.StatusSeeOther)
}

// manualMark handles POST /executions/{id}/mark — marks an execution record
// manually.  Takes a status form param (success/failed) and records who
// performed the mark.
func (s *Server) manualMark(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id == 0 {
		http.NotFound(w, r)
		return
	}

	status := r.FormValue("status")
	if status != model.ExecStatusSuccess && status != model.ExecStatusFailed {
		http.Redirect(w, r, "/executions/"+strconv.FormatInt(id, 10), http.StatusSeeOther)
		return
	}

	var markedBy string
	if u, ok := CurrentUser(r.Context()); ok {
		markedBy = u.Username
	}

	execRepo := repo.NewExecutionRepo(s.deps.DB)
	if err := execRepo.MarkManual(id, status, markedBy); err != nil {
		http.Error(w, "标记失败: "+err.Error(), http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/executions/"+strconv.FormatInt(id, 10), http.StatusSeeOther)
}

// --------------------------------------------------------------------------- //
// AJAX Handlers — Executions
// --------------------------------------------------------------------------- //

// ajaxPushExecute handles POST /executions/ajax/push — pushes scripts
// in a release to a target datasource and returns a JSON response.
//
// Optional form parameter "script_ids" (comma-separated IDs) can be used to
// select a subset of scripts to execute.  When omitted, all scripts in the
// release are executed (backward compatible).
func (s *Server) ajaxPushExecute(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if s.deps.Executor == nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": "执行引擎未配置",
		})
		return
	}

	releaseID, err := strconv.ParseInt(r.FormValue("release_id"), 10, 64)
	if err != nil || releaseID == 0 {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": "无效的发布ID",
		})
		return
	}

	datasourceID, err := strconv.ParseInt(r.FormValue("datasource_id"), 10, 64)
	if err != nil || datasourceID == 0 {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": "请选择目标数据源",
		})
		return
	}

	// Parse optional script_ids parameter.
	scriptIDParam := r.FormValue("script_ids")
	selectedIDs, err := parseScriptIDs(scriptIDParam)
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	// Load the target datasource.
	dsRepo := repo.NewDatasourceRepo(s.deps.DB)
	ds, err := dsRepo.Get(datasourceID)
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": "数据源不存在: " + err.Error(),
		})
		return
	}

	// Load all scripts associated with the release.
	releaseRepo := repo.NewReleaseRepo(s.deps.DB)
	allScripts, err := releaseRepo.ListScripts(releaseID)
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": "加载发布脚本失败: " + err.Error(),
		})
		return
	}
	if len(allScripts) == 0 {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": "发布下没有关联的脚本，无法执行",
		})
		return
	}

	// Filter scripts if script_ids was provided.
	scripts := allScripts
	if len(selectedIDs) > 0 {
		scripts, err = filterScriptsByIDs(allScripts, selectedIDs)
		if err != nil {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"success": false,
				"message": err.Error(),
			})
			return
		}
	}
	if len(scripts) == 0 {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": "未选中任何脚本",
		})
		return
	}

	// Resolve the current user for executed_by.
	var executedBy string
	if u, ok := CurrentUser(r.Context()); ok {
		executedBy = u.Username
	}

	execRepo := repo.NewExecutionRepo(s.deps.DB)
	scriptRepo := repo.NewScriptRepo(s.deps.DB)

	allSuccess := true

	// For each script: create a pending record, execute, then update result.
	for _, script := range scripts {
		rec := &model.ExecutionRecord{
			ReleaseID:     releaseID,
			ScriptID:      script.ID,
			DatasourceID:  datasourceID,
			EnvironmentID: ds.EnvironmentID,
			Status:        model.ExecStatusPending,
			MarkMethod:    model.MarkMethodAuto,
			ExecutedBy:    executedBy,
		}

		recID, err := execRepo.Create(rec)
		if err != nil {
			allSuccess = false
			continue
		}

		// Execute the script against the target datasource.
		result, execErr := s.deps.Executor.Execute(r.Context(), script, ds)
		if execErr != nil {
			_ = execRepo.UpdateResult(recID, model.ExecStatusFailed, "",
				fmt.Sprintf("引擎错误: %v", execErr))
			allSuccess = false
		} else {
			_ = execRepo.UpdateResult(recID, result.Status, "", result.Err)
			if result.Status == model.ExecStatusSuccess {
				script.Status = model.ScriptStatusExecuted
				_ = scriptRepo.Update(&script)
			} else {
				allSuccess = false
			}
		}
	}

	// If all scripts executed successfully, update the release status.
	if allSuccess {
		_, _ = s.deps.DB.Exec(`UPDATE sql_release SET status = ? WHERE id = ?`,
			model.ReleaseStatusExecuting, releaseID)
	}

	totalCount := len(scripts)
	if allSuccess {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success":     true,
			"message":     fmt.Sprintf("全部 %d 个脚本执行成功", totalCount),
			"executed":    totalCount,
			"total":       totalCount,
			"redirect":    "/executions?release=" + strconv.FormatInt(releaseID, 10),
		})
	} else {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success":     true,
			"message":     fmt.Sprintf("执行完成（共 %d 个脚本），部分脚本可能失败，请查看执行记录", totalCount),
			"executed":    totalCount,
			"total":       totalCount,
			"redirect":    "/executions?release=" + strconv.FormatInt(releaseID, 10),
		})
	}
}

// ajaxManualMark handles POST /executions/ajax/{id}/mark — marks an execution
// record manually and returns JSON.
func (s *Server) ajaxManualMark(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id == 0 {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": "无效的执行记录ID",
		})
		return
	}

	status := r.FormValue("status")
	if status != model.ExecStatusSuccess && status != model.ExecStatusFailed {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": "无效的状态值",
		})
		return
	}

	var markedBy string
	if u, ok := CurrentUser(r.Context()); ok {
		markedBy = u.Username
	}

	execRepo := repo.NewExecutionRepo(s.deps.DB)
	if err := execRepo.MarkManual(id, status, markedBy); err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": "标记失败: " + err.Error(),
		})
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":  true,
		"message":  "标记成功",
		"redirect": "/executions/" + strconv.FormatInt(id, 10),
	})
}

// ajaxDeleteExecution handles POST /executions/ajax/{id}/delete — deletes
// an execution record and returns JSON.
func (s *Server) ajaxDeleteExecution(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id == 0 {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": "无效的执行记录ID",
		})
		return
	}

	execRepo := repo.NewExecutionRepo(s.deps.DB)
	if err := execRepo.Delete(id); err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": "删除执行记录失败: " + err.Error(),
		})
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":  true,
		"message":  "执行记录删除成功",
		"redirect": "/executions",
	})
}

// --------------------------------------------------------------------------- //
// Route registration
// --------------------------------------------------------------------------- //

// RegisterExecution registers all execution routes on the given mux.
// All routes are authenticated (wrapped with s.authed).
func (s *Server) RegisterExecution(mux *http.ServeMux) {
	mux.Handle("GET /executions", s.authed(s.listExecutions))
	mux.Handle("GET /executions/{id}", s.authed(s.executionDetail))
	mux.Handle("POST /executions/push", s.authed(s.pushExecute))
	mux.Handle("POST /executions/{id}/mark", s.authed(s.manualMark))

	// Executions — AJAX
	mux.Handle("POST /executions/ajax/push", s.authed(s.ajaxPushExecute))
	mux.Handle("POST /executions/ajax/{id}/mark", s.authed(s.ajaxManualMark))
	mux.Handle("POST /executions/ajax/{id}/delete", s.authed(s.ajaxDeleteExecution))
}
