CREATE TABLE IF NOT EXISTS departments (
    id text,
    name text,
    description text,
    is_active boolean,
    created_at timestamp,
    updated_at timestamp,
    deleted_at timestamp,
    PRIMARY KEY ((name), created_at, id)
) WITH CLUSTERING ORDER BY (created_at DESC, id ASC);