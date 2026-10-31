---
created: 2026-10-29
status: complete
requirements:
  - REQ-PLUGINS-AUTOMATION-READS-001
system_design:
  - ../../specs/plugins/system-design/automation-history-reads.md
legacy_specs: []
---

# Implementation Plan: Plugin Automation History Reads

## Overview

Expose supported plugin reads for automation definitions, bounded run history,
and exact run refresh. Deliver source observations first, then the public
Host/SDK slice, then independent consumer evidence and author documentation.

The source work has an independent verification boundary: it reconciles raw
and projected run state without changing execution readers. The Host work
depends on that shared observation contract. All work orders run sequentially.
Implementation is complete for the current version. The stable-version backport remains separate scope.

## Scope

### In scope

- Optional Go SDK reader and additive Host protobuf/gRPC methods.
- Current-version service adapters, safe DTOs, workspace authorization,
  capability discovery, and existing exact-query snapshots.
- Source projection consistency, bounded filtered history, and exact refresh.
- Targeted tests, public author documentation, and consumer compatibility.

### Out of scope

- Stable-version backports, deployment, SDK publication, and release changes.
- Frontend/browser APIs, rendered UI, settings, and feature flags.
- Scheduler, execution, retention, or durable event-delivery changes.
- Provider integrations, external exports, and consumer reconciliation workers.
- Unbounded scans or automatic query partitioning.

## Inputs and ownership

- [Requirements](../../specs/plugins/requirements/automation-history-reads.md):
  `REQ-PLUGINS-AUTOMATION-READS-001` and criteria `.1` through `.12`.
