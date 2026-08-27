// Package git -- go-git client implementation (workflow E).
//
// GoGitClient implements the GitClient interface using
// github.com/go-git/go-git/v5.  It clones (or re-opens) a remote repository
// into a local working directory, writes the given files, stages them,
// creates a single commit, and pushes back to the remote.
package git

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
)

// GoGitClient implements GitClient using github.com/go-git/go-git/v5.
//
// repoURL is the remote repository URL, token is an optional access token
// for HTTP basic auth, and workDir is the local directory used for cloning.
type GoGitClient struct {
	repoURL string
	token   string
	workDir string
}

// NewGoGitClient returns a GoGitClient configured with the given repo URL,
// auth token, and local working directory for cloning.
func NewGoGitClient(repoURL, token, workDir string) *GoGitClient {
	return &GoGitClient{
		repoURL: repoURL,
		token:   token,
		workDir: workDir,
	}
}

// InitRepo implements GitClient.  It tries to clone the remote repository.
// If the remote is empty (no commits yet), it initialises a fresh local
// repository, writes a README.md, sets the origin remote, and pushes the
// initial commit so that subsequent CommitAndPush calls work normally.
func (c *GoGitClient) InitRepo(ctx context.Context) error {
	var auth *http.BasicAuth
	if c.token != "" {
		auth = &http.BasicAuth{
			Username: "x-access-token",
			Password: c.token,
		}
	}

	// Clean any stale working directory first so we start fresh.
	if err := os.RemoveAll(c.workDir); err != nil {
		return fmt.Errorf("clean work dir: %w", err)
	}
	if err := os.MkdirAll(c.workDir, 0755); err != nil {
		return fmt.Errorf("create work dir: %w", err)
	}

	// Try a plain clone first.
	_, err := git.PlainClone(c.workDir, false, &git.CloneOptions{
		URL:  c.repoURL,
		Auth: auth,
	})
	if err == nil {
		// Clone succeeded — repo already has content.
		return nil
	}

	// If the remote is empty, initialise a new repo and push an initial
	// commit.  go-git returns a transport error containing "empty
	// repository" for a freshly-created remote.
	if !isRemoteEmptyErr(err) {
		return fmt.Errorf("git clone: %w", err)
	}

	// --- Remote is empty: init locally and push initial commit --- //
	if err := c.initEmptyRepo(auth); err != nil {
		return err
	}

	return nil
}

// isRemoteEmptyErr returns true if the error indicates the remote repository
// is empty (has no commits).
func isRemoteEmptyErr(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "remote repository is empty") ||
		strings.Contains(msg, "empty remote") ||
		strings.Contains(msg, "repository not found")
}

// initEmptyRepo initialises a fresh local repository, writes a README.md,
// adds the origin remote, creates an initial commit, and pushes it to the
// remote.  The workDir must already exist.
func (c *GoGitClient) initEmptyRepo(auth *http.BasicAuth) error {
	repo, err := git.PlainInit(c.workDir, false)
	if err != nil {
		return fmt.Errorf("git init: %w", err)
	}

	if _, err := repo.CreateRemote(&config.RemoteConfig{
		Name: "origin",
		URLs: []string{c.repoURL},
	}); err != nil {
		return fmt.Errorf("git remote add origin: %w", err)
	}

	readmePath := filepath.Join(c.workDir, "README.md")
	readmeContent := "# SQL Manager\n\nSQL script repository managed by SQL Manager platform.\n\n## Structure\n\n```\n{project_code}/\n  pending/     # scripts not yet released\n  archived/    # scripts that have been archived\n```\n"
	if err := os.WriteFile(readmePath, []byte(readmeContent), 0644); err != nil {
		return fmt.Errorf("write README: %w", err)
	}

	w, err := repo.Worktree()
	if err != nil {
		return fmt.Errorf("git worktree: %w", err)
	}
	if _, err := w.Add("README.md"); err != nil {
		return fmt.Errorf("git add README: %w", err)
	}
	if _, err := w.Commit("Initial commit", &git.CommitOptions{
		Author: &object.Signature{
			Name:  "SQL Manager",
			Email: "sql-mgr@localhost",
			When:  time.Now(),
		},
	}); err != nil {
		return fmt.Errorf("git commit: %w", err)
	}

	// Push to origin; try master first, fall back to main.
	if err := repo.Push(&git.PushOptions{
		Auth:     auth,
		RefSpecs: []config.RefSpec{config.RefSpec("refs/heads/master:refs/heads/master")},
	}); err != nil {
		if err2 := repo.Push(&git.PushOptions{
			Auth:     auth,
			RefSpecs: []config.RefSpec{config.RefSpec("refs/heads/master:refs/heads/main")},
		}); err2 != nil {
			return fmt.Errorf("git push initial commit (master): %w; (main): %v", err, err2)
		}
	}
	return nil
}

