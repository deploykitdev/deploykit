package http

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/heyjorgedev/deploykit"
)

// Server represents the HTTP server and holds all handler dependencies.
type Server struct {
	server *http.Server
	router *http.ServeMux
	logger *slog.Logger

	// Addr is the bind address for the server.
	Addr string

	// Service dependencies.
	ProjectService deploykit.ProjectService
	UserService    deploykit.UserService
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

	s.router.HandleFunc("POST /projects", s.handleCreateProject)
	s.router.HandleFunc("GET /projects", s.handleListProjects)
	s.router.HandleFunc("GET /projects/{id}", s.handleGetProject)
	s.router.HandleFunc("PATCH /projects/{id}", s.handleUpdateProject)
	s.router.HandleFunc("DELETE /projects/{id}", s.handleDeleteProject)

	s.router.HandleFunc("POST /users", s.handleCreateUser)
	s.router.HandleFunc("GET /users", s.handleListUsers)
	s.router.HandleFunc("GET /users/{id}", s.handleGetUser)
	s.router.HandleFunc("PATCH /users/{id}", s.handleUpdateUser)
	s.router.HandleFunc("DELETE /users/{id}", s.handleDeleteUser)
}

// handleIndex serves a basic health check response.
func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	jsonResponse(w, http.StatusOK, map[string]string{"status": "ok"})
}

// jsonResponse writes a JSON response with the given status code.
func jsonResponse(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// errorResponse writes a JSON error response, mapping domain error codes
// to appropriate HTTP status codes.
func (s *Server) errorResponse(w http.ResponseWriter, r *http.Request, err error) {
	code := deploykit.ErrorCode(err)
	message := deploykit.ErrorMessage(err)

	status := http.StatusInternalServerError
	switch code {
	case deploykit.EINVALID:
		status = http.StatusBadRequest
	case deploykit.ENOTFOUND:
		status = http.StatusNotFound
	case deploykit.ECONFLICT:
		status = http.StatusConflict
	}

	if status >= 500 {
		s.logger.Error("internal error", "err", err, "method", r.Method, "path", r.URL.Path)
	}

	jsonResponse(w, status, deploykit.Error{
		Code:    code,
		Message: message,
	})
}
