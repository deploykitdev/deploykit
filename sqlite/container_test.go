package sqlite

import (
	"context"
	"testing"

	"github.com/deploykitdev/deploykit"
)

func TestContainerService_CreateContainer(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		db := MustOpenDB(t)
		project := MustCreateProject(t, NewProjectService(db), "p")
		service := MustCreateService(t, db, project.ID, "web")
		deployment := MustCreateDeployment(t, db, service.ID, "nginx:latest")
		svc := NewContainerService(db)

		c, err := svc.CreateContainer(context.Background(), deploykit.ContainerCreate{
			ServiceID:         service.ID,
			DeploymentID:      deployment.ID,
			DockerContainerID: "abc123",
			Status:            deploykit.ContainerStatusRunning,
		})
		if err != nil {
			t.Fatal("unexpected error:", err)
		}
		if c.ID == "" {
			t.Fatal("expected non-empty ID")
		}
		if c.ServiceID != service.ID {
			t.Fatalf("got service_id %q, want %q", c.ServiceID, service.ID)
		}
		if c.DockerContainerID != "abc123" {
			t.Fatalf("got docker_container_id %q, want %q", c.DockerContainerID, "abc123")
		}
		if c.Status != deploykit.ContainerStatusRunning {
			t.Fatalf("got status %q, want %q", c.Status, deploykit.ContainerStatusRunning)
		}
	})

	t.Run("defaults status to created", func(t *testing.T) {
		db := MustOpenDB(t)
		project := MustCreateProject(t, NewProjectService(db), "p")
		service := MustCreateService(t, db, project.ID, "web")
		deployment := MustCreateDeployment(t, db, service.ID, "nginx:latest")
		svc := NewContainerService(db)

		c, err := svc.CreateContainer(context.Background(), deploykit.ContainerCreate{
			ServiceID:         service.ID,
			DeploymentID:      deployment.ID,
			DockerContainerID: "abc123",
		})
		if err != nil {
			t.Fatal("unexpected error:", err)
		}
		if c.Status != deploykit.ContainerStatusCreated {
			t.Fatalf("got status %q, want %q", c.Status, deploykit.ContainerStatusCreated)
		}
	})

	t.Run("validation", func(t *testing.T) {
		db := MustOpenDB(t)
		svc := NewContainerService(db)

		_, err := svc.CreateContainer(context.Background(), deploykit.ContainerCreate{})
		if err == nil {
			t.Fatal("expected error")
		}
		if code := deploykit.ErrorCode(err); code != deploykit.EINVALID {
			t.Fatalf("got error code %q, want %q", code, deploykit.EINVALID)
		}
	})
}

func TestContainerService_ListContainers(t *testing.T) {
	t.Run("filter by service", func(t *testing.T) {
		db := MustOpenDB(t)
		project := MustCreateProject(t, NewProjectService(db), "p")
		s1 := MustCreateService(t, db, project.ID, "web")
		s2 := MustCreateService(t, db, project.ID, "api")
		d1 := MustCreateDeployment(t, db, s1.ID, "nginx:latest")
		d2 := MustCreateDeployment(t, db, s2.ID, "node:latest")

		cs := NewContainerService(db)
		cs.CreateContainer(context.Background(), deploykit.ContainerCreate{
			ServiceID: s1.ID, DeploymentID: d1.ID, DockerContainerID: "c1",
		})
		cs.CreateContainer(context.Background(), deploykit.ContainerCreate{
			ServiceID: s1.ID, DeploymentID: d1.ID, DockerContainerID: "c2",
		})
		cs.CreateContainer(context.Background(), deploykit.ContainerCreate{
			ServiceID: s2.ID, DeploymentID: d2.ID, DockerContainerID: "c3",
		})

		containers, count, err := cs.ListContainers(context.Background(), deploykit.ContainerFilter{
			ServiceID: &s1.ID,
		})
		if err != nil {
			t.Fatal("unexpected error:", err)
		}
		if len(containers) != 2 {
			t.Fatalf("got %d containers, want 2", len(containers))
		}
		if count != 2 {
			t.Fatalf("got count %d, want 2", count)
		}
	})
}

func TestContainerService_UpdateContainerStatus(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		db := MustOpenDB(t)
		project := MustCreateProject(t, NewProjectService(db), "p")
		service := MustCreateService(t, db, project.ID, "web")
		deployment := MustCreateDeployment(t, db, service.ID, "nginx:latest")
		cs := NewContainerService(db)

		c, _ := cs.CreateContainer(context.Background(), deploykit.ContainerCreate{
			ServiceID: service.ID, DeploymentID: deployment.ID, DockerContainerID: "abc",
		})

		updated, err := cs.UpdateContainerStatus(context.Background(), c.ID, deploykit.ContainerStatusRunning)
		if err != nil {
			t.Fatal("unexpected error:", err)
		}
		if updated.Status != deploykit.ContainerStatusRunning {
			t.Fatalf("got status %q, want %q", updated.Status, deploykit.ContainerStatusRunning)
		}
	})

	t.Run("not found", func(t *testing.T) {
		db := MustOpenDB(t)
		cs := NewContainerService(db)

		_, err := cs.UpdateContainerStatus(context.Background(), "nonexistent", deploykit.ContainerStatusRunning)
		if err == nil {
			t.Fatal("expected error")
		}
		if code := deploykit.ErrorCode(err); code != deploykit.ENOTFOUND {
			t.Fatalf("got error code %q, want %q", code, deploykit.ENOTFOUND)
		}
	})
}

func TestContainerService_DeleteContainer(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		db := MustOpenDB(t)
		project := MustCreateProject(t, NewProjectService(db), "p")
		service := MustCreateService(t, db, project.ID, "web")
		deployment := MustCreateDeployment(t, db, service.ID, "nginx:latest")
		cs := NewContainerService(db)

		c, _ := cs.CreateContainer(context.Background(), deploykit.ContainerCreate{
			ServiceID: service.ID, DeploymentID: deployment.ID, DockerContainerID: "abc",
		})

		if err := cs.DeleteContainer(context.Background(), c.ID); err != nil {
			t.Fatal("unexpected error:", err)
		}

		_, err := cs.GetContainer(context.Background(), c.ID)
		if err == nil {
			t.Fatal("expected error after deletion")
		}
	})

	t.Run("not found", func(t *testing.T) {
		db := MustOpenDB(t)
		cs := NewContainerService(db)

		err := cs.DeleteContainer(context.Background(), "nonexistent")
		if err == nil {
			t.Fatal("expected error")
		}
		if code := deploykit.ErrorCode(err); code != deploykit.ENOTFOUND {
			t.Fatalf("got error code %q, want %q", code, deploykit.ENOTFOUND)
		}
	})
}
