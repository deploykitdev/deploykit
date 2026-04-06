ALTER TABLE users ADD COLUMN role TEXT NOT NULL DEFAULT 'member';

-- Promote the first-ever registered user to admin.
UPDATE users SET role = 'admin'
WHERE id = (SELECT id FROM users ORDER BY created_at ASC LIMIT 1);
