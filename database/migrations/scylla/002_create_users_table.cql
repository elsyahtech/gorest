CREATE TABLE IF NOT EXISTS users (
    id text,
    email text,
    password text,
    department_id text,
    is_active boolean,
    is_deleted boolean,
    created_at timestamp,
    updated_at timestamp,
    deleted_at timestamp,
    PRIMARY KEY (id)
);

CREATE INDEX IF NOT EXISTS idx_users_email ON users (email);
CREATE INDEX IF NOT EXISTS idx_users_department ON users (department_id);