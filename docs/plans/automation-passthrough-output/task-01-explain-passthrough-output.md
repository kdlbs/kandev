---
id: "01-explain-passthrough-output"
title: "Explain passthrough capture and exclude false notices"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-OFFICE-AUTOMATION-RUNS-001
  - REQ-UI-EMPTY-TURN-NOTICE-001
acceptance_criteria:
  - AC-OFFICE-AUTOMATION-RUNS-001.13
  - AC-OFFICE-AUTOMATION-RUNS-001.14
  - AC-OFFICE-AUTOMATION-RUNS-001.15
  - AC-OFFICE-AUTOMATION-RUNS-001.16
  - AC-OFFICE-AUTOMATION-RUNS-001.17
  - AC-OFFICE-AUTOMATION-RUNS-001.18
  - AC-UI-EMPTY-TURN-NOTICE-001.11
system_design:
  - ../../specs/office/system-design/automation-run-output.md
  - ../../specs/office/system-design/automation-runs.md
  - ../../specs/ui/system-design/empty-turn-notice.md
---

# Task 01: Explain passthrough capture and exclude false notices

## Summary

Add capture guidance to the selected passthrough run while preserving its saved
messages and reply composer. Suppress false empty-turn notices at creation and
rendering, including notices created before session hydration.

## In scope

- Use the existing session-mode helper and selected-session hydration.
- Show the two-part localized explanation from the Office design above the message region.
- Pass session mode through the shared notice decision and message-filter pipeline.
- Add real-store/component and pure filtering tests before production changes.
- Update every locale and the public automation how-to.

## Out of scope

Terminal attachment, output capture, provider transcript parsing, backend output
signals, run-state repair, new profile actions, and public API changes.

## Acceptance

1. Running, parked, directly loaded, and newly selected passthrough sessions
   show the design's capture explanation. Structured sessions retain their existing behavior.
2. Real completion events suppress passthrough warnings using their own session
   identity. A warning inserted before hydration disappears when mode becomes known.
3. Saved text, genuine errors, composer actions, and status remain available.
   Reading the view creates no terminal connection or automatic execution request.

## Implementation and TDD

1. Add `explains uncaptured output for a parked passthrough run` to
   `run-transcript.test.tsx`. Add the real-store passthrough completion regression
   in `turns.test.ts`. Run them before production edits and record the two RED failures.
2. Extend the existing transcript API mock with a valid `listTaskSessions`
   response. Preserve the real store and `useChatPanelState` in component tests.
3. Use `isPassthroughSession(panelState.session)` for guidance. Retain the
   message list, focus behavior, and submission/cancellation handlers.
4. Add `isPassthrough` to `EmptyTurnNoticeInput`. Resolve it from
   `taskSessions.items[payload.session_id]` in `maybeEmitEmptyTurnNotice`.
5. Carry mode through `useSessionData`, `ProcessedMessagesOptions`, and
   `useVisibleMessages` to `filterVisibleMessages`. Remove only synthetic
   empty-turn entries from presentation. Include mode in memo dependencies.
6. Cover true flag, legacy snapshot fallback, explicit false precedence,
   different globally active session, unchanged-message hydration, preserved errors,
   running/parked state, and selection back to structured mode.
7. Add English and real-locale keys in the `automations` namespace. Generate
   Traditional Chinese with `pnpm run i18n:zh-hant` and pseudo with `pnpm run i18n:pseudo`.
8. Clarify capture limitations in `docs/public/automation-and-mcp.md`.

Unknown mode retains the existing notice behavior until hydration. Profile
edits must not reclassify an existing run. Do not set `had_output=true` to hide
the problem or delete warnings from persisted/store history.

## ASCII UI preview

