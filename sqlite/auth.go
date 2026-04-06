package sqlite

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/heyjorgedev/deploykit"
	"golang.org/x/crypto/bcrypt"
)

const (
	accessTokenTTL  = 15 * time.Minute
	refreshTokenTTL = 7 * 24 * time.Hour
)

// AuthService implements deploykit.AuthService using SQLite.
type AuthService struct {
	db *DB
}

// NewAuthService creates a new AuthService backed by the given DB.
func NewAuthService(db *DB) *AuthService {
	return &AuthService{db: db}
}

func (s *AuthService) Login(ctx context.Context, req deploykit.LoginRequest) (*deploykit.AuthTokens, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}

	user, err := s.findUserByEmail(ctx, req.Email)
	if err != nil {
		return nil, deploykit.Errorf(deploykit.EUNAUTHORIZED, "Invalid email or password.")
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		return nil, deploykit.Errorf(deploykit.EUNAUTHORIZED, "Invalid email or password.")
	}

	return s.createSession(ctx, user.ID)
}

func (s *AuthService) RefreshSession(ctx context.Context, refreshToken string) (*deploykit.AuthTokens, error) {
	if refreshToken == "" {
		return nil, deploykit.Errorf(deploykit.EUNAUTHORIZED, "Refresh token is required.")
	}

	refreshHash := hashToken(refreshToken)

	tx, err := s.db.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback()

	var session deploykit.Session
	var expiresAt, refreshExpiresAt, createdAt string
	err = tx.QueryRowContext(ctx,
		`SELECT id, user_id, expires_at, refresh_expires_at, created_at
		 FROM sessions WHERE refresh_token_hash = ?`, refreshHash,
	).Scan(&session.ID, &session.UserID, &expiresAt, &refreshExpiresAt, &createdAt)
	if err == sql.ErrNoRows {
		return nil, deploykit.Errorf(deploykit.EUNAUTHORIZED, "Invalid refresh token.")
	} else if err != nil {
		return nil, fmt.Errorf("looking up refresh token: %w", err)
	}

	session.RefreshExpiresAt, _ = time.Parse(timeFormat, refreshExpiresAt)
	if time.Now().UTC().After(session.RefreshExpiresAt) {
		// Delete expired session.
		tx.ExecContext(ctx, `DELETE FROM sessions WHERE id = ?`, session.ID)
		tx.Commit()
		return nil, deploykit.Errorf(deploykit.EUNAUTHORIZED, "Refresh token has expired.")
	}

	// Generate new token pair (rotation).
	accessPlain, accessHash, err := generateToken()
	if err != nil {
		return nil, fmt.Errorf("generating access token: %w", err)
	}
	refreshPlain, newRefreshHash, err := generateToken()
	if err != nil {
		return nil, fmt.Errorf("generating refresh token: %w", err)
	}

	now := time.Now().UTC()
	newExpiresAt := now.Add(accessTokenTTL)
	newRefreshExpiresAt := now.Add(refreshTokenTTL)

	_, err = tx.ExecContext(ctx,
		`UPDATE sessions SET access_token_hash = ?, refresh_token_hash = ?, expires_at = ?, refresh_expires_at = ?
		 WHERE id = ?`,
		accessHash, newRefreshHash,
		newExpiresAt.Format(timeFormat), newRefreshExpiresAt.Format(timeFormat),
		session.ID,
	)
	if err != nil {
		return nil, fmt.Errorf("updating session: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("committing session refresh: %w", err)
	}

	return &deploykit.AuthTokens{
		AccessToken:  accessPlain,
		RefreshToken: refreshPlain,
		ExpiresAt:    newExpiresAt,
	}, nil
}

func (s *AuthService) ValidateAccessToken(ctx context.Context, token string) (*deploykit.User, error) {
	if token == "" {
		return nil, deploykit.Errorf(deploykit.EUNAUTHORIZED, "Token is required.")
	}

	tokenHash := hashToken(token)

	var userID, expiresAtStr string
	err := s.db.db.QueryRowContext(ctx,
		`SELECT user_id, expires_at FROM sessions WHERE access_token_hash = ?`, tokenHash,
	).Scan(&userID, &expiresAtStr)
	if err == sql.ErrNoRows {
		return nil, deploykit.Errorf(deploykit.EUNAUTHORIZED, "Invalid access token.")
	} else if err != nil {
		return nil, fmt.Errorf("looking up access token: %w", err)
	}

	expiresAt, _ := time.Parse(timeFormat, expiresAtStr)
	if time.Now().UTC().After(expiresAt) {
		return nil, deploykit.Errorf(deploykit.EUNAUTHORIZED, "Access token has expired.")
	}

	return s.findUserByID(ctx, userID)
}

func (s *AuthService) ValidateAPIKey(ctx context.Context, token string) (*deploykit.User, error) {
	if token == "" {
		return nil, deploykit.Errorf(deploykit.EUNAUTHORIZED, "API key is required.")
	}

	tokenHash := hashToken(token)

	var userID string
	var expiresAtStr sql.NullString
	err := s.db.db.QueryRowContext(ctx,
		`SELECT user_id, expires_at FROM api_keys WHERE token_hash = ?`, tokenHash,
	).Scan(&userID, &expiresAtStr)
	if err == sql.ErrNoRows {
		return nil, deploykit.Errorf(deploykit.EUNAUTHORIZED, "Invalid API key.")
	} else if err != nil {
		return nil, fmt.Errorf("looking up API key: %w", err)
	}

	if expiresAtStr.Valid {
		expiresAt, _ := time.Parse(timeFormat, expiresAtStr.String)
		if time.Now().UTC().After(expiresAt) {
			return nil, deploykit.Errorf(deploykit.EUNAUTHORIZED, "API key has expired.")
		}
	}

	// Update last_used_at (best-effort, don't fail the request).
	s.db.db.ExecContext(ctx,
		`UPDATE api_keys SET last_used_at = ? WHERE token_hash = ?`,
		time.Now().UTC().Format(timeFormat), tokenHash,
	)

	return s.findUserByID(ctx, userID)
}

func (s *AuthService) Logout(ctx context.Context, sessionID string) error {
	_, err := s.db.db.ExecContext(ctx, `DELETE FROM sessions WHERE id = ?`, sessionID)
	if err != nil {
		return fmt.Errorf("deleting session: %w", err)
	}
	return nil
}

func (s *AuthService) LogoutAll(ctx context.Context, userID string) error {
	_, err := s.db.db.ExecContext(ctx, `DELETE FROM sessions WHERE user_id = ?`, userID)
	if err != nil {
		return fmt.Errorf("deleting user sessions: %w", err)
	}
	return nil
}

func (s *AuthService) CreateAPIKey(ctx context.Context, userID string, create deploykit.APIKeyCreate) (*deploykit.APIKeyCreated, error) {
	if err := create.Validate(); err != nil {
		return nil, err
	}

	plaintext, tokenHash, err := generateToken()
	if err != nil {
		return nil, fmt.Errorf("generating API key: %w", err)
	}

	prefix := plaintext
	if len(prefix) > 8 {
		prefix = prefix[:8]
	}

	now := time.Now().UTC()
	apiKey := &deploykit.APIKey{
		ID:          uuid.New().String(),
		UserID:      userID,
		Description: create.Description,
		Prefix:      prefix,
		TokenHash:   tokenHash,
		ExpiresAt:   create.ExpiresAt,
		CreatedAt:   now,
	}

	var expiresAtVal any
	if create.ExpiresAt != nil {
		expiresAtVal = create.ExpiresAt.UTC().Format(timeFormat)
	}

	_, err = s.db.db.ExecContext(ctx,
		`INSERT INTO api_keys (id, user_id, description, prefix, token_hash, expires_at, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		apiKey.ID, apiKey.UserID, apiKey.Description, apiKey.Prefix, apiKey.TokenHash,
		expiresAtVal, apiKey.CreatedAt.Format(timeFormat),
	)
	if err != nil {
		return nil, fmt.Errorf("inserting API key: %w", err)
	}

	return &deploykit.APIKeyCreated{
		APIKey: *apiKey,
		Token:  plaintext,
	}, nil
}

func (s *AuthService) ListAPIKeys(ctx context.Context, userID string) ([]*deploykit.APIKey, error) {
	rows, err := s.db.db.QueryContext(ctx,
		`SELECT id, user_id, description, prefix, expires_at, last_used_at, created_at
		 FROM api_keys WHERE user_id = ? ORDER BY created_at DESC`, userID,
	)
	if err != nil {
		return nil, fmt.Errorf("listing API keys: %w", err)
	}
	defer rows.Close()

	var keys []*deploykit.APIKey
	for rows.Next() {
		k := &deploykit.APIKey{}
		var expiresAt, lastUsedAt sql.NullString
		var createdAt string
		if err := rows.Scan(&k.ID, &k.UserID, &k.Description, &k.Prefix, &expiresAt, &lastUsedAt, &createdAt); err != nil {
			return nil, fmt.Errorf("scanning API key row: %w", err)
		}
		k.CreatedAt, _ = time.Parse(timeFormat, createdAt)
		if expiresAt.Valid {
			t, _ := time.Parse(timeFormat, expiresAt.String)
			k.ExpiresAt = &t
		}
		if lastUsedAt.Valid {
			t, _ := time.Parse(timeFormat, lastUsedAt.String)
			k.LastUsedAt = &t
		}
		keys = append(keys, k)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating API key rows: %w", err)
	}

	return keys, nil
}

func (s *AuthService) DeleteAPIKey(ctx context.Context, id string) error {
	result, err := s.db.db.ExecContext(ctx, `DELETE FROM api_keys WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("deleting API key: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return deploykit.Errorf(deploykit.ENOTFOUND, "API key not found.")
	}

	return nil
}

func (s *AuthService) CanRegister(ctx context.Context) (bool, error) {
	var count int
	err := s.db.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("counting users: %w", err)
	}
	return count == 0, nil
}

func (s *AuthService) CleanExpiredSessions(ctx context.Context) error {
	_, err := s.db.db.ExecContext(ctx,
		`DELETE FROM sessions WHERE refresh_expires_at < ?`,
		time.Now().UTC().Format(timeFormat),
	)
	if err != nil {
		return fmt.Errorf("cleaning expired sessions: %w", err)
	}
	return nil
}

// createSession generates a new token pair and inserts a session row.
func (s *AuthService) createSession(ctx context.Context, userID string) (*deploykit.AuthTokens, error) {
	accessPlain, accessHash, err := generateToken()
	if err != nil {
		return nil, fmt.Errorf("generating access token: %w", err)
	}
	refreshPlain, refreshHash, err := generateToken()
	if err != nil {
		return nil, fmt.Errorf("generating refresh token: %w", err)
	}

	now := time.Now().UTC()
	session := &deploykit.Session{
		ID:               uuid.New().String(),
		UserID:           userID,
		AccessTokenHash:  accessHash,
		RefreshTokenHash: refreshHash,
		ExpiresAt:        now.Add(accessTokenTTL),
		RefreshExpiresAt: now.Add(refreshTokenTTL),
		CreatedAt:        now,
	}

	_, err = s.db.db.ExecContext(ctx,
		`INSERT INTO sessions (id, user_id, access_token_hash, refresh_token_hash, expires_at, refresh_expires_at, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		session.ID, session.UserID, session.AccessTokenHash, session.RefreshTokenHash,
		session.ExpiresAt.Format(timeFormat), session.RefreshExpiresAt.Format(timeFormat),
		session.CreatedAt.Format(timeFormat),
	)
	if err != nil {
		return nil, fmt.Errorf("inserting session: %w", err)
	}

	return &deploykit.AuthTokens{
		AccessToken:  accessPlain,
		RefreshToken: refreshPlain,
		ExpiresAt:    session.ExpiresAt,
	}, nil
}

// findUserByEmail looks up a user by email, including the password hash.
func (s *AuthService) findUserByEmail(ctx context.Context, email string) (*deploykit.User, error) {
	user := &deploykit.User{}
	var createdAt, updatedAt string

	err := s.db.db.QueryRowContext(ctx,
		`SELECT id, email, name, role, password_hash, created_at, updated_at FROM users WHERE email = ?`, email,
	).Scan(&user.ID, &user.Email, &user.Name, &user.Role, &user.PasswordHash, &createdAt, &updatedAt)
	if err == sql.ErrNoRows {
		return nil, deploykit.Errorf(deploykit.ENOTFOUND, "User not found.")
	} else if err != nil {
		return nil, fmt.Errorf("finding user by email: %w", err)
	}

	user.CreatedAt, _ = time.Parse(timeFormat, createdAt)
	user.UpdatedAt, _ = time.Parse(timeFormat, updatedAt)

	return user, nil
}

// findUserByID looks up a user by ID.
func (s *AuthService) findUserByID(ctx context.Context, id string) (*deploykit.User, error) {
	user := &deploykit.User{}
	var createdAt, updatedAt string

	err := s.db.db.QueryRowContext(ctx,
		`SELECT id, email, name, role, password_hash, created_at, updated_at FROM users WHERE id = ?`, id,
	).Scan(&user.ID, &user.Email, &user.Name, &user.Role, &user.PasswordHash, &createdAt, &updatedAt)
	if err == sql.ErrNoRows {
		return nil, deploykit.Errorf(deploykit.ENOTFOUND, "User not found.")
	} else if err != nil {
		return nil, fmt.Errorf("finding user by id: %w", err)
	}

	user.CreatedAt, _ = time.Parse(timeFormat, createdAt)
	user.UpdatedAt, _ = time.Parse(timeFormat, updatedAt)

	return user, nil
}

// generateToken creates a 32-byte random token and returns the base64url-encoded
// plaintext and its SHA-256 hex digest.
func generateToken() (plaintext string, hash string, err error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", "", fmt.Errorf("reading random bytes: %w", err)
	}
	plaintext = base64.RawURLEncoding.EncodeToString(b)
	return plaintext, hashToken(plaintext), nil
}

// hashToken returns the hex-encoded SHA-256 digest of a token string.
func hashToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}
