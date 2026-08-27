// Package web — script handlers (workflow C: scripts & releases).
//
// This file implements the script list, create-form, and create handlers,
// plus the shared RegisterScriptRelease route registration that wires up
// every script and release route onto the mux.
package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"sql-mgr/internal/model"
	"sql-mgr/internal/repo"
)

// --------------------------------------------------------------------------- //
// View data structs
// --------------------------------------------------------------------------- //

// scriptListView holds data for the script list page.
type scriptListView struct {
	Scripts         []model.SQLScript
	Projects        []model.Project
	FilterProjectID int64
	ProjectNames    map[int64]string
}

// scriptFormView holds data for the script create/edit form.
type scriptFormView struct {
	Projects []model.Project
	Script   *model.SQLScript
	IsEdit   bool
}

// --------------------------------------------------------------------------- //
// Helpers
// --------------------------------------------------------------------------- //

// loadProjectsForForms queries the project table directly (shared table)
// and returns all projects ordered by name, suitable for dropdown lists.
func (s *Server) loadProjectsForForms() ([]model.Project, error) {
	rows, err := s.deps.DB.Query(
		`SELECT id, code, name, owner_team, pom_version_current, created_at
		 FROM project ORDER BY name`,
	)
	if err != nil {
		return nil, fmt.Errorf("query projects: %w", err)
	}
	defer rows.Close()

	var projects []model.Project
	for rows.Next() {
		var p model.Project
		if err := rows.Scan(
			&p.ID, &p.Code, &p.Name, &p.OwnerTeam,
			&p.PomVersionCurrent, &p.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan project: %w", err)
		}
		projects = append(projects, p)
	}
	return projects, rows.Err()
}

// --------------------------------------------------------------------------- //
// Handlers — Scripts
// --------------------------------------------------------------------------- //

// listScripts handles GET /scripts.  An optional ?project=<id> query parameter
// filters the list to a single project.
func (s *Server) listScripts(w http.ResponseWriter, r *http.Request) {
	projectID, _ := strconv.ParseInt(r.URL.Query().Get("project"), 10, 64)

	scriptRepo := repo.NewScriptRepo(s.deps.DB)
	scripts, err := scriptRepo.List(projectID)
	if err != nil {
		s.render(w, r, "script.html", nil, "加载脚本失败: "+err.Error())
		return
	}

	projects, perr := s.loadProjectsForForms()
	if perr != nil {
		s.render(w, r, "script.html", nil, "加载项目失败: "+perr.Error())
		return
	}

	projectNames := make(map[int64]string, len(projects))
	for _, p := range projects {
		projectNames[p.ID] = p.Name
	}

	s.render(w, r, "script.html", scriptListView{
		Scripts:         scripts,
		Projects:        projects,
		FilterProjectID: projectID,
		ProjectNames:    projectNames,
	}, "")
}

// scriptForm handles GET /scripts/new — renders the script create form.
func (s *Server) scriptForm(w http.ResponseWriter, r *http.Request) {
	projects, err := s.loadProjectsForForms()
	if err != nil {
		s.render(w, r, "script_form.html", nil, "加载项目失败: "+err.Error())
		return
	}
	s.render(w, r, "script_form.html", scriptFormView{Projects: projects}, "")
}

