---
id: "01-reconcile-inactive-responses"
title: "Reconcile inactive clarification responses"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-CLARIFICATION-LIFECYCLE-001
  - REQ-TASKS-CLARIFICATION-RESPONSE-RELIABILITY-001
acceptance_criteria:
  - AC-TASKS-CLARIFICATION-LIFECYCLE-001.2
  - AC-TASKS-CLARIFICATION-RESPONSE-RELIABILITY-001.1
  - AC-TASKS-CLARIFICATION-RESPONSE-RELIABILITY-001.4
system_design:
  - ../../specs/tasks/system-design/clarification-active-lifecycle.md
  - ../../specs/tasks/system-design/clarification-response-reliability.md
---

# Task 01: Reconcile inactive clarification responses

## Follow-up scope

This work order records the completed removal-only implementation at PR #3799
head `6edc7320dd`. The user's later direction supersedes that presentation.
[Task 02](task-02-late-answer-messages.md) adds answers as ordinary messages.
The results below are historical evidence, not validation of the new scope.

## Summary

Close stale task-chat questions when X receives an authoritative inactive response.
Keep expiry distinct from successful rejection and retryable failure.

## In scope

- Shared hook reconciliation against the latest store snapshot.
- Non-actionable expired rendering and response-shortcut cleanup.
- Focused hook/component regressions and desktop/phone browser evidence.
- Accurate plan and work-order results after implementation.

## Out of scope

Backend changes, permanent dismissal storage, new UI copy, layout redesign,
and changing Escape/collapse into rejection.

## Acceptance

1. A recognized inactive response retires only matching pending rows in the
   submitted bundle. Task chat removes the panel without a WebSocket event.
   Preserve terminal siblings, unrelated bundles, deleted rows, and newer metadata.
2. A static expired overlay exposes no answer, Skip, Submit, or Retry action.
   Expiry reports `no_longer_active`, never successful rejection. Unknown or
   malformed responses and transport failures preserve retryable behavior.
3. A delayed A response cannot alter B's submission state or callbacks. A later
   authoritative pending restoration remains answerable, including the same ID.
   Desktop and phone pass the same inactive-dismissal flow.

## Implementation sequence

1. Read the linked requirements and design's inactive-response section.
2. Add the regression before changing production code. Mock `409 not_active`,
   click X, and assert that no actionable stale question remains.
3. Reconcile submitted rows using their latest message-cache values. Reuse
   `isPendingClarificationMessage`, including its missing-status compatibility.
   Compare each latest `updated_at` with its submitted snapshot and preserve a
   newer authoritative row while retiring unchanged pending siblings. Fence
   cache expiry when the request generation is stale and the same pending ID is
   current again; still retire an old bundle while another pending ID is active.
4. Keep current request-generation checks for UI state and callbacks. Test a
   mixed bundle containing pending and terminal rows, plus an unrelated live bundle.
5. Render the expired notice without response actions for static hosts. Prevent
   repeated hook submissions until replacement or authoritative restoration.
   An ordinary rerender of stale props must not reset expiry. Do not arm the
   Escape guard when the expired notice has no keyboard handler.
6. Add desktop and phone coverage, run the commands below, and record results.

## ASCII UI preview

UI-01: Task chat after inactive X response, shared desktop/phone structure.

```text
Before: [Question / expired notice / X / answers]
After:  [Conversation / question history]
        [Message composer              ]
```

See the [full preview](plan.md#ascii-ui-preview) for composition and constraints.
The stale panel disappears; static hosts retain only the non-actionable notice.
Keep current phone scroll ownership, safe areas, and remaining touch targets.
This view covers lifecycle AC `.2` and response-reliability AC `.1`.

## Verification

Run from the repository root. Install dependencies first in a fresh worktree.
Run the new regression alone before the correction and record its expected failure.
Then run the complete checks below; E2E commands run sequentially.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run hooks/domains/session/use-clarification-group.test.ts hooks/domains/session/use-clarification-group.regressions.test.ts hooks/domains/session/use-clarification-group.timeout.test.ts components/task/chat/clarification-input-overlay.test.tsx components/task/chat/clarification-panel-section.test.tsx)
(cd apps/web && pnpm exec eslint hooks/domains/session/use-clarification-group.ts components/task/chat/clarification-input-overlay.tsx)
(cd apps/web && pnpm run typecheck)
make build-web
make build-backend
(cd apps/web && pnpm e2e:run --no-build --project chromium -- tests/chat/clarification.spec.ts --grep 'inactive dismissal')
(cd apps/web && pnpm e2e:run --no-build --project mobile-chrome -- tests/chat/mobile-clarification.spec.ts --grep 'inactive dismissal')
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Use the current E2E fixture and causal-wait helpers. Intercept only the target
response and retain stale client data so a message event cannot hide the bug.
Inspect the phone rendering against UI-01. Do not claim screenshots or E2E
results from the design turn; those checks have not run.

