CREATE TABLE IF NOT EXISTS orders (
    id text,
    user_id text,
    order_number text,
    total_amount decimal,
    status text,
    is_active boolean,
    is_deleted boolean,
    created_at timestamp,
    updated_at timestamp,
    deleted_at timestamp,
    PRIMARY KEY (id)
);

CREATE INDEX IF NOT EXISTS idx_orders_user_id ON orders (user_id);
CREATE INDEX IF NOT EXISTS idx_orders_order_number ON orders (order_number);