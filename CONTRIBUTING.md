# Contributing to GOREST

Thank you for your interest in GOREST. Contributions are welcome for review,
but submitting an issue or pull request does not guarantee that a change will
be accepted or that a contributor will be given ongoing access to the project.

GOREST is maintained at the owner's discretion. The owner reviews proposals and
selects contributors based on the project's needs, the proposed scope, and
readiness to maintain the change. The owner may request changes, defer or
decline a proposal, close a pull request, or limit repository access. This
process helps protect the project's stability, security, and direction.

## Contribution principles

Contributions should address a clear user or maintenance need and fit GOREST's
existing architecture and public API. Consider backward compatibility,
security, performance, and long-term maintenance as part of the design.
Changes that affect framework behavior should include focused tests and
documentation.

The knowledge needed depends on the change. For example, a database adapter
may require Go, SQL/CQL, and driver knowledge; an HTTP feature may require
understanding of request handling and middleware. Contributors are not
expected to know every part of the framework, but should be prepared to
explain and test the area they propose to change. See the [architecture
overview](./docs/architecture.md) before proposing a change that spans
multiple components.

## Before making a change

- For substantial changes, open an issue first. Describe the
  problem, proposed solution, compatibility impact, and alternatives.
- Wait for maintainer feedback before investing in large changes. An open
  issue is not approval to begin implementation or a promise to merge.
- Small, focused fixes may be submitted directly as a pull request.
- Do not start work that depends on write access. Repository access is granted
  separately and only when the owner decides it is appropriate.
- Use a separate branch for proposed work and keep each pull request focused
  on one coherent change.

## Pull requests

Pull requests should:

- Explain the problem and the behavior being changed.
- Stay focused; separate unrelated changes.
- Include or update tests for behavior changes.
- Update relevant documentation and examples.
- Identify breaking changes, migration needs, and limitations.
- Avoid including credentials, personal data, generated reports, or unrelated
  files.

The owner may ask for revisions or additional evidence before reviewing a
change further. A pull request can be declined even when its implementation is
technically correct, for example if it does not fit the project's direction,
maintenance capacity, or release plan.

## Local checks

Use the Go version declared in [`go.mod`](./go.mod). Before submitting, run
the checks relevant to your changes, typically:

```sh
go test ./...
go vet ./...
git diff --check
```

Database integration suites require their configured services. See the
[integration test guide](./integration_test/README.md) for setup and commands.
Do not report a skipped or unconfigured service test as a successful
integration run.

## Security issues

Do not report suspected vulnerabilities in public issues or pull requests.
Follow the private reporting process in [`SECURITY.md`](./SECURITY.md).

## Review and project access

All contributions are subject to review and explicit owner approval. Being
listed as a contributor or having a pull request merged does not automatically
grant maintainer status, organization membership, or repository write access.
The owner makes those decisions separately.