// createScript handles POST /scripts — validates and inserts a new script.
//
// Required fields: title, description, created_by.
// If created_by is empty the current user's username is used.
// sql_type defaults to DDL; status defaults to draft.
func (s *Server) createScript(w http.ResponseWriter, r *http.Request) {
	projects, perr := s.loadProjectsForForms()
	if perr != nil {
		http.Error(w, "加载项目失败: "+perr.Error(), http.StatusInternalServerError)
		return
	}

	projectID, err := strconv.ParseInt(r.FormValue("project_id"), 10, 64)
	if err != nil || projectID == 0 {
		s.render(w, r, "script_form.html", scriptFormView{Projects: projects}, "请选择项目")
		return
	}

	title := r.FormValue("title")
	if title == "" {
		s.render(w, r, "script_form.html", scriptFormView{Projects: projects}, "标题不能为空")
		return
	}

	description := r.FormValue("description")
	if description == "" {
		s.render(w, r, "script_form.html", scriptFormView{Projects: projects}, "描述不能为空")
		return
	}

	createdBy := r.FormValue("created_by")
	if createdBy == "" {
		if u, ok := CurrentUser(r.Context()); ok {
			createdBy = u.Username
		}
	}
	if createdBy == "" {
		s.render(w, r, "script_form.html", scriptFormView{Projects: projects}, "创建人不能为空")
		return
	}

	sqlType := r.FormValue("sql_type")
	if sqlType == "" {
		sqlType = model.SqlTypeDDL
	}

	script := &model.SQLScript{
		ProjectID:   projectID,
		Title:       title,
		Description: description,
		Content:     r.FormValue("content"),
		SqlType:     sqlType,
		CreatedBy:    createdBy,
		Status:      model.ScriptStatusDraft,
	}

	scriptRepo := repo.NewScriptRepo(s.deps.DB)
	if _, err := scriptRepo.Create(script); err != nil {
		s.render(w, r, "script_form.html", scriptFormView{Projects: projects}, "创建脚本失败: "+err.Error())
		return
	}

	// Push to Git pending directory if Git is configured.
	if s.deps.Git != nil {
		go s.pushScriptToPending(script, projects, "新增脚本")
	}

	http.Redirect(w, r, "/scripts", http.StatusSeeOther)
}

// editScriptForm handles GET /scripts/{id}/edit — renders the script edit form.
// Only scripts in draft status can be edited.
func (s *Server) editScriptForm(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id == 0 {
		http.Error(w, "无效的脚本ID", http.StatusBadRequest)
		return
	}

	scriptRepo := repo.NewScriptRepo(s.deps.DB)
	script, err := scriptRepo.Get(id)
	if err != nil {
		http.Error(w, "加载脚本失败: "+err.Error(), http.StatusInternalServerError)
		return
	}

	if script.Status != model.ScriptStatusDraft {
		s.render(w, r, "script.html", nil, "只有草稿状态的脚本才能编辑")
		return
	}

	projects, perr := s.loadProjectsForForms()
	if perr != nil {
		s.render(w, r, "script_form.html", nil, "加载项目失败: "+perr.Error())
		return
	}

	s.render(w, r, "script_form.html", scriptFormView{
		Projects: projects,
		Script:   &script,
		IsEdit:   true,
	}, "")
}

// updateScript handles POST /scripts/{id} — validates and updates an existing script.
// Only title, description, content, and sql_type can be edited; created_by and status are preserved.
func (s *Server) updateScript(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id == 0 {
		http.Error(w, "无效的脚本ID", http.StatusBadRequest)
		return
	}

	scriptRepo := repo.NewScriptRepo(s.deps.DB)
	script, err := scriptRepo.Get(id)
	if err != nil {
		http.Error(w, "加载脚本失败: "+err.Error(), http.StatusInternalServerError)
		return
	}

	if script.Status != model.ScriptStatusDraft {
		http.Error(w, "只有草稿状态的脚本才能编辑", http.StatusBadRequest)
		return
	}

	projects, perr := s.loadProjectsForForms()
	if perr != nil {
		http.Error(w, "加载项目失败: "+perr.Error(), http.StatusInternalServerError)
		return
	}

	projectID, err := strconv.ParseInt(r.FormValue("project_id"), 10, 64)
	if err != nil || projectID == 0 {
		s.render(w, r, "script_form.html", scriptFormView{Projects: projects, Script: &script, IsEdit: true}, "请选择项目")
		return
	}

	title := r.FormValue("title")
	if title == "" {
		s.render(w, r, "script_form.html", scriptFormView{Projects: projects, Script: &script, IsEdit: true}, "标题不能为空")
		return
	}

	description := r.FormValue("description")
	if description == "" {
		s.render(w, r, "script_form.html", scriptFormView{Projects: projects, Script: &script, IsEdit: true}, "描述不能为空")
		return
	}

	sqlType := r.FormValue("sql_type")
	if sqlType == "" {
		sqlType = model.SqlTypeDDL
	}

	// Update only editable fields; preserve created_by and status.
	script.ProjectID = projectID
	script.Title = title
	script.Description = description
	script.Content = r.FormValue("content")
	script.SqlType = sqlType

	if err := scriptRepo.Update(&script); err != nil {
		s.render(w, r, "script_form.html", scriptFormView{Projects: projects, Script: &script, IsEdit: true}, "保存脚本失败: "+err.Error())
		return
	}

	// Push updated script to Git pending directory if Git is configured.
	if s.deps.Git != nil {
		go s.pushScriptToPending(&script, projects, "更新脚本")
	}

	http.Redirect(w, r, "/scripts", http.StatusSeeOther)
}

