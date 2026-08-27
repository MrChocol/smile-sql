// Package config loads platform configuration from a YAML file (simple
// key: value lines) with environment-variable overrides and sensible defaults.
//
// No third-party YAML dependency is required; the parser understands only
// flat "key: value" lines (no nesting, no lists).
package config

import (
	"bufio"
	"os"
	"strconv"
	"strings"
)

// Config holds all platform configuration.
type Config struct {
	ServerPort  int    `yaml:"server_port"`
	SQLitePath  string `yaml:"sqlite_path"`
	GitRepoURL  string `yaml:"git_repo_url"`
	GitTokenEnc string `yaml:"git_token_enc"`
	EncryptKey  string `yaml:"encrypt_key"`
	AdminPwHash string `yaml:"admin_pw_hash"`
}

// Default returns a Config populated with development defaults.
func Default() Config {
	return Config{
		ServerPort:  8080,
		SQLitePath:  "sql-mgr.db",
		GitRepoURL:  "",
		GitTokenEnc: "",
		EncryptKey:  "change-me-32-byte-encrypt-key!!!",
		AdminPwHash: "",
	}
}

// Load reads config.yaml from the given path (if it exists), then applies
// environment-variable overrides, and finally fills in defaults for any
// field still unset.  A missing file is not an error — defaults are used.
func Load(path string) Config {
	cfg := Default()

	// 1. Read YAML file (simple key: value parser).
	if f, err := os.Open(path); err == nil {
		parseSimpleYAML(f, &cfg)
		f.Close()
	}

	// 2. Environment-variable overrides.
	if v := os.Getenv("SQLMGR_SERVER_PORT"); v != "" {
		if port, err := strconv.Atoi(v); err == nil {
			cfg.ServerPort = port
		}
	}
	if v := os.Getenv("SQLMGR_SQLITE_PATH"); v != "" {
		cfg.SQLitePath = v
	}
	if v := os.Getenv("SQLMGR_GIT_REPO_URL"); v != "" {
		cfg.GitRepoURL = v
	}
	if v := os.Getenv("SQLMGR_GIT_TOKEN_ENC"); v != "" {
		cfg.GitTokenEnc = v
	}
	if v := os.Getenv("SQLMGR_ENCRYPT_KEY"); v != "" {
		cfg.EncryptKey = v
	}
	if v := os.Getenv("SQLMGR_ADMIN_PW_HASH"); v != "" {
		cfg.AdminPwHash = v
	}

	return cfg
}

// parseSimpleYAML reads flat "key: value" lines and sets matching fields.
func parseSimpleYAML(f *os.File, cfg *Config) {
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		idx := strings.Index(line, ":")
		if idx < 0 {
			continue
		}
		key := strings.TrimSpace(line[:idx])
		val := strings.TrimSpace(line[idx+1:])
		// Strip surrounding quotes if present.
		val = strings.Trim(val, `"'`)

		switch key {
		case "server_port":
			if port, err := strconv.Atoi(val); err == nil {
				cfg.ServerPort = port
			}
		case "sqlite_path":
			cfg.SQLitePath = val
		case "git_repo_url":
			cfg.GitRepoURL = val
		case "git_token_enc":
			cfg.GitTokenEnc = val
		case "encrypt_key":
			cfg.EncryptKey = val
		case "admin_pw_hash":
			cfg.AdminPwHash = val
		}
	}
}

// Addr returns the listen address (":port").
func (c Config) Addr() string {
	return ":" + strconv.Itoa(c.ServerPort)
}
