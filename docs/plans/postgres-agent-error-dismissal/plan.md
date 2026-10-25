---
created: 2026-10-18
status: implemented
requirements:
  - REQ-TASKS-AGENT-ERROR-DISMISSAL-001
system_design:
  - ../../specs/tasks/system-design/agent-error-dismissal.md
legacy_specs: []
---

# Implementation Plan: PostgreSQL Agent Error Dismissal

## Overview

Restore the existing task-session agent-error acknowledgment on PostgreSQL
while preserving SQLite behavior. One sequential work order owns the regression
tests, conditional metadata update, focused verification, and resource cleanup.
Local implementation is complete through its single work order. Complete
hosted validation remains a delivery requirement.

Tasks owns the operation because it persists the session error. Existing
Git-push dismissal explicitly excludes this session-level contract, and shared
Platform portability leaves domain operations with their domain owners. The
focused [requirement](../../specs/tasks/requirements/agent-error-dismissal.md)
and [design](../../specs/tasks/system-design/agent-error-dismissal.md) are one
vertical pair; no owner boundary or catalog policy changes are needed.

## Scope

### In scope

- Only `Repository.DismissLastAgentError` and its message/time predicates.
- Real SQLite and env-gated PostgreSQL behavior coverage with actual model
  readback, preserved metadata, and unchanged-row assertions.
- Existing caller, authorization, stamp, notification, and notice contracts as
  compatibility constraints.

### Out of scope

- Broad SQL porting, shared dialect rewrites, schema, flags, ACP, launch/recovery.
- UI, public API corrections, new copy, optimistic acknowledgment, and other
  dismissal operations, including Git-push message dismissal.
- Reusing private qualification source or replaying the qualification harness.

## Root cause and qualification

The actual repository method unconditionally uses `json_set`, `json_extract`,
and `julianday`. PostgreSQL returns SQLSTATE 42883 for
`json_extract(text, unknown)`, leaving the current error undismissed when read
through the actual repository and `LastAgentError` consumer. SQLite succeeds;
real PostgreSQL metadata writers and readers succeed before dismissal. The
HTTP handler and service call this method; `LastAgentErrorNotice` hides only
after API success.

Qualification was supplied by ROOT and inspected through the five allowed
receipts under `/tmp/kandev-root-error-dismiss-discovery/toolchain-corrected/`:
`qualified-proof.json`, `receipt.json`, `stdout.log`, `stderr.log`, and
`source-preflight.json`. The corrected installed-Go-1.26 run is causal; the
earlier default-Go-1.22 attempt failed before tests. Receipt claims include
joined native handle 99511, Go exit 1, SQLite control pass, owned PostgreSQL
removal, no anonymous volumes, and absent owned process group. These are
historical qualification receipts, not this package's implementation results.
ROOT's later admission states the eight dependency blobs remain unchanged.
Do not read, copy, execute, or delete ROOT's `candidate_test.go` or `run.py`.

## Technical approach

Keep the payload and return behavior in `session.go`. Assemble the setter with
`metadataKeyUpdateQuery` / `metadataKeyUpdateArgs` and the nested extraction
with `dialect.JSONExtractPath`. Keep SQLite's current exact-text-or-julianday
guard; use PostgreSQL's exact-text-or-timestamptz equality. Bind all data and
rebind the complete guarded update once. The only Go production change is the
owning method. Remove only its obsolete exact SQL-guard exemption after the
guard recognizes the portable operation; no guard policy changes. Private
helper extraction is justified only if the existing function limit requires it.

| Database / transport | Identity and behavior | Evidence / fallback |
| --- | --- | --- |
| SQLite / existing task HTTP API | Same session, message and time; existing timestamp equivalence | Real SQLite operation and modeled readback; existing failure propagation |
| PostgreSQL pgx / same API | Same guards; bound JSONB update and zoned timestamp comparison | Disposable PostgreSQL 16, real repository/model readback; errors stay errors |

There is no provider-specific or ACP path. Supported valid model timestamps are
the compatibility domain; malformed metadata repair is excluded. Empty request
stamp behavior and repeated dismissal timestamps are unchanged.

## Tests

