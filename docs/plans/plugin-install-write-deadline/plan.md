---
created: 2026-11-25
status: done
requirements:
  - REQ-PLUGINS-PLUGINS-001
system_design:
  - ../../specs/plugins/system-design/plugins-01.md
legacy_specs: []
---

# Fix plan for plugin URL install responses

## Overview

Return the final result of an authorized URL install after Kandev downloads
and processes the package. The plugin system owns this result for the
Settings dialog, marketplace, and direct API clients. [GitHub issue #5519](https://github.com/kdlbs/kandev/issues/5519)

The existing URL install contract is `AC-PLUGINS-PLUGINS-001.5`. The final
response behavior is `AC-PLUGINS-PLUGINS-001.22`. [Plugin requirements](../../specs/plugins/requirements/plugins.md#req-plugins-plugins-001-plugin-system)

## Scope

### In scope

- The response deadline for JSON requests to `POST /api/plugins/install`.
- A real HTTP regression for successful and failed delayed downloads.
- Public documentation for the URL install response window.

### Out of scope

- Multipart upload, filesystem sync, and background updates.
- Asynchronous install jobs and reverse proxy configuration.
- Plugin package format and activation policy.

## Reproduction

The default server write deadline is 30 seconds. The URL download can take
60 seconds before verification, extraction, persistence, and activation.
The base write deadline can expire before the handler has a result to send.
[Server setup](../../../apps/backend/internal/backendapp/httpserver.go)
[URL installer](../../../apps/backend/internal/plugins/service_install.go)

A temporary Go test used the registered plugin route, a real HTTP server, a
temporary plugin store, and the existing fake runtime. A second HTTP server
sent the first archive byte and delayed the rest by 500 ms.

| Server write timeout | Handler | Client result | Stored record | Repeat |
| --- | --- | --- | --- | --- |
| 200 ms | Current | EOF after 523 ms | Active | 409 |
| 3 s | Current | 201 after 518 ms | Active | 409 |
| 200 ms | Prototype | 201 after 511 ms on HTTP/1.1 and 520 ms on HTTP/2 | Active | 409 |
| Disabled | Prototype | 201 after 517 ms | Active | 409 |

## Technical approach

Use `http.ResponseController` in the URL branch of
`Controller.installFromRequest`. Clear a positive server write deadline at
the start of the install and defer setting a fresh one before JSON output.
Read the configured duration from `http.ServerContextKey` so the handler
can use the serving instance's timeout. The existing file preview handler
uses the same response controller on the Gin writer.
[URL install response design](../../specs/plugins/system-design/plugins-01.md#url-install-response-deadline)

The URL handler remains synchronous. Its response window covers the final
write even when verification or activation follows a slow download.
The install service continues to use its request context and download limit.

## Tests

| Criterion | Verification |
| --- | --- |
| `AC-PLUGINS-PLUGINS-001.5` | Existing `TestInstallHandlerFromURLCreatesActivePlugin`, `TestInstallHandlerMultipartUpload`, and `TestInstallHandlerDuplicateVersionReturns409` cover the installation paths and conflict. |
| `AC-PLUGINS-PLUGINS-001.22` | Add `TestInstallHandlerURLResponseSurvivesWriteTimeout` in `handlers_install_deadline_test.go`. Send a delayed success and a delayed download error through the real HTTP route. |
| `AC-PLUGINS-PLUGINS-001.22` | Existing `TestServiceInstallSpawnFailureLeavesRecordInErrorStatus` verifies the activation error record returned to the handler. |

## E2E tests

The new request runs over a real socket through Gin, `RequireAdmin`,
the URL download, package verification, and storage. It verifies the
outcome consumed by `installPluginFromUrl` on desktop and phone.
The existing desktop and phone plugin install tests cover the shared
dialog's pending state.
[Desktop plugin tests](../../../apps/web/e2e/tests/plugins/plugins.spec.ts)
[Phone plugin tests](../../../apps/web/e2e/tests/plugins/mobile-plugin-nav.spec.ts)

## Work orders

- [x] [Task 01. Reserve the write deadline for the install result](task-01-url-install-response-deadline.md)

## Verification results

Task 01 is complete. The regression first failed with EOF in both delayed
subtests, then passed after the URL handler reserved the server write deadline.
The socket test also captures the deadline when each JSON response is written
and asserts it is fresh for the delayed success, duplicate conflict, and
delayed upstream error. Temporarily removing the deferred reset made the
delayed success and error assertions fail.

- `go test -trimpath -tags fts5 ./internal/plugins -run '^(TestInstallHandler|TestServiceInstallSpawnFailureLeavesRecordInErrorStatus$)' -count=1` — passed.
- `GOCACHE=/tmp/kandev-5519-review-gocache go test -trimpath -tags fts5 ./internal/plugins -run '^TestInstallHandlerURLResponseSurvivesWriteTimeout$' -count=1` — passed after the shared cache returned missing archive errors.
- `bash scripts/lint-go-changed apps/backend/internal/plugins/handlers.go apps/backend/internal/plugins/handlers_install_deadline_test.go` — passed, 0 issues.
- `make -C apps/backend build-kandev` — passed.
- `python3 scripts/list-docs.py validate` — passed.
- `python3 scripts/lint-spec-files.py --all` — passed.
- `node --test scripts/validate-public-docs.test.mjs` — passed, 63 tests.
- `node scripts/validate-public-docs.mjs` — passed, 56 pages.
- `git diff --check` for the changed implementation, spec, plan, and public-doc paths — passed.

## Risks

- Clients and reverse proxies can enforce a shorter deadline of their own.
- The response controller requires access to the server writer through the
  Gin writer. The real HTTP regression exercises that path.
