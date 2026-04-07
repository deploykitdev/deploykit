package sqlite

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"testing"
	"time"

	"github.com/heyjorgedev/deploykit"
)

func MustCreateAuthUser(t *testing.T, db *DB, email, name, password string) *deploykit.User {
	t.Helper()
	svc := NewUserService(db)
	u, err := svc.CreateUser(context.Background(), deploykit.UserCreate{
		Email: email, Name: name, Password: password,
	})
	if err != nil {
		t.Fatal("creating seed user:", err)
	}
	return u
}

func TestAuthService_CanRegister(t *testing.T) {
	db := MustOpenDB(t)
	auth := NewAuthService(db)
	ctx := context.Background()

	t.Run("true when no users", func(t *testing.T) {
		ok, err := auth.CanRegister(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if !ok {
			t.Fatal("expected can_register=true with no users")
		}
	})

	t.Run("false after user created", func(t *testing.T) {
		MustCreateAuthUser(t, db, "admin@example.com", "Admin", "password123")

		ok, err := auth.CanRegister(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if ok {
			t.Fatal("expected can_register=false after user exists")
		}
	})
}

func TestAuthService_Login(t *testing.T) {
	db := MustOpenDB(t)
	auth := NewAuthService(db)
	ctx := context.Background()
	MustCreateAuthUser(t, db, "alice@example.com", "Alice", "secret123")

	t.Run("ok", func(t *testing.T) {
		tokens, err := auth.Login(ctx, deploykit.LoginRequest{
			Email: "alice@example.com", Password: "secret123",
		})
		if err != nil {
			t.Fatal("unexpected error:", err)
		}
		if tokens.AccessToken == "" {
			t.Fatal("expected non-empty access token")
		}
		if tokens.RefreshToken == "" {
			t.Fatal("expected non-empty refresh token")
		}
		if tokens.ExpiresAt.Before(time.Now()) {
			t.Fatal("expected expires_at in the future")
		}
	})

	t.Run("wrong password", func(t *testing.T) {
		_, err := auth.Login(ctx, deploykit.LoginRequest{
			Email: "alice@example.com", Password: "wrongpassword",
		})
		if err == nil {
			t.Fatal("expected error")
		}
		if code := deploykit.ErrorCode(err); code != deploykit.EUNAUTHORIZED {
			t.Fatalf("got %q, want %q", code, deploykit.EUNAUTHORIZED)
		}
	})

	t.Run("nonexistent email", func(t *testing.T) {
		_, err := auth.Login(ctx, deploykit.LoginRequest{
			Email: "nobody@example.com", Password: "secret123",
		})
		if err == nil {
			t.Fatal("expected error")
		}
		if code := deploykit.ErrorCode(err); code != deploykit.EUNAUTHORIZED {
			t.Fatalf("got %q, want %q", code, deploykit.EUNAUTHORIZED)
		}
	})

	t.Run("validation errors", func(t *testing.T) {
		_, err := auth.Login(ctx, deploykit.LoginRequest{Email: "", Password: "secret123"})
		if code := deploykit.ErrorCode(err); code != deploykit.EINVALID {
			t.Fatalf("got %q, want %q", code, deploykit.EINVALID)
		}

		_, err = auth.Login(ctx, deploykit.LoginRequest{Email: "alice@example.com", Password: ""})
		if code := deploykit.ErrorCode(err); code != deploykit.EINVALID {
			t.Fatalf("got %q, want %q", code, deploykit.EINVALID)
		}
	})

	t.Run("tokens are stored as hashes not plaintext", func(t *testing.T) {
		tokens, err := auth.Login(ctx, deploykit.LoginRequest{
			Email: "alice@example.com", Password: "secret123",
		})
		if err != nil {
			t.Fatal(err)
		}

		// Verify the plaintext token is NOT stored in the DB.
		var count int
		err = db.db.QueryRow(`SELECT COUNT(*) FROM sessions WHERE access_token_hash = ?`, tokens.AccessToken).Scan(&count)
		if err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatal("plaintext token should not match any hash in DB")
		}

		// Verify the SHA-256 hash IS stored.
		h := sha256.Sum256([]byte(tokens.AccessToken))
		hash := hex.EncodeToString(h[:])
		err = db.db.QueryRow(`SELECT COUNT(*) FROM sessions WHERE access_token_hash = ?`, hash).Scan(&count)
		if err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Fatal("expected hashed token to be in DB")
		}
	})
}

