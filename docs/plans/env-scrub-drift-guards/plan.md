---
created: 2026-09-14
status: done
requirements:
  - REQ-INTEGRATIONS-GITLAB-INTEGRATION-001
  - REQ-INTEGRATIONS-GITHUB-AUTHENTICATION-001
system_design:
  - ../../specs/integrations/system-design/gitlab-integration-02.md
  - ../../specs/integrations/system-design/github-authentication-01.md
legacy_specs: []
---

# Implementation plan: Environment scrub drift guards

## Overview

Keep the hermetic environment scrubs used by the GitHub and GitLab integration
test suites from silently falling behind the environment reads those packages
actually perform. One shared scanner in `internal/testutil` resolves the
file-local binding of the `os` package, so an aliased or dot-imported
`os.Getenv` / `os.LookupEnv` call is classified the same way as a plain
`os.Getenv` call instead of bypassing the guard.

The integrations system owns this package because the guarded scrubs exist to
make the providers' environment-credential fallbacks deterministic under test:
an ambient `GITLAB_TOKEN` or `GH_TOKEN` from a developer shell would otherwise
silently activate those fallbacks and change which connection path a test
exercises.

## Scope

In scope: import-aware classification of `os` environment reads in the shared
scanner, adopting it for the GitHub guard, and regression coverage for every
classified import form.

Out of scope: production credential resolution, new environment fallbacks,
bulk `os.Environ`-style unnamed reads, non-`os` accessors such as
`syscall.Getenv`, and public user documentation.

## Technical approach

The original guard matched the literal identifier `os`, which left aliased and
dot-imported calls unscanned. `AssertEnvReadsCovered` now resolves identifier
bindings with `go/types` per file: a selector counts as an environment read
when its package identifier is bound to import path `os`, and a bare
`Getenv` / `LookupEnv` call counts when the identifier resolves to the `os`
package function. Type errors in unrelated package code do not stop the scan
because only import bindings are consulted.

`internal/github`'s package-local guard is replaced by the shared scanner, so
the GitHub and GitLab/process scrubs now follow one classification rule.

## Tests

| Criteria | Evidence |
| --- | --- |
| Aliased `os` import reads are reported as uncovered | `envscan_test.go` import-variant fixtures |
| Dot-imported `Getenv` / `LookupEnv` count as reads | `envscan_test.go` dot-import fixtures |
| Plain imports, extra readers, and non-`os` selectors stay unchanged | Existing `envscan_test.go` suite plus `internal/github` and `internal/gitlab` hermetic suites |

Red-green evidence: the aliased-import regression failed before the scanner
change and passed after it; the remaining import-variant tests pass against
the shared scanner.

## Work orders

- [x] [Task 01: Scan aliased os environment reads](task-01-scan-aliased-os-reads.md)

## Verification results

Commands run from `apps/backend` with the workspace GOMODCACHE/GOCACHE:

- `go test -count=1 ./internal/testutil ./internal/github/... ./internal/gitlab/...`: passed.
- `gofmt -l` on changed files: clean.

Existing referenced requirement and design documents are unchanged; they
qualify at the PR revision as-is.

## Risks

The scanner still does not name bulk unnamed environment reads or non-`os`
accessors; those channels remain outside a name-keyed scrub list and are
guarded by `extraReaders` and review instead.

## References

- [GitLab requirements](../../specs/integrations/requirements/gitlab-integration.md)
- [GitLab system design](../../specs/integrations/system-design/gitlab-integration-02.md)
- [GitHub authentication requirements](../../specs/integrations/requirements/github-authentication.md)
- [GitHub authentication system design](../../specs/integrations/system-design/github-authentication-01.md)
