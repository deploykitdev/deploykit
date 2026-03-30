package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/heyjorgedev/deploykit"
	"golang.org/x/crypto/bcrypt"
)

// MustCreateUser is a test helper that creates a user or fails the test.
func MustCreateUser(t *testing.T, s *UserService, email, name, password string) *deploykit.User {
	t.Helper()
	u, err := s.CreateUser(context.Background(), deploykit.UserCreate{
		Email: email, Name: name, Password: password,
	})
	if err != nil {
		t.Fatal("creating seed user:", err)
	}
	return u
}

func TestUserService_CreateUser(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		svc := NewUserService(MustOpenDB(t))

		u, err := svc.CreateUser(context.Background(), deploykit.UserCreate{
			Email: "alice@example.com", Name: "Alice", Password: "secret123",
		})
		if err != nil {
			t.Fatal("unexpected error:", err)
		}
		if u.ID == "" {
			t.Fatal("expected non-empty ID")
		}
		if u.Email != "alice@example.com" {
			t.Fatalf("got email %q, want %q", u.Email, "alice@example.com")
		}
		if u.Name != "Alice" {
			t.Fatalf("got name %q, want %q", u.Name, "Alice")
		}
		if u.PasswordHash == "" {
			t.Fatal("expected non-empty PasswordHash")
		}
		if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte("secret123")); err != nil {
			t.Fatal("password hash does not match password:", err)
		}
		if u.CreatedAt.IsZero() {
			t.Fatal("expected non-zero CreatedAt")
		}
		if u.UpdatedAt.IsZero() {
			t.Fatal("expected non-zero UpdatedAt")
		}
	})

	t.Run("empty email", func(t *testing.T) {
		svc := NewUserService(MustOpenDB(t))

		_, err := svc.CreateUser(context.Background(), deploykit.UserCreate{
			Email: "", Name: "Alice", Password: "secret123",
		})
		if err == nil {
			t.Fatal("expected error")
		}
		if code := deploykit.ErrorCode(err); code != deploykit.EINVALID {
			t.Fatalf("got error code %q, want %q", code, deploykit.EINVALID)
		}
	})

	t.Run("empty name", func(t *testing.T) {
		svc := NewUserService(MustOpenDB(t))

		_, err := svc.CreateUser(context.Background(), deploykit.UserCreate{
			Email: "alice@example.com", Name: "", Password: "secret123",
		})
		if err == nil {
			t.Fatal("expected error")
		}
		if code := deploykit.ErrorCode(err); code != deploykit.EINVALID {
			t.Fatalf("got error code %q, want %q", code, deploykit.EINVALID)
		}
	})

	t.Run("empty password", func(t *testing.T) {
		svc := NewUserService(MustOpenDB(t))

		_, err := svc.CreateUser(context.Background(), deploykit.UserCreate{
			Email: "alice@example.com", Name: "Alice", Password: "",
		})
		if err == nil {
			t.Fatal("expected error")
		}
		if code := deploykit.ErrorCode(err); code != deploykit.EINVALID {
			t.Fatalf("got error code %q, want %q", code, deploykit.EINVALID)
		}
	})

	t.Run("duplicate email", func(t *testing.T) {
		svc := NewUserService(MustOpenDB(t))

		_, err := svc.CreateUser(context.Background(), deploykit.UserCreate{
			Email: "alice@example.com", Name: "Alice", Password: "secret123",
		})
		if err != nil {
			t.Fatal("first create:", err)
		}

		_, err = svc.CreateUser(context.Background(), deploykit.UserCreate{
			Email: "alice@example.com", Name: "Alice 2", Password: "other456",
		})
		if err == nil {
			t.Fatal("expected error on duplicate email")
		}
		if code := deploykit.ErrorCode(err); code != deploykit.ECONFLICT {
			t.Fatalf("got error code %q, want %q", code, deploykit.ECONFLICT)
		}
	})
}

func TestUserService_GetUser(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		svc := NewUserService(MustOpenDB(t))
		created := MustCreateUser(t, svc, "alice@example.com", "Alice", "secret123")

		got, err := svc.GetUser(context.Background(), created.ID)
		if err != nil {
			t.Fatal("unexpected error:", err)
		}
		if got.ID != created.ID {
			t.Fatalf("got ID %q, want %q", got.ID, created.ID)
		}
		if got.Email != created.Email {
			t.Fatalf("got email %q, want %q", got.Email, created.Email)
		}
		if got.Name != created.Name {
			t.Fatalf("got name %q, want %q", got.Name, created.Name)
		}
	})

	t.Run("not found", func(t *testing.T) {
		svc := NewUserService(MustOpenDB(t))

		_, err := svc.GetUser(context.Background(), "nonexistent-id")
		if err == nil {
			t.Fatal("expected error")
		}
		if code := deploykit.ErrorCode(err); code != deploykit.ENOTFOUND {
			t.Fatalf("got error code %q, want %q", code, deploykit.ENOTFOUND)
		}
	})
}

