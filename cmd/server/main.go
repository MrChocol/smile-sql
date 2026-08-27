// Command server is the single binary entry point for the SQL management
// platform.  It loads configuration, opens the SQLite metadata database,
// runs migrations, ensures a default admin user exists, wires dependencies,
// and starts the HTTP server.
package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"sql-mgr/internal/archive"
	"sql-mgr/internal/config"
	"sql-mgr/internal/crypto"
	dbpkg "sql-mgr/internal/db"
	"sql-mgr/internal/executor"
	"sql-mgr/internal/git"
	"sql-mgr/internal/repo"
	"sql-mgr/internal/web"
)

func main() {
	// 1. Load configuration.
	cfg := config.Load("config.yaml")
	log.Printf("config loaded: sqlite=%s port=%d", cfg.SQLitePath, cfg.ServerPort)

	// 2. Open and migrate the metadata database.
	store, err := dbpkg.Open(cfg.SQLitePath)
	if err != nil {
		log.Fatalf("db open: %v", err)
	}
	defer store.Close()

	if err := dbpkg.Migrate(store); err != nil {
		log.Fatalf("db migrate: %v", err)
	}
	log.Println("database migrated")

	// 3. Ensure the default admin user exists.
	if err := web.EnsureAdmin(store); err != nil {
		log.Fatalf("ensure admin: %v", err)
	}
	log.Println("admin user ensured")

	// 3.5 Override Git config from settings table if present.
	//    Settings saved via the web UI take precedence over config.yaml.
	settingsRepo := repo.NewSettingsRepo(store)
	if settings, err := settingsRepo.GetSettings(); err != nil {
		log.Printf("warning: failed to read settings from db: %v", err)
	} else if settings.GitRepoURL != "" {
		cfg.GitRepoURL = settings.GitRepoURL
		cfg.GitTokenEnc = settings.GitTokenEnc
		log.Println("git: using configuration from settings table")
	} else {
		log.Println("git: using configuration from config.yaml (settings table empty)")
	}

	// 4. Construct dependencies.
	//    Crypto: use AES when admin password hash is configured, otherwise
	//    fall back to Stub for development.
	var cryptoImpl crypto.Crypto
	if cfg.AdminPwHash != "" {
		aes, err := crypto.NewAES(cfg.EncryptKey, cfg.AdminPwHash)
		if err != nil {
			log.Fatalf("init crypto: %v", err)
		}
		cryptoImpl = aes
		log.Println("crypto: AES-256-GCM enabled")
	} else {
		cryptoImpl = &crypto.Stub{}
		log.Println("crypto: Stub (no admin password configured)")
	}

	//    Executor: always wire the MySQL executor.
	exec := executor.NewMySQLExecutor(cryptoImpl)

	//    Git + Archiver: only when Git repo URL is configured.
	var gitClient git.GitClient
	var archiver archive.Archiver
	if cfg.GitRepoURL != "" {
		workDir := filepath.Join(os.TempDir(), "sql-mgr-git")
		gitClient = git.NewGoGitClient(cfg.GitRepoURL, cfg.GitTokenEnc, workDir)
		archiver = archive.NewArchiver(store, gitClient)
		log.Printf("git: %s (workdir=%s)", cfg.GitRepoURL, workDir)
	} else {
		log.Println("git: not configured (archive disabled)")
	}

	deps := &web.Deps{
		DB:       store,
		Crypto:   cryptoImpl,
		Executor: exec,
		Git:      gitClient,
		Archiver: archiver,
	}

	// 5. Build the HTTP server and register all module routes.
	srv := web.New(deps)
	mux := srv.Routes()
	srv.RegisterDashboard(mux)
	srv.RegisterConfig(mux)
	srv.RegisterScriptRelease(mux)
	srv.RegisterReveal(mux)
	srv.RegisterExecution(mux)
	srv.RegisterArchive(mux)
	srv.RegisterSettings(mux)
	srv.RegisterCleanup(mux)

	httpSrv := &http.Server{
		Addr:         cfg.Addr(),
		Handler:      mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// 6. Start serving with graceful shutdown.
	go func() {
		log.Printf("listening on %s", httpSrv.Addr)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	log.Println("shutting down...")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := httpSrv.Shutdown(ctx); err != nil {
		log.Printf("shutdown error: %v", err)
	}
	log.Println("server stopped")
}
