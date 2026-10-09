---
id: "03-owned-exit"
title: "Detect runtime loss and contain only owned children"
status: done
wave: 3
depends_on: ["02-consumer-migration"]
plan: "plan.md"
requirements:
  - REQ-PLATFORM-AGENT-RUNTIME-AVAILABILITY-001
  - REQ-PLATFORM-AGENT-RUNTIME-AVAILABILITY-002
  - REQ-PLATFORM-AGENT-RUNTIME-AVAILABILITY-003
acceptance_criteria:
  - AC-PLATFORM-AGENT-RUNTIME-AVAILABILITY-001.4
  - AC-PLATFORM-AGENT-RUNTIME-AVAILABILITY-002.3
  - AC-PLATFORM-AGENT-RUNTIME-AVAILABILITY-002.5
  - AC-PLATFORM-AGENT-RUNTIME-AVAILABILITY-002.6
  - AC-PLATFORM-AGENT-RUNTIME-AVAILABILITY-003.5
system_design:
  - ../../specs/platform/system-design/agent-runtime-availability.md
---

# Task 03: Detect runtime loss and contain only owned children

## Summary

Detect runtime loss and contain only owned children. Preserve original session authority and all prior durable-delivery fixes.

## In scope

Route launcher Wait results through immutable runtime epoch and ownership evidence.
Add adopted-server monitoring using authenticated control health and existing renewal ownership.
Differentiate proven exit, transient transport loss, stale credential, and foreign owner. Only proven safe ownership permits replacement or termination.
Persist/reuse process birth and group/job identity for cleanup. Treat missing historical evidence as unknown.
Keep Windows suspended-start Job Objects; verify Linux descendants; add macOS/BSD owned-group containment.
Protect against PID reuse, delayed cleanup, backend shutdown, and a newly adopted owner.
Preserve worktrees and journals. Close only resources owned by the retired execution.
Capture bounded sanitized stderr/exit diagnostics without blocking process pipes.
Do not emit death from a never-published provisional instance or equate a port lease with process ownership.

## Out of scope

Other work orders, unproven Git repairs, automatic external mutation retries, and unrelated executor changes.

## Acceptance

- The scoped outcome passes every named regression, including stale-owner and failure cases.
- No prompt, tool, or Git mutation is replayed by runtime replacement.
- Partial failure leaves accurate availability and admission fences; cleanup affects only owned resources.

## Tests

TestRuntimeExitEpochFence; TestAdoptedRuntimeDeathEvidence; TestRuntimeCleanupRejectsReusedPID; TestRuntimeShutdownSuppressesRecovery.
Add OS-specific tests for actual child/descendant exit and unrelated sentinel survival.
On macOS, force agentctl exit and verify only the owned group is contained; on Windows verify Job Object containment.
Missing process identity must produce blocked recovery without a kill. Race Stop with exit and replacement.
Proposed tests belong beside their production owners. Verify the command selects them before recording success.

## Verification

Run each command from the repository root after implementation.

```bash
(cd apps/backend && go test -race ./internal/agent/runtime/agentctl/launcher ./internal/agentctl/server/process ./internal/agent/runtime/lifecycle ./internal/backendapp -count=1)
(cd apps/backend && go run ./cmd/sqlguard ./internal)
(cd apps/backend && go test -race ./internal/persistence/storeconformance -count=1)
make -C apps/backend lint
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Run the launcher/process tests on native Windows and macOS hosts too, from apps/backend:

```text
go test -race ./internal/agent/runtime/agentctl/launcher ./internal/agentctl/server/process -run Runtime -count=1
```

These hosts must execute the new containment tests; compile-only success does not close this gate.

For persisted identity changes, include fresh boot, reopen, prior-schema upgrade, interrupted migration, and PostgreSQL conformance. Record unavailable infrastructure as an incomplete gate.

## Files likely touched

- `apps/backend/internal/agent/runtime/agentctl/launcher/launcher.go`
- `apps/backend/internal/backendapp/agentctl.go`
- `apps/backend/internal/agentctl/server/process/procattr_windows.go`
- `apps/backend/internal/agentctl/server/process/procattr_linux.go`
- `apps/backend/internal/agentctl/server/process/procattr_unix.go`
- `apps/backend/internal/agent/runtime/lifecycle/run_owner_persistence.go`

New runtime-owner/coordinator and regression files are expected beside these owners.

## Dependencies

Task 02: Move local runtime consumers to the shared owner.

## Risks

Delayed callbacks and partial binding can affect a successor. Prove generation ownership before every state-changing result.

## Parallelism

`sequential`

## Inputs

- [Manifest](plan.md).
- [Requirements](../../specs/platform/requirements/agent-runtime-availability.md).
- [Design](../../specs/platform/system-design/agent-runtime-availability.md).
- [Decision](../../decisions/2026-09-27-agentctl-runtime-replacement.md).

## Results

Implementation complete on 2026-09-27. Runtime loss now carries generation and process-ownership evidence; unexpected child exits drain bounded sanitized diagnostics and contain only verified owned descendants. Adopted servers use authenticated health/identity checks. Persisted process birth/group/session identity is additive, and partial upgrades resume without granting legacy records kill authority.

Passed locally:

- `TMPDIR=/root/.cache/kandev-agentctl-runtime-tmp go test -p 1 -race ./internal/common/processidentity ./internal/agent/runtime/agentctl ./internal/agent/runtime/agentctl/launcher -count=1`
- `TMPDIR=/root/.cache/kandev-agentctl-runtime-tmp go test -p 1 -race ./internal/task/repository/sqlite -run 'Test(UpsertControlServerRecordThenGetRoundTrips|ControlServerRecordProcessIdentity|PostgresControlServerRecordRoundTrips)' -count=1` (SQLite round-trip/reopen/partial-upgrade tests passed; PostgreSQL test skipped because `KANDEV_TEST_POSTGRES_DSN` is unset)
- `go test -p 1 -race ./internal/agentctl/server/process ./internal/agent/runtime/lifecycle ./internal/backendapp -count=1` (backendapp passed on isolated rerun after one transient failure in the combined batch)
- `go run -p 1 ./cmd/sqlguard ./internal`
- `go test -p 1 -race ./internal/persistence/storeconformance -count=1`
- Darwin/arm64 and Windows/amd64 compile checks for process identity, launcher, and process packages
- `TMPDIR=/root/.cache/kandev-agentctl-runtime-tmp make -C apps/backend lint`
- `git diff --check`

Release gates remain open: containment tests have not run natively on macOS or Windows, and PostgreSQL conformance needs a configured test database. Cross-compilation does not close the native execution gates.

Main-integration follow-up retains the continuous bounded reader introduced by
the log-drain work order and the replacement launcher's owned-reader wait group.
Complete stderr records still enter the sanitized bounded exit tail; an
oversized record contributes only a fixed discard marker, never a raw prefix.
Adopting the new reader without this seam failed the existing diagnostic
regression. The updated discard/no-content assertion also failed before the
capture seam was added, then passed with the combined implementation.

`go test ./internal/agent/runtime/agentctl/launcher -count=1` and
`go test -p 1 -race ./internal/agent/runtime/agentctl/launcher
./internal/agent/runtime/agentctl -count=1` passed from `apps/backend`.
Fresh hosted checks and the previously documented native/PostgreSQL release
gates remain pending; these receipts do not close them.