func TestUserService_ListUsers(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		svc := NewUserService(MustOpenDB(t))

		users, count, err := svc.ListUsers(context.Background(), deploykit.UserFilter{})
		if err != nil {
			t.Fatal("unexpected error:", err)
		}
		if len(users) != 0 {
			t.Fatalf("got %d users, want 0", len(users))
		}
		if count != 0 {
			t.Fatalf("got count %d, want 0", count)
		}
	})

	t.Run("returns all ordered by created_at desc", func(t *testing.T) {
		svc := NewUserService(MustOpenDB(t))
		MustCreateUser(t, svc, "first@example.com", "First", "password1")
		time.Sleep(time.Second) // ensure different created_at timestamps
		MustCreateUser(t, svc, "second@example.com", "Second", "password2")
		time.Sleep(time.Second)
		MustCreateUser(t, svc, "third@example.com", "Third", "password3")

		users, count, err := svc.ListUsers(context.Background(), deploykit.UserFilter{})
		if err != nil {
			t.Fatal("unexpected error:", err)
		}
		if len(users) != 3 {
			t.Fatalf("got %d users, want 3", len(users))
		}
		if count != 3 {
			t.Fatalf("got count %d, want 3", count)
		}
		// Most recent first
		if users[0].Email != "third@example.com" {
			t.Fatalf("got first user email %q, want %q", users[0].Email, "third@example.com")
		}
		if users[2].Email != "first@example.com" {
			t.Fatalf("got last user email %q, want %q", users[2].Email, "first@example.com")
		}
	})

	t.Run("filter by email", func(t *testing.T) {
		svc := NewUserService(MustOpenDB(t))
		MustCreateUser(t, svc, "alice@example.com", "Alice", "password1")
		MustCreateUser(t, svc, "bob@example.com", "Bob", "password2")
		MustCreateUser(t, svc, "alicia@example.com", "Alicia", "password3")

		users, count, err := svc.ListUsers(context.Background(), deploykit.UserFilter{
			Email: stringPtr("ali"),
		})
		if err != nil {
			t.Fatal("unexpected error:", err)
		}
		if len(users) != 2 {
			t.Fatalf("got %d users, want 2", len(users))
		}
		if count != 2 {
			t.Fatalf("got count %d, want 2", count)
		}
	})

	t.Run("pagination", func(t *testing.T) {
		svc := NewUserService(MustOpenDB(t))
		MustCreateUser(t, svc, "a@example.com", "A", "password1")
		MustCreateUser(t, svc, "b@example.com", "B", "password2")
		MustCreateUser(t, svc, "c@example.com", "C", "password3")

		users, count, err := svc.ListUsers(context.Background(), deploykit.UserFilter{
			Limit: 2, Offset: 0,
		})
		if err != nil {
			t.Fatal("unexpected error:", err)
		}
		if len(users) != 2 {
			t.Fatalf("got %d users, want 2", len(users))
		}
		if count != 3 {
			t.Fatalf("got count %d, want 3", count)
		}
	})
}

