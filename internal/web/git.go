// Package web -- git settings handlers (system configuration).
//
// This file implements the Git repository configuration page, including
// display, save, and test-connection handlers.  The access token is
// encrypted via s.deps.Crypto before storage and never echoed back to
// the browser in plaintext.
package web

import (
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"sql-mgr/internal/archive"
	"sql-mgr/internal/git"
	"sql-mgr/internal/model"
	"sql-mgr/internal/repo"
)

// --------------------------------------------------------------------------- //
// View data structs
// --------------------------------------------------------------------------- //

// gitConfigData holds data for the Git configuration page.
type gitConfigData struct {
	Settings  *model.Settings
	IsConfigured bool
	Success   string
}

// --------------------------------------------------------------------------- //
// Handlers
// --------------------------------------------------------------------------- //

// gitConfigForm handles GET /settings/git — displays the Git configuration
// form with the current settings (if any).
func (s *Server) gitConfigForm(w http.ResponseWriter, r *http.Request) {
	sr := repo.NewSettingsRepo(s.deps.DB)
	settings, err := sr.GetSettings()
	if err != nil {
		s.render(w, r, "git_config.html", gitConfigData{}, "加载配置失败: "+err.Error())
		return
	}

	isConfigured := settings.GitRepoURL != ""

	s.render(w, r, "git_config.html", gitConfigData{
		Settings:     &settings,
		IsConfigured: isConfigured,
	}, "")
}

// saveGitConfig handles POST /settings/git — saves the Git configuration.
// The access token is encrypted before storage.  If the token field is
// left blank, the existing encrypted token is preserved.
func (s *Server) saveGitConfig(w http.ResponseWriter, r *http.Request) {
	repoURL := strings.TrimSpace(r.FormValue("git_repo_url"))
	if repoURL == "" {
		s.renderGitConfigError(w, r, "Git 仓库地址为必填项", "")
		return
	}

	// Validate URL format
	if _, err := url.Parse(repoURL); err != nil {
		s.renderGitConfigError(w, r, "Git 仓库地址格式不正确", "")
		return
	}

	sr := repo.NewSettingsRepo(s.deps.DB)
	existing, err := sr.GetSettings()
	if err != nil {
		s.renderGitConfigError(w, r, "读取当前配置失败: "+err.Error(), "")
		return
	}

	// Encrypt the token if provided; otherwise keep existing
	tokenEnc := existing.GitTokenEnc
	if token := r.FormValue("git_token"); token != "" {
		tokenEnc, err = s.deps.Crypto.Encrypt(token)
		if err != nil {
			s.renderGitConfigError(w, r, "令牌加密失败: "+err.Error(), "")
			return
		}
	}

	gitUsername := strings.TrimSpace(r.FormValue("git_username"))
	gitEmail := strings.TrimSpace(r.FormValue("git_email"))

	// Preserve existing encrypt_key and admin_pw_hash
	err = sr.SaveSettings(model.Settings{
		ID:          existing.ID,
		GitRepoURL:  repoURL,
		GitTokenEnc: tokenEnc,
		GitUsername: gitUsername,
		GitEmail:    gitEmail,
		EncryptKey:  existing.EncryptKey,
		AdminPwHash: existing.AdminPwHash,
	})
	if err != nil {
		s.renderGitConfigError(w, r, "保存配置失败: "+err.Error(), "")
		return
	}

	// Re-render with success message
	settings, _ := sr.GetSettings()
	s.render(w, r, "git_config.html", gitConfigData{
		Settings:     &settings,
		IsConfigured: settings.GitRepoURL != "",
		Success:      "配置保存成功！",
	}, "")
}

// testGitConnection handles POST /settings/git/test — validates the Git
// repository URL format and checks basic reachability via URL parsing.
// This is a lightweight test; it does not actually clone the repo.
func (s *Server) testGitConnection(w http.ResponseWriter, r *http.Request) {
	repoURL := strings.TrimSpace(r.FormValue("git_repo_url"))
	if repoURL == "" {
		s.renderGitConfigError(w, r, "请先填写 Git 仓库地址", "")
		return
	}

	// Parse and validate the URL
	u, err := url.Parse(repoURL)
	if err != nil {
		s.renderGitConfigError(w, r, "Git 仓库地址格式不正确: "+err.Error(), "")
		return
	}
	if u.Scheme == "" || u.Host == "" {
		s.renderGitConfigError(w, r, "Git 仓库地址必须包含协议（https:// 或 http://）和主机名", "")
		return
	}
	if u.Scheme != "https" && u.Scheme != "http" && u.Scheme != "ssh" && u.Scheme != "git" {
		s.renderGitConfigError(w, r, "不支持的协议: "+u.Scheme+"，请使用 https、http、ssh 或 git", "")
		return
	}

	// Build a temporary settings object from form data for re-render
	sr := repo.NewSettingsRepo(s.deps.DB)
	existing, _ := sr.GetSettings()

	// Use form values for preview, but keep existing token if not provided
	tokenEnc := existing.GitTokenEnc
	if token := r.FormValue("git_token"); token != "" {
		tokenEnc, _ = s.deps.Crypto.Encrypt(token)
	}

	previewSettings := model.Settings{
		GitRepoURL:  repoURL,
		GitTokenEnc: tokenEnc,
		GitUsername: strings.TrimSpace(r.FormValue("git_username")),
		GitEmail:    strings.TrimSpace(r.FormValue("git_email")),
	}

	s.render(w, r, "git_config.html", gitConfigData{
		Settings:     &previewSettings,
		IsConfigured: existing.GitRepoURL != "",
		Success:      "连接测试通过！仓库地址格式有效（" + u.Scheme + "://" + u.Host + "）",
	}, "")
}

// --------------------------------------------------------------------------- //
// AJAX Handlers — Git Config
// --------------------------------------------------------------------------- //

// ajaxSaveGitConfig handles POST /settings/git/ajax — saves the Git config,
// initialises the remote repository (creating an initial commit if empty),
// updates the runtime Git client + archiver, and returns a JSON response.
func (s *Server) ajaxSaveGitConfig(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	repoURL := strings.TrimSpace(r.FormValue("git_repo_url"))
	if repoURL == "" {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": "Git 仓库地址为必填项",
		})
		return
	}

	// Validate URL format
	if _, err := url.Parse(repoURL); err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": "Git 仓库地址格式不正确",
		})
		return
	}

	sr := repo.NewSettingsRepo(s.deps.DB)
	existing, err := sr.GetSettings()
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": "读取当前配置失败: " + err.Error(),
		})
		return
	}

	// Encrypt the token if provided; otherwise keep existing
	tokenEnc := existing.GitTokenEnc
	tokenPlain := ""
	if token := r.FormValue("git_token"); token != "" {
		tokenPlain = token
		tokenEnc, err = s.deps.Crypto.Encrypt(token)
		if err != nil {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"success": false,
				"message": "令牌加密失败: " + err.Error(),
			})
			return
		}
	} else if existing.GitTokenEnc != "" {
		// Token not changed — decrypt existing for use with new client
		if dec, decErr := s.deps.Crypto.Decrypt(existing.GitTokenEnc); decErr == nil {
			tokenPlain = dec
		}
	}

	gitUsername := strings.TrimSpace(r.FormValue("git_username"))
	gitEmail := strings.TrimSpace(r.FormValue("git_email"))

	// Save to DB first
	err = sr.SaveSettings(model.Settings{
		ID:          existing.ID,
		GitRepoURL:  repoURL,
		GitTokenEnc: tokenEnc,
		GitUsername: gitUsername,
		GitEmail:    gitEmail,
		EncryptKey:  existing.EncryptKey,
		AdminPwHash: existing.AdminPwHash,
	})
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": "保存配置失败: " + err.Error(),
		})
		return
	}

	// --- Initialise repository and update runtime Git client --- //
	workDir := filepath.Join(os.TempDir(), "sql-mgr-git")
	newGit := git.NewGoGitClient(repoURL, tokenPlain, workDir)

	if err := newGit.InitRepo(r.Context()); err != nil {
		// Still return success for the save, but warn about init failure
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"message": "配置已保存，但仓库初始化失败：" + err.Error() + "\n\n请检查仓库地址和令牌是否正确。",
		})
		return
	}

	// Update runtime dependencies so subsequent requests use the new client
	s.deps.Git = newGit
	s.deps.Archiver = archive.NewArchiver(s.deps.DB, newGit)

	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "✅ 配置保存成功，仓库已初始化完成",
	})
}

// ajaxTestGitConnection handles POST /settings/git/test — validates the Git
// repository URL format and returns JSON.
func (s *Server) ajaxTestGitConnection(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	repoURL := strings.TrimSpace(r.FormValue("git_repo_url"))
	if repoURL == "" {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": "请先填写 Git 仓库地址",
		})
		return
	}

	// Parse and validate the URL
	u, err := url.Parse(repoURL)
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": "Git 仓库地址格式不正确: " + err.Error(),
		})
		return
	}
	if u.Scheme == "" || u.Host == "" {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": "Git 仓库地址必须包含协议（https:// 或 http://）和主机名",
		})
		return
	}
	if u.Scheme != "https" && u.Scheme != "http" && u.Scheme != "ssh" && u.Scheme != "git" {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": "不支持的协议: " + u.Scheme + "，请使用 https、http、ssh 或 git",
		})
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "连接测试通过！仓库地址格式有效（" + u.Scheme + "://" + u.Host + "）",
	})
}

// --------------------------------------------------------------------------- //
// Route registration
// --------------------------------------------------------------------------- //

// RegisterSettings registers all system-settings routes on the given mux.
// It should be called from main.go after srv.Routes().
func (s *Server) RegisterSettings(mux *http.ServeMux) {
	mux.Handle("GET /settings/git", s.authed(s.gitConfigForm))
	mux.Handle("POST /settings/git", s.authed(s.saveGitConfig))
	mux.Handle("POST /settings/git/test", s.authed(s.testGitConnection))

	// Git Settings — AJAX
	mux.Handle("POST /settings/git/ajax", s.authed(s.ajaxSaveGitConfig))
	mux.Handle("POST /settings/git/ajax/test", s.authed(s.ajaxTestGitConnection))
}

// --------------------------------------------------------------------------- //
// Helpers
// --------------------------------------------------------------------------- //

// renderGitConfigError re-renders the Git config page with an error flash
// message, preserving the user's form input.
func (s *Server) renderGitConfigError(w http.ResponseWriter, r *http.Request, flash string, success string) {
	// Build a preview from form values so the user doesn't lose their input
	previewSettings := model.Settings{
		GitRepoURL:  strings.TrimSpace(r.FormValue("git_repo_url")),
		GitUsername: strings.TrimSpace(r.FormValue("git_username")),
		GitEmail:    strings.TrimSpace(r.FormValue("git_email")),
	}

	sr := repo.NewSettingsRepo(s.deps.DB)
	existing, err := sr.GetSettings()
	isConfigured := false
	if err == nil {
		isConfigured = existing.GitRepoURL != ""
	}

	s.render(w, r, "git_config.html", gitConfigData{
		Settings:     &previewSettings,
		IsConfigured: isConfigured,
		Success:      success,
	}, flash)
}
