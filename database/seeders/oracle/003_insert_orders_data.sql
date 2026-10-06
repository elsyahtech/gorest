BEGIN
    INSERT ALL
        INTO orders (id, user_id, order_number, total_amount, status, is_active, is_deleted, created_at) 
        VALUES 
        (
            'ord-001',
            'a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a01',
            'INV/2026/09/001',
            150000.00,
            'COMPLETED',
            1,
            0,
            CURRENT_TIMESTAMP
        )
        INTO orders (id, user_id, order_number, total_amount, status, is_active, is_deleted, created_at) 
        VALUES (
            'ord-002',
            'a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a01',
            'INV/2026/09/002',
            250050.50, -- disesuaikan presisi desimal jika perlu
            'PENDING',
            1,
            0,
            CURRENT_TIMESTAMP
        )
        INTO orders (id, user_id, order_number, total_amount, status, is_active, is_deleted, created_at) 
        VALUES (
            'ord-003', 
            'a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a02',
            'INV/2026/09/003', 
            75000.00, 
            'COMPLETED',
            1, 
            0, 
            CURRENT_TIMESTAMP
        )
        INTO orders (id, user_id, order_number, total_amount, status, is_active, is_deleted, created_at) 
        VALUES (
            'ord-004', 
            'a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a02',
            'INV/2026/09/004', 
            320000.00, 
            'CANCELED',
            1, 
            0, 
            CURRENT_TIMESTAMP
        )
        INTO orders (id, user_id, order_number, total_amount, status, is_active, is_deleted, created_at) 
        VALUES (
            'ord-005', 
            'a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a03',
            'INV/2026/09/005', 
            1250000.00, 
            'COMPLETED',
            1, 
            0, 
            CURRENT_TIMESTAMP
        )
        INTO orders (id, user_id, order_number, total_amount, status, is_active, is_deleted, created_at) 
        VALUES (
            'ord-006', 
            'a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a04',
            'INV/2026/09/006', 
            450000.00, 
            'PENDING',
            1, 
            0, 
            CURRENT_TIMESTAMP
        )
        INTO orders (id, user_id, order_number, total_amount, status, is_active, is_deleted, created_at) 
        VALUES (
            'ord-007', 
            'a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a05',
            'INV/2026/09/007', 
            90000.00, 
            'COMPLETED',
            1, 
            0, 
            CURRENT_TIMESTAMP
        )
        INTO orders (id, user_id, order_number, total_amount, status, is_active, is_deleted, created_at) 
        VALUES (
            'ord-008', 
            'a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a06',
            'INV/2026/09/008', 
            100000.00, 
            'PENDING',
            1, 
            0, 
            CURRENT_TIMESTAMP
        )
        INTO orders (id, user_id, order_number, total_amount, status, is_active, is_deleted, created_at) 
        VALUES (
            'ord-009', 
            'a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a06',
            'INV/2026/09/009', 
            200000.00, 
            'CANCELED',
            1, 
            0, 
            CURRENT_TIMESTAMP
        )
        INTO orders (id, user_id, order_number, total_amount, status, is_active, is_deleted, created_at) 
        VALUES (
            'ord-010', 
            'a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a07',
            'INV/2026/09/010', 
            550000.00, 
            'COMPLETED',
            1, 
            0, 
            CURRENT_TIMESTAMP
        )
        INTO orders (id, user_id, order_number, total_amount, status, is_active, is_deleted, created_at) 
        VALUES (
             'ord-011', 
            'a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a08',
            'INV/2026/09/011', 
            110000.00, 
            'PENDING',
            1, 
            0, 
            CURRENT_TIMESTAMP
        )
        INTO orders (id, user_id, order_number, total_amount, status, is_active, is_deleted, created_at) 
        VALUES (
            'ord-012', 
            'a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a09',
            'INV/2026/09/012', 
            850000.00, 
            'COMPLETED',
            1, 
            0, 
            CURRENT_TIMESTAMP
        )
    SELECT * FROM dual;
END;