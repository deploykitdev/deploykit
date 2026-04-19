package http

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/coder/websocket"

	"github.com/heyjorgedev/deploykit"
)

const (
	// authTimeout is the max time a client has to send the auth message after connecting.
	authTimeout = 5 * time.Second

	// writeWait is the time allowed to write a message to the client.
	writeWait = 10 * time.Second

	// pingPeriod is how often to send pings to keep the connection alive.
	pingPeriod = 54 * time.Second

	// maxMessageSize is the maximum message size allowed from a client.
	maxMessageSize = 64 * 1024 // 64KB
)

// handleCanvasWebSocket upgrades to WebSocket and manages the canvas connection.
func (s *Server) handleCanvasWebSocket(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("projectId")

	// Verify project exists before upgrading.
	_, err := s.ProjectService.GetProject(r.Context(), projectID)
	if err != nil {
		s.errorResponse(w, r, err)
		return
	}

	// Accept the WebSocket upgrade.
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		OriginPatterns: []string{"*"},
	})
	if err != nil {
		s.logger.Error("websocket accept failed", "err", err)
		return
	}
	conn.SetReadLimit(maxMessageSize)

	// Authenticate via first message.
	user, err := s.authenticateWebSocket(r.Context(), conn)
	if err != nil {
		s.logger.Debug("websocket auth failed", "err", err, "project_id", projectID)
		conn.Close(websocket.StatusCode(4001), "Authentication failed.")
		return
	}

	// Load initial canvas state.
	nodes, edges, err := s.CanvasService.GetCanvasState(r.Context(), projectID)
	if err != nil {
		s.logger.Error("loading canvas state", "err", err, "project_id", projectID)
		conn.Close(websocket.StatusInternalError, "Failed to load canvas state.")
		return
	}

	// Create client and atomically join the room.
	client := &canvasClient{
		connID:               uuid.New().String(),
		userID:               user.ID,
		userName:             user.Name,
		conn:                 conn,
		send:                 make(chan []byte, 256),
		hub:                  s.canvasHub,
		projectID:            projectID,
		canvasService:        s.CanvasService,
		serviceService:       s.ServiceService,
		deploymentService:    s.DeploymentService,
		pendingChangeService: s.PendingChangeService,
		reconciler:           s.Reconciler,
		eventBus:             s.EventBus,
		logger:               s.logger,
	}
	room := s.canvasHub.joinRoom(projectID, client)
	client.room = room

	// Send initial canvas state.
	if err := client.sendCanvasState(nodes, edges); err != nil {
		s.logger.Error("sending initial state", "err", err)
		room.removeClient(client.connID)
		conn.Close(websocket.StatusInternalError, "Failed to send canvas state.")
		s.canvasHub.cleanupRoom(projectID)
		return
	}

	// Send list of connected users (deduplicated).
	if err := client.sendConnectedUsers(room.connectedUsers()); err != nil {
		s.logger.Error("sending connected users", "err", err)
	}

	// Send snapshot of any in-flight service drafts so late-joiners see ghosts.
	if err := client.sendServiceDrafts(room.draftsList()); err != nil {
		s.logger.Error("sending service drafts", "err", err)
	}

	// Send the current pending change log so the client shows the bottom panel.
	if s.PendingChangeService != nil {
		changes, err := s.PendingChangeService.List(r.Context(), projectID)
		if err != nil {
			s.logger.Error("loading pending changes", "err", err, "project_id", projectID)
		} else if err := client.sendPendingChanges(changes); err != nil {
			s.logger.Error("sending pending changes", "err", err)
		}
	}

	s.logger.Info("canvas client connected",
		"user_id", user.ID,
		"user_name", user.Name,
		"project_id", projectID,
		"conn_id", client.connID,
	)

	// Start write pump in background, read pump blocks.
	// Parented off the request context so server shutdown tears the pumps down.
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	go client.writePump(ctx)
	client.readPump(ctx)

	// Cleanup on disconnect.
	room.removeClient(client.connID)
	s.canvasHub.cleanupRoom(projectID)

	s.logger.Info("canvas client disconnected",
		"user_id", user.ID,
		"project_id", projectID,
		"conn_id", client.connID,
	)
}

