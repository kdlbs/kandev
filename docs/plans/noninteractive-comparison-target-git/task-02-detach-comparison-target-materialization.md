---
id: "02-detach-comparison-target-materialization"
title: "Detach comparison-target materialization"
status: done
wave: 2
depends_on: ["01-propagate-instance-git-environment"]
plan: "plan.md"
spec: "../../specs/platform/requirements/workspace-git-status.md"
---

# Task 02: Detach comparison-target materialization

Move comparison-target network work out of instance readiness and Git-status request paths. Preserve fail-closed comparison state during background work.

## Red tests first

Use a controllable Git shim and synchronization channels. Add focused tests that prove these behaviors:

- initial target preparation returns while the fetch shim remains blocked
- the tracker reports `comparison_target_pending` before the fetch completes
- a successful fetch publishes ready state and starts a status refresh
- an error publishes the existing bounded unavailable code
- a newer target cannot receive a stale result from an older operation
- manager shutdown cancels an unfinished fetch
- `UpdateComparisonTargets` and `GetWorkspaceTrackerFor` do not wait for a fetch
- one HTTPS fetch error produces no SSH command and no second fetch
- instance creation returns while initial materialization remains blocked

Extend the existing browser scenario. Make the comparison remote unreadable, then prove that the session opens and file status remains usable.

Record each focused red command before production changes.

## Implementation

- Set tracker comparison state to pending before scheduling materialization.
- Schedule initial, update, rescan, submodule, and lazy materialization with the manager lifetime context.
- Bound each operation with the existing Git-command deadline.
- Track current work by repository key. Cancel or supersede old work when its target changes.
- Keep the existing identity comparison before every state publication.
- Publish ready or unavailable state only for the current target.
- Start the existing detached status refresh after state publication.
- Make manager stop wait for or cancel background target operations without leaking a goroutine.
- Remove synchronous target work from instance creation and lazy tracker creation.
- Do not add transport fallback or mutate any Git remote after an authentication error.

## Acceptance

- Kandev instance creation does not wait for comparison-target network work.
- Git-status requests do not wait for comparison-target network work.
- Comparison data remains fail-closed until the exact target is ready.
- Missing credentials produce no terminal prompt and no SSH retry.
- Shutdown leaves no target-materialization goroutine or subprocess alive.

## Likely files

- `apps/backend/internal/agentctl/server/process/manager.go`
- `apps/backend/internal/agentctl/server/process/manager_comparison_targets.go`
- `apps/backend/internal/agentctl/server/process/comparison_target_test.go`
- `apps/backend/internal/agentctl/server/process/manager_comparison_targets_test.go` (new)
- `apps/backend/internal/agentctl/server/instance/manager.go`
- `apps/backend/internal/agentctl/server/instance/manager_comparison_targets_test.go` (new)
- `apps/web/e2e/tests/git/fork-pr-comparison-target.spec.ts`
- `apps/web/e2e/tests/git/fork-pr-comparison-target-helpers.ts`

## Verification

```bash
cd apps/backend && go test ./internal/agentctl/server/process -run 'TestComparisonTarget.*(NonBlocking|Pending|Cancellation|NoTransportFallback|Stale)' -count=1
cd apps/backend && go test ./internal/agentctl/server/instance -run 'TestCreateInstanceDoesNotWaitForComparisonTarget' -count=1
cd apps/backend && go test ./internal/agentctl/server/process ./internal/agentctl/server/instance
cd apps/web && pnpm e2e:run --host --no-build --project chromium -- e2e/tests/git/fork-pr-comparison-target.spec.ts
```

## Parallelism

`sequential`. This task depends on Task 01 and owns the comparison-target lifecycle.

## Output contract

Record the red and green commands. Record startup timing proof, cancellation proof, final state transitions, and proof that no transport fallback occurs.

## Implementation record

- Red: the blocking-shim preparation test took about 400 ms while Git was blocked, proving the previous synchronous path consumed the fetch deadline before production changes.
- Green: `cd apps/backend && go test ./internal/agentctl/server/process -run 'TestComparisonTarget.*(NonBlocking|Pending|Cancellation|NoTransportFallback|Stale)' -count=1` passed 4 tests; the instance readiness regression passed 1 test; the process and instance packages passed 787 tests together.
- Startup proof: `TestCreateInstanceDoesNotWaitForComparisonTarget`, `TestComparisonTargetPreparationNonBlocking`, `TestUpdateComparisonTargetsDoesNotWaitForMaterialization`, and `TestGetWorkspaceTrackerForDoesNotWaitForMaterialization` return before the controllable Git work completes and observe pending state.
- Lifecycle proof: ready and bounded unavailable states publish only for the current target; stale operations are superseded; manager teardown cancels and drains an unfinished materialization.
- Transport proof: `TestComparisonTargetMaterializationPublishesBoundedFetchFailureNoTransportFallback` observes one HTTPS fetch attempt, no SSH command, and no second fetch. The existing fork pull-request browser scenario passed both its ready and unavailable cases.

## PR fixup record

- Review fix: comparison-target admission is closed under the same mutex as `WaitGroup.Add` during teardown, normal `Stop` reopens it only after draining, and teardown remains permanently closed.
- Review fix: the `StatusStopped` cleanup path now continues through adapter, shell, and process teardown when comparison-target shutdown returns an error.
- Test portability: lifecycle coverage uses a copied test executable as the Git shim, so state, cancellation, stale-result, and startup assertions also compile on Windows.
- Test determinism: preparation, target updates, and lazy tracker creation now use completion channels plus a closed fetch gate instead of wall-clock performance thresholds.
