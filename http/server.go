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

// Triggerable is implemented by components that support on-demand triggering.
type Triggerable interface {
	Trigger()
}

// Server represents the HTTP server and holds all handler dependencies.
type Server struct {
	server *http.Server
	router *http.ServeMux
	logger *slog.Logger

	// Addr is the bind address for the server.
	Addr string

	// CORSOrigin is the allowed origin for CORS requests.
	CORSOrigin string

	// loginLimiter rate-limits login attempts per email.
	loginLimiter *loginRateLimiter

	// Reconciler triggers infrastructure reconciliation after state changes.
	Reconciler Triggerable

	// Service dependencies.
	ProjectService    deploykit.ProjectService
	UserService       deploykit.UserService
	AuthService       deploykit.AuthService
	ServiceService    deploykit.ServiceService
	DeploymentService deploykit.DeploymentService
	ContainerService  deploykit.ContainerService
}

// NewServer creates a new Server instance.
func NewServer(logger *slog.Logger) *Server {
	s := &Server{
		logger:       logger,
		router:       http.NewServeMux(),
		loginLimiter: newLoginRateLimiter(),
		CORSOrigin:   "*",
	}

	s.registerRoutes()

	s.server = &http.Server{
		Handler: s.cors(s.router),
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
	// Public routes (no authentication required).
	s.router.HandleFunc("GET /{$}", s.handleIndex)
	s.router.HandleFunc("GET /auth/register", s.handleCanRegister)
	s.router.HandleFunc("POST /auth/register", s.handleRegister)
	s.router.HandleFunc("POST /auth/login", s.handleLogin)
	s.router.HandleFunc("POST /auth/refresh", s.handleRefresh)

	// Protected routes (authentication required).
	protected := http.NewServeMux()
	protected.HandleFunc("POST /auth/logout", s.handleLogout)
	protected.HandleFunc("GET /auth/me", s.handleGetCurrentUser)

	protected.HandleFunc("POST /projects", s.handleCreateProject)
	protected.HandleFunc("GET /projects", s.handleListProjects)
	protected.HandleFunc("GET /projects/{projectId}", s.handleGetProject)
	protected.HandleFunc("PATCH /projects/{projectId}", s.handleUpdateProject)
	protected.HandleFunc("DELETE /projects/{projectId}", s.handleDeleteProject)

	protected.HandleFunc("POST /projects/{projectId}/services", s.handleCreateService)
	protected.HandleFunc("GET /projects/{projectId}/services", s.handleListServices)
	protected.HandleFunc("GET /projects/{projectId}/services/{serviceId}", s.handleGetService)
	protected.HandleFunc("PATCH /projects/{projectId}/services/{serviceId}", s.handleUpdateService)
	protected.HandleFunc("DELETE /projects/{projectId}/services/{serviceId}", s.handleDeleteService)

	protected.HandleFunc("POST /projects/{projectId}/services/{serviceId}/deployments", s.handleCreateDeployment)
	protected.HandleFunc("GET /projects/{projectId}/services/{serviceId}/deployments", s.handleListDeployments)
	protected.HandleFunc("GET /projects/{projectId}/services/{serviceId}/deployments/{deploymentId}", s.handleGetDeployment)
	protected.HandleFunc("POST /projects/{projectId}/services/{serviceId}/rollback", s.handleRollbackService)

	protected.HandleFunc("GET /projects/{projectId}/services/{serviceId}/containers", s.handleListContainers)

	protected.HandleFunc("POST /users", s.handleCreateUser)
	protected.HandleFunc("GET /users", s.handleListUsers)
	protected.HandleFunc("GET /users/{id}", s.handleGetUser)
	protected.HandleFunc("PATCH /users/{id}", s.handleUpdateUser)
	protected.HandleFunc("DELETE /users/{id}", s.handleDeleteUser)

	protected.HandleFunc("GET /api-keys", s.handleListAPIKeys)
	protected.HandleFunc("POST /api-keys", s.handleCreateAPIKey)
	protected.HandleFunc("DELETE /api-keys/{id}", s.handleDeleteAPIKey)

	s.router.Handle("/", s.authenticate(protected))
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
	// Handle structured validation errors with 422 status.
	if ve, ok := err.(*deploykit.ValidationError); ok {
		jsonResponse(w, http.StatusUnprocessableEntity, ve)
		return
	}

	code := deploykit.ErrorCode(err)
	message := deploykit.ErrorMessage(err)

	status := http.StatusInternalServerError
	switch code {
	case deploykit.EINVALID:
		status = http.StatusBadRequest
	case deploykit.EUNAUTHORIZED:
		status = http.StatusUnauthorized
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
