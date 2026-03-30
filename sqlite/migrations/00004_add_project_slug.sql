ALTER TABLE projects ADD COLUMN slug TEXT NOT NULL DEFAULT '';

UPDATE projects SET slug = lower(replace(replace(name, ' ', '-'), '.', '-')) || '-' || lower(substr(hex(randomblob(3)), 1, 6)) WHERE slug = '';

CREATE UNIQUE INDEX idx_projects_slug ON projects (slug);
