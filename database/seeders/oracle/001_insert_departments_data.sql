BEGIN
    INSERT ALL
        INTO departments (id, name, description, is_active, is_deleted, created_at) VALUES ('dept-01', 'Finance', 'Finance departemen', 1, 0, CURRENT_TIMESTAMP)
        INTO departments (id, name, description, is_active, is_deleted, created_at) VALUES ('dept-02', 'Human Resource', 'Human resource departemen', 1, 0, CURRENT_TIMESTAMP)
        INTO departments (id, name, description, is_active, is_deleted, created_at) VALUES ('dept-03', 'Marketing', 'Marketing departemen', 1, 0, CURRENT_TIMESTAMP)
        INTO departments (id, name, description, is_active, is_deleted, created_at) VALUES ('dept-04', 'Operational', 'Operational Department', 1, 0, CURRENT_TIMESTAMP)
        INTO departments (id, name, description, is_active, is_deleted, created_at) VALUES ('dept-05', 'Warehouse', 'Warehouse departemen', 1, 0, CURRENT_TIMESTAMP)
    SELECT * FROM dual;
END;