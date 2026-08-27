// Package web — archive handlers (workflow E: archive/Git integration).
//
// This file implements the archive list, create-form, create-submit, and
// detail handlers, plus the RegisterArchive route registration.
package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"sql-mgr/internal/model"
)

// --------------------------------------------------------------------------- //
// View data structs
// --------------------------------------------------------------------------- //

// archiveWithNames extends ArchiveLog with the project name and release
// title resolved via JOIN, for display purposes.
type archiveWithNames struct {
	model.ArchiveLog
	ProjectName  string
	ReleaseTitle string
}

// archiveListView holds data for the archive list page.
type archiveListView struct {
	Logs []archiveWithNames
}

// archiveReleaseItem holds a single archivable release with project name
// and current version, for the archive form dropdown.
type archiveReleaseItem struct {
	ReleaseID      int64
	ReleaseTitle   string
	ProjectName    string
	CurrentVersion string
}

// archiveFormView holds data for the archive create form.
type archiveFormView struct {
	Releases []archiveReleaseItem
}

// archiveDetailView holds data for the archive detail page.
type archiveDetailView struct {
	Log archiveWithNames
}

// --------------------------------------------------------------------------- //
// Handlers — Archives
// --------------------------------------------------------------------------- //

// listArchives handles GET /archives — shows all archive logs with
// project name and release title resolved via JOIN.
func (s *Server) listArchives(w http.ResponseWriter, r *http.Request) {
	rows, err := s.deps.DB.Query(
		`SELECT al.id, al.release_id, al.project_id, al.version_from, al.version_to,
		        al.git_commit_hash, al.commit_message, al.archived_by, al.archived_at,
		        p.name, r.title
		 FROM archive_log al
		 JOIN project p ON al.project_id = p.id
		 JOIN sql_release r ON al.release_id = r.id
		 ORDER BY al.id DESC`,
	)
	if err != nil {
		s.render(w, r, "archive.html", nil, "加载归档记录失败: "+err.Error())
		return
	}
	defer rows.Close()

	var logs []archiveWithNames
	for rows.Next() {
		var l archiveWithNames
		if err := rows.Scan(
			&l.ID, &l.ReleaseID, &l.ProjectID, &l.VersionFrom, &l.VersionTo,
			&l.GitCommitHash, &l.CommitMessage, &l.ArchivedBy, &l.ArchivedAt,
			&l.ProjectName, &l.ReleaseTitle,
		); err != nil {
			s.render(w, r, "archive.html", nil, "扫描归档记录失败: "+err.Error())
			return
		}
		logs = append(logs, l)
	}
	if err := rows.Err(); err != nil {
		s.render(w, r, "archive.html", nil, "归档记录遍历错误: "+err.Error())
		return
	}

	s.render(w, r, "archive.html", archiveListView{Logs: logs}, "")
}

// archiveForm handles GET /archives/new — renders the archive form with a
// dropdown of releases that have status 'published' or 'executing'.
func (s *Server) archiveForm(w http.ResponseWriter, r *http.Request) {
	releases, err := s.queryArchivableReleases()
	if err != nil {
		s.render(w, r, "archive_form.html", nil, "加载可归档发布失败: "+err.Error())
		return
	}
	s.render(w, r, "archive_form.html", archiveFormView{Releases: releases}, "")
}