func TestAuthService_ValidateAccessToken(t *testing.T) {
	db := MustOpenDB(t)
	auth := NewAuthService(db)
	ctx := context.Background()
	MustCreateAuthUser(t, db, "alice@example.com", "Alice", "secret123")

	t.Run("valid token", func(t *testing.T) {
		tokens, err := auth.Login(ctx, deploykit.LoginRequest{
			Email: "alice@example.com", Password: "secret123",
		})
		if err != nil {
			t.Fatal(err)
		}

		user, err := auth.ValidateAccessToken(ctx, tokens.AccessToken)
		if err != nil {
			t.Fatal("unexpected error:", err)
		}
		if user.Email != "alice@example.com" {
			t.Fatalf("got email %q, want %q", user.Email, "alice@example.com")
		}
	})

	t.Run("invalid token", func(t *testing.T) {
		_, err := auth.ValidateAccessToken(ctx, "garbage-token")
		if err == nil {
			t.Fatal("expected error")
		}
		if code := deploykit.ErrorCode(err); code != deploykit.EUNAUTHORIZED {
			t.Fatalf("got %q, want %q", code, deploykit.EUNAUTHORIZED)
		}
	})

	t.Run("empty token", func(t *testing.T) {
		_, err := auth.ValidateAccessToken(ctx, "")
		if code := deploykit.ErrorCode(err); code != deploykit.EUNAUTHORIZED {
			t.Fatalf("got %q, want %q", code, deploykit.EUNAUTHORIZED)
		}
	})
}

func TestAuthService_RefreshSession(t *testing.T) {
	db := MustOpenDB(t)
	auth := NewAuthService(db)
	ctx := context.Background()
	MustCreateAuthUser(t, db, "alice@example.com", "Alice", "secret123")

	t.Run("rotation", func(t *testing.T) {
		tokens, err := auth.Login(ctx, deploykit.LoginRequest{
			Email: "alice@example.com", Password: "secret123",
		})
		if err != nil {
			t.Fatal(err)
		}

		// Refresh with the refresh token.
		newTokens, err := auth.RefreshSession(ctx, tokens.RefreshToken)
		if err != nil {
			t.Fatal("unexpected error:", err)
		}
		if newTokens.AccessToken == tokens.AccessToken {
			t.Fatal("new access token should differ from old")
		}
		if newTokens.RefreshToken == tokens.RefreshToken {
			t.Fatal("new refresh token should differ from old")
		}

		// Old access token should be invalid.
		_, err = auth.ValidateAccessToken(ctx, tokens.AccessToken)
		if err == nil {
			t.Fatal("old access token should be invalid after refresh")
		}

		// Old refresh token should be invalid.
		_, err = auth.RefreshSession(ctx, tokens.RefreshToken)
		if err == nil {
			t.Fatal("old refresh token should be invalid after rotation")
		}

		// New access token should work.
		user, err := auth.ValidateAccessToken(ctx, newTokens.AccessToken)
		if err != nil {
			t.Fatal("new access token should be valid:", err)
		}
		if user.Email != "alice@example.com" {
			t.Fatalf("got email %q, want %q", user.Email, "alice@example.com")
		}
	})

	t.Run("invalid refresh token", func(t *testing.T) {
		_, err := auth.RefreshSession(ctx, "garbage-token")
		if code := deploykit.ErrorCode(err); code != deploykit.EUNAUTHORIZED {
			t.Fatalf("got %q, want %q", code, deploykit.EUNAUTHORIZED)
		}
	})
}

func TestAuthService_Logout(t *testing.T) {
	db := MustOpenDB(t)
	auth := NewAuthService(db)
	ctx := context.Background()
	MustCreateAuthUser(t, db, "alice@example.com", "Alice", "secret123")

	t.Run("logout all", func(t *testing.T) {
		// Create two sessions.
		tokens1, _ := auth.Login(ctx, deploykit.LoginRequest{Email: "alice@example.com", Password: "secret123"})
		tokens2, _ := auth.Login(ctx, deploykit.LoginRequest{Email: "alice@example.com", Password: "secret123"})

		// Get user to find the ID.
		user, _ := auth.ValidateAccessToken(ctx, tokens1.AccessToken)

		// Logout all.
		if err := auth.LogoutAll(ctx, user.ID); err != nil {
			t.Fatal(err)
		}

		// Both tokens should be invalid.
		if _, err := auth.ValidateAccessToken(ctx, tokens1.AccessToken); err == nil {
			t.Fatal("token1 should be invalid after logout all")
		}
		if _, err := auth.ValidateAccessToken(ctx, tokens2.AccessToken); err == nil {
			t.Fatal("token2 should be invalid after logout all")
		}
	})
}

