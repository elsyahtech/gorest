IF NOT EXISTS (SELECT * FROM sysobjects WHERE name='departments' and xtype='U')
CREATE TABLE departments (
    id VARCHAR(36) NOT NULL,
    name VARCHAR(255) NOT NULL,
    description VARCHAR(MAX) NULL,
    is_active BIT NOT NULL DEFAULT 1,
    is_deleted BIT NOT NULL DEFAULT 0,
    created_at DATETIME2 NOT NULL DEFAULT GETDATE(),
    updated_at DATETIME2 NULL,
    deleted_at DATETIME2 NULL,
    PRIMARY KEY (id)
);