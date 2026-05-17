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

// Readiness gate constants. While a deployment is in-flight, the per-service
// worker self-requeues every fastTickInterval to drive the readiness gate.
// A starting deployment is promoted only after every replica has been
// running for at least stablePromotionWindow without its restart count
// climbing above the per-replica baseline; a deployment is failed after
// badObservationsToFail consecutive bad observations.
const (
	fastTickInterval      = 2 * time.Second
	stablePromotionWindow = 10 * time.Second
	badObservationsToFail = 3
	logTailLines          = 50
)

// Workqueue configuration.
const (
	defaultWorkers     = 4
	rateLimitBaseDelay = 1 * time.Second
	rateLimitMaxDelay  = 30 * time.Second

	// sweepKey is the single key for the sweep queue. The sweep is a
	// singleton job (networks + orphan detection) so only one is ever
	// pending — per-key serialization in the workqueue does the rest.
	sweepKey = "sweep"
)

// readinessState is the per-replica observation ledger for the readiness gate.
type readinessState struct {
	firstSeenRunningAt   time.Time
	baselineRestartCount int
	consecutiveBadObs    int
	lastRestartCount     int
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

// serviceSnapshot is the per-service state one worker operates against.
// All queries are scoped to svcID so a slow operation in one service's
// reconcile doesn't affect another's.
//
// If snap.service is nil, the service was deleted from the DB — any
// containers still tagged with this svcID are orphans and get torn down.
type serviceSnapshot struct {
	serviceID string
	service   *deploykit.Service
	project   *deploykit.Project

	inFlight        []*deploykit.Deployment
	deploymentsByID map[string]*deploykit.Deployment

	runningContainers       []deploykit.RunningContainer
	actualByKey             map[containerKey]deploykit.RunningContainer
	containerRowsByDockerID map[string][]*deploykit.Container

	desired map[containerKey]desiredContainer

	inFlightOK   bool
	containersOK bool
}

func newServiceSnapshot(svcID string) *serviceSnapshot {
	return &serviceSnapshot{
		serviceID:               svcID,
		deploymentsByID:         map[string]*deploykit.Deployment{},
		actualByKey:             map[containerKey]deploykit.RunningContainer{},
		containerRowsByDockerID: map[string][]*deploykit.Container{},
		desired:                 map[containerKey]desiredContainer{},
	}
}

// sweepSnapshot is the input to the sweep worker: network reconciliation
// inputs plus the cross-service view needed to identify orphan containers
// (those tagged with a serviceID that no longer exists).
type sweepSnapshot struct {
	projects     []*deploykit.Project
	projectsByID map[string]*deploykit.Project
	servicesByID map[string]*deploykit.Service

	networks []string

	runningContainers []deploykit.RunningContainer

	networksOK   bool
	containersOK bool
}

func newSweepSnapshot() *sweepSnapshot {
	return &sweepSnapshot{
		projectsByID: map[string]*deploykit.Project{},
		servicesByID: map[string]*deploykit.Service{},
	}
}

// Reconciler reconciles desired DB state with actual Docker state. Driven
// by a per-key workqueue: each service has its own reconcile invocations,
// serialized per-service but concurrent across services. A single sweep
// job handles networks and orphan detection.
type Reconciler struct {
	projects    deploykit.ProjectService
	services    deploykit.ServiceService
	deployments deploykit.DeploymentService
	containers  deploykit.ContainerService
	provisioner deploykit.Provisioner
	logger      *slog.Logger
	interval    time.Duration
	bus         deploykit.EventBus

	serviceQueue *Queue[string]
	sweepQueue   *Queue[string]

	// trigger asks Run to fire an immediate resync (enqueue all services
	// plus the sweep). Buffered length 1; rapid Triggers coalesce.
	trigger chan struct{}

	// readiness is the shared per-replica observation ledger consulted by
	// stageInspectForService. Multiple workers may inspect different
	// services concurrently, so access is mutex-protected. Keys for one
	// service are touched only by the worker holding that service ID, so
	// contention is brief and rare.
	readinessMu sync.Mutex
	readiness   map[containerKey]*readinessState

	numWorkers int

	// now and stableWindow are test overrides.
	now          func() time.Time
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
		projects:     ps,
		services:     ss,
		deployments:  ds,
		containers:   cs,
		provisioner:  prov,
		logger:       logger,
		interval:     interval,
		bus:          bus,
		serviceQueue: NewQueue[string](rateLimitBaseDelay, rateLimitMaxDelay),
		sweepQueue:   NewQueue[string](rateLimitBaseDelay, rateLimitMaxDelay),
		trigger:      make(chan struct{}, 1),
		readiness:    map[containerKey]*readinessState{},
		numWorkers:   defaultWorkers,
		now:          time.Now,
	}
}

