package sqlite

import (
	"context"
	"testing"

	"github.com/heyjorgedev/deploykit"
)

// MustCreateService is a test helper that creates a service or fails the test.
func MustCreateService(t *testing.T, db *DB, projectID string, name string) *deploykit.Service {
	t.Helper()
	svc := NewServiceService(db)
	s, err := svc.CreateService(context.Background(), projectID, deploykit.ServiceCreate{Name: name})
	if err != nil {
		t.Fatal("creating seed service:", err)
	}
	return s
}

func TestServiceService_CreateService(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		db := MustOpenDB(t)
		project := MustCreateProject(t, NewProjectService(db), "my-project")
		svc := NewServiceService(db)

		s, err := svc.CreateService(context.Background(), project.ID, deploykit.ServiceCreate{Name: "web"})
		if err != nil {
			t.Fatal("unexpected error:", err)
		}
		if s.ID == "" {
			t.Fatal("expected non-empty ID")
		}
		if s.ProjectID != project.ID {
			t.Fatalf("got project_id %q, want %q", s.ProjectID, project.ID)
		}
		if s.Name != "web" {
			t.Fatalf("got name %q, want %q", s.Name, "web")
		}
		if s.Status != deploykit.ServiceStatusCreated {
			t.Fatalf("got status %q, want %q", s.Status, deploykit.ServiceStatusCreated)
		}
		if s.ActiveDeploymentID != nil {
			t.Fatal("expected nil active_deployment_id")
		}
		if s.CreatedAt.IsZero() {
			t.Fatal("expected non-zero CreatedAt")
		}
	})

	t.Run("empty name", func(t *testing.T) {
		db := MustOpenDB(t)
		project := MustCreateProject(t, NewProjectService(db), "p")
		svc := NewServiceService(db)

		_, err := svc.CreateService(context.Background(), project.ID, deploykit.ServiceCreate{Name: ""})
		if err == nil {
			t.Fatal("expected error")
		}
		if code := deploykit.ErrorCode(err); code != deploykit.EINVALID {
			t.Fatalf("got error code %q, want %q", code, deploykit.EINVALID)
		}
	})

	t.Run("project not found", func(t *testing.T) {
		db := MustOpenDB(t)
		svc := NewServiceService(db)

		_, err := svc.CreateService(context.Background(), "nonexistent", deploykit.ServiceCreate{Name: "web"})
		if err == nil {
			t.Fatal("expected error")
		}
		if code := deploykit.ErrorCode(err); code != deploykit.ENOTFOUND {
			t.Fatalf("got error code %q, want %q", code, deploykit.ENOTFOUND)
		}
	})

	t.Run("duplicate name in same project", func(t *testing.T) {
		db := MustOpenDB(t)
		project := MustCreateProject(t, NewProjectService(db), "p")
		svc := NewServiceService(db)

		_, err := svc.CreateService(context.Background(), project.ID, deploykit.ServiceCreate{Name: "web"})
		if err != nil {
			t.Fatal("first create:", err)
		}

		_, err = svc.CreateService(context.Background(), project.ID, deploykit.ServiceCreate{Name: "web"})
		if err == nil {
			t.Fatal("expected error on duplicate name")
		}
		if code := deploykit.ErrorCode(err); code != deploykit.ECONFLICT {
			t.Fatalf("got error code %q, want %q", code, deploykit.ECONFLICT)
		}
	})

	t.Run("same name in different projects ok", func(t *testing.T) {
		db := MustOpenDB(t)
		ps := NewProjectService(db)
		p1 := MustCreateProject(t, ps, "project-1")
		p2 := MustCreateProject(t, ps, "project-2")
		svc := NewServiceService(db)

		_, err := svc.CreateService(context.Background(), p1.ID, deploykit.ServiceCreate{Name: "web"})
		if err != nil {
			t.Fatal("first create:", err)
		}

		_, err = svc.CreateService(context.Background(), p2.ID, deploykit.ServiceCreate{Name: "web"})
		if err != nil {
			t.Fatal("second create should succeed:", err)
		}
	})
}

