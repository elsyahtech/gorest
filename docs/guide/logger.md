## Logger

GOREST uses Zap for structured application and framework logs. The configured
file rotates when it reaches `MaxFileSize`; `MaxBackups` and `MaxAge` limit
retained rotated files. GOREST closes the log output during `app.Close()`.
The rotating sink does not provide `fsync` durability. One
process should write to a given rotating log file.

The logger initialized by GOREST and exposed through `app.Log(...)`
automatically redacts recognized sensitive field names (including
passwords, tokens, cookies, session identifiers, credentials, and common
personal-data fields) and common message patterns such as credential
assignments, Bearer/Basic authorization values, JWTs, email addresses, and
phone numbers. Recognized values are replaced with `xxxxx`. Redaction applies
to log messages, structured fields, and fields bound with Zap's `With(...)`.
It does not affect output sent directly to other logger instances or logging
libraries.

This is a heuristic safeguard, not a guarantee that arbitrary sensitive data
will be identified. Free-form names, addresses, identifiers, custom encodings,
and secrets logged without a recognizable field name or pattern may remain
visible; false positives are also possible for generic patterns such as phone
numbers. Do not rely on automatic redaction as a substitute for data
minimization: avoid logging credentials, session data, or unnecessary personal
information, and redact application-specific data before logging it.

Zap logging methods do not return file-write errors to the business operation.
Zap reports output failures through its internal error output (stderr by
default); logging is not a durable audit trail or a transaction.

### Info:

```go
app.Log(gorest.Map{
   "content": "sample content",
}).Info("Message")
```

### Error:

```go
app.Log(gorest.Map{
   "error": err,
   "content": "sample content",
   "others":  "Others",
}).Error("Message")
```

### Warn:

```go
app.Log(gorest.Map{
   "content": "sample content",
   "others":  "Others",
}).Warn("Message")
```

### Debug:

```go
app.Log(gorest.Map{
   "content": "sample content",
   "others":  "Others",
}).Debug("Message")
```

### DPanic:

```go
app.Log(gorest.Map{
   "content": "sample content",
   "others":  "Others",
}).DPanic("Message")
```

### Fatal:

```go
app.Log(gorest.Map{
    "content": "sample content",
    "others":  "Others",
}).Fatal("Message")
```