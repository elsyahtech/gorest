# Getting help and reporting issues

Before asking for help, check the [documentation index](README.md) and
[troubleshooting guide](troubleshoot.md). Confirm the GOREST version and
review the relevant configuration and error details.

## Report a bug or request a feature

Use [GitHub Issues](https://github.com/elsyahtech/gorest/issues) for
reproducible bugs, unexpected behavior, and feature requests. For usage and
configuration questions, open an issue and clearly identify it as a question
in the title.

Include only information relevant to reproducing and understanding the
request:

- GOREST version or commit and Go version.
- Operating system and architecture.
- Database driver, database product/version, and relevant driver options.
- Sanitized environment/configuration values; do not include secret values.
- Minimal reproduction steps and a small example when practical.
- Expected and actual behavior.
- Relevant sanitized errors and logs.

Remove credentials, access tokens, private keys, internal hostnames, personal
data, and production data. A small disposable test case is preferable to a
production dump.

Use a bug report for behavior that differs from documented or expected
behavior. Use a feature request to describe the problem and desired capability
without assuming a particular implementation. For a technical question,
describe what you are trying to do, the configuration involved, and what you
have already checked.

## Security reports

Do not disclose suspected vulnerabilities in a public issue or pull request.
Follow the private process in [SECURITY.md](../SECURITY.md).

## Response expectations

GOREST is an open-source project. Maintainer and contributor assistance
depends on availability and project priorities; there is no general support
SLA or guaranteed response time. The security policy's acknowledgement target
is an objective for private vulnerability reports, not a general support
commitment.
