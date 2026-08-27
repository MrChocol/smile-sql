// Package web -- project handlers (workflow B: config management).
//
// This file implements project list, create-form, create, edit-form, and
// update handlers, plus the shared RegisterConfig route registration that
// wires up every config-management route onto the mux.
package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"sql-mgr/internal/model"
	"sql-mgr/internal/repo"
)

// RegisterConfig registers all configuration-management routes on the
// given mux.  It should be called from main.go after srv.Routes().
func (s *Server) RegisterConfig(mux *http.ServeMux) {
	// Projects
	mux.Handle("GET /projects", s.authed(s.listProjects))
	mux.Handle("GET /projects/new", s.authed(s.newProjectForm))
	mux.Handle("POST /projects", s.authed(s.createProject))
	mux.Handle("GET /projects/{id}", s.authed(s.projectDetail))
	mux.Handle("GET /projects/{id}/edit", s.authed(s.editProjectForm))
	mux.Handle("POST /projects/{id}", s.authed(s.updateProject))
	// AJAX endpoints
	mux.Handle("POST /projects/ajax/new", s.authed(s.ajaxCreateProject))
	mux.Handle("POST /projects/ajax/{id}/edit", s.authed(s.ajaxUpdateProject))
	mux.Handle("POST /projects/ajax/{id}/delete", s.authed(s.ajaxDeleteProject))

	// Versions
	mux.Handle("GET /versions", s.authed(s.listVersions))
	mux.Handle("POST /versions", s.authed(s.createVersion))
	mux.Handle("POST /versions/{id}/set-current", s.authed(s.setCurrentVersion))

	// Environments
	mux.Handle("GET /environments", s.authed(s.listEnvironments))
	mux.Handle("POST /environments", s.authed(s.createEnvironment))
	mux.Handle("GET /environments/{id}/edit", s.authed(s.editEnvironmentForm))
	mux.Handle("POST /environments/{id}", s.authed(s.updateEnvironment))
	// AJAX endpoints
	mux.Handle("POST /environments/ajax/new", s.authed(s.ajaxCreateEnvironment))
	mux.Handle("POST /environments/ajax/{id}/edit", s.authed(s.ajaxUpdateEnvironment))
	mux.Handle("POST /environments/ajax/{id}/delete", s.authed(s.ajaxDeleteEnvironment))

	// Datasources
	mux.Handle("GET /datasources", s.authed(s.listDatasources))
	mux.Handle("GET /datasources/new", s.authed(s.newDatasourceForm))
	mux.Handle("POST /datasources", s.authed(s.createDatasource))
	mux.Handle("GET /datasources/{id}/edit", s.authed(s.editDatasourceForm))
	mux.Handle("POST /datasources/{id}", s.authed(s.updateDatasource))
	mux.Handle("POST /datasources/{id}/test", s.authed(s.testDatasourceConnection))
	mux.Handle("POST /datasources/{id}/test-ajax", s.authed(s.testDatasourceConnectionAJAX))
	// AJAX endpoints
	mux.Handle("POST /datasources/ajax/new", s.authed(s.ajaxCreateDatasource))
	mux.Handle("POST /datasources/ajax/{id}/edit", s.authed(s.ajaxUpdateDatasource))
	mux.Handle("POST /datasources/ajax/{id}/delete", s.authed(s.ajaxDeleteDatasource))
}

// --------------------------------------------------------------------------- //
// View data structs
// --------------------------------------------------------------------------- //

// projectFormData is passed to the project form template.
type projectFormData struct {
	Project *model.Project
	IsEdit  bool
}

// --------------------------------------------------------------------------- //
// Handlers
// --------------------------------------------------------------------------- //

// listProjects handles GET /projects.
func (s *Server) listProjects(w http.ResponseWriter, r *http.Request) {
	pr := repo.NewProjectRepo(s.deps.DB)
	projects, err := pr.List()
	if err != nil {
		s.render(w, r, "project.html", nil, "加载项目列表失败: "+err.Error())
		return
	}
	s.render(w, r, "project.html", projects, "")
}

// newProjectForm handles GET /projects/new.
func (s *Server) newProjectForm(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, "project_form.html", projectFormData{IsEdit: false}, "")
}

// createProject handles POST /projects.
func (s *Server) createProject(w http.ResponseWriter, r *http.Request) {
	code := strings.TrimSpace(r.FormValue("code"))
	name := strings.TrimSpace(r.FormValue("name"))
	if code == "" || name == "" {
		s.render(w, r, "project_form.html", projectFormData{IsEdit: false}, "项目编码和名称为必填项")
		return
	}
	pr := repo.NewProjectRepo(s.deps.DB)
	_, err := pr.Create(model.Project{
		Code:              code,
		Name:              name,
		OwnerTeam:         r.FormValue("owner_team"),
		PomVersionCurrent: r.FormValue("pom_version_current"),
	})
	if err != nil {
		s.render(w, r, "project_form.html", projectFormData{IsEdit: false}, "创建项目失败: "+err.Error())
		return
	}
	http.Redirect(w, r, "/projects", http.StatusSeeOther)
}

