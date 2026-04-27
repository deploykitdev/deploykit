package sysinfo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/deploykitdev/deploykit"
)

// SettingsStore is the persistence boundary the system service needs.
// Implemented by sqlite.SystemSettingsStore.
type SettingsStore interface {
	Get(ctx context.Context) (*deploykit.SystemSettings, error)
	Update(ctx context.Context, u deploykit.SystemSettingsUpdate) (*deploykit.SystemSettings, error)
}

// ServiceLister is the slice of ServiceService the upgrade guard needs.
// Implemented by sqlite.ServiceService — we keep the surface tight to avoid
// importing the wider interface here.
type ServiceLister interface {
	ListServices(ctx context.Context, filter deploykit.ServiceFilter) ([]*deploykit.Service, int, error)
}

// upgradeFiles names the files the privileged upgrade unit reads/writes.
type upgradeFiles struct {
	dir       string
	requested string
	status    string
	log       string
}

func newUpgradeFiles(dataDir string) upgradeFiles {
	return upgradeFiles{
		dir:       dataDir,
		requested: filepath.Join(dataDir, "upgrade.requested"),
		status:    filepath.Join(dataDir, "upgrade.status"),
		log:       filepath.Join(dataDir, "upgrade.log"),
	}
}

// releaseCache holds the most recent successful poll of the upstream
// release feed. Concurrent access is guarded by mu.
type releaseCache struct {
	mu      sync.RWMutex
	release *deploykit.ReleaseInfo
}

func (c *releaseCache) get() *deploykit.ReleaseInfo {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.release == nil {
		return nil
	}
	cp := *c.release
	return &cp
}

func (c *releaseCache) set(r *deploykit.ReleaseInfo) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.release = r
}

// LatestRelease returns the cached upstream release info.
func (s *Service) LatestRelease(ctx context.Context) (*deploykit.ReleaseInfo, error) {
	r := s.releases.get()
	if r == nil {
		return nil, deploykit.Errorf(deploykit.ENOTFOUND, "Upstream release info not yet available.")
	}
	return r, nil
}

// RefreshLatestRelease forces a poll of the GitHub release feed.
func (s *Service) RefreshLatestRelease(ctx context.Context) (*deploykit.ReleaseInfo, error) {
	r, err := s.fetchLatestRelease(ctx)
	if err != nil {
		return nil, err
	}
	s.releases.set(r)
	return r, nil
}

// githubRelease mirrors the subset of the GitHub API release payload we use.
type githubRelease struct {
	TagName     string    `json:"tag_name"`
	Name        string    `json:"name"`
	HTMLURL     string    `json:"html_url"`
	Body        string    `json:"body"`
	PublishedAt time.Time `json:"published_at"`
	Draft       bool      `json:"draft"`
	Prerelease  bool      `json:"prerelease"`
}

func (s *Service) fetchLatestRelease(ctx context.Context) (*deploykit.ReleaseInfo, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", s.githubRepo)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("building request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "deploykitd/"+s.version)

	resp, err := s.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching latest release: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, deploykit.Errorf(deploykit.ENOTFOUND, "No releases published for %s.", s.githubRepo)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github returned %d", resp.StatusCode)
	}

	var gr githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&gr); err != nil {
		return nil, fmt.Errorf("decoding release: %w", err)
	}

	return &deploykit.ReleaseInfo{
		Version:     gr.TagName,
		URL:         gr.HTMLURL,
		Notes:       gr.Body,
		PublishedAt: gr.PublishedAt,
		FetchedAt:   time.Now().UTC(),
	}, nil
}

// versionPattern matches semver-ish tags: optional leading v, then digits.
var versionPattern = regexp.MustCompile(`^v?\d+\.\d+\.\d+(?:[-+][\w.\-]+)?$`)

