# Helper package

The `helper` package contains small request, conversion, and hashing utilities.
These helpers are optional; they do not replace application-specific
validation, authentication, or authorization.

## Parse request bodies

`helper.BodyRequest` binds the incoming body to a destination. Pass a pointer
to the request value and handle the returned status and error:

```go
var input CreateUserRequest
message, status, err := helper.BodyRequest(ctx, &input)
if err != nil {
	return ctx.Status(status).SendString(message)
}
```

Check that the client sends the content type expected by the endpoint, that
the body is valid JSON when using JSON, and that decoded fields receive the
application's required validation. Parsing a body does not validate business
rules.

## Convert values

The package provides string-to-integer and time-to-string helpers, including
pointer variants for optional values. Conversion functions return a result,
diagnostic message, HTTP status, and error. Check the error before using the
result; optional pointer conversions return `nil` for absent values.

Use endpoint-specific validation and error responses rather than exposing
internal error details directly to clients.

## Password hashes and digests

Use `GenerateArgon` and `VerifyArgon` or `GenerateBcrypt` and `VerifyBcrypt`
for password hashing and verification. Argon2id uses the parameters defined
by GOREST; bcrypt has its standard 72-byte input limit. Account creation,
login throttling, password policy, reset flows, and credential storage remain
application responsibilities.

`GenerateMD5`, `VerifyMD5`, and the SHA-512 digest helpers are not password
hashing functions. MD5 is deprecated; unkeyed SHA-512 is fast and unsalted.
Use these only for appropriate non-secret checksums or legacy compatibility,
not passwords, API keys, or authentication tokens.

For exact exported function signatures, see the [`helper` API reference](../api-reference.md#api-reference).
