package deploykit

import (
	"context"
	"time"
)

// Session represents an active authentication session with access and refresh tokens.
type Session struct {
	ID               string    `json:"id"`
	UserID           string    `json:"user_id"`
	AccessTokenHash  string    `json:"-"`
	RefreshTokenHash string    `json:"-"`
	ExpiresAt        time.Time `json:"expires_at"`
	RefreshExpiresAt time.Time `json:"refresh_expires_at"`
	CreatedAt        time.Time `json:"created_at"`
}

// APIKey represents a long-lived API key for programmatic access.
type APIKey struct {
	ID          string     `json:"id"`
	UserID      string     `json:"user_id"`
	Description string     `json:"description"`
	Prefix      string     `json:"prefix"`
	TokenHash   string     `json:"-"`
	ExpiresAt   *time.Time `json:"expires_at"`
	LastUsedAt  *time.Time `json:"last_used_at"`
	CreatedAt   time.Time  `json:"created_at"`
}

// AuthTokens is returned on login and refresh. The plaintext tokens are
// ephemeral — they are returned to the caller but never stored.
type AuthTokens struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	ExpiresAt    time.Time `json:"expires_at"`
}

// LoginRequest holds credentials for authentication.
type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// Validate checks that all required fields are present.
func (r *LoginRequest) Validate() error {
	ve := NewValidationErrors()
	if r.Email == "" {
		ve.Add("email", "Email is required.")
	}
	if r.Password == "" {
		ve.Add("password", "Password is required.")
	}
	return ve.Err()
}

// APIKeyCreate holds fields for creating an API key.
type APIKeyCreate struct {
	Description string     `json:"description"`
	ExpiresAt   *time.Time `json:"expires_at"`
}

// Validate checks that all required fields are present.
func (c *APIKeyCreate) Validate() error {
	ve := NewValidationErrors()
	if c.Description == "" {
		ve.Add("description", "Description is required.")
	}
	return ve.Err()
}

// APIKeyCreated is returned once at creation time, including the plaintext token.
type APIKeyCreated struct {
	APIKey
	Token string `json:"token"`
}

// AuthService manages authentication sessions and API keys.
type AuthService interface {
	// Login authenticates a user by email and password and creates a new session.
	// Returns EUNAUTHORIZED if credentials are invalid.
	Login(ctx context.Context, req LoginRequest) (*AuthTokens, error)

	// RefreshSession exchanges a valid refresh token for a new token pair.
	// The old tokens are invalidated (refresh token rotation).
	// Returns EUNAUTHORIZED if the refresh token is invalid or expired.
	RefreshSession(ctx context.Context, refreshToken string) (*AuthTokens, error)

	// ValidateAccessToken validates an access token and returns the associated user.
	// Returns EUNAUTHORIZED if the token is invalid or expired.
	ValidateAccessToken(ctx context.Context, token string) (*User, error)

	// ValidateAPIKey validates an API key and returns the associated user.
	// Returns EUNAUTHORIZED if the key is invalid or expired.
	ValidateAPIKey(ctx context.Context, token string) (*User, error)

	// Logout invalidates a session by ID.
	Logout(ctx context.Context, sessionID string) error

	// LogoutAll invalidates all sessions for a user.
	LogoutAll(ctx context.Context, userID string) error

	// CreateAPIKey creates a new API key for a user.
	// The plaintext token is returned only once.
	CreateAPIKey(ctx context.Context, userID string, create APIKeyCreate) (*APIKeyCreated, error)

	// ListAPIKeys returns all API keys for a user.
	ListAPIKeys(ctx context.Context, userID string) ([]*APIKey, error)

	// DeleteAPIKey deletes an API key owned by userID.
	// Returns ENOTFOUND if the key does not exist or is not owned by this user.
	DeleteAPIKey(ctx context.Context, userID, id string) error

	// CanRegister returns true if no users exist yet (first-user registration gate).
	CanRegister(ctx context.Context) (bool, error)

	// CleanExpiredSessions removes sessions whose refresh tokens have expired.
	CleanExpiredSessions(ctx context.Context) error
}