func TestUserService_UpdateUser(t *testing.T) {
	t.Run("update name", func(t *testing.T) {
		svc := NewUserService(MustOpenDB(t))
		original := MustCreateUser(t, svc, "alice@example.com", "Alice", "secret123")

		updated, err := svc.UpdateUser(context.Background(), original.ID, deploykit.UserUpdate{
			Name: stringPtr("Alice Smith"),
		})
		if err != nil {
			t.Fatal("unexpected error:", err)
		}
		if updated.Name != "Alice Smith" {
			t.Fatalf("got name %q, want %q", updated.Name, "Alice Smith")
		}
		if updated.UpdatedAt.Before(original.UpdatedAt) {
			t.Fatal("expected UpdatedAt to not be before original")
		}
	})

	t.Run("update email", func(t *testing.T) {
		svc := NewUserService(MustOpenDB(t))
		original := MustCreateUser(t, svc, "old@example.com", "Alice", "secret123")

		updated, err := svc.UpdateUser(context.Background(), original.ID, deploykit.UserUpdate{
			Email: stringPtr("new@example.com"),
		})
		if err != nil {
			t.Fatal("unexpected error:", err)
		}
		if updated.Email != "new@example.com" {
			t.Fatalf("got email %q, want %q", updated.Email, "new@example.com")
		}
	})

	t.Run("update password", func(t *testing.T) {
		svc := NewUserService(MustOpenDB(t))
		original := MustCreateUser(t, svc, "alice@example.com", "Alice", "old-password")

		updated, err := svc.UpdateUser(context.Background(), original.ID, deploykit.UserUpdate{
			Password: stringPtr("new-password"),
		})
		if err != nil {
			t.Fatal("unexpected error:", err)
		}
		if updated.PasswordHash == original.PasswordHash {
			t.Fatal("expected PasswordHash to change")
		}
		if err := bcrypt.CompareHashAndPassword([]byte(updated.PasswordHash), []byte("new-password")); err != nil {
			t.Fatal("new password hash does not match:", err)
		}
	})

	t.Run("no-op nil fields", func(t *testing.T) {
		svc := NewUserService(MustOpenDB(t))
		original := MustCreateUser(t, svc, "alice@example.com", "Alice", "secret123")

		updated, err := svc.UpdateUser(context.Background(), original.ID, deploykit.UserUpdate{})
		if err != nil {
			t.Fatal("unexpected error:", err)
		}
		if updated.Email != original.Email {
			t.Fatalf("got email %q, want %q", updated.Email, original.Email)
		}
		if updated.Name != original.Name {
			t.Fatalf("got name %q, want %q", updated.Name, original.Name)
		}
	})

	t.Run("empty name", func(t *testing.T) {
		svc := NewUserService(MustOpenDB(t))
		original := MustCreateUser(t, svc, "alice@example.com", "Alice", "secret123")

		_, err := svc.UpdateUser(context.Background(), original.ID, deploykit.UserUpdate{
			Name: stringPtr(""),
		})
		if err == nil {
			t.Fatal("expected error")
		}
		if code := deploykit.ErrorCode(err); code != deploykit.EINVALID {
			t.Fatalf("got error code %q, want %q", code, deploykit.EINVALID)
		}
	})

	t.Run("empty email", func(t *testing.T) {
		svc := NewUserService(MustOpenDB(t))
		original := MustCreateUser(t, svc, "alice@example.com", "Alice", "secret123")

		_, err := svc.UpdateUser(context.Background(), original.ID, deploykit.UserUpdate{
			Email: stringPtr(""),
		})
		if err == nil {
			t.Fatal("expected error")
		}
		if code := deploykit.ErrorCode(err); code != deploykit.EINVALID {
			t.Fatalf("got error code %q, want %q", code, deploykit.EINVALID)
		}
	})

	t.Run("duplicate email on update", func(t *testing.T) {
		svc := NewUserService(MustOpenDB(t))
		MustCreateUser(t, svc, "alice@example.com", "Alice", "password1")
		bob := MustCreateUser(t, svc, "bob@example.com", "Bob", "password2")

		_, err := svc.UpdateUser(context.Background(), bob.ID, deploykit.UserUpdate{
			Email: stringPtr("alice@example.com"),
		})
		if err == nil {
			t.Fatal("expected error on duplicate email")
		}
		if code := deploykit.ErrorCode(err); code != deploykit.ECONFLICT {
			t.Fatalf("got error code %q, want %q", code, deploykit.ECONFLICT)
		}
	})

	t.Run("not found", func(t *testing.T) {
		svc := NewUserService(MustOpenDB(t))

		_, err := svc.UpdateUser(context.Background(), "nonexistent-id", deploykit.UserUpdate{
			Name: stringPtr("x"),
		})
		if err == nil {
			t.Fatal("expected error")
		}
		if code := deploykit.ErrorCode(err); code != deploykit.ENOTFOUND {
			t.Fatalf("got error code %q, want %q", code, deploykit.ENOTFOUND)
		}
	})
}

func TestUserService_DeleteUser(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		svc := NewUserService(MustOpenDB(t))
		created := MustCreateUser(t, svc, "alice@example.com", "Alice", "secret123")

		if err := svc.DeleteUser(context.Background(), created.ID); err != nil {
			t.Fatal("unexpected error:", err)
		}

		// Verify it's actually gone.
		_, err := svc.GetUser(context.Background(), created.ID)
		if err == nil {
			t.Fatal("expected error after deletion")
		}
		if code := deploykit.ErrorCode(err); code != deploykit.ENOTFOUND {
			t.Fatalf("got error code %q, want %q", code, deploykit.ENOTFOUND)
		}
	})

	t.Run("not found", func(t *testing.T) {
		svc := NewUserService(MustOpenDB(t))

		err := svc.DeleteUser(context.Background(), "nonexistent-id")
		if err == nil {
			t.Fatal("expected error")
		}
		if code := deploykit.ErrorCode(err); code != deploykit.ENOTFOUND {
			t.Fatalf("got error code %q, want %q", code, deploykit.ENOTFOUND)
		}
	})

	t.Run("delete twice", func(t *testing.T) {
		svc := NewUserService(MustOpenDB(t))
		created := MustCreateUser(t, svc, "alice@example.com", "Alice", "secret123")

		if err := svc.DeleteUser(context.Background(), created.ID); err != nil {
			t.Fatal("first delete:", err)
		}

		err := svc.DeleteUser(context.Background(), created.ID)
		if err == nil {
			t.Fatal("expected error on second delete")
		}
		if code := deploykit.ErrorCode(err); code != deploykit.ENOTFOUND {
			t.Fatalf("got error code %q, want %q", code, deploykit.ENOTFOUND)
		}
	})
}
