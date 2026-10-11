CREATE TABLE IF NOT EXISTS departments (
    id VARCHAR(36) NOT NULL,
    name TEXT NOT NULL,
    description TEXT NULL,
    is_active INTEGER NOT NULL DEFAULT 1,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT NULL,
    deleted_at DATETIME DEFAULT NULL,
    PRIMARY KEY (id)
    INDEX idx_departments_name (name)
);