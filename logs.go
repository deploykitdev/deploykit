package deploykit

import "context"

// LogLine is a single line of output from a container.
type LogLine struct {
	// ContainerID is the short (12-char) runtime container identifier.
	ContainerID string
	// Stream is "stdout" or "stderr".
	Stream string
	// Data is the line content with any runtime-added timestamp stripped
	// and the trailing newline removed.
	Data string
}

// LogStreamer streams logs from a container runtime. Implementations back
// onto Docker, Podman, or any other runtime that exposes a log feed.
type LogStreamer interface {
	// StreamContainerLogs streams logs for a single container onto out. It
	// emits the last `tail` lines then follows until ctx is cancelled or
	// the container exits. The caller owns `out` — implementations must
	// not close it.
	StreamContainerLogs(ctx context.Context, runtimeID string, tail int, out chan<- LogLine) error
}
