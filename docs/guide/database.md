## Database Query

### ORM

```go
query := db.Table("users").Find(&user)
```

Read the full [ORM Guide](orm.md) for query methods, driver support, limitations, and troubleshooting.

### Native APIs and choosing an approach

For a neutral comparison of the ORM and native APIs, including tradeoffs and examples for SQL, MongoDB, and ScyllaDB, see the [Querying Guide](querying.md).