// --------------------------------------------------------------------------- //
// Git pending sync helpers
// --------------------------------------------------------------------------- //

// pushScriptToPending commits a single script to the Git repo's pending
// directory asynchronously.  Errors are logged but not returned to the user
// since this is a best-effort background operation.
func (s *Server) pushScriptToPending(script *model.SQLScript, projects []model.Project, action string) {
	// Find project code
	var projectCode, projectName string
	for _, p := range projects {
		if p.ID == script.ProjectID {
			projectCode = p.Code
			projectName = p.Name
			break
		}
	}
	if projectCode == "" {
		// Fallback: load project directly
		pr := repo.NewProjectRepo(s.deps.DB)
		if p, err := pr.Get(script.ProjectID); err == nil {
			projectCode = p.Code
			projectName = p.Name
		}
	}
	if projectCode == "" {
		return
	}

	filename := fmt.Sprintf("%d_%s.sql", script.ID, sanitizeFileTitle(script.Title))
	relPath := fmt.Sprintf("%s/pending/%s", projectCode, filename)
	now := time.Now().Format("2006-01-02 15:04:05")
	content := fmt.Sprintf(`-- ============================================================
-- Project: %s
-- Script: %s
-- Description: %s
-- Created By: %s
-- Created At: %s
-- Last Sync: %s
-- Status: Pending
-- ============================================================
%s
`,
		projectName, script.Title, script.Description,
		script.CreatedBy, script.CreatedAt, now, script.Content,
	)

	files := map[string]string{relPath: content}
	msg := fmt.Sprintf("%s: %s (%s)", action, script.Title, projectCode)

	_, err := s.deps.Git.CommitAndPush(nil, files, msg)
	if err != nil {
		// Best effort: log but don't fail the request
		fmt.Printf("[git] push pending script %d failed: %v\n", script.ID, err)
	}
}

// sanitizeFileTitle replaces spaces and special characters to produce a
// filesystem-safe filename component.
func sanitizeFileTitle(s string) string {
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

// --------------------------------------------------------------------------- //
// AJAX Handlers — Scripts
// --------------------------------------------------------------------------- //

// ajaxCreateScript handles POST /scripts/ajax/new — creates a script and
// returns a JSON response instead of redirecting.
func (s *Server) ajaxCreateScript(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	projectID, err := strconv.ParseInt(r.FormValue("project_id"), 10, 64)
	if err != nil || projectID == 0 {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": "请选择项目",
		})
		return
	}

	title := r.FormValue("title")
	if title == "" {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": "标题不能为空",
		})
		return
	}

	description := r.FormValue("description")
	if description == "" {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": "描述不能为空",
		})
		return
	}

	createdBy := r.FormValue("created_by")
	if createdBy == "" {
		if u, ok := CurrentUser(r.Context()); ok {
			createdBy = u.Username
		}
	}
	if createdBy == "" {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": "创建人不能为空",
		})
		return
	}

	sqlType := r.FormValue("sql_type")
	if sqlType == "" {
		sqlType = model.SqlTypeDDL
	}

	script := &model.SQLScript{
		ProjectID:   projectID,
		Title:       title,
		Description: description,
		Content:     r.FormValue("content"),
		SqlType:     sqlType,
		CreatedBy:   createdBy,
		Status:      model.ScriptStatusDraft,
	}

	scriptRepo := repo.NewScriptRepo(s.deps.DB)
	if _, err := scriptRepo.Create(script); err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": "创建脚本失败: " + err.Error(),
		})
		return
	}

	// Push to Git pending directory if Git is configured.
	if s.deps.Git != nil {
		projects, _ := s.loadProjectsForForms()
		go s.pushScriptToPending(script, projects, "新增脚本")
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":  true,
		"message":  "脚本创建成功",
		"redirect": "/scripts",
	})
}