// SetClockForTesting overrides the wall clock the readiness gate consults.
func (r *Reconciler) SetClockForTesting(now func() time.Time) {
	r.now = now
}

// SetStableWindowForTesting overrides the promotion stability window.
// Pass 0 to keep the default (stablePromotionWindow).
func (r *Reconciler) SetStableWindowForTesting(d time.Duration) {
	r.stableWindow = d
}

// SetWorkersForTesting overrides the worker pool size. Must be called
// before Run.
func (r *Reconciler) SetWorkersForTesting(n int) {
	r.numWorkers = n
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

// hasDeploymentStack reports whether the reconciler was wired with the
// services/deployments/containers triad. The network-only construction used
// by the lightweight tests passes nils for these.
func (r *Reconciler) hasDeploymentStack() bool {
	return r.services != nil && r.deployments != nil && r.containers != nil
}

// Run starts the worker pool and the resync ticker. Blocks until ctx is
// cancelled. When ctx is done, the queues are shut down and Run waits for
// workers to drain in-flight items before returning.
func (r *Reconciler) Run(ctx context.Context) {
	r.logger.Info("reconciler started",
		"interval", r.interval, "workers", r.numWorkers)

	var wg sync.WaitGroup
	for i := 0; i < r.numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r.runServiceWorker(ctx)
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		r.runSweepWorker(ctx)
	}()

	r.enqueueResync(ctx)

	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			r.enqueueResync(ctx)
		case <-r.trigger:
			r.enqueueResync(ctx)
		case <-ctx.Done():
			r.logger.Info("reconciler stopping")
			r.serviceQueue.Shutdown()
			r.sweepQueue.Shutdown()
			wg.Wait()
			r.logger.Info("reconciler stopped")
			return
		}
	}
}

// Trigger requests an immediate resync — every service plus the sweep get
// enqueued. Multiple rapid triggers coalesce into one (buffered channel).
func (r *Reconciler) Trigger() {
	select {
	case r.trigger <- struct{}{}:
	default:
	}
}

// TriggerService enqueues a single service for reconciliation. Use this
// when the caller knows exactly which service changed (e.g. after creating
// a deployment) — it skips the full resync.
func (r *Reconciler) TriggerService(svcID string) {
	r.serviceQueue.Add(svcID)
}

// enqueueResync enqueues every existing service plus the sweep job. Called
// on initial start, on ticker tick, and on Trigger. Failures listing
// services are logged but not propagated — the next tick retries.
func (r *Reconciler) enqueueResync(ctx context.Context) {
	r.sweepQueue.Add(sweepKey)
	if !r.hasDeploymentStack() {
		return
	}
	services, err := r.allServices(ctx)
	if err != nil {
		r.logger.Error("failed to list services for resync", "err", err)
		return
	}
	for _, svc := range services {
		r.serviceQueue.Add(svc.ID)
	}
}

// runServiceWorker pulls service IDs and reconciles them. Failures are
// requeued with exponential backoff via AddRateLimited; success calls
// Forget to clear the failure counter.
func (r *Reconciler) runServiceWorker(ctx context.Context) {
	for {
		svcID, shutdown := r.serviceQueue.Get()
		if shutdown {
			return
		}
		err := r.reconcileService(ctx, svcID)
		if err != nil {
			r.logger.Error("reconcile service failed; will retry with backoff",
				"service_id", svcID, "err", err)
			r.serviceQueue.AddRateLimited(svcID)
		} else {
			r.serviceQueue.Forget(svcID)
		}
		r.serviceQueue.Done(svcID)
	}
}

