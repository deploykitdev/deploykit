package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/heyjorgedev/deploykit/docker"
	"github.com/heyjorgedev/deploykit/events"
	dkhttp "github.com/heyjorgedev/deploykit/http"
	"github.com/heyjorgedev/deploykit/presets"
	"github.com/heyjorgedev/deploykit/reconciler"
	"github.com/heyjorgedev/deploykit/sqlite"
	"github.com/heyjorgedev/deploykit/sysinfo"
)

// Build metadata, populated by goreleaser via -ldflags. Defaults are used
// for local `go build` / `go run` invocations.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

// Config represents the application configuration.
type Config struct {
	Addr              string
	DBPath            string
	DataDir           string
	LogLevel          string
	CORSOrigin        string
	GitHubRepo        string
	ReconcileInterval time.Duration
}

// DefaultConfig returns the default configuration. DataDir falls back to
// $DEPLOYKIT_DATA_DIR (set by the systemd unit) or the directory of DBPath.
func DefaultConfig() Config {
	return Config{
		Addr:              ":8080",
		DBPath:            "deploykit.db",
		DataDir:           os.Getenv("DEPLOYKIT_DATA_DIR"),
		LogLevel:          "info",
		CORSOrigin:        "*",
		GitHubRepo:        "deploykitdev/deploykit",
		ReconcileInterval: 30 * time.Second,
	}
}

// Main represents the application and owns all long-lived resources.
type Main struct {
	Config Config
	Logger *slog.Logger

	DB           *sqlite.DB
	HTTPServer   *dkhttp.Server
	DockerClient *docker.Client
}

// NewMain creates a new Main instance with the given config and logger.
func NewMain(cfg Config, logger *slog.Logger) *Main {
	return &Main{
		Config: cfg,
		Logger: logger,
	}
}

