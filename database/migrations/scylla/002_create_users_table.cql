CREATE TABLE IF NOT EXISTS users (
    department_id text,
    created_at timestamp,
    id text,
    email text,
    password text,
    is_active boolean,
    updated_at timestamp,
    deleted_at timestamp,
    PRIMARY KEY ((department_id), created_at, id)
) WITH CLUSTERING ORDER BY (created_at DESC, id ASC);

-- Buat index tambahan jika masih ingin mencari berdasarkan email (opsional)
CREATE INDEX IF NOT EXISTS idx_users_email ON users (email);