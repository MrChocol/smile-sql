// Package web — release handlers (workflow C: scripts & releases).
//
// This file implements the release list, create-form, create, detail,
// add-script and remove-script handlers.
package web

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"sql-mgr/internal/model"
	"sql-mgr/internal/repo"
)

// --------------------------------------------------------------------------- //
// View data structs
// --------------------------------------------------------------------------- //

// releaseWithNames extends SQLRelease with the project name and version
// string resolved via JOIN, for display purposes.
type releaseWithNames struct {
	model.SQLRelease
	ProjectName string
	VersionStr  string
}

// releaseListView holds data for the release list page.
type releaseListView struct {
	Releases []releaseWithNames
}

// versionWithName pairs a project version with its project name.
type versionWithName struct {
	ID          int64
	ProjectID   int64
	Version     string
	ProjectName string
	IsCurrent   bool
}

// releaseFormView holds data for the release create form.
type releaseFormView struct {
	Projects []model.Project
	Versions []versionWithName
}

// releaseDetailView holds data for the release detail page.
type releaseDetailView struct {
	Release     releaseWithNames
	Scripts     []model.SQLScript
	Available   []model.SQLScript
	Datasources []datasourceWithEnv // for push execution dropdown
}

// datasourceWithEnv extends Datasource with environment name.
type datasourceWithEnv struct {
	ID              int64
	Name            string
	EnvironmentID   int64
	EnvironmentName string
}

// --------------------------------------------------------------------------- //
// Helpers
// --------------------------------------------------------------------------- //

// loadVersionsForForms queries project_version JOIN project and returns all
// versions with their project name, ordered by project then version.
func (s *Server) loadVersionsForForms() ([]versionWithName, error) {
	rows, err := s.deps.DB.Query(
		`SELECT pv.id, pv.project_id, pv.version, pv.is_current, p.name
		 FROM project_version pv
		 JOIN project p ON pv.project_id = p.id
		 ORDER BY p.name, pv.version`,
	)
	if err != nil {
		return nil, fmt.Errorf("query versions: %w", err)
	}
	defer rows.Close()

	var versions []versionWithName
	for rows.Next() {
		var v versionWithName
		var isCurrent int64
		if err := rows.Scan(&v.ID, &v.ProjectID, &v.Version, &isCurrent, &v.ProjectName); err != nil {
			return nil, fmt.Errorf("scan version: %w", err)
		}
		v.IsCurrent = isCurrent != 0
		versions = append(versions, v)
	}
	return versions, rows.Err()
}

// queryReleasesWithNames runs the release JOIN query and returns enriched rows.
func (s *Server) queryReleasesWithNames(query string, args ...any) ([]releaseWithNames, error) {
	rows, err := s.deps.DB.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("query releases: %w", err)
	}
	defer rows.Close()

	var result []releaseWithNames
	for rows.Next() {
		var rwn releaseWithNames
		var versionID sql.NullInt64
		if err := rows.Scan(
			&rwn.ID, &rwn.ProjectID, &versionID,
			&rwn.Title, &rwn.Status, &rwn.CreatedBy, &rwn.CreatedAt,
			&rwn.ProjectName, &rwn.VersionStr,
		); err != nil {
			return nil, fmt.Errorf("scan release: %w", err)
		}
		if versionID.Valid {
			vid := versionID.Int64
			rwn.VersionID = &vid
		}
		result = append(result, rwn)
	}
	return result, rows.Err()
}

// getReleaseWithNames returns a single release with project/version names.
func (s *Server) getReleaseWithNames(id int64) (releaseWithNames, error) {
	var rwn releaseWithNames
	var versionID sql.NullInt64
	err := s.deps.DB.QueryRow(
		`SELECT r.id, r.project_id, r.version_id, r.title, r.status, r.created_by, r.created_at,
		        p.name,
		        COALESCE(pv.version, '')
		 FROM sql_release r
		 JOIN project p ON r.project_id = p.id
		 LEFT JOIN project_version pv ON r.version_id = pv.id
		 WHERE r.id = ?`,
		id,
	).Scan(
		&rwn.ID, &rwn.ProjectID, &versionID,
		&rwn.Title, &rwn.Status, &rwn.CreatedBy, &rwn.CreatedAt,
		&rwn.ProjectName, &rwn.VersionStr,
	)
	if err != nil {
		return rwn, fmt.Errorf("get release %d: %w", id, err)
	}
	if versionID.Valid {
		vid := versionID.Int64
		rwn.VersionID = &vid
	}
	return rwn, nil
}

