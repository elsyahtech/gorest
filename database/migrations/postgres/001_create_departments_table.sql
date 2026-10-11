CREATE TABLE IF NOT EXISTS departments (
    id VARCHAR(36) NOT NULL,
    name VARCHAR(255) NOT NULL,
    description TEXT NULL,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NULL,
    deleted_at TIMESTAMP NULL,
    PRIMARY KEY (id)
);

CREATE INDEX IF NOT EXISTS idx_departments_name ON departments (name);