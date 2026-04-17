-- Optional user-supplied icon image URL for a service, rendered on the canvas
-- tile and the detail panel header. NULL means use the default Container icon.
ALTER TABLE services ADD COLUMN icon_url TEXT;
