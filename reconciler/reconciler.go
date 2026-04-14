package reconciler

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/heyjorgedev/deploykit"
)

// Reconciler periodically reconciles desired state (projects, services,
// deployments in the DB) with actual state (Docker networks and containers)
// and corrects any drift.
type Reconciler struct {
	mu          sync.Mutex
	projects    deploykit.ProjectService
	services    deploykit.ServiceService
	deployments deploykit.DeploymentService
	containers  deploykit.ContainerService
	provisioner deploykit.Provisioner
	logger      *slog.Logger
	interval    time.Duration
	trigger     chan struct{}
}

// New creates a new Reconciler.
func New(
	ps deploykit.ProjectService,
	ss deploykit.ServiceService,
	ds deploykit.DeploymentService,
	cs deploykit.ContainerService,
	prov deploykit.Provisioner,
	logger *slog.Logger,
	interval time.Duration,
) *Reconciler {
	return &Reconciler{
		projects:    ps,
		services:    ss,
		deployments: ds,
		containers:  cs,
		provisioner: prov,
		logger:      logger,
		interval:    interval,
		trigger:     make(chan struct{}, 1),
	}
}

// Run starts the reconciliation loop. It blocks until ctx is cancelled.
func (r *Reconciler) Run(ctx context.Context) {
	r.logger.Info("reconciler started", "interval", r.interval)

	r.ReconcileOnce(ctx)

	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			r.ReconcileOnce(ctx)
		case <-r.trigger:
			r.ReconcileOnce(ctx)
		case <-ctx.Done():
			r.logger.Info("reconciler stopped")
			return
		}
	}
}

// Trigger requests an immediate reconciliation cycle. If a trigger is already
// pending, the call is a no-op (multiple rapid triggers coalesce into one).
func (r *Reconciler) Trigger() {
	select {
	case r.trigger <- struct{}{}:
	default:
	}
}

// ReconcileOnce performs a single reconciliation cycle. It skips if a previous
// cycle is still running.
func (r *Reconciler) ReconcileOnce(ctx context.Context) {
	if !r.mu.TryLock() {
		r.logger.Debug("skipping reconciliation cycle, previous still running")
		return
	}
	defer r.mu.Unlock()

	r.logger.Debug("reconciliation cycle started")

	projects, err := r.allProjects(ctx)
	if err != nil {
		r.logger.Error("failed to list projects", "err", err)
		return
	}

	r.reconcileNetworks(ctx, projects)

	if r.services != nil && r.deployments != nil && r.containers != nil {
		r.reconcileContainers(ctx, projects)
	}

	r.logger.Debug("reconciliation cycle complete", "projects", len(projects))
}

func (r *Reconciler) reconcileNetworks(ctx context.Context, projects []*deploykit.Project) {
	actualNetworks, err := r.provisioner.ListNetworks(ctx)
	if err != nil {
		r.logger.Error("failed to list networks", "err", err)
		return
	}

	actualSet := make(map[string]struct{}, len(actualNetworks))
	for _, name := range actualNetworks {
		actualSet[name] = struct{}{}
	}

	desiredSet := make(map[string]struct{}, len(projects))
	for _, p := range projects {
		desiredSet[deploykit.NetworkName(p)] = struct{}{}
	}

	for _, p := range projects {
		name := deploykit.NetworkName(p)
		if _, exists := actualSet[name]; exists {
			continue
		}
		if err := r.provisioner.EnsureNetwork(ctx, p); err != nil {
			r.logger.Error("failed to ensure network", "network", name, "project_id", p.ID, "err", err)
			continue
		}
	}

	for _, name := range actualNetworks {
		if _, desired := desiredSet[name]; desired {
			continue
		}
		if err := r.provisioner.RemoveNetwork(ctx, name); err != nil {
			r.logger.Error("failed to remove orphaned network", "network", name, "err", err)
			continue
		}
	}
}

// containerKey uniquely identifies a desired or actual container slot.
type containerKey struct {
	serviceID    string
	deploymentID string
	replicaIndex int
}

type desiredContainer struct {
	project *deploykit.Project
	service *deploykit.Service
	dep     *deploykit.Deployment
	spec    deploykit.ContainerSpec
	key     containerKey
}

