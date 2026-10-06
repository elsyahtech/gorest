## Logger

### Info:

```go
app.Log(gorest.Map{
   "content": "sample content":
}).Info("Message")
```

### Error:

```go
app.Log(gorest.Map{
   "error": err,
   "content": "sample content":
   "others": "Others",
}).Error("Message")
```

### Warn:

```go
app.Log(gorest.Map{
   "content": "sample content":
   "others": "Others",
}).Wan("Message")
```

### Debug:

```go
app.Log(gorest.Map{
   "content": "sample content":
   "others": "Others",
}).Debug("Message")
```

### DPanic:

```go
app.Log(gorest.Map{
   "content": "sample content":
   "others": "Others",
}).DPanic("Message")
```

### Fatal:

```go
app.Log(gorest.Map{
    "content": "sample content":
    "others": "Others",
}).Fatal("Message")
```