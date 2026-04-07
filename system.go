package deploykit

import (
	"context"
	"time"
)

// SystemService exposes information about the running deploykit instance,
// the host it runs on, and the Docker daemon it manages.
type SystemService interface {
	// About returns a snapshot of static information about the deploykit
	// build, the connected Docker daemon, and the database. About is safe to
	// call when Docker is unreachable: the Docker section will indicate the
	// failure rather than returning an error.
	About(ctx context.Context) (*SystemAbout, error)

	// Status returns a live snapshot of host resource usage and Docker
	// object counts. Like About, Status tolerates an unreachable Docker
	// daemon and returns partial data rather than failing.
	Status(ctx context.Context) (*SystemStatus, error)
}

// SystemAbout is the static "what am I running" view returned by
// SystemService.About.
type SystemAbout struct {
	DeployKit DeployKitInfo `json:"deploykit"`
	Docker    DockerInfo    `json:"docker"`
	Database  DatabaseInfo  `json:"database"`
}

// DeployKitInfo describes the running deploykit binary.
type DeployKitInfo struct {
	Version   string    `json:"version"`
	GoVersion string    `json:"go_version"`
	StartedAt time.Time `json:"started_at"`
}

// DockerInfo describes the connected Docker daemon. When the daemon is
// unreachable, Reachable is false and Error contains the failure reason;
// the remaining fields will be empty.
type DockerInfo struct {
	Reachable     bool     `json:"reachable"`
	Error         string   `json:"error,omitempty"`
	ServerVersion string   `json:"server_version,omitempty"`
	APIVersion    string   `json:"api_version,omitempty"`
	OS            string   `json:"os,omitempty"`
	KernelVersion string   `json:"kernel_version,omitempty"`
	Architecture  string   `json:"architecture,omitempty"`
	StorageDriver string   `json:"storage_driver,omitempty"`
	LoggingDriver string   `json:"logging_driver,omitempty"`
	CgroupDriver  string   `json:"cgroup_driver,omitempty"`
	DockerRootDir string   `json:"docker_root_dir,omitempty"`
	Warnings      []string `json:"warnings,omitempty"`
}

// DatabaseInfo describes the SQLite database backing deploykit.
type DatabaseInfo struct {
	Path       string `json:"path"`
	SizeBytes  int64  `json:"size_bytes"`
}

// SystemStatus is the live "is the box ok" view returned by
// SystemService.Status.
type SystemStatus struct {
	Host   HostStatus   `json:"host"`
	Docker DockerStatus `json:"docker"`
}

// HostStatus is a point-in-time snapshot of host resource usage.
type HostStatus struct {
	Hostname string       `json:"hostname"`
	Uptime   uint64       `json:"uptime"`
	CPU      CPUStatus    `json:"cpu"`
	Memory   MemStatus    `json:"memory"`
	Swap     MemStatus    `json:"swap"`
	Disks    []DiskStatus `json:"disks"`
}

// CPUStatus reports CPU saturation and load averages.
type CPUStatus struct {
	Cores    int     `json:"cores"`
	UsagePct float64 `json:"usage_pct"`
	Load1    float64 `json:"load1"`
	Load5    float64 `json:"load5"`
	Load15   float64 `json:"load15"`
}

// MemStatus reports memory or swap usage.
type MemStatus struct {
	TotalBytes uint64  `json:"total_bytes"`
	UsedBytes  uint64  `json:"used_bytes"`
	UsagePct   float64 `json:"usage_pct"`
}

// DiskStatus reports usage for a single mounted filesystem.
type DiskStatus struct {
	Mountpoint string  `json:"mountpoint"`
	TotalBytes uint64  `json:"total_bytes"`
	UsedBytes  uint64  `json:"used_bytes"`
	UsagePct   float64 `json:"usage_pct"`
}

// DockerStatus reports counts and disk usage for Docker objects. When the
// daemon is unreachable, Reachable is false and the remaining fields are
// zero.
type DockerStatus struct {
	Reachable         bool  `json:"reachable"`
	Containers        int   `json:"containers"`
	ContainersRunning int   `json:"containers_running"`
	ContainersStopped int   `json:"containers_stopped"`
	Images            int   `json:"images"`
	ImagesSizeBytes   int64 `json:"images_size_bytes"`
	Volumes           int   `json:"volumes"`
	VolumesSizeBytes  int64 `json:"volumes_size_bytes"`
	BuildCacheBytes   int64 `json:"build_cache_bytes"`
}
