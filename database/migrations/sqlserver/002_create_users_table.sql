IF NOT EXISTS (SELECT * FROM sysobjects WHERE name='users' and xtype='U')
CREATE TABLE users (
    id VARCHAR(36) NOT NULL,
    email VARCHAR(255) NOT NULL,
    password VARCHAR(255) NOT NULL,
    department_id VARCHAR(36) NOT NULL,
    is_active BIT NOT NULL DEFAULT 1,
    created_at DATETIME2 NOT NULL DEFAULT GETDATE(),
    updated_at DATETIME2 NULL,
    deleted_at DATETIME2 NULL,
    PRIMARY KEY (id),
    CONSTRAINT UQ_users_email UNIQUE (email)
);

IF NOT EXISTS (SELECT * FROM sys.indexes WHERE name = 'IX_users_department_id' AND object_id = OBJECT_ID('users'))
BEGIN
    CREATE INDEX IX_users_department_id ON users(department_id);
END;