// Package web -- datasource handlers (workflow B: config management).
//
// This file implements datasource list (with project/env filters),
// create-form, create, edit-form, update, and test-connection handlers.
// Password encryption is performed in the handlers via s.deps.Crypto before
// storage; the edit form never echoes the password back.
package web

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"sql-mgr/internal/model"
	"sql-mgr/internal/repo"
)

// --------------------------------------------------------------------------- //
// View data structs
// --------------------------------------------------------------------------- //

// datasourceListData holds data for the datasource list page.
type datasourceListData struct {
	Datasources   []model.Datasource
	Projects      []model.Project
	Environments  []model.Environment
	ProjectNames  map[int64]string
	EnvNames      map[int64]string
	FilterProject int64
	FilterEnv     int64
}

// datasourceFormData holds data for the datasource create/edit form.
type datasourceFormData struct {
	Datasource   *model.Datasource
	Projects     []model.Project
	Environments []model.Environment
	IsEdit       bool
}

// --------------------------------------------------------------------------- //
// Handlers
// --------------------------------------------------------------------------- //

// listDatasources handles GET /datasources?project={id}&env={id}.
func (s *Server) listDatasources(w http.ResponseWriter, r *http.Request) {
	filterProject, _ := strconv.ParseInt(r.FormValue("project"), 10, 64)
	filterEnv, _ := strconv.ParseInt(r.FormValue("env"), 10, 64)

	pr := repo.NewProjectRepo(s.deps.DB)
	projects, err := pr.List()
	if err != nil {
		s.render(w, r, "datasource.html", nil, "加载项目列表失败: "+err.Error())
		return
	}

	er := repo.NewEnvironmentRepo(s.deps.DB)
	envs, err := er.List()
	if err != nil {
		s.render(w, r, "datasource.html", nil, "加载环境列表失败: "+err.Error())
		return
	}

	dr := repo.NewDatasourceRepo(s.deps.DB)
	dss, err := dr.List(filterProject, filterEnv)
	if err != nil {
		s.render(w, r, "datasource.html", nil, "加载数据源列表失败: "+err.Error())
		return
	}

	projectNames := make(map[int64]string, len(projects))
	for _, p := range projects {
		projectNames[p.ID] = p.Name
	}
	envNames := make(map[int64]string, len(envs))
	for _, e := range envs {
		envNames[e.ID] = e.Name
	}

	s.render(w, r, "datasource.html", datasourceListData{
		Datasources:   dss,
		Projects:      projects,
		Environments:  envs,
		ProjectNames:  projectNames,
		EnvNames:      envNames,
		FilterProject: filterProject,
		FilterEnv:     filterEnv,
	}, "")
}

// newDatasourceForm handles GET /datasources/new.
func (s *Server) newDatasourceForm(w http.ResponseWriter, r *http.Request) {
	data, err := s.loadDatasourceFormData(nil, false)
	if err != nil {
		s.render(w, r, "datasource_form.html", datasourceFormData{IsEdit: false}, err.Error())
		return
	}
	s.render(w, r, "datasource_form.html", data, "")
}