// queryArchivableReleases returns releases with status 'published' or
// 'executing', enriched with project name and current version.
func (s *Server) queryArchivableReleases() ([]archiveReleaseItem, error) {
	rows, err := s.deps.DB.Query(
		`SELECT r.id, r.title, p.name,
		        COALESCE((SELECT pv.version FROM project_version pv
		                  WHERE pv.project_id = r.project_id AND pv.is_current = 1
		                  LIMIT 1), '') AS current_version
		 FROM sql_release r
		 JOIN project p ON r.project_id = p.id
		 WHERE r.status IN ('published', 'executing')
		 ORDER BY r.id DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("query archivable releases: %w", err)
	}
	defer rows.Close()

	var releases []archiveReleaseItem
	for rows.Next() {
		var item archiveReleaseItem
		if err := rows.Scan(
			&item.ReleaseID, &item.ReleaseTitle, &item.ProjectName,
			&item.CurrentVersion,
		); err != nil {
			return nil, fmt.Errorf("scan archivable release: %w", err)
		}
		releases = append(releases, item)
	}
	return releases, rows.Err()
}

// archiveSubmit handles POST /archives — validates form input and calls
// the Archiver to commit SQL files to Git, bump the version, and write
// an archive_log entry.
func (s *Server) archiveSubmit(w http.ResponseWriter, r *http.Request) {
	// Nil-check the archiver dependency.
	if s.deps.Archiver == nil {
		s.render(w, r, "archive_form.html", nil, "归档引擎未配置")
		return
	}

	releaseID, err := strconv.ParseInt(r.FormValue("release_id"), 10, 64)
	if err != nil || releaseID == 0 {
		releases, qerr := s.queryArchivableReleases()
		if qerr != nil {
			s.render(w, r, "archive_form.html", nil, "加载可归档发布失败: "+qerr.Error())
			return
		}
		s.render(w, r, "archive_form.html", archiveFormView{Releases: releases}, "请选择要归档的发布")
		return
	}

	commitMsg := r.FormValue("commit_message")
	if commitMsg == "" {
		// Generate a default commit message using the release title.
		var title string
		if qerr := s.deps.DB.QueryRow(
			`SELECT title FROM sql_release WHERE id = ?`, releaseID,
		).Scan(&title); qerr == nil {
			commitMsg = fmt.Sprintf("chore: archive SQL scripts for %s", title)
		} else {
			commitMsg = "chore: archive SQL scripts"
		}
	}

	versionBump := r.FormValue("version_bump") == "on"

	// Call the archiver.
	log, err := s.deps.Archiver.Archive(r.Context(), releaseID, commitMsg, versionBump)
	if err != nil {
		releases, qerr := s.queryArchivableReleases()
		if qerr != nil {
			s.render(w, r, "archive_form.html", nil, "归档失败且加载可归档发布也失败: "+err.Error())
			return
		}
		s.render(w, r, "archive_form.html", archiveFormView{Releases: releases}, "归档失败: "+err.Error())
		return
	}

	_ = log // archive_log entry created successfully
	http.Redirect(w, r, "/archives", http.StatusSeeOther)
}

// archiveSubmitAJAX handles POST /archives/ajax — same as archiveSubmit but
// returns a JSON response instead of rendering HTML or redirecting.  Used by
// the archive form for in-page async submission with result modal.
func (s *Server) archiveSubmitAJAX(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Nil-check the archiver dependency.
	if s.deps.Archiver == nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": "归档引擎未配置，请先在系统配置中设置 Git 仓库",
		})
		return
	}

	releaseID, err := strconv.ParseInt(r.FormValue("release_id"), 10, 64)
	if err != nil || releaseID == 0 {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": "请选择要归档的发布",
		})
		return
	}

	commitMsg := r.FormValue("commit_message")
	if commitMsg == "" {
		// Generate a default commit message using the release title.
		var title string
		if qerr := s.deps.DB.QueryRow(
			`SELECT title FROM sql_release WHERE id = ?`, releaseID,
		).Scan(&title); qerr == nil {
			commitMsg = fmt.Sprintf("chore: archive SQL scripts for %s", title)
		} else {
			commitMsg = "chore: archive SQL scripts"
		}
	}

	versionBump := r.FormValue("version_bump") == "on"

	// Call the archiver.
	log, err := s.deps.Archiver.Archive(r.Context(), releaseID, commitMsg, versionBump)
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": "归档失败: " + err.Error(),
		})
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "归档成功",
		"data": map[string]interface{}{
			"id":              log.ID,
			"git_commit_hash": log.GitCommitHash,
			"version_from":    log.VersionFrom,
			"version_to":      log.VersionTo,
			"archived_at":     log.ArchivedAt,
		},
	})
}

// archiveDetail handles GET /archives/{id} — shows a single archive log
// entry with project name and release title.
func (s *Server) archiveDetail(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id == 0 {
		http.NotFound(w, r)
		return
	}

	var l archiveWithNames
	err = s.deps.DB.QueryRow(
		`SELECT al.id, al.release_id, al.project_id, al.version_from, al.version_to,
		        al.git_commit_hash, al.commit_message, al.archived_by, al.archived_at,
		        p.name, r.title
		 FROM archive_log al
		 JOIN project p ON al.project_id = p.id
		 JOIN sql_release r ON al.release_id = r.id
		 WHERE al.id = ?`,
		id,
	).Scan(
		&l.ID, &l.ReleaseID, &l.ProjectID, &l.VersionFrom, &l.VersionTo,
		&l.GitCommitHash, &l.CommitMessage, &l.ArchivedBy, &l.ArchivedAt,
		&l.ProjectName, &l.ReleaseTitle,
	)
	if err != nil {
		s.render(w, r, "archive.html", nil, "归档记录不存在或加载失败: "+err.Error())
		return
	}

	s.render(w, r, "archive_detail.html", archiveDetailView{Log: l}, "")
}

// --------------------------------------------------------------------------- //
// Route registration
// --------------------------------------------------------------------------- //

// RegisterArchive registers all archive routes on the given mux.
// All routes are authenticated (wrapped with s.authed).
func (s *Server) RegisterArchive(mux *http.ServeMux) {
	mux.Handle("GET /archives", s.authed(s.listArchives))
	mux.Handle("GET /archives/new", s.authed(s.archiveForm))
	mux.Handle("POST /archives", s.authed(s.archiveSubmit))
	mux.Handle("POST /archives/ajax", s.authed(s.archiveSubmitAJAX))
	mux.Handle("GET /archives/{id}", s.authed(s.archiveDetail))
}
