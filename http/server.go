package http

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"
)

// Server represents the HTTP server and holds all handler dependencies.
type Server struct {
	server *http.Server
	router *http.ServeMux
	logger *slog.Logger

	// Addr is the bind address for the server.
	Addr string

	// TODO: Add service dependencies as they are implemented.
	// ProjectService deploykit.ProjectService
	// ResourceService deploykit.ResourceService
}

// NewServer creates a new Server instance.
func NewServer(logger *slog.Logger) *Server {
	s := &Server{
		logger: logger,
		router: http.NewServeMux(),
	}

	s.registerRoutes()

	s.server = &http.Server{
		Handler: s.router,
	}

	return s
}

// Open starts listening on the configured address.
// It returns once the server is actively listening.
func (s *Server) Open() error {
	ln, err := net.Listen("tcp", s.Addr)
	if err != nil {
		return fmt.Errorf("listening on %s: %w", s.Addr, err)
	}

	s.logger.Info("starting http server", "addr", ln.Addr())

	go func() {
		if err := s.server.Serve(ln); err != nil && err != http.ErrServerClosed {
			s.logger.Error("http server error", "err", err)
		}
	}()

	return nil
}

// Close gracefully shuts down the server with a 10-second timeout.
func (s *Server) Close() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	s.logger.Info("shutting down http server")
	return s.server.Shutdown(ctx)
}

// registerRoutes sets up all HTTP routes.
func (s *Server) registerRoutes() {
	s.router.HandleFunc("GET /", s.handleIndex)
}

// handleIndex serves a basic health check response.
func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintln(w, `{"status":"ok"}`)
}
