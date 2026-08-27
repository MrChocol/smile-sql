// Package model defines Go structs for every table in the platform metadata
// database (SQLite), plus string enums and filter types.
//
// Structs map 1:1 to the DDL in migrations/0001_init.sql (design §5).
// Nullable foreign keys / timestamps use pointer types so that a nil value
// unambiguously means "not set".
package model

// --------------------------------------------------------------------------- //
// Enums (string constants)
// --------------------------------------------------------------------------- //

// ScriptStatus — sql_script.status
const (
	ScriptStatusDraft    = "draft"
	ScriptStatusExecuted = "executed"
	ScriptStatusArchived = "archived"
)

// ReleaseStatus — sql_release.status
const (
	ReleaseStatusDraft     = "draft"
	ReleaseStatusPublished = "published"
	ReleaseStatusExecuting = "executing"
	ReleaseStatusArchived  = "archived"
)

// ExecStatus — execution_record.status
const (
	ExecStatusPending      = "pending"
	ExecStatusSuccess      = "success"
	ExecStatusFailed       = "failed"
	ExecStatusManualMarked = "manual_marked"
)

// MarkMethod — execution_record.mark_method
const (
	MarkMethodAuto   = "auto"
	MarkMethodManual = "manual"
)

// SqlType — sql_script.sql_type
const (
	SqlTypeDDL = "DDL"
	SqlTypeDML = "DML"
)

// VersionSource — project_version.source
const (
	VersionSourceManualInit = "manual_init"
	VersionSourceAutoInc    = "auto_inc"
)

// UserRole — user.role
const (
	RoleAdmin = "admin"
	RoleDBA   = "dba"
	RoleDev   = "dev"
)

// --------------------------------------------------------------------------- //
// Table structs
// --------------------------------------------------------------------------- //

// Project maps to table project (§5.1).
type Project struct {
	ID                int64  `json:"id"`
	Code              string `json:"code"`
	Name              string `json:"name"`
	OwnerTeam         string `json:"owner_team"`
	PomVersionCurrent string `json:"pom_version_current"`
	CreatedAt         string `json:"created_at"`
}

// ProjectVersion maps to table project_version (§5.2).
type ProjectVersion struct {
	ID        int64  `json:"id"`
	ProjectID int64  `json:"project_id"`
	Version   string `json:"version"`
	IsCurrent bool   `json:"is_current"`
	Source    string `json:"source"`
	CreatedAt string `json:"created_at"`
}

// Environment maps to table environment (§5.3).
type Environment struct {
	ID          int64  `json:"id"`
	Code        string `json:"code"`
	Name        string `json:"name"`
	PromoteOrder int   `json:"promote_order"`
}

// Datasource maps to table datasource (§5.4).
type Datasource struct {
	ID            int64  `json:"id"`
	ProjectID     int64  `json:"project_id"`
	EnvironmentID int64  `json:"environment_id"`
	Name          string `json:"name"`
	DBType        string `json:"db_type"`
	Host          string `json:"host"`
	Port          int    `json:"port"`
	DBName        string `json:"db_name"`
	Username      string `json:"username"`
	PasswordEnc   string `json:"password_enc"`
	Enabled       bool   `json:"enabled"`
	CreatedAt     string `json:"created_at"`
}

// SQLScript maps to table sql_script (§5.5).
type SQLScript struct {
	ID          int64  `json:"id"`
	ProjectID   int64  `json:"project_id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Content     string `json:"content"`
	SqlType     string `json:"sql_type"`
	CreatedBy   string `json:"created_by"`
	Status      string `json:"status"`
	CreatedAt   string `json:"created_at"`
}

// SQLRelease maps to table sql_release (§5.6).
type SQLRelease struct {
	ID         int64  `json:"id"`
	ProjectID  int64  `json:"project_id"`
	VersionID  *int64 `json:"version_id"`
	Title      string `json:"title"`
	Status     string `json:"status"`
	CreatedBy  string `json:"created_by"`
	CreatedAt  string `json:"created_at"`
}

// ReleaseScript maps to table release_script (§5.7).
type ReleaseScript struct {
	ID        int64  `json:"id"`
	ReleaseID int64  `json:"release_id"`
	ScriptID  int64  `json:"script_id"`
	CreatedAt string `json:"created_at"`
}

// ExecutionRecord maps to table execution_record (§5.8).
type ExecutionRecord struct {
	ID            int64   `json:"id"`
	ReleaseID     int64   `json:"release_id"`
	ScriptID      int64   `json:"script_id"`
	DatasourceID  int64   `json:"datasource_id"`
	EnvironmentID int64   `json:"environment_id"`
	Status        string  `json:"status"`
	MarkMethod    string  `json:"mark_method"`
	Result        string  `json:"result"`
	ExecutedBy    string  `json:"executed_by"`
	ExecutedAt    *string `json:"executed_at"`
	MarkedBy      string  `json:"marked_by"`
	MarkedAt      *string `json:"marked_at"`
	Error         string  `json:"error"`
}

// ArchiveLog maps to table archive_log (§5.9).
type ArchiveLog struct {
	ID            int64  `json:"id"`
	ReleaseID     int64  `json:"release_id"`
	ProjectID     int64  `json:"project_id"`
	VersionFrom   string `json:"version_from"`
	VersionTo     string `json:"version_to"`
	GitCommitHash string `json:"git_commit_hash"`
	CommitMessage string `json:"commit_message"`
	ArchivedBy    string `json:"archived_by"`
	ArchivedAt    string `json:"archived_at"`
}

// User maps to table user (§5.10).
type User struct {
	ID           int64  `json:"id"`
	Username     string `json:"username"`
	PasswordHash string `json:"-"`
	DisplayName  string `json:"display_name"`
	Role         string `json:"role"`
	CreatedAt    string `json:"created_at"`
}

// Settings maps to table settings (§5.10, single-row).
type Settings struct {
	ID           int64  `json:"id"`
	GitRepoURL   string `json:"git_repo_url"`
	GitTokenEnc  string `json:"git_token_enc"`
	GitUsername  string `json:"git_username"`
	GitEmail     string `json:"git_email"`
	EncryptKey   string `json:"encrypt_key"`
	AdminPwHash  string `json:"-"`
}

// --------------------------------------------------------------------------- //
// Filter types
// --------------------------------------------------------------------------- //

// ExecutionFilter holds optional filter criteria for querying execution records.
// nil pointer fields mean "no filter on this column".
type ExecutionFilter struct {
	ProjectID     *int64
	EnvironmentID *int64
	Status        *string
}