// editProjectForm handles GET /projects/{id}/edit.
func (s *Server) editProjectForm(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Redirect(w, r, "/projects", http.StatusSeeOther)
		return
	}
	pr := repo.NewProjectRepo(s.deps.DB)
	p, err := pr.Get(id)
	if err != nil {
		http.Redirect(w, r, "/projects", http.StatusSeeOther)
		return
	}
	s.render(w, r, "project_form.html", projectFormData{Project: &p, IsEdit: true}, "")
}

// updateProject handles POST /projects/{id}.
func (s *Server) updateProject(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Redirect(w, r, "/projects", http.StatusSeeOther)
		return
	}
	code := strings.TrimSpace(r.FormValue("code"))
	name := strings.TrimSpace(r.FormValue("name"))
	if code == "" || name == "" {
		pr := repo.NewProjectRepo(s.deps.DB)
		p, _ := pr.Get(id)
		s.render(w, r, "project_form.html", projectFormData{Project: &p, IsEdit: true}, "项目编码和名称为必填项")
		return
	}
	pr := repo.NewProjectRepo(s.deps.DB)
	err = pr.Update(model.Project{
		ID:                id,
		Code:              code,
		Name:              name,
		OwnerTeam:         r.FormValue("owner_team"),
		PomVersionCurrent: r.FormValue("pom_version_current"),
	})
	if err != nil {
		p, _ := pr.Get(id)
		s.render(w, r, "project_form.html", projectFormData{Project: &p, IsEdit: true}, "更新项目失败: "+err.Error())
		return
	}
	http.Redirect(w, r, "/projects", http.StatusSeeOther)
}

// --------------------------------------------------------------------------- //
// AJAX handlers
// --------------------------------------------------------------------------- //

// ajaxCreateProject handles POST /projects/ajax/new — creates a project
// and returns a JSON response.
func (s *Server) ajaxCreateProject(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	code := strings.TrimSpace(r.FormValue("code"))
	name := strings.TrimSpace(r.FormValue("name"))
	if code == "" || name == "" {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": "项目编码和名称为必填项",
		})
		return
	}
	pr := repo.NewProjectRepo(s.deps.DB)
	_, err := pr.Create(model.Project{
		Code:              code,
		Name:              name,
		OwnerTeam:         r.FormValue("owner_team"),
		PomVersionCurrent: r.FormValue("pom_version_current"),
	})
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": "创建项目失败: " + err.Error(),
		})
		return
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":  true,
		"message":  "创建成功",
		"redirect": "/projects",
	})
}

// ajaxUpdateProject handles POST /projects/ajax/{id}/edit — updates a
// project and returns a JSON response.
func (s *Server) ajaxUpdateProject(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": "无效的项目 ID",
		})
		return
	}
	code := strings.TrimSpace(r.FormValue("code"))
	name := strings.TrimSpace(r.FormValue("name"))
	if code == "" || name == "" {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": "项目编码和名称为必填项",
		})
		return
	}
	pr := repo.NewProjectRepo(s.deps.DB)
	err = pr.Update(model.Project{
		ID:                id,
		Code:              code,
		Name:              name,
		OwnerTeam:         r.FormValue("owner_team"),
		PomVersionCurrent: r.FormValue("pom_version_current"),
	})
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": "更新项目失败: " + err.Error(),
		})
		return
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":  true,
		"message":  "保存成功",
		"redirect": "/projects",
	})
}

// ajaxDeleteProject handles POST /projects/ajax/{id}/delete — deletes a
// project and returns a JSON response.
func (s *Server) ajaxDeleteProject(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": "无效的项目 ID",
		})
		return
	}
	pr := repo.NewProjectRepo(s.deps.DB)
	if err := pr.Delete(id); err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": "删除项目失败: " + err.Error(),
		})
		return
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":  true,
		"message":  "删除成功",
		"redirect": "/projects",
	})
}

// --------------------------------------------------------------------------- //
// Project detail — environment execution roadmap
// --------------------------------------------------------------------------- //

// pipelineItem represents one row in the environment execution roadmap.
// Each row corresponds to a release and carries its worst execution status
// per environment (keyed by environment ID).
type pipelineItem struct {
	ReleaseID    int64
	ReleaseTitle string
	Status       string // release-level status (draft/published/executing/archived)
	CreatedAt    string
	CreatedBy    string
	ScriptCount  int
	// EnvStatus maps env_id -> worst status among all scripts of this
	// release in that environment.  Possible values:
	//   "success" — all scripts succeeded
	//   "failed"  — at least one script failed
	//   "pending" — at least one script pending, none failed
	//   "none"    — no execution records for this env
	EnvStatus map[int64]string
}

// projectDetailView is the data passed to project_detail.html.
type projectDetailView struct {
	Project       model.Project
	Environments  []model.Environment
	PipelineItems []pipelineItem
}

