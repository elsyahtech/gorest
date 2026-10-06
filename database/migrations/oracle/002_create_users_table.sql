DECLARE
    v_count NUMBER;
BEGIN
    SELECT COUNT(*) INTO v_count 
    FROM user_tables 
    WHERE table_name = 'USERS';

    IF v_count = 0 THEN
        EXECUTE IMMEDIATE '
            CREATE TABLE users (
                id VARCHAR2(36) NOT NULL,
                email VARCHAR2(255) NOT NULL,
                password VARCHAR2(255) NOT NULL,
                department_id VARCHAR2(36) NOT NULL,
                is_active NUMBER(1) DEFAULT 1 NOT NULL,
                is_deleted NUMBER(1) DEFAULT 0 NOT NULL,
                created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP NOT NULL,
                updated_at TIMESTAMP NULL,
                deleted_at TIMESTAMP NULL,
                PRIMARY KEY (id),
                CONSTRAINT UQ_users_email UNIQUE (email),
                CONSTRAINT CHK_users_active CHECK (is_active IN (0, 1)),
                CONSTRAINT CHK_users_deleted CHECK (is_deleted IN (0, 1))
            )
        ';
    END IF;
END;