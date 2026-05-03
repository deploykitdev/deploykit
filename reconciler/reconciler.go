package reconciler

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/deploykitdev/deploykit"
)

// maxDeploymentAttempts is how many reconcile cycles a pending/starting
// deployment may fail (image pull or container create) before being marked
// failed. The previous healthy deployment keeps serving regardless.
const maxDeploymentAttempts = 3

// Readiness gate constants. The reconciler runs a cheap inspect pass on
// every fast tick and a full reconcile every fullReconcileTicks. A starting
// deployment is promoted only after every replica has been running for at
// least stablePromotionWindow without its restart count climbing above the
// per-replica baseline; a deployment is failed after badObservationsToFail
// consecutive bad observations.
const (
	fastTickInterval      = 2 * time.Second
	stablePromotionWindow = 10 * time.Second
	badObservationsToFail = 3
	logTailLines          = 50
)

// readinessState is the per-replica observation ledger for the readiness gate.
// It lives in memory only — baselineRestartCount is seeded from the persisted
// deployment row on first observation, so the ledger can be rebuilt safely
// after a reconciler restart.
type readinessState struct {
	firstSeenRunningAt   time.Time
	baselineRestartCount int
	consecutiveBadObs    int
	lastRestartCount     int
}

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
	bus         deploykit.EventBus

	// readiness is the per-replica observation ledger consulted by
	// inspectInFlight to decide promotion and crashloop detection.
	readiness map[containerKey]*readinessState

	// now is the wall-clock used by the readiness gate. Tests override it.
	now func() time.Time

	// stableWindow overrides stablePromotionWindow when non-zero (tests).
	stableWindow time.Duration
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
	bus deploykit.EventBus,
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
		bus:         bus,
		readiness:   map[containerKey]*readinessState{},
		now:         time.Now,
	}
}

// SetClockForTesting overrides the wall clock the readiness gate consults.
// Test-only.
func (r *Reconciler) SetClockForTesting(now func() time.Time) {
	r.now = now
}

// SetStableWindowForTesting overrides the promotion stability window. Test-only.
// Pass 0 to keep the default (stablePromotionWindow).
func (r *Reconciler) SetStableWindowForTesting(d time.Duration) {
	r.stableWindow = d
}

// stablePromotionWindowDur returns the active stability window.
func (r *Reconciler) stablePromotionWindowDur() time.Duration {
	if r.stableWindow > 0 {
		return r.stableWindow
	}
	return stablePromotionWindow
}

// publish is a non-blocking helper that emits an event if a bus is attached.
func (r *Reconciler) publish(ctx context.Context, evt deploykit.Event) {
	if r.bus == nil {
		return
	}
	r.bus.Publish(ctx, evt)
}