// loadDatasourcesForProject loads all enabled datasources for a project,
// with their environment names, suitable for push-execution dropdowns.
func (s *Server) loadDatasourcesForProject(projectID int64) ([]datasourceWithEnv, error) {
	rows, err := s.deps.DB.Query(
		`SELECT d.id, d.name, d.environment_id, e.name
		 FROM datasource d
		 JOIN environment e ON d.environment_id = e.id
		 WHERE d.project_id = ? AND d.enabled = 1
		 ORDER BY e.promote_order, d.name`,
		projectID,
	)
	if err != nil {
		return nil, fmt.Errorf("query datasources for project: %w", err)
	}
	defer rows.Close()

	var result []datasourceWithEnv
	for rows.Next() {
		var d datasourceWithEnv
		if err := rows.Scan(&d.ID, &d.Name, &d.EnvironmentID, &d.EnvironmentName); err != nil {
			return nil, fmt.Errorf("scan datasource: %w", err)
		}
		result = append(result, d)
	}
	return result, rows.Err()
}

// availableScripts returns scripts in the given project that are not yet
// linked to the given release.
func (s *Server) availableScripts(projectID, releaseID int64) ([]model.SQLScript, error) {
	rows, err := s.deps.DB.Query(
		`SELECT id, project_id, title, description, content, sql_type, created_by, status, created_at
		 FROM sql_script
		 WHERE project_id = ?
		   AND id NOT IN (SELECT script_id FROM release_script WHERE release_id = ?)
		 ORDER BY id DESC`,
		projectID, releaseID,
	)
	if err != nil {
		return nil, fmt.Errorf("query available scripts: %w", err)
	}
	defer rows.Close()

	var scripts []model.SQLScript
	for rows.Next() {
		var sc model.SQLScript
		if err := rows.Scan(
			&sc.ID, &sc.ProjectID, &sc.Title, &sc.Description,
			&sc.Content, &sc.SqlType, &sc.CreatedBy, &sc.Status, &sc.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan available script: %w", err)
		}
		scripts = append(scripts, sc)
	}
	return scripts, rows.Err()
}

// --------------------------------------------------------------------------- //
// Handlers — Releases
// --------------------------------------------------------------------------- //

// listReleases handles GET /releases — shows all releases with project names.
// Optional query param: ?project=<id> filters to a single project.
func (s *Server) listReleases(w http.ResponseWriter, r *http.Request) {
	projectID, _ := strconv.ParseInt(r.URL.Query().Get("project"), 10, 64)

	var releases []releaseWithNames
	var err error
	if projectID > 0 {
		releases, err = s.queryReleasesWithNames(
			`SELECT r.id, r.project_id, r.version_id, r.title, r.status, r.created_by, r.created_at,
			        p.name,
			        COALESCE(pv.version, '')
			 FROM sql_release r
			 JOIN project p ON r.project_id = p.id
			 LEFT JOIN project_version pv ON r.version_id = pv.id
			 WHERE r.project_id = ?
			 ORDER BY r.id DESC`,
			projectID,
		)
	} else {
		releases, err = s.queryReleasesWithNames(
			`SELECT r.id, r.project_id, r.version_id, r.title, r.status, r.created_by, r.created_at,
			        p.name,
			        COALESCE(pv.version, '')
			 FROM sql_release r
			 JOIN project p ON r.project_id = p.id
			 LEFT JOIN project_version pv ON r.version_id = pv.id
			 ORDER BY r.id DESC`,
		)
	}
	if err != nil {
		s.render(w, r, "release.html", nil, "加载发布失败: "+err.Error())
		return
	}
	s.render(w, r, "release.html", releaseListView{Releases: releases}, "")
}

