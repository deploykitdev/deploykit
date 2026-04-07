// Package sysinfo implements deploykit.SystemService by combining the
// gopsutil library (host CPU/memory/disk metrics) with the running Docker
// daemon (versions, container/image/volume counts and sizes).
//
// Both About and Status tolerate an unreachable Docker daemon: the Docker
// portion of the response will report Reachable: false and the rest of the
// payload will still be populated. The "is the box ok" page should remain
// useful even — especially — when something is wrong.
package sysinfo

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/host"
	"github.com/shirou/gopsutil/v4/load"
	"github.com/shirou/gopsutil/v4/mem"

	"github.com/heyjorgedev/deploykit"
	"github.com/heyjorgedev/deploykit/docker"
)

// Service implements deploykit.SystemService.
type Service struct {
	docker    *docker.Client
	logger    *slog.Logger
	dbPath    string
	version   string
	startedAt time.Time
}

// NewService constructs a Service. version is the deploykit build version
// to report in the About payload (currently "dev"). startedAt should be the
// process start time, used to compute uptime.
func NewService(d *docker.Client, logger *slog.Logger, dbPath, version string, startedAt time.Time) *Service {
	return &Service{
		docker:    d,
		logger:    logger,
		dbPath:    dbPath,
		version:   version,
		startedAt: startedAt,
	}
}

// About returns a snapshot of static deploykit/Docker/database info.
func (s *Service) About(ctx context.Context) (*deploykit.SystemAbout, error) {
	about := &deploykit.SystemAbout{
		DeployKit: deploykit.DeployKitInfo{
			Version:   s.version,
			GoVersion: runtime.Version(),
			StartedAt: s.startedAt,
		},
		Docker:   s.dockerInfo(ctx),
		Database: s.databaseInfo(),
	}
	return about, nil
}

// Status returns a live snapshot of host + Docker resource usage.
func (s *Service) Status(ctx context.Context) (*deploykit.SystemStatus, error) {
	return &deploykit.SystemStatus{
		Host:   s.hostStatus(ctx),
		Docker: s.dockerStatus(ctx),
	}, nil
}

// dockerInfo collects About-level Docker daemon info. On any failure it
// returns a partial DockerInfo with Reachable=false and Error populated.
func (s *Service) dockerInfo(ctx context.Context) deploykit.DockerInfo {
	info, err := s.docker.Info(ctx)
	if err != nil {
		s.logger.Warn("docker info failed", "err", err)
		return deploykit.DockerInfo{Reachable: false, Error: err.Error()}
	}

	out := deploykit.DockerInfo{
		Reachable:     true,
		ServerVersion: info.ServerVersion,
		OS:            info.OperatingSystem,
		KernelVersion: info.KernelVersion,
		Architecture:  info.Architecture,
		StorageDriver: info.Driver,
		LoggingDriver: info.LoggingDriver,
		CgroupDriver:  info.CgroupDriver,
		DockerRootDir: info.DockerRootDir,
		Warnings:      info.Warnings,
	}

	// APIVersion is on the Ping response, not Info. Best-effort: don't fail
	// the whole About if Ping happens to fail after Info succeeded.
	if ping, err := s.docker.PingDaemon(ctx); err == nil {
		out.APIVersion = ping.APIVersion
	}

	return out
}

// databaseInfo reports the configured DB path and its current on-disk size.
// A missing file is reported as size 0 rather than as an error.
func (s *Service) databaseInfo() deploykit.DatabaseInfo {
	out := deploykit.DatabaseInfo{Path: s.dbPath}
	if fi, err := os.Stat(s.dbPath); err == nil {
		out.SizeBytes = fi.Size()
	}
	return out
}

// hostStatus collects all host-level metrics via gopsutil. Each subsection
// is best-effort: a failure in one (e.g. swap not configured) does not
// prevent the others from being reported.
func (s *Service) hostStatus(ctx context.Context) deploykit.HostStatus {
	out := deploykit.HostStatus{}

	if h, err := host.InfoWithContext(ctx); err == nil {
		out.Hostname = h.Hostname
		out.Uptime = h.Uptime
	} else {
		s.logger.Warn("host info failed", "err", err)
	}

	out.CPU = s.cpuStatus(ctx)
	out.Memory = s.memStatus(ctx)
	out.Swap = s.swapStatus(ctx)
	out.Disks = s.diskStatus(ctx)

	return out
}

