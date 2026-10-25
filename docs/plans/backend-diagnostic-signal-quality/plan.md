---
spec: docs/specs/platform/requirements/expected-runtime-log-severity.md
created: 2026-08-28
status: implemented
---

# Implementation Plan: Backend diagnostic signal quality

## Overview

This repair removes false warning and error entries from four confirmed backend
paths. It preserves warnings for unknown child output and unexpected storage
errors.

The work changes no successful API payload, process lifecycle, workspace
selection, or runtime cleanup result. A plan write for a missing task changes
from an internal storage error to the stable `not_found` contract.

## Specifications

- [Expected runtime log severity requirements](../../specs/platform/requirements/expected-runtime-log-severity.md)
- [Expected runtime log severity design](../../specs/platform/system-design/expected-runtime-log-severity.md)
- [Task runtime cleanup requirements](../../specs/tasks/requirements/runtime-cleanup.md)
- [Task runtime cleanup design](../../specs/tasks/system-design/runtime-cleanup.md)
- [Task documents requirements](../../specs/tasks/requirements/documents.md)
- [Task plan write lifecycle design](../../specs/tasks/system-design/plan-write-lifecycle.md)

## Implementation order

Wave 1 contains four independent backend corrections:

- [x] [Task 01: Preserve child log severity](task-01-preserve-child-log-severity.md)
- [x] [Task 02: Quiet stale environment fallback](task-02-quiet-stale-environment-fallback.md)
- [x] [Task 03: Quiet rotated liveness repair](task-03-quiet-rotated-liveness-repair.md)
- [x] [Task 04: Classify missing-task plan writes](task-04-classify-missing-task-plan-writes.md)

Wave 2 depends on Task 04:

- [x] [Task 05: Map missing-task plan responses](task-05-map-missing-task-plan-responses.md)

Implementation remains sequential in the primary conversation unless the user
explicitly authorizes delegation.

## TDD strategy

Each work order starts with a focused regression that fails on the current
behavior. The implementation then makes that regression pass without changing
the neighboring error path.

Repository coverage runs against SQLite. The existing environment-gated
PostgreSQL suite proves dialect parity for the plan foreign-key mapping.

No browser end-to-end test is required. The changed behavior is a backend log
contract and a shared WebSocket error response. Handler integration tests cover
both browser and MCP surfaces.

## Final verification

Run these commands from the repository root after all work orders are complete:

```bash
cd apps/backend && go test ./internal/agent/runtime/agentctl/launcher ./internal/task/service ./internal/orchestrator ./internal/task/repository/sqlite ./internal/task/planws ./internal/task/handlers ./internal/mcp/handlers -count=1
make -C apps/backend lint
git diff --check
```

If `KANDEV_TEST_POSTGRES_DSN` is available, also run:

```bash
cd apps/backend && go test ./internal/task/repository/sqlite -run TestPostgresWritePlanRevisionMissingTask -count=1
```

## Risks

- A permissive child-log parser can hide unstructured stderr. The parser must
  accept only anchored Zap and Go `slog` records.
- A broad not-found check can hide a database outage. The service must match the
  typed task-environment sentinel only.
- A liveness repair can race a newer execution. The fix must preserve the newer
  row and change only the log classification.
- Foreign-key text differs by database. The repository must use
  `internal/db.IsForeignKeyViolation` instead of matching new strings.
- A missing-task plan rejection can race after access validation. The write
  transaction remains the authoritative check.

## Out of scope

- Enabling authentication or changing bind addresses.
- Changing authentication warning text or severity.
- Repairing third-party plugin webhook defaults or provider outages.
- Changing the designed host-utility idle-repair behavior.
- Reclassifying network disconnect warnings without a confirmed caller defect.
- Changing raw ACP collection, retention, or diagnostic bundle size.
- Migrating plans to the generalized task-documents model.

## Open questions

None.
