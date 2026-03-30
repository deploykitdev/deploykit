package deploykit

import (
	"context"
	"time"
)

// User represents a registered user account.
type User struct {
	ID           string    `json:"id"`
	Email        string    `json:"email"`
	Name         string    `json:"name"`
	PasswordHash string    `json:"-"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// UserService manages users.
type UserService interface {
	// CreateUser creates a new user. The password is hashed by the
	// implementation. The ID and timestamps are set automatically.
	// Returns ECONFLICT if the email is already taken.
	CreateUser(ctx context.Context, create UserCreate) (*User, error)

	// GetUser returns a user by ID.
	// Returns ENOTFOUND if the user does not exist.
	GetUser(ctx context.Context, id string) (*User, error)

	// ListUsers returns a filtered, paginated list of users
	// and the total matching count.
	ListUsers(ctx context.Context, filter UserFilter) ([]*User, int, error)

	// UpdateUser applies a partial update to a user by ID.
	// Returns the updated user. Returns ENOTFOUND if not found.
	// Returns ECONFLICT if the new email is already taken.
	UpdateUser(ctx context.Context, id string, update UserUpdate) (*User, error)

	// DeleteUser permanently removes a user by ID.
	// Returns ENOTFOUND if not found.
	DeleteUser(ctx context.Context, id string) error
}

// UserCreate holds fields required to create a user.
type UserCreate struct {
	Email    string `json:"email"`
	Name     string `json:"name"`
	Password string `json:"password"`
}

// Validate checks that all required fields are present.
func (c *UserCreate) Validate() error {
	if c.Email == "" {
		return Errorf(EINVALID, "Email is required.")
	}
	if c.Name == "" {
		return Errorf(EINVALID, "Name is required.")
	}
	if c.Password == "" {
		return Errorf(EINVALID, "Password is required.")
	}
	return nil
}

// UserUpdate holds fields that can be updated on a user.
// Nil pointer fields are left unchanged.
type UserUpdate struct {
	Email    *string `json:"email"`
	Name     *string `json:"name"`
	Password *string `json:"password"`
}

// Validate checks update fields.
func (u *UserUpdate) Validate() error {
	if u.Email != nil && *u.Email == "" {
		return Errorf(EINVALID, "Email cannot be empty.")
	}
	if u.Name != nil && *u.Name == "" {
		return Errorf(EINVALID, "Name cannot be empty.")
	}
	if u.Password != nil && *u.Password == "" {
		return Errorf(EINVALID, "Password cannot be empty.")
	}
	return nil
}

// UserFilter controls filtering and pagination for listing users.
type UserFilter struct {
	Email  *string
	Offset int
	Limit  int
}
