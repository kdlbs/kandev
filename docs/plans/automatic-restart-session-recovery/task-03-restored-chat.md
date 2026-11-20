---
id: "03-restored-chat"
title: "Show recovered chat and verify both viewports"
status: done
wave: 3
depends_on:
  - "02-automatic-recovery"
plan: "plan.md"
requirements:
  - REQ-PLATFORM-DURABLE-RESTART-RECONCILIATION-001
  - REQ-PLATFORM-DURABLE-RESTART-RECONCILIATION-002
acceptance_criteria:
  - AC-PLATFORM-DURABLE-RESTART-RECONCILIATION-001.1
  - AC-PLATFORM-DURABLE-RESTART-RECONCILIATION-001.2
  - AC-PLATFORM-DURABLE-RESTART-RECONCILIATION-001.3
  - AC-PLATFORM-DURABLE-RESTART-RECONCILIATION-001.4
  - AC-PLATFORM-DURABLE-RESTART-RECONCILIATION-001.5
  - AC-PLATFORM-DURABLE-RESTART-RECONCILIATION-002.6
  - AC-PLATFORM-DURABLE-RESTART-RECONCILIATION-002.9
  - AC-PLATFORM-DURABLE-RESTART-RECONCILIATION-002.10
  - AC-PLATFORM-DURABLE-RESTART-RECONCILIATION-002.11
system_design:
  - ../../specs/platform/system-design/session-recovery-convergence.md
---

# Task 03: Show recovered chat and verify both viewports

## Summary

Render the restored incident as one informational chat message. The shared
recovery model returns the ordinary composer after committed recovery.
Prove automatic recovery and a later instruction on desktop and phone.

## In scope

- The typed status notice from Task 02 and its translated transcript renderer.
- The same notice after explicit native Resume, including recovered-notice restoration.
- Committed recovery precedence over stale incident controls or local errors.
- Preserved drafts, delayed snapshots, reload, and a genuine independent blocker.
- English, Portuguese, Chinese, Japanese, Korean, and generated pseudo catalogs.
  Generate the Traditional Chinese pair with `pnpm run i18n:zh-hant`.
- Desktop and phone E2E with real restart and backend proof.
- An ordinary interrupted-restart case without earlier terminal history in both specs.
- An upgrade case that recovers an existing failed session on first startup.
- Public session and task recovery guidance after the behavior is implemented.

## Out of scope

- Hiding an unresolved block by session state, process readiness, or local dismissal.
- Removing recovery actions for missing native state or conflicting live ownership.
- Changing phone navigation, desktop preferences, or provider capability contracts.

## Acceptance

1. The recovered incident shows one informational message and an ordinary usable
   composer. This includes an existing failed session on first startup after upgrade.
   It requires no Resume or Retry connection, including after reload.
2. A restored draft can submit a distinct instruction in the same native
   conversation. The old prompt is never repeated. An independent blocker remains visible.
3. Desktop and phone use the shared model. The phone notice wraps, its input
   clears the keyboard/safe area, and Send remains reachable by touch.

## ASCII UI preview

