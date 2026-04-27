package reconciler

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/deploykitdev/deploykit"
)

// mockProjectService implements deploykit.ProjectService for testing.
type mockProjectService struct {
	projects []*deploykit.Project
	err      error
}

func (m *mockProjectService) ListProjects(_ context.Context, filter deploykit.ProjectFilter) ([]*deploykit.Project, int, error) {
	if m.err != nil {
		return nil, 0, m.err
	}
	start := min(filter.Offset, len(m.projects))
	end := min(start+filter.Limit, len(m.projects))
	return m.projects[start:end], len(m.projects), nil
}

func (m *mockProjectService) CreateProject(context.Context, deploykit.ProjectCreate) (*deploykit.Project, error) {
	return nil, errors.New("not implemented")
}
func (m *mockProjectService) GetProject(context.Context, string) (*deploykit.Project, error) {
	return nil, errors.New("not implemented")
}
func (m *mockProjectService) UpdateProject(context.Context, string, deploykit.ProjectUpdate) (*deploykit.Project, error) {
	return nil, errors.New("not implemented")
}
func (m *mockProjectService) DeleteProject(context.Context, string) error {
	return errors.New("not implemented")
}

// mockProvisioner implements deploykit.Provisioner for testing.
type mockProvisioner struct {
	networks     []string
	ensureCalls  []string
	removeCalls  []string
	ensureErr    error
	removeErr    error
	listErr      error
	ensureErrFor map[string]error // per-network errors
	removeErrFor map[string]error
}

func (m *mockProvisioner) EnsureNetwork(_ context.Context, project *deploykit.Project) error {
	name := deploykit.NetworkName(project)
	m.ensureCalls = append(m.ensureCalls, name)
	if err, ok := m.ensureErrFor[name]; ok {
		return err
	}
	return m.ensureErr
}

func (m *mockProvisioner) RemoveNetwork(_ context.Context, networkName string) error {
	m.removeCalls = append(m.removeCalls, networkName)
	if err, ok := m.removeErrFor[networkName]; ok {
		return err
	}
	return m.removeErr
}

func (m *mockProvisioner) ListNetworks(context.Context) ([]string, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	return m.networks, nil
}

func (m *mockProvisioner) EnsureImage(context.Context, string) error { return nil }
func (m *mockProvisioner) CreateAndStartContainer(context.Context, deploykit.ContainerSpec) (string, error) {
	return "", nil
}
func (m *mockProvisioner) StopAndRemoveContainer(context.Context, string) error { return nil }
func (m *mockProvisioner) ListContainers(context.Context) ([]deploykit.RunningContainer, error) {
	return nil, nil
}

func testProject(id, slug string) *deploykit.Project {
	return &deploykit.Project{ID: id, Slug: slug}
}

