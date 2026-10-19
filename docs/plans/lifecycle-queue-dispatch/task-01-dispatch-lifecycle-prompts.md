---
id: "01-dispatch-lifecycle-prompts"
title: "Dispatch retained lifecycle prompts"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-CI-PR-AUTOMATION-001
acceptance_criteria:
  - AC-UI-CI-PR-AUTOMATION-001.9
  - AC-UI-CI-PR-AUTOMATION-001.10
  - AC-UI-CI-PR-AUTOMATION-001.18
system_design:
  - ../../specs/ui/system-design/ci-pr-automation-01.md
---

# Task 01: Dispatch retained lifecycle prompts

## Summary

Make SQLite queued lifecycle delivery independent of ordinary dispatch claims,
while preserving ordinary claim validation and lifecycle admission/acceptance
guards. Document and characterize conservative durable-error recovery separately
from known unaccepted legacy retry.

## In scope

The original production correction selects the ordinary updater only for
non-lifecycle entries. The SQLite consumer matrix in [the plan](plan.md#tests-and-end-to-end-evidence)
covers delivery, payload/hash persistence, CI attempt binding and ordinary claim
controls. The remaining correction adds one durable-error characterization in
the existing test file and aligns the same requirement/design/plan/order.

## Out of scope

No runtime, agentctl, `task_operations.go`, schema, new outcome types or shared
ordinary/direct settlement changes. No new claim/retry mechanism, generic
retained bypass, plan-comment/managed-input repair, async drain redesign,
global claim-error suppression or durable-protocol disablement.

## Acceptance

1. Legacy and compatible v1 lifecycle consumers reach transport through their
   retained reservations, acknowledge accepted delivery and preserve ordinary
   missing/stale claim rejection and existing task/session guards.
2. Known unaccepted legacy handoff retains one retry. A generic ambiguous v1
   error retains the lifecycle row and persisted interrupted-unknown submission
   behind the matching session/submission recovery block. A second real
   lifecycle admission rejects without a second transport or false accepted
   acknowledgement, preserving the same evidence.
3. Requirements distinguish errors lacking authoritative backend terminal
   evidence from known legacy rejection. The existing shared classification
   limitation is documented candidly; no unsafe weakening or automatic durable
   retry/exactly-once promise is introduced. Affected checks pass.

## Verification

The added test is an existing-behavior characterization and may pass on its
first run. Use a generic ambiguous transport error, not a manufactured
agentctl known-failure label. The original legacy failure control remains.
From repository root:

```bash
export PATH="/home/jcfs/.local/share/mise/installs/go/1.26.0/bin:/home/jcfs/.local/share/mise/installs/node/24.21.0/bin:$PATH"
export GOTOOLCHAIN=local GOMAXPROCS=2 GOMEMLIMIT=512MiB GOFLAGS=-p=1 KANDEV_TEST_POSTGRES_DSN=
export KANDEV_RUN_QUIET_DIR="$(mktemp -d /tmp/child146-4551-durable-error.XXXXXXXX)"
scripts/run-quiet backend-test -- timeout --kill-after=10s 420s bash -c 'cd apps/backend && exec go test -race -trimpath -tags fts5 -p=1 ./internal/orchestrator -run "^TestQueuedLifecycleDelivery(DurableError|Failure)$" -count=1 -timeout=300s'
export GOMEMLIMIT=3GiB GOFLAGS=-p=2
export KANDEV_RUN_QUIET_DIR="$(mktemp -d /tmp/child146-4551-amendment-lint.XXXXXXXX)"
scripts/run-quiet backend-lint -- timeout --kill-after=10s 400s bash scripts/lint-go-changed apps/backend/internal/orchestrator/event_handlers_queue_lifecycle_delivery_test.go
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Run the repository documentation-coverage preflight with the changed documents
and verify requirement/design/work-order links. Lint only the changed Go test
path for this correction. No dependency changes require another frozen install.

## Files

- `apps/backend/internal/orchestrator/event_handlers_agent.go`: original
  lifecycle-only correction, unchanged by the durable-error characterization.
- `apps/backend/internal/orchestrator/event_handlers_queue_lifecycle_delivery_test.go`.
- The same owning requirement/design pair and this plan/work order.

## Dependencies

None. The original production correction and SQLite consumer fixture are present.

## Risks

Memory queues cannot prove SQL claim behavior. V1 success/hash evidence does not
prove post-handoff certainty. Preserve generation, ownership and recovery fences;
an absent dispatch callback or error text alone cannot establish a safe retry.

## Parallelism

`sequential`

## Inputs

- [Requirement](../../specs/ui/requirements/ci-pr-automation.md), `.9`, `.10`, `.18`.
- [Owning design](../../specs/ui/system-design/ci-pr-automation-01.md), retained
  lifecycle delivery and existing durable classification limitation.
- Platform durable-agent-delivery ordinary claim contract, real queue and task
  repositories, lifecycle/submission/receipt tests and existing completion guards.

## Results

The original updater repair passed its targeted legacy/v1 SQLite consumer, CI
binding, ordinary claim and existing lifecycle/workflow tests under race/FTS5
(test runtime 3.537s). Its scoped lint, catalog/specification/coverage checks and
normal delivery hooks passed. The initial lint timeout remained recorded as
exit 4; the later same-package/config/diff-base 10-minute run passed. The frozen
workspace install completed successfully.

The added durable-error characterization and existing legacy failure controls
passed the affected selector with race, trimpath, FTS5 and one package worker
(1.503s test runtime, exit 0). The test seeds a real matching harness generation,
asserts the exact retained SQL queue ID and matching interrupted-unknown
submission/recovery block, and proves a second real lifecycle admission is
blocked without another transport. Initial fixture/oracle assertions for an
absent generation and pending-status count were corrected; neither failure was
treated as a causal production regression.

Canonical changed-file lint on the sole changed Go test path passed with zero
issues (exit 0). Catalog validation, specification lint, documentation coverage
and diff checks passed. Production code is unchanged by this characterization.
