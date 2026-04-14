package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/heyjorgedev/deploykit"
)

// CanvasService implements deploykit.CanvasService using SQLite.
type CanvasService struct {
	db *DB
}

// NewCanvasService creates a new CanvasService backed by the given DB.
func NewCanvasService(db *DB) *CanvasService {
	return &CanvasService{db: db}
}

func (s *CanvasService) GetCanvasState(ctx context.Context, projectID string) ([]*deploykit.CanvasNode, []*deploykit.CanvasEdge, error) {
	nodes, err := s.getNodes(ctx, projectID)
	if err != nil {
		return nil, nil, err
	}

	edges, err := s.getEdges(ctx, projectID)
	if err != nil {
		return nil, nil, err
	}

	return nodes, edges, nil
}

func (s *CanvasService) getNodes(ctx context.Context, projectID string) ([]*deploykit.CanvasNode, error) {
	rows, err := s.db.db.QueryContext(ctx,
		`SELECT id, project_id, type, label, position_x, position_y, width, height, service_id, data, created_at, updated_at
		 FROM canvas_nodes WHERE project_id = ? ORDER BY created_at ASC`, projectID,
	)
	if err != nil {
		return nil, fmt.Errorf("listing canvas nodes: %w", err)
	}
	defer rows.Close()

	var nodes []*deploykit.CanvasNode
	for rows.Next() {
		n := &deploykit.CanvasNode{}
		var createdAt, updatedAt string
		var width, height sql.NullFloat64
		var serviceID sql.NullString

		if err := rows.Scan(
			&n.ID, &n.ProjectID, &n.Type, &n.Label,
			&n.PositionX, &n.PositionY, &width, &height,
			&serviceID, &n.Data, &createdAt, &updatedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning canvas node row: %w", err)
		}

		if width.Valid {
			n.Width = &width.Float64
		}
		if height.Valid {
			n.Height = &height.Float64
		}
		if serviceID.Valid {
			n.ServiceID = &serviceID.String
		}

		n.CreatedAt, _ = time.Parse(timeFormat, createdAt)
		n.UpdatedAt, _ = time.Parse(timeFormat, updatedAt)
		nodes = append(nodes, n)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating canvas node rows: %w", err)
	}

	return nodes, nil
}

