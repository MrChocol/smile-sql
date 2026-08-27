// Package web — data cleanup handlers (system configuration).
//
// Provides a unified entry point for clearing data per module,
// with statistics and confirmation prompts.
package web

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// --------------------------------------------------------------------------- //
// View data structs
// --------------------------------------------------------------------------- //

// cleanDataView holds statistics for each module displayed on the cleanup page.
type cleanDataView struct {
	ProjectCount    int64
	VersionCount    int64
	EnvCount        int64
	DatasourceCount int64
	ScriptCount     int64
	ReleaseCount    int64
	ExecutionCount  int64
	ArchiveCount    int64
}

// --------------------------------------------------------------------------- //
// Page handler
// --------------------------------------------------------------------------- //

// cleanDataPage shows the data cleanup overview page with per-module counts.
func (s *Server) cleanDataPage(w http.ResponseWriter, r *http.Request) {
	var v cleanDataView

	s.deps.DB.QueryRow(`SELECT COUNT(*) FROM project`).Scan(&v.ProjectCount)
	s.deps.DB.QueryRow(`SELECT COUNT(*) FROM project_version`).Scan(&v.VersionCount)
	s.deps.DB.QueryRow(`SELECT COUNT(*) FROM environment`).Scan(&v.EnvCount)
	s.deps.DB.QueryRow(`SELECT COUNT(*) FROM datasource`).Scan(&v.DatasourceCount)
	s.deps.DB.QueryRow(`SELECT COUNT(*) FROM sql_script`).Scan(&v.ScriptCount)
	s.deps.DB.QueryRow(`SELECT COUNT(*) FROM sql_release`).Scan(&v.ReleaseCount)
	s.deps.DB.QueryRow(`SELECT COUNT(*) FROM execution_record`).Scan(&v.ExecutionCount)
	s.deps.DB.QueryRow(`SELECT COUNT(*) FROM archive_log`).Scan(&v.ArchiveCount)

	s.render(w, r, "clean_data.html", v, "")
}

// --------------------------------------------------------------------------- //
// Cleanup handlers — return JSON for AJAX
// --------------------------------------------------------------------------- //

// cleanProjects deletes all projects and cascades to related data.
func (s *Server) cleanProjects(w http.ResponseWriter, r *http.Request) {
	tx, err := s.deps.DB.Begin()
	if err != nil {
		writeCleanError(w, "开始事务失败: "+err.Error())
		return
	}
	defer tx.Rollback()

	tables := []string{
		"archive_log",
		"execution_record",
		"release_script",
		"sql_release",
		"sql_script",
		"datasource",
		"environment",
		"project_version",
		"project",
	}
	for _, t := range tables {
		if _, err := tx.Exec(`DELETE FROM ` + t); err != nil {
			writeCleanError(w, fmt.Sprintf("清空 %s 失败: %v", t, err))
			return
		}
	}

	if err := tx.Commit(); err != nil {
		writeCleanError(w, "提交事务失败: "+err.Error())
		return
	}

	writeCleanSuccess(w, "所有项目及关联数据已全部清空")
}

// cleanVersions deletes all project versions.
func (s *Server) cleanVersions(w http.ResponseWriter, r *http.Request) {
	tx, err := s.deps.DB.Begin()
	if err != nil {
		writeCleanError(w, "开始事务失败: "+err.Error())
		return
	}
	defer tx.Rollback()

	// sql_release.version_id references project_version(id); set to NULL first.
	if _, err := tx.Exec(`UPDATE sql_release SET version_id = NULL WHERE version_id IS NOT NULL`); err != nil {
		writeCleanError(w, "解除发布版本关联失败: "+err.Error())
		return
	}
	if _, err := tx.Exec(`DELETE FROM project_version`); err != nil {
		writeCleanError(w, "清空版本管理失败: "+err.Error())
		return
	}

	if err := tx.Commit(); err != nil {
		writeCleanError(w, "提交事务失败: "+err.Error())
		return
	}
	writeCleanSuccess(w, "所有版本记录已清空")
}

// cleanEnvironments deletes all environments and their datasources/execution records.
func (s *Server) cleanEnvironments(w http.ResponseWriter, r *http.Request) {
	tx, err := s.deps.DB.Begin()
	if err != nil {
		writeCleanError(w, "开始事务失败: "+err.Error())
		return
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM execution_record WHERE environment_id IN (SELECT id FROM environment)`); err != nil {
		writeCleanError(w, "清空执行记录失败: "+err.Error())
		return
	}
	if _, err := tx.Exec(`DELETE FROM datasource WHERE environment_id IN (SELECT id FROM environment)`); err != nil {
		writeCleanError(w, "清空数据源失败: "+err.Error())
		return
	}
	if _, err := tx.Exec(`DELETE FROM environment`); err != nil {
		writeCleanError(w, "清空环境配置失败: "+err.Error())
		return
	}

	if err := tx.Commit(); err != nil {
		writeCleanError(w, "提交事务失败: "+err.Error())
		return
	}

	writeCleanSuccess(w, "所有环境及关联数据源、执行记录已清空")
}

// cleanDatasources deletes all datasources and their execution records.
func (s *Server) cleanDatasources(w http.ResponseWriter, r *http.Request) {
	tx, err := s.deps.DB.Begin()
	if err != nil {
		writeCleanError(w, "开始事务失败: "+err.Error())
		return
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM execution_record WHERE datasource_id IN (SELECT id FROM datasource)`); err != nil {
		writeCleanError(w, "清空执行记录失败: "+err.Error())
		return
	}
	if _, err := tx.Exec(`DELETE FROM datasource`); err != nil {
		writeCleanError(w, "清空数据源失败: "+err.Error())
		return
	}

	if err := tx.Commit(); err != nil {
		writeCleanError(w, "提交事务失败: "+err.Error())
		return
	}

	writeCleanSuccess(w, "所有数据源及关联执行记录已清空")
}

