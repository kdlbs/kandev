---
id: "01-exact-cancel-and-census"
title: "Exact cancellation and census"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-PENDING-MOVE-CANCELLATION-001
  - REQ-TASKS-PENDING-MOVE-CANCELLATION-002
  - REQ-TASKS-PENDING-MOVE-CANCELLATION-003
  - REQ-TASKS-PENDING-MOVE-CANCELLATION-004
acceptance_criteria:
  - AC-TASKS-PENDING-MOVE-CANCELLATION-001.1
  - AC-TASKS-PENDING-MOVE-CANCELLATION-001.2
  - AC-TASKS-PENDING-MOVE-CANCELLATION-001.3
  - AC-TASKS-PENDING-MOVE-CANCELLATION-001.4
  - AC-TASKS-PENDING-MOVE-CANCELLATION-001.5
  - AC-TASKS-PENDING-MOVE-CANCELLATION-001.6
  - AC-TASKS-PENDING-MOVE-CANCELLATION-002.1
  - AC-TASKS-PENDING-MOVE-CANCELLATION-002.2
  - AC-TASKS-PENDING-MOVE-CANCELLATION-002.3
  - AC-TASKS-PENDING-MOVE-CANCELLATION-002.4
  - AC-TASKS-PENDING-MOVE-CANCELLATION-002.5
  - AC-TASKS-PENDING-MOVE-CANCELLATION-003.1
  - AC-TASKS-PENDING-MOVE-CANCELLATION-003.2
  - AC-TASKS-PENDING-MOVE-CANCELLATION-003.3
  - AC-TASKS-PENDING-MOVE-CANCELLATION-003.4
  - AC-TASKS-PENDING-MOVE-CANCELLATION-004.1
  - AC-TASKS-PENDING-MOVE-CANCELLATION-004.2
  - AC-TASKS-PENDING-MOVE-CANCELLATION-004.3
  - AC-TASKS-PENDING-MOVE-CANCELLATION-004.4
  - AC-TASKS-PENDING-MOVE-CANCELLATION-004.5
system_design:
  - ../../specs/tasks/system-design/pending-move-exact-cancellation.md
---

# Task 01: Exact cancellation and census

## Scope

Implement the two Coordinator tools end to end against the current merged tree:
the exact compare-and-delete (`cancel_pending_move_kandev`) and the non-mutating
census (`read_pending_move_kandev`) inside the messagequeue repository, their
backend actions, the Coordinator-confined MCP catalog entries, caller identity
derivation, the shared workspace grant shape, and the pending-move cancellation
audit table and its idempotent migration.

## Exclusions

- No live pending-move mutation outside tests.
- The TTL/reaper lifecycle is unchanged and remains the sole owner of expiry
  and orphan reaping (REQ-TASKS-PENDING-MOVE-CANCELLATION-003.4).
- Ordinary-agent self-only authorization, the workflow move service, and the
  frontend are out of scope.

## Acceptance criteria covered

- AC-TASKS-PENDING-MOVE-CANCELLATION-001.1 through 001.6: seven canonical
  predicates, race-safe single-row delete, cancellation-fenced snapshot
  restore, retry receives the stable miss.
- AC-TASKS-PENDING-MOVE-CANCELLATION-002.1 through 002.6: designated
  Coordinator grants, live execution/rotation checks, non-leaking denial.
- AC-TASKS-PENDING-MOVE-CANCELLATION-003.1 through 003.4: atomic audited
  outcome evidence, no unsafe payload logging, schema-invalid calls reach the
  denial-audit path.
- AC-TASKS-PENDING-MOVE-CANCELLATION-004.1 through 004.5: exact census
  projection plus authoritative zero-row proof and non-mutation.

## Implementation conditions

1. Every denial, mismatch, replacement, revoked-caller, and storage failure
   path returns one public collapsed result; only a full tuple match deletes
   exactly one row inside the transaction.
2. The census proves a zero-row authorized absence exactly as a first-class
   `found=false` result and never writes.
3. The migration replays cleanly on fresh and existing installs and the grant
   relation is database-enforced (workspace/task pair).

## Verification

    cd apps/backend
    go test -race -count=1 ./internal/orchestrator/messagequeue ./internal/mcp/handlers ./internal/mcp/server ./internal/mcp/scope ./internal/task/repository/sqlite ./internal/agent/runtime/lifecycle
    go test -race -count=20 -run 'PendingMove|ExecutorStatus|TerminalStatus' ./internal/orchestrator/messagequeue ./internal/agent/runtime/lifecycle ./internal/task/repository/sqlite
    go test -count=1 ./internal/backendapp

    cd /path/to/repository/root
    python3 scripts/list-docs.py validate
    python3 scripts/lint-spec-files.py --all

## Likely files

- `apps/backend/internal/orchestrator/messagequeue/` (exact cancel, census,
  repository composition, audit migration) and focused tests.
- `apps/backend/internal/mcp/handlers/`, `internal/mcp/server/`,
  `internal/mcp/scope/` for catalog and identity.
- `apps/backend/internal/task/repository/sqlite/` for the grant/audit storage.
- `apps/backend/internal/agent/runtime/lifecycle/` for the CAS and refresh
  serialization.
- `apps/backend/pkg/websocket/actions.go` for backend action routing.

## Dependencies

- Binary upstream counterpart to this branch, already consumed and reconciled
  by the merge commit that carried this plan in (messagequeue
  `entry_options_json` coexistence).

## Results

Implemented. Race-enabled focused suites pass for messagequeue, MCP
handlers/server/scope, SQLite task repository, and lifecycle; twenty repeated
race-enabled runs cover concurrent cancellers, successor-generation survival,
retry idempotence, state/authorization revalidation, non-leaking denial/read
audit, audit/delete rollback, lifecycle refresh/stop serialization, terminal
persistence retry, and rotated-execution rejection; backend composition tests
pass. Remaining note: local PostgreSQL suites were skipped because
`KANDEV_TEST_POSTGRES_DSN` is unavailable in this environment; CI run the
Postgres jobs.