// createDatasource handles POST /datasources.
func (s *Server) createDatasource(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.FormValue("name"))
	host := strings.TrimSpace(r.FormValue("host"))
	portStr := strings.TrimSpace(r.FormValue("port"))
	if name == "" || host == "" || portStr == "" {
		s.renderDatasourceFormError(w, r, 0, "数据源名称、主机和端口为必填项", false)
		return
	}

	port, err := strconv.Atoi(portStr)
	if err != nil || port <= 0 {
		s.renderDatasourceFormError(w, r, 0, "端口必须为正整数", false)
		return
	}

	projectID, _ := strconv.ParseInt(r.FormValue("project_id"), 10, 64)
	environmentID, _ := strconv.ParseInt(r.FormValue("environment_id"), 10, 64)

	passwordEnc, err := s.deps.Crypto.Encrypt(r.FormValue("password"))
	if err != nil {
		s.renderDatasourceFormError(w, r, 0, "密码加密失败: "+err.Error(), false)
		return
	}

	dr := repo.NewDatasourceRepo(s.deps.DB)
	_, err = dr.Create(model.Datasource{
		ProjectID:     projectID,
		EnvironmentID: environmentID,
		Name:          name,
		DBType:        r.FormValue("db_type"),
		Host:          host,
		Port:          port,
		DBName:        r.FormValue("db_name"),
		Username:      r.FormValue("username"),
		PasswordEnc:   passwordEnc,
		Enabled:       r.FormValue("enabled") != "",
	})
	if err != nil {
		s.renderDatasourceFormError(w, r, 0, "创建数据源失败: "+err.Error(), false)
		return
	}

	http.Redirect(w, r, "/datasources", http.StatusSeeOther)
}

// editDatasourceForm handles GET /datasources/{id}/edit.
func (s *Server) editDatasourceForm(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Redirect(w, r, "/datasources", http.StatusSeeOther)
		return
	}

	dr := repo.NewDatasourceRepo(s.deps.DB)
	ds, err := dr.Get(id)
	if err != nil {
		http.Redirect(w, r, "/datasources", http.StatusSeeOther)
		return
	}

	data, err := s.loadDatasourceFormData(&ds, true)
	if err != nil {
		s.render(w, r, "datasource_form.html", datasourceFormData{IsEdit: true}, err.Error())
		return
	}
	s.render(w, r, "datasource_form.html", data, "")
}

// updateDatasource handles POST /datasources/{id}.
func (s *Server) updateDatasource(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Redirect(w, r, "/datasources", http.StatusSeeOther)
		return
	}

	name := strings.TrimSpace(r.FormValue("name"))
	host := strings.TrimSpace(r.FormValue("host"))
	portStr := strings.TrimSpace(r.FormValue("port"))
	if name == "" || host == "" || portStr == "" {
		s.renderDatasourceFormError(w, r, id, "数据源名称、主机和端口为必填项", true)
		return
	}

	port, err := strconv.Atoi(portStr)
	if err != nil || port <= 0 {
		s.renderDatasourceFormError(w, r, id, "端口必须为正整数", true)
		return
	}

	dr := repo.NewDatasourceRepo(s.deps.DB)
	existing, err := dr.Get(id)
	if err != nil {
		http.Redirect(w, r, "/datasources", http.StatusSeeOther)
		return
	}

	// If the password field is left blank, keep the existing encrypted
	// password; otherwise encrypt the new value.
	passwordEnc := existing.PasswordEnc
	if pw := r.FormValue("password"); pw != "" {
		passwordEnc, err = s.deps.Crypto.Encrypt(pw)
		if err != nil {
			s.renderDatasourceFormError(w, r, id, "密码加密失败: "+err.Error(), true)
			return
		}
	}

	projectID, _ := strconv.ParseInt(r.FormValue("project_id"), 10, 64)
	environmentID, _ := strconv.ParseInt(r.FormValue("environment_id"), 10, 64)

	err = dr.Update(model.Datasource{
		ID:            id,
		ProjectID:     projectID,
		EnvironmentID: environmentID,
		Name:          name,
		DBType:        r.FormValue("db_type"),
		Host:          host,
		Port:          port,
		DBName:        r.FormValue("db_name"),
		Username:      r.FormValue("username"),
		PasswordEnc:   passwordEnc,
		Enabled:       r.FormValue("enabled") != "",
	})
	if err != nil {
		s.renderDatasourceFormError(w, r, id, "更新数据源失败: "+err.Error(), true)
		return
	}

	http.Redirect(w, r, "/datasources", http.StatusSeeOther)
}

