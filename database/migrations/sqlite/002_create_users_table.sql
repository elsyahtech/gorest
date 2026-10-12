CREATE TABLE IF NOT EXISTS users (
    id VARCHAR(36) NOT NULL,
    email TEXT NOT NULL UNIQUE,
    password TEXT NOT NULL,
    department_id VARCHAR(36) NOT NULL,
    is_active INTEGER NOT NULL DEFAULT 1,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT NULL,
    deleted_at DATETIME DEFAULT NULL,
    PRIMARY KEY (id)
);

CREATE INDEX IF NOT EXISTS idx_users_department_id ON users (department_id);