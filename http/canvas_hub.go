package http

import (
	"encoding/json"
	"log/slog"
	"sync"

	"github.com/coder/websocket"

	"github.com/heyjorgedev/deploykit"
)

// wsMessage is the envelope for all WebSocket messages.
type wsMessage struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

// canvasHub manages WebSocket connections for all projects.
type canvasHub struct {
	mu     sync.RWMutex
	rooms  map[string]*projectRoom
	logger *slog.Logger

	sub deploykit.Subscription
}

// newCanvasHub creates a new canvasHub.
func newCanvasHub(logger *slog.Logger) *canvasHub {
	return &canvasHub{
		rooms:  make(map[string]*projectRoom),
		logger: logger,
	}
}

// subscribe starts forwarding events from bus to matching project rooms.
// Must be called at most once before serving traffic.
func (h *canvasHub) subscribe(bus deploykit.EventBus) {
	sub := bus.Subscribe(128)
	h.sub = sub
	go func() {
		for evt := range sub.C() {
			h.dispatchEvent(evt)
		}
	}()
}

// unsubscribe stops the subscription goroutine, if any.
func (h *canvasHub) unsubscribe() {
	if h.sub != nil {
		h.sub.Close()
		h.sub = nil
	}
}

// broadcastToProject sends a pre-encoded message to all clients in a project's
// room. No-op if the room is empty or missing. Used by HTTP handlers that
// mutate project state outside of WebSocket messages.
func (h *canvasHub) broadcastToProject(projectID string, msg []byte) {
	h.mu.RLock()
	room, ok := h.rooms[projectID]
	h.mu.RUnlock()
	if !ok {
		return
	}
	room.broadcastAll(msg)
}

// broadcastPendingChangeAdded fans out a newly-appended change log entry.
func (h *canvasHub) broadcastPendingChangeAdded(projectID string, pc *deploykit.PendingChange) {
	msg, ok := h.marshalWS("pending-change:added", pc)
	if !ok {
		return
	}
	h.broadcastToProject(projectID, msg)
}

// broadcastPendingChangesRemoved announces that specific change log entries
// have been removed (e.g. when a pending-added service's canvas node is
// deleted before deploy). Clients drop the matching IDs from their cache.
func (h *canvasHub) broadcastPendingChangesRemoved(projectID string, ids []string) {
	msg, ok := h.marshalWS("pending-changes:removed", map[string]any{"ids": ids})
	if !ok {
		return
	}
	h.broadcastToProject(projectID, msg)
}

// broadcastPendingCleared announces that the project's pending change log was
// cleared without applying.
func (h *canvasHub) broadcastPendingCleared(projectID string) {
	msg, ok := h.marshalWS("pending-changes:cleared", struct{}{})
	if !ok {
		return
	}
	h.broadcastToProject(projectID, msg)
}

// broadcastPendingApplied announces that a deploy has landed. Clients should
// refetch applied state (services, env vars) since multiple resources may
// have changed atomically.
func (h *canvasHub) broadcastPendingApplied(projectID string, result *deploykit.ApplyResult) {
	msg, ok := h.marshalWS("pending-changes:applied", result)
	if !ok {
		return
	}
	h.broadcastToProject(projectID, msg)
}

// marshalWS produces a wsMessage envelope, logging and discarding on failure.
// Marshal errors on the static shapes used by these broadcasts indicate a
// programming bug rather than transient failure — silently swallowing them
// would mask a regression.
func (h *canvasHub) marshalWS(msgType string, payload any) ([]byte, bool) {
	encoded, err := json.Marshal(payload)
	if err != nil {
		h.logger.Error("marshaling ws payload", "err", err, "type", msgType)
		return nil, false
	}
	msg, err := json.Marshal(wsMessage{Type: msgType, Payload: encoded})
	if err != nil {
		h.logger.Error("marshaling ws envelope", "err", err, "type", msgType)
		return nil, false
	}
	return msg, true
}

// dispatchEvent routes a single bus event to the matching project room.
func (h *canvasHub) dispatchEvent(evt deploykit.Event) {
	if evt.ProjectID == "" {
		return
	}
	h.mu.RLock()
	room, ok := h.rooms[evt.ProjectID]
	h.mu.RUnlock()
	if !ok {
		return
	}

	msg, ok := translateBusEvent(evt)
	if !ok {
		return
	}
	room.broadcastAll(msg)
}

// translateBusEvent maps a domain Event to the WebSocket envelope consumed by
// the frontend canvas. Returns (nil, false) for events the canvas doesn't care
// about.
func translateBusEvent(evt deploykit.Event) ([]byte, bool) {
	var msgType string
	switch evt.Type {
	case deploykit.EventServiceCreated, deploykit.EventServiceUpdated:
		msgType = "service:upserted"
	case deploykit.EventServiceStatusChanged:
		msgType = "service:status-changed"
	case deploykit.EventServiceDeleted:
		msgType = "service:deleted"
	case deploykit.EventDeploymentCreated:
		msgType = "deployment:created"
	case deploykit.EventContainerCreated:
		msgType = "container:created"
	case deploykit.EventContainerDeleted:
		msgType = "container:deleted"
	default:
		return nil, false
	}

	payload, err := json.Marshal(evt.Payload)
	if err != nil {
		return nil, false
	}
	msg, err := json.Marshal(wsMessage{Type: msgType, Payload: payload})
	if err != nil {
		return nil, false
	}
	return msg, true
}

// joinRoom atomically gets-or-creates the room for a project and inserts the
// client. The user:joined broadcast (if needed) is sent after locks are released.
// Lock order: h.mu -> room.mu (consistent with cleanupRoom).
func (h *canvasHub) joinRoom(projectID string, client *canvasClient) *projectRoom {
	h.mu.Lock()
	room, ok := h.rooms[projectID]
	if !ok {
		room = &projectRoom{
			projectID: projectID,
			clients:   make(map[string]*canvasClient),
			drafts:    make(map[string]serviceDraft),
		}
		h.rooms[projectID] = room
	}

	room.mu.Lock()
	alreadyPresent := room.hasUser(client.userID)
	room.insertLocked(client)
	room.mu.Unlock()
	h.mu.Unlock()

	if !alreadyPresent {
		payload, _ := json.Marshal(map[string]string{
			"user_id":   client.userID,
			"user_name": client.userName,
		})
		msg, _ := json.Marshal(wsMessage{Type: "user:joined", Payload: payload})
		room.broadcast(client.connID, msg)
	}

	return room
}

// cleanupRoom removes a room if it has no connected clients. Holds both h.mu
// and room.mu so no joinRoom can race with the empty check.
func (h *canvasHub) cleanupRoom(projectID string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	room, ok := h.rooms[projectID]
	if !ok {
		return
	}

	room.mu.Lock()
	defer room.mu.Unlock()

	if len(room.clients) == 0 {
		delete(h.rooms, projectID)
	}
}

// projectRoom manages all clients connected to a single project's canvas.
type projectRoom struct {
	projectID string
	mu        sync.RWMutex
	clients   map[string]*canvasClient
	// drafts tracks in-flight service drafts (forms open) so other clients can
	// render ghost placeholders. Keyed by client-generated draft ID so a single
	// user can have multiple concurrent drafts. Ephemeral — cleared on submit,
	// cancel, or disconnect.
	drafts map[string]serviceDraft
}

// serviceDraft is the ephemeral state of a drafting form.
type serviceDraft struct {
	DraftID  string  `json:"draft_id"`
	UserID   string  `json:"user_id"`
	UserName string  `json:"user_name"`
	X        float64 `json:"x"`
	Y        float64 `json:"y"`
}

// insertLocked inserts a client into the room's client map. Caller must hold r.mu.
func (r *projectRoom) insertLocked(client *canvasClient) {
	r.clients[client.connID] = client
}

// removeClient unregisters a client and broadcasts a user:left message only if
// this was the user's last connection to the room (deduplicates multi-tab).
// Also clears any in-flight service draft for that user and broadcasts
// service:draft-cancelled.
func (r *projectRoom) removeClient(connID string) {
	r.mu.Lock()
	client, ok := r.clients[connID]
	if ok {
		close(client.send)
		delete(r.clients, connID)
	}
	stillPresent := ok && r.hasUser(client.userID)
	var cancelledDraftIDs []string
	if ok && !stillPresent {
		for id, d := range r.drafts {
			if d.UserID == client.userID {
				delete(r.drafts, id)
				cancelledDraftIDs = append(cancelledDraftIDs, id)
			}
		}
	}
	r.mu.Unlock()

	if ok && !stillPresent {
		payload, _ := json.Marshal(map[string]string{
			"user_id": client.userID,
		})
		msg, _ := json.Marshal(wsMessage{Type: "user:left", Payload: payload})
		r.broadcastAll(msg)

		for _, draftID := range cancelledDraftIDs {
			dp, _ := json.Marshal(map[string]string{"draft_id": draftID})
			dm, _ := json.Marshal(wsMessage{Type: "service:draft-cancelled", Payload: dp})
			r.broadcastAll(dm)
		}
	}
}

// broadcast sends a message to all clients except the sender.
func (r *projectRoom) broadcast(senderConnID string, msg []byte) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for id, client := range r.clients {
		if id == senderConnID {
			continue
		}
		select {
		case client.send <- msg:
		default:
			// Client send buffer full, skip this message.
		}
	}
}

// broadcastAll sends a message to all connected clients.
func (r *projectRoom) broadcastAll(msg []byte) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, client := range r.clients {
		select {
		case client.send <- msg:
		default:
		}
	}
}

// connectedUser is a deduplicated user entry for the users:list payload.
type connectedUser struct {
	UserID   string `json:"user_id"`
	UserName string `json:"user_name"`
}

// connectedUsers returns a deduplicated list of users in the room.
func (r *projectRoom) connectedUsers() []connectedUser {
	r.mu.RLock()
	defer r.mu.RUnlock()

	seen := make(map[string]bool)
	var users []connectedUser
	for _, c := range r.clients {
		if !seen[c.userID] {
			seen[c.userID] = true
			users = append(users, connectedUser{UserID: c.userID, UserName: c.userName})
		}
	}
	return users
}

// draftsList returns a snapshot of all active service drafts in the room.
func (r *projectRoom) draftsList() []serviceDraft {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]serviceDraft, 0, len(r.drafts))
	for _, d := range r.drafts {
		out = append(out, d)
	}
	return out
}

// hasUser returns true if any client in the room belongs to the given user ID.
// Must be called with r.mu held (read or write).
func (r *projectRoom) hasUser(userID string) bool {
	for _, c := range r.clients {
		if c.userID == userID {
			return true
		}
	}
	return false
}

// canvasClient represents a single WebSocket connection.
type canvasClient struct {
	connID   string
	userID   string
	userName string
	conn     *websocket.Conn
	send     chan []byte

	// Dependencies for handling messages.
	hub                  *canvasHub
	room                 *projectRoom
	projectID            string
	canvasService        deploykit.CanvasService
	serviceService       deploykit.ServiceService
	deploymentService    deploykit.DeploymentService
	pendingChangeService deploykit.PendingChangeService
	reconciler           Triggerable
	eventBus             deploykit.EventBus
	logger               *slog.Logger
}