// RequestUpgrade asks the privileged upgrade unit to install version.
func (s *Service) RequestUpgrade(ctx context.Context, version string) error {
	version = strings.TrimSpace(version)
	if version == "" {
		return deploykit.Errorf(deploykit.EINVALID, "Version is required.")
	}
	if !versionPattern.MatchString(version) {
		return deploykit.Errorf(deploykit.EINVALID, "Version %q does not look like a semver tag.", version)
	}
	if !strings.HasPrefix(version, "v") {
		version = "v" + version
	}

	// No-downgrade and same-version guards.
	if cmp, ok := compareVersions(s.version, version); ok {
		if cmp == 0 {
			return deploykit.Errorf(deploykit.ECONFLICT, "Already running %s.", version)
		}
		if cmp > 0 {
			return deploykit.Errorf(deploykit.EFORBIDDEN, "Refusing to downgrade %s -> %s.", s.version, version)
		}
	}

	// Mid-deploy guard.
	if s.services != nil {
		deploying := deploykit.ServiceStatusDeploying
		_, count, err := s.services.ListServices(ctx, deploykit.ServiceFilter{Status: &deploying, Limit: 1})
		if err != nil {
			return fmt.Errorf("checking deploying services: %w", err)
		}
		if count > 0 {
			return deploykit.Errorf(deploykit.ECONFLICT,
				"A deployment is in progress. The upgrade will start once it finishes.")
		}
	}

	// Don't queue a second upgrade on top of a running one.
	if cur, err := s.UpgradeStatus(ctx); err == nil {
		if cur.State == deploykit.UpgradeStateRunning || cur.State == deploykit.UpgradeStateQueued {
			return deploykit.Errorf(deploykit.ECONFLICT, "An upgrade is already %s.", cur.State)
		}
	}

	// Atomic-ish write: tmp + rename so the path unit doesn't fire on a
	// half-written file.
	tmp := s.upgrades.requested + ".tmp"
	if err := os.WriteFile(tmp, []byte(version+"\n"), 0o644); err != nil {
		return fmt.Errorf("writing upgrade request: %w", err)
	}
	if err := os.Rename(tmp, s.upgrades.requested); err != nil {
		return fmt.Errorf("placing upgrade request: %w", err)
	}

	// Pre-seed the status so the UI sees "queued" immediately rather than
	// reading the previous run's "done".
	queued := deploykit.UpgradeStatus{
		State:         deploykit.UpgradeStateQueued,
		TargetVersion: version,
		StartedAt:     time.Now().UTC(),
	}
	if blob, err := json.Marshal(queued); err == nil {
		_ = os.WriteFile(s.upgrades.status, blob, 0o644)
	}

	s.logger.Info("upgrade requested", "version", version)
	return nil
}

// UpgradeStatus reads the JSON status file written by the upgrade runner
// and tails the rolling log.
func (s *Service) UpgradeStatus(ctx context.Context) (*deploykit.UpgradeStatus, error) {
	out := &deploykit.UpgradeStatus{State: deploykit.UpgradeStateIdle}

	blob, err := os.ReadFile(s.upgrades.status)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("reading upgrade status: %w", err)
	}
	if err == nil && len(blob) > 0 {
		if err := json.Unmarshal(blob, out); err != nil {
			s.logger.Warn("malformed upgrade status", "err", err)
			out = &deploykit.UpgradeStatus{State: deploykit.UpgradeStateIdle}
		}
	}

	out.LogTail = tailFile(s.upgrades.log, 4096)
	return out, nil
}

// GetSettings returns the persisted system-wide settings.
func (s *Service) GetSettings(ctx context.Context) (*deploykit.SystemSettings, error) {
	if s.settings == nil {
		return &deploykit.SystemSettings{}, nil
	}
	return s.settings.Get(ctx)
}

// UpdateSettings persists changes to system-wide settings.
func (s *Service) UpdateSettings(ctx context.Context, u deploykit.SystemSettingsUpdate) (*deploykit.SystemSettings, error) {
	if s.settings == nil {
		return nil, deploykit.Errorf(deploykit.EINTERNAL, "Settings store is not configured.")
	}
	return s.settings.Update(ctx, u)
}

// tailFile returns up to maxBytes from the end of path. Best-effort:
// any error reading just returns "".
func tailFile(path string, maxBytes int64) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
		return ""
	}
	if fi.Size() > maxBytes {
		if _, err := f.Seek(-maxBytes, io.SeekEnd); err != nil {
			return ""
		}
	}
	buf, err := io.ReadAll(f)
	if err != nil {
		return ""
	}
	return string(buf)
}

// compareVersions returns -1 / 0 / 1 if a < b / a == b / a > b. ok is false
// if either side isn't a parseable semver-ish tag, in which case callers
// should skip the comparison.
func compareVersions(a, b string) (int, bool) {
	pa, oka := parseSemver(a)
	pb, okb := parseSemver(b)
	if !oka || !okb {
		return 0, false
	}
	for i := range 3 {
		if pa[i] != pb[i] {
			if pa[i] < pb[i] {
				return -1, true
			}
			return 1, true
		}
	}
	return 0, true
}

func parseSemver(v string) ([3]int, bool) {
	v = strings.TrimPrefix(v, "v")
	// Drop any pre-release / build metadata for the comparison.
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		v = v[:i]
	}
	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return [3]int{}, false
	}
	out := [3]int{}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return [3]int{}, false
		}
		out[i] = n
	}
	return out, true
}
