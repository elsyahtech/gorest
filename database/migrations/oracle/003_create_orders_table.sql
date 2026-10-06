DECLARE
    v_count NUMBER;
BEGIN
    -- Cek apakah tabel 'ORDERS' sudah ada di skema pengguna
    SELECT COUNT(*) INTO v_count 
    FROM user_tables 
    WHERE table_name = 'ORDERS';

    IF v_count = 0 THEN
        EXECUTE IMMEDIATE '
            CREATE TABLE orders (
                id VARCHAR2(36) NOT NULL,
                user_id VARCHAR2(36) NOT NULL,
                order_number VARCHAR2(50) NOT NULL,
                total_amount DECIMAL(10, 2) DEFAULT 0.00 NOT NULL,
                status VARCHAR2(50) DEFAULT ''PENDING'' NOT NULL,
                
                is_active NUMBER(1) DEFAULT 1 NOT NULL,
                is_deleted NUMBER(1) DEFAULT 0 NOT NULL,
                created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP NOT NULL,
                updated_at TIMESTAMP NULL,
                deleted_at TIMESTAMP NULL,
                
                PRIMARY KEY (id),
                CONSTRAINT FK_orders_users FOREIGN KEY (user_id) REFERENCES users(id),
                CONSTRAINT CHK_orders_active CHECK (is_active IN (0, 1)),
                CONSTRAINT CHK_orders_deleted CHECK (is_deleted IN (0, 1))
            )
        ';
    END IF;
END;