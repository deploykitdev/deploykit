package sqlite

import (
	"io"
	"log/slog"
	"testing"
)

// MustOpenDB creates an in-memory SQLite database with migrations applied.
func MustOpenDB(t *testing.T) *DB {
	t.Helper()
	db := NewDB(":memory:", slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := db.Open(); err != nil {
		t.Fatal("opening db:", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Fatal("closing db:", err)
		}
	})
	return db
}

func stringPtr(s string) *string { return &s }