func (s *Service) cpuStatus(ctx context.Context) deploykit.CPUStatus {
	out := deploykit.CPUStatus{}

	if n, err := cpu.Counts(true); err == nil {
		out.Cores = n
	}

	// interval=0 returns the percentage since the last call (or since boot
	// on the very first call). This is non-blocking and good enough for a
	// page that polls every few seconds.
	if pcts, err := cpu.PercentWithContext(ctx, 0, false); err == nil && len(pcts) > 0 {
		out.UsagePct = pcts[0]
	} else if err != nil {
		s.logger.Warn("cpu percent failed", "err", err)
	}

	if avg, err := load.AvgWithContext(ctx); err == nil {
		out.Load1 = avg.Load1
		out.Load5 = avg.Load5
		out.Load15 = avg.Load15
	}

	return out
}

func (s *Service) memStatus(ctx context.Context) deploykit.MemStatus {
	v, err := mem.VirtualMemoryWithContext(ctx)
	if err != nil {
		s.logger.Warn("mem virtual failed", "err", err)
		return deploykit.MemStatus{}
	}
	return deploykit.MemStatus{
		TotalBytes: v.Total,
		UsedBytes:  v.Used,
		UsagePct:   v.UsedPercent,
	}
}

func (s *Service) swapStatus(ctx context.Context) deploykit.MemStatus {
	v, err := mem.SwapMemoryWithContext(ctx)
	if err != nil {
		return deploykit.MemStatus{}
	}
	return deploykit.MemStatus{
		TotalBytes: v.Total,
		UsedBytes:  v.Used,
		UsagePct:   v.UsedPercent,
	}
}

// diskStatus reports usage for the root filesystem and, if it lives on a
// distinct mount, the filesystem holding the Docker root dir. We
// deliberately do NOT enumerate every partition: a single-node PaaS
// operator cares about "/" and "where Docker stores its stuff", not about
// /boot or tmpfs mounts.
func (s *Service) diskStatus(ctx context.Context) []deploykit.DiskStatus {
	mounts := []string{"/"}

	// Discover the Docker root dir if Docker is reachable, and add the
	// filesystem that contains it (resolved against the actual mountpoints).
	if info, err := s.docker.Info(ctx); err == nil && info.DockerRootDir != "" {
		if mp := mountpointFor(ctx, info.DockerRootDir); mp != "" && mp != "/" {
			mounts = append(mounts, mp)
		}
	}

	out := make([]deploykit.DiskStatus, 0, len(mounts))
	seen := map[string]bool{}
	for _, mp := range mounts {
		if seen[mp] {
			continue
		}
		seen[mp] = true

		u, err := disk.UsageWithContext(ctx, mp)
		if err != nil {
			s.logger.Warn("disk usage failed", "mountpoint", mp, "err", err)
			continue
		}
		out = append(out, deploykit.DiskStatus{
			Mountpoint: mp,
			TotalBytes: u.Total,
			UsedBytes:  u.Used,
			UsagePct:   u.UsedPercent,
		})
	}
	return out
}

// mountpointFor returns the longest mountpoint that is a prefix of path,
// i.e. the filesystem that path actually lives on. Empty string on failure.
func mountpointFor(ctx context.Context, path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		return ""
	}
	parts, err := disk.PartitionsWithContext(ctx, false)
	if err != nil {
		return ""
	}
	best := ""
	for _, p := range parts {
		if p.Mountpoint == "" {
			continue
		}
		if abs == p.Mountpoint || strings.HasPrefix(abs, p.Mountpoint+string(filepath.Separator)) {
			if len(p.Mountpoint) > len(best) {
				best = p.Mountpoint
			}
		}
	}
	return best
}

// dockerStatus collects Docker container/image/volume counts. If the
// daemon is unreachable, returns Reachable: false with everything zeroed.
func (s *Service) dockerStatus(ctx context.Context) deploykit.DockerStatus {
	info, err := s.docker.Info(ctx)
	if err != nil {
		s.logger.Warn("docker info failed", "err", err)
		return deploykit.DockerStatus{Reachable: false}
	}

	out := deploykit.DockerStatus{
		Reachable:         true,
		Containers:        info.Containers,
		ContainersRunning: info.ContainersRunning,
		ContainersStopped: info.ContainersStopped,
		Images:            info.Images,
	}

	// DiskUsage is a heavier call and may fail independently — treat it as
	// best-effort.
	du, err := s.docker.DiskUsage(ctx)
	if err != nil {
		s.logger.Warn("docker disk usage failed", "err", err)
		return out
	}

	for _, img := range du.Images {
		out.ImagesSizeBytes += img.Size
	}
	out.Volumes = len(du.Volumes)
	for _, v := range du.Volumes {
		if v.UsageData != nil {
			out.VolumesSizeBytes += v.UsageData.Size
		}
	}
	for _, bc := range du.BuildCache {
		out.BuildCacheBytes += bc.Size
	}

	return out
}