// ajaxUpdateScript handles POST /scripts/ajax/{id}/edit — updates a script
// and returns a JSON response instead of redirecting.
func (s *Server) ajaxUpdateScript(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id == 0 {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": "无效的脚本ID",
		})
		return
	}

	scriptRepo := repo.NewScriptRepo(s.deps.DB)
	script, err := scriptRepo.Get(id)
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": "加载脚本失败: " + err.Error(),
		})
		return
	}

	if script.Status != model.ScriptStatusDraft {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": "只有草稿状态的脚本才能编辑",
		})
		return
	}

	projectID, err := strconv.ParseInt(r.FormValue("project_id"), 10, 64)
	if err != nil || projectID == 0 {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": "请选择项目",
		})
		return
	}

	title := r.FormValue("title")
	if title == "" {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": "标题不能为空",
		})
		return
	}

	description := r.FormValue("description")
	if description == "" {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": "描述不能为空",
		})
		return
	}

	sqlType := r.FormValue("sql_type")
	if sqlType == "" {
		sqlType = model.SqlTypeDDL
	}

	// Update only editable fields; preserve created_by and status.
	script.ProjectID = projectID
	script.Title = title
	script.Description = description
	script.Content = r.FormValue("content")
	script.SqlType = sqlType

	if err := scriptRepo.Update(&script); err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": "保存脚本失败: " + err.Error(),
		})
		return
	}

	// Push updated script to Git pending directory if Git is configured.
	if s.deps.Git != nil {
		projects, _ := s.loadProjectsForForms()
		go s.pushScriptToPending(&script, projects, "更新脚本")
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":  true,
		"message":  "脚本保存成功",
		"redirect": "/scripts",
	})
}

// ajaxDeleteScript handles POST /scripts/ajax/{id}/delete — deletes a script
// and returns a JSON response.
func (s *Server) ajaxDeleteScript(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id == 0 {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": "无效的脚本ID",
		})
		return
	}

	scriptRepo := repo.NewScriptRepo(s.deps.DB)
	if err := scriptRepo.Delete(id); err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": "删除脚本失败: " + err.Error(),
		})
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":  true,
		"message":  "脚本删除成功",
		"redirect": "/scripts",
	})
}

// --------------------------------------------------------------------------- //
// Export helpers
// --------------------------------------------------------------------------- //

// scriptWithProject holds a script plus its project name, used for export.
type scriptWithProject struct {
	model.SQLScript
	ProjectName string
}

// getScriptWithProject fetches a script by ID and JOINs the project table
// to get the project name.
func (s *Server) getScriptWithProject(id int64) (*scriptWithProject, error) {
	var sp scriptWithProject
	err := s.deps.DB.QueryRow(
		`SELECT s.id, s.project_id, s.title, s.description, s.content,
		        s.sql_type, s.created_by, s.status, s.created_at, p.name
		 FROM sql_script s
		 LEFT JOIN project p ON s.project_id = p.id
		 WHERE s.id = ?`,
		id,
	).Scan(
		&sp.ID, &sp.ProjectID, &sp.Title, &sp.Description, &sp.Content,
		&sp.SqlType, &sp.CreatedBy, &sp.Status, &sp.CreatedAt, &sp.ProjectName,
	)
	if err != nil {
		return nil, fmt.Errorf("get script with project: %w", err)
	}
	return &sp, nil
}

// getScriptsWithProjectByIDs fetches multiple scripts by IDs in the order
// of the provided IDs, JOINing the project table for project names.
func (s *Server) getScriptsWithProjectByIDs(ids []int64) ([]scriptWithProject, error) {
	if len(ids) == 0 {
		return nil, nil
	}

	// Build placeholders for IN clause
	placeholders := make([]string, len(ids))
	args := make([]interface{}, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = id
	}

	query := fmt.Sprintf(
		`SELECT s.id, s.project_id, s.title, s.description, s.content,
		        s.sql_type, s.created_by, s.status, s.created_at, p.name
		 FROM sql_script s
		 LEFT JOIN project p ON s.project_id = p.id
		 WHERE s.id IN (%s)`,
		strings.Join(placeholders, ","),
	)

	rows, err := s.deps.DB.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("query scripts by ids: %w", err)
	}
	defer rows.Close()

	// Build a map for quick lookup
	scriptMap := make(map[int64]scriptWithProject, len(ids))
	for rows.Next() {
		var sp scriptWithProject
		if err := rows.Scan(
			&sp.ID, &sp.ProjectID, &sp.Title, &sp.Description, &sp.Content,
			&sp.SqlType, &sp.CreatedBy, &sp.Status, &sp.CreatedAt, &sp.ProjectName,
		); err != nil {
			return nil, fmt.Errorf("scan script with project: %w", err)
		}
		scriptMap[sp.ID] = sp
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Return results in the order of the input IDs
	result := make([]scriptWithProject, 0, len(ids))
	for _, id := range ids {
		if sp, ok := scriptMap[id]; ok {
			result = append(result, sp)
		}
	}
	return result, nil
}