// authenticateWebSocket waits for the first message to be an auth message
// and validates the token.
func (s *Server) authenticateWebSocket(ctx context.Context, conn *websocket.Conn) (*deploykit.User, error) {
	authCtx, cancel := context.WithTimeout(ctx, authTimeout)
	defer cancel()

	_, data, err := conn.Read(authCtx)
	if err != nil {
		return nil, fmt.Errorf("reading auth message: %w", err)
	}

	var msg wsMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		return nil, fmt.Errorf("decoding auth message: %w", err)
	}

	if msg.Type != "auth" {
		return nil, fmt.Errorf("expected auth message, got %q", msg.Type)
	}

	var payload struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(msg.Payload, &payload); err != nil {
		return nil, fmt.Errorf("decoding auth payload: %w", err)
	}

	if payload.Token == "" {
		return nil, fmt.Errorf("empty auth token")
	}

	// Try access token first, then API key (same as authenticate middleware).
	user, err := s.AuthService.ValidateAccessToken(ctx, payload.Token)
	if err != nil {
		user, err = s.AuthService.ValidateAPIKey(ctx, payload.Token)
	}
	if err != nil {
		return nil, fmt.Errorf("invalid token: %w", err)
	}

	return user, nil
}

// sendCanvasState sends the full canvas state to the client.
func (c *canvasClient) sendCanvasState(nodes []*deploykit.CanvasNode, edges []*deploykit.CanvasEdge) error {
	if nodes == nil {
		nodes = []*deploykit.CanvasNode{}
	}
	if edges == nil {
		edges = []*deploykit.CanvasEdge{}
	}

	payload, err := json.Marshal(map[string]any{
		"nodes": nodes,
		"edges": edges,
	})
	if err != nil {
		return fmt.Errorf("marshaling canvas state: %w", err)
	}

	msg, err := json.Marshal(wsMessage{Type: "canvas:state", Payload: payload})
	if err != nil {
		return fmt.Errorf("marshaling state message: %w", err)
	}

	select {
	case c.send <- msg:
		return nil
	default:
		return fmt.Errorf("send buffer full")
	}
}

// sendConnectedUsers sends the deduplicated list of connected users to this client.
func (c *canvasClient) sendConnectedUsers(users []connectedUser) error {
	payload, err := json.Marshal(map[string]any{
		"users": users,
	})
	if err != nil {
		return fmt.Errorf("marshaling connected users: %w", err)
	}

	msg, err := json.Marshal(wsMessage{Type: "users:list", Payload: payload})
	if err != nil {
		return fmt.Errorf("marshaling users list message: %w", err)
	}

	select {
	case c.send <- msg:
		return nil
	default:
		return fmt.Errorf("send buffer full")
	}
}

// sendPendingChanges sends the current pending change log to this client.
func (c *canvasClient) sendPendingChanges(changes []*deploykit.PendingChange) error {
	if changes == nil {
		changes = []*deploykit.PendingChange{}
	}
	payload, err := json.Marshal(map[string]any{"changes": changes})
	if err != nil {
		return fmt.Errorf("marshaling pending changes: %w", err)
	}
	msg, err := json.Marshal(wsMessage{Type: "pending-changes:state", Payload: payload})
	if err != nil {
		return fmt.Errorf("marshaling pending changes message: %w", err)
	}
	select {
	case c.send <- msg:
		return nil
	default:
		return fmt.Errorf("send buffer full")
	}
}

// sendServiceDrafts sends the current set of in-flight service drafts to this client.
func (c *canvasClient) sendServiceDrafts(drafts []serviceDraft) error {
	if drafts == nil {
		drafts = []serviceDraft{}
	}
	payload, err := json.Marshal(map[string]any{"drafts": drafts})
	if err != nil {
		return fmt.Errorf("marshaling service drafts: %w", err)
	}
	msg, err := json.Marshal(wsMessage{Type: "service:drafts", Payload: payload})
	if err != nil {
		return fmt.Errorf("marshaling service drafts message: %w", err)
	}
	select {
	case c.send <- msg:
		return nil
	default:
		return fmt.Errorf("send buffer full")
	}
}