UI-01 and UI-02 excerpt from [the combined preview](plan.md#ascii-ui-preview):

```text
DESKTOP CONTENT             PHONE CONTENT
+-----------------------+   +-----------------------+
| Capture explanation   |   | Run status     [Runs] |
| Future-session advice |   | Capture explanation   |
| Stored messages       |   | Future-session advice |
| Genuine errors        |   | Stored messages       |
| [Reply to this run]   |   | [Reply]        [Send] |
+-----------------------+   +-----------------------+
```

Office .13 through .18 require this hierarchy. The rail/drawer remains in its
current viewport-specific owner. Guidance wraps without horizontal overflow.
The existing message region scrolls, and the composer keeps safe-area handling.
Preview spacing and shortened copy are illustrative. Use the design's localized text.

## Verification

Run these commands from the repository root. In a fresh worktree, first run
`pnpm install --frozen-lockfile` from `apps/`.

```bash
(cd apps/web && pnpm exec vitest run \
  components/runs/run-transcript.test.tsx \
  lib/ws/handlers/empty-turn-notice.test.ts \
  lib/ws/handlers/turns.test.ts \
  hooks/processed-message-filtering.test.ts \
  hooks/processed-message-filtering-passthrough.test.ts \
  hooks/use-processed-messages.test.ts \
  lib/session/is-passthrough-session.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec oxlint -c oxlint.config.mjs --deny-warnings \
  components/runs/run-transcript.tsx components/runs/run-transcript.test.tsx \
  components/task/chat/use-chat-panel-state.ts \
  hooks/processed-message-filtering.ts hooks/processed-message-filtering.test.ts \
  hooks/processed-message-filtering-passthrough.test.ts \
  hooks/use-processed-messages.ts hooks/use-processed-messages.test.ts \
  lib/ws/handlers/empty-turn-notice.ts lib/ws/handlers/empty-turn-notice.test.ts \
  lib/ws/handlers/turns.test.ts)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check
```

Run the [plan's documentation gates and coverage preflight](plan.md#verification-results).
Work order 02 supplies the required rendered desktop/phone proof.
If file-limit extraction adds a source/test file, include that exact path in
the unit and lint commands and record the updated command here.

## Files likely touched

- `apps/web/components/runs/run-transcript.tsx`
- `apps/web/components/runs/run-transcript.test.tsx`
- `apps/web/components/task/chat/use-chat-panel-state.ts`
- `apps/web/lib/ws/handlers/empty-turn-notice.ts`
- `apps/web/lib/ws/handlers/empty-turn-notice.test.ts`
- `apps/web/lib/ws/handlers/turns.test.ts`
- `apps/web/hooks/processed-message-filtering.ts`
- `apps/web/hooks/processed-message-filtering.test.ts`
- `apps/web/hooks/processed-message-filtering-passthrough.test.ts`
- `apps/web/hooks/use-processed-messages.ts`
- `apps/web/hooks/use-processed-messages.test.ts`
- `apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw,ja,ko,pseudo}/automations.json`
- `docs/public/automation-and-mcp.md`

## Dependencies

None. Read the existing `is-passthrough-session` helper and its tests before implementation.

## Risks

The snapshot fallback cannot override explicit false. Stale memoized filtering
can preserve an invalid warning after hydration. Broad status filtering can hide
real failures. Capture guidance must not imply that old output is recoverable.

## Parallelism

`sequential`

## Inputs

- [Office requirement](../../specs/office/requirements/automation-runs.md#run-output-capture).
- [Office output design](../../specs/office/system-design/automation-run-output.md).
- [Shared notice design](../../specs/ui/system-design/empty-turn-notice.md#passthrough-exclusion).
- Existing run transcript, mode helper, notice, and filtering tests.
- Repository `/tdd`, `/mobile-parity`, `/e2e`, and `/docs-maintainer` skills.

## Results

Implemented selected-session capture guidance and shared passthrough exclusion.
The targeted RED checks failed before production changes; all 171 focused tests
then passed across seven files. Frontend typecheck and focused Oxlint passed.
`i18n:zh-hant`, `i18n:pseudo`, `i18n:check`, and `i18n:ratchet` passed. Public
documentation validation passed: 63 tests and 55 published pages. The final
package documentation coverage preflight covered the implementation paths with
no reference errors. `git diff --check` passed after work order 02.
