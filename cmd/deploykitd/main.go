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

	dkhttp "github.com/heyjorgedev/deploykit/http"
	"github.com/heyjorgedev/deploykit/sqlite"
)

// Config represents the application configuration.
type Config struct {
	Addr       string
	DBPath     string
	LogLevel   string
	CORSOrigin string
}

// DefaultConfig returns the default configuration.
func DefaultConfig() Config {
	return Config{
		Addr:       ":8080",
		DBPath:     "deploykit.db",
		LogLevel:   "info",
		CORSOrigin: "*",
	}
}

// Main represents the application and owns all long-lived resources.
type Main struct {
	Config Config
	Logger *slog.Logger

	DB         *sqlite.DB
	HTTPServer *dkhttp.Server

	// TODO: Add when package is implemented.
	// DockerClient *docker.Client
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
	// Initialize SQLite database.
	m.DB = sqlite.NewDB(m.Config.DBPath, m.Logger)
	if err := m.DB.Open(); err != nil {
		return fmt.Errorf("opening database: %w", err)
	}

	// TODO: Initialize Docker client.
	// m.DockerClient = docker.NewClient()
	// if err := m.DockerClient.Open(); err != nil {
	//     return fmt.Errorf("opening docker client: %w", err)
	// }

	// Initialize services.
	projectService := sqlite.NewProjectService(m.DB)
	userService := sqlite.NewUserService(m.DB)
	authService := sqlite.NewAuthService(m.DB)

	// Initialize HTTP server.
	m.HTTPServer = dkhttp.NewServer(m.Logger)
	m.HTTPServer.Addr = m.Config.Addr
	m.HTTPServer.CORSOrigin = m.Config.CORSOrigin
	m.HTTPServer.ProjectService = projectService
	m.HTTPServer.UserService = userService
	m.HTTPServer.AuthService = authService

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
				if err := authService.CleanExpiredSessions(context.Background()); err != nil {
					m.Logger.Error("cleaning expired sessions", "err", err)
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	// Wait for context cancellation.
	<-ctx.Done()

	// Graceful shutdown.
	if err := m.HTTPServer.Close(); err != nil {
		return fmt.Errorf("shutting down http server: %w", err)
	}

	return nil
}

// Close tears down all resources in reverse initialization order.
func (m *Main) Close() error {
	// TODO: Close Docker client.
	// if m.DockerClient != nil {
	//     m.DockerClient.Close()
	// }

	if m.DB != nil {
		m.DB.Close()
	}

	return nil
}

func main() {
	cfg := DefaultConfig()

	flag.StringVar(&cfg.Addr, "addr", cfg.Addr, "HTTP listen address")
	flag.StringVar(&cfg.DBPath, "db", cfg.DBPath, "SQLite database path")
	flag.StringVar(&cfg.LogLevel, "log-level", cfg.LogLevel, "Log level (debug, info, warn, error)")
	flag.StringVar(&cfg.CORSOrigin, "cors-origin", cfg.CORSOrigin, "Allowed CORS origin")
	flag.Parse()

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
