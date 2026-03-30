package reconciler

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/heyjorgedev/deploykit"
)

// Reconciler periodically reconciles desired state (projects in DB) with
// actual state (Docker networks) and corrects any drift.
type Reconciler struct {
	mu          sync.Mutex
	projects    deploykit.ProjectService
	provisioner deploykit.Provisioner
	logger      *slog.Logger
	interval    time.Duration
	trigger     chan struct{}
}

// New creates a new Reconciler.
func New(ps deploykit.ProjectService, prov deploykit.Provisioner, logger *slog.Logger, interval time.Duration) *Reconciler {
	return &Reconciler{
		projects:    ps,
		provisioner: prov,
		logger:      logger,
		interval:    interval,
		trigger:     make(chan struct{}, 1),
	}
}

// Run starts the reconciliation loop. It blocks until ctx is cancelled.
func (r *Reconciler) Run(ctx context.Context) {
	r.logger.Info("reconciler started", "interval", r.interval)

	// Run immediately on startup.
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

	// Fetch desired state: all projects.
	projects, err := r.allProjects(ctx)
	if err != nil {
		r.logger.Error("failed to list projects", "err", err)
		return
	}

	// Fetch actual state: all DeployKit-managed networks.
	actualNetworks, err := r.provisioner.ListNetworks(ctx)
	if err != nil {
		r.logger.Error("failed to list networks", "err", err)
		return
	}

	// Build lookup sets.
	actualSet := make(map[string]struct{}, len(actualNetworks))
	for _, name := range actualNetworks {
		actualSet[name] = struct{}{}
	}

	desiredSet := make(map[string]struct{}, len(projects))
	for _, p := range projects {
		desiredSet[deploykit.NetworkName(p)] = struct{}{}
	}

	// Create missing networks.
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

	// Remove orphaned networks.
	for _, name := range actualNetworks {
		if _, desired := desiredSet[name]; desired {
			continue
		}
		if err := r.provisioner.RemoveNetwork(ctx, name); err != nil {
			r.logger.Error("failed to remove orphaned network", "network", name, "err", err)
			continue
		}
	}

	r.logger.Debug("reconciliation cycle complete",
		"projects", len(projects),
		"networks_actual", len(actualNetworks),
		"networks_desired", len(desiredSet),
	)
}

// allProjects fetches all projects by paginating through ListProjects.
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
