-- 0001_init.sql — SQL 统一管理平台 初始化 DDL (10 张表)
-- 依据: 设计文档 §5 数据模型
-- 时间用 TEXT(ISO8601), 布尔用 INTEGER(0/1), 主键 INTEGER PRIMARY KEY AUTOINCREMENT

PRAGMA foreign_keys = ON;

-- 5.1 project 项目
CREATE TABLE IF NOT EXISTS project (
    id                 INTEGER PRIMARY KEY AUTOINCREMENT,
    code               TEXT NOT NULL UNIQUE,
    name               TEXT NOT NULL,
    owner_team         TEXT NOT NULL DEFAULT '',
    pom_version_current TEXT NOT NULL DEFAULT '',
    created_at         TEXT NOT NULL DEFAULT (datetime('now'))
);

-- 5.2 project_version pom 版本线
CREATE TABLE IF NOT EXISTS project_version (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    project_id  INTEGER NOT NULL REFERENCES project(id),
    version     TEXT NOT NULL,
    is_current  INTEGER NOT NULL DEFAULT 0,
    source      TEXT NOT NULL DEFAULT 'manual_init',
    created_at  TEXT NOT NULL DEFAULT (datetime('now'))
);

-- 5.3 environment 环境
CREATE TABLE IF NOT EXISTS environment (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    code          TEXT NOT NULL UNIQUE,
    name          TEXT NOT NULL,
    promote_order INTEGER NOT NULL DEFAULT 0
);

-- 5.4 datasource 数据源
CREATE TABLE IF NOT EXISTS datasource (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    project_id     INTEGER NOT NULL REFERENCES project(id),
    environment_id INTEGER NOT NULL REFERENCES environment(id),
    name           TEXT NOT NULL,
    db_type        TEXT NOT NULL DEFAULT 'mysql',
    host           TEXT NOT NULL DEFAULT '',
    port           INTEGER NOT NULL DEFAULT 3306,
    db_name        TEXT NOT NULL DEFAULT '',
    username       TEXT NOT NULL DEFAULT '',
    password_enc   TEXT NOT NULL DEFAULT '',
    enabled        INTEGER NOT NULL DEFAULT 1,
    created_at     TEXT NOT NULL DEFAULT (datetime('now'))
);

-- 5.5 sql_script 脚本
CREATE TABLE IF NOT EXISTS sql_script (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    project_id  INTEGER NOT NULL REFERENCES project(id),
    title       TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    content     TEXT NOT NULL DEFAULT '',
    sql_type    TEXT NOT NULL DEFAULT 'DDL',
    created_by  TEXT NOT NULL DEFAULT '',
    status      TEXT NOT NULL DEFAULT 'draft',
    created_at  TEXT NOT NULL DEFAULT (datetime('now'))
);

-- 5.6 sql_release 发布
CREATE TABLE IF NOT EXISTS sql_release (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    project_id INTEGER NOT NULL REFERENCES project(id),
    version_id INTEGER REFERENCES project_version(id),
    title      TEXT NOT NULL,
    status     TEXT NOT NULL DEFAULT 'draft',
    created_by TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL DEFAULT (datetime('now'))
);

-- 5.7 release_script 发布-脚本关联 (多对多)
CREATE TABLE IF NOT EXISTS release_script (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    release_id INTEGER NOT NULL REFERENCES sql_release(id),
    script_id  INTEGER NOT NULL REFERENCES sql_script(id),
    created_at TEXT NOT NULL DEFAULT (datetime('now'))
);

-- 5.8 execution_record 执行记录 (单一事实来源)
CREATE TABLE IF NOT EXISTS execution_record (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    release_id     INTEGER NOT NULL REFERENCES sql_release(id),
    script_id      INTEGER NOT NULL REFERENCES sql_script(id),
    datasource_id  INTEGER NOT NULL REFERENCES datasource(id),
    environment_id INTEGER NOT NULL REFERENCES environment(id),
    status         TEXT NOT NULL DEFAULT 'pending',
    mark_method    TEXT NOT NULL DEFAULT 'auto',
    result         TEXT NOT NULL DEFAULT '',
    executed_by    TEXT NOT NULL DEFAULT '',
    executed_at    TEXT,
    marked_by      TEXT NOT NULL DEFAULT '',
    marked_at      TEXT,
    error          TEXT NOT NULL DEFAULT ''
);

-- 5.9 archive_log 归档 / Git 提交记录
CREATE TABLE IF NOT EXISTS archive_log (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    release_id      INTEGER NOT NULL REFERENCES sql_release(id),
    project_id      INTEGER NOT NULL REFERENCES project(id),
    version_from    TEXT NOT NULL DEFAULT '',
    version_to      TEXT NOT NULL DEFAULT '',
    git_commit_hash TEXT NOT NULL DEFAULT '',
    commit_message  TEXT NOT NULL DEFAULT '',
    archived_by     TEXT NOT NULL DEFAULT '',
    archived_at     TEXT NOT NULL DEFAULT (datetime('now'))
);

-- 5.10 user 用户
CREATE TABLE IF NOT EXISTS user (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    username      TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL DEFAULT '',
    display_name  TEXT NOT NULL DEFAULT '',
    role          TEXT NOT NULL DEFAULT 'dev',
    created_at    TEXT NOT NULL DEFAULT (datetime('now'))
);

-- 5.10 settings 全局配置 (单行)
CREATE TABLE IF NOT EXISTS settings (
    id             INTEGER PRIMARY KEY CHECK (id = 1),
    git_repo_url   TEXT NOT NULL DEFAULT '',
    git_token_enc  TEXT NOT NULL DEFAULT '',
    git_username   TEXT NOT NULL DEFAULT '',
    git_email      TEXT NOT NULL DEFAULT '',
    encrypt_key    TEXT NOT NULL DEFAULT '',
    admin_pw_hash  TEXT NOT NULL DEFAULT ''
);

-- 索引
CREATE INDEX IF NOT EXISTS idx_project_version_project    ON project_version(project_id);
CREATE INDEX IF NOT EXISTS idx_datasource_project_env     ON datasource(project_id, environment_id);
CREATE INDEX IF NOT EXISTS idx_sql_script_project          ON sql_script(project_id);
CREATE INDEX IF NOT EXISTS idx_sql_script_status           ON sql_script(status);
CREATE INDEX IF NOT EXISTS idx_sql_release_project        ON sql_release(project_id);
CREATE INDEX IF NOT EXISTS idx_sql_release_status         ON sql_release(status);
CREATE INDEX IF NOT EXISTS idx_release_script_release    ON release_script(release_id);
CREATE INDEX IF NOT EXISTS idx_release_script_script     ON release_script(script_id);
CREATE INDEX IF NOT EXISTS idx_exec_record_release        ON execution_record(release_id);
CREATE INDEX IF NOT EXISTS idx_exec_record_script         ON execution_record(script_id);
CREATE INDEX IF NOT EXISTS idx_exec_record_datasource     ON execution_record(datasource_id);
CREATE INDEX IF NOT EXISTS idx_exec_record_env            ON execution_record(environment_id);
CREATE INDEX IF NOT EXISTS idx_exec_record_status         ON execution_record(status);
CREATE INDEX IF NOT EXISTS idx_archive_log_release        ON archive_log(release_id);
CREATE INDEX IF NOT EXISTS idx_archive_log_project        ON archive_log(project_id);