// runSweepWorker drives the singleton sweep job (networks + orphan
// detection). Same retry semantics as service workers.
func (r *Reconciler) runSweepWorker(ctx context.Context) {
	for {
		key, shutdown := r.sweepQueue.Get()
		if shutdown {
			return
		}
		err := r.runSweep(ctx)
		if err != nil {
			r.logger.Error("sweep failed; will retry with backoff", "err", err)
			r.sweepQueue.AddRateLimited(key)
		} else {
			r.sweepQueue.Forget(key)
		}
		r.sweepQueue.Done(key)
	}
}

// ReconcileOnce performs a single synchronous reconciliation pass over
// every service and the sweep. This is the test entry point; production
// uses the worker-driven Run loop. Each call advances readiness gate
// state, so tests can drive the lifecycle by calling ReconcileOnce in a
// loop.
func (r *Reconciler) ReconcileOnce(ctx context.Context) {
	if r.hasDeploymentStack() {
		services, err := r.allServices(ctx)
		if err != nil {
			r.logger.Error("failed to list services", "err", err)
		} else {
			for _, svc := range services {
				if err := r.reconcileService(ctx, svc.ID); err != nil {
					r.logger.Error("reconcile service",
						"service_id", svc.ID, "err", err)
				}
			}
		}
	}
	if err := r.runSweep(ctx); err != nil {
		r.logger.Error("sweep failed", "err", err)
	}
}

// reconcileService runs the per-service pipeline. Returns an error so the
// worker can apply rate-limited retry.
//
// If anything is still in-flight after the pass (i.e. there's a starting
// or healthy deployment that needs ongoing readiness observation), the
// service self-requeues after fastTickInterval. This replaces the previous
// global fast-tick loop.
func (r *Reconciler) reconcileService(ctx context.Context, svcID string) error {
	snap, err := r.loadServiceSnapshot(ctx, svcID)
	if err != nil {
		return err
	}

	r.stageContainersForService(ctx, snap)
	r.stageServiceStatusForService(ctx, snap)
	r.stageInspectForService(ctx, snap)

	if len(snap.inFlight) > 0 {
		r.serviceQueue.AddAfter(svcID, fastTickInterval)
	}
	return nil
}

// loadServiceSnapshot loads every input the per-service reconcile needs.
// Returns an error only if a hard precondition fails (an unexpected DB
// error). A missing service (ENOTFOUND) is not an error — the snapshot is
// returned with svc=nil so the stages can tear down any orphan containers
// that still belong to this svcID.
//
// NOTE: this issues one ListContainers Docker call per service per pass.
// At org scale (dozens of services), that's dozens of concurrent Docker
// calls on resync. If profiling shows this is a hot spot, the right
// optimization is a shared container-list cache with a short TTL or a
// label-filtered list query — both are local changes to this function.
func (r *Reconciler) loadServiceSnapshot(ctx context.Context, svcID string) (*serviceSnapshot, error) {
	snap := newServiceSnapshot(svcID)

	svc, err := r.services.GetService(ctx, svcID)
	if err != nil && !isNotFound(err) {
		return nil, fmt.Errorf("get service: %w", err)
	}
	snap.service = svc

	if svc != nil {
		project, err := r.projects.GetProject(ctx, svc.ProjectID)
		if err != nil && !isNotFound(err) {
			return nil, fmt.Errorf("get project: %w", err)
		}
		snap.project = project
	}

	if err := r.loadServiceInFlight(ctx, snap); err != nil {
		r.logger.Error("failed to list deployments for service",
			"service_id", svcID, "err", err)
	}

	if err := r.loadServiceContainers(ctx, snap); err != nil {
		r.logger.Error("failed to list containers", "err", err)
	}

	if err := r.loadServiceContainerRows(ctx, snap); err != nil {
		r.logger.Error("failed to list container rows",
			"service_id", svcID, "err", err)
	}

	return snap, nil
}

