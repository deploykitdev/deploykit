package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/deploykitdev/deploykit"
	"golang.org/x/crypto/bcrypt"
)

// UserService implements deploykit.UserService using SQLite.
type UserService struct {
	db *DB
}

// NewUserService creates a new UserService backed by the given DB.
func NewUserService(db *DB) *UserService {
	return &UserService{db: db}
}

func (s *UserService) CreateUser(ctx context.Context, create deploykit.UserCreate) (*deploykit.User, error) {
	if err := create.Validate(); err != nil {
		return nil, err
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(create.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("hashing password: %w", err)
	}

	role := create.Role
	if role == "" {
		role = deploykit.RoleMember
	}

	user := &deploykit.User{
		ID:           uuid.New().String(),
		Email:        create.Email,
		Name:         create.Name,
		Role:         role,
		PasswordHash: string(hash),
		CreatedAt:    time.Now().UTC(),
		UpdatedAt:    time.Now().UTC(),
	}

	_, err = s.db.db.ExecContext(ctx,
		`INSERT INTO users (id, email, name, role, password_hash, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		user.ID, user.Email, user.Name, user.Role, user.PasswordHash,
		user.CreatedAt.Format(timeFormat),
		user.UpdatedAt.Format(timeFormat),
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return nil, deploykit.Errorf(deploykit.ECONFLICT, "A user with that email already exists.")
		}
		return nil, fmt.Errorf("creating user: %w", err)
	}

	return user, nil
}

func (s *UserService) GetUser(ctx context.Context, id string) (*deploykit.User, error) {
	user := &deploykit.User{}
	var createdAt, updatedAt string

	err := s.db.db.QueryRowContext(ctx,
		`SELECT id, email, name, role, password_hash, created_at, updated_at FROM users WHERE id = ?`, id,
	).Scan(&user.ID, &user.Email, &user.Name, &user.Role, &user.PasswordHash, &createdAt, &updatedAt)
	if err == sql.ErrNoRows {
		return nil, deploykit.Errorf(deploykit.ENOTFOUND, "User not found.")
	} else if err != nil {
		return nil, fmt.Errorf("getting user %s: %w", id, err)
	}

	user.CreatedAt, _ = time.Parse(timeFormat, createdAt)
	user.UpdatedAt, _ = time.Parse(timeFormat, updatedAt)

	return user, nil
}

func (s *UserService) ListUsers(ctx context.Context, filter deploykit.UserFilter) ([]*deploykit.User, int, error) {
	where := []string{"1=1"}
	args := []any{}

	if filter.Email != nil {
		where = append(where, "email LIKE ?")
		args = append(args, "%"+*filter.Email+"%")
	}
	if filter.Role != nil {
		where = append(where, "role = ?")
		args = append(args, string(*filter.Role))
	}

	limit := filter.Limit
	if limit <= 0 {
		limit = 20
	} else if limit > 100 {
		limit = 100
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}

	query := fmt.Sprintf(
		`SELECT id, email, name, role, password_hash, created_at, updated_at, COUNT(*) OVER() AS total_count
		 FROM users WHERE %s ORDER BY created_at DESC LIMIT ? OFFSET ?`,
		strings.Join(where, " AND "),
	)
	args = append(args, limit, offset)

	rows, err := s.db.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("listing users: %w", err)
	}
	defer rows.Close()

	var users []*deploykit.User
	var totalCount int

	for rows.Next() {
		u := &deploykit.User{}
		var createdAt, updatedAt string
		if err := rows.Scan(&u.ID, &u.Email, &u.Name, &u.Role, &u.PasswordHash, &createdAt, &updatedAt, &totalCount); err != nil {
			return nil, 0, fmt.Errorf("scanning user row: %w", err)
		}
		u.CreatedAt, _ = time.Parse(timeFormat, createdAt)
		u.UpdatedAt, _ = time.Parse(timeFormat, updatedAt)
		users = append(users, u)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterating user rows: %w", err)
	}

	return users, totalCount, nil
}

func (s *UserService) UpdateUser(ctx context.Context, id string, update deploykit.UserUpdate) (*deploykit.User, error) {
	if err := update.Validate(); err != nil {
		return nil, err
	}

	tx, err := s.db.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback()

	user := &deploykit.User{}
	var createdAt, updatedAt string
	err = tx.QueryRowContext(ctx,
		`SELECT id, email, name, role, password_hash, created_at, updated_at FROM users WHERE id = ?`, id,
	).Scan(&user.ID, &user.Email, &user.Name, &user.Role, &user.PasswordHash, &createdAt, &updatedAt)
	if err == sql.ErrNoRows {
		return nil, deploykit.Errorf(deploykit.ENOTFOUND, "User not found.")
	} else if err != nil {
		return nil, fmt.Errorf("getting user for update %s: %w", id, err)
	}
	user.CreatedAt, _ = time.Parse(timeFormat, createdAt)

	if update.Email != nil {
		user.Email = *update.Email
	}
	if update.Name != nil {
		user.Name = *update.Name
	}
	if update.Password != nil {
		hash, err := bcrypt.GenerateFromPassword([]byte(*update.Password), bcrypt.DefaultCost)
		if err != nil {
			return nil, fmt.Errorf("hashing password: %w", err)
		}
		user.PasswordHash = string(hash)
	}
	if update.Role != nil {
		user.Role = *update.Role
	}
	user.UpdatedAt = time.Now().UTC()

	_, err = tx.ExecContext(ctx,
		`UPDATE users SET email = ?, name = ?, role = ?, password_hash = ?, updated_at = ? WHERE id = ?`,
		user.Email, user.Name, user.Role, user.PasswordHash, user.UpdatedAt.Format(timeFormat), user.ID,
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return nil, deploykit.Errorf(deploykit.ECONFLICT, "A user with that email already exists.")
		}
		return nil, fmt.Errorf("updating user %s: %w", id, err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("committing user update: %w", err)
	}

	return user, nil
}

func (s *UserService) DeleteUser(ctx context.Context, id string) error {
	result, err := s.db.db.ExecContext(ctx, `DELETE FROM users WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("deleting user %s: %w", id, err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return deploykit.Errorf(deploykit.ENOTFOUND, "User not found.")
	}

	return nil
}
