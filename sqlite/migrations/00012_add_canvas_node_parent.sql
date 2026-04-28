-- Adds parent_id to canvas_nodes so child nodes can be grouped inside a
-- type='group' container. ON DELETE SET NULL orphans children when their
-- group is deleted (rather than cascading) so service-typed children retain
-- their pending-delete flow.
ALTER TABLE canvas_nodes
    ADD COLUMN parent_id TEXT
    REFERENCES canvas_nodes(id) ON DELETE SET NULL;

CREATE INDEX idx_canvas_nodes_parent_id ON canvas_nodes(parent_id);
