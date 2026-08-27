// Package repo -- settings repository (system configuration).
//
// SettingsRepo manages the single-row settings table.  The git_token_enc
// field is stored as-is; encryption is the caller's responsibility.
package repo

import (
	"database/sql"

	"sql-mgr/internal/model"
)

// SettingsRepo provides data access for the single-row settings table.
type SettingsRepo struct {
	db *sql.DB
}

// NewSettingsRepo returns a SettingsRepo backed by the given *sql.DB.
func NewSettingsRepo(db *sql.DB) *SettingsRepo {
	return &SettingsRepo{db: db}
}

// GetSettings returns the single settings row (id=1).
// If the row does not exist yet, it returns a zero-value Settings
// with ID=0; callers should use SaveSettings to create it.
func (r *SettingsRepo) GetSettings() (model.Settings, error) {
	var s model.Settings
	err := r.db.QueryRow(
		`SELECT id, git_repo_url, git_token_enc, git_username, git_email, encrypt_key, admin_pw_hash
		 FROM settings WHERE id = 1`,
	).Scan(&s.ID, &s.GitRepoURL, &s.GitTokenEnc, &s.GitUsername, &s.GitEmail, &s.EncryptKey, &s.AdminPwHash)
	if err == sql.ErrNoRows {
		return model.Settings{}, nil
	}
	return s, err
}

// SaveSettings inserts or updates the single settings row (id=1).
// It performs an UPSERT: insert on first call, update thereafter.
func (r *SettingsRepo) SaveSettings(s model.Settings) error {
	_, err := r.db.Exec(
		`INSERT INTO settings (id, git_repo_url, git_token_enc, git_username, git_email, encrypt_key, admin_pw_hash)
		 VALUES (1, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET
		   git_repo_url  = excluded.git_repo_url,
		   git_token_enc = excluded.git_token_enc,
		   git_username  = excluded.git_username,
		   git_email     = excluded.git_email,
		   encrypt_key   = excluded.encrypt_key,
		   admin_pw_hash = excluded.admin_pw_hash`,
		s.GitRepoURL, s.GitTokenEnc, s.GitUsername, s.GitEmail, s.EncryptKey, s.AdminPwHash,
	)
	return err
}