// loadServiceInFlight pages through deployments for this service and
// filters to in-flight statuses (pending/starting/healthy) in memory.
// Pushing the status filter down to SQL is a follow-up.
func (r *Reconciler) loadServiceInFlight(ctx context.Context, snap *serviceSnapshot) error {
	offset := 0
	const pageSize = 100
	svcID := snap.serviceID
	for {
		page, total, err := r.deployments.ListDeployments(ctx, deploykit.DeploymentFilter{
			ServiceID: &svcID,
			Limit:     pageSize,
			Offset:    offset,
		})
		if err != nil {
			return err
		}
		for _, dep := range page {
			if isInFlightStatus(dep.Status) {
				snap.inFlight = append(snap.inFlight, dep)
				snap.deploymentsByID[dep.ID] = dep
			}
		}
		offset += len(page)
		if offset >= total || len(page) == 0 {
			break
		}
	}
	snap.inFlightOK = true
	return nil
}

func isInFlightStatus(status string) bool {
	return status == deploykit.DeploymentStatusPending ||
		status == deploykit.DeploymentStatusStarting ||
		status == deploykit.DeploymentStatusHealthy
}

// loadServiceContainers lists all DeployKit-managed containers from the
// runtime and filters to this service in memory.
func (r *Reconciler) loadServiceContainers(ctx context.Context, snap *serviceSnapshot) error {
	running, err := r.provisioner.ListContainers(ctx)
	if err != nil {
		return err
	}
	for _, rc := range running {
		if rc.ServiceID != snap.serviceID {
			continue
		}
		snap.runningContainers = append(snap.runningContainers, rc)
		if rc.DeploymentID == "" {
			continue
		}
		snap.actualByKey[containerKey{
			serviceID:    rc.ServiceID,
			deploymentID: rc.DeploymentID,
			replicaIndex: rc.ReplicaIndex,
		}] = rc
	}
	snap.containersOK = true
	return nil
}

// loadServiceContainerRows pages the DB container rows for this service
// and indexes them by docker ID, so deleteContainerRow is O(1) lookup
// rather than per-call ListContainers.
func (r *Reconciler) loadServiceContainerRows(ctx context.Context, snap *serviceSnapshot) error {
	offset := 0
	const pageSize = 100
	svcID := snap.serviceID
	for {
		page, total, err := r.containers.ListContainers(ctx, deploykit.ContainerFilter{
			ServiceID: &svcID,
			Limit:     pageSize,
			Offset:    offset,
		})
		if err != nil {
			return err
		}
		for _, row := range page {
			snap.containerRowsByDockerID[row.DockerContainerID] = append(
				snap.containerRowsByDockerID[row.DockerContainerID], row)
		}
		offset += len(page)
		if offset >= total || len(page) == 0 {
			break
		}
	}
	return nil
}