// projectDetail handles GET /projects/{id} — renders the project detail
// page with an environment execution roadmap (Git-branch-style).
func (s *Server) projectDetail(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id == 0 {
		http.Redirect(w, r, "/projects", http.StatusSeeOther)
		return
	}

	pr := repo.NewProjectRepo(s.deps.DB)
	project, err := pr.Get(id)
	if err != nil {
		http.Redirect(w, r, "/projects", http.StatusSeeOther)
		return
	}

	// Load all environments ordered by promote_order.
	envRepo := repo.NewEnvironmentRepo(s.deps.DB)
	envs, err := envRepo.List()
	if err != nil {
		s.render(w, r, "project_detail.html", nil, "加载环境列表失败: "+err.Error())
		return
	}

	// Load latest 20 releases for this project.
	relRepo := repo.NewReleaseRepo(s.deps.DB)
	releases, err := relRepo.List(id)
	if err != nil {
		s.render(w, r, "project_detail.html", nil, "加载发布列表失败: "+err.Error())
		return
	}
	// Limit to 20 most recent.
	if len(releases) > 20 {
		releases = releases[:20]
	}

	// Build pipeline items and compute per-env worst status.
	items := make([]pipelineItem, 0, len(releases))
	if len(releases) > 0 {
		// Collect release IDs for the IN clause.
		releaseIDs := make([]int64, len(releases))
		for i := range releases {
			releaseIDs[i] = releases[i].ID
		}

		// Count scripts per release.
		scriptCounts, err := s.countScriptsPerRelease(releaseIDs)
		if err != nil {
			s.render(w, r, "project_detail.html", nil, "统计脚本数量失败: "+err.Error())
			return
		}

		// Query worst execution status per (release, environment).
		envStatusMap, err := s.worstStatusPerReleaseEnv(releaseIDs)
		if err != nil {
			s.render(w, r, "project_detail.html", nil, "加载执行状态失败: "+err.Error())
			return
		}

		for _, rel := range releases {
			envStatus := make(map[int64]string, len(envs))
			for _, e := range envs {
				if st, ok := envStatusMap[rel.ID][e.ID]; ok {
					envStatus[e.ID] = st
				} else {
					envStatus[e.ID] = "none"
				}
			}
			items = append(items, pipelineItem{
				ReleaseID:    rel.ID,
				ReleaseTitle: rel.Title,
				Status:       rel.Status,
				CreatedAt:    rel.CreatedAt,
				CreatedBy:    rel.CreatedBy,
				ScriptCount:  scriptCounts[rel.ID],
				EnvStatus:    envStatus,
			})
		}
	}

	s.render(w, r, "project_detail.html", projectDetailView{
		Project:       project,
		Environments:  envs,
		PipelineItems: items,
	}, "")
}

// countScriptsPerRelease returns the number of scripts linked to each release.
func (s *Server) countScriptsPerRelease(releaseIDs []int64) (map[int64]int, error) {
	if len(releaseIDs) == 0 {
		return nil, nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(releaseIDs)), ",")
	args := make([]any, len(releaseIDs))
	for i, id := range releaseIDs {
		args[i] = id
	}
	rows, err := s.deps.DB.Query(
		`SELECT release_id, COUNT(*) FROM release_script
		 WHERE release_id IN (`+placeholders+`)
		 GROUP BY release_id`,
		args...,
	)
	if err != nil {
		return nil, fmt.Errorf("count scripts per release: %w", err)
	}
	defer rows.Close()

	result := make(map[int64]int)
	for rows.Next() {
		var rid int64
		var cnt int
		if err := rows.Scan(&rid, &cnt); err != nil {
			return nil, fmt.Errorf("scan script count: %w", err)
		}
		result[rid] = cnt
	}
	return result, rows.Err()
}

// latestStatusPerReleaseEnv returns the most recent execution status for each
// (release, environment) pair.  It uses a subquery to pick the row with the
// max created_at per group.
func (s *Server) worstStatusPerReleaseEnv(releaseIDs []int64) (map[int64]map[int64]string, error) {
	if len(releaseIDs) == 0 {
		return nil, nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(releaseIDs)), ",")
	args := make([]any, len(releaseIDs))
	for i, id := range releaseIDs {
		args[i] = id
	}

	// Pick the latest record per (release, environment) using a correlated
	// subquery on created_at.  SQLite doesn't support ROW_NUMBER() window
	// functions in all versions, so we use MAX(created_at) + self-join.
	query := `
		SELECT er.release_id, er.environment_id, er.status
		FROM execution_record er
		INNER JOIN (
		    SELECT release_id, environment_id, MAX(created_at) AS max_ts
		    FROM execution_record
		    WHERE release_id IN (` + placeholders + `)
		    GROUP BY release_id, environment_id
		) latest ON er.release_id = latest.release_id
		        AND er.environment_id = latest.environment_id
		        AND er.created_at = latest.max_ts
	`
	rows, err := s.deps.DB.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("latest status query: %w", err)
	}
	defer rows.Close()

	// result[releaseID][envID] = status string
	result := make(map[int64]map[int64]string)
	for rows.Next() {
		var rid, eid int64
		var status string
		if err := rows.Scan(&rid, &eid, &status); err != nil {
			return nil, fmt.Errorf("scan latest status: %w", err)
		}
		if result[rid] == nil {
			result[rid] = make(map[int64]string)
		}
		result[rid][eid] = status
	}
	return result, rows.Err()
}
