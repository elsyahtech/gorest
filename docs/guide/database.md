## Database Query

### ORM

```go
query := db.Table("users").Find(&user)
```

Read detail ORM Query

### Native

```go
query := `
        SELECT id, email, password, is_active, is_deleted, created_at, updated_at, deleted_at
        FROM users where deleted_at IS NULL
    `
db.QuerySQL(ctx, query)
```

Read detail native query