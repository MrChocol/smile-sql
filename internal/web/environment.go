// Package web -- environment handlers (workflow B: config management).
//
// This file implements environment list, create, edit, and delete handlers.
// The list page includes an inline create form.  AJAX endpoints are also
// provided for async form submission.
package web

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"sql-mgr/internal/model"
	"sql-mgr/internal/repo"
)

// listEnvironments handles GET /environments.
func (s *Server) listEnvironments(w http.ResponseWriter, r *http.Request) {
	er := repo.NewEnvironmentRepo(s.deps.DB)
	envs, err := er.List()
	if err != nil {
		s.render(w, r, "environment.html", nil, "加载环境列表失败: "+err.Error())
		return
	}
	s.render(w, r, "environment.html", envs, "")
}

// createEnvironment handles POST /environments.
func (s *Server) createEnvironment(w http.ResponseWriter, r *http.Request) {
	code := strings.TrimSpace(r.FormValue("code"))
	name := strings.TrimSpace(r.FormValue("name"))
	if code == "" || name == "" {
		er := repo.NewEnvironmentRepo(s.deps.DB)
		envs, _ := er.List()
		s.render(w, r, "environment.html", envs, "环境编码和名称为必填项")
		return
	}
	promoteOrder, _ := strconv.Atoi(r.FormValue("promote_order"))

	er := repo.NewEnvironmentRepo(s.deps.DB)
	_, err := er.Create(model.Environment{
		Code:         code,
		Name:         name,
		PromoteOrder: promoteOrder,
	})
	if err != nil {
		envs, _ := er.List()
		s.render(w, r, "environment.html", envs, "创建环境失败: "+err.Error())
		return
	}
	http.Redirect(w, r, "/environments", http.StatusSeeOther)
}

// editEnvironmentForm handles GET /environments/{id}/edit.
func (s *Server) editEnvironmentForm(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Redirect(w, r, "/environments", http.StatusSeeOther)
		return
	}
	er := repo.NewEnvironmentRepo(s.deps.DB)
	env, err := er.Get(id)
	if err != nil {
		http.Redirect(w, r, "/environments", http.StatusSeeOther)
		return
	}
	s.render(w, r, "environment_form.html", env, "")
}

// updateEnvironment handles POST /environments/{id}.
func (s *Server) updateEnvironment(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Redirect(w, r, "/environments", http.StatusSeeOther)
		return
	}
	code := strings.TrimSpace(r.FormValue("code"))
	name := strings.TrimSpace(r.FormValue("name"))
	if code == "" || name == "" {
		er := repo.NewEnvironmentRepo(s.deps.DB)
		env, _ := er.Get(id)
		s.render(w, r, "environment_form.html", env, "环境编码和名称为必填项")
		return
	}
	promoteOrder, _ := strconv.Atoi(r.FormValue("promote_order"))

	er := repo.NewEnvironmentRepo(s.deps.DB)
	err = er.Update(model.Environment{
		ID:           id,
		Code:         code,
		Name:         name,
		PromoteOrder: promoteOrder,
	})
	if err != nil {
		env, _ := er.Get(id)
		s.render(w, r, "environment_form.html", env, "更新环境失败: "+err.Error())
		return
	}
	http.Redirect(w, r, "/environments", http.StatusSeeOther)
}

// --------------------------------------------------------------------------- //
// AJAX handlers
// --------------------------------------------------------------------------- //

// ajaxCreateEnvironment handles POST /environments/ajax/new — creates an
// environment and returns a JSON response.
func (s *Server) ajaxCreateEnvironment(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	code := strings.TrimSpace(r.FormValue("code"))
	name := strings.TrimSpace(r.FormValue("name"))
	if code == "" || name == "" {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": "环境编码和名称为必填项",
		})
		return
	}
	promoteOrder, _ := strconv.Atoi(r.FormValue("promote_order"))

	er := repo.NewEnvironmentRepo(s.deps.DB)
	_, err := er.Create(model.Environment{
		Code:         code,
		Name:         name,
		PromoteOrder: promoteOrder,
	})
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": "创建环境失败: " + err.Error(),
		})
		return
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":  true,
		"message":  "创建成功",
		"redirect": "/environments",
	})
}

// ajaxUpdateEnvironment handles POST /environments/ajax/{id}/edit — updates
// an environment and returns a JSON response.
func (s *Server) ajaxUpdateEnvironment(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": "无效的环境 ID",
		})
		return
	}
	code := strings.TrimSpace(r.FormValue("code"))
	name := strings.TrimSpace(r.FormValue("name"))
	if code == "" || name == "" {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": "环境编码和名称为必填项",
		})
		return
	}
	promoteOrder, _ := strconv.Atoi(r.FormValue("promote_order"))

	er := repo.NewEnvironmentRepo(s.deps.DB)
	err = er.Update(model.Environment{
		ID:           id,
		Code:         code,
		Name:         name,
		PromoteOrder: promoteOrder,
	})
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": "更新环境失败: " + err.Error(),
		})
		return
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":  true,
		"message":  "保存成功",
		"redirect": "/environments",
	})
}

// ajaxDeleteEnvironment handles POST /environments/ajax/{id}/delete — deletes
// an environment and returns a JSON response.
func (s *Server) ajaxDeleteEnvironment(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": "无效的环境 ID",
		})
		return
	}
	er := repo.NewEnvironmentRepo(s.deps.DB)
	if err := er.Delete(id); err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": "删除环境失败: " + err.Error(),
		})
		return
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":  true,
		"message":  "删除成功",
		"redirect": "/environments",
	})
}