// readPump reads messages from the WebSocket and dispatches them.
func (c *canvasClient) readPump(ctx context.Context) {
	defer c.conn.CloseNow()

	for {
		_, data, err := c.conn.Read(ctx)
		if err != nil {
			// Normal close or context cancelled.
			if websocket.CloseStatus(err) == websocket.StatusNormalClosure ||
				websocket.CloseStatus(err) == websocket.StatusGoingAway {
				return
			}
			if ctx.Err() != nil {
				return
			}
			c.logger.Debug("websocket read error", "err", err, "conn_id", c.connID)
			return
		}

		var msg wsMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			c.sendError("Invalid message format.")
			continue
		}

		c.handleMessage(ctx, msg)
	}
}

// writePump writes messages from the send channel to the WebSocket.
func (c *canvasClient) writePump(ctx context.Context) {
	ticker := time.NewTicker(pingPeriod)
	defer ticker.Stop()
	defer c.conn.CloseNow()

	for {
		select {
		case msg, ok := <-c.send:
			if !ok {
				// Channel closed, connection is being cleaned up.
				return
			}
			writeCtx, cancel := context.WithTimeout(ctx, writeWait)
			err := c.conn.Write(writeCtx, websocket.MessageText, msg)
			cancel()
			if err != nil {
				c.logger.Debug("websocket write error", "err", err, "conn_id", c.connID)
				return
			}

		case <-ticker.C:
			pingCtx, cancel := context.WithTimeout(ctx, writeWait)
			err := c.conn.Ping(pingCtx)
			cancel()
			if err != nil {
				c.logger.Debug("websocket ping failed", "err", err, "conn_id", c.connID)
				return
			}

		case <-ctx.Done():
			return
		}
	}
}

// handleMessage dispatches an incoming message by type.
func (c *canvasClient) handleMessage(ctx context.Context, msg wsMessage) {
	switch msg.Type {
	case "node:upsert":
		c.handleNodeUpsert(ctx, msg.Payload)
	case "node:delete":
		c.handleNodeDelete(ctx, msg.Payload)
	case "node:move":
		c.handleNodeMove(ctx, msg.Payload)
	case "edge:upsert":
		c.handleEdgeUpsert(ctx, msg.Payload)
	case "edge:delete":
		c.handleEdgeDelete(ctx, msg.Payload)
	case "cursor:move":
		c.handleCursorMove(msg.Payload)
	case "service:draft-start":
		c.handleServiceDraftStart(msg.Payload)
	case "service:draft-cancel":
		c.handleServiceDraftCancel(msg.Payload)
	case "service:create":
		c.handleServiceCreate(ctx, msg.Payload)
	default:
		c.sendError(fmt.Sprintf("Unknown message type: %s", msg.Type))
	}
}

func (c *canvasClient) handleNodeUpsert(ctx context.Context, payload json.RawMessage) {
	var upsert deploykit.CanvasNodeUpsert
	if err := json.Unmarshal(payload, &upsert); err != nil {
		c.sendError("Invalid node upsert payload.")
		return
	}

	node, err := c.canvasService.UpsertNode(ctx, c.projectID, upsert)
	if err != nil {
		c.logger.Error("upserting canvas node", "err", err)
		c.sendError("Failed to save node.")
		return
	}

	response, _ := json.Marshal(node)
	msg, _ := json.Marshal(wsMessage{Type: "node:upserted", Payload: response})
	c.room.broadcastAll(msg)
}