// buildSingleExportContent formats a single script's export content with
// the standard header block.
func buildSingleExportContent(sp *scriptWithProject) string {
	return fmt.Sprintf(`-- ============================================================
-- Script: %s
-- Description: %s
-- Created By: %s
-- Created At: %s
-- Project: %s
-- ============================================================

%s
`,
		sp.Title, sp.Description, sp.CreatedBy, sp.CreatedAt,
		sp.ProjectName, sp.Content,
	)
}

// buildBatchExportContent merges multiple scripts into one export file,
// with separator lines between scripts.
func buildBatchExportContent(scripts []scriptWithProject) string {
	var b strings.Builder

	for i, sp := range scripts {
		if i == 0 {
			b.WriteString(buildSingleExportContent(&sp))
		} else {
			b.WriteString(fmt.Sprintf("\n-- ------------------------------------------------------------\n"))
			b.WriteString(fmt.Sprintf("-- Script %d: %s\n", i+1, sp.Title))
			b.WriteString(fmt.Sprintf("-- ------------------------------------------------------------\n\n"))
			b.WriteString(fmt.Sprintf("-- Description: %s\n", sp.Description))
			b.WriteString(fmt.Sprintf("-- Created By: %s\n", sp.CreatedBy))
			b.WriteString(fmt.Sprintf("-- Created At: %s\n", sp.CreatedAt))
			b.WriteString(fmt.Sprintf("-- Project: %s\n", sp.ProjectName))
			b.WriteString("\n")
			b.WriteString(sp.Content)
			b.WriteString("\n")
		}
	}

	return b.String()
}

// writeSQLFile writes a SQL file response with proper headers: UTF-8 BOM,
// Content-Type text/plain, and Content-Disposition attachment with a
// URL-encoded filename.
func writeSQLFile(w http.ResponseWriter, filename string, content string) {
	// UTF-8 BOM for Excel/记事本 compatibility with Chinese characters
	bom := []byte{0xEF, 0xBB, 0xBF}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Disposition",
		"attachment; filename=\""+url.PathEscape(filename)+"\"")

	w.Write(bom)
	w.Write([]byte(content))
}

// --------------------------------------------------------------------------- //
// Handlers — Script Export
// --------------------------------------------------------------------------- //

// exportScript handles GET /scripts/{id}/export — exports a single script
// as a downloadable .sql file.
func (s *Server) exportScript(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id == 0 {
		http.Error(w, "无效的脚本ID", http.StatusBadRequest)
		return
	}

	sp, err := s.getScriptWithProject(id)
	if err != nil {
		http.Error(w, "加载脚本失败: "+err.Error(), http.StatusInternalServerError)
		return
	}

	filename := fmt.Sprintf("%s_%s_%d.sql", sanitizeFileTitle(sp.ProjectName), sanitizeFileTitle(sp.Title), sp.ID)
	content := buildSingleExportContent(sp)
	writeSQLFile(w, filename, content)
}

