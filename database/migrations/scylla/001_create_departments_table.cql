CREATE TABLE IF NOT EXISTS departments (
    id text,
    name text,
    description text,
    is_active boolean,
    created_at timestamp,
    updated_at timestamp,
    deleted_at timestamp,
    PRIMARY KEY (id)
);

CREATE INDEX IF NOT EXISTS idx_departments_name ON users (name);