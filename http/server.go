package http

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"

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
	CanvasService     deploykit.CanvasService
	SystemService     deploykit.SystemService

	// canvasHub manages WebSocket connections for canvas collaboration.
	canvasHub *canvasHub
}

// NewServer creates a new Server instance.
func NewServer(logger *slog.Logger) *Server {
	s := &Server{
		logger:       logger,
		router:       http.NewServeMux(),
		loginLimiter: newLoginRateLimiter(),
		canvasHub:    newCanvasHub(logger),
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

// Close gracefully shuts down the server. The caller controls the timeout
// and any escalation (e.g. force-cancel on a second signal) via ctx.
func (s *Server) Close(ctx context.Context) error {
	s.logger.Info("shutting down http server")
	return s.server.Shutdown(ctx)
}

// registerRoutes sets up all HTTP routes.
func (s *Server) registerRoutes() {
	// API sub-mux: all backend routes live under /api/.
	api := http.NewServeMux()

	// Public API routes (no authentication required).
	api.HandleFunc("GET /health", s.handleIndex)
	api.HandleFunc("GET /auth/register", s.handleCanRegister)
	api.HandleFunc("POST /auth/register", s.handleRegister)
	api.HandleFunc("POST /auth/login", s.handleLogin)
	api.HandleFunc("POST /auth/refresh", s.handleRefresh)
	api.HandleFunc("GET /projects/{projectId}/canvas/ws", s.handleCanvasWebSocket)

	// Protected API routes (authentication required).
	protected := http.NewServeMux()
	protected.HandleFunc("POST /auth/logout", s.handleLogout)
	protected.HandleFunc("GET /auth/me", s.handleGetCurrentUser)
	protected.HandleFunc("PATCH /auth/profile", s.handleUpdateProfile)

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

	adminOnly := s.requireRole(deploykit.RoleAdmin)
	protected.Handle("POST /users", adminOnly(http.HandlerFunc(s.handleCreateUser)))
	protected.Handle("GET /users", adminOnly(http.HandlerFunc(s.handleListUsers)))
	protected.Handle("GET /users/{id}", adminOnly(http.HandlerFunc(s.handleGetUser)))
	protected.Handle("PATCH /users/{id}", adminOnly(http.HandlerFunc(s.handleUpdateUser)))
	protected.Handle("DELETE /users/{id}", adminOnly(http.HandlerFunc(s.handleDeleteUser)))

	protected.Handle("GET /system/about", adminOnly(http.HandlerFunc(s.handleGetAbout)))
	protected.Handle("GET /system/status", adminOnly(http.HandlerFunc(s.handleGetSystemStatus)))

	protected.HandleFunc("GET /api-keys", s.handleListAPIKeys)
	protected.HandleFunc("POST /api-keys", s.handleCreateAPIKey)
	protected.HandleFunc("DELETE /api-keys/{id}", s.handleDeleteAPIKey)

	api.Handle("/", s.authenticate(protected))

	// Mount API under /api/ prefix and SPA catch-all at root.
	s.router.Handle("/api/", http.StripPrefix("/api", api))
	s.router.Handle("/", s.spaHandler())
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
	case deploykit.EFORBIDDEN:
		status = http.StatusForbidden
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
