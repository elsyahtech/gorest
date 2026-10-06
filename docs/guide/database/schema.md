## ORACLE

### Example: Schema Migration (./database/migrations/oracle/002_create_users_table.sql)
```text
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
```

### Example: Data Seeder (./database/seeders/oracle/002_insert_users_data.sql)
```text
BEGIN
    INSERT ALL
        INTO users (id, email, password, is_active, is_deleted, created_at)
        VALUES (
            'a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a01',
            'admin1@mail.com',
            '$2a$12$eXampleHashedPasswordStringForTestingOnly',
            1,
            0, 
            CURRENT_TIMESTAMP
        )
        INTO users (id, email, password, is_active, is_deleted, created_at)
        VALUES (
            'a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a02',
            'admin2@mail.com',
            '$2a$12$eXampleHashedPasswordStringForTestingOnly',
            1,
            0,
            CURRENT_TIMESTAMP
        )
    SELECT * FROM dual;
END;
```

---

## SQL SERVER

### Example: Schema Migration (./database/migrations/sqlserver/002_create_users_table.sql)
```text
IF NOT EXISTS (SELECT * FROM sysobjects WHERE name='users' and xtype='U')
CREATE TABLE users (
    id VARCHAR(36) NOT NULL,
    email VARCHAR(255) NOT NULL,
    password VARCHAR(255) NOT NULL,
    is_active BIT NOT NULL DEFAULT 1,
    is_deleted BIT NOT NULL DEFAULT 0,
    created_at DATETIME2 NOT NULL DEFAULT GETDATE(),
    updated_at DATETIME2 NULL,
    deleted_at DATETIME2 NULL,
    PRIMARY KEY (id),
    CONSTRAINT UQ_users_email UNIQUE (email)
);
```

### Example: Data Seeder (./database/seeders/sqlserver/002_insert_users_data.sql)
```text
INSERT INTO users (id, email, password, is_active, is_deleted, created_at) 
VALUES
(
    'a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a01',
    'admin1@mail.com',
    '$2a$12$eXampleHashedPasswordStringForTestingOnly',
    1,
    0, 
    GETDATE()
),
(
    'a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a02',
    'admin2@mail.com',
    '$2a$12$eXampleHashedPasswordStringForTestingOnly',
    1,
    0,
    GETDATE()
);
```

---

## MYSQL

### Example: Schema Migration (./database/migrations/mysql/002_create_users_table.sql)
```text
CREATE TABLE IF NOT EXISTS users (
    id VARCHAR(36) NOT NULL,
    email VARCHAR(255) NOT NULL UNIQUE,
    password VARCHAR(255) NOT NULL,
    is_active TINYINT(1) NOT NULL DEFAULT 1,
    is_deleted TINYINT(1) NOT NULL DEFAULT 0,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NULL DEFAULT NULL ON UPDATE CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP NULL DEFAULT NULL,
    PRIMARY KEY (id)
);
```

### Example: Data Seeder (./database/seeders/mysql/002_insert_users_data.sql)
```text
INSERT INTO users (id, email, password, is_active, is_deleted, created_at) 
VALUES
(
    'a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a13',
    'admin1@mail.com',
    '$2a$12$eXampleHashedPasswordStringForTestingOnly001',
    1,
    0, 
    NOW()
),
(
    'b1eebc99-9c0b-4ef8-bb6d-6bb9bd380a14',
    'admin2@mail.com',
    '$2a$12$eXampleHashedPasswordStringForTestingOnly002',
    1,
    0,
    NOW()
);
```

---

## POSTGRESQL

### Example: Schema Migration (./database/migrations/postgres/002_create_users_table.sql)
```text
CREATE TABLE IF NOT EXISTS users (
    id VARCHAR(36) NOT NULL,
    email VARCHAR(255) NOT NULL UNIQUE,
    password VARCHAR(255) NOT NULL,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    is_deleted BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NULL,
    deleted_at TIMESTAMP NULL,
    PRIMARY KEY (id)
);
```

### Example: Data Seeder (./database/seeders/postgres/002_insert_users_data.sql)
```text
INSERT INTO users (id, email, password, is_active, is_deleted, created_at) 
VALUES
(
    'a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a01',
    'admin1@mail.com',
    '$2a$12$eXampleHashedPasswordStringForTestingOnly',
    TRUE,
    FALSE, 
    NOW()
),
(
    'a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a02',
    'admin2@mail.com',
    '$2a$12$eXampleHashedPasswordStringForTestingOnly',
    TRUE,
    FALSE, 
    NOW()
);
```

