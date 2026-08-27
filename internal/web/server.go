package web

import (
	"html/template"
	"net/http"

	"sql-mgr/internal/model"
	"sql-mgr/web/static"
	"sql-mgr/web/templates"
)

// viewData is the top-level data passed to every rendered template.
// The layout accesses .User and .Flash; page templates access .Data.
type viewData struct {
	User  *model.User
	Flash string
	Data  any
}

// Server holds shared dependencies for all HTTP handlers.
type Server struct {
	deps *Deps
}

// New returns a Server wired to the given dependencies.
func New(deps *Deps) *Server {
	return &Server{deps: deps}
}

// Routes creates and returns a *http.ServeMux with all Sprint 0 routes
// registered.  Module-specific routes are added to the returned mux by
// each workflow agent (see the comment block below).
func (s *Server) Routes() *http.ServeMux {
	mux := http.NewServeMux()

	// --- Sprint 0: auth routes (public) ---
	mux.HandleFunc("GET /login", s.loginForm)
	mux.HandleFunc("POST /login", s.loginSubmit)
	mux.HandleFunc("GET /logout", s.logout)

	// --- Sprint 0: root (authenticated) ---
	mux.Handle("GET /{$}", s.authed(s.index))

	// ------------------------------------------------------------------ //
	// Module route registration (added by each workflow agent)
	// ------------------------------------------------------------------ //
	//
	// Each agent creates handler functions and registers them on the mux
	// returned by Routes().  Two integration points are available:
	//
	//   1. Add routes in main.go after calling Routes():
	//
	//        mux := srv.Routes()
	//        mux.Handle("GET /projects", srv.authed(projectHandler.List))
	//        mux.Handle("POST /projects", srv.authed(projectHandler.Create))
	//        mux.Handle("GET /projects/{id}", srv.authed(projectHandler.Detail))
	//
	//   2. Or add a Register function per module:
	//
	//        func RegisterProjects(mux *http.ServeMux, s *Server) {
	//            mux.Handle("GET /projects", s.authed(s.listProjects))
	//        }
	//
	// All authenticated routes MUST be wrapped with s.authed() so the
	// AuthMiddleware runs and CurrentUser(r.Context()) is available.
	//
	// Static assets
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServerFS(static.Files)))
	// ------------------------------------------------------------------ //

	return mux
}

// index handles GET / — redirects authenticated users to /dashboard.
func (s *Server) index(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
}

// render parses the layout template together with the named page template,
// injects the current user (from context) and an optional flash message,
// and writes the rendered HTML to w.
//
// Parameters:
//   - w     : the response writer
//   - r     : the request (used to extract CurrentUser from context)
//   - name  : the page template file name (e.g. "login.html")
//   - data  : page-specific data (may be nil)
//   - flash : one-time error/info message (may be "")
func (s *Server) render(w http.ResponseWriter, r *http.Request, name string, data any, flash string) {
	vd := viewData{Data: data, Flash: flash}
	if u, ok := CurrentUser(r.Context()); ok {
		vd.User = &u
	}

	tmpl, err := template.New("").ParseFS(templates.Files, "layout.html", name)
	if err != nil {
		http.Error(w, "template parse error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if err := tmpl.ExecuteTemplate(w, "layout.html", vd); err != nil {
		http.Error(w, "template exec error: "+err.Error(), http.StatusInternalServerError)
	}
}
