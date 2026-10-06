IF NOT EXISTS (SELECT * FROM sysobjects WHERE name='orders' and xtype='U')
CREATE TABLE orders (
    id VARCHAR(36) NOT NULL,
    user_id VARCHAR(36) NOT NULL,
    order_number VARCHAR(50) NOT NULL,
    total_amount DECIMAL(10, 2) NOT NULL DEFAULT 0.00,
    status VARCHAR(50) NOT NULL DEFAULT 'PENDING',
    
    is_active BIT NOT NULL DEFAULT 1,
    is_deleted BIT NOT NULL DEFAULT 0,
    created_at DATETIME2 NOT NULL DEFAULT GETDATE(),
    updated_at DATETIME2 NULL,
    deleted_at DATETIME2 NULL,
    
    PRIMARY KEY (id),
    CONSTRAINT FK_orders_users FOREIGN KEY (user_id) REFERENCES users(id)
);