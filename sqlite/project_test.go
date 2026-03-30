package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/heyjorgedev/deploykit"
)

// MustCreateProject is a test helper that creates a project or fails the test.
func MustCreateProject(t *testing.T, s *ProjectService, name string) *deploykit.Project {
	t.Helper()
	p, err := s.CreateProject(context.Background(), deploykit.ProjectCreate{Name: name})
	if err != nil {
		t.Fatal("creating seed project:", err)
	}
	return p
}

func TestProjectService_CreateProject(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		svc := NewProjectService(MustOpenDB(t))

		p, err := svc.CreateProject(context.Background(), deploykit.ProjectCreate{Name: "my-app"})
		if err != nil {
			t.Fatal("unexpected error:", err)
		}
		if p.ID == "" {
			t.Fatal("expected non-empty ID")
		}
		if p.Name != "my-app" {
			t.Fatalf("got name %q, want %q", p.Name, "my-app")
		}
		if p.CreatedAt.IsZero() {
			t.Fatal("expected non-zero CreatedAt")
		}
		if p.UpdatedAt.IsZero() {
			t.Fatal("expected non-zero UpdatedAt")
		}
	})

	t.Run("empty name", func(t *testing.T) {
		svc := NewProjectService(MustOpenDB(t))

		_, err := svc.CreateProject(context.Background(), deploykit.ProjectCreate{Name: ""})
		if err == nil {
			t.Fatal("expected error")
		}
		if code := deploykit.ErrorCode(err); code != deploykit.EINVALID {
			t.Fatalf("got error code %q, want %q", code, deploykit.EINVALID)
		}
	})

	// Documents current behavior: no UNIQUE constraint on name in the schema,
	// so duplicate names are allowed. If a UNIQUE index is added, this test
	// should change to expect ECONFLICT on the second create.
	t.Run("duplicate name allowed", func(t *testing.T) {
		svc := NewProjectService(MustOpenDB(t))

		p1, err := svc.CreateProject(context.Background(), deploykit.ProjectCreate{Name: "my-app"})
		if err != nil {
			t.Fatal("first create:", err)
		}

		p2, err := svc.CreateProject(context.Background(), deploykit.ProjectCreate{Name: "my-app"})
		if err != nil {
			t.Fatal("second create:", err)
		}

		if p1.ID == p2.ID {
			t.Fatal("expected different IDs for duplicate names")
		}
	})
}

func TestProjectService_GetProject(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		svc := NewProjectService(MustOpenDB(t))
		created := MustCreateProject(t, svc, "my-app")

		got, err := svc.GetProject(context.Background(), created.ID)
		if err != nil {
			t.Fatal("unexpected error:", err)
		}
		if got.ID != created.ID {
			t.Fatalf("got ID %q, want %q", got.ID, created.ID)
		}
		if got.Name != created.Name {
			t.Fatalf("got name %q, want %q", got.Name, created.Name)
		}
	})

	t.Run("not found", func(t *testing.T) {
		svc := NewProjectService(MustOpenDB(t))

		_, err := svc.GetProject(context.Background(), "nonexistent-id")
		if err == nil {
			t.Fatal("expected error")
		}
		if code := deploykit.ErrorCode(err); code != deploykit.ENOTFOUND {
			t.Fatalf("got error code %q, want %q", code, deploykit.ENOTFOUND)
		}
	})
}

func TestProjectService_ListProjects(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		svc := NewProjectService(MustOpenDB(t))

		projects, count, err := svc.ListProjects(context.Background(), deploykit.ProjectFilter{})
		if err != nil {
			t.Fatal("unexpected error:", err)
		}
		if len(projects) != 0 {
			t.Fatalf("got %d projects, want 0", len(projects))
		}
		if count != 0 {
			t.Fatalf("got count %d, want 0", count)
		}
	})

	t.Run("returns all ordered by created_at desc", func(t *testing.T) {
		svc := NewProjectService(MustOpenDB(t))
		MustCreateProject(t, svc, "first")
		time.Sleep(time.Second) // ensure different created_at timestamps
		MustCreateProject(t, svc, "second")
		time.Sleep(time.Second)
		MustCreateProject(t, svc, "third")

		projects, count, err := svc.ListProjects(context.Background(), deploykit.ProjectFilter{})
		if err != nil {
			t.Fatal("unexpected error:", err)
		}
		if len(projects) != 3 {
			t.Fatalf("got %d projects, want 3", len(projects))
		}
		if count != 3 {
			t.Fatalf("got count %d, want 3", count)
		}
		// Most recent first
		if projects[0].Name != "third" {
			t.Fatalf("got first project %q, want %q", projects[0].Name, "third")
		}
		if projects[2].Name != "first" {
			t.Fatalf("got last project %q, want %q", projects[2].Name, "first")
		}
	})

	t.Run("filter by name", func(t *testing.T) {
		svc := NewProjectService(MustOpenDB(t))
		MustCreateProject(t, svc, "alpha")
		MustCreateProject(t, svc, "beta")
		MustCreateProject(t, svc, "alphabet")

		projects, count, err := svc.ListProjects(context.Background(), deploykit.ProjectFilter{
			Name: stringPtr("alph"),
		})
		if err != nil {
			t.Fatal("unexpected error:", err)
		}
		if len(projects) != 2 {
			t.Fatalf("got %d projects, want 2", len(projects))
		}
		if count != 2 {
			t.Fatalf("got count %d, want 2", count)
		}
	})

	t.Run("filter no match", func(t *testing.T) {
		svc := NewProjectService(MustOpenDB(t))
		MustCreateProject(t, svc, "alpha")

		projects, count, err := svc.ListProjects(context.Background(), deploykit.ProjectFilter{
			Name: stringPtr("zzz"),
		})
		if err != nil {
			t.Fatal("unexpected error:", err)
		}
		if len(projects) != 0 {
			t.Fatalf("got %d projects, want 0", len(projects))
		}
		if count != 0 {
			t.Fatalf("got count %d, want 0", count)
		}
	})

	t.Run("pagination", func(t *testing.T) {
		svc := NewProjectService(MustOpenDB(t))
		MustCreateProject(t, svc, "a")
		MustCreateProject(t, svc, "b")
		MustCreateProject(t, svc, "c")

		projects, count, err := svc.ListProjects(context.Background(), deploykit.ProjectFilter{
			Limit: 2, Offset: 0,
		})
		if err != nil {
			t.Fatal("unexpected error:", err)
		}
		if len(projects) != 2 {
			t.Fatalf("got %d projects, want 2", len(projects))
		}
		if count != 3 {
			t.Fatalf("got count %d, want 3", count)
		}
	})

	t.Run("pagination second page", func(t *testing.T) {
		svc := NewProjectService(MustOpenDB(t))
		MustCreateProject(t, svc, "a")
		MustCreateProject(t, svc, "b")
		MustCreateProject(t, svc, "c")

		projects, count, err := svc.ListProjects(context.Background(), deploykit.ProjectFilter{
			Limit: 2, Offset: 2,
		})
		if err != nil {
			t.Fatal("unexpected error:", err)
		}
		if len(projects) != 1 {
			t.Fatalf("got %d projects, want 1", len(projects))
		}
		if count != 3 {
			t.Fatalf("got count %d, want 3", count)
		}
	})

	t.Run("negative offset treated as zero", func(t *testing.T) {
		svc := NewProjectService(MustOpenDB(t))
		MustCreateProject(t, svc, "a")

		projects, _, err := svc.ListProjects(context.Background(), deploykit.ProjectFilter{
			Offset: -5,
		})
		if err != nil {
			t.Fatal("unexpected error:", err)
		}
		if len(projects) != 1 {
			t.Fatalf("got %d projects, want 1", len(projects))
		}
	})
}

