---
id: "02-gate-runtime-activation"
title: "Complete local housekeeping before runtime activation"
status: done
wave: 2
depends_on:
  - "01-restrict-historical-repair"
plan: "plan.md"
requirements:
  - REQ-PLATFORM-STARTUP-HOUSEKEEPING-001
  - REQ-PLATFORM-STARTUP-HOUSEKEEPING-002
acceptance_criteria:
  - AC-PLATFORM-STARTUP-HOUSEKEEPING-001.1
  - AC-PLATFORM-STARTUP-HOUSEKEEPING-001.2
  - AC-PLATFORM-STARTUP-HOUSEKEEPING-001.3
  - AC-PLATFORM-STARTUP-HOUSEKEEPING-001.4
  - AC-PLATFORM-STARTUP-HOUSEKEEPING-001.5
  - AC-PLATFORM-STARTUP-HOUSEKEEPING-002.3
  - AC-PLATFORM-STARTUP-HOUSEKEEPING-002.4
system_design:
  - ../../specs/platform/system-design/startup-housekeeping.md
---

# Gate runtime activation

## Summary and scope

Use TDD to split required local preparation from normal activation. Keep the
bootstrap listener and passive wiring available, finish required housekeeping,
validate persistence, publish readiness, then activate producers and restoration.
Audit constructor/immediate-first-pass side effects and retain exact cleanup
ownership. Do not wait for remote conversations at the global gate.

## Owned files and dependencies

- `apps/backend/internal/backendapp/main.go`, `services.go`, and actual
  activation helpers reached by those composition functions.
- `apps/backend/internal/orchestrator/service.go` and owning local reconciliation
  helpers, startup/liveness tests, task runtime-inventory repository selection.
- `apps/backend/internal/startup/step.go` and registry tests for the five named
  housekeeping operations; backend/web locale labels for those identifiers.
- Companion `startup-progress-visibility` registry requirement and design.

Task 01 must be complete. Preserve the current MCP and watcher startup tests,
pending remote guards, runtime token preservation, and restore quiescing.

## Implementation acceptance

1. Production composition tests hold real preparation at a barrier and prove no
   producer/restoration work is admitted. Required failure/cancellation prevents
   activation, while liveness and startup status remain available.
2. A retained stable stopped resume-only row incurs no process/remote probe;
   terminal-but-still-owned and pending remote rows retain all existing guards.
   A blocked remote cannot prevent readiness or cause dead-owner normalization.
3. Preparation has the five real named steps and suboperation timing logs, runs
   once per service lifecycle, and preserves final health, cleanup, and event/MCP
   ordering. Tests must exercise composition, not merely a callback-order helper.

## Verification

Use the exact packages below plus any additional constructor owner whose
activation API changes. Add that owner's focused command before marking done.

```bash
(cd apps/backend && go test -race ./internal/backendapp ./internal/startup -run 'Housekeeping|Startup|Bootstrap|StartOrchestrator|Required|MCP' -count=1)
(cd apps/backend && go test -race ./internal/orchestrator ./internal/agent/runtime/lifecycle -run 'Housekeeping|Startup|PendingRemote|RecoveryGuard|MCP' -count=1)
(cd apps/backend && go test -race ./internal/task/repository/sqlite -run 'Executor.*(Inventory|Running|Preserv)|Housekeeping' -count=1)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Run changed-file Go lint for this work order. Extend registry and locale
completeness tests when introducing the labels; task 04 consumes these same IDs.

## Risks and exclusions

Do not move the entire existing executor reconciliation before adoption:
retracked/pending status is not yet evidence then. No weakening of process
liveness, duplicate-process guards, authentication, or default survival policy.
Schema/store mutation remains sequential and cancellable. No generic worker
framework is required; use explicit owner start functions.

## Results

Implemented explicit preparation and deferred activation in production composition.
The five named operations finish before producer activation; local survivor
ownership is adopted before publishing launch routes. Remote attachment retains
exact-owner guards and runs after readiness. Stable stopped plugin environments
remain resume-only; unfinished terminal operations are repaired by the owned,
joined plugin boot worker after plugins start.

Validation: race-enabled backendapp/startup/orchestrator/lifecycle/delivery and
inventory regressions passed, including real immediate-first-pass reachability
and blocked plugin boot positive controls, cancellation/failure, preserved remote
ownership, idempotent preparation, and stopped-handle behavior. New plugin
retention and cleanup-order regressions failed before the fix and passed after.
Scoped Go lint passed with zero issues, including the final plugin refinement. The final isolated restart proof and final lint
receipt are recorded in the combined plan.

PR review refinement: starts registered during activation join the pending batch
and observe earlier failure. Barrier and nested-registration regressions reproduced
the race before the fix; targeted startup race tests passed.

Aggregate review refinement: stopped rows whose sessions are still created,
starting or running remain selected using a batched session read. Malformed plugin
metadata is logged and skipped without blocking other unfinished operations.
Regressions reproduced both defects. A separate guard test preserves required
persisted ownership reads; endpoint availability stays outside global readiness.