func TestReconcileOnce(t *testing.T) {
	logger := slog.Default()

	tests := []struct {
		name            string
		projects        []*deploykit.Project
		networks        []string
		projectErr      error
		listNetErr      error
		ensureErr       error
		removeErr       error
		ensureErrFor    map[string]error
		removeErrFor    map[string]error
		wantEnsure      []string
		wantRemove      []string
		wantEnsureCount int
		wantRemoveCount int
	}{
		{
			name:            "no projects no networks",
			projects:        nil,
			networks:        nil,
			wantEnsureCount: 0,
			wantRemoveCount: 0,
		},
		{
			name:       "projects without networks creates all",
			projects:   []*deploykit.Project{testProject("1", "app-a"), testProject("2", "app-b")},
			networks:   nil,
			wantEnsure: []string{"dk-app-a", "dk-app-b"},
		},
		{
			name:       "networks without projects removes all",
			projects:   nil,
			networks:   []string{"dk-app-a", "dk-app-b"},
			wantRemove: []string{"dk-app-a", "dk-app-b"},
		},
		{
			name:            "matching state is a no-op",
			projects:        []*deploykit.Project{testProject("1", "app-a")},
			networks:        []string{"dk-app-a"},
			wantEnsureCount: 0,
			wantRemoveCount: 0,
		},
		{
			name:       "mixed state creates and removes correctly",
			projects:   []*deploykit.Project{testProject("1", "app-a"), testProject("2", "app-c")},
			networks:   []string{"dk-app-a", "dk-app-b"},
			wantEnsure: []string{"dk-app-c"},
			wantRemove: []string{"dk-app-b"},
		},
		{
			name:         "ensure failure for one project does not block others",
			projects:     []*deploykit.Project{testProject("1", "app-a"), testProject("2", "app-b")},
			networks:     nil,
			ensureErrFor: map[string]error{"dk-app-a": errors.New("docker error")},
			wantEnsure:   []string{"dk-app-a", "dk-app-b"},
		},
		{
			name:         "remove failure for one network does not block others",
			projects:     nil,
			networks:     []string{"dk-app-a", "dk-app-b"},
			removeErrFor: map[string]error{"dk-app-a": errors.New("docker error")},
			wantRemove:   []string{"dk-app-a", "dk-app-b"},
		},
		{
			name:            "list projects error aborts cycle",
			projectErr:      errors.New("db error"),
			wantEnsureCount: 0,
			wantRemoveCount: 0,
		},
		{
			name:            "list networks error aborts cycle",
			projects:        []*deploykit.Project{testProject("1", "app-a")},
			listNetErr:      errors.New("docker error"),
			wantEnsureCount: 0,
			wantRemoveCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ps := &mockProjectService{projects: tt.projects, err: tt.projectErr}
			prov := &mockProvisioner{
				networks:     tt.networks,
				ensureErr:    tt.ensureErr,
				removeErr:    tt.removeErr,
				listErr:      tt.listNetErr,
				ensureErrFor: tt.ensureErrFor,
				removeErrFor: tt.removeErrFor,
			}

			rec := New(ps, nil, nil, nil, prov, logger, 30*time.Second, nil)
			rec.ReconcileOnce(context.Background())

			if tt.wantEnsure != nil {
				if len(prov.ensureCalls) != len(tt.wantEnsure) {
					t.Fatalf("ensure calls: got %v, want %v", prov.ensureCalls, tt.wantEnsure)
				}
				for i, want := range tt.wantEnsure {
					if prov.ensureCalls[i] != want {
						t.Errorf("ensure call %d: got %q, want %q", i, prov.ensureCalls[i], want)
					}
				}
			} else {
				if len(prov.ensureCalls) != tt.wantEnsureCount {
					t.Fatalf("ensure call count: got %d, want %d", len(prov.ensureCalls), tt.wantEnsureCount)
				}
			}

			if tt.wantRemove != nil {
				if len(prov.removeCalls) != len(tt.wantRemove) {
					t.Fatalf("remove calls: got %v, want %v", prov.removeCalls, tt.wantRemove)
				}
				for i, want := range tt.wantRemove {
					if prov.removeCalls[i] != want {
						t.Errorf("remove call %d: got %q, want %q", i, prov.removeCalls[i], want)
					}
				}
			} else {
				if len(prov.removeCalls) != tt.wantRemoveCount {
					t.Fatalf("remove call count: got %d, want %d", len(prov.removeCalls), tt.wantRemoveCount)
				}
			}
		})
	}
}

func TestTrigger(t *testing.T) {
	rec := New(nil, nil, nil, nil, nil, slog.Default(), 30*time.Second, nil)

	// First trigger should succeed.
	rec.Trigger()
	select {
	case <-rec.trigger:
	default:
		t.Fatal("expected trigger to be pending")
	}

	// Trigger when already pending should not block.
	rec.Trigger()
	rec.Trigger()

	select {
	case <-rec.trigger:
	default:
		// Channel may or may not have a value depending on timing; both are fine.
	}
}