// testDatasourceConnection handles POST /datasources/{id}/test — tests
// connectivity to an existing datasource by decrypting its password and
// pinging the target database.
func (s *Server) testDatasourceConnection(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id == 0 {
		http.Redirect(w, r, "/datasources", http.StatusSeeOther)
		return
	}

	if s.deps.Executor == nil {
		s.render(w, r, "datasource.html", nil, "执行引擎未配置，无法测试连接")
		return
	}

	dr := repo.NewDatasourceRepo(s.deps.DB)
	ds, err := dr.Get(id)
	if err != nil {
		s.render(w, r, "datasource.html", nil, "数据源不存在: "+err.Error())
		return
	}

	// Decrypt the stored password.
	password, err := s.deps.Crypto.Decrypt(ds.PasswordEnc)
	if err != nil {
		s.render(w, r, "datasource.html", nil, "密码解密失败: "+err.Error())
		return
	}

	if err := s.deps.Executor.TestConnection(r.Context(), ds, password); err != nil {
		s.render(w, r, "datasource.html", nil, "连接测试失败: "+err.Error())
		return
	}

	s.render(w, r, "datasource.html", nil, "连接测试成功！")
}

// testDatasourceConnectionAJAX handles POST /datasources/{id}/test-ajax —
// same as testDatasourceConnection but returns a JSON response instead of
// rendering an HTML page.  Used by the frontend for in-page test results.
func (s *Server) testDatasourceConnectionAJAX(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id == 0 {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": "无效的数据源 ID",
		})
		return
	}

	if s.deps.Executor == nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": "执行引擎未配置，无法测试连接",
		})
		return
	}

	dr := repo.NewDatasourceRepo(s.deps.DB)
	ds, err := dr.Get(id)
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": "数据源不存在: " + err.Error(),
		})
		return
	}

	password, err := s.deps.Crypto.Decrypt(ds.PasswordEnc)
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": "密码解密失败: " + err.Error(),
		})
		return
	}

	if err := s.deps.Executor.TestConnection(r.Context(), ds, password); err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": "连接测试失败: " + err.Error(),
		})
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "连接测试成功！",
	})
}

// --------------------------------------------------------------------------- //
// AJAX handlers — create / update / delete
// --------------------------------------------------------------------------- //

// ajaxCreateDatasource handles POST /datasources/ajax/new — creates a
// datasource and returns a JSON response.
func (s *Server) ajaxCreateDatasource(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	name := strings.TrimSpace(r.FormValue("name"))
	host := strings.TrimSpace(r.FormValue("host"))
	portStr := strings.TrimSpace(r.FormValue("port"))
	if name == "" || host == "" || portStr == "" {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": "数据源名称、主机和端口为必填项",
		})
		return
	}

	port, err := strconv.Atoi(portStr)
	if err != nil || port <= 0 {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": "端口必须为正整数",
		})
		return
	}

	projectID, _ := strconv.ParseInt(r.FormValue("project_id"), 10, 64)
	environmentID, _ := strconv.ParseInt(r.FormValue("environment_id"), 10, 64)

	passwordEnc, err := s.deps.Crypto.Encrypt(r.FormValue("password"))
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": "密码加密失败: " + err.Error(),
		})
		return
	}

	dr := repo.NewDatasourceRepo(s.deps.DB)
	_, err = dr.Create(model.Datasource{
		ProjectID:     projectID,
		EnvironmentID: environmentID,
		Name:          name,
		DBType:        r.FormValue("db_type"),
		Host:          host,
		Port:          port,
		DBName:        r.FormValue("db_name"),
		Username:      r.FormValue("username"),
		PasswordEnc:   passwordEnc,
		Enabled:       r.FormValue("enabled") != "",
	})
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": "创建数据源失败: " + err.Error(),
		})
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":  true,
		"message":  "创建成功",
		"redirect": "/datasources",
	})
}

