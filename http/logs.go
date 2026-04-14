package http

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/heyjorgedev/deploykit"
)

const (
	defaultLogTail   = 500
	maxLogTail       = 5000
	logHeartbeat     = 15 * time.Second
	logChannelBuffer = 256
)

func (s *Server) handleStreamServiceLogs(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("projectId")
	serviceID := r.PathValue("serviceId")

	svc, err := s.ServiceService.GetService(r.Context(), serviceID)
	if err != nil {
		s.errorResponse(w, r, err)
		return
	}
	if svc.ProjectID != projectID {
		s.errorResponse(w, r, deploykit.Errorf(deploykit.ENOTFOUND, "Service not found."))
		return
	}

	tail := defaultLogTail
	if v := r.URL.Query().Get("tail"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			tail = n
		}
	}
	if tail > maxLogTail {
		tail = maxLogTail
	}

	running := deploykit.ContainerStatusRunning
	containers, _, err := s.ContainerService.ListContainers(r.Context(), deploykit.ContainerFilter{
		ServiceID: &serviceID,
		Status:    &running,
	})
	if err != nil {
		s.errorResponse(w, r, err)
		return
	}
	if len(containers) == 0 {
		s.errorResponse(w, r, deploykit.Errorf(deploykit.ENOTFOUND, "No running containers for this service."))
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	lines := make(chan deploykit.LogLine, logChannelBuffer)
	var wg sync.WaitGroup

	for _, c := range containers {
		wg.Add(1)
		go func(dockerID string) {
			defer wg.Done()
			shortID := dockerID
			if len(shortID) > 12 {
				shortID = shortID[:12]
			}
			if err := s.LogStreamer.StreamContainerLogs(r.Context(), dockerID, tail, lines); err != nil && r.Context().Err() == nil {
				s.logger.Warn("log stream ended with error", "container", shortID, "err", err)
			}
			select {
			case <-r.Context().Done():
			case lines <- deploykit.LogLine{ContainerID: shortID, Stream: "event", Data: "__container_exited__"}:
			}
		}(c.DockerContainerID)
	}

	go func() {
		wg.Wait()
		close(lines)
	}()

	defer s.logger.Info("logs stream closed", "service_id", serviceID)

	ticker := time.NewTicker(logHeartbeat)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			if _, err := fmt.Fprint(w, ": ping\n\n"); err != nil {
				return
			}
			flusher.Flush()
		case ln, ok := <-lines:
			if !ok {
				fmt.Fprint(w, "event: end\ndata: {}\n\n")
				flusher.Flush()
				return
			}
			payload := map[string]any{
				"container_id": ln.ContainerID,
				"stream":       ln.Stream,
				"line":         ln.Data,
			}
			if ln.Stream == "event" && ln.Data == "__container_exited__" {
				payload = map[string]any{
					"container_id": ln.ContainerID,
					"event":        "container_exited",
				}
			}
			b, err := json.Marshal(payload)
			if err != nil {
				continue
			}
			if _, err := fmt.Fprintf(w, "data: %s\n\n", b); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}
