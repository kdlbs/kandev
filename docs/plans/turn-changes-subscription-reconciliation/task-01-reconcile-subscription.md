---
id: "01-reconcile-subscription"
title: "Reconcile turn changes after subscription"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-TURN-CHANGES-005
acceptance_criteria:
  - AC-TASKS-TURN-CHANGES-005.1
  - AC-TASKS-TURN-CHANGES-005.9
system_design:
  - ../../specs/tasks/system-design/turn-changed-files-subscription.md
---

# Task 01: Reconcile subscription gap

## Summary and scope

Refresh the newest history page after the session subscription is ready so a
terminal update between the initial request and subscription is not lost. Keep
existing offline reads and revision-aware merging. Release the subscription on
unmount and session switches.

## Acceptance

- The final summary appears after readiness, including when the initial request
  remains in flight; a newer live revision cannot be overwritten.
- Reconnection and cleanup remain bounded and existing pagination works.
- Desktop and mobile changed-file browser scenarios pass.

## UI-01: Transcript

```text
[assistant reply]
[Changed files] [Open diff]
```

Desktop and phone use the existing transcript and diff surfaces; see [plan](plan.md).

## Verification

```bash
(cd apps && pnpm --filter @kandev/web test -- --run hooks/domains/session/use-turn-changes-subscription.test.tsx hooks/domains/session/use-turn-changes-pagination.test.tsx lib/state/slices/session/turn-changes-actions.test.ts)
(cd apps/web && ../../scripts/run-quiet e2e --summary -- pnpm e2e:run e2e/tests/git/turn-changed-files.spec.ts -- --grep "keeps the captured first turn after later edits and reload" --retries=0)
(cd apps/web && ../../scripts/run-quiet e2e --summary -- pnpm e2e:run --no-build --project mobile-chrome e2e/tests/git/mobile-turn-changed-files.spec.ts -- --retries=0)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
```

## Files likely touched

- `apps/web/hooks/domains/session/use-turn-changes.ts` and focused tests
- `docs/specs/tasks/system-design/turn-changed-files-subscription.md`

## Dependencies

None. Sequential.

## Risks

An old in-flight page must settle before the post-subscription read begins.

## Results

The initial regression failed because no post-subscription refresh occurred.
The focused suites passed (8 tests), including reconnect, unmount, initial-read
failure recovery, an overlapping pre-subscription read, and a newer live
revision. Desktop E2E passed (1); the mobile spec passed (3), without retries.
The full web suite passed (3,113 files, 28,285 tests, 4 skipped); the added
error-recovery test passed in the subsequent focused run. Lint, typecheck,
and specification validation passed.

### SDK branch integration (2026-10-27)

Combine reactive WebSocket client ownership with the branch's background-only
refresh and post-await consumer guards. Client installation or replacement
while connection status stays connected must release the old lease, acquire
the new lease, and reconcile after its readiness acknowledgment. A real-store
regression fails with the previous singleton-only lookup (zero subscriptions)
and covers installation, replacement, revision refresh and balanced release.
Retain the incoming readiness/reconnect cases, existing session-switch guards,
active-detail gating and historical-viewer visibility propagation. Remove
only duplicate prop declarations introduced by automatic merge. No read demand
or product interaction changes are intended. Fresh combined tests and builds
are required before delivery.

The merged 125-test selection, full typecheck and scoped lint pass. The phone
historical-capture case passes on a fresh combined build with retries disabled.
Desktop integration and fresh current-head CI remain delivery gates.

Desktop historical-capture validation also passes against the combined source
with retries disabled. Fresh remote CI remains the delivery gate.
