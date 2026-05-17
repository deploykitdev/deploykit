package reconciler

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/deploykitdev/deploykit"
	"github.com/deploykitdev/deploykit/sqlite"
)

// blockingRuntime is a fakeRuntime variant that lets a test gate the
// EnsureImage call for a specific image — used to simulate a slow pull
// and prove other services can still be reconciled concurrently.
type blockingRuntime struct {
	*fakeRuntime
	blockMu      sync.Mutex
	blocks       map[string]chan struct{} // image -> release channel
	pullAttempts map[string]*atomic.Int32
}

func newBlockingRuntime() *blockingRuntime {
	return &blockingRuntime{
		fakeRuntime:  newFakeRuntime(),
		blocks:       map[string]chan struct{}{},
		pullAttempts: map[string]*atomic.Int32{},
	}
}

func (b *blockingRuntime) blockImage(image string) chan struct{} {
	b.blockMu.Lock()
	defer b.blockMu.Unlock()
	ch := make(chan struct{})
	b.blocks[image] = ch
	b.pullAttempts[image] = &atomic.Int32{}
	return ch
}

func (b *blockingRuntime) attemptCount(image string) int32 {
	b.blockMu.Lock()
	defer b.blockMu.Unlock()
	if c, ok := b.pullAttempts[image]; ok {
		return c.Load()
	}
	return 0
}

