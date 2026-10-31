# Database Query Overview

Use the ORM for mapped models and common query operations:

```go
query := db.Table("users").Find(&user)
```

For available operations and driver-specific limitations, see the
[ORM reference](orm.md).

Use native APIs when you need direct driver access or database-specific
features. The [querying guide](querying.md) compares both approaches and
provides examples for SQL, MongoDB, and ScyllaDB.
