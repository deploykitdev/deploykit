-- Canvas nodes: visual elements on a project's canvas.
CREATE TABLE canvas_nodes (
    id TEXT PRIMARY KEY,
    project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    type TEXT NOT NULL DEFAULT 'service',
    label TEXT NOT NULL DEFAULT '',
    position_x REAL NOT NULL DEFAULT 0,
    position_y REAL NOT NULL DEFAULT 0,
    width REAL,
    height REAL,
    service_id TEXT REFERENCES services(id) ON DELETE SET NULL,
    data TEXT NOT NULL DEFAULT '{}',
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX idx_canvas_nodes_project_id ON canvas_nodes(project_id);
CREATE UNIQUE INDEX idx_canvas_nodes_service_id ON canvas_nodes(service_id) WHERE service_id IS NOT NULL;

-- Canvas edges: connections between canvas nodes.
CREATE TABLE canvas_edges (
    id TEXT PRIMARY KEY,
    project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    source_id TEXT NOT NULL REFERENCES canvas_nodes(id) ON DELETE CASCADE,
    target_id TEXT NOT NULL REFERENCES canvas_nodes(id) ON DELETE CASCADE,
    label TEXT,
    data TEXT NOT NULL DEFAULT '{}',
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX idx_canvas_edges_project_id ON canvas_edges(project_id);
CREATE INDEX idx_canvas_edges_source_id ON canvas_edges(source_id);
CREATE INDEX idx_canvas_edges_target_id ON canvas_edges(target_id);