## Files likely touched

- `apps/web/hooks/domains/session/use-clarification-group.ts`
- `apps/web/hooks/domains/session/use-clarification-group.test.ts`
- `apps/web/hooks/domains/session/use-clarification-group.regressions.test.ts`
- `apps/web/hooks/domains/session/use-clarification-group.timeout.test.ts`
- `apps/web/components/task/chat/clarification-input-overlay.tsx`
- `apps/web/components/task/chat/clarification-input-overlay.test.tsx`
- `apps/web/components/task/chat/clarification-panel-section.test.tsx`
- `apps/web/e2e/tests/chat/clarification.spec.ts`
- `apps/web/e2e/tests/chat/mobile-clarification.spec.ts`

Read-only integration points include `lib/utils/pending-clarification.ts`,
`hooks/use-processed-messages.ts`, `components/task/task-chat-panel.tsx`,
`components/quick-chat/quick-chat-content.tsx`, and `components/runs/run-transcript.tsx`
under `apps/web/`. Inspect the Inbox's existing outcome consumer too.

## Dependencies

None. Execute only after the user's explicit implementation request.

## Risks

Local expiry must not become a backend write or permanent suppression. Read
current rows before cache updates; a submitted snapshot can contain stale metadata.
The report's exact runtime cause remains unconfirmed beyond this reproduced path.

## Parallelism

`sequential`

## Inputs

- [Evidence and root cause](plan.md#evidence-and-root-cause).
- [Lifecycle requirements](../../specs/tasks/requirements/clarification-active-lifecycle.md).
- [Response requirements](../../specs/tasks/requirements/clarification-response-reliability.md).
- [Client recovery design](../../specs/tasks/system-design/clarification-response-reliability.md#inactive-response-reconciliation).
- Existing expired-banner test and successful Skip/store-update tests.

## Results

Implemented the inactive-response repair. A recognized `409 not_active` now
reconciles only matching pending rows from the latest message cache to local
`expired` status, preserving newer metadata and terminal siblings while
skipping deleted rows and unrelated bundles. A newer authoritative `updated_at`
wins over a delayed inactive response, and stale A→B→A generations cannot
expire the bundle that is current again. The overlay exposes only the existing
expired notice, does not arm its Escape guard, and direct submit, Skip, and
Retry calls are guarded until a replacement or authoritative pending
restoration arrives.

Added shared hook and overlay regressions, delayed-response coverage, and
desktop/mobile E2E scenarios. The browser-first regression failed before the
production change at the expected cache-update and stale-control assertions.
The review regressions then failed before the ordering and Escape-guard
correction, and pass after it.

Validation passed:

- Focused Vitest: 5 files, 96 tests.
- Strict timestamp comparison preserves RFC3339Nano ordering and rejects
  malformed or Date-normalized values.
- Selective expiry preserves a newer restored row while retiring an unchanged
  pending sibling in the same bundle.
- Targeted ESLint: no errors or warnings.
- Prettier and TypeScript typecheck.
- `make build-web` and `make build-backend`.
- Desktop inactive-dismissal E2E: 1 passed.
- Phone inactive-dismissal E2E: 1 passed.
- E2E fixture plugin packaging.
- Documentation catalog validation, full specification lint, and
  `git diff --check`.

The full specification lint first exposed a pre-existing duplicate acceptance
ID in the queued-session ownership requirements. The unrelated
conversation-surface criterion was renumbered to
`AC-TASKS-QUEUED-SESSION-OWNERSHIP-003.10`; the final catalog lint passed.
Companion parking work orders now reference that corrected criterion.
