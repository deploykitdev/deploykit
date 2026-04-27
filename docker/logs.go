package docker

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/deploykitdev/deploykit"
)

// StreamContainerLogs streams logs for a single container onto out. It emits
// the last `tail` lines then follows until ctx is cancelled or the container
// exits. Callers own `out` and must not close it until all producers return.
//
// StreamContainerLogs satisfies deploykit.LogStreamer.
func (c *Client) StreamContainerLogs(ctx context.Context, dockerID string, tail int, out chan<- deploykit.LogLine) error {
	shortID := dockerID
	if len(shortID) > 12 {
		shortID = shortID[:12]
	}

	inspect, err := c.cli.ContainerInspect(ctx, dockerID)
	if err != nil {
		return fmt.Errorf("inspecting container %s: %w", shortID, err)
	}

	rc, err := c.cli.ContainerLogs(ctx, dockerID, container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     true,
		Tail:       strconv.Itoa(tail),
		Timestamps: true,
	})
	if err != nil {
		return fmt.Errorf("opening log stream for %s: %w", shortID, err)
	}
	defer rc.Close()

	emit := func(stream string, line string) {
		line = stripTimestamp(line)
		if line == "" {
			return
		}
		select {
		case <-ctx.Done():
		case out <- deploykit.LogLine{ContainerID: shortID, Stream: stream, Data: line}:
		}
	}

	if inspect.Config != nil && inspect.Config.Tty {
		scanner := bufio.NewScanner(rc)
		scanner.Buffer(make([]byte, 64*1024), 1024*1024)
		for scanner.Scan() {
			emit("stdout", scanner.Text())
		}
		if err := scanner.Err(); err != nil && ctx.Err() == nil {
			return fmt.Errorf("reading tty logs for %s: %w", shortID, err)
		}
		return nil
	}

	stdoutR, stdoutW := io.Pipe()
	stderrR, stderrW := io.Pipe()

	var wg sync.WaitGroup
	wg.Add(2)
	scan := func(r io.Reader, stream string) {
		defer wg.Done()
		scanner := bufio.NewScanner(r)
		scanner.Buffer(make([]byte, 64*1024), 1024*1024)
		for scanner.Scan() {
			emit(stream, scanner.Text())
		}
	}
	go scan(stdoutR, "stdout")
	go scan(stderrR, "stderr")

	_, copyErr := stdcopy.StdCopy(stdoutW, stderrW, rc)
	stdoutW.Close()
	stderrW.Close()
	wg.Wait()

	if copyErr != nil && ctx.Err() == nil && copyErr != io.EOF {
		return fmt.Errorf("demuxing logs for %s: %w", shortID, copyErr)
	}
	return nil
}

// stripTimestamp removes the RFC3339Nano timestamp Docker prepends when
// Timestamps: true is set. Format: "<ts> <rest>".
func stripTimestamp(line string) string {
	if i := strings.IndexByte(line, ' '); i > 0 {
		return line[i+1:]
	}
	return line
}
