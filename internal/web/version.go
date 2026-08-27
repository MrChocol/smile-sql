// Package web -- version handlers (workflow B: config management).
//
// This file implements version list (filtered by project), create, and
// set-current handlers.
package web

import (
	"net/http"
	"strconv"
	"strings"

	"sql-mgr/internal/model"
	"sql-mgr/internal/repo"
)

// versionPageData holds the data for the version list page.
type versionPageData struct {
	Versions  []model.ProjectVersion
	ProjectID int64
	Projects  []model.Project
}

// loadVersionPageData loads the common data (project list) needed by the
// version page and returns it with the given projectID.
func (s *Server) loadVersionPageData(projectID int64) versionPageData {
	pr := repo.NewProjectRepo(s.deps.DB)
	projects, _ := pr.List()
	data := versionPageData{
		ProjectID: projectID,
		Projects:  projects,
	}
	if projectID > 0 {
		vr := repo.NewVersionRepo(s.deps.DB)
		data.Versions, _ = vr.ListByProject(projectID)
	}
	return data
}

// listVersions handles GET /versions?project={id}.
func (s *Server) listVersions(w http.ResponseWriter, r *http.Request) {
	projectID, _ := strconv.ParseInt(r.FormValue("project"), 10, 64)
	data := s.loadVersionPageData(projectID)
	s.render(w, r, "version.html", data, "")
}

// createVersion handles POST /versions.
func (s *Server) createVersion(w http.ResponseWriter, r *http.Request) {
	projectID, err := strconv.ParseInt(r.FormValue("project_id"), 10, 64)
	if err != nil || projectID == 0 {
		http.Redirect(w, r, "/versions", http.StatusSeeOther)
		return
	}
	version := strings.TrimSpace(r.FormValue("version"))
	if version == "" {
		data := s.loadVersionPageData(projectID)
		s.render(w, r, "version.html", data, "版本号为必填项")
		return
	}

	vr := repo.NewVersionRepo(s.deps.DB)
	vid, err := vr.Create(model.ProjectVersion{
		ProjectID: projectID,
		Version:   version,
		IsCurrent: false,
		Source:    model.VersionSourceManualInit,
	})
	if err != nil {
		data := s.loadVersionPageData(projectID)
		s.render(w, r, "version.html", data, "创建版本失败: "+err.Error())
		return
	}

	// Optionally set as current.
	if r.FormValue("is_current") != "" {
		_ = vr.SetCurrent(vid, projectID)
	}

	http.Redirect(w, r, "/versions?project="+strconv.FormatInt(projectID, 10), http.StatusSeeOther)
}

// setCurrentVersion handles POST /versions/{id}/set-current.
func (s *Server) setCurrentVersion(w http.ResponseWriter, r *http.Request) {
	versionID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Redirect(w, r, "/versions", http.StatusSeeOther)
		return
	}
	projectID, _ := strconv.ParseInt(r.FormValue("project_id"), 10, 64)

	vr := repo.NewVersionRepo(s.deps.DB)
	if err := vr.SetCurrent(versionID, projectID); err != nil {
		data := s.loadVersionPageData(projectID)
		s.render(w, r, "version.html", data, "设置当前版本失败: "+err.Error())
		return
	}

	http.Redirect(w, r, "/versions?project="+strconv.FormatInt(projectID, 10), http.StatusSeeOther)
}