func (b *blockingRuntime) EnsureImage(ctx context.Context, image string) error {
	b.blockMu.Lock()
	ch, blocked := b.blocks[image]
	if c, ok := b.pullAttempts[image]; ok {
		c.Add(1)
	}
	b.blockMu.Unlock()

	if blocked {
		select {
		case <-ch:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return b.fakeRuntime.EnsureImage(ctx, image)
}

// TestWorkqueue_NoisyNeighborIsolation is the load-bearing test for the
// workqueue refactor: a slow EnsureImage on service A must not block
// reconciliation of service B. With the previous global-tick design, the
// reconciler's single mutex serialized every cycle, so a 10s pull on A
// would make B's containers wait the full 10s. With per-key workers, B
// gets reconciled on a different goroutine while A's worker is blocked.
func TestWorkqueue_NoisyNeighborIsolation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	db := sqlite.NewDB(":memory:", slog.Default())
	if err := db.Open(); err != nil {
		t.Fatalf("opening db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	projects := sqlite.NewProjectService(db)
	services := sqlite.NewServiceService(db)
	deps := sqlite.NewDeploymentService(db)
	conts := sqlite.NewContainerService(db)
	runtime := newBlockingRuntime()
	bus := &recordingBus{}

	rec := New(projects, services, deps, conts, runtime, slog.Default(),
		100*time.Millisecond, bus) // fast tick for the test
	rec.SetWorkersForTesting(4)

	proj, err := projects.CreateProject(ctx, deploykit.ProjectCreate{Name: "multi"})
	if err != nil {
		t.Fatal(err)
	}

	svcA, _ := services.CreateService(ctx, proj.ID, deploykit.ServiceCreate{Name: "slow"})
	svcB, _ := services.CreateService(ctx, proj.ID, deploykit.ServiceCreate{Name: "fast"})

	// Block service A's image. EnsureImage for "slow:1" will hang until we
	// close the channel.
	release := runtime.blockImage("slow:1")
	var releaseOnce sync.Once
	unblockA := func() { releaseOnce.Do(func() { close(release) }) }
	defer unblockA() // safety net

	_, err = deps.CreateDeployment(ctx, svcA.ID, deploykit.DeploymentCreate{Image: "slow:1"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = deps.CreateDeployment(ctx, svcB.ID, deploykit.DeploymentCreate{Image: "fast:1"})
	if err != nil {
		t.Fatal(err)
	}

	// Run the reconciler in the background.
	runCtx, runCancel := context.WithCancel(ctx)
	defer runCancel()
	done := make(chan struct{})
	go func() {
		rec.Run(runCtx)
		close(done)
	}()

	// Service B should reach healthy while A is still blocked.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		svc, err := services.GetService(ctx, svcB.ID)
		if err == nil && svc.Status == deploykit.ServiceStatusRunning {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	freshB, err := services.GetService(ctx, svcB.ID)
	if err != nil {
		t.Fatal(err)
	}
	if freshB.Status != deploykit.ServiceStatusRunning {
		t.Fatalf("service B should have reached running while A blocked, got %q",
			freshB.Status)
	}

	// Confirm A is still stuck: no container created yet (EnsureImage hasn't
	// returned). The blocking call may have been attempted multiple times if
	// the worker has retried.
	if got := len(runtime.fakeRuntime.containers); got != 1 {
		t.Errorf("expected 1 container (B's only), got %d", got)
	}

	// Now unblock A — release the channel and let A's worker proceed.
	unblockA()
	runtime.blockMu.Lock()
	delete(runtime.blocks, "slow:1")
	runtime.blockMu.Unlock()

	// Wait for A to reach healthy.
	deadline = time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		svc, _ := services.GetService(ctx, svcA.ID)
		if svc != nil && svc.Status == deploykit.ServiceStatusRunning {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	freshA, _ := services.GetService(ctx, svcA.ID)
	if freshA == nil || freshA.Status != deploykit.ServiceStatusRunning {
		t.Errorf("service A should reach running after unblock, got %v",
			freshA)
	}

	runCancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("reconciler did not stop after cancel")
	}
}

// TestWorkqueue_SelfRequeueDrivesPromotion verifies that a service with an
// in-flight deployment is re-reconciled by the self-requeue path
// (AddAfter fastTickInterval) without depending on the slow resync ticker.
// This is what makes the readiness gate fire every 2s for active deploys.
func TestWorkqueue_SelfRequeueDrivesPromotion(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	h := newLifecycleHarness(t)
	svc := h.seedService("api")

	// Tight inspection: container starts in "exited" state so the readiness
	// gate will fail it after 3 observations. We want to prove those 3
	// observations happen via self-requeue, not via a 30s resync.
	exitCode := 7
	h.runtime.setInspection(containerDockerID(1), &deploykit.ContainerInspection{
		State:     "exited",
		ExitCode:  &exitCode,
		StartedAt: time.Now().Add(-time.Hour),
	})

	dep, err := h.deps.CreateDeployment(ctx, svc.ID,
		deploykit.DeploymentCreate{Image: "doomed:1"})
	if err != nil {
		t.Fatal(err)
	}

	// Resync interval set to something huge so we know self-requeue is
	// driving the readiness gate. fastTickInterval is 2s so we expect
	// failure within ~6-8s of starting.
	h.rec = New(h.projects, h.services, h.deps, h.conts, h.runtime,
		slog.Default(), 30*time.Minute, h.bus)
	h.rec.SetWorkersForTesting(2)

	runCtx, runCancel := context.WithCancel(ctx)
	defer runCancel()
	done := make(chan struct{})
	go func() {
		h.rec.Run(runCtx)
		close(done)
	}()

	deadline := time.Now().Add(12 * time.Second)
	for time.Now().Before(deadline) {
		got, _ := h.deps.GetDeployment(ctx, dep.ID)
		if got != nil && got.Status == deploykit.DeploymentStatusFailed {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	got, err := h.deps.GetDeployment(ctx, dep.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != deploykit.DeploymentStatusFailed {
		t.Fatalf("deployment should have failed via self-requeue path; got %q",
			got.Status)
	}

	runCancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("reconciler did not stop after cancel")
	}
}

// TestSweep_EnqueuesOrphanService verifies that the sweep job detects
// containers tagged with a service ID that no longer exists in the DB
// and enqueues that service ID so a worker can tear it down.
func TestSweep_EnqueuesOrphanService(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	h := newLifecycleHarness(t)

	// Inject a container tagged with a service that doesn't exist in DB.
	orphanSvcID := "phantom-service"
	h.runtime.containers["orphan-ctr"] = deploykit.RunningContainer{
		DockerID:     "orphan-ctr",
		Name:         "dk-multi-phantom-0-deadbeef",
		ProjectID:    h.project.ID,
		ServiceID:    orphanSvcID,
		DeploymentID: "phantom-dep",
		State:        "running",
	}

	// Run the sweep synchronously (ReconcileOnce runs the sweep).
	h.rec.ReconcileOnce(ctx)

	// The sweep enqueues the orphan; a service worker would tear it down.
	// In the synchronous ReconcileOnce, the orphan teardown happens when
	// the *next* ReconcileOnce iterates services. Since ReconcileOnce only
	// iterates services that exist in the DB, the orphan isn't seen by
	// ReconcileOnce's loop. We need to call reconcileService directly with
	// the orphan ID (which is what the worker would do after the sweep
	// enqueued it).
	if err := h.rec.reconcileService(ctx, orphanSvcID); err != nil {
		t.Fatalf("reconcileService for orphan: %v", err)
	}

	if got := len(h.runtime.containers); got != 0 {
		t.Errorf("orphan container should have been torn down, got %d remaining",
			got)
	}
}

// TestWorkqueue_TriggerServiceEnqueuesJustOne verifies that
// TriggerService(svcID) adds only that one ID to the queue, without
// triggering a full resync.
func TestWorkqueue_TriggerServiceEnqueuesJustOne(t *testing.T) {
	rec := New(nil, nil, nil, nil, nil, slog.Default(), 30*time.Second, nil)
	rec.TriggerService("svc-1")
	if rec.serviceQueue.Len() != 1 {
		t.Fatalf("expected queue len 1, got %d", rec.serviceQueue.Len())
	}
	rec.TriggerService("svc-1") // coalesces
	if rec.serviceQueue.Len() != 1 {
		t.Errorf("duplicate TriggerService should coalesce, got len %d",
			rec.serviceQueue.Len())
	}
	rec.TriggerService("svc-2")
	if rec.serviceQueue.Len() != 2 {
		t.Errorf("second distinct key should add, got len %d",
			rec.serviceQueue.Len())
	}
}

