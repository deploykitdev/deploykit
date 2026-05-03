package sqlite

import (
	"context"
	"strings"
	"testing"

	"github.com/deploykitdev/deploykit"
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
		if d.Status != deploykit.DeploymentStatusPending {
			t.Fatalf("got deployment status %q, want %q", d.Status, deploykit.DeploymentStatusPending)
		}

		// First-deploy UX: service status flipped to deploying, but
		// active_deployment_id stays nil — the reconciler flips it once the
		// deployment actually becomes healthy.
		ss := NewServiceService(db)
		updated, err := ss.GetService(context.Background(), service.ID)
		if err != nil {
			t.Fatal("getting updated service:", err)
		}
		if updated.ActiveDeploymentID != nil {
			t.Fatalf("expected active_deployment_id to remain nil until reconciler promotes the deployment, got %v", *updated.ActiveDeploymentID)
		}
		if updated.Status != deploykit.ServiceStatusDeploying {
			t.Fatalf("got status %q, want %q", updated.Status, deploykit.ServiceStatusDeploying)
		}
	})

	t.Run("cancels prior pending deployment", func(t *testing.T) {
		db := MustOpenDB(t)
		project := MustCreateProject(t, NewProjectService(db), "p")
		service := MustCreateService(t, db, project.ID, "web")
		svc := NewDeploymentService(db)

		first, err := svc.CreateDeployment(context.Background(), service.ID, deploykit.DeploymentCreate{Image: "nginx:1"})
		if err != nil {
			t.Fatal(err)
		}
		second, err := svc.CreateDeployment(context.Background(), service.ID, deploykit.DeploymentCreate{Image: "nginx:2"})
		if err != nil {
			t.Fatal(err)
		}

		got, err := svc.GetDeployment(context.Background(), first.ID)
		if err != nil {
			t.Fatal(err)
		}
		if got.Status != deploykit.DeploymentStatusCancelled {
			t.Fatalf("first deployment status: got %q, want %q", got.Status, deploykit.DeploymentStatusCancelled)
		}
		got2, err := svc.GetDeployment(context.Background(), second.ID)
		if err != nil {
			t.Fatal(err)
		}
		if got2.Status != deploykit.DeploymentStatusPending {
			t.Fatalf("second deployment status: got %q, want %q", got2.Status, deploykit.DeploymentStatusPending)
		}
	})

	t.Run("redeploy of running service does not flip service status", func(t *testing.T) {
		db := MustOpenDB(t)
		project := MustCreateProject(t, NewProjectService(db), "p")
		service := MustCreateService(t, db, project.ID, "web")
		svc := NewDeploymentService(db)

		first, err := svc.CreateDeployment(context.Background(), service.ID, deploykit.DeploymentCreate{Image: "nginx:1"})
		if err != nil {
			t.Fatal(err)
		}
		// Simulate the reconciler promoting it to healthy.
		if _, err := svc.MarkDeploymentHealthy(context.Background(), first.ID, 0); err != nil {
			t.Fatal(err)
		}
		ss := NewServiceService(db)
		if err := ss.SetServiceStatus(context.Background(), service.ID, deploykit.ServiceStatusRunning); err != nil {
			t.Fatal(err)
		}

		// New deployment: service should stay "running" (the prior healthy deployment is still serving).
		if _, err := svc.CreateDeployment(context.Background(), service.ID, deploykit.DeploymentCreate{Image: "nginx:2"}); err != nil {
			t.Fatal(err)
		}
		updated, err := ss.GetService(context.Background(), service.ID)
		if err != nil {
			t.Fatal(err)
		}
		if updated.Status != deploykit.ServiceStatusRunning {
			t.Fatalf("got status %q, want %q", updated.Status, deploykit.ServiceStatusRunning)
		}
		if updated.ActiveDeploymentID == nil || *updated.ActiveDeploymentID != first.ID {
			t.Fatalf("active_deployment_id should still point at first deployment until new one is healthy")
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

func TestDeploymentService_MarkDeploymentHealthy_PersistsBaseline(t *testing.T) {
	db := MustOpenDB(t)
	project := MustCreateProject(t, NewProjectService(db), "p")
	service := MustCreateService(t, db, project.ID, "web")
	svc := NewDeploymentService(db)

	dep, err := svc.CreateDeployment(context.Background(), service.ID, deploykit.DeploymentCreate{Image: "nginx:1"})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.MarkDeploymentStarting(context.Background(), dep.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.MarkDeploymentHealthy(context.Background(), dep.ID, 5); err != nil {
		t.Fatal(err)
	}
	got, err := svc.GetDeployment(context.Background(), dep.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.BaselineRestartCount != 5 {
		t.Errorf("baseline_restart_count: got %d want 5", got.BaselineRestartCount)
	}
	if got.Status != deploykit.DeploymentStatusHealthy {
		t.Errorf("status: got %q want healthy", got.Status)
	}
}

func TestDeploymentService_MarkDeploymentFailed_PersistsContext(t *testing.T) {
	t.Run("with exit code and log tail", func(t *testing.T) {
		db := MustOpenDB(t)
		project := MustCreateProject(t, NewProjectService(db), "p")
		service := MustCreateService(t, db, project.ID, "web")
		svc := NewDeploymentService(db)

		dep, err := svc.CreateDeployment(context.Background(), service.ID, deploykit.DeploymentCreate{Image: "broken:1"})
		if err != nil {
			t.Fatal(err)
		}
		exitCode := 137
		logs := "panic: out of memory"
		if err := svc.MarkDeploymentFailed(context.Background(), dep.ID, "OOMKilled", &exitCode, logs); err != nil {
			t.Fatal(err)
		}
		got, err := svc.GetDeployment(context.Background(), dep.ID)
		if err != nil {
			t.Fatal(err)
		}
		if got.Status != deploykit.DeploymentStatusFailed {
			t.Errorf("status: got %q want failed", got.Status)
		}
		if got.ExitCode == nil || *got.ExitCode != 137 {
			t.Errorf("exit_code: got %v want 137", got.ExitCode)
		}
		if got.LogTail == nil || *got.LogTail != logs {
			t.Errorf("log_tail: got %v want %q", got.LogTail, logs)
		}
	})

	t.Run("with nil exit code (image pull failure)", func(t *testing.T) {
		db := MustOpenDB(t)
		project := MustCreateProject(t, NewProjectService(db), "p")
		service := MustCreateService(t, db, project.ID, "web")
		svc := NewDeploymentService(db)

		dep, err := svc.CreateDeployment(context.Background(), service.ID, deploykit.DeploymentCreate{Image: "nginx:nope"})
		if err != nil {
			t.Fatal(err)
		}
		if err := svc.MarkDeploymentFailed(context.Background(), dep.ID, "manifest unknown", nil, ""); err != nil {
			t.Fatal(err)
		}
		got, _ := svc.GetDeployment(context.Background(), dep.ID)
		if got.ExitCode != nil {
			t.Errorf("exit_code should be nil for image-pull failure, got %d", *got.ExitCode)
		}
		if got.LogTail != nil {
			t.Errorf("log_tail should be nil for image-pull failure, got %q", *got.LogTail)
		}
	})

	t.Run("truncates log tail to 10 KB and preserves the tail", func(t *testing.T) {
		db := MustOpenDB(t)
		project := MustCreateProject(t, NewProjectService(db), "p")
		service := MustCreateService(t, db, project.ID, "web")
		svc := NewDeploymentService(db)

		dep, err := svc.CreateDeployment(context.Background(), service.ID, deploykit.DeploymentCreate{Image: "broken:1"})
		if err != nil {
			t.Fatal(err)
		}
		// 50 KB string with a unique panic message at the END so we can assert
		// the truncation kept the tail.
		head := strings.Repeat("boot ", 10000)
		marker := "FATAL ASSERTION at end-of-file\n"
		if err := svc.MarkDeploymentFailed(context.Background(), dep.ID, "killed", nil, head+marker); err != nil {
			t.Fatal(err)
		}
		got, _ := svc.GetDeployment(context.Background(), dep.ID)
		if got.LogTail == nil {
			t.Fatal("log_tail should be populated")
		}
		if len(*got.LogTail) > 10*1024 {
			t.Errorf("log_tail longer than cap: got %d bytes", len(*got.LogTail))
		}
		if !strings.HasSuffix(*got.LogTail, marker) {
			t.Errorf("log_tail should preserve the trailing marker; tail = %q", (*got.LogTail)[len(*got.LogTail)-50:])
		}
	})
}