// ajaxUpdateDatasource handles POST /datasources/ajax/{id}/edit — updates a
// datasource and returns a JSON response.
func (s *Server) ajaxUpdateDatasource(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": "无效的数据源 ID",
		})
		return
	}

	name := strings.TrimSpace(r.FormValue("name"))
	host := strings.TrimSpace(r.FormValue("host"))
	portStr := strings.TrimSpace(r.FormValue("port"))
	if name == "" || host == "" || portStr == "" {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": "数据源名称、主机和端口为必填项",
		})
		return
	}

	port, err := strconv.Atoi(portStr)
	if err != nil || port <= 0 {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": "端口必须为正整数",
		})
		return
	}

	dr := repo.NewDatasourceRepo(s.deps.DB)
	existing, err := dr.Get(id)
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": "数据源不存在",
		})
		return
	}

	// If the password field is left blank, keep the existing encrypted
	// password; otherwise encrypt the new value.
	passwordEnc := existing.PasswordEnc
	if pw := r.FormValue("password"); pw != "" {
		passwordEnc, err = s.deps.Crypto.Encrypt(pw)
		if err != nil {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"success": false,
				"message": "密码加密失败: " + err.Error(),
			})
			return
		}
	}

	projectID, _ := strconv.ParseInt(r.FormValue("project_id"), 10, 64)
	environmentID, _ := strconv.ParseInt(r.FormValue("environment_id"), 10, 64)

	err = dr.Update(model.Datasource{
		ID:            id,
		ProjectID:     projectID,
		EnvironmentID: environmentID,
		Name:          name,
		DBType:        r.FormValue("db_type"),
		Host:          host,
		Port:          port,
		DBName:        r.FormValue("db_name"),
		Username:      r.FormValue("username"),
		PasswordEnc:   passwordEnc,
		Enabled:       r.FormValue("enabled") != "",
	})
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": "更新数据源失败: " + err.Error(),
		})
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":  true,
		"message":  "保存成功",
		"redirect": "/datasources",
	})
}

// ajaxDeleteDatasource handles POST /datasources/ajax/{id}/delete — deletes
// a datasource and returns a JSON response.
func (s *Server) ajaxDeleteDatasource(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": "无效的数据源 ID",
		})
		return
	}
	dr := repo.NewDatasourceRepo(s.deps.DB)
	if err := dr.Delete(id); err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": "删除数据源失败: " + err.Error(),
		})
		return
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":  true,
		"message":  "删除成功",
		"redirect": "/datasources",
	})
}

// --------------------------------------------------------------------------- //
// Helpers
// --------------------------------------------------------------------------- //

// loadDatasourceFormData builds the form view data, loading projects and
// environments for the dropdown selectors.  When ds is non-nil the form
// is in edit mode and pre-selects the datasource's project/environment.
func (s *Server) loadDatasourceFormData(ds *model.Datasource, isEdit bool) (datasourceFormData, error) {
	pr := repo.NewProjectRepo(s.deps.DB)
	projects, err := pr.List()
	if err != nil {
		return datasourceFormData{}, err
	}

	er := repo.NewEnvironmentRepo(s.deps.DB)
	envs, err := er.List()
	if err != nil {
		return datasourceFormData{}, err
	}

	return datasourceFormData{
		Datasource:   ds,
		Projects:     projects,
		Environments: envs,
		IsEdit:       isEdit,
	}, nil
}

// renderDatasourceFormError re-renders the datasource form with a flash
// message.  When id > 0 the existing datasource is loaded for edit mode.
func (s *Server) renderDatasourceFormError(w http.ResponseWriter, r *http.Request, id int64, flash string, isEdit bool) {
	var ds *model.Datasource
	if isEdit && id > 0 {
		dr := repo.NewDatasourceRepo(s.deps.DB)
		if existing, err := dr.Get(id); err == nil {
			ds = &existing
		}
	}
	data, err := s.loadDatasourceFormData(ds, isEdit)
	if err != nil {
		s.render(w, r, "datasource_form.html", datasourceFormData{IsEdit: isEdit}, flash)
		return
	}
	s.render(w, r, "datasource_form.html", data, flash)
}