| Acceptance | Planned evidence |
| --- | --- |
| AC-TASKS-AGENT-ERROR-DISMISSAL-001.1 | New `session_agent_error_dismissal_test.go`: `TestDismissLastAgentErrorSQLiteBehavior` and shared fixture cases; new `session_agent_error_dismissal_postgres_test.go`: `TestDismissLastAgentErrorPostgresBehavior`. Assert exact supplied dismissal time, modeled error fields, unrelated metadata, a changed update time, and real `IsDismissed` readback. |
| AC-TASKS-AGENT-ERROR-DISMISSAL-001.2 | Same table: message-only replacement at identical time; time-only replacement at identical message; both changed; missing session; absent key; SQL NULL / JSON null metadata. Assert false/no error for ordinary missing targets, unchanged current error and unchanged `updated_at`; another session remains untouched. |
| AC-TASKS-AGENT-ERROR-DISMISSAL-001.3 | Same table: canonical exact text, `.310Z` versus `.31Z`, trailing zeros, equivalent explicit nonzero offset, and same-message 1ms-separated timestamps in the same second. Retain existing `TestDismissLastAgentErrorMatchesEquivalentTimestampText` and `TestDismissLastAgentErrorDoesNotOverwriteNewerError`. |
| AC-TASKS-AGENT-ERROR-DISMISSAL-001.4 | Existing `TestService_DismissLastAgentError`, `TestService_DismissLastAgentErrorAcceptsBrowserTimestampStamp`, `TestService_DismissLastAgentErrorIgnoresStaleStamp`, and `TestSessionScopingDismissLastAgentError` document unchanged service boundaries. No service/handler edits or new authorization mechanism. |
| AC-TASKS-AGENT-ERROR-DISMISSAL-001.5 | Repository/model readback proves the durable acknowledgment consumed by the existing API; inspect unchanged HTTP/service/notice success and failure wiring. PostgreSQL RED must fail on the actual operation, GREEN must return a dismissed modeled record. |

Tests must be authored after implementation release, independently of the ROOT
qualification source. Keep new coverage in small files, not the oversized
`session_test.go`; reuse existing fixture helpers where appropriate.

## E2E boundary

There are no rendered UI or interaction changes and no new Playwright project
or browser run in this work package. AC-TASKS-AGENT-ERROR-DISMISSAL-001.5 is
covered at the changed persistence boundary with actual writer, operation,
reader, and model consumer, plus the existing unchanged caller wiring above.
This is not a claim of fresh browser or PostgreSQL-backed HTTP runtime proof.
An unrelated stale-response or optimistic-hiding correction is excluded.

## Work orders

- [x] [Task 01: Port the guarded dismissal operation](task-01-port-dismissal.md)

Wave 1 completed sequentially after the explicit implementation release.

## Verification results

Design checkpoint: catalog validation and full specification lint passed; both
new specifications are catalog-discovered. The repository PR-documentation
coverage helper accepted the four-file chain against the planned production
path, with all five AC references and the work-order backlink valid. Direct
untracked-file whitespace, local-link, shell-syntax, and size checks passed.
No runtime implementation evidence is claimed.

Focused RED/GREEN used Go 5m / GNU 6m bounds with installed Go 1.26, race,
trimpath, fts5, one build worker and count 1. The clean causal RED reached the
actual PostgreSQL 42883 while all twelve SQLite cases and two existing controls
passed. GREEN passed four top-level tests and all 24 new cross-dialect subtests.
Scoped changed-Go lint passed. The full SQL source scan passed after removal
of only the corrected method's obsolete exact exemption.

The single complete local owning-package attempt ran with the PostgreSQL DSN
unset and exited 1 on Go's ten-minute alarm (600.153s). Its terminal stack was
in `TestArchiveTaskPurgesDurableQueueRecovery`, running for zero seconds during
`NewWithDB` / `initTaskTransferSchema`. No dismissal assertion failure was
logged. This is incomplete validation, not a package pass or demonstrated
application defect. Native handle 89186 was joined with exit 1, and both
recorded process groups were absent. No local broad rerun or budget extension.

Complete owning-package validation is deferred to the existing mandatory
Backend Repository 1/2/3 and PostgreSQL repository 1/2/3 hosted cohorts. Their
full native inventory and complete-result validation, plus the current required
Backend parent, must succeed before readiness. This package is already excluded
from ordinary Go-test shards because of its size; no CI or test changes are
needed. Broad shared store-conformance, frontend and E2E checks also remain
with CI. Final catalog/spec lint, actual eight-path documentation coverage,
relative links, size, Bash syntax, formatting and whitespace checks passed.
The owned PostgreSQL fixture was removed after identity verification; no
schemas, volumes or recorded local command groups remained. Normal active
commit hooks remain a delivery gate and their receipt is retained externally.

## Documentation impact

Internal requirements, design, and delivery records only. Public search found
no dismissal-specific interface guidance to revise. This restores a supported
database operation without changing commands, configuration, public payloads,
events, labels, or layouts; no public page or screenshot update is needed.

## Risks

- Using dotted single-key extraction silently breaks PostgreSQL nesting.
- Whole-second normalization could dismiss a later error; bound JSON strings
  must remain JSON objects rather than JSON-encoded strings.
- PostgreSQL native microsecond and SQLite date-function precision differ;
  preserve existing SQLite precision without promising nanosecond equivalence.
- A guard moved out of the update could overwrite a replacement. Keep one
  conditional write. Do not introduce stricter stamp or first-writer semantics.
- Full repository tests with a PostgreSQL DSN would run unrelated gated tests.
  Use that DSN only for focused affected tests, then unset it for the one
  complete owning package run.