func TestProjectService_UpdateProject(t *testing.T) {
	t.Run("update name", func(t *testing.T) {
		svc := NewProjectService(MustOpenDB(t))
		original := MustCreateProject(t, svc, "old-name")

		updated, err := svc.UpdateProject(context.Background(), original.ID, deploykit.ProjectUpdate{
			Name: stringPtr("new-name"),
		})
		if err != nil {
			t.Fatal("unexpected error:", err)
		}
		if updated.Name != "new-name" {
			t.Fatalf("got name %q, want %q", updated.Name, "new-name")
		}
		if updated.UpdatedAt.Before(original.UpdatedAt) {
			t.Fatal("expected UpdatedAt to not be before original")
		}
	})

	t.Run("no-op nil name", func(t *testing.T) {
		svc := NewProjectService(MustOpenDB(t))
		original := MustCreateProject(t, svc, "keep-me")

		updated, err := svc.UpdateProject(context.Background(), original.ID, deploykit.ProjectUpdate{
			Name: nil,
		})
		if err != nil {
			t.Fatal("unexpected error:", err)
		}
		if updated.Name != "keep-me" {
			t.Fatalf("got name %q, want %q", updated.Name, "keep-me")
		}
	})

	t.Run("empty name", func(t *testing.T) {
		svc := NewProjectService(MustOpenDB(t))
		original := MustCreateProject(t, svc, "x")

		_, err := svc.UpdateProject(context.Background(), original.ID, deploykit.ProjectUpdate{
			Name: stringPtr(""),
		})
		if err == nil {
			t.Fatal("expected error")
		}
		if code := deploykit.ErrorCode(err); code != deploykit.EINVALID {
			t.Fatalf("got error code %q, want %q", code, deploykit.EINVALID)
		}
	})

	t.Run("not found", func(t *testing.T) {
		svc := NewProjectService(MustOpenDB(t))

		_, err := svc.UpdateProject(context.Background(), "nonexistent-id", deploykit.ProjectUpdate{
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

func TestProjectService_DeleteProject(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		svc := NewProjectService(MustOpenDB(t))
		created := MustCreateProject(t, svc, "doomed")

		if err := svc.DeleteProject(context.Background(), created.ID); err != nil {
			t.Fatal("unexpected error:", err)
		}

		// Verify it's actually gone.
		_, err := svc.GetProject(context.Background(), created.ID)
		if err == nil {
			t.Fatal("expected error after deletion")
		}
		if code := deploykit.ErrorCode(err); code != deploykit.ENOTFOUND {
			t.Fatalf("got error code %q, want %q", code, deploykit.ENOTFOUND)
		}
	})

	t.Run("not found", func(t *testing.T) {
		svc := NewProjectService(MustOpenDB(t))

		err := svc.DeleteProject(context.Background(), "nonexistent-id")
		if err == nil {
			t.Fatal("expected error")
		}
		if code := deploykit.ErrorCode(err); code != deploykit.ENOTFOUND {
			t.Fatalf("got error code %q, want %q", code, deploykit.ENOTFOUND)
		}
	})

	t.Run("delete twice", func(t *testing.T) {
		svc := NewProjectService(MustOpenDB(t))
		created := MustCreateProject(t, svc, "doomed")

		if err := svc.DeleteProject(context.Background(), created.ID); err != nil {
			t.Fatal("first delete:", err)
		}

		err := svc.DeleteProject(context.Background(), created.ID)
		if err == nil {
			t.Fatal("expected error on second delete")
		}
		if code := deploykit.ErrorCode(err); code != deploykit.ENOTFOUND {
			t.Fatalf("got error code %q, want %q", code, deploykit.ENOTFOUND)
		}
	})
}
