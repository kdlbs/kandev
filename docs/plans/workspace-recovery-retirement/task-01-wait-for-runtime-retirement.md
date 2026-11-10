---
id: workspace-recovery-retirement-01
title: Wait for runtime retirement before cached workspace recovery
status: done
wave: 1
depends_on: []
plan: plan.md
requirements:
  - REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-005
acceptance_criteria:
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-005.1
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-005.4
system_design:
  - ../../specs/agents/system-design/workspace-recovery-retirement.md
---

# Wait for runtime retirement

## Scope and acceptance

1. Workspace lookup joins the session operation before reading cached state.
2. A cached execution under runtime retirement cannot escape to a caller;
   recheck its registration after retirement releases its lock. Cancelled
   lookup contexts stop waiting for retirement.
3. Failed-session workspace recovery succeeds without changing the existing
   provider identity, terminal-state, or admission contracts.

Change `manager_execution.go` and `cleanup_execution_coalescing_test.go` in
`apps/backend/internal/agent/runtime/lifecycle/`. Reuse existing operation and
runtime ownership locks. Do not change UI, timeouts, recovery eligibility, or
persistence. This work has no delivery dependencies.

## Verification

From the repository root:

```sh
GOMAXPROCS=4 GOMEMLIMIT=2GiB go -C apps/backend test -race -trimpath -p=2 ./internal/agent/runtime/lifecycle -run 'Test(WorkspaceLookup|CachedWorkspaceLookup|ConditionalCleanupRunsAfterWorkspacePeer|PromotionRejectsExecutionRemovedByCleanup|EnsureWorkspaceExecutionForSession|CachedWorkspaceRestore|WorkspaceRestore)' -count=1
bash scripts/lint-go-changed apps/backend/internal/agent/runtime/lifecycle/manager_execution.go apps/backend/internal/agent/runtime/lifecycle/cleanup_execution_coalescing_test.go
```

With isolated test configuration, from `apps/web/`, after building E2E artifacts:

```sh
pnpm e2e:run --host --no-build --shards 1 --project mobile-chrome tests/session/mobile-session-resume-settings-recovery.spec.ts --retries=0
```

## Results

The exact CI failure reproduced locally with retries disabled: workspace
restore received an error while its controller was retiring. Both new lifecycle
regressions failed before their corresponding corrections. Targeted lifecycle
tests and the original mobile E2E scenario pass after the correction.
The mobile scenario also passed three consecutive runs with retries disabled.
The corresponding desktop scenario passed with retries disabled. Scoped Go lint
reports zero issues. The targeted lifecycle suite also passes with the race
detector enabled. A further regression verifies cancellation while retirement holds the lock.
The retirement regression uses `testing/synctest` to prove the lookup has
blocked before retirement proceeds. It also fails when the retirement guard
is deliberately omitted; no wall-clock scheduling assumption is required.
Final remote CI remains a publication gate.