func TestAuthService_APIKey(t *testing.T) {
	db := MustOpenDB(t)
	auth := NewAuthService(db)
	ctx := context.Background()
	user := MustCreateAuthUser(t, db, "alice@example.com", "Alice", "secret123")

	t.Run("create and validate", func(t *testing.T) {
		created, err := auth.CreateAPIKey(ctx, user.ID, deploykit.APIKeyCreate{
			Description: "CI/CD key",
		})
		if err != nil {
			t.Fatal(err)
		}
		if created.Token == "" {
			t.Fatal("expected non-empty plaintext token")
		}
		if created.Prefix == "" {
			t.Fatal("expected non-empty prefix")
		}
		if created.Description != "CI/CD key" {
			t.Fatalf("got description %q, want %q", created.Description, "CI/CD key")
		}

		// Validate the API key.
		u, err := auth.ValidateAPIKey(ctx, created.Token)
		if err != nil {
			t.Fatal("unexpected error:", err)
		}
		if u.ID != user.ID {
			t.Fatalf("got user ID %q, want %q", u.ID, user.ID)
		}
	})

	t.Run("list", func(t *testing.T) {
		keys, err := auth.ListAPIKeys(ctx, user.ID)
		if err != nil {
			t.Fatal(err)
		}
		if len(keys) == 0 {
			t.Fatal("expected at least one API key")
		}
	})

	t.Run("delete", func(t *testing.T) {
		created, _ := auth.CreateAPIKey(ctx, user.ID, deploykit.APIKeyCreate{
			Description: "Temp key",
		})

		if err := auth.DeleteAPIKey(ctx, user.ID, created.ID); err != nil {
			t.Fatal(err)
		}

		// Should no longer validate.
		_, err := auth.ValidateAPIKey(ctx, created.Token)
		if err == nil {
			t.Fatal("deleted API key should not validate")
		}
	})

	t.Run("delete not found", func(t *testing.T) {
		err := auth.DeleteAPIKey(ctx, user.ID, "nonexistent-id")
		if code := deploykit.ErrorCode(err); code != deploykit.ENOTFOUND {
			t.Fatalf("got %q, want %q", code, deploykit.ENOTFOUND)
		}
	})

	t.Run("delete other user's key returns not found", func(t *testing.T) {
		// Create a key owned by `user`.
		created, err := auth.CreateAPIKey(ctx, user.ID, deploykit.APIKeyCreate{
			Description: "Owned by user A",
		})
		if err != nil {
			t.Fatal(err)
		}

		// Create a second user and attempt deletion as that user.
		otherUser := MustCreateAuthUser(t, db, "other@example.com", "Other", "password123")

		err = auth.DeleteAPIKey(ctx, otherUser.ID, created.ID)
		if code := deploykit.ErrorCode(err); code != deploykit.ENOTFOUND {
			t.Fatalf("got %q, want %q", code, deploykit.ENOTFOUND)
		}

		// Key should still exist and still validate.
		if _, err := auth.ValidateAPIKey(ctx, created.Token); err != nil {
			t.Fatalf("key should still be valid: %v", err)
		}
	})

	t.Run("expired key rejected", func(t *testing.T) {
		past := time.Now().Add(-1 * time.Hour)
		created, err := auth.CreateAPIKey(ctx, user.ID, deploykit.APIKeyCreate{
			Description: "Expired key",
			ExpiresAt:   &past,
		})
		if err != nil {
			t.Fatal(err)
		}

		_, err = auth.ValidateAPIKey(ctx, created.Token)
		if err == nil {
			t.Fatal("expired API key should not validate")
		}
		if code := deploykit.ErrorCode(err); code != deploykit.EUNAUTHORIZED {
			t.Fatalf("got %q, want %q", code, deploykit.EUNAUTHORIZED)
		}
	})

	t.Run("validation error", func(t *testing.T) {
		_, err := auth.CreateAPIKey(ctx, user.ID, deploykit.APIKeyCreate{Description: ""})
		if code := deploykit.ErrorCode(err); code != deploykit.EINVALID {
			t.Fatalf("got %q, want %q", code, deploykit.EINVALID)
		}
	})
}

func TestAuthService_CleanExpiredSessions(t *testing.T) {
	db := MustOpenDB(t)
	auth := NewAuthService(db)
	ctx := context.Background()
	user := MustCreateAuthUser(t, db, "alice@example.com", "Alice", "secret123")

	// Create a session, then manually expire it.
	tokens, err := auth.Login(ctx, deploykit.LoginRequest{
		Email: "alice@example.com", Password: "secret123",
	})
	if err != nil {
		t.Fatal(err)
	}

	// Set refresh_expires_at to the past.
	past := time.Now().Add(-1 * time.Hour).UTC().Format(timeFormat)
	_, err = db.db.Exec(`UPDATE sessions SET refresh_expires_at = ? WHERE user_id = ?`, past, user.ID)
	if err != nil {
		t.Fatal(err)
	}

	if err := auth.CleanExpiredSessions(ctx); err != nil {
		t.Fatal(err)
	}

	// Token should be invalid now.
	_, err = auth.ValidateAccessToken(ctx, tokens.AccessToken)
	if err == nil {
		t.Fatal("expected expired session to be cleaned")
	}
}