// releaseForm handles GET /releases/new — renders the release create form
// with project and version dropdowns.
func (s *Server) releaseForm(w http.ResponseWriter, r *http.Request) {
	projects, err := s.loadProjectsForForms()
	if err != nil {
		s.render(w, r, "release_form.html", nil, "加载项目失败: "+err.Error())
		return
	}
	versions, err := s.loadVersionsForForms()
	if err != nil {
		s.render(w, r, "release_form.html", nil, "加载版本失败: "+err.Error())
		return
	}
	s.render(w, r, "release_form.html", releaseFormView{Projects: projects, Versions: versions}, "")
}

// createRelease handles POST /releases — validates and inserts a new release.
//
// Required: project_id, title, created_by (falls back to current user).
// Optional: version_id.  Status defaults to draft.
func (s *Server) createRelease(w http.ResponseWriter, r *http.Request) {
	projects, perr := s.loadProjectsForForms()
	if perr != nil {
		http.Error(w, "加载项目失败: "+perr.Error(), http.StatusInternalServerError)
		return
	}
	versions, verr := s.loadVersionsForForms()
	if verr != nil {
		http.Error(w, "加载版本失败: "+verr.Error(), http.StatusInternalServerError)
		return
	}
	fv := releaseFormView{Projects: projects, Versions: versions}

	projectID, err := strconv.ParseInt(r.FormValue("project_id"), 10, 64)
	if err != nil || projectID == 0 {
		s.render(w, r, "release_form.html", fv, "请选择项目")
		return
	}

	title := r.FormValue("title")
	if title == "" {
		s.render(w, r, "release_form.html", fv, "标题不能为空")
		return
	}

	createdBy := r.FormValue("created_by")
	if createdBy == "" {
		if u, ok := CurrentUser(r.Context()); ok {
			createdBy = u.Username
		}
	}
	if createdBy == "" {
		s.render(w, r, "release_form.html", fv, "创建人不能为空")
		return
	}

	// version_id is optional.
	var versionID *int64
	if vidStr := r.FormValue("version_id"); vidStr != "" && vidStr != "0" {
		vid, err := strconv.ParseInt(vidStr, 10, 64)
		if err == nil {
			versionID = &vid
		}
	}

	rel := &model.SQLRelease{
		ProjectID: projectID,
		VersionID: versionID,
		Title:     title,
		Status:    model.ReleaseStatusDraft,
		CreatedBy: createdBy,
	}

	releaseRepo := repo.NewReleaseRepo(s.deps.DB)
	if _, err := releaseRepo.Create(rel); err != nil {
		s.render(w, r, "release_form.html", fv, "创建发布失败: "+err.Error())
		return
	}

	http.Redirect(w, r, "/releases", http.StatusSeeOther)
}

// releaseDetail handles GET /releases/{id} — shows release info, associated
// scripts, available scripts to add, and a push-execution dropdown.
func (s *Server) releaseDetail(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id == 0 {
		http.NotFound(w, r)
		return
	}

	rwn, err := s.getReleaseWithNames(id)
	if err != nil {
		s.render(w, r, "release.html", nil, "发布不存在或加载失败: "+err.Error())
		return
	}

	releaseRepo := repo.NewReleaseRepo(s.deps.DB)
	scripts, err := releaseRepo.ListScripts(id)
	if err != nil {
		s.render(w, r, "release.html", nil, "加载关联脚本失败: "+err.Error())
		return
	}

	available, err := s.availableScripts(rwn.ProjectID, id)
	if err != nil {
		s.render(w, r, "release.html", nil, "加载可选脚本失败: "+err.Error())
		return
	}

	// Load datasources for the push-execution dropdown.
	datasources, err := s.loadDatasourcesForProject(rwn.ProjectID)
	if err != nil {
		s.render(w, r, "release.html", nil, "加载数据源失败: "+err.Error())
		return
	}

	s.render(w, r, "release_detail.html", releaseDetailView{
		Release:     rwn,
		Scripts:     scripts,
		Available:   available,
		Datasources: datasources,
	}, "")
}

