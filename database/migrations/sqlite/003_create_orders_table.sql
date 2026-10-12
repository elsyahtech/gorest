CREATE TABLE IF NOT EXISTS orders (
    id VARCHAR(36) NOT NULL,
    user_id VARCHAR(36) NOT NULL,
    order_number VARCHAR(50) NOT NULL,
    total_amount DECIMAL(10, 2) NOT NULL DEFAULT 0.00,
    status VARCHAR(50) NOT NULL DEFAULT 'PENDING',
    
    is_active INTEGER NOT NULL DEFAULT 1,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT NULL,
    deleted_at DATETIME DEFAULT NULL,
    
    PRIMARY KEY (id),
    FOREIGN KEY (user_id) REFERENCES users(id)
);

CREATE INDEX IF NOT EXISTS uk_orders_order_number ON orders (order_number);