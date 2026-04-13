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
}

// newCanvasHub creates a new canvasHub.
func newCanvasHub(logger *slog.Logger) *canvasHub {
	return &canvasHub{
		rooms:  make(map[string]*projectRoom),
		logger: logger,
	}
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
}

// insertLocked inserts a client into the room's client map. Caller must hold r.mu.
func (r *projectRoom) insertLocked(client *canvasClient) {
	r.clients[client.connID] = client
}

// removeClient unregisters a client and broadcasts a user:left message only if
// this was the user's last connection to the room (deduplicates multi-tab).
func (r *projectRoom) removeClient(connID string) {
	r.mu.Lock()
	client, ok := r.clients[connID]
	if ok {
		close(client.send)
		delete(r.clients, connID)
	}
	stillPresent := ok && r.hasUser(client.userID)
	r.mu.Unlock()

	if ok && !stillPresent {
		payload, _ := json.Marshal(map[string]string{
			"user_id": client.userID,
		})
		msg, _ := json.Marshal(wsMessage{Type: "user:left", Payload: payload})
		r.broadcastAll(msg)
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
	room          *projectRoom
	projectID     string
	canvasService deploykit.CanvasService
	logger        *slog.Logger
}
