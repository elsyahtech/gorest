# Versions and change history

GOREST is distributed as a Go module. Use a tagged version for reproducible
application builds and review release information before upgrading.

## Select or inspect a version

In an application module, select a specific GOREST version with:

```sh
go get github.com/elsyahtech/gorest@v1.0.6
```

Replace the example tag with the version selected for your application.
Available repository tags are listed on
[GitHub](https://github.com/elsyahtech/gorest/tags). To inspect a dependency's
selected version from your application module, run:

```sh
go list -m github.com/elsyahtech/gorest
go list -m -versions github.com/elsyahtech/gorest
```

For the API of the selected module version, use its version selector on
[pkg.go.dev](https://pkg.go.dev/github.com/elsyahtech/gorest).

## Review changes before upgrading

This repository does not currently include a standalone `CHANGELOG.md`.
Review the GitHub tag history, release information when available, and commit
changes between the version you use and the version you plan to adopt. Check
for API and behavior changes, update the version in a test branch, and run the
unit and relevant integration suites before deploying.

When an upgrade affects database behavior, migrations, TLS, or application
lifecycle, validate it against a disposable or staging environment before
updating production.
