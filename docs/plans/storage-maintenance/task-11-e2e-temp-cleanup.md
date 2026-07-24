---
id: "11-e2e-temp-cleanup"
title: "Contain E2E temporary roots"
status: complete
wave: 7
depends_on: ["10-inherit-service-temp"]
plan: "plan.md"
spec: "../../specs/system-page/storage-maintenance.md"
---

# Task 11: Contain E2E temporary roots

Prevent normal managed E2E runs from leaving large numbers of `kandev-e2e-*` backend roots in the
service's inherited temporary directory.

## Acceptance

- Backend fixture directories created by one managed run have one explicit run owner and are removed
  on normal success and failure without deleting another concurrent run's files.
- The cleanup guard begins immediately after `mkdtempSync`, covers setup/spawn/initial health
  failures, passes the spawned process to the initial health wait for fast early-exit detection, and
  reports cleanup failures instead of silently discarding them.
- Cleanup targets only the exact run-owned root; interrupted-process residue remains subject to host
  temp policy unless a safe existing runner lifecycle can prove ownership.
- Tests or a deterministic harness regression prove success/failure cleanup and concurrent-run
  isolation. No test uses the host's real Kandev data directory.

## Verification

```bash
cd apps/web && pnpm e2e:run tests/system/storage-maintenance.spec.ts
cd apps/web && pnpm e2e:run tests/system/mobile-storage-maintenance.spec.ts -- --project=mobile-chrome
```

## Files likely touched

- `apps/web/e2e/fixtures/backend.ts`
- a focused fixture-lifecycle test beside the existing E2E harness tests
- focused fixture tests

## Dependencies

Task 10.

## Inputs

- Read-only audit of 1,099 abandoned `kandev-e2e-*` roots
- Existing managed runner cleanup and concurrency conventions

## Output contract

Report root ownership, success/failure/interruption semantics, concurrency safety, tests/commands,
remaining host-policy boundary, and update this task plus the serialized plan checkbox when done.

## Completion

- The atomically-created worker root is now guarded immediately after creation. Setup, spawn,
  initial-health, test-use, and restart failures all converge on the same exact-root cleanup path.
- The latest registered backend process group is stopped before bounded directory removal. Cleanup
  failures are surfaced, including alongside the original fixture failure.
- Focused lifecycle coverage proves early child-exit detection, success/failure cleanup, process-stop
  ordering, peer-root isolation, and cleanup-error reporting.
- Both required managed Storage E2E commands passed, and neither left a new `kandev-e2e-*` root.
