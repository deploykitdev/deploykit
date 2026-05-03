package reconciler

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/deploykitdev/deploykit"
	"github.com/deploykitdev/deploykit/sqlite"
)

// fakeRuntime is a stand-in for docker.Client used by the lifecycle tests.
// It tracks pulled images, running containers, and lets tests inject pull
// failures per-image to exercise the bad-image path.
type fakeRuntime struct {
	mu sync.Mutex

	imageErrors  map[string]error               // image -> error to return from EnsureImage
	pulledImages map[string]int                 // image -> pull count
	containers   map[string]deploykit.RunningContainer
	createErrors map[string]error               // image -> error from CreateAndStartContainer
	nextID       int

	// inspections, when set for a docker ID, override the default running
	// snapshot returned by InspectContainer. logTails likewise overrides the
	// default empty string returned by GetContainerLogTail.
	inspections map[string]*deploykit.ContainerInspection
	logTails    map[string]string
}

func newFakeRuntime() *fakeRuntime {
	return &fakeRuntime{
		imageErrors:  map[string]error{},
		pulledImages: map[string]int{},
		containers:   map[string]deploykit.RunningContainer{},
		createErrors: map[string]error{},
		inspections:  map[string]*deploykit.ContainerInspection{},
		logTails:     map[string]string{},
	}
}

func (f *fakeRuntime) EnsureNetwork(_ context.Context, _ *deploykit.Project) error {
	return nil
}
func (f *fakeRuntime) RemoveNetwork(context.Context, string) error    { return nil }
func (f *fakeRuntime) ListNetworks(context.Context) ([]string, error) { return nil, nil }

func (f *fakeRuntime) EnsureImage(_ context.Context, image string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.pulledImages[image]++
	if err, ok := f.imageErrors[image]; ok {
		return err
	}
	return nil
}

func (f *fakeRuntime) CreateAndStartContainer(_ context.Context, spec deploykit.ContainerSpec) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err, ok := f.createErrors[spec.Image]; ok {
		return "", err
	}
	f.nextID++
	id := containerDockerID(f.nextID)
	replica := 0
	if r, ok := spec.Labels[deploykit.LabelReplicaIndex]; ok && r != "" {
		// Cheap parse — tests use single-digit replicas.
		replica = int(r[0] - '0')
	}
	f.containers[id] = deploykit.RunningContainer{
		DockerID:     id,
		Name:         spec.Name,
		ProjectID:    spec.Labels[deploykit.LabelProjectID],
		ServiceID:    spec.Labels[deploykit.LabelServiceID],
		DeploymentID: spec.Labels[deploykit.LabelDeploymentID],
		ReplicaIndex: replica,
		State:        "running",
	}
	return id, nil
}

func (f *fakeRuntime) StopAndRemoveContainer(_ context.Context, dockerID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.containers, dockerID)
	return nil
}

func (f *fakeRuntime) ListContainers(context.Context) ([]deploykit.RunningContainer, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]deploykit.RunningContainer, 0, len(f.containers))
	for _, c := range f.containers {
		out = append(out, c)
	}
	return out, nil
}

// InspectContainer returns the override if one was registered for this
// dockerID; otherwise it returns a snapshot reporting State=running with
// StartedAt one hour in the past so the readiness gate's stable-window check
// is permissive by default.
func (f *fakeRuntime) InspectContainer(_ context.Context, dockerID string) (*deploykit.ContainerInspection, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if ins, ok := f.inspections[dockerID]; ok {
		return ins, nil
	}
	return &deploykit.ContainerInspection{
		State:        "running",
		RestartCount: 0,
		StartedAt:    time.Now().Add(-1 * time.Hour),
	}, nil
}

func (f *fakeRuntime) GetContainerLogTail(_ context.Context, dockerID string, _ int) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.logTails[dockerID], nil
}

// setInspection registers an inspect override for a dockerID. Used to
// simulate exited / crashlooping containers in the readiness gate tests.
func (f *fakeRuntime) setInspection(dockerID string, ins *deploykit.ContainerInspection) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.inspections[dockerID] = ins
}

// setLogTail registers a log tail override for a dockerID, used to assert
// the failure-context capture path persists what the runtime returned.
func (f *fakeRuntime) setLogTail(dockerID, logs string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.logTails[dockerID] = logs
}

func containerDockerID(n int) string {
	return "ctr-" + string(rune('a'+n))
}

// recordingBus collects every published event for assertions.
type recordingBus struct {
	mu     sync.Mutex
	events []deploykit.Event
}

func (b *recordingBus) Publish(_ context.Context, evt deploykit.Event) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.events = append(b.events, evt)
}
func (b *recordingBus) Subscribe(int) deploykit.Subscription { return nil }

func (b *recordingBus) typeCount(t deploykit.EventType) int {
	b.mu.Lock()
	defer b.mu.Unlock()
	n := 0
	for _, e := range b.events {
		if e.Type == t {
			n++
		}
	}
	return n
}

// lifecycleHarness wires up an in-memory SQLite stack + fake runtime + reconciler.
type lifecycleHarness struct {
	t        *testing.T
	db       *sqlite.DB
	projects *sqlite.ProjectService
	services *sqlite.ServiceService
	deps     *sqlite.DeploymentService
	conts    *sqlite.ContainerService
	runtime  *fakeRuntime
	bus      *recordingBus
	rec      *Reconciler
	project  *deploykit.Project
}

func newLifecycleHarness(t *testing.T) *lifecycleHarness {
	t.Helper()
	db := sqlite.NewDB(":memory:", slog.Default())
	if err := db.Open(); err != nil {
		t.Fatalf("opening db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	projects := sqlite.NewProjectService(db)
	services := sqlite.NewServiceService(db)
	deps := sqlite.NewDeploymentService(db)
	conts := sqlite.NewContainerService(db)
	runtime := newFakeRuntime()
	bus := &recordingBus{}

	rec := New(projects, services, deps, conts, runtime, slog.Default(), 30*time.Second, bus)

	proj, err := projects.CreateProject(context.Background(), deploykit.ProjectCreate{Name: "demo"})
	if err != nil {
		t.Fatalf("creating project: %v", err)
	}
	return &lifecycleHarness{
		t:        t,
		db:       db,
		projects: projects,
		services: services,
		deps:     deps,
		conts:    conts,
		runtime:  runtime,
		bus:      bus,
		rec:      rec,
		project:  proj,
	}
}

func (h *lifecycleHarness) seedService(name string) *deploykit.Service {
	h.t.Helper()
	svc, err := h.services.CreateService(context.Background(), h.project.ID, deploykit.ServiceCreate{Name: name})
	if err != nil {
		h.t.Fatalf("creating service: %v", err)
	}
	return svc
}

func TestReconcile_GoodDeploy_FlipsActiveOnHealthy(t *testing.T) {
	ctx := context.Background()
	h := newLifecycleHarness(t)
	svc := h.seedService("api")

	dep, err := h.deps.CreateDeployment(ctx, svc.ID, deploykit.DeploymentCreate{Image: "nginx:1"})
	if err != nil {
		t.Fatal(err)
	}

	// Reconcile cycle 1: pull image, start container, mark starting → healthy,
	// flip active_deployment_id, and flip service.status to running in the
	// same cycle (just-created containers count toward the have tally).
	h.rec.ReconcileOnce(ctx)

	got, err := h.deps.GetDeployment(ctx, dep.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != deploykit.DeploymentStatusHealthy {
		t.Fatalf("deployment status: got %q, want %q", got.Status, deploykit.DeploymentStatusHealthy)
	}
	freshSvc, err := h.services.GetService(ctx, svc.ID)
	if err != nil {
		t.Fatal(err)
	}
	if freshSvc.ActiveDeploymentID == nil || *freshSvc.ActiveDeploymentID != dep.ID {
		t.Fatalf("active_deployment_id: got %v, want %s", freshSvc.ActiveDeploymentID, dep.ID)
	}
	if freshSvc.Status != deploykit.ServiceStatusRunning {
		t.Errorf("service status should reach running in the same cycle, got %q", freshSvc.Status)
	}
	if h.bus.typeCount(deploykit.EventDeploymentHealthy) != 1 {
		t.Errorf("expected 1 EventDeploymentHealthy, got %d", h.bus.typeCount(deploykit.EventDeploymentHealthy))
	}
}

func TestReconcile_BadImage_KeepsPriorRunning(t *testing.T) {
	ctx := context.Background()
	h := newLifecycleHarness(t)
	svc := h.seedService("api")

	good, err := h.deps.CreateDeployment(ctx, svc.ID, deploykit.DeploymentCreate{Image: "nginx:1"})
	if err != nil {
		t.Fatal(err)
	}
	// Cycle: bring the good deployment to healthy.
	h.rec.ReconcileOnce(ctx)

	// Now create a deployment with a bad image.
	bad, err := h.deps.CreateDeployment(ctx, svc.ID, deploykit.DeploymentCreate{Image: "nginx:doesnt-exist"})
	if err != nil {
		t.Fatal(err)
	}
	h.runtime.imageErrors["nginx:doesnt-exist"] = errors.New("manifest unknown")

	// Cycle: image pre-flight fails for the bad deployment, but the good one
	// is still in-flight so its container stays.
	h.rec.ReconcileOnce(ctx)

	// Good container still exists in the runtime.
	if len(h.runtime.containers) != 1 {
		t.Fatalf("expected 1 container still running, got %d", len(h.runtime.containers))
	}
	for _, c := range h.runtime.containers {
		if c.DeploymentID != good.ID {
			t.Fatalf("expected container to belong to good deployment %s, got %s", good.ID, c.DeploymentID)
		}
	}

	// Bad deployment's attempt count bumped, still pending (not yet failed —
	// max attempts is 3).
	gotBad, err := h.deps.GetDeployment(ctx, bad.ID)
	if err != nil {
		t.Fatal(err)
	}
	if gotBad.AttemptCount != 1 {
		t.Errorf("attempt_count: got %d, want 1", gotBad.AttemptCount)
	}
	if gotBad.Status == deploykit.DeploymentStatusFailed {
		t.Errorf("deployment should not be failed yet (only 1 attempt)")
	}

	// Service still serving the good deployment.
	freshSvc, err := h.services.GetService(ctx, svc.ID)
	if err != nil {
		t.Fatal(err)
	}
	if freshSvc.ActiveDeploymentID == nil || *freshSvc.ActiveDeploymentID != good.ID {
		t.Fatalf("service should still point at good deployment %s, got %v", good.ID, freshSvc.ActiveDeploymentID)
	}
}

func TestReconcile_BadImage_FailsAfterMaxAttempts(t *testing.T) {
	ctx := context.Background()
	h := newLifecycleHarness(t)
	svc := h.seedService("api")

	bad, err := h.deps.CreateDeployment(ctx, svc.ID, deploykit.DeploymentCreate{Image: "nginx:doesnt-exist"})
	if err != nil {
		t.Fatal(err)
	}
	h.runtime.imageErrors["nginx:doesnt-exist"] = errors.New("manifest unknown")

	// Three failed pulls = deployment marked failed and EventDeploymentFailed published.
	for i := 0; i < maxDeploymentAttempts; i++ {
		h.rec.ReconcileOnce(ctx)
	}

	got, err := h.deps.GetDeployment(ctx, bad.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != deploykit.DeploymentStatusFailed {
		t.Fatalf("deployment status: got %q, want %q", got.Status, deploykit.DeploymentStatusFailed)
	}
	if got.FailureReason == nil || *got.FailureReason == "" {
		t.Error("failure_reason should be set")
	}
	if h.bus.typeCount(deploykit.EventDeploymentFailed) != 1 {
		t.Errorf("expected 1 EventDeploymentFailed, got %d", h.bus.typeCount(deploykit.EventDeploymentFailed))
	}

	// No prior healthy → service status flipped to failed.
	freshSvc, err := h.services.GetService(ctx, svc.ID)
	if err != nil {
		t.Fatal(err)
	}
	if freshSvc.Status != deploykit.ServiceStatusFailed {
		t.Errorf("service status: got %q, want %q", freshSvc.Status, deploykit.ServiceStatusFailed)
	}
}

func TestReconcile_NewHealthy_SupersedesPrior(t *testing.T) {
	ctx := context.Background()
	h := newLifecycleHarness(t)
	svc := h.seedService("api")

	first, err := h.deps.CreateDeployment(ctx, svc.ID, deploykit.DeploymentCreate{Image: "nginx:1"})
	if err != nil {
		t.Fatal(err)
	}
	h.rec.ReconcileOnce(ctx) // first becomes healthy

	second, err := h.deps.CreateDeployment(ctx, svc.ID, deploykit.DeploymentCreate{Image: "nginx:2"})
	if err != nil {
		t.Fatal(err)
	}

	// Cycle 1: second becomes healthy (its container is created), first is
	// superseded, active flips. First's container is still running this cycle
	// because it was in-flight when desired was built.
	h.rec.ReconcileOnce(ctx)

	freshSvc, err := h.services.GetService(ctx, svc.ID)
	if err != nil {
		t.Fatal(err)
	}
	if freshSvc.ActiveDeploymentID == nil || *freshSvc.ActiveDeploymentID != second.ID {
		t.Fatalf("active should have flipped to second %s, got %v", second.ID, freshSvc.ActiveDeploymentID)
	}
	gotFirst, err := h.deps.GetDeployment(ctx, first.ID)
	if err != nil {
		t.Fatal(err)
	}
	if gotFirst.Status != deploykit.DeploymentStatusSuperseded {
		t.Errorf("first deployment should be superseded, got %q", gotFirst.Status)
	}

	// Cycle 2: superseded deployment falls out of in-flight, container torn down.
	h.rec.ReconcileOnce(ctx)
	if len(h.runtime.containers) != 1 {
		t.Fatalf("expected 1 container after teardown, got %d", len(h.runtime.containers))
	}
	for _, c := range h.runtime.containers {
		if c.DeploymentID != second.ID {
			t.Errorf("remaining container should belong to second deployment, got %s", c.DeploymentID)
		}
	}
}

// TestInspectInFlight_StableDeployPromotes covers the happy path of the
// readiness gate: a single replica that's been running long enough with no
// restarts gets promoted to healthy with baseline_restart_count = 0.
func TestInspectInFlight_StableDeployPromotes(t *testing.T) {
	ctx := context.Background()
	h := newLifecycleHarness(t)
	svc := h.seedService("api")

	dep, err := h.deps.CreateDeployment(ctx, svc.ID, deploykit.DeploymentCreate{Image: "nginx:1"})
	if err != nil {
		t.Fatal(err)
	}

	// One ReconcileOnce creates the container; fakeRuntime.InspectContainer
	// returns StartedAt 1h in the past so the stable-window check passes
	// immediately. The same cycle promotes it.
	h.rec.ReconcileOnce(ctx)

	got, err := h.deps.GetDeployment(ctx, dep.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != deploykit.DeploymentStatusHealthy {
		t.Fatalf("status: got %q want healthy", got.Status)
	}
	if got.BaselineRestartCount != 0 {
		t.Errorf("baseline_restart_count: got %d want 0", got.BaselineRestartCount)
	}
}

// TestInspectInFlight_CrashloopFailsStartingDeployment verifies that a
// container exiting immediately is detected via 3 consecutive bad
// observations and the deployment is failed with exit_code + log_tail
// captured from the runtime.
func TestInspectInFlight_CrashloopFailsStartingDeployment(t *testing.T) {
	ctx := context.Background()
	h := newLifecycleHarness(t)
	svc := h.seedService("db")

	// Pre-register the inspection override under the docker ID the next
	// CreateAndStartContainer will assign (fakeRuntime numbers from "ctr-b").
	exitCode := 1
	dockerID := containerDockerID(1)
	h.runtime.setInspection(dockerID, &deploykit.ContainerInspection{
		State:     "exited",
		ExitCode:  &exitCode,
		StartedAt: time.Now().Add(-1 * time.Hour),
	})
	h.runtime.setLogTail(dockerID, "ERROR 1067 (42000): Invalid default value for 'created_at'\n")

	dep, err := h.deps.CreateDeployment(ctx, svc.ID, deploykit.DeploymentCreate{Image: "mysql:8"})
	if err != nil {
		t.Fatal(err)
	}

	// Cycle 1 creates the container; inspect immediately shows "exited" so
	// the readiness gate accumulates one bad observation. Two more cycles
	// should fail the deployment.
	for i := 0; i < badObservationsToFail; i++ {
		h.rec.ReconcileOnce(ctx)
	}

	got, err := h.deps.GetDeployment(ctx, dep.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != deploykit.DeploymentStatusFailed {
		t.Fatalf("status: got %q want failed", got.Status)
	}
	if got.ExitCode == nil || *got.ExitCode != 1 {
		t.Errorf("exit_code: got %v want 1", got.ExitCode)
	}
	if got.LogTail == nil || *got.LogTail == "" {
		t.Errorf("log_tail should be populated")
	}
	if h.bus.typeCount(deploykit.EventDeploymentFailed) == 0 {
		t.Errorf("expected EventDeploymentFailed on bus")
	}
}

// TestInspectInFlight_RestartCountClimbingFailsDeployment covers the case
// where Docker reports the container as running but the restart count keeps
// climbing — Docker's unless-stopped policy masking a crashloop.
func TestInspectInFlight_RestartCountClimbingFailsDeployment(t *testing.T) {
	ctx := context.Background()
	h := newLifecycleHarness(t)
	svc := h.seedService("api")

	// Make the stable window strict so the first cycle's "running" inspect
	// doesn't promote the deployment before we can swap to "restart count
	// climbing" snapshots.
	h.rec.SetStableWindowForTesting(2 * time.Hour)

	dockerID := containerDockerID(1)
	// First observation seeds lastRestartCount; subsequent climbs each count
	// as a bad observation. Need 1 (seed) + badObservationsToFail (climbs)
	// to fail the deployment.
	h.runtime.setInspection(dockerID, &deploykit.ContainerInspection{
		State:        "running",
		RestartCount: 0,
		StartedAt:    time.Now().Add(-30 * time.Minute),
	})

	dep, err := h.deps.CreateDeployment(ctx, svc.ID, deploykit.DeploymentCreate{Image: "broken:1"})
	if err != nil {
		t.Fatal(err)
	}

	for i := 0; i < badObservationsToFail+1; i++ {
		h.runtime.setInspection(dockerID, &deploykit.ContainerInspection{
			State:        "running",
			RestartCount: i,
			StartedAt:    time.Now().Add(-30 * time.Minute),
		})
		h.rec.ReconcileOnce(ctx)
	}

	got, err := h.deps.GetDeployment(ctx, dep.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != deploykit.DeploymentStatusFailed {
		t.Fatalf("status: got %q want failed", got.Status)
	}
}

// TestInspectInFlight_HealthyCrashloopDegradesService verifies that a
// promoted multi-replica deployment whose containers start crashlooping flips
// service.status to degraded (some replicas surviving) without changing
// deployment.status. Single-replica failures flip to failed instead — see
// TestInspectInFlight_HealthyCrashloopFailsSingleReplica.
func TestInspectInFlight_HealthyCrashloopDegradesService(t *testing.T) {
	ctx := context.Background()
	h := newLifecycleHarness(t)
	svc := h.seedService("api")

	dep, err := h.deps.CreateDeployment(ctx, svc.ID, deploykit.DeploymentCreate{
		Image:    "nginx:1",
		Replicas: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	h.rec.ReconcileOnce(ctx) // promotes both replicas

	got, _ := h.deps.GetDeployment(ctx, dep.ID)
	if got.Status != deploykit.DeploymentStatusHealthy {
		t.Fatalf("preconditions: deployment should be healthy, got %q", got.Status)
	}

	// Identify replica 0's docker ID. fakeRuntime numbers IDs sequentially;
	// replica 0 is the first container created.
	replica0ID := containerDockerID(1)

	// One replica starts crashlooping while the other stays healthy. Service
	// should flip to degraded, not failed.
	for i := 0; i < badObservationsToFail; i++ {
		h.runtime.setInspection(replica0ID, &deploykit.ContainerInspection{
			State:        "running",
			RestartCount: i + 1,
			StartedAt:    time.Now().Add(-1 * time.Hour),
		})
		h.rec.ReconcileOnce(ctx)
	}

	freshSvc, err := h.services.GetService(ctx, svc.ID)
	if err != nil {
		t.Fatal(err)
	}
	if freshSvc.Status != deploykit.ServiceStatusDegraded {
		t.Errorf("service.status: got %q want degraded", freshSvc.Status)
	}
	freshDep, _ := h.deps.GetDeployment(ctx, dep.ID)
	if freshDep.Status != deploykit.DeploymentStatusHealthy {
		t.Errorf("deployment.status should stay healthy, got %q", freshDep.Status)
	}
}

// TestInspectInFlight_HealthyCrashloopFailsSingleReplica covers the
// "deployment had 1 replica, that replica is now crashlooping" case —
// service.status flips to failed because no replicas survive.
func TestInspectInFlight_HealthyCrashloopFailsSingleReplica(t *testing.T) {
	ctx := context.Background()
	h := newLifecycleHarness(t)
	svc := h.seedService("api")

	if _, err := h.deps.CreateDeployment(ctx, svc.ID, deploykit.DeploymentCreate{Image: "nginx:1"}); err != nil {
		t.Fatal(err)
	}
	h.rec.ReconcileOnce(ctx)

	dockerID := containerDockerID(1)
	for i := 0; i < badObservationsToFail; i++ {
		h.runtime.setInspection(dockerID, &deploykit.ContainerInspection{
			State:        "running",
			RestartCount: i + 1,
			StartedAt:    time.Now().Add(-1 * time.Hour),
		})
		h.rec.ReconcileOnce(ctx)
	}

	freshSvc, _ := h.services.GetService(ctx, svc.ID)
	if freshSvc.Status != deploykit.ServiceStatusFailed {
		t.Errorf("service.status: got %q want failed", freshSvc.Status)
	}
}

// TestInspectInFlight_BaselineSurvivesReconcilerRestart checks the
// in-memory + persisted hybrid: a healthy deployment with
// baseline_restart_count=3 (legitimate restarts before promotion) survives
// a reconciler restart without being false-flagged when Docker still
// reports RestartCount=3.
func TestInspectInFlight_BaselineSurvivesReconcilerRestart(t *testing.T) {
	ctx := context.Background()
	h := newLifecycleHarness(t)
	svc := h.seedService("api")

	// Pre-seed an inspection that reports RestartCount=3 BEFORE promotion,
	// simulating a container that legitimately restarted a few times during
	// startup (e.g. transient daemon hiccup). The promotion path stamps
	// baseline_restart_count = max RestartCount across replicas.
	dockerID := containerDockerID(1)
	h.runtime.setInspection(dockerID, &deploykit.ContainerInspection{
		State:        "running",
		RestartCount: 3,
		StartedAt:    time.Now().Add(-1 * time.Hour),
	})

	dep, err := h.deps.CreateDeployment(ctx, svc.ID, deploykit.DeploymentCreate{Image: "nginx:1"})
	if err != nil {
		t.Fatal(err)
	}
	h.rec.ReconcileOnce(ctx) // create + promote with baseline 3

	gotDep, err := h.deps.GetDeployment(ctx, dep.ID)
	if err != nil {
		t.Fatal(err)
	}
	if gotDep.BaselineRestartCount != 3 {
		t.Fatalf("baseline_restart_count: got %d want 3", gotDep.BaselineRestartCount)
	}

	// New Reconciler with empty in-memory readiness map (simulates restart).
	rec2 := New(h.projects, h.services, h.deps, h.conts, h.runtime, slog.Default(), 30*time.Second, h.bus)

	// Inspect at RestartCount=3 should NOT trigger crashloop detection.
	h.runtime.setInspection(dockerID, &deploykit.ContainerInspection{
		State:        "running",
		RestartCount: 3,
		StartedAt:    time.Now().Add(-1 * time.Hour),
	})
	for i := 0; i < badObservationsToFail+1; i++ {
		rec2.ReconcileOnce(ctx)
	}
	freshSvc, _ := h.services.GetService(ctx, svc.ID)
	if freshSvc.Status == deploykit.ServiceStatusDegraded {
		t.Errorf("service should not be flagged degraded when restart count == baseline")
	}
	gotDep, _ = h.deps.GetDeployment(ctx, dep.ID)
	if gotDep.Status != deploykit.DeploymentStatusHealthy {
		t.Errorf("deployment status should stay healthy, got %q", gotDep.Status)
	}

	// Restart count climbing above the baseline on each tick → 3 consecutive
	// bad obs flips the service. With a single replica, no replicas survive,
	// so the service flips to failed (not degraded — that path is covered by
	// TestInspectInFlight_HealthyCrashloopDegradesService). Bumping per tick
	// (instead of holding RestartCount=4) reflects what a real crashloop looks
	// like; a single restart should not cascade into a failure.
	for i := 0; i < badObservationsToFail; i++ {
		h.runtime.setInspection(dockerID, &deploykit.ContainerInspection{
			State:        "running",
			RestartCount: 4 + i,
			StartedAt:    time.Now().Add(-1 * time.Hour),
		})
		rec2.ReconcileOnce(ctx)
	}
	freshSvc, _ = h.services.GetService(ctx, svc.ID)
	if freshSvc.Status != deploykit.ServiceStatusFailed {
		t.Errorf("service.status: got %q want failed after baseline exceeded", freshSvc.Status)
	}
}