func (c *canvasClient) handleNodeDelete(ctx context.Context, payload json.RawMessage) {
	var req struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(payload, &req); err != nil || req.ID == "" {
		c.sendError("Invalid node delete payload.")
		return
	}

	// Branch on whether this node is linked to a real (applied) service.
	// If yes, the service keeps running until deploy — stage a service.delete
	// pending change but leave the canvas node in place so the diff UI can
	// render it as "pending deletion". Frontend styles it accordingly.
	//
	// If no, this is a pending-added service that was never deployed —
	// remove the node outright and drop any pending changes that reference it.
	nodes, _, err := c.canvasService.GetCanvasState(ctx, c.projectID)
	if err != nil {
		c.logger.Error("loading canvas state for node delete", "err", err)
		c.sendError("Failed to delete node.")
		return
	}
	var target *deploykit.CanvasNode
	for _, n := range nodes {
		if n.ID == req.ID {
			target = n
			break
		}
	}
	if target == nil {
		c.sendError("Node not found.")
		return
	}

	if target.Type == deploykit.CanvasNodeTypeService && target.ServiceID != nil {
		// Stage a service.delete pending change; keep the node visible.
		pcPayload := json.RawMessage(`{}`)
		pc, err := c.pendingChangeService.Append(ctx, c.projectID, deploykit.PendingChangeInput{
			Op:         deploykit.PendingOpServiceDelete,
			TargetType: deploykit.PendingTargetService,
			TargetID:   target.ServiceID,
			Payload:    pcPayload,
			UserID:     &c.userID,
		})
		if err != nil {
			c.logger.Error("staging service delete", "err", err)
			c.sendError("Failed to stage service deletion.")
			return
		}
		c.hub.broadcastPendingChangeAdded(c.projectID, pc)
		return
	}

	// Pending-added service node (no backing service yet) or a plain canvas
	// node — remove it directly and drop any pending changes keyed by its ID.
	if _, err := c.canvasService.DeleteNode(ctx, c.projectID, req.ID); err != nil {
		c.logger.Error("deleting canvas node", "err", err)
		c.sendError("Failed to delete node.")
		return
	}
	var removedChangeIDs []string
	if c.pendingChangeService != nil {
		ids, err := c.pendingChangeService.RemoveByTempID(ctx, c.projectID, req.ID)
		if err != nil {
			c.logger.Error("removing pending changes for temp id", "err", err, "temp_id", req.ID)
		}
		removedChangeIDs = ids
	}

	response, _ := json.Marshal(map[string]string{"id": req.ID})
	msg, _ := json.Marshal(wsMessage{Type: "node:deleted", Payload: response})
	c.room.broadcastAll(msg)

	if len(removedChangeIDs) > 0 {
		c.hub.broadcastPendingChangesRemoved(c.projectID, removedChangeIDs)
	}
}

func (c *canvasClient) handleNodeMove(ctx context.Context, payload json.RawMessage) {
	var req struct {
		Positions []deploykit.NodePosition `json:"positions"`
	}
	if err := json.Unmarshal(payload, &req); err != nil || len(req.Positions) == 0 {
		c.sendError("Invalid node move payload.")
		return
	}

	if err := c.canvasService.BatchUpdateNodePositions(ctx, c.projectID, req.Positions); err != nil {
		c.logger.Error("updating node positions", "err", err)
		c.sendError("Failed to update positions.")
		return
	}

	response, _ := json.Marshal(map[string]any{
		"positions": req.Positions,
		"user_id":   c.userID,
	})
	msg, _ := json.Marshal(wsMessage{Type: "node:moved", Payload: response})
	c.room.broadcast(c.connID, msg)
}

func (c *canvasClient) handleEdgeUpsert(ctx context.Context, payload json.RawMessage) {
	var upsert deploykit.CanvasEdgeUpsert
	if err := json.Unmarshal(payload, &upsert); err != nil {
		c.sendError("Invalid edge upsert payload.")
		return
	}

	edge, err := c.canvasService.UpsertEdge(ctx, c.projectID, upsert)
	if err != nil {
		c.logger.Error("upserting canvas edge", "err", err)
		c.sendError("Failed to save edge.")
		return
	}

	response, _ := json.Marshal(edge)
	msg, _ := json.Marshal(wsMessage{Type: "edge:upserted", Payload: response})
	c.room.broadcastAll(msg)
}

func (c *canvasClient) handleEdgeDelete(ctx context.Context, payload json.RawMessage) {
	var req struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(payload, &req); err != nil || req.ID == "" {
		c.sendError("Invalid edge delete payload.")
		return
	}

	if err := c.canvasService.DeleteEdge(ctx, c.projectID, req.ID); err != nil {
		c.logger.Error("deleting canvas edge", "err", err)
		c.sendError("Failed to delete edge.")
		return
	}

	response, _ := json.Marshal(map[string]string{"id": req.ID})
	msg, _ := json.Marshal(wsMessage{Type: "edge:deleted", Payload: response})
	c.room.broadcastAll(msg)
}

func (c *canvasClient) handleCursorMove(payload json.RawMessage) {
	var pos struct {
		X float64 `json:"x"`
		Y float64 `json:"y"`
	}
	if err := json.Unmarshal(payload, &pos); err != nil {
		return // Silently ignore invalid cursor messages.
	}

	response, _ := json.Marshal(map[string]any{
		"user_id":   c.userID,
		"user_name": c.userName,
		"x":         pos.X,
		"y":         pos.Y,
	})
	msg, _ := json.Marshal(wsMessage{Type: "cursor:updated", Payload: response})
	c.room.broadcast(c.connID, msg)
}

func (c *canvasClient) handleServiceDraftStart(payload json.RawMessage) {
	var req struct {
		DraftID string  `json:"draft_id"`
		X       float64 `json:"x"`
		Y       float64 `json:"y"`
	}
	if err := json.Unmarshal(payload, &req); err != nil || req.DraftID == "" {
		c.sendError("Invalid draft start payload.")
		return
	}

	draft := serviceDraft{
		DraftID:  req.DraftID,
		UserID:   c.userID,
		UserName: c.userName,
		X:        req.X,
		Y:        req.Y,
	}

	c.room.mu.Lock()
	c.room.drafts[req.DraftID] = draft
	c.room.mu.Unlock()

	response, _ := json.Marshal(draft)
	msg, _ := json.Marshal(wsMessage{Type: "service:drafting", Payload: response})
	c.room.broadcast(c.connID, msg)
}

func (c *canvasClient) handleServiceDraftCancel(payload json.RawMessage) {
	var req struct {
		DraftID string `json:"draft_id"`
	}
	if err := json.Unmarshal(payload, &req); err != nil || req.DraftID == "" {
		return
	}

	c.room.mu.Lock()
	d, existed := c.room.drafts[req.DraftID]
	// Only owner can cancel.
	if existed && d.UserID != c.userID {
		c.room.mu.Unlock()
		return
	}
	delete(c.room.drafts, req.DraftID)
	c.room.mu.Unlock()

	if !existed {
		return
	}

	response, _ := json.Marshal(map[string]string{"draft_id": req.DraftID})
	msg, _ := json.Marshal(wsMessage{Type: "service:draft-cancelled", Payload: response})
	c.room.broadcast(c.connID, msg)
}

func (c *canvasClient) handleServiceCreate(ctx context.Context, payload json.RawMessage) {
	var req struct {
		DraftID string  `json:"draft_id"`
		Name    string  `json:"name"`
		Image   string  `json:"image"`
		X       float64 `json:"x"`
		Y       float64 `json:"y"`
	}
	if err := json.Unmarshal(payload, &req); err != nil || req.DraftID == "" {
		c.sendError("Invalid service create payload.")
		return
	}
	if req.Name == "" {
		c.sendCreateError(req.DraftID, "Name is required.")
		return
	}
	if req.Image == "" {
		c.sendCreateError(req.DraftID, "Image is required.")
		return
	}

	// Place the canvas node up-front so everyone sees the pending service.
	// The node's ID doubles as the pending-change temp ID: later env var
	// edits against this not-yet-created service reference it via
	// parent_temp_id, and Apply populates service_id on this same node row.
	nodeID := uuid.New().String()
	nodeData, _ := json.Marshal(map[string]string{"image": req.Image})
	node, err := c.canvasService.UpsertNode(ctx, c.projectID, deploykit.CanvasNodeUpsert{
		ID:        nodeID,
		Type:      deploykit.CanvasNodeTypeService,
		Label:     req.Name,
		PositionX: req.X,
		PositionY: req.Y,
		Data:      string(nodeData),
	})
	if err != nil {
		c.logger.Error("creating canvas node for pending service", "err", err)
		c.sendCreateError(req.DraftID, "Failed to place service on canvas.")
		return
	}

	pcPayload, err := json.Marshal(deploykit.ServiceCreatePayload{
		Name:  req.Name,
		Image: req.Image,
	})
	if err != nil {
		c.logger.Error("marshaling service create payload", "err", err)
		c.sendCreateError(req.DraftID, "Failed to stage service creation.")
		return
	}

	pc, err := c.pendingChangeService.Append(ctx, c.projectID, deploykit.PendingChangeInput{
		Op:           deploykit.PendingOpServiceCreate,
		TargetType:   deploykit.PendingTargetService,
		TargetTempID: &nodeID,
		Payload:      pcPayload,
		UserID:       &c.userID,
	})
	if err != nil {
		c.logger.Error("staging service create", "err", err)
		// Roll back the canvas node so the user isn't left with a dangling visual.
		if _, delErr := c.canvasService.DeleteNode(ctx, c.projectID, nodeID); delErr != nil {
			c.logger.Error("rolling back canvas node after append failure", "err", delErr)
		}
		c.sendCreateError(req.DraftID, serviceCreateErrorMessage(err))
		return
	}

	// Clear the draft so other clients' ghosts disappear.
	c.room.mu.Lock()
	delete(c.room.drafts, req.DraftID)
	c.room.mu.Unlock()

	cancelledPayload, _ := json.Marshal(map[string]string{"draft_id": req.DraftID})
	cancelledMsg, _ := json.Marshal(wsMessage{Type: "service:draft-cancelled", Payload: cancelledPayload})
	c.room.broadcast(c.connID, cancelledMsg)

	nodePayload, _ := json.Marshal(node)
	nodeMsg, _ := json.Marshal(wsMessage{Type: "node:upserted", Payload: nodePayload})
	c.room.broadcastAll(nodeMsg)

	c.hub.broadcastPendingChangeAdded(c.projectID, pc)

	createdPayload, _ := json.Marshal(map[string]any{
		"draft_id":       req.DraftID,
		"node":           node,
		"pending_change": pc,
	})
	createdMsg, _ := json.Marshal(wsMessage{Type: "service:created", Payload: createdPayload})
	select {
	case c.send <- createdMsg:
	default:
	}
}

// sendCreateError sends a scoped error tied to a specific draft so the client
// can surface it inline on the right form.
func (c *canvasClient) sendCreateError(draftID, message string) {
	payload, _ := json.Marshal(map[string]string{"draft_id": draftID, "message": message})
	msg, _ := json.Marshal(wsMessage{Type: "service:create-error", Payload: payload})
	select {
	case c.send <- msg:
	default:
	}
}

// serviceCreateErrorMessage picks a user-facing message for a service-create
// failure. Known domain errors (conflict, validation) surface their message so
// the form can render it inline; anything else is a generic fallback.
func serviceCreateErrorMessage(err error) string {
	switch deploykit.ErrorCode(err) {
	case deploykit.ECONFLICT, deploykit.EINVALID:
		return deploykit.ErrorMessage(err)
	}
	return "Failed to create service."
}

// sendError sends an error message to this client.
func (c *canvasClient) sendError(message string) {
	payload, _ := json.Marshal(map[string]string{"message": message})
	msg, _ := json.Marshal(wsMessage{Type: "error", Payload: payload})
	select {
	case c.send <- msg:
	default:
	}
}
