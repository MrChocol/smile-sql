package web

import (
	"database/sql"
	"net/http"

	"sql-mgr/internal/model"
)

// revealData is the page-specific data passed to datasource_reveal.html.
type revealData struct {
	DatasourceID   string // from the URL path
	DatasourceName string // populated on successful reveal
	Plaintext      string // populated on successful reveal
	Verified       bool   // true when admin password was verified
}

// RevealForm handles GET /datasources/{id}/reveal.
//
// Displays a form requesting the management (admin) password. The
// datasource ID is extracted from the path so the form can POST back
// to the same URL.
func (s *Server) RevealForm(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	s.render(w, r, "datasource_reveal.html", revealData{
		DatasourceID: id,
	}, "")
}

// RevealSubmit handles POST /datasources/{id}/reveal.
//
// It verifies the management password via Crypto.VerifyAdminPw. On
// failure it re-renders the form with a flash error. On success it
// queries the datasource row directly from the DB (avoiding cross-agent
// repo dependencies), decrypts password_enc via Crypto.Decrypt, and
// renders a page that temporarily displays the plaintext.
//
// Decrypt is backward-compatible: Stub-era plaintext values are returned
// unchanged, so the reveal endpoint works seamlessly during the transition
// from Stub to AES.
func (s *Server) RevealSubmit(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	adminPw := r.FormValue("admin_password")

	// --- Step 1: verify management password ---
	if !s.deps.Crypto.VerifyAdminPw(adminPw) {
		s.render(w, r, "datasource_reveal.html", revealData{
			DatasourceID: id,
		}, "管理密码错误")
		return
	}

	// --- Step 2: load the datasource row ---
	var ds model.Datasource
	err := s.deps.DB.QueryRow(
		`SELECT id, project_id, environment_id, name, db_type, host, port,
		        db_name, username, password_enc, enabled, created_at
		 FROM datasource WHERE id = ?`,
		id,
	).Scan(
		&ds.ID, &ds.ProjectID, &ds.EnvironmentID, &ds.Name, &ds.DBType,
		&ds.Host, &ds.Port, &ds.DBName, &ds.Username, &ds.PasswordEnc,
		&ds.Enabled, &ds.CreatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			s.render(w, r, "datasource_reveal.html", revealData{
				DatasourceID: id,
			}, "数据源不存在")
			return
		}
		s.render(w, r, "datasource_reveal.html", revealData{
			DatasourceID: id,
		}, "查询数据源失败: "+err.Error())
		return
	}

	// --- Step 3: decrypt the password ---
	plaintext, err := s.deps.Crypto.Decrypt(ds.PasswordEnc)
	if err != nil {
		s.render(w, r, "datasource_reveal.html", revealData{
			DatasourceID: id,
		}, "解密失败: "+err.Error())
		return
	}

	// --- Step 4: render the plaintext (temporary display) ---
	s.render(w, r, "datasource_reveal.html", revealData{
		DatasourceID:   id,
		DatasourceName: ds.Name,
		Plaintext:      plaintext,
		Verified:       true,
	}, "")
}

// RegisterReveal registers the password-reveal routes on the given mux.
// Both routes are wrapped with s.authed so the auth middleware runs and
// CurrentUser is available in the handler context.
func (s *Server) RegisterReveal(mux *http.ServeMux) {
	mux.Handle("GET /datasources/{id}/reveal", s.authed(s.RevealForm))
	mux.Handle("POST /datasources/{id}/reveal", s.authed(s.RevealSubmit))
}
