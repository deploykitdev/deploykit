package sqlite

import (
	"context"
	"testing"

	"github.com/heyjorgedev/deploykit"
)

// MustCreateDeployment is a test helper that creates a deployment or fails the test.
func MustCreateDeployment(t *testing.T, db *DB, serviceID string, image string) *deploykit.Deployment {
	t.Helper()
	svc := NewDeploymentService(db)
	d, err := svc.CreateDeployment(context.Background(), serviceID, deploykit.DeploymentCreate{Image: image})
	if err != nil {
		t.Fatal("creating seed deployment:", err)
	}
	return d
}

func TestDeploymentService_CreateDeployment(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		db := MustOpenDB(t)
		project := MustCreateProject(t, NewProjectService(db), "p")
		service := MustCreateService(t, db, project.ID, "web")
		svc := NewDeploymentService(db)

		d, err := svc.CreateDeployment(context.Background(), service.ID, deploykit.DeploymentCreate{
			Image: "nginx:latest",
			EnvVars: map[string]string{
				"PORT": "8080",
			},
			Ports: []deploykit.PortMapping{
				{ContainerPort: 80, HostPort: 8080, Protocol: "tcp"},
			},
			Resources: &deploykit.ResourceLimits{
				CPUShares: 1024,
				MemoryMB:  512,
			},
			Replicas: 2,
		})
		if err != nil {
			t.Fatal("unexpected error:", err)
		}
		if d.ID == "" {
			t.Fatal("expected non-empty ID")
		}
		if d.ServiceID != service.ID {
			t.Fatalf("got service_id %q, want %q", d.ServiceID, service.ID)
		}
		if d.Image != "nginx:latest" {
			t.Fatalf("got image %q, want %q", d.Image, "nginx:latest")
		}
		if d.EnvVars["PORT"] != "8080" {
			t.Fatalf("got env PORT %q, want %q", d.EnvVars["PORT"], "8080")
		}
		if len(d.Ports) != 1 || d.Ports[0].ContainerPort != 80 {
			t.Fatalf("got ports %v, want [{80 8080 tcp}]", d.Ports)
		}
		if d.Resources == nil || d.Resources.MemoryMB != 512 {
			t.Fatal("expected resources with 512MB memory")
		}
		if d.Replicas != 2 {
			t.Fatalf("got replicas %d, want 2", d.Replicas)
		}

		// Verify the service was updated.
		ss := NewServiceService(db)
		updated, err := ss.GetService(context.Background(), service.ID)
		if err != nil {
			t.Fatal("getting updated service:", err)
		}
		if updated.ActiveDeploymentID == nil || *updated.ActiveDeploymentID != d.ID {
			t.Fatal("expected active_deployment_id to be set")
		}
		if updated.Status != deploykit.ServiceStatusDeploying {
			t.Fatalf("got status %q, want %q", updated.Status, deploykit.ServiceStatusDeploying)
		}
	})

	t.Run("defaults replicas to 1", func(t *testing.T) {
		db := MustOpenDB(t)
		project := MustCreateProject(t, NewProjectService(db), "p")
		service := MustCreateService(t, db, project.ID, "web")
		svc := NewDeploymentService(db)

		d, err := svc.CreateDeployment(context.Background(), service.ID, deploykit.DeploymentCreate{
			Image: "nginx:latest",
		})
		if err != nil {
			t.Fatal("unexpected error:", err)
		}
		if d.Replicas != 1 {
			t.Fatalf("got replicas %d, want 1", d.Replicas)
		}
	})

	t.Run("empty image", func(t *testing.T) {
		db := MustOpenDB(t)
		project := MustCreateProject(t, NewProjectService(db), "p")
		service := MustCreateService(t, db, project.ID, "web")
		svc := NewDeploymentService(db)

		_, err := svc.CreateDeployment(context.Background(), service.ID, deploykit.DeploymentCreate{
			Image: "",
		})
		if err == nil {
			t.Fatal("expected error")
		}
		if code := deploykit.ErrorCode(err); code != deploykit.EINVALID {
			t.Fatalf("got error code %q, want %q", code, deploykit.EINVALID)
		}
	})

	t.Run("service not found", func(t *testing.T) {
		db := MustOpenDB(t)
		svc := NewDeploymentService(db)

		_, err := svc.CreateDeployment(context.Background(), "nonexistent", deploykit.DeploymentCreate{
			Image: "nginx:latest",
		})
		if err == nil {
			t.Fatal("expected error")
		}
		if code := deploykit.ErrorCode(err); code != deploykit.ENOTFOUND {
			t.Fatalf("got error code %q, want %q", code, deploykit.ENOTFOUND)
		}
	})
}

