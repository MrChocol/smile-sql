// Package web — dashboard handler (home page).
//
// This file implements the dashboard view that shows an overview of the
// SQL lifecycle, quick actions, statistics, and recent execution records.
package web

import (
	"net/http"
	"time"

	"sql-mgr/internal/model"
	"sql-mgr/internal/repo"
)

// --------------------------------------------------------------------------- //
// View data structs
// --------------------------------------------------------------------------- //

// DashboardStats holds summary statistics shown on the dashboard.
type DashboardStats struct {
	ProjectCount          int
	PendingScriptCount    int
	MonthlyExecutionCount int
	ArchiveCount          int
}

// DashboardRecentExecution is a flattened execution record with display
// names for the dashboard's recent-executions table.
type DashboardRecentExecution struct {
	ID              int64
	ScriptTitle     string
	ProjectName     string
	EnvironmentName string
	Status          string
	Error           string
	ExecutedBy      string
	ExecutedAt      *string
}

// DashboardView is the view-data struct passed to dashboard.html.
type DashboardView struct {
	CurrentDate      string
	CurrentWeekday   string
	Stats            DashboardStats
	RecentExecutions []DashboardRecentExecution
}

// --------------------------------------------------------------------------- //
// Route registration
// --------------------------------------------------------------------------- //

// RegisterDashboard adds dashboard-related routes to the given mux.
// It should be called from main.go after srv.Routes().
func (s *Server) RegisterDashboard(mux *http.ServeMux) {
	mux.Handle("GET /dashboard", s.authed(s.dashboard))
}

// --------------------------------------------------------------------------- //
// Handlers
// --------------------------------------------------------------------------- //

// dashboard handles GET /dashboard — renders the home dashboard with
// lifecycle overview, quick actions, stats, and recent executions.
func (s *Server) dashboard(w http.ResponseWriter, r *http.Request) {
	// 1. Current date info
	now := time.Now()
	weekdays := []string{"星期日", "星期一", "星期二", "星期三", "星期四", "星期五", "星期六"}

	view := DashboardView{
		CurrentDate:    now.Format("2006年01月02日"),
		CurrentWeekday: weekdays[now.Weekday()],
	}

	// 2. Statistics — load from DB, zero values on error
	view.Stats = s.loadDashboardStats()

	// 3. Recent execution records (last 5)
	view.RecentExecutions = s.loadRecentExecutions(5)

	s.render(w, r, "dashboard.html", view, "")
}

// --------------------------------------------------------------------------- //
// Helpers
// --------------------------------------------------------------------------- //

// loadDashboardStats queries summary counts for the dashboard stat cards.
func (s *Server) loadDashboardStats() DashboardStats {
	stats := DashboardStats{}

	// Project count
	pr := repo.NewProjectRepo(s.deps.DB)
	if projects, err := pr.List(); err == nil {
		stats.ProjectCount = len(projects)
	}

	// Pending (draft) script count — pass 0 to list all projects
	sr := repo.NewScriptRepo(s.deps.DB)
	if scripts, err := sr.List(0); err == nil {
		count := 0
		for _, sc := range scripts {
			if sc.Status == model.ScriptStatusDraft {
				count++
			}
		}
		stats.PendingScriptCount = count
	}

	// Monthly execution count
	er := repo.NewExecutionRepo(s.deps.DB)
	if records, err := er.ListWithFilter(model.ExecutionFilter{}); err == nil {
		monthStart := time.Now().Format("2006-01")
		count := 0
		for _, rec := range records {
			if rec.ExecutedAt != nil && len(*rec.ExecutedAt) >= 7 {
				if (*rec.ExecutedAt)[:7] == monthStart {
					count++
				}
			}
		}
		stats.MonthlyExecutionCount = count
	}

	// Archive count
	ar := repo.NewArchiveRepo(s.deps.DB)
	if logs, err := ar.List(); err == nil {
		stats.ArchiveCount = len(logs)
	}

	return stats
}

// loadRecentExecutions fetches the most recent N execution records with
// resolved display names (script title, project name, environment name).
func (s *Server) loadRecentExecutions(limit int) []DashboardRecentExecution {
	er := repo.NewExecutionRepo(s.deps.DB)
	records, err := er.ListWithFilter(model.ExecutionFilter{})
	if err != nil {
		return nil
	}

	// Build lookup maps for display names
	projectMap := s.loadProjectMap()       // projectID -> name
	envMap := s.loadEnvironmentMap()       // envID -> name
	scriptMap := s.loadScriptMap()         // scriptID -> title
	scriptProjectMap := s.loadScriptProjectMap() // scriptID -> projectID

	// Records are ordered by id ASC; we want the most recent first, so reverse.
	// Then take up to `limit` records.
	result := make([]DashboardRecentExecution, 0, limit)
	for i := len(records) - 1; i >= 0 && len(result) < limit; i-- {
		rec := records[i]
		item := DashboardRecentExecution{
			ID:         rec.ID,
			Status:     rec.Status,
			Error:      rec.Error,
			ExecutedBy: rec.ExecutedBy,
			ExecutedAt: rec.ExecutedAt,
		}

		// Script title
		if st, ok := scriptMap[rec.ScriptID]; ok {
			item.ScriptTitle = st
		} else {
			item.ScriptTitle = "未知脚本"
		}

		// Environment name
		if e, ok := envMap[rec.EnvironmentID]; ok {
			item.EnvironmentName = e
		} else {
			item.EnvironmentName = "—"
		}

		// Project name (resolved via script -> project)
		if pid, ok := scriptProjectMap[rec.ScriptID]; ok {
			if p, ok := projectMap[pid]; ok {
				item.ProjectName = p
			} else {
				item.ProjectName = "—"
			}
		} else {
			item.ProjectName = "—"
		}

		result = append(result, item)
	}

	return result
}

// loadProjectMap returns a map of project ID -> project name.
func (s *Server) loadProjectMap() map[int64]string {
	m := make(map[int64]string)
	pr := repo.NewProjectRepo(s.deps.DB)
	projects, err := pr.List()
	if err != nil {
		return m
	}
	for _, p := range projects {
		m[p.ID] = p.Name
	}
	return m
}

// loadEnvironmentMap returns a map of environment ID -> environment name.
func (s *Server) loadEnvironmentMap() map[int64]string {
	m := make(map[int64]string)
	er := repo.NewEnvironmentRepo(s.deps.DB)
	envs, err := er.List()
	if err != nil {
		return m
	}
	for _, e := range envs {
		m[e.ID] = e.Name
	}
	return m
}

// loadScriptMap returns a map of script ID -> script title.
func (s *Server) loadScriptMap() map[int64]string {
	m := make(map[int64]string)
	sr := repo.NewScriptRepo(s.deps.DB)
	scripts, err := sr.List(0)
	if err != nil {
		return m
	}
	for _, sc := range scripts {
		m[sc.ID] = sc.Title
	}
	return m
}

// loadScriptProjectMap returns a map of script ID -> project ID.
func (s *Server) loadScriptProjectMap() map[int64]int64 {
	m := make(map[int64]int64)
	sr := repo.NewScriptRepo(s.deps.DB)
	scripts, err := sr.List(0)
	if err != nil {
		return m
	}
	for _, sc := range scripts {
		m[sc.ID] = sc.ProjectID
	}
	return m
}