UI-02 from the [combined preview](plan.md#ascii-ui-preview), after automatic
recovery on either viewport:

```text
Transcript
The session was interrupted. Your conversation is restored. You can continue.

[Type your next instruction...                         ] [Send]
```

Use the existing task chat layout and single transcript scroller. The normal
composer keeps its fixed placement. Phone copy wraps above the existing composer.
During pending recovery the draft remains editable and Send waits for readiness.
Do not add a drawer or action row for a recovered incident. Structure is required.
Spacing is illustrative. The notice uses the canonical English string above and
its translations. Maps to 001.4 and 002.10.

## Verification

Install the workspace once before the first pnpm command in a fresh worktree.
Run these blocks from the repository root. Keep the E2E commands sequential.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run components/task/chat/messages/session-recovery-notice.test.tsx components/task/chat/session-recovery-model.test.ts components/task/chat/session-recovery-context.test.tsx hooks/domains/session/use-session-recovery-actions.test.ts lib/state/slices/session/session-merge-delivery-recovery.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm e2e:run --project chromium tests/session/automatic-restart-recovery.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/session/mobile-automatic-restart-recovery.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

The two new E2E files must prove recovery before task navigation. Use existing
worker-owned backend restart and durable seeding helpers. Do not mock a successful
`session.recover` response or click a recovery button to reach the asserted state.
Seed the upgrade case before the corrected backend starts. Do not create a new
interruption or submit a prompt to trigger its recovery. Assert the same native
conversation, then navigate and check its composer and single informational notice.
Both specs include ordinary interrupted restart without older terminal history,
the historical-terminal case, and the existing-failed-session upgrade case.

### Startup without browser participation

Acquire `testPage` before seeding, because its fixture resets persisted task state.
Retain its browser context, then close all pages in that test-owned context.
Assert `context.pages()` is empty immediately before `backend.restart()` and
through the API observation below. No app document or WebSocket remains mounted.
Create the replacement page only after that observation succeeds.

Use `expect.poll` over `apiClient.getTaskSession(sessionId)`, the authorized
`GET /api/v1/task-sessions/:id` detail read used by
`captureSilentRestartSnapshot` in `silent-restart-recovery.ts`. Require:

- The exact session and native ACP ID remain unchanged.
- `state` is `WAITING_FOR_INPUT` with a nonempty current `agent_execution_id`.
- `metadata.agent_delivery_recovery.phase` is `restored` for the interrupted
  source submission, rather than a previous incident's restored notice.
- `session_recovery_blocks` is explicitly empty for the fixture without an
  independent blocker. An omitted projection is insufficient.

Also poll `apiClient.listSessionMessages(sessionId)` for exactly one status row
with the expected source identity and `kind: session_restored_after_interruption`.
Require its canonical content fallback and deterministic ID. Neither read may
invoke launch, `session.recover`, workspace hydration, or prompt admission.
These observations must succeed while the context still has no pages.

Then create a page in the retained context, call the existing `setupPage` helper,
and navigate to the task. Assert UI with the standard timeout. The existing
`prCapture` fixture is bound to the closed `testPage`, so use `PrAssetCapture`
from `e2e/helpers/pr-asset-capture.ts` with the replacement page and flush it
through the same cleanup pattern. Capture desktop and phone screenshots after
the committed observation. Close the replacement page during cleanup.
Assert a new turn-specific result for the follow-up, not an earlier transcript row.

## Files likely touched

- `apps/web/components/task/chat/messages/action-message.tsx`
- `apps/web/components/task/chat/messages/session-recovery-notice.tsx` (new)
- `apps/web/components/task/chat/messages/session-recovery-notice.test.tsx` (new)
- `apps/web/components/task/chat/session-recovery-model.ts`
- `apps/web/components/task/chat/session-recovery-context.tsx`
- `apps/web/lib/state/slices/session/`, only if revision merge correction is required.
- `apps/web/src/locales/*/task.json`
- `apps/web/e2e/helpers/automatic-restart-recovery.ts` (new)
- `apps/web/e2e/tests/session/automatic-restart-recovery.spec.ts` (new)
- `apps/web/e2e/tests/session/mobile-automatic-restart-recovery.spec.ts` (new)
- Existing E2E-only seed helpers if the historical terminal fixture needs extension.
- `docs/public/sessions-and-review.md` and `docs/public/tasks-and-workflows.md`

## Dependencies

Task 02 supplies automatic proof, restored publication, and the durable status notice.

## Risks

A local success latch can hide a real block or lose a draft after remount.
Notice publication and stale events must not switch the native conversation.
E2E must use isolated fixture processes, never the developer's running instance.
Public documentation must describe the implemented eligibility limits accurately.

## Parallelism

`sequential`

## Inputs

- Convergence design: Historical terminal incidents and Restored chat presentation.
- Task 02 results and the UI-02 preview.
- Mobile parity, E2E, frontend scoped guidance, and public documentation guidance.
- Existing `restart-reconciliation.ts` and `silent-restart-recovery.ts` E2E helpers.

## Results

- Focused recovery UI tests passed: 5 files, 46 tests. Web typecheck and
  `pnpm run i18n:check` passed.
- Desktop automatic-restart E2E passed all 3 scenarios: ordinary interruption,
  historical terminal incident, and first-startup upgrade recovery.
- Phone E2E passed the same 3 scenarios and verified the usable composer.
- Public documentation validation passed: 63 tests and 55 published pages.
- Specification catalog validation found 450 decisions and 1995 specifications;
  the complete specification lint passed.
- `git diff --check` passed.
- Peer-review follow-up preserves a newer session or request-local failure after
  an older restoration. The five-file focused recovery UI suite passed 46 tests;
  web typecheck and `pnpm run i18n:check` passed. Desktop and phone production
  E2E each passed all three restart and upgrade scenarios.
- Remote Docker confirmed-absence Resume passed on desktop and phone, including
  the assertion that the interrupted prompt remains in the transcript without
  being sent again.
