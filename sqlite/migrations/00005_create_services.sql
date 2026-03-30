-- Services: a deployable unit within a project.
CREATE TABLE services (
    id TEXT PRIMARY KEY,
    project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'created',
    active_deployment_id TEXT,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now')),
    UNIQUE(project_id, name)
);

CREATE INDEX idx_services_project_id ON services(project_id);

-- Deployments: immutable snapshots of desired state for a service.
CREATE TABLE deployments (
    id TEXT PRIMARY KEY,
    service_id TEXT NOT NULL REFERENCES services(id) ON DELETE CASCADE,
    image TEXT NOT NULL,
    env_vars TEXT NOT NULL DEFAULT '{}',
    ports TEXT NOT NULL DEFAULT '[]',
    resources TEXT,
    replicas INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX idx_deployments_service_id ON deployments(service_id);

-- Containers: tracked instances of Docker containers.
CREATE TABLE containers (
    id TEXT PRIMARY KEY,
    service_id TEXT NOT NULL REFERENCES services(id) ON DELETE CASCADE,
    deployment_id TEXT NOT NULL REFERENCES deployments(id) ON DELETE CASCADE,
    docker_container_id TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'created',
    created_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX idx_containers_service_id ON containers(service_id);
CREATE INDEX idx_containers_deployment_id ON containers(deployment_id);
