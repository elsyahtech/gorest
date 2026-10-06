CREATE TABLE IF NOT EXISTS migration_history (
    migration_name text,
    executed_at timestamp,
    PRIMARY KEY (migration_name)
);

CREATE TABLE IF NOT EXISTS departments (
    id text,
    name text,
    description text,
    is_active boolean,
    is_deleted boolean,
    created_at timestamp,
    updated_at timestamp,
    deleted_at timestamp,
    PRIMARY KEY (id)
);