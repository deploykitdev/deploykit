package http

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/heyjorgedev/deploykit"
	"golang.org/x/crypto/bcrypt"
)

// loginRateLimiter tracks login attempts per email to prevent brute force.
type loginRateLimiter struct {
	mu       sync.Mutex
	attempts map[string][]time.Time
}

func newLoginRateLimiter() *loginRateLimiter {
	return &loginRateLimiter{
		attempts: make(map[string][]time.Time),
	}
}

const (
	loginRateWindow  = 15 * time.Minute
	loginRateMaxHits = 5
)

// allow checks if a login attempt for the given email is allowed.
func (l *loginRateLimiter) allow(email string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-loginRateWindow)

	// Prune old attempts.
	attempts := l.attempts[email]
	valid := attempts[:0]
	for _, t := range attempts {
		if t.After(cutoff) {
			valid = append(valid, t)
		}
	}
	l.attempts[email] = valid

	return len(valid) < loginRateMaxHits
}

// record records a failed login attempt for the given email.
func (l *loginRateLimiter) record(email string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.attempts[email] = append(l.attempts[email], time.Now())
}

// clear removes all recorded attempts for the given email.
func (l *loginRateLimiter) clear(email string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.attempts, email)
}

func (s *Server) handleCanRegister(w http.ResponseWriter, r *http.Request) {
	canRegister, err := s.AuthService.CanRegister(r.Context())
	if err != nil {
		s.errorResponse(w, r, err)
		return
	}

	jsonResponse(w, http.StatusOK, map[string]bool{"can_register": canRegister})
}

func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	canRegister, err := s.AuthService.CanRegister(r.Context())
	if err != nil {
		s.errorResponse(w, r, err)
		return
	}
	if !canRegister {
		s.errorResponse(w, r, deploykit.Errorf(deploykit.ECONFLICT, "Registration is closed."))
		return
	}

	var req deploykit.UserCreate
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.errorResponse(w, r, deploykit.Errorf(deploykit.EINVALID, "Invalid JSON body."))
		return
	}

	req.Role = deploykit.RoleAdmin

	user, err := s.UserService.CreateUser(r.Context(), req)
	if err != nil {
		s.errorResponse(w, r, err)
		return
	}

	// Auto-login after registration.
	tokens, err := s.AuthService.Login(r.Context(), deploykit.LoginRequest{
		Email:    req.Email,
		Password: req.Password,
	})
	if err != nil {
		s.errorResponse(w, r, err)
		return
	}

	jsonResponse(w, http.StatusCreated, map[string]any{
		"user":   user,
		"tokens": tokens,
	})
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req deploykit.LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.errorResponse(w, r, deploykit.Errorf(deploykit.EINVALID, "Invalid JSON body."))
		return
	}

	if !s.loginLimiter.allow(req.Email) {
		s.errorResponse(w, r, deploykit.Errorf(deploykit.EINVALID, "Too many login attempts. Try again later."))
		return
	}

	tokens, err := s.AuthService.Login(r.Context(), req)
	if err != nil {
		s.loginLimiter.record(req.Email)
		s.errorResponse(w, r, err)
		return
	}

	s.loginLimiter.clear(req.Email)
	jsonResponse(w, http.StatusOK, tokens)
}

func (s *Server) handleRefresh(w http.ResponseWriter, r *http.Request) {
	var req struct {
		RefreshToken string `json:"refresh_token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.errorResponse(w, r, deploykit.Errorf(deploykit.EINVALID, "Invalid JSON body."))
		return
	}

	tokens, err := s.AuthService.RefreshSession(r.Context(), req.RefreshToken)
	if err != nil {
		s.errorResponse(w, r, err)
		return
	}

	jsonResponse(w, http.StatusOK, tokens)
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())

	if err := s.AuthService.LogoutAll(r.Context(), user.ID); err != nil {
		s.errorResponse(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleGetCurrentUser(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())
	jsonResponse(w, http.StatusOK, user)
}

func (s *Server) handleUpdateProfile(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())

	var req deploykit.ProfileUpdate
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.errorResponse(w, r, deploykit.Errorf(deploykit.EINVALID, "Invalid JSON body."))
		return
	}

	if err := req.Validate(); err != nil {
		s.errorResponse(w, r, err)
		return
	}

	// Re-fetch user to get fresh password hash (context user may be stale).
	fresh, err := s.UserService.GetUser(r.Context(), user.ID)
	if err != nil {
		s.errorResponse(w, r, err)
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(fresh.PasswordHash), []byte(req.CurrentPassword)); err != nil {
		ve := deploykit.NewValidationErrors()
		ve.Add("current_password", "Current password is incorrect.")
		s.errorResponse(w, r, ve.Err())
		return
	}

	update := deploykit.UserUpdate{
		Email:    req.Email,
		Name:     req.Name,
		Password: req.NewPassword,
	}

	updated, err := s.UserService.UpdateUser(r.Context(), user.ID, update)
	if err != nil {
		s.errorResponse(w, r, err)
		return
	}

	jsonResponse(w, http.StatusOK, updated)
}

func (s *Server) handleCreateAPIKey(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())

	var req deploykit.APIKeyCreate
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.errorResponse(w, r, deploykit.Errorf(deploykit.EINVALID, "Invalid JSON body."))
		return
	}

	key, err := s.AuthService.CreateAPIKey(r.Context(), user.ID, req)
	if err != nil {
		s.errorResponse(w, r, err)
		return
	}

	jsonResponse(w, http.StatusCreated, key)
}

func (s *Server) handleListAPIKeys(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())

	keys, err := s.AuthService.ListAPIKeys(r.Context(), user.ID)
	if err != nil {
		s.errorResponse(w, r, err)
		return
	}

	if keys == nil {
		keys = make([]*deploykit.APIKey, 0)
	}

	jsonResponse(w, http.StatusOK, map[string]any{"data": keys})
}

func (s *Server) handleDeleteAPIKey(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	if err := s.AuthService.DeleteAPIKey(r.Context(), id); err != nil {
		s.errorResponse(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
