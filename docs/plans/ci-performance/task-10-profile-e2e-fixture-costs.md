---
id: "10-profile-e2e-fixture-costs"
title: "Attribute E2E fixture and planner costs"
status: done
wave: 6
depends_on: ["07-measure-ci-efficiency"]
plan: "plan.md"
requirements:
  - REQ-PLATFORM-CI-PERFORMANCE-004
  - REQ-PLATFORM-CI-PERFORMANCE-005
acceptance_criteria:
  - AC-PLATFORM-CI-PERFORMANCE-004.1
  - AC-PLATFORM-CI-PERFORMANCE-004.2
  - AC-PLATFORM-CI-PERFORMANCE-005.1
  - AC-PLATFORM-CI-PERFORMANCE-005.5
system_design:
  - ../../specs/platform/system-design/ci-performance.md
---

# Task 10: Attribute E2E fixture and planner costs

## Summary

Measure the identified shared fixture operations and container estimate errors. Produce a bounded follow-up design only where the measurements justify an implementation.

## In scope

- Attribute existing hosted fixture steps first. Use temporary instrumentation only if missing per-operation timings would justify a candidate implementation.
- Compare the opt-in Kubernetes skip context with container predictions; preserve all declared project/file identities.
- Record the slow idle, LSP release, retry-exhaustion, and stall tests with their owning lifecycle contracts. Retain them unless an equivalent proof exists.

## Out of scope

Permanent runtime clock hooks, new profile/manifest schemas, more workers/shards, removed cleanup, removed tests, or unmeasured fixture rewrites.

## Acceptance

- A report attributes measured costs without adding aggregate test durations to wall time or hiding skipped declarations.
- Focused desktop/mobile and container samples preserve outcomes and isolation; unavailable Docker evidence remains pending.
- Each proposed follow-up names its exact owner, expected mechanism, regression protection, and benchmark gate; unsupported candidates are explicitly retained.

## Verification

Run from the repository root. Install workspace dependencies once in a fresh worktree.
New test and helper paths below are implementation deliverables.

```bash
(cd apps/web && pnpm exec vitest run e2e/scripts/plan-shards.test.ts e2e/scripts/e2e-timings.test.ts e2e/scripts/run-planned-shard.test.ts e2e/scripts/retry-summary.test.ts)
(cd apps/web && pnpm e2e:run --project chromium tests/chat/message-queue.spec.ts tests/session/session-idle-parking.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/session/mobile-transient-turn-runtime-continuity.spec.ts)
(cd apps/web && KANDEV_E2E_CONTAINERS=1 pnpm e2e:run --project containers tests/docker/docker-launch.spec.ts tests/kubernetes/kubernetes-docker-workloads.spec.ts)
git diff --check
```

If fixture source changes or new timing probes are needed, run the samples above sequentially with the resource guards. For unchanged fixtures, existing hosted blobs can establish attribution without another E2E run. Record incomplete per-operation evidence. Restore any temporary instrumentation before completion.

## Files likely touched

- `docs/plans/ci-performance/e2e-cost-follow-up.md (new)`
- `apps/web/e2e/fixtures/test-base.ts (temporary instrumentation only)`
- `apps/web/e2e/fixtures/backend.ts (temporary instrumentation only)`
- `apps/web/e2e/fixtures/docker-test-base.ts (temporary instrumentation only)`
- `apps/web/e2e/scripts/plan-shards.ts (read)`
- `apps/web/e2e/tests/coordinator/stall.spec.ts (read)`

## Dependencies

07-measure-ci-efficiency.

## Risks

Temporary timing instrumentation can distort scheduling. Restore sources byte-for-byte and use repeated samples. The skipped full-worker case is not executed acceptance evidence.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/platform/requirements/ci-performance.md), IDs in frontmatter.
- [System design](../../specs/platform/system-design/ci-performance.md), October continuation.
- [October evidence](evidence-2026-10-08.md).
- [Plan](plan.md), acceptance gates and preserved companion statuses.

## Results

Completed the bounded attribution report in [e2e-cost-follow-up.md](e2e-cost-follow-up.md).
Existing hosted Playwright fixture steps in 20 blobs provided measurements
without instrumenting sources or dispatching more jobs. This replaces the
proposed temporary-instrumentation pass. Per-operation attribution and opt-in
full-worker execution remain explicitly unmeasured; no fixture rewrite is adopted.
Shared fixture sources match the measured run. The report names exact follow-up
owners, preservation requirements, and adoption gates.