func TestServiceService_GetService(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		db := MustOpenDB(t)
		project := MustCreateProject(t, NewProjectService(db), "p")
		created := MustCreateService(t, db, project.ID, "web")

		svc := NewServiceService(db)
		got, err := svc.GetService(context.Background(), created.ID)
		if err != nil {
			t.Fatal("unexpected error:", err)
		}
		if got.ID != created.ID {
			t.Fatalf("got ID %q, want %q", got.ID, created.ID)
		}
		if got.Name != "web" {
			t.Fatalf("got name %q, want %q", got.Name, "web")
		}
	})

	t.Run("not found", func(t *testing.T) {
		db := MustOpenDB(t)
		svc := NewServiceService(db)

		_, err := svc.GetService(context.Background(), "nonexistent")
		if err == nil {
			t.Fatal("expected error")
		}
		if code := deploykit.ErrorCode(err); code != deploykit.ENOTFOUND {
			t.Fatalf("got error code %q, want %q", code, deploykit.ENOTFOUND)
		}
	})
}

func TestServiceService_ListServices(t *testing.T) {
	t.Run("filter by project", func(t *testing.T) {
		db := MustOpenDB(t)
		ps := NewProjectService(db)
		p1 := MustCreateProject(t, ps, "p1")
		p2 := MustCreateProject(t, ps, "p2")
		MustCreateService(t, db, p1.ID, "web")
		MustCreateService(t, db, p1.ID, "db")
		MustCreateService(t, db, p2.ID, "api")

		svc := NewServiceService(db)
		services, count, err := svc.ListServices(context.Background(), deploykit.ServiceFilter{
			ProjectID: &p1.ID,
		})
		if err != nil {
			t.Fatal("unexpected error:", err)
		}
		if len(services) != 2 {
			t.Fatalf("got %d services, want 2", len(services))
		}
		if count != 2 {
			t.Fatalf("got count %d, want 2", count)
		}
	})

	t.Run("pagination", func(t *testing.T) {
		db := MustOpenDB(t)
		project := MustCreateProject(t, NewProjectService(db), "p")
		MustCreateService(t, db, project.ID, "a")
		MustCreateService(t, db, project.ID, "b")
		MustCreateService(t, db, project.ID, "c")

		svc := NewServiceService(db)
		services, count, err := svc.ListServices(context.Background(), deploykit.ServiceFilter{
			ProjectID: &project.ID,
			Limit:     2,
		})
		if err != nil {
			t.Fatal("unexpected error:", err)
		}
		if len(services) != 2 {
			t.Fatalf("got %d services, want 2", len(services))
		}
		if count != 3 {
			t.Fatalf("got count %d, want 3", count)
		}
	})
}

func TestServiceService_UpdateService(t *testing.T) {
	t.Run("update name", func(t *testing.T) {
		db := MustOpenDB(t)
		project := MustCreateProject(t, NewProjectService(db), "p")
		created := MustCreateService(t, db, project.ID, "old-name")

		svc := NewServiceService(db)
		updated, err := svc.UpdateService(context.Background(), created.ID, deploykit.ServiceUpdate{
			Name: stringPtr("new-name"),
		})
		if err != nil {
			t.Fatal("unexpected error:", err)
		}
		if updated.Name != "new-name" {
			t.Fatalf("got name %q, want %q", updated.Name, "new-name")
		}
	})

	t.Run("not found", func(t *testing.T) {
		db := MustOpenDB(t)
		svc := NewServiceService(db)

		_, err := svc.UpdateService(context.Background(), "nonexistent", deploykit.ServiceUpdate{
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

func TestServiceService_DeleteService(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		db := MustOpenDB(t)
		project := MustCreateProject(t, NewProjectService(db), "p")
		created := MustCreateService(t, db, project.ID, "web")

		svc := NewServiceService(db)
		if err := svc.DeleteService(context.Background(), created.ID); err != nil {
			t.Fatal("unexpected error:", err)
		}

		_, err := svc.GetService(context.Background(), created.ID)
		if err == nil {
			t.Fatal("expected error after deletion")
		}
		if code := deploykit.ErrorCode(err); code != deploykit.ENOTFOUND {
			t.Fatalf("got error code %q, want %q", code, deploykit.ENOTFOUND)
		}
	})

	t.Run("not found", func(t *testing.T) {
		db := MustOpenDB(t)
		svc := NewServiceService(db)

		err := svc.DeleteService(context.Background(), "nonexistent")
		if err == nil {
			t.Fatal("expected error")
		}
		if code := deploykit.ErrorCode(err); code != deploykit.ENOTFOUND {
			t.Fatalf("got error code %q, want %q", code, deploykit.ENOTFOUND)
		}
	})
}
