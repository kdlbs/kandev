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
GOMAXPROCS=4 GOMEMLIMIT=2GiB go -C apps/backend test -race -trimpath -p=2 ./internal/agent/runtime/lifecycle -run 'Test(Environment|WorkspaceLookup|CachedWorkspaceLookup|ConditionalCleanupRunsAfterWorkspacePeer|PromotionRejectsExecutionRemovedByCleanup|EnsureWorkspaceExecution|GetOrEnsureExecutionForEnvironment|CachedWorkspaceRestore|WorkspaceRestore)' -count=1
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
The environment operation also guards its session-cache read before sharing a
result with joined session lookups. A deterministic regression publishes the
retiring execution after the initial environment cache miss, joins a session
lookup, and verifies that both wait. It fails before the guard is applied.
A later CI cancellation-control test exposed a visibility/disabled-state
check race after the button unmounted. Its poll now reads both states in one
browser evaluation. Cancellation behavior and timeout limits are unchanged.
Integration with the current base preserves terminal-execution replacement and
retained-runtime selection checks after guarded cache lookup. The cancellation
E2E keeps the base's session-scoped locator and settlement barrier alongside
the atomic DOM check. Run the replacement, retirement, and cancellation suites
again after integration. The first integrated lifecycle run caught the distinct
cached-access contract: `GetOrEnsureExecution` retains admitted terminal
infrastructure, while explicit `EnsureWorkspaceExecutionForSession` replaces
terminal executions. Both now read the cache inside the session gate and wait
for retirement before applying their existing behavior.
A deterministic joined-read regression also proves explicit restoration runs
its own replacement check rather than borrowing the ordinary read's terminal
result. It fails before restoration uses the existing serialized operation
helper. Creation still shares the session gate and remains deduplicated.
The environment cache-hit path also enters the serialized session operation
and uses the shared retirement guard. It rechecks session registration and the
requested environment under store ownership before returning a cached runtime.
Regressions fail before the correction for pending cleanup, retirement removal,
and environment rebinding. A further regression proves the shared operation
also rejects a different environment's execution published during provider
resolution. Both cache paths preserve the requested environment constraint.
These regressions pass after the correction.
A backend CI shard also exposed an unsynchronized start-call counter in the
executor test mock. The existing sibling-start scenario reproduced the race
under a 1,000-run local stress test. The counter now uses atomic increment and
load; no production executor behavior changes.
Configured-runtime regressions also cover a mismatch before allocation, a
registration-conflict winner, and a mismatched result borrowed from another
coalesced lookup. All three failed before the correction. Conflicts now fail
before allocation where detectable, created results pass the retirement guard
before publication, and the final coalesced result retains its environment
constraint. A late allocation conflict rolls back only its own new runtime.
Final remote CI remains a publication gate.
