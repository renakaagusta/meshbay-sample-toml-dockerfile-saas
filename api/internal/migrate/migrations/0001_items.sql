CREATE TABLE IF NOT EXISTS items (
	id serial PRIMARY KEY,
	name text NOT NULL,
	status text NOT NULL DEFAULT 'queued' CHECK (status IN ('queued', 'processed')),
	created_at timestamptz NOT NULL DEFAULT now(),
	processed_at timestamptz
);

CREATE INDEX IF NOT EXISTS items_status_idx ON items (status);