// exportBatchScripts handles POST /scripts/export-batch — exports multiple
// scripts as a single merged .sql file.  The `ids` parameter is a
// comma-separated list of script IDs; output is ordered by ID list order.
func (s *Server) exportBatchScripts(w http.ResponseWriter, r *http.Request) {
	idsStr := r.FormValue("ids")
	if idsStr == "" {
		idsStr = r.URL.Query().Get("ids")
	}
	if idsStr == "" {
		http.Error(w, "请选择要导出的脚本", http.StatusBadRequest)
		return
	}

	// Parse comma-separated IDs
	idStrs := strings.Split(idsStr, ",")
	ids := make([]int64, 0, len(idStrs))
	for _, idStr := range idStrs {
		idStr = strings.TrimSpace(idStr)
		if idStr == "" {
			continue
		}
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil || id == 0 {
			http.Error(w, "无效的脚本ID: "+idStr, http.StatusBadRequest)
			return
		}
		ids = append(ids, id)
	}

	if len(ids) == 0 {
		http.Error(w, "请选择要导出的脚本", http.StatusBadRequest)
		return
	}

	scripts, err := s.getScriptsWithProjectByIDs(ids)
	if err != nil {
		http.Error(w, "加载脚本失败: "+err.Error(), http.StatusInternalServerError)
		return
	}

	if len(scripts) == 0 {
		http.Error(w, "未找到匹配的脚本", http.StatusNotFound)
		return
	}

	content := buildBatchExportContent(scripts)
	timestamp := time.Now().Format("20060102_150405")
	// Build filename with project name(s)
	projectSet := make(map[string]bool)
	for _, s := range scripts {
		projectSet[s.ProjectName] = true
	}
	var filename string
	if len(projectSet) == 1 {
		var projName string
		for k := range projectSet { projName = k; break }
		filename = fmt.Sprintf("%s_sql_scripts_%s.sql", sanitizeFileTitle(projName), timestamp)
	} else {
		filename = fmt.Sprintf("sql_scripts_%s.sql", timestamp)
	}
	writeSQLFile(w, filename, content)
}

// --------------------------------------------------------------------------- //
// Route registration
// --------------------------------------------------------------------------- //

// RegisterScriptRelease registers all script and release routes on the given
// mux.  All routes are authenticated (wrapped with s.authed).
func (s *Server) RegisterScriptRelease(mux *http.ServeMux) {
	// Scripts
	mux.Handle("GET /scripts", s.authed(s.listScripts))
	mux.Handle("GET /scripts/new", s.authed(s.scriptForm))
	mux.Handle("POST /scripts", s.authed(s.createScript))
	mux.Handle("GET /scripts/{id}/edit", s.authed(s.editScriptForm))
	mux.Handle("POST /scripts/{id}", s.authed(s.updateScript))

	// Scripts — AJAX
	mux.Handle("POST /scripts/ajax/new", s.authed(s.ajaxCreateScript))
	mux.Handle("POST /scripts/ajax/{id}/edit", s.authed(s.ajaxUpdateScript))
	mux.Handle("POST /scripts/ajax/{id}/delete", s.authed(s.ajaxDeleteScript))

	// Scripts — Export
	mux.Handle("GET /scripts/{id}/export", s.authed(s.exportScript))
	mux.Handle("POST /scripts/export-batch", s.authed(s.exportBatchScripts))

	// Releases
	mux.Handle("GET /releases", s.authed(s.listReleases))
	mux.Handle("GET /releases/new", s.authed(s.releaseForm))
	mux.Handle("POST /releases", s.authed(s.createRelease))
	mux.Handle("GET /releases/{id}", s.authed(s.releaseDetail))
	mux.Handle("POST /releases/{id}/scripts", s.authed(s.addReleaseScript))
	mux.Handle("POST /releases/{id}/scripts/{sid}/remove", s.authed(s.removeReleaseScript))
	mux.Handle("POST /releases/{id}/scripts/clear", s.authed(s.clearAllScripts))

	// Releases — AJAX
	mux.Handle("POST /releases/ajax/new", s.authed(s.ajaxCreateRelease))
	mux.Handle("POST /releases/ajax/{id}/edit", s.authed(s.ajaxUpdateRelease))
	mux.Handle("POST /releases/ajax/{id}/delete", s.authed(s.ajaxDeleteRelease))
	mux.Handle("POST /releases/ajax/{id}/add-script", s.authed(s.ajaxAddReleaseScript))
	mux.Handle("POST /releases/ajax/{id}/remove-script", s.authed(s.ajaxRemoveReleaseScript))
	mux.Handle("POST /releases/ajax/{id}/clear-scripts", s.authed(s.ajaxClearAllScripts))
}