// CommitAndPush implements the GitClient interface.
//
// It clones or opens the repository at the configured workDir, writes each
// file in the files map (relative path → content), stages all changes,
// commits with the given message, pushes to the remote, and returns the
// resulting commit hash.
func (c *GoGitClient) CommitAndPush(ctx context.Context, files map[string]string, msg string) (string, error) {
	// Build HTTP basic auth when a token is configured.
	var auth *http.BasicAuth
	if c.token != "" {
		auth = &http.BasicAuth{
			Username: "x-access-token",
			Password: c.token,
		}
	}

	// 1. Clone or open the repository.
	repo, err := git.PlainOpen(c.workDir)
	if err != nil {
		// Repository does not exist locally — clone it.
		if mkErr := os.MkdirAll(filepath.Dir(c.workDir), 0755); mkErr != nil {
			return "", fmt.Errorf("create work dir parent: %w", mkErr)
		}
		repo, err = git.PlainClone(c.workDir, false, &git.CloneOptions{
			URL:  c.repoURL,
			Auth: auth,
		})
		if err != nil {
			// If the remote is empty, initialise locally first.
			if isRemoteEmptyErr(err) {
				if initErr := c.initEmptyRepo(auth); initErr != nil {
					return "", fmt.Errorf("init empty repo: %w", initErr)
				}
				repo, err = git.PlainOpen(c.workDir)
				if err != nil {
					return "", fmt.Errorf("reopen after init: %w", err)
				}
			} else {
				return "", fmt.Errorf("git clone: %w", err)
			}
		}
	}

	// 2. Write each file to the working directory.
	for relPath, content := range files {
		fullPath := filepath.Join(c.workDir, relPath)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
			return "", fmt.Errorf("create dir for %s: %w", relPath, err)
		}
		if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
			return "", fmt.Errorf("write file %s: %w", relPath, err)
		}
	}

	// 3. Stage all changes (git add .).
	w, err := repo.Worktree()
	if err != nil {
		return "", fmt.Errorf("git worktree: %w", err)
	}
	if _, err := w.Add("."); err != nil {
		return "", fmt.Errorf("git add: %w", err)
	}

	// 4. Commit with the given message.
	hash, err := w.Commit(msg, &git.CommitOptions{
		Author: &object.Signature{
			Name:  "SQL Manager",
			Email: "sql-mgr@localhost",
			When:  time.Now(),
		},
	})
	if err != nil {
		return "", fmt.Errorf("git commit: %w", err)
	}

	// 5. Push to the remote.
	if err := repo.Push(&git.PushOptions{
		Auth: auth,
	}); err != nil {
		// "already up-to-date" is not a real error.
		if errors.Is(err, git.NoErrAlreadyUpToDate) {
			// fall through
		} else {
			return "", fmt.Errorf("git push: %w", err)
		}
	}

	// 6. Return the commit hash.
	return hash.String(), nil
}
