---
id: "05-verification-and-doc-sync"
title: "Verification and doc sync"
status: completed
wave: 3
depends_on: ["01-runtime-inventory", "02-task-cleanup-ordering", "03-agentctl-process-group-shutdown", "04-startup-reconciliation"]
plan: "plan.md"
spec: "../../specs/tasks/runtime-cleanup.md"
---

# Task 05: Verification and Doc Sync

## Acceptance

- Backend formatting, tests, lint, and build/type checks pass for touched areas.
- The spec and ADR still match the implemented behavior.
- Relevant `AGENTS.md` scoped guidance is updated if cleanup conventions changed.

## Verification

```bash
make -C apps/backend fmt
make -C apps/backend test
make -C apps/backend lint
make -C apps/backend build
```

## Files likely touched

- `docs/specs/tasks/runtime-cleanup.md`
- `docs/decisions/0025-runtime-cleanup-uses-executors-running.md`
- `docs/plans/task-runtime-cleanup/plan.md`
- `apps/backend/AGENTS.md`
- `apps/backend/internal/agentctl/AGENTS.md`

## Dependencies

Tasks 01, 02, 03, and 04.

## Inputs

- All implementation tasks
- Root and backend engineering guidance

## Output contract

Report final verification commands and results, any doc updates, remaining risks,
and plan checkbox updates.

## Result

- Plan, task files, spec index, and decision index are in sync.
- `make -C apps/backend fmt` passed.
- `make -C apps/backend test` passed.
- `make -C apps/backend lint` passed.
- `make -C apps/backend build` passed.