// Run starts the reconciliation loop. It blocks until ctx is cancelled.
//
// The loop ticks at fastTickInterval. Most ticks run a cheap inspectInFlight
// pass that only consults Docker's container-inspect output to drive the
// readiness gate. Every Nth tick (derived from r.interval) runs a full
// ReconcileOnce that also reconciles networks and creates/tears down
// containers. One ticker, one mutex — fast and full passes never overlap.
func (r *Reconciler) Run(ctx context.Context) {
	fastTick := fastTickInterval
	fullEvery := int(r.interval / fastTick)
	if fullEvery < 1 {
		fullEvery = 1
		if r.interval > 0 && r.interval < fastTick {
			fastTick = r.interval
		}
	}
	r.logger.Info("reconciler started",
		"fast_tick", fastTick, "full_reconcile_every", fullEvery, "interval", r.interval)

	r.ReconcileOnce(ctx)

	ticker := time.NewTicker(fastTick)
	defer ticker.Stop()

	tickCount := 0
	for {
		select {
		case <-ticker.C:
			tickCount++
			if tickCount >= fullEvery {
				tickCount = 0
				r.ReconcileOnce(ctx)
			} else {
				r.inspectInFlight(ctx)
			}
		case <-r.trigger:
			tickCount = 0
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
		r.doInspectInFlight(ctx)
	}

	r.logger.Debug("reconciliation cycle complete", "projects", len(projects))
}

// inspectInFlight is the cheap fast-tick pass: it consults Docker's container
// inspect output for every in-flight deployment's containers and drives the
// readiness gate (promote when stable, fail when crashlooping). Skips if a
// reconcile cycle is already in flight.
func (r *Reconciler) inspectInFlight(ctx context.Context) {
	if !r.mu.TryLock() {
		return
	}
	defer r.mu.Unlock()
	if r.services == nil || r.deployments == nil || r.containers == nil {
		return
	}
	r.doInspectInFlight(ctx)
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
	projectsByID := make(map[string]*deploykit.Project, len(projects))
	for _, p := range projects {
		projectsByID[p.ID] = p
	}

	servicesByID := map[string]*deploykit.Service{}
	for _, p := range projects {
		services, err := r.allServices(ctx, p.ID)
		if err != nil {
			r.logger.Error("failed to list services", "project_id", p.ID, "err", err)
			continue
		}
		for _, svc := range services {
			servicesByID[svc.ID] = svc
		}
	}

	inFlight, err := r.deployments.ListInFlightDeployments(ctx)
	if err != nil {
		r.logger.Error("failed to list in-flight deployments", "err", err)
		return
	}

	// Build the desired set from every in-flight deployment. Keeping the
	// previous healthy deployment in the desired set is what protects its
	// containers from being torn down when a new deployment is being brought up.
	desired := map[containerKey]desiredContainer{}
	deploymentsByID := make(map[string]*deploykit.Deployment, len(inFlight))
	for _, dep := range inFlight {
		deploymentsByID[dep.ID] = dep

		svc := servicesByID[dep.ServiceID]
		if svc == nil {
			continue
		}
		if svc.Status == deploykit.ServiceStatusStopped {
			continue
		}
		project := projectsByID[svc.ProjectID]
		if project == nil {
			continue
		}
		replicas := dep.Replicas
		if replicas <= 0 {
			replicas = 1
		}
		for i := 0; i < replicas; i++ {
			key := containerKey{serviceID: svc.ID, deploymentID: dep.ID, replicaIndex: i}
			desired[key] = desiredContainer{
				project: project,
				service: svc,
				dep:     dep,
				key:     key,
				spec:    buildSpec(project, svc, dep, i),
			}
		}
	}

	// Pre-flight images for pending/starting deployments only. Healthy
	// deployments already have running containers; no need to re-pull.
	// Image failures are scoped per-deployment so the healthy deployment's
	// containers stay desired even if the new deployment's image is bad.
	failedDeployments := map[string]struct{}{}
	imagesChecked := map[string]struct{}{}
	for _, dc := range desired {
		dep := dc.dep
		if dep.Status != deploykit.DeploymentStatusPending && dep.Status != deploykit.DeploymentStatusStarting {
			continue
		}
		if _, ok := failedDeployments[dep.ID]; ok {
			continue
		}
		// Only check each image once per cycle.
		if _, ok := imagesChecked[dep.Image]; ok {
			continue
		}
		imagesChecked[dep.Image] = struct{}{}
		if err := r.provisioner.EnsureImage(ctx, dep.Image); err != nil {
			r.handleDeploymentError(ctx, dep, fmt.Sprintf("image pull failed: %v", err))
			failedDeployments[dep.ID] = struct{}{}
		}
	}
	// Drop the image-failed deployments from this cycle's desired set so we
	// don't try to start them. The previous healthy deployment, if any, is
	// already in `desired` separately and stays.
	for key, dc := range desired {
		if _, bad := failedDeployments[dc.dep.ID]; bad {
			delete(desired, key)
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

	// Tear down containers no longer desired. With the in-flight model, this
	// only catches: superseded/failed/cancelled deployments whose containers
	// are still around, and orphans whose service was deleted.
	for key, rc := range actualByKey {
		if _, ok := desired[key]; ok {
			continue
		}
		if err := r.provisioner.StopAndRemoveContainer(ctx, rc.DockerID); err != nil {
			r.logger.Error("failed to remove container",
				"docker_id", rc.DockerID, "service_id", rc.ServiceID, "err", err)
			continue
		}
		var projectID string
		if svc := servicesByID[rc.ServiceID]; svc != nil {
			projectID = svc.ProjectID
		}
		r.deleteContainerRow(ctx, rc.DockerID, projectID)
	}

	// Create missing desired containers.
	for key, dc := range desired {
		if _, ok := actualByKey[key]; ok {
			continue
		}
		// Mark pending → starting on the first attempt to create a container
		// for this deployment in this cycle. The status mutation on dc.dep
		// (a shared pointer) gates subsequent replicas of the same deployment.
		if dc.dep.Status == deploykit.DeploymentStatusPending {
			if err := r.deployments.MarkDeploymentStarting(ctx, dc.dep.ID); err != nil {
				r.logger.Error("failed to mark deployment starting",
					"deployment_id", dc.dep.ID, "err", err)
			} else {
				dc.dep.Status = deploykit.DeploymentStatusStarting
				r.publish(ctx, deploykit.Event{
					Type:      deploykit.EventDeploymentStarting,
					ProjectID: dc.service.ProjectID,
					Payload: deploykit.DeploymentStatusPayload{
						DeploymentID: dc.dep.ID,
						ServiceID:    dc.service.ID,
						Status:       deploykit.DeploymentStatusStarting,
					},
				})
			}
		}

		dockerID, err := r.provisioner.CreateAndStartContainer(ctx, dc.spec)
		if err != nil {
			r.logger.Error("failed to create container",
				"name", dc.spec.Name, "service_id", dc.service.ID, "err", err)
			r.handleDeploymentError(ctx, dc.dep, fmt.Sprintf("container start failed: %v", err))
			continue
		}
		created, err := r.containers.CreateContainer(ctx, deploykit.ContainerCreate{
			ServiceID:         dc.service.ID,
			DeploymentID:      dc.dep.ID,
			DockerContainerID: dockerID,
			Status:            deploykit.ContainerStatusRunning,
		})
		if err != nil {
			r.logger.Error("failed to record container",
				"docker_id", dockerID, "service_id", dc.service.ID, "err", err)
			continue
		}
		r.publish(ctx, deploykit.Event{
			Type:      deploykit.EventContainerCreated,
			ProjectID: dc.service.ProjectID,
			Payload: deploykit.ContainerCreatedPayload{
				ServiceID:   dc.service.ID,
				ContainerID: created.ID,
				Status:      created.Status,
			},
		})
	}

	r.reconcileServiceStatuses(ctx, servicesByID, desired, actualByKey, deploymentsByID)
}

// handleDeploymentError bumps attempt_count and, if we've exhausted retries,
// marks the deployment failed and publishes EventDeploymentFailed. Used for
// "container never started" failures (image pull, container create) where we
// have neither an exit code nor logs to capture.
func (r *Reconciler) handleDeploymentError(ctx context.Context, dep *deploykit.Deployment, reason string) {
	count, err := r.deployments.IncrementDeploymentAttempt(ctx, dep.ID)
	if err != nil {
		r.logger.Error("failed to increment deployment attempt",
			"deployment_id", dep.ID, "err", err)
		return
	}
	if count < maxDeploymentAttempts {
		r.logger.Warn("deployment attempt failed; will retry",
			"deployment_id", dep.ID, "attempt", count, "reason", reason)
		return
	}
	priorSvc, _ := r.services.GetService(ctx, dep.ServiceID)
	if err := r.deployments.MarkDeploymentFailed(ctx, dep.ID, reason, nil, ""); err != nil {
		r.logger.Error("failed to mark deployment failed",
			"deployment_id", dep.ID, "err", err)
		return
	}
	r.publishDeploymentFailed(ctx, dep, reason, count)
	r.publishServiceStatusChangedIfFlipped(ctx, priorSvc)
}

// handleDeploymentRuntimeError fails a starting deployment whose container
// has been observed crashlooping or exited. Captures the exit code and last
// logTailLines lines of output as failure context. Bypasses the attempt
// counter — by the time we get here the readiness ledger has already
// observed badObservationsToFail consecutive bad ticks.
func (r *Reconciler) handleDeploymentRuntimeError(ctx context.Context, dep *deploykit.Deployment, dockerID string, inspect *deploykit.ContainerInspection, reason string) {
	logTail, err := r.provisioner.GetContainerLogTail(ctx, dockerID, logTailLines)
	if err != nil {
		r.logger.Warn("failed to capture log tail for failed deployment",
			"deployment_id", dep.ID, "docker_id", dockerID, "err", err)
		logTail = ""
	}
	var exitCode *int
	if inspect != nil && inspect.ExitCode != nil {
		ec := *inspect.ExitCode
		exitCode = &ec
	}
	priorSvc, _ := r.services.GetService(ctx, dep.ServiceID)
	if err := r.deployments.MarkDeploymentFailed(ctx, dep.ID, reason, exitCode, logTail); err != nil {
		r.logger.Error("failed to mark deployment failed",
			"deployment_id", dep.ID, "err", err)
		return
	}
	r.publishDeploymentFailed(ctx, dep, reason, dep.AttemptCount)
	r.publishServiceStatusChangedIfFlipped(ctx, priorSvc)
}

// publishServiceStatusChangedIfFlipped re-fetches the service after a
// MarkDeploymentFailed call and publishes EventServiceStatusChanged if the
// SQL transaction flipped service.status (e.g. deploying → failed when no
// healthy fallback exists). Without this, the frontend's WebSocket cache
// invalidation never fires and the user sees a stale "deploying" pill.
func (r *Reconciler) publishServiceStatusChangedIfFlipped(ctx context.Context, prior *deploykit.Service) {
	if prior == nil {
		return
	}
	fresh, err := r.services.GetService(ctx, prior.ID)
	if err != nil || fresh == nil || fresh.Status == prior.Status {
		return
	}
	r.publish(ctx, deploykit.Event{
		Type:      deploykit.EventServiceStatusChanged,
		ProjectID: fresh.ProjectID,
		Payload: deploykit.ServiceStatusChangedPayload{
			ServiceID: fresh.ID,
			OldStatus: prior.Status,
			NewStatus: fresh.Status,
		},
	})
}

// publishDeploymentFailed emits EventDeploymentFailed with the project ID
// resolved so canvas subscribers can filter. The exit code is persisted on the
// deployment row, so the bus payload stays compact.
func (r *Reconciler) publishDeploymentFailed(ctx context.Context, dep *deploykit.Deployment, reason string, attempt int) {
	r.logger.Error("deployment failed",
		"deployment_id", dep.ID, "service_id", dep.ServiceID, "reason", reason)
	var projectID string
	if svc, _ := r.services.GetService(ctx, dep.ServiceID); svc != nil {
		projectID = svc.ProjectID
	}
	reasonCopy := reason
	r.publish(ctx, deploykit.Event{
		Type:      deploykit.EventDeploymentFailed,
		ProjectID: projectID,
		Payload: deploykit.DeploymentStatusPayload{
			DeploymentID:  dep.ID,
			ServiceID:     dep.ServiceID,
			Status:        deploykit.DeploymentStatusFailed,
			FailureReason: &reasonCopy,
			AttemptCount:  attempt,
		},
	})
}

// doInspectInFlight is the readiness gate. For every replica of every
// in-flight deployment, it inspects the container and updates an in-memory
// ledger to decide:
//
//   - starting + stable for stableWindow + RestartCount unchanged → promote
//     (atomically supersede prior healthy + flip active_deployment_id).
//   - starting + exited or RestartCount climbing for badObservationsToFail
//     consecutive ticks → fail (capture exit code + log tail).
//   - healthy + exited or RestartCount climbing for badObservationsToFail
//     consecutive ticks → flip service.status to degraded (or failed if no
//     replica survives). Deployment status is left at healthy so the future
//     proxy can resolve services.active_deployment_id and decide policy.
//
// Caller holds r.mu.
func (r *Reconciler) doInspectInFlight(ctx context.Context) {
	inFlight, err := r.deployments.ListInFlightDeployments(ctx)
	if err != nil {
		r.logger.Error("failed to list in-flight deployments", "err", err)
		return
	}
	if len(inFlight) == 0 {
		// Nothing in flight; clear the ledger so we don't leak.
		if len(r.readiness) > 0 {
			r.readiness = map[containerKey]*readinessState{}
		}
		return
	}

	servicesByID := map[string]*deploykit.Service{}
	for _, dep := range inFlight {
		if _, ok := servicesByID[dep.ServiceID]; ok {
			continue
		}
		svc, err := r.services.GetService(ctx, dep.ServiceID)
		if err != nil || svc == nil {
			continue
		}
		servicesByID[dep.ServiceID] = svc
	}

	actual, err := r.provisioner.ListContainers(ctx)
	if err != nil {
		r.logger.Error("failed to list containers for inspect pass", "err", err)
		return
	}
	actualByKey := map[containerKey]deploykit.RunningContainer{}
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

	now := r.now()
	stableWindow := r.stablePromotionWindowDur()

	// Track which keys are still live this pass so we can GC the ledger.
	live := map[containerKey]struct{}{}

	for _, dep := range inFlight {
		svc := servicesByID[dep.ServiceID]
		if svc == nil {
			continue
		}
		if svc.Status == deploykit.ServiceStatusStopped {
			continue
		}
		replicas := dep.Replicas
		if replicas <= 0 {
			replicas = 1
		}

		readyReplicas := 0
		maxRestartCount := dep.BaselineRestartCount
		failed := false
		var failureReason string
		var failureDockerID string
		var failureInspect *deploykit.ContainerInspection

		for i := 0; i < replicas; i++ {
			key := containerKey{serviceID: svc.ID, deploymentID: dep.ID, replicaIndex: i}
			rc, exists := actualByKey[key]
			if !exists {
				continue
			}
			live[key] = struct{}{}

			inspect, err := r.provisioner.InspectContainer(ctx, rc.DockerID)
			if err != nil {
				r.logger.Warn("inspect failed; will retry next tick",
					"docker_id", rc.DockerID, "deployment_id", dep.ID, "err", err)
				continue
			}

			st, ok := r.readiness[key]
			if !ok {
				// Seed lastRestartCount from the persisted baseline for
				// healthy deployments so a restart that happened between
				// promotion and the first post-restart inspect still
				// registers as "climbed". For starting deployments there's
				// no baseline yet, so seed from the live inspect.
				seedLast := inspect.RestartCount
				if dep.Status == deploykit.DeploymentStatusHealthy {
					seedLast = dep.BaselineRestartCount
				}
				st = &readinessState{
					baselineRestartCount: dep.BaselineRestartCount,
					lastRestartCount:     seedLast,
				}
				if inspect.State == "running" {
					st.firstSeenRunningAt = inspect.StartedAt
				}
				r.readiness[key] = st
			} else if st.firstSeenRunningAt.IsZero() && inspect.State == "running" {
				st.firstSeenRunningAt = inspect.StartedAt
			}

			if inspect.RestartCount > maxRestartCount {
				maxRestartCount = inspect.RestartCount
			}

			// "bad" means crashloop signal: the restart count climbed
			// since the previous observation. For healthy deployments
			// the first observation is seeded from the persisted baseline
			// (so legitimate pre-promotion restarts don't trigger us);
			// for starting deployments it's seeded from the first
			// observation. Either way, "climbed" measures fresh restarts,
			// so a single restart shows up as bad once instead of every
			// tick after.
			climbed := inspect.RestartCount > st.lastRestartCount
			bad := inspect.State == "exited" || inspect.State == "dead" || climbed
			stable := inspect.State == "running" &&
				!st.firstSeenRunningAt.IsZero() &&
				now.Sub(st.firstSeenRunningAt) >= stableWindow

			if bad {
				st.consecutiveBadObs++
				if st.consecutiveBadObs >= badObservationsToFail {
					failed = true
					failureReason = describeFailure(inspect)
					failureDockerID = rc.DockerID
					failureInspect = inspect
				}
			} else {
				st.consecutiveBadObs = 0
				if stable {
					readyReplicas++
				}
			}
			st.lastRestartCount = inspect.RestartCount
		}

		switch {
		case dep.Status == deploykit.DeploymentStatusStarting && failed:
			r.handleDeploymentRuntimeError(ctx, dep, failureDockerID, failureInspect, failureReason)
			// Drop ledger entries for this deployment since it just left in-flight.
			for key := range r.readiness {
				if key.deploymentID == dep.ID {
					delete(r.readiness, key)
				}
			}

		case dep.Status == deploykit.DeploymentStatusStarting && readyReplicas == replicas:
			priorActive, err := r.deployments.MarkDeploymentHealthy(ctx, dep.ID, maxRestartCount)
			if err != nil {
				r.logger.Error("failed to promote deployment to healthy",
					"deployment_id", dep.ID, "err", err)
				continue
			}
			dep.Status = deploykit.DeploymentStatusHealthy
			dep.BaselineRestartCount = maxRestartCount
			r.publish(ctx, deploykit.Event{
				Type:      deploykit.EventDeploymentHealthy,
				ProjectID: svc.ProjectID,
				Payload: deploykit.DeploymentStatusPayload{
					DeploymentID: dep.ID,
					ServiceID:    dep.ServiceID,
					Status:       deploykit.DeploymentStatusHealthy,
				},
			})
			r.logger.Info("deployment healthy",
				"deployment_id", dep.ID, "service_id", dep.ServiceID,
				"superseded", priorActive, "baseline_restart_count", maxRestartCount)

			// Promotion implies all replicas are ready — flip the service to
			// running. Without this the next reconcileServiceStatuses pass
			// would tally containers and might briefly report deploying for
			// the same-cycle case where the container row was just created.
			if svc.Status != deploykit.ServiceStatusRunning {
				old := svc.Status
				if err := r.services.SetServiceStatus(ctx, svc.ID, deploykit.ServiceStatusRunning); err != nil {
					r.logger.Error("failed to set service running after promotion",
						"service_id", svc.ID, "err", err)
				} else {
					svc.Status = deploykit.ServiceStatusRunning
					r.publish(ctx, deploykit.Event{
						Type:      deploykit.EventServiceStatusChanged,
						ProjectID: svc.ProjectID,
						Payload: deploykit.ServiceStatusChangedPayload{
							ServiceID: svc.ID,
							OldStatus: old,
							NewStatus: deploykit.ServiceStatusRunning,
						},
					})
				}
			}

		case dep.Status == deploykit.DeploymentStatusHealthy && failed:
			r.handleHealthyCrashloop(ctx, svc, dep)
		}
	}

	// GC ledger entries for keys that no longer exist on the runtime.
	for key := range r.readiness {
		if _, ok := live[key]; !ok {
			delete(r.readiness, key)
		}
	}
}

// describeFailure produces a short, user-facing reason string from an inspect
// snapshot. Used as the failure_reason on the deployment row.
func describeFailure(inspect *deploykit.ContainerInspection) string {
	if inspect == nil {
		return "container failed runtime checks"
	}
	if inspect.State == "exited" || inspect.State == "dead" {
		if inspect.ExitCode != nil {
			return fmt.Sprintf("container exited with code %d", *inspect.ExitCode)
		}
		return "container exited"
	}
	if inspect.RestartCount > 0 {
		return fmt.Sprintf("container is restarting (restart count %d)", inspect.RestartCount)
	}
	return "container failed runtime checks"
}

// handleHealthyCrashloop runs when a deployment that was already promoted to
// healthy starts crashlooping. The deployment status is intentionally left at
// healthy — services.active_deployment_id continues to point at it so the
// proxy in slice 3 can decide cutover policy. We surface the problem on
// service.status instead (degraded, or failed if no replicas survive).
func (r *Reconciler) handleHealthyCrashloop(ctx context.Context, svc *deploykit.Service, dep *deploykit.Deployment) {
	// Count surviving replicas.
	survivors := 0
	replicas := dep.Replicas
	if replicas <= 0 {
		replicas = 1
	}
	for i := 0; i < replicas; i++ {
		key := containerKey{serviceID: svc.ID, deploymentID: dep.ID, replicaIndex: i}
		st := r.readiness[key]
		if st == nil {
			continue
		}
		if st.consecutiveBadObs == 0 {
			survivors++
		}
	}

	target := deploykit.ServiceStatusDegraded
	if survivors == 0 {
		target = deploykit.ServiceStatusFailed
	}

	fresh, err := r.services.GetService(ctx, svc.ID)
	if err != nil || fresh == nil {
		return
	}
	if fresh.Status == target {
		// Already there; reset bad obs counters so we don't churn the bus.
		for i := 0; i < replicas; i++ {
			key := containerKey{serviceID: svc.ID, deploymentID: dep.ID, replicaIndex: i}
			if st := r.readiness[key]; st != nil && st.consecutiveBadObs >= badObservationsToFail {
				st.consecutiveBadObs = 0
			}
		}
		return
	}
	old := fresh.Status
	if err := r.services.SetServiceStatus(ctx, svc.ID, target); err != nil {
		r.logger.Error("failed to set service status during crashloop",
			"service_id", svc.ID, "target", target, "err", err)
		return
	}
	r.publish(ctx, deploykit.Event{
		Type:      deploykit.EventServiceStatusChanged,
		ProjectID: svc.ProjectID,
		Payload: deploykit.ServiceStatusChangedPayload{
			ServiceID: svc.ID,
			OldStatus: old,
			NewStatus: target,
		},
	})
	r.logger.Warn("healthy deployment crashlooping; service status flipped",
		"service_id", svc.ID, "deployment_id", dep.ID, "target", target)

	// Reset the bad-obs counters so we don't republish the same event every tick.
	for i := 0; i < replicas; i++ {
		key := containerKey{serviceID: svc.ID, deploymentID: dep.ID, replicaIndex: i}
		if st := r.readiness[key]; st != nil {
			st.consecutiveBadObs = 0
		}
	}
}

// reconcileServiceStatuses reconciles service.status against the active
// deployment's actual container count. If a service has no in-flight active
// deployment for the current cycle (e.g. all containers were torn down), the
// status is left unchanged so user-driven `stopped` and CreateDeployment-set
// `deploying` aren't clobbered.
func (r *Reconciler) reconcileServiceStatuses(
	ctx context.Context,
	servicesByID map[string]*deploykit.Service,
	desired map[containerKey]desiredContainer,
	actualByKey map[containerKey]deploykit.RunningContainer,
	deploymentsByID map[string]*deploykit.Deployment,
) {
	type tally struct {
		want, have int
	}
	// Service status reflects the *active* deployment only. We resolve that
	// here from the latest read of services.active_deployment_id (the inspect
	// pass may have just flipped it, so re-fetch).
	for svcID, svc := range servicesByID {
		if svc.Status == deploykit.ServiceStatusStopped {
			continue
		}

		fresh, err := r.services.GetService(ctx, svcID)
		if err != nil || fresh == nil {
			continue
		}
		if fresh.ActiveDeploymentID == nil {
			continue
		}
		activeDepID := *fresh.ActiveDeploymentID
		dep := deploymentsByID[activeDepID]
		if dep == nil {
			continue
		}

		t := tally{}
		for key := range desired {
			if key.serviceID != svcID || key.deploymentID != activeDepID {
				continue
			}
			t.want++
			if _, ok := actualByKey[key]; ok {
				t.have++
			}
		}
		if t.have > t.want {
			t.have = t.want
		}
		if t.want == 0 {
			continue
		}

		var target string
		switch {
		case t.have == t.want:
			target = deploykit.ServiceStatusRunning
		case t.have > 0 && t.have < t.want:
			target = deploykit.ServiceStatusDegraded
		default:
			target = deploykit.ServiceStatusDeploying
		}

		if fresh.Status == target {
			continue
		}

		oldStatus := fresh.Status
		if err := r.services.SetServiceStatus(ctx, svcID, target); err != nil {
			r.logger.Error("failed to update service status",
				"service_id", svcID, "status", target, "err", err)
			continue
		}
		r.publish(ctx, deploykit.Event{
			Type:      deploykit.EventServiceStatusChanged,
			ProjectID: svc.ProjectID,
			Payload: deploykit.ServiceStatusChangedPayload{
				ServiceID: svcID,
				OldStatus: oldStatus,
				NewStatus: target,
			},
		})
	}
}

func buildSpec(project *deploykit.Project, svc *deploykit.Service, dep *deploykit.Deployment, replicaIndex int) deploykit.ContainerSpec {
	return deploykit.ContainerSpec{
		Name:        fmt.Sprintf("dk-%s-%s-%d-%s", project.Slug, svc.Name, replicaIndex, dep.ID[:8]),
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
// runtime ID. Best-effort: errors are logged, not propagated. projectID is
// used when publishing the resulting ContainerDeleted event; pass "" if the
// owning project is unknown (event is then skipped).
func (r *Reconciler) deleteContainerRow(ctx context.Context, dockerID, projectID string) {
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
			continue
		}
		if projectID != "" {
			r.publish(ctx, deploykit.Event{
				Type:      deploykit.EventContainerDeleted,
				ProjectID: projectID,
				Payload: deploykit.ContainerDeletedPayload{
					ServiceID:   row.ServiceID,
					ContainerID: row.ID,
				},
			})
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
