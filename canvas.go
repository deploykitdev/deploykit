package deploykit

import (
	"context"
	"time"
)

// Canvas node type constants.
const (
	CanvasNodeTypeService = "service"
	CanvasNodeTypeLabel   = "label"
	CanvasNodeTypeGroup   = "group"
)

// CanvasNode represents a visual element on a project's canvas.
// Nodes are independent of services — some may reference a service via
// ServiceID, others are purely visual (labels, groups).
type CanvasNode struct {
	ID        string    `json:"id"`
	ProjectID string    `json:"project_id"`
	Type      string    `json:"type"`
	Label     string    `json:"label"`
	PositionX float64   `json:"position_x"`
	PositionY float64   `json:"position_y"`
	Width     *float64  `json:"width,omitempty"`
	Height    *float64  `json:"height,omitempty"`
	ServiceID *string   `json:"service_id,omitempty"`
	Data      string    `json:"data"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// CanvasEdge represents a connection between two canvas nodes.
type CanvasEdge struct {
	ID        string    `json:"id"`
	ProjectID string    `json:"project_id"`
	SourceID  string    `json:"source_id"`
	TargetID  string    `json:"target_id"`
	Label     *string   `json:"label,omitempty"`
	Data      string    `json:"data"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// CanvasService manages canvas nodes and edges for projects.
type CanvasService interface {
	// GetCanvasState returns all nodes and edges for a project.
	GetCanvasState(ctx context.Context, projectID string) ([]*CanvasNode, []*CanvasEdge, error)

	// UpsertNode creates or updates a canvas node.
	UpsertNode(ctx context.Context, projectID string, node CanvasNodeUpsert) (*CanvasNode, error)

	// DeleteNode removes a canvas node and its connected edges. If the node
	// referenced a service, returns that service ID so callers can cascade
	// the service deletion; returns nil otherwise.
	DeleteNode(ctx context.Context, projectID string, nodeID string) (*string, error)

	// UpsertEdge creates or updates a canvas edge.
	UpsertEdge(ctx context.Context, projectID string, edge CanvasEdgeUpsert) (*CanvasEdge, error)

	// DeleteEdge removes a canvas edge.
	DeleteEdge(ctx context.Context, projectID string, edgeID string) error

	// BatchUpdateNodePositions updates positions for multiple nodes in a single transaction.
	BatchUpdateNodePositions(ctx context.Context, projectID string, positions []NodePosition) error
}

// CanvasNodeUpsert holds fields for creating or updating a canvas node.
type CanvasNodeUpsert struct {
	ID        string   `json:"id"`
	Type      string   `json:"type"`
	Label     string   `json:"label"`
	PositionX float64  `json:"position_x"`
	PositionY float64  `json:"position_y"`
	Width     *float64 `json:"width,omitempty"`
	Height    *float64 `json:"height,omitempty"`
	ServiceID *string  `json:"service_id,omitempty"`
	Data      string   `json:"data"`
}

// Validate checks that all required fields are present.
func (c *CanvasNodeUpsert) Validate() error {
	ve := NewValidationErrors()
	if c.ID == "" {
		ve.Add("id", "ID is required.")
	}
	if c.Type == "" {
		ve.Add("type", "Type is required.")
	}
	return ve.Err()
}

// CanvasEdgeUpsert holds fields for creating or updating a canvas edge.
type CanvasEdgeUpsert struct {
	ID       string  `json:"id"`
	SourceID string  `json:"source_id"`
	TargetID string  `json:"target_id"`
	Label    *string `json:"label,omitempty"`
	Data     string  `json:"data"`
}

// Validate checks that all required fields are present.
func (c *CanvasEdgeUpsert) Validate() error {
	ve := NewValidationErrors()
	if c.ID == "" {
		ve.Add("id", "ID is required.")
	}
	if c.SourceID == "" {
		ve.Add("source_id", "Source ID is required.")
	}
	if c.TargetID == "" {
		ve.Add("target_id", "Target ID is required.")
	}
	return ve.Err()
}

// NodePosition is a lightweight struct for batch position updates.
type NodePosition struct {
	ID        string  `json:"id"`
	PositionX float64 `json:"position_x"`
	PositionY float64 `json:"position_y"`
}