// addReleaseScript handles POST /releases/{id}/scripts — links a script
// (identified by the form field script_id) to the release.
func (s *Server) addReleaseScript(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id == 0 {
		http.NotFound(w, r)
		return
	}

	scriptID, err := strconv.ParseInt(r.FormValue("script_id"), 10, 64)
	if err != nil || scriptID == 0 {
		http.Redirect(w, r, "/releases/"+strconv.FormatInt(id, 10), http.StatusSeeOther)
		return
	}

	releaseRepo := repo.NewReleaseRepo(s.deps.DB)
	if err := releaseRepo.AddScript(id, scriptID); err != nil {
		http.Error(w, "添加脚本失败: "+err.Error(), http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/releases/"+strconv.FormatInt(id, 10), http.StatusSeeOther)
}

// removeReleaseScript handles POST /releases/{id}/scripts/{sid}/remove —
// unlinks a script from the release.
func (s *Server) removeReleaseScript(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id == 0 {
		http.NotFound(w, r)
		return
	}

	sid, err := strconv.ParseInt(r.PathValue("sid"), 10, 64)
	if err != nil || sid == 0 {
		http.NotFound(w, r)
		return
	}

	releaseRepo := repo.NewReleaseRepo(s.deps.DB)
	if err := releaseRepo.RemoveScript(id, sid); err != nil {
		http.Error(w, "移除脚本失败: "+err.Error(), http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/releases/"+strconv.FormatInt(id, 10), http.StatusSeeOther)
}

// clearAllScripts handles POST /releases/{id}/scripts/clear — removes all
// scripts from a release in one action.
func (s *Server) clearAllScripts(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id == 0 {
		http.NotFound(w, r)
		return
	}

	releaseRepo := repo.NewReleaseRepo(s.deps.DB)
	count, err := releaseRepo.ClearAllScripts(id)
	if err != nil {
		http.Error(w, "清空脚本失败: "+err.Error(), http.StatusInternalServerError)
		return
	}

	s.renderClearFlash(w, r, id, fmt.Sprintf("已移除 %d 个脚本", count))
}

// renderClearFlash re-renders the release detail page with a success flash.
func (s *Server) renderClearFlash(w http.ResponseWriter, r *http.Request, releaseID int64, flash string) {
	rwn, err := s.getReleaseWithNames(releaseID)
	if err != nil {
		http.Redirect(w, r, "/releases", http.StatusSeeOther)
		return
	}

	releaseRepo := repo.NewReleaseRepo(s.deps.DB)
	scripts, err := releaseRepo.ListScripts(releaseID)
	if err != nil {
		http.Redirect(w, r, "/releases", http.StatusSeeOther)
		return
	}

	available, err := s.availableScripts(rwn.ProjectID, releaseID)
	if err != nil {
		http.Redirect(w, r, "/releases", http.StatusSeeOther)
		return
	}

	datasources, err := s.loadDatasourcesForProject(rwn.ProjectID)
	if err != nil {
		http.Redirect(w, r, "/releases", http.StatusSeeOther)
		return
	}

	s.render(w, r, "release_detail.html", releaseDetailView{
		Release:     rwn,
		Scripts:     scripts,
		Available:   available,
		Datasources: datasources,
	}, flash)
}

// --------------------------------------------------------------------------- //
// AJAX Handlers — Releases
// --------------------------------------------------------------------------- //

// ajaxCreateRelease handles POST /releases/ajax/new — creates a release and
// returns a JSON response instead of redirecting.
func (s *Server) ajaxCreateRelease(w http.ResponseWriter, r *http.Request) {
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

	// version_id is optional.
	var versionID *int64
	if vidStr := r.FormValue("version_id"); vidStr != "" && vidStr != "0" {
		vid, err := strconv.ParseInt(vidStr, 10, 64)
		if err == nil {
			versionID = &vid
		}
	}

	rel := &model.SQLRelease{
		ProjectID: projectID,
		VersionID: versionID,
		Title:     title,
		Status:    model.ReleaseStatusDraft,
		CreatedBy: createdBy,
	}

	releaseRepo := repo.NewReleaseRepo(s.deps.DB)
	if _, err := releaseRepo.Create(rel); err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": "创建发布失败: " + err.Error(),
		})
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":  true,
		"message":  "发布创建成功",
		"redirect": "/releases",
	})
}