func (s *CanvasService) getEdges(ctx context.Context, projectID string) ([]*deploykit.CanvasEdge, error) {
	rows, err := s.db.db.QueryContext(ctx,
		`SELECT id, project_id, source_id, target_id, label, data, created_at, updated_at
		 FROM canvas_edges WHERE project_id = ? ORDER BY created_at ASC`, projectID,
	)
	if err != nil {
		return nil, fmt.Errorf("listing canvas edges: %w", err)
	}
	defer rows.Close()

	var edges []*deploykit.CanvasEdge
	for rows.Next() {
		e := &deploykit.CanvasEdge{}
		var createdAt, updatedAt string
		var label sql.NullString

		if err := rows.Scan(
			&e.ID, &e.ProjectID, &e.SourceID, &e.TargetID,
			&label, &e.Data, &createdAt, &updatedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning canvas edge row: %w", err)
		}

		if label.Valid {
			e.Label = &label.String
		}

		e.CreatedAt, _ = time.Parse(timeFormat, createdAt)
		e.UpdatedAt, _ = time.Parse(timeFormat, updatedAt)
		edges = append(edges, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating canvas edge rows: %w", err)
	}

	return edges, nil
}

func (s *CanvasService) UpsertNode(ctx context.Context, projectID string, node deploykit.CanvasNodeUpsert) (*deploykit.CanvasNode, error) {
	if err := node.Validate(); err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	result := &deploykit.CanvasNode{
		ID:        node.ID,
		ProjectID: projectID,
		Type:      node.Type,
		Label:     node.Label,
		PositionX: node.PositionX,
		PositionY: node.PositionY,
		Width:     node.Width,
		Height:    node.Height,
		ServiceID: node.ServiceID,
		Data:      node.Data,
		UpdatedAt: now,
	}

	var width, height sql.NullFloat64
	var serviceID sql.NullString

	if node.Width != nil {
		width = sql.NullFloat64{Float64: *node.Width, Valid: true}
	}
	if node.Height != nil {
		height = sql.NullFloat64{Float64: *node.Height, Valid: true}
	}
	if node.ServiceID != nil {
		serviceID = sql.NullString{String: *node.ServiceID, Valid: true}
	}

	data := node.Data
	if data == "" {
		data = "{}"
	}

	var createdAt string
	err := s.db.db.QueryRowContext(ctx,
		`INSERT INTO canvas_nodes (id, project_id, type, label, position_x, position_y, width, height, service_id, data, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET
		   type = excluded.type,
		   label = excluded.label,
		   position_x = excluded.position_x,
		   position_y = excluded.position_y,
		   width = excluded.width,
		   height = excluded.height,
		   service_id = excluded.service_id,
		   data = excluded.data,
		   updated_at = excluded.updated_at
		 RETURNING created_at`,
		result.ID, projectID, result.Type, result.Label,
		result.PositionX, result.PositionY, width, height,
		serviceID, data,
		now.Format(timeFormat), now.Format(timeFormat),
	).Scan(&createdAt)
	if err != nil {
		return nil, fmt.Errorf("upserting canvas node: %w", err)
	}

	result.CreatedAt, _ = time.Parse(timeFormat, createdAt)
	result.Data = data

	return result, nil
}

func (s *CanvasService) DeleteNode(ctx context.Context, projectID string, nodeID string) (*string, error) {
	tx, err := s.db.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	var serviceID *string
	if err := tx.QueryRowContext(ctx,
		`SELECT service_id FROM canvas_nodes WHERE id = ? AND project_id = ?`,
		nodeID, projectID,
	).Scan(&serviceID); err != nil {
		if err == sql.ErrNoRows {
			return nil, deploykit.Errorf(deploykit.ENOTFOUND, "Canvas node not found.")
		}
		return nil, fmt.Errorf("loading canvas node %s: %w", nodeID, err)
	}

	if _, err := tx.ExecContext(ctx,
		`DELETE FROM canvas_nodes WHERE id = ? AND project_id = ?`, nodeID, projectID,
	); err != nil {
		return nil, fmt.Errorf("deleting canvas node %s: %w", nodeID, err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}

	return serviceID, nil
}

func (s *CanvasService) UpsertEdge(ctx context.Context, projectID string, edge deploykit.CanvasEdgeUpsert) (*deploykit.CanvasEdge, error) {
	if err := edge.Validate(); err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	result := &deploykit.CanvasEdge{
		ID:        edge.ID,
		ProjectID: projectID,
		SourceID:  edge.SourceID,
		TargetID:  edge.TargetID,
		Label:     edge.Label,
		UpdatedAt: now,
	}

	var label sql.NullString
	if edge.Label != nil {
		label = sql.NullString{String: *edge.Label, Valid: true}
	}

	data := edge.Data
	if data == "" {
		data = "{}"
	}

	var createdAt string
	err := s.db.db.QueryRowContext(ctx,
		`INSERT INTO canvas_edges (id, project_id, source_id, target_id, label, data, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET
		   source_id = excluded.source_id,
		   target_id = excluded.target_id,
		   label = excluded.label,
		   data = excluded.data,
		   updated_at = excluded.updated_at
		 RETURNING created_at`,
		result.ID, projectID, result.SourceID, result.TargetID,
		label, data,
		now.Format(timeFormat), now.Format(timeFormat),
	).Scan(&createdAt)
	if err != nil {
		return nil, fmt.Errorf("upserting canvas edge: %w", err)
	}

	result.CreatedAt, _ = time.Parse(timeFormat, createdAt)
	result.Data = data

	return result, nil
}

func (s *CanvasService) DeleteEdge(ctx context.Context, projectID string, edgeID string) error {
	result, err := s.db.db.ExecContext(ctx,
		`DELETE FROM canvas_edges WHERE id = ? AND project_id = ?`, edgeID, projectID,
	)
	if err != nil {
		return fmt.Errorf("deleting canvas edge %s: %w", edgeID, err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return deploykit.Errorf(deploykit.ENOTFOUND, "Canvas edge not found.")
	}

	return nil
}

func (s *CanvasService) BatchUpdateNodePositions(ctx context.Context, projectID string, positions []deploykit.NodePosition) error {
	tx, err := s.db.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx,
		`UPDATE canvas_nodes SET position_x = ?, position_y = ?, updated_at = ? WHERE id = ? AND project_id = ?`)
	if err != nil {
		return fmt.Errorf("preparing position update: %w", err)
	}
	defer stmt.Close()

	now := time.Now().UTC().Format(timeFormat)
	for _, p := range positions {
		if _, err := stmt.ExecContext(ctx, p.PositionX, p.PositionY, now, p.ID, projectID); err != nil {
			return fmt.Errorf("updating node position %s: %w", p.ID, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing position updates: %w", err)
	}

	return nil
}
