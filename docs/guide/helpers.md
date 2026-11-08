# Helpers

GOREST provides reusable utilities in the `helper` package and in the packages
that own related functionality. These helpers are optional; they do not replace
application-specific validation, authentication, or authorization.

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

## Read environment configuration

The `helper` package includes typed environment readers for strings, integers,
booleans, durations, comma-separated string slices, file modes, and TLS minimum
versions. Parsing helpers return errors for invalid configured values; check
those errors rather than silently accepting a fallback. Readers with a default
return that default when the variable is unset or empty, and return the default
alongside an error when parsing fails.

String readers that do not say `Trimmed` preserve whitespace. This matters for
values such as passwords and tokens. Use `ReadTrimmedEnvironmentVariable` only
when surrounding whitespace should be ignored. Pointer readers are available
for optional `*string` and `*bool` configuration values; the string pointer
reader distinguishes unset from explicitly empty, while the boolean pointer
reader treats unset or empty as absent.

```go
timeout, err := helper.ReadEnvironmentDurationWithDefault("APP_TIMEOUT", 30*time.Second)
if err != nil {
	return fmt.Errorf("read APP_TIMEOUT: %w", err)
}

enabled, err := helper.ReadEnvironmentBooleanPointer("APP_FEATURE_ENABLED")
if err != nil {
	return fmt.Errorf("read APP_FEATURE_ENABLED: %w", err)
}
```

`ReadEnvironmentStringSliceWithDefault` copies the default slice when the
variable is unset. An explicitly empty variable produces an empty slice, which
can be used to clear a built-in list. `ValidateApplicationEnvironmentMode`
rejects an empty value but permits custom labels such as `staging` and `uat`,
which GOREST treats as production-like for transport security.

## Utilities in other packages

Use helpers from their owning packages rather than duplicating them in
`helper`: `database.IsDuplicateKeyError` for duplicate-key errors,
`database.NormalizeDatabaseDriver` for database driver names, and
`filter.GetClientIP` for resolving a client IP according to the filter
package's proxy-header rules. `filter.GetClientIP` is not a substitute for
configuring trusted proxies correctly.

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