- [System design](../../specs/plugins/system-design/automation-history-reads.md).
- [Discussion #4941](https://github.com/kdlbs/kandev/discussions/4941) and its
  [fork proposal](https://github.com/illuwa-soft/kandev/pull/1) supply problem
  context. This package owns the current-version delivery scope.
- [Host data boundary](../../decisions/0043-plugin-host-data-api.md) and
  [generic coordination boundary](../../decisions/2026-09-25-plugin-coordination-platform.md).
- [Capability approval design](../../specs/plugins/system-design/capability-approval.md).

The plugin system owns this package because it defines a public consumer
contract. Automation scheduling and source state retain their existing owners.
No existing companion plan gains new implementation work or changes status.

## Technical approach

### Source observations

Add bounded automation-owned history queries instead of wrapping the capped
recent feed. Source results retain both stored and observed run statuses.
Use the same task-state and summary projection for history lists and exact
refresh. Keep existing raw `Store.GetRun` callers unchanged.

New queries filter workspace membership before projection. Their row limit is
the shared snapshot maximum plus one. They support an automation selector and
creation-time ranges. No schema migration is planned.

### Host and SDK

Add the optional `HostAutomationHistory` SDK helper and three additive RPCs:
`ListAutomationDefinitionsExact`, `ListAutomationRunsExact`, and
`GetAutomationRunExact`. Keep the base Host and exact-query interfaces stable.
Use explicit DTO mapping and a `backendapp` adapter to avoid import cycles.

Reuse `api_read: automations`, `host.v2.read:automations`, current admission,
snapshot identity, and read receipts. Register actual method availability in
capability discovery. Read dependencies through live getters, including on
already-created Host objects.

### Paging and compatibility

Reuse the current 200-row page maximum and bounded immutable snapshots.
Oversized queries fail with `ResourceExhausted`. They never appear exhausted.
Consumers can narrow automation and creation-time filters. Exact run refresh
remains available independently of the list snapshot.

| Host/consumer                                     | Transport and authority                                            | Intended behavior           | Evidence and fallback                                   |
| ------------------------------------------------- | ------------------------------------------------------------------ | --------------------------- | ------------------------------------------------------- |
| Current Host, approved plugin                     | Optional SDK over existing gRPC broker, workspace automation grant | Three working reads         | Host wire and source tests                              |
| Current Host, missing or revoked grant            | Same transport, no current authority                               | Denial before source reads  | Authorization and revocation tests                      |
| Current Host, absent reader dependency            | Discovery plus optional RPCs                                       | Unsupported method          | Missing-dependency tests, no internal fallback          |
| Older Host, new SDK                               | Existing handshake, new RPC absent                                 | Explicit unsupported result | Independent consumer test, existing calls remain usable |
| Existing Host implementation or schedule consumer | Existing Go interfaces and wire methods                            | Existing contracts retained | SDK compile tests and managed-schedule regression       |
| Browser/TypeScript consumer or stable backport    | No new surface in this package                                     | Separate delivery scope     | No support claim                                        |

## Tests

The names below are planned tests unless described as existing patterns.
Every work order uses TDD and records its actual commands and results.

| Acceptance criteria            | Test file and planned evidence                                                                                                                               | Work order |
| ------------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------ | ---------- |
| `.1`, `.5`, `.7`, `.10`, `.12` | `automation/history_reads_test.go`: `TestAutomationHistoryDefinitions`, `TestAutomationHistoryScopeAndBounds`, `TestAutomationHistoryReadsDoNotMutate`       | 01         |
| `.2-.4`, `.12`                 | `automation/history_reads_test.go`: `TestAutomationHistoryRunObservations`, `TestAutomationHistoryExactRefresh`, `TestAutomationHistoryRetention`            | 01         |
| `.2-.5`, `.7`                  | `automation/history_reads_postgres_test.go`: `TestAutomationHistoryPostgres`                                                                                 | 01         |
| `.1-.6`, `.9`, `.11`           | `pluginsdk/automation_history_test.go`: `TestAutomationHistoryRoundTrip`, `TestAutomationHistoryOptionalHost`                                                | 02         |
| `.5-.8`, `.12`                 | `plugins/host_automation_history_test.go`: `TestAutomationHistoryHostAuthorization`, `TestAutomationHistoryHostSnapshots`, `TestAutomationHistoryHostLimits` | 02         |
| `.1-.4`, `.9-.12`              | `plugins/host_automation_history_wire_test.go`: `TestAutomationHistoryHostWire`, `TestAutomationHistoryHostAvailability`                                     | 02         |
| `.1-.4`, `.7`, `.9-.10`        | `backendapp/adapters_plugin_automation_history_test.go`: `TestPluginAutomationHistoryAdapter`, `TestPluginAutomationHistoryWiring`                           | 02         |
| `.4-.6`, `.8`, `.11-.12`       | `pluginsdk/automation_history_consumer_test.go`: `TestAutomationHistoryIndependentConsumer`, `TestAutomationHistoryOldHost`                                  | 03         |

Source tests use real SQLite service/store fixtures. New dialect-sensitive
queries also need the existing environment-gated PostgreSQL fixture.
An unset `KANDEV_TEST_POSTGRES_DSN` means skipped PostgreSQL evidence.
It does not mean that PostgreSQL passed.

Reuse `host_data_wire_test.go` and `dialPluginHostOverWire` for real broker
tests. Source fixture tests and adapter tests complete the transport evidence.
Add an independent consumer fixture using public SDK imports only.

No rendered UI changes require Playwright. The consumer tests provide the
end-to-end evidence for this public API.

## Documentation and release handoff

Task 03 updates `docs/public/plugins-authoring.md`,
`docs/public/plugins-manifest.md`, and `docs/plans/plugins/GRPC-CONTRACT.md`.
Document the resource scope, optional-method discovery, status meanings,
snapshot limits, expiry, narrowing, and exact refresh.
The design turn does not advertise an unimplemented API in public docs.

Use the existing SDK release compatibility process. Record the contract change
for the next release without inventing a Host baseline or publishing packages.
The stable backport requested in the fork remains a separate initiative.

## Work orders

- [x] [Task 01: Add bounded source observations](task-01-source-observations.md) (done)
- [x] [Task 02: Expose the authorized Host reader](task-02-host-reader.md) (done)
- [x] [Task 03: Prove consumer compatibility and document the API](task-03-consumer-docs.md) (done)

Dependency order: 01 -> 02 -> 03. Shared contracts and generated files make
these work orders sequential.

## Verification results

Implementation checks passed on 2026-10-29:

- `go -C apps/backend test -trimpath -race ./internal/automation -run 'TestAutomationHistory|TestListRunPage|TestListRuns|TestListWorkspaceRuns' -count=1`: passed.
- `go -C apps/backend test -trimpath -race ./internal/plugins -run 'TestAutomationHistory|TestExactWorkspaceSnapshot|TestManagedConversationScheduleHostAuthorizationAndReplay' -count=1`: passed.
- `go -C apps/backend test -trimpath -race ./internal/backendapp -run '^TestPluginAutomationHistory' -count=1`: passed.
- `make -C pluginsdk proto` and `make -C pluginsdk test`: passed.
- `go -C apps/backend build ./...`: passed.
- `go -C apps/backend run ./cmd/sqlguard ./internal` and `go -C apps/backend test -trimpath -race ./internal/persistence/storeconformance -count=1`: passed.
- `node --test scripts/validate-public-docs.test.mjs`: 63 tests passed.
- `node scripts/validate-public-docs.mjs`: 54 published pages validated.
- `python3 scripts/list-docs.py validate` and `python3 scripts/lint-spec-files.py --all`: passed.
- Added source regressions for recorded task/session/turn identities, primary
  session replacement, linked-task deletion, and large excluded payloads.
- Added adapter regressions for a system block crossing the old summary prefix,
  an unterminated system block failing closed, and a truncated credential tail.
- Added a legacy-run regression for the preserved task-level summary fallback.
- `bash scripts/lint-go-changed ...`: 0 issues.
- `git diff --check`: passed.

`TestAutomationHistoryPostgres` reported a skip because
`KANDEV_TEST_POSTGRES_DSN` was not set. That is not PostgreSQL coverage.

The independent consumer fixture under
`testdata/pluginsdk/automation-history-consumer/` compiles and runs with only
public SDK imports. It uses a local module replacement, so it is candidate-SDK
evidence, not registry-installation or published-release evidence.

Design-package checks passed on 2026-10-29:

- `python3 scripts/list-docs.py validate`: catalog references passed.
- `python3 scripts/lint-spec-files.py --all`: specification lint passed.
- `python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- `node --test .github/scripts/pr-docs.test.cjs .github/scripts/pr-docs-git.test.cjs`:
  111 tests passed.
- Local `validateCoverage` preflight with a simulated runtime path: all three
  changed work orders were covered, with no reference errors.
- Relative Markdown links resolved within the workspace.

These checks validate the documents and their coverage chain. They do not
claim runtime behavior, PostgreSQL execution, or SDK availability.

## Risks

- A single recent-feed call silently loses older history. New source queries
  must detect the full bounded result, not inherit the 200-row feed cap.
- Raw run lookup and existing list projection differ. Changing raw internal
  readers can alter execution behavior, so the new observation path is separate.
- A snapshot exceeding 5,000 rows or 8 MiB requires a narrower query. A consumer
  must retain an incomplete state if it cannot narrow the query enough.
- Summary and error fields can contain credential patterns. Sanitize them
  before applying their output bound.
- Advertising support before late source wiring completes produces a false
  capability claim. Test missing dependencies and already-created Host objects.
- Generated source and SDK declarations do not prove old-Host compatibility.
  Task 03 requires a public consumer calling an older RPC surface.