func TestDeploymentService_GetDeployment(t *testing.T) {
	t.Run("ok with json fields", func(t *testing.T) {
		db := MustOpenDB(t)
		project := MustCreateProject(t, NewProjectService(db), "p")
		service := MustCreateService(t, db, project.ID, "web")
		created := MustCreateDeployment(t, db, service.ID, "nginx:latest")

		svc := NewDeploymentService(db)
		got, err := svc.GetDeployment(context.Background(), created.ID)
		if err != nil {
			t.Fatal("unexpected error:", err)
		}
		if got.ID != created.ID {
			t.Fatalf("got ID %q, want %q", got.ID, created.ID)
		}
		if got.Image != "nginx:latest" {
			t.Fatalf("got image %q, want %q", got.Image, "nginx:latest")
		}
		if got.EnvVars == nil {
			t.Fatal("expected non-nil env_vars")
		}
		if got.Ports == nil {
			t.Fatal("expected non-nil ports")
		}
	})

	t.Run("not found", func(t *testing.T) {
		db := MustOpenDB(t)
		svc := NewDeploymentService(db)

		_, err := svc.GetDeployment(context.Background(), "nonexistent")
		if err == nil {
			t.Fatal("expected error")
		}
		if code := deploykit.ErrorCode(err); code != deploykit.ENOTFOUND {
			t.Fatalf("got error code %q, want %q", code, deploykit.ENOTFOUND)
		}
	})
}

func TestDeploymentService_ListDeployments(t *testing.T) {
	t.Run("returns all for service", func(t *testing.T) {
		db := MustOpenDB(t)
		project := MustCreateProject(t, NewProjectService(db), "p")
		service := MustCreateService(t, db, project.ID, "web")
		MustCreateDeployment(t, db, service.ID, "nginx:1.0")
		MustCreateDeployment(t, db, service.ID, "nginx:2.0")

		svc := NewDeploymentService(db)
		deployments, count, err := svc.ListDeployments(context.Background(), deploykit.DeploymentFilter{
			ServiceID: &service.ID,
		})
		if err != nil {
			t.Fatal("unexpected error:", err)
		}
		if count != 2 {
			t.Fatalf("got count %d, want 2", count)
		}
		if len(deployments) != 2 {
			t.Fatalf("got %d deployments, want 2", len(deployments))
		}
		// Verify both images are present.
		images := map[string]bool{}
		for _, d := range deployments {
			images[d.Image] = true
		}
		if !images["nginx:1.0"] || !images["nginx:2.0"] {
			t.Fatal("expected both deployments to be returned")
		}
	})
}

func TestDeploymentService_RollbackService(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		db := MustOpenDB(t)
		project := MustCreateProject(t, NewProjectService(db), "p")
		service := MustCreateService(t, db, project.ID, "web")
		d1 := MustCreateDeployment(t, db, service.ID, "nginx:1.0")
		MustCreateDeployment(t, db, service.ID, "nginx:2.0")

		svc := NewDeploymentService(db)
		updated, err := svc.RollbackService(context.Background(), service.ID, d1.ID)
		if err != nil {
			t.Fatal("unexpected error:", err)
		}
		if updated.ActiveDeploymentID == nil || *updated.ActiveDeploymentID != d1.ID {
			t.Fatal("expected active_deployment_id to be rolled back")
		}
		if updated.Status != deploykit.ServiceStatusDeploying {
			t.Fatalf("got status %q, want %q", updated.Status, deploykit.ServiceStatusDeploying)
		}
	})

	t.Run("deployment not found", func(t *testing.T) {
		db := MustOpenDB(t)
		project := MustCreateProject(t, NewProjectService(db), "p")
		service := MustCreateService(t, db, project.ID, "web")

		svc := NewDeploymentService(db)
		_, err := svc.RollbackService(context.Background(), service.ID, "nonexistent")
		if err == nil {
			t.Fatal("expected error")
		}
		if code := deploykit.ErrorCode(err); code != deploykit.ENOTFOUND {
			t.Fatalf("got error code %q, want %q", code, deploykit.ENOTFOUND)
		}
	})

	t.Run("deployment belongs to different service", func(t *testing.T) {
		db := MustOpenDB(t)
		project := MustCreateProject(t, NewProjectService(db), "p")
		s1 := MustCreateService(t, db, project.ID, "web")
		s2 := MustCreateService(t, db, project.ID, "api")
		d1 := MustCreateDeployment(t, db, s1.ID, "nginx:1.0")

		svc := NewDeploymentService(db)
		_, err := svc.RollbackService(context.Background(), s2.ID, d1.ID)
		if err == nil {
			t.Fatal("expected error")
		}
		if code := deploykit.ErrorCode(err); code != deploykit.EINVALID {
			t.Fatalf("got error code %q, want %q", code, deploykit.EINVALID)
		}
	})
}