func (r *Reconciler) reconcileContainers(ctx context.Context, projects []*deploykit.Project) {
	desired := map[containerKey]desiredContainer{}

	for _, p := range projects {
		services, err := r.allServices(ctx, p.ID)
		if err != nil {
			r.logger.Error("failed to list services", "project_id", p.ID, "err", err)
			continue
		}
		for _, svc := range services {
			if svc.Status == deploykit.ServiceStatusStopped {
				continue
			}
			if svc.ActiveDeploymentID == nil {
				continue
			}
			dep, err := r.deployments.GetDeployment(ctx, *svc.ActiveDeploymentID)
			if err != nil {
				r.logger.Error("failed to get active deployment",
					"service_id", svc.ID, "deployment_id", *svc.ActiveDeploymentID, "err", err)
				continue
			}
			replicas := dep.Replicas
			if replicas <= 0 {
				replicas = 1
			}
			for i := 0; i < replicas; i++ {
				key := containerKey{serviceID: svc.ID, deploymentID: dep.ID, replicaIndex: i}
				desired[key] = desiredContainer{
					project: p,
					service: svc,
					dep:     dep,
					key:     key,
					spec:    buildSpec(p, svc, dep, i),
				}
			}
		}
	}

	actual, err := r.provisioner.ListContainers(ctx)
	if err != nil {
		r.logger.Error("failed to list containers", "err", err)
		return
	}

	actualByKey := make(map[containerKey]deploykit.RunningContainer, len(actual))
	for _, rc := range actual {
		if rc.ServiceID == "" || rc.DeploymentID == "" {
			continue
		}
		actualByKey[containerKey{
			serviceID:    rc.ServiceID,
			deploymentID: rc.DeploymentID,
			replicaIndex: rc.ReplicaIndex,
		}] = rc
	}

	// Remove undesired first so a redeploy reusing the same container name
	// doesn't collide on create.
	for key, rc := range actualByKey {
		if _, ok := desired[key]; ok {
			continue
		}
		if err := r.provisioner.StopAndRemoveContainer(ctx, rc.DockerID); err != nil {
			r.logger.Error("failed to remove container",
				"docker_id", rc.DockerID, "service_id", rc.ServiceID, "err", err)
			continue
		}
		r.deleteContainerRow(ctx, rc.DockerID)
	}

	for key, dc := range desired {
		if _, ok := actualByKey[key]; ok {
			continue
		}
		if err := r.provisioner.EnsureImage(ctx, dc.spec.Image); err != nil {
			r.logger.Error("failed to ensure image",
				"image", dc.spec.Image, "service_id", dc.service.ID, "err", err)
			continue
		}
		dockerID, err := r.provisioner.CreateAndStartContainer(ctx, dc.spec)
		if err != nil {
			r.logger.Error("failed to create container",
				"name", dc.spec.Name, "service_id", dc.service.ID, "err", err)
			continue
		}
		if _, err := r.containers.CreateContainer(ctx, deploykit.ContainerCreate{
			ServiceID:         dc.service.ID,
			DeploymentID:      dc.dep.ID,
			DockerContainerID: dockerID,
			Status:            deploykit.ContainerStatusRunning,
		}); err != nil {
			r.logger.Error("failed to record container",
				"docker_id", dockerID, "service_id", dc.service.ID, "err", err)
		}
	}
}

func buildSpec(project *deploykit.Project, svc *deploykit.Service, dep *deploykit.Deployment, replicaIndex int) deploykit.ContainerSpec {
	return deploykit.ContainerSpec{
		Name:        fmt.Sprintf("dk-%s-%s-%d", project.Slug, svc.Name, replicaIndex),
		Image:       dep.Image,
		NetworkName: deploykit.NetworkName(project),
		EnvVars:     dep.EnvVars,
		Ports:       dep.Ports,
		Resources:   dep.Resources,
		Labels: map[string]string{
			deploykit.LabelManagedBy:    deploykit.LabelManagedValue,
			deploykit.LabelProjectID:    project.ID,
			deploykit.LabelServiceID:    svc.ID,
			deploykit.LabelDeploymentID: dep.ID,
			deploykit.LabelReplicaIndex: fmt.Sprintf("%d", replicaIndex),
		},
	}
}

// deleteContainerRow removes the DB row(s) for a container identified by its
// runtime ID. Best-effort: errors are logged, not propagated.
func (r *Reconciler) deleteContainerRow(ctx context.Context, dockerID string) {
	rows, _, err := r.containers.ListContainers(ctx, deploykit.ContainerFilter{Limit: 100})
	if err != nil {
		r.logger.Error("failed to list container rows for cleanup", "err", err)
		return
	}
	for _, row := range rows {
		if row.DockerContainerID != dockerID {
			continue
		}
		if err := r.containers.DeleteContainer(ctx, row.ID); err != nil {
			r.logger.Error("failed to delete container row", "id", row.ID, "err", err)
		}
	}
}

func (r *Reconciler) allProjects(ctx context.Context) ([]*deploykit.Project, error) {
	var all []*deploykit.Project
	offset := 0
	const pageSize = 100

	for {
		page, total, err := r.projects.ListProjects(ctx, deploykit.ProjectFilter{
			Limit:  pageSize,
			Offset: offset,
		})
		if err != nil {
			return nil, err
		}
		all = append(all, page...)
		offset += len(page)
		if offset >= total || len(page) == 0 {
			break
		}
	}
	return all, nil
}

func (r *Reconciler) allServices(ctx context.Context, projectID string) ([]*deploykit.Service, error) {
	var all []*deploykit.Service
	offset := 0
	const pageSize = 100

	for {
		page, total, err := r.services.ListServices(ctx, deploykit.ServiceFilter{
			ProjectID: &projectID,
			Limit:     pageSize,
			Offset:    offset,
		})
		if err != nil {
			return nil, err
		}
		all = append(all, page...)
		offset += len(page)
		if offset >= total || len(page) == 0 {
			break
		}
	}
	return all, nil
}