// Run starts the application and blocks until the context is cancelled.
func (m *Main) Run(ctx context.Context) error {
	startedAt := time.Now()

	// Initialize SQLite database.
	m.DB = sqlite.NewDB(m.Config.DBPath, m.Logger)
	if err := m.DB.Open(); err != nil {
		return fmt.Errorf("opening database: %w", err)
	}

	// Initialize Docker client.
	m.DockerClient = docker.NewClient(m.Logger)
	if err := m.DockerClient.Open(); err != nil {
		return fmt.Errorf("opening docker client: %w", err)
	}
	if err := m.DockerClient.Ping(ctx); err != nil {
		return fmt.Errorf("connecting to docker daemon: %w", err)
	}

	// Initialize services.
	projectService := sqlite.NewProjectService(m.DB)
	userService := sqlite.NewUserService(m.DB)
	authService := sqlite.NewAuthService(m.DB)
	serviceService := sqlite.NewServiceService(m.DB)
	deploymentService := sqlite.NewDeploymentService(m.DB)
	containerService := sqlite.NewContainerService(m.DB)
	canvasService := sqlite.NewCanvasService(m.DB)
	envVarService := sqlite.NewEnvVarService(m.DB)
	pendingChangeService := sqlite.NewPendingChangeService(m.DB)
	systemSettingsStore := sqlite.NewSystemSettingsStore(m.DB)
	systemService := sysinfo.New(sysinfo.Config{
		Docker:     m.DockerClient,
		Logger:     m.Logger,
		DBPath:     m.Config.DBPath,
		Version:    version,
		StartedAt:  startedAt,
		DataDir:    m.Config.DataDir,
		GitHubRepo: m.Config.GitHubRepo,
		Settings:   systemSettingsStore,
		Services:   serviceService,
	})
	presetService, err := presets.New()
	if err != nil {
		return fmt.Errorf("loading presets: %w", err)
	}

	// Initialize event bus (in-process pub/sub).
	bus := events.NewBus(m.Logger)

	// Initialize reconciler.
	rec := reconciler.New(projectService, serviceService, deploymentService, containerService, m.DockerClient, m.Logger, m.Config.ReconcileInterval, bus)
	go rec.Run(ctx)

	// Initialize HTTP server.
	m.HTTPServer = dkhttp.NewServer(m.Logger)
	m.HTTPServer.Addr = m.Config.Addr
	m.HTTPServer.CORSOrigin = m.Config.CORSOrigin
	m.HTTPServer.Reconciler = rec
	m.HTTPServer.EventBus = bus
	m.HTTPServer.ProjectService = projectService
	m.HTTPServer.UserService = userService
	m.HTTPServer.AuthService = authService
	m.HTTPServer.ServiceService = serviceService
	m.HTTPServer.DeploymentService = deploymentService
	m.HTTPServer.ContainerService = containerService
	m.HTTPServer.CanvasService = canvasService
	m.HTTPServer.SystemService = systemService
	m.HTTPServer.EnvVarService = envVarService
	m.HTTPServer.PendingChangeService = pendingChangeService
	m.HTTPServer.PresetService = presetService
	m.HTTPServer.LogStreamer = m.DockerClient

	if err := m.HTTPServer.Open(); err != nil {
		return fmt.Errorf("starting http server: %w", err)
	}

	// Periodically clean expired sessions.
	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if err := authService.CleanExpiredSessions(ctx); err != nil {
					m.Logger.Error("cleaning expired sessions", "err", err)
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	// Poll the upstream release feed once at startup (after a short delay
	// so we don't slow boot) and once a day thereafter. Failures are
	// logged at debug level — release info is best-effort.
	go func() {
		select {
		case <-time.After(30 * time.Second):
		case <-ctx.Done():
			return
		}
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()
		for {
			if _, err := systemService.RefreshLatestRelease(ctx); err != nil {
				m.Logger.Debug("refreshing latest release", "err", err)
			} else if settings, err := systemService.GetSettings(ctx); err == nil && settings.AutoUpdate {
				if release, err := systemService.LatestRelease(ctx); err == nil && release != nil {
					if err := systemService.RequestUpgrade(ctx, release.Version); err != nil {
						m.Logger.Debug("auto-update skipped", "err", err)
					} else {
						m.Logger.Info("auto-update queued", "version", release.Version)
					}
				}
			}
			select {
			case <-ticker.C:
			case <-ctx.Done():
				return
			}
		}
	}()

	// Wait for context cancellation.
	<-ctx.Done()

	// Graceful shutdown with a 10-second budget. A second SIGINT/SIGTERM
	// cancels shutdownCtx and forces immediate close.
	shutdownCtx, stopShutdown := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stopShutdown()
	shutdownCtx, cancelShutdown := context.WithTimeout(shutdownCtx, 10*time.Second)
	defer cancelShutdown()

	if err := m.HTTPServer.Close(shutdownCtx); err != nil {
		return fmt.Errorf("shutting down http server: %w", err)
	}

	return nil
}

// Close tears down all resources in reverse initialization order.
func (m *Main) Close() error {
	if m.DockerClient != nil {
		m.DockerClient.Close()
	}

	if m.DB != nil {
		m.DB.Close()
	}

	return nil
}

func main() {
	cfg := DefaultConfig()

	var showVersion bool
	flag.BoolVar(&showVersion, "version", false, "Print version and exit")
	flag.StringVar(&cfg.Addr, "addr", cfg.Addr, "HTTP listen address")
	flag.StringVar(&cfg.DBPath, "db", cfg.DBPath, "SQLite database path")
	flag.StringVar(&cfg.DataDir, "data-dir", cfg.DataDir, "Directory for upgrade trigger and status files (defaults to dirname of -db)")
	flag.StringVar(&cfg.LogLevel, "log-level", cfg.LogLevel, "Log level (debug, info, warn, error)")
	flag.StringVar(&cfg.CORSOrigin, "cors-origin", cfg.CORSOrigin, "Allowed CORS origin")
	flag.StringVar(&cfg.GitHubRepo, "github-repo", cfg.GitHubRepo, "owner/repo to poll for upstream releases")
	flag.DurationVar(&cfg.ReconcileInterval, "reconcile-interval", cfg.ReconcileInterval, "Interval between reconciliation cycles")
	flag.Parse()

	if showVersion {
		fmt.Printf("deploykitd %s (commit %s, built %s)\n", version, commit, date)
		return
	}

	// Parse log level.
	var level slog.Level
	if err := level.UnmarshalText([]byte(cfg.LogLevel)); err != nil {
		fmt.Fprintf(os.Stderr, "invalid log level %q: %v\n", cfg.LogLevel, err)
		os.Exit(1)
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: level,
	}))

	m := NewMain(cfg, logger)
	defer m.Close()

	// Listen for interrupt signals.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := m.Run(ctx); err != nil {
		logger.Error("application error", "err", err)
		os.Exit(1)
	}

	logger.Info("shutdown complete")
}