---

## SQLITE

### Example: Schema Migration (./database/migrations/sqlite/002_create_users_table.sql)
```text
CREATE TABLE IF NOT EXISTS users (
    id VARCHAR(36) NOT NULL,
    email TEXT NOT NULL UNIQUE,
    password TEXT NOT NULL,
    is_active INTEGER NOT NULL DEFAULT 1,
    is_deleted INTEGER NOT NULL DEFAULT 0,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT NULL,
    deleted_at DATETIME DEFAULT NULL,
    PRIMARY KEY (id)
);
```

### Example: Data Seeder (./database/seeders/sqlite/002_insert_users_data.sql)
```text
INSERT INTO users (id, email, password, is_active, is_deleted, created_at) 
VALUES 
(
    'a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a01',
    'admin1@mail.com',
    '$2a$12$eXampleHashedPasswordStringForTestingOnly',
    1,
    0, 
    CURRENT_TIMESTAMP
),
(
    'a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a02',
    'admin2@mail.com',
    '$2a$12$eXampleHashedPasswordStringForTestingOnly',
    1,
    0,
    CURRENT_TIMESTAMP
);
```

---

## MONGODB

### Example: Schema Migration (./database/migrations/mongo/002_create_users_collection.json)
```text
{
  "collection": "users",
  "validator": {
    "$jsonSchema": {
      "bsonType": "object",
      "required": [
        "id",
        "email",
        "password",
        "is_active",
        "is_deleted",
        "created_at"
      ],
      "properties": {
        "id": { "bsonType": "string" },
        "email": { "bsonType": "string" },
        "password": { "bsonType": "string" },
        "is_active": { "bsonType": "bool" },
        "is_deleted": { "bsonType": "bool" },
        "created_at": { "bsonType": "date" },
        "updated_at": { "bsonType": ["date", "null"] },
        "deleted_at": { "bsonType": ["date", "null"] }
      }
    }
  },
  "indexes": [
    {
      "key": { "id": 1 },
      "name": "pk_users_id",
      "unique": true
    },
    {
      "key": { "email": 1 },
      "name": "uk_users_email",
      "unique": true
    }
  ]
}
```

### Example: Data Seeder (./database/seeders/mongo/002_insert_users_data.json)
```text
{
  "collection": "users",
  "data": [
    {
      "id": "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a01",
      "email": "admin1@mail.com",
      "password": "$2a$12$eXampleHashedPasswordStringForTestingOnly",
      "is_active": true,
      "is_deleted": false,
      "created_at": "2026-09-21T00:00:00Z",
      "updated_at": null,
      "deleted_at": null
    },
    {
      "id": "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a02",
      "email": "admin2@mail.com",
      "password": "$2a$12$eXampleHashedPasswordStringForTestingOnly",
      "is_active": true,
      "is_deleted": false,
      "created_at": "2026-09-21T00:00:00Z",
      "updated_at": null,
      "deleted_at": null
    }
  ]
}
```

---

## SCYLLA DB (CASSANDRA)

### Example: Schema Migration (./database/migrations/scylla/002_create_users_table.cql)
```text
CREATE TABLE IF NOT EXISTS users (
    id text,
    email text,
    password text,
    is_active boolean,
    is_deleted boolean,
    created_at timestamp,
    updated_at timestamp,
    deleted_at timestamp,
    PRIMARY KEY (id)
);

CREATE INDEX IF NOT EXISTS idx_users_email ON users (email);
```

### Example: Data Seeder (./database/seeders/scylla/002_insert_users_data.cql)
```text
INSERT INTO users (id, email, password, is_active, is_deleted, created_at, updated_at) 
VALUES ('a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a01', 'admin1@mail.com', '$2a$12$eXampleHashedPasswordStringForTestingOnly', true, false, toTimestamp(now()), toTimestamp(now()));

INSERT INTO users (id, email, password, is_active, is_deleted, created_at, updated_at) 
VALUES ('a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a02', 'admin2@mail.com', '$2a$12$eXampleHashedPasswordStringForTestingOnly', true, false, toTimestamp(now()), toTimestamp(now()));
```