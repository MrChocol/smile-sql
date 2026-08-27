package web

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"net/http"
	"sync"

	"golang.org/x/crypto/bcrypt"

	"sql-mgr/internal/model"
)

// --------------------------------------------------------------------------- //
// In-memory session store (development only; replace with Redis/DB later)
// --------------------------------------------------------------------------- //

var sessionStore = struct {
	sync.RWMutex
	tokens map[string]int64 // token → user ID
}{
	tokens: make(map[string]int64),
}

const sessionCookieName = "sqlmgr_session"

// generateToken returns a cryptographically random 32-byte hex string.
func generateToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic("crypto/rand failed: " + err.Error())
	}
	return hex.EncodeToString(b)
}

// --------------------------------------------------------------------------- //
// User lookup helpers
// --------------------------------------------------------------------------- //

func loadUserByID(db *sql.DB, id int64) (model.User, error) {
	var u model.User
	err := db.QueryRow(
		`SELECT id, username, password_hash, display_name, role, created_at FROM user WHERE id = ?`,
		id,
	).Scan(&u.ID, &u.Username, &u.PasswordHash, &u.DisplayName, &u.Role, &u.CreatedAt)
	return u, err
}

func loadUserByUsername(db *sql.DB, username string) (model.User, error) {
	var u model.User
	err := db.QueryRow(
		`SELECT id, username, password_hash, display_name, role, created_at FROM user WHERE username = ?`,
		username,
	).Scan(&u.ID, &u.Username, &u.PasswordHash, &u.DisplayName, &u.Role, &u.CreatedAt)
	return u, err
}

// --------------------------------------------------------------------------- //
// Admin bootstrap
// --------------------------------------------------------------------------- //

// EnsureAdmin inserts a default admin user (username=admin, password=admin123)
// if one does not already exist.  Safe to call on every startup.
func EnsureAdmin(db *sql.DB) error {
	var count int
	err := db.QueryRow(`SELECT COUNT(*) FROM user WHERE username = ?`, "admin").Scan(&count)
	if err != nil {
		return fmt.Errorf("check admin user: %w", err)
	}
	if count > 0 {
		return nil
	}
	hash, err := bcrypt.GenerateFromPassword([]byte("admin123"), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash admin password: %w", err)
	}
	_, err = db.Exec(
		`INSERT INTO user (username, password_hash, display_name, role) VALUES (?, ?, ?, ?)`,
		"admin", string(hash), "管理员", model.RoleAdmin,
	)
	if err != nil {
		return fmt.Errorf("insert admin user: %w", err)
	}
	return nil
}

// --------------------------------------------------------------------------- //
// Auth middleware
// --------------------------------------------------------------------------- //

// AuthMiddleware checks the session cookie, loads the user from the DB, and
// stores it in the request context.  Unauthenticated requests are redirected
// to /login.
func (s *Server) AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(sessionCookieName)
		if err != nil {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		sessionStore.RLock()
		uid, ok := sessionStore.tokens[c.Value]
		sessionStore.RUnlock()
		if !ok {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		u, err := loadUserByID(s.deps.DB, uid)
		if err != nil {
			sessionStore.Lock()
			delete(sessionStore.tokens, c.Value)
			sessionStore.Unlock()
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		ctx := withUser(r.Context(), u)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// authed wraps a HandlerFunc with AuthMiddleware, returning an http.Handler
// suitable for mux.Handle.
func (s *Server) authed(h http.HandlerFunc) http.Handler {
	return s.AuthMiddleware(h)
}

// --------------------------------------------------------------------------- //
// Login / logout handlers
// --------------------------------------------------------------------------- //

// loginForm handles GET /login.
func (s *Server) loginForm(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, "login.html", nil, "")
}

// loginSubmit handles POST /login.
func (s *Server) loginSubmit(w http.ResponseWriter, r *http.Request) {
	username := r.FormValue("username")
	password := r.FormValue("password")

	u, err := loadUserByUsername(s.deps.DB, username)
	if err != nil {
		s.render(w, r, "login.html", nil, "用户名或密码错误")
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)); err != nil {
		s.render(w, r, "login.html", nil, "用户名或密码错误")
		return
	}

	token := generateToken()
	sessionStore.Lock()
	sessionStore.tokens[token] = u.ID
	sessionStore.Unlock()

	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		MaxAge:   86400 * 7, // 7 days
	})
	http.Redirect(w, r, "/projects", http.StatusSeeOther)
}

// logout handles GET /logout.
func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookieName); err == nil {
		sessionStore.Lock()
		delete(sessionStore.tokens, c.Value)
		sessionStore.Unlock()
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		MaxAge:   -1,
	})
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}