// ajaxUpdateRelease handles POST /releases/ajax/{id}/edit — updates a release
// and returns a JSON response.
func (s *Server) ajaxUpdateRelease(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id == 0 {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": "无效的发布ID",
		})
		return
	}

	releaseRepo := repo.NewReleaseRepo(s.deps.DB)
	rel, err := releaseRepo.Get(id)
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": "加载发布失败: " + err.Error(),
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

	// version_id is optional.
	var versionID *int64
	if vidStr := r.FormValue("version_id"); vidStr != "" && vidStr != "0" {
		vid, err := strconv.ParseInt(vidStr, 10, 64)
		if err == nil {
			versionID = &vid
		}
	}

	rel.ProjectID = projectID
	rel.VersionID = versionID
	rel.Title = title

	if err := releaseRepo.Update(&rel); err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": "保存发布失败: " + err.Error(),
		})
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":  true,
		"message":  "发布保存成功",
		"redirect": "/releases",
	})
}

// ajaxDeleteRelease handles POST /releases/ajax/{id}/delete — deletes a release
// and returns a JSON response.
func (s *Server) ajaxDeleteRelease(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id == 0 {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": "无效的发布ID",
		})
		return
	}

	releaseRepo := repo.NewReleaseRepo(s.deps.DB)
	if err := releaseRepo.Delete(id); err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": "删除发布失败: " + err.Error(),
		})
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":  true,
		"message":  "发布删除成功",
		"redirect": "/releases",
	})
}

// ajaxAddReleaseScript handles POST /releases/ajax/{id}/add-script — links a
// script to the release and returns JSON.
func (s *Server) ajaxAddReleaseScript(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id == 0 {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": "无效的发布ID",
		})
		return
	}

	scriptID, err := strconv.ParseInt(r.FormValue("script_id"), 10, 64)
	if err != nil || scriptID == 0 {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": "请选择脚本",
		})
		return
	}

	releaseRepo := repo.NewReleaseRepo(s.deps.DB)
	if err := releaseRepo.AddScript(id, scriptID); err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": "添加脚本失败: " + err.Error(),
		})
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":  true,
		"message":  "脚本添加成功",
		"redirect": "/releases/" + strconv.FormatInt(id, 10),
	})
}

// ajaxRemoveReleaseScript handles POST /releases/ajax/{id}/remove-script —
// unlinks a script from the release and returns JSON.
func (s *Server) ajaxRemoveReleaseScript(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id == 0 {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": "无效的发布ID",
		})
		return
	}

	sid, err := strconv.ParseInt(r.FormValue("script_id"), 10, 64)
	if err != nil || sid == 0 {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": "无效的脚本ID",
		})
		return
	}

	releaseRepo := repo.NewReleaseRepo(s.deps.DB)
	if err := releaseRepo.RemoveScript(id, sid); err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": "移除脚本失败: " + err.Error(),
		})
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":  true,
		"message":  "脚本移除成功",
		"redirect": "/releases/" + strconv.FormatInt(id, 10),
	})
}

// ajaxClearAllScripts handles POST /releases/ajax/{id}/clear-scripts — removes
// all scripts from a release and returns JSON.
func (s *Server) ajaxClearAllScripts(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id == 0 {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": "无效的发布ID",
		})
		return
	}

	releaseRepo := repo.NewReleaseRepo(s.deps.DB)
	count, err := releaseRepo.ClearAllScripts(id)
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": "清空脚本失败: " + err.Error(),
		})
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":  true,
		"message":  fmt.Sprintf("已移除 %d 个脚本", count),
		"redirect": "/releases/" + strconv.FormatInt(id, 10),
	})
}