// cleanScripts deletes all SQL scripts and their release links / executions / archives.
func (s *Server) cleanScripts(w http.ResponseWriter, r *http.Request) {
	tx, err := s.deps.DB.Begin()
	if err != nil {
		writeCleanError(w, "开始事务失败: "+err.Error())
		return
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM execution_record WHERE script_id IN (SELECT id FROM sql_script)`); err != nil {
		writeCleanError(w, "清空执行记录失败: "+err.Error())
		return
	}
	if _, err := tx.Exec(`DELETE FROM release_script WHERE script_id IN (SELECT id FROM sql_script)`); err != nil {
		writeCleanError(w, "清空发布关联失败: "+err.Error())
		return
	}
	if _, err := tx.Exec(`DELETE FROM sql_script`); err != nil {
		writeCleanError(w, "清空SQL脚本失败: "+err.Error())
		return
	}

	if err := tx.Commit(); err != nil {
		writeCleanError(w, "提交事务失败: "+err.Error())
		return
	}

	writeCleanSuccess(w, "所有脚本及关联数据已清空")
}

// cleanReleases deletes all releases and their script links / executions / archives.
func (s *Server) cleanReleases(w http.ResponseWriter, r *http.Request) {
	tx, err := s.deps.DB.Begin()
	if err != nil {
		writeCleanError(w, "开始事务失败: "+err.Error())
		return
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM archive_log WHERE release_id IN (SELECT id FROM sql_release)`); err != nil {
		writeCleanError(w, "清空归档记录失败: "+err.Error())
		return
	}
	if _, err := tx.Exec(`DELETE FROM execution_record WHERE release_id IN (SELECT id FROM sql_release)`); err != nil {
		writeCleanError(w, "清空执行记录失败: "+err.Error())
		return
	}
	if _, err := tx.Exec(`DELETE FROM release_script WHERE release_id IN (SELECT id FROM sql_release)`); err != nil {
		writeCleanError(w, "清空发布关联失败: "+err.Error())
		return
	}
	if _, err := tx.Exec(`DELETE FROM sql_release`); err != nil {
		writeCleanError(w, "清空发布记录失败: "+err.Error())
		return
	}

	if err := tx.Commit(); err != nil {
		writeCleanError(w, "提交事务失败: "+err.Error())
		return
	}

	writeCleanSuccess(w, "所有发布及关联数据已清空")
}

// cleanExecutions deletes all execution records only.
func (s *Server) cleanExecutions(w http.ResponseWriter, r *http.Request) {
	if _, err := s.deps.DB.Exec(`DELETE FROM execution_record`); err != nil {
		writeCleanError(w, "清空执行记录失败: "+err.Error())
		return
	}
	writeCleanSuccess(w, "所有执行记录已清空")
}

// cleanArchives deletes all archive logs only.
func (s *Server) cleanArchives(w http.ResponseWriter, r *http.Request) {
	if _, err := s.deps.DB.Exec(`DELETE FROM archive_log`); err != nil {
		writeCleanError(w, "清空归档记录失败: "+err.Error())
		return
	}
	writeCleanSuccess(w, "所有归档记录已清空")
}

// --------------------------------------------------------------------------- //
// JSON helpers
// --------------------------------------------------------------------------- //

func writeCleanSuccess(w http.ResponseWriter, message string) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": message,
	})
}

func writeCleanError(w http.ResponseWriter, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": false,
		"message": message,
	})
}

// RegisterCleanup registers all data cleanup routes under /settings/clean/*.
func (s *Server) RegisterCleanup(mux *http.ServeMux) {
	mux.Handle("GET /settings/clean", s.authed(s.cleanDataPage))
	mux.Handle("POST /settings/clean/projects", s.authed(s.cleanProjects))
	mux.Handle("POST /settings/clean/versions", s.authed(s.cleanVersions))
	mux.Handle("POST /settings/clean/environments", s.authed(s.cleanEnvironments))
	mux.Handle("POST /settings/clean/datasources", s.authed(s.cleanDatasources))
	mux.Handle("POST /settings/clean/scripts", s.authed(s.cleanScripts))
	mux.Handle("POST /settings/clean/releases", s.authed(s.cleanReleases))
	mux.Handle("POST /settings/clean/executions", s.authed(s.cleanExecutions))
	mux.Handle("POST /settings/clean/archives", s.authed(s.cleanArchives))
}