// stageContainersForService builds the desired container set for this
// service, pre-flights images, tears down undesired containers, and
// creates missing ones. No-op if the in-flight or running-container view
// failed to load this cycle — partial state risks duplicate creates.
func (r *Reconciler) stageContainersForService(ctx context.Context, snap *serviceSnapshot) {
	if !snap.inFlightOK || !snap.containersOK {
		return
	}

	// Build the desired set. If service was deleted (snap.service == nil)
	// or its project is gone, desired stays empty and every actual
	// container for this svcID gets torn down below.
	if snap.service != nil && snap.project != nil &&
		snap.service.Status != deploykit.ServiceStatusStopped {
		for _, dep := range snap.inFlight {
			replicas := dep.Replicas
			if replicas <= 0 {
				replicas = 1
			}
			for i := 0; i < replicas; i++ {
				key := containerKey{serviceID: snap.service.ID, deploymentID: dep.ID, replicaIndex: i}
				snap.desired[key] = desiredContainer{
					project: snap.project,
					service: snap.service,
					dep:     dep,
					key:     key,
					spec:    buildSpec(snap.project, snap.service, dep, i),
				}
			}
		}
	}

	// Pre-flight images for pending/starting deployments only. Healthy
	// deployments already have running containers. Image failures are
	// scoped per-deployment so a previous healthy deployment's containers
	// stay desired even if a new deployment's image is bad.
	failedDeployments := map[string]struct{}{}
	imagesChecked := map[string]struct{}{}
	for _, dc := range snap.desired {
		dep := dc.dep
		if dep.Status != deploykit.DeploymentStatusPending && dep.Status != deploykit.DeploymentStatusStarting {
			continue
		}
		if _, ok := failedDeployments[dep.ID]; ok {
			continue
		}
		if _, ok := imagesChecked[dep.Image]; ok {
			continue
		}
		imagesChecked[dep.Image] = struct{}{}
		if err := r.provisioner.EnsureImage(ctx, dep.Image); err != nil {
			r.handleDeploymentError(ctx, dep, fmt.Sprintf("image pull failed: %v", err))
			failedDeployments[dep.ID] = struct{}{}
		}
	}
	for key, dc := range snap.desired {
		if _, bad := failedDeployments[dc.dep.ID]; bad {
			delete(snap.desired, key)
		}
	}

	// Tear down containers no longer desired (superseded/failed/cancelled
	// deployments still in Docker, or orphans whose service was deleted).
	for key, rc := range snap.actualByKey {
		if _, ok := snap.desired[key]; ok {
			continue
		}
		if err := r.provisioner.StopAndRemoveContainer(ctx, rc.DockerID); err != nil {
			r.logger.Error("failed to remove container",
				"docker_id", rc.DockerID, "service_id", rc.ServiceID, "err", err)
			continue
		}
		var projectID string
		if snap.service != nil {
			projectID = snap.service.ProjectID
		}
		r.deleteContainerRow(ctx, snap, rc.DockerID, projectID)
		delete(snap.actualByKey, key)
	}

	// Create missing desired containers.
	for key, dc := range snap.desired {
		if _, ok := snap.actualByKey[key]; ok {
			continue
		}
		// Mark pending → starting on first create attempt for this
		// deployment. The status mutation on dc.dep (shared pointer)
		// gates subsequent replicas.
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
		// Reflect the new container in the snapshot so the same pass's
		// stageInspectForService can see it (required for same-cycle
		// promotion of a fresh deployment).
		snap.actualByKey[key] = deploykit.RunningContainer{
			DockerID:     dockerID,
			Name:         dc.spec.Name,
			ProjectID:    dc.project.ID,
			ServiceID:    dc.service.ID,
			DeploymentID: dc.dep.ID,
			ReplicaIndex: key.replicaIndex,
			State:        "running",
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
}

// stageServiceStatusForService reconciles service.status against the
// active deployment's actual container count. No-op for deleted services,
// stopped services, or services with no active deployment yet.
//
// active_deployment_id is re-read so any flip from a prior fast-tick pass
// (stageInspectForService can promote a deployment) is reflected here.
func (r *Reconciler) stageServiceStatusForService(ctx context.Context, snap *serviceSnapshot) {
	if !snap.inFlightOK || !snap.containersOK || snap.service == nil {
		return
	}
	if snap.service.Status == deploykit.ServiceStatusStopped {
		return
	}

	fresh, err := r.services.GetService(ctx, snap.serviceID)
	if err != nil || fresh == nil {
		return
	}
	if fresh.ActiveDeploymentID == nil {
		return
	}
	activeDepID := *fresh.ActiveDeploymentID
	dep := snap.deploymentsByID[activeDepID]
	if dep == nil {
		return
	}

	want, have := 0, 0
	for key := range snap.desired {
		if key.deploymentID != activeDepID {
			continue
		}
		want++
		if _, ok := snap.actualByKey[key]; ok {
			have++
		}
	}
	if have > want {
		have = want
	}
	if want == 0 {
		return
	}

	var target string
	switch {
	case have == want:
		target = deploykit.ServiceStatusRunning
	case have > 0 && have < want:
		target = deploykit.ServiceStatusDegraded
	default:
		target = deploykit.ServiceStatusDeploying
	}

	if fresh.Status == target {
		return
	}

	oldStatus := fresh.Status
	if err := r.services.SetServiceStatus(ctx, snap.serviceID, target); err != nil {
		r.logger.Error("failed to update service status",
			"service_id", snap.serviceID, "status", target, "err", err)
		return
	}
	r.publish(ctx, deploykit.Event{
		Type:      deploykit.EventServiceStatusChanged,
		ProjectID: snap.service.ProjectID,
		Payload: deploykit.ServiceStatusChangedPayload{
			ServiceID: snap.serviceID,
			OldStatus: oldStatus,
			NewStatus: target,
		},
	})
}

// stageInspectForService is the readiness gate scoped to one service.
// Mutates the shared r.readiness ledger (mutex-protected) and may flip
// dep.Status (in-memory) and service.status (DB) on promotion or
// crashloop. Semantics are identical to the previous global readiness
// gate; only the iteration is scoped.
func (r *Reconciler) stageInspectForService(ctx context.Context, snap *serviceSnapshot) {
	if !snap.inFlightOK || !snap.containersOK || snap.service == nil {
		// Service deleted → garbage-collect any ledger entries for this
		// service so the map doesn't grow unboundedly. Containers were
		// torn down in stageContainersForService.
		r.gcReadinessForService(snap.serviceID, nil)
		return
	}
	if snap.service.Status == deploykit.ServiceStatusStopped {
		return
	}
	if len(snap.inFlight) == 0 {
		r.gcReadinessForService(snap.serviceID, nil)
		return
	}

	now := r.now()
	stableWindow := r.stablePromotionWindowDur()
	live := map[containerKey]struct{}{}

	for _, dep := range snap.inFlight {
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
			key := containerKey{serviceID: snap.service.ID, deploymentID: dep.ID, replicaIndex: i}
			rc, exists := snap.actualByKey[key]
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

			r.readinessMu.Lock()
			st, ok := r.readiness[key]
			if !ok {
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
			r.readinessMu.Unlock()
		}

		switch {
		case dep.Status == deploykit.DeploymentStatusStarting && failed:
			r.handleDeploymentRuntimeError(ctx, dep, failureDockerID, failureInspect, failureReason)
			r.dropReadinessForDeployment(dep.ID)

		case dep.Status == deploykit.DeploymentStatusStarting && readyReplicas == replicas:
			r.promoteDeployment(ctx, snap.service, dep, maxRestartCount)

		case dep.Status == deploykit.DeploymentStatusHealthy && failed:
			r.handleHealthyCrashloop(ctx, snap.service, dep)
		}
	}

	r.gcReadinessForService(snap.serviceID, live)
}

// promoteDeployment marks a starting deployment healthy, supersedes the
// prior active, and flips service.status to running. Extracted from the
// inspect path for readability.
func (r *Reconciler) promoteDeployment(ctx context.Context, svc *deploykit.Service, dep *deploykit.Deployment, baselineRestartCount int) {
	priorActive, err := r.deployments.MarkDeploymentHealthy(ctx, dep.ID, baselineRestartCount)
	if err != nil {
		r.logger.Error("failed to promote deployment to healthy",
			"deployment_id", dep.ID, "err", err)
		return
	}
	dep.Status = deploykit.DeploymentStatusHealthy
	dep.BaselineRestartCount = baselineRestartCount
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
		"superseded", priorActive, "baseline_restart_count", baselineRestartCount)

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
}

// gcReadinessForService removes ledger entries for the given service that
// aren't in the live set. If live is nil, all entries for the service are
// removed (used when the service is deleted or has no in-flight deps).
func (r *Reconciler) gcReadinessForService(svcID string, live map[containerKey]struct{}) {
	r.readinessMu.Lock()
	defer r.readinessMu.Unlock()
	for key := range r.readiness {
		if key.serviceID != svcID {
			continue
		}
		if live == nil {
			delete(r.readiness, key)
			continue
		}
		if _, ok := live[key]; !ok {
			delete(r.readiness, key)
		}
	}
}

// dropReadinessForDeployment removes every ledger entry tied to a
// deployment, used when the deployment just left in-flight (failed).
func (r *Reconciler) dropReadinessForDeployment(depID string) {
	r.readinessMu.Lock()
	defer r.readinessMu.Unlock()
	for key := range r.readiness {
		if key.deploymentID == depID {
			delete(r.readiness, key)
		}
	}
}

// runSweep handles cross-service maintenance: network reconciliation and
// orphan detection. Orphan service IDs (containers tagged with a service
// that no longer exists in the DB) are enqueued on the service queue so
// the existing per-service teardown path cleans them up.
func (r *Reconciler) runSweep(ctx context.Context) error {
	snap, err := r.loadSweepSnapshot(ctx)
	if err != nil {
		return err
	}
	r.stageNetworks(ctx, snap)
	if r.hasDeploymentStack() {
		r.enqueueOrphanServices(snap)
	}
	return nil
}

// loadSweepSnapshot loads inputs the sweep needs: projects + networks for
// network reconciliation, services + containers for orphan detection.
// Returns an error only when project listing fails — every other source
// is optional and degrades gracefully via the *OK flags.
func (r *Reconciler) loadSweepSnapshot(ctx context.Context) (*sweepSnapshot, error) {
	projects, err := r.allProjects(ctx)
	if err != nil {
		return nil, fmt.Errorf("list projects: %w", err)
	}

	snap := newSweepSnapshot()
	snap.projects = projects
	for _, p := range projects {
		snap.projectsByID[p.ID] = p
	}

	if networks, err := r.provisioner.ListNetworks(ctx); err != nil {
		r.logger.Error("failed to list networks", "err", err)
	} else {
		snap.networks = networks
		snap.networksOK = true
	}

	if !r.hasDeploymentStack() {
		return snap, nil
	}

	services, err := r.allServices(ctx)
	if err != nil {
		r.logger.Error("failed to list services for sweep", "err", err)
	} else {
		for _, svc := range services {
			snap.servicesByID[svc.ID] = svc
		}
	}

	if running, err := r.provisioner.ListContainers(ctx); err != nil {
		r.logger.Error("failed to list containers for sweep", "err", err)
	} else {
		snap.runningContainers = running
		snap.containersOK = true
	}

	return snap, nil
}

// stageNetworks ensures one Docker network per project and removes
// orphaned networks.
func (r *Reconciler) stageNetworks(ctx context.Context, snap *sweepSnapshot) {
	if !snap.networksOK {
		return
	}

	actualSet := make(map[string]struct{}, len(snap.networks))
	for _, name := range snap.networks {
		actualSet[name] = struct{}{}
	}

	desiredSet := make(map[string]struct{}, len(snap.projects))
	for _, p := range snap.projects {
		desiredSet[deploykit.NetworkName(p)] = struct{}{}
	}

	for _, p := range snap.projects {
		name := deploykit.NetworkName(p)
		if _, exists := actualSet[name]; exists {
			continue
		}
		if err := r.provisioner.EnsureNetwork(ctx, p); err != nil {
			r.logger.Error("failed to ensure network",
				"network", name, "project_id", p.ID, "err", err)
			continue
		}
	}

	for _, name := range snap.networks {
		if _, desired := desiredSet[name]; desired {
			continue
		}
		if err := r.provisioner.RemoveNetwork(ctx, name); err != nil {
			r.logger.Error("failed to remove orphaned network",
				"network", name, "err", err)
			continue
		}
	}
}

// enqueueOrphanServices finds containers tagged with a service ID that's
// no longer in the services table and enqueues each such service ID for
// the service workers. The workers will see snap.service == nil and tear
// the containers down via the standard teardown path.
func (r *Reconciler) enqueueOrphanServices(snap *sweepSnapshot) {
	if !snap.containersOK {
		return
	}
	enqueued := map[string]struct{}{}
	for _, rc := range snap.runningContainers {
		if rc.ServiceID == "" {
			continue
		}
		if _, exists := snap.servicesByID[rc.ServiceID]; exists {
			continue
		}
		if _, already := enqueued[rc.ServiceID]; already {
			continue
		}
		enqueued[rc.ServiceID] = struct{}{}
		r.serviceQueue.Add(rc.ServiceID)
	}
}

// handleDeploymentError bumps attempt_count and, if we've exhausted retries,
// marks the deployment failed. Used for "container never started" failures
// (image pull, container create) where we have neither an exit code nor
// logs to capture.
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
// has been observed crashlooping or exited. Captures the exit code and
// last logTailLines lines of output as failure context. Bypasses the
// attempt counter — the readiness ledger has already observed
// badObservationsToFail consecutive bad ticks.
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

// handleHealthyCrashloop runs when a deployment that was already promoted
// to healthy starts crashlooping. The deployment status stays at healthy
// (services.active_deployment_id continues to point at it for the proxy
// to decide cutover policy); we flip service.status to degraded or
// failed depending on whether any replica survives.
func (r *Reconciler) handleHealthyCrashloop(ctx context.Context, svc *deploykit.Service, dep *deploykit.Deployment) {
	replicas := dep.Replicas
	if replicas <= 0 {
		replicas = 1
	}

	r.readinessMu.Lock()
	survivors := 0
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
	r.readinessMu.Unlock()

	target := deploykit.ServiceStatusDegraded
	if survivors == 0 {
		target = deploykit.ServiceStatusFailed
	}

	fresh, err := r.services.GetService(ctx, svc.ID)
	if err != nil || fresh == nil {
		return
	}
	if fresh.Status == target {
		r.readinessMu.Lock()
		for i := 0; i < replicas; i++ {
			key := containerKey{serviceID: svc.ID, deploymentID: dep.ID, replicaIndex: i}
			if st := r.readiness[key]; st != nil && st.consecutiveBadObs >= badObservationsToFail {
				st.consecutiveBadObs = 0
			}
		}
		r.readinessMu.Unlock()
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

	r.readinessMu.Lock()
	for i := 0; i < replicas; i++ {
		key := containerKey{serviceID: svc.ID, deploymentID: dep.ID, replicaIndex: i}
		if st := r.readiness[key]; st != nil {
			st.consecutiveBadObs = 0
		}
	}
	r.readinessMu.Unlock()
}

// publishServiceStatusChangedIfFlipped re-fetches the service after a
// MarkDeploymentFailed call and publishes EventServiceStatusChanged if
// the SQL transaction flipped service.status. Without this, the
// frontend's WebSocket cache invalidation never fires.
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
// resolved so canvas subscribers can filter.
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

// describeFailure produces a short, user-facing reason string from an
// inspect snapshot. Used as the failure_reason on the deployment row.
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

// deleteContainerRow removes the DB row(s) for a container identified by
// its runtime ID, using the per-service snapshot's pre-built index.
// Best-effort: errors are logged, not propagated.
func (r *Reconciler) deleteContainerRow(ctx context.Context, snap *serviceSnapshot, dockerID, projectID string) {
	rows, ok := snap.containerRowsByDockerID[dockerID]
	if !ok {
		return
	}
	for _, row := range rows {
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
	delete(snap.containerRowsByDockerID, dockerID)
}

// isNotFound reports whether err is a domain ENOTFOUND error.
func isNotFound(err error) bool {
	if err == nil {
		return false
	}
	return deploykit.ErrorCode(err) == deploykit.ENOTFOUND
}

func (r *Reconciler) allProjects(ctx context.Context) ([]*deploykit.Project, error) {
	var all []*deploykit.Project
	offset := 0
	const pageSize = 100
	for {
		page, total, err := r.projects.ListProjects(ctx, deploykit.ProjectFilter{
			Limit: pageSize, Offset: offset,
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

func (r *Reconciler) allServices(ctx context.Context) ([]*deploykit.Service, error) {
	var all []*deploykit.Service
	offset := 0
	const pageSize = 100
	for {
		page, total, err := r.services.ListServices(ctx, deploykit.ServiceFilter{
			Limit: pageSize, Offset: offset,
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

