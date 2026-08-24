---
id: "02-message-actions-ui"
title: "Render duration in message action row"
status: done
wave: 2
depends_on: ["01-duration-helper"]
plan: "plan.md"
spec: "../../specs/ui/prompt-turn-duration.md"
---

# Task 02: Render duration in message action row

- **Acceptance:**
  1. `MessageActions` in `apps/web/components/task/chat/messages/message-actions.tsx` renders, for a user prompt whose turn is completed, a `data-testid="message-turn-duration"` span: `IconHourglass` (`h-3 w-3`, `aria-hidden`) + `formatPromptDuration(seconds, { s/m/h })` with the existing `task:durationUnitSeconds|Minutes|Hours` keys, styled like the row's other meta (`inline-flex items-center gap-1 text-[10px] text-muted-foreground/60 font-mono`) PLUS `whitespace-nowrap shrink-0` so a multi-unit duration (e.g. `5m 23s`) never wraps or compresses on narrow widths, placed after `<MessageMetaInfo>`. The render guard MUST be `durationSeconds !== null` — never a truthiness check like `{durationSeconds && …}`, which would silently omit the required `0s` (0 is falsy).
  2. No duration renders for running turns, agent messages, or messages without a resolvable completed turn (no placeholder).
  3. Component tests in `apps/web/components/task/chat/messages/message-actions.test.tsx` cover: completed user prompt shows hourglass + formatted duration; running turn shows none; agent message shows none; AND a completed turn whose `completed_at` is before or within <1s of `created_at` renders the span with the localized `0s` text (catches a truthiness-guard regression that drops 0). Assert the hourglass by scoping to the duration element — `getByTestId("message-turn-duration").querySelector("svg")` with `aria-hidden` and `h-3 w-3` — never a bare `container.querySelector("svg")` (the row already renders other SVGs).
  4. A DETERMINISTIC multi-unit case: a user message whose turn completed 323 seconds after `created_at` renders the `5m 23s` text (localized units) and the span carries the `whitespace-nowrap` and `shrink-0` classes — this is the ONLY deterministic no-wrap proof, because E2E wall-clock durations are uncontrolled and normally sub-minute.
- **Verification:**
  ```sh
  cd apps/web && pnpm vitest run components/task/chat/messages/message-actions.test.tsx lib/prompt-history.test.ts
  cd apps/web && pnpm exec eslint components/task/chat/messages/message-actions.tsx components/task/chat/messages/message-actions.test.tsx lib/prompt-history.ts lib/prompt-history.test.ts
  cd apps/web && pnpm run typecheck
  ```
- **Files likely touched:**
  - `apps/web/components/task/chat/messages/message-actions.tsx`
  - `apps/web/components/task/chat/messages/message-actions.test.tsx`
- **Dependencies:** task 01 (helper must exist).
- **Parallelism:** sequential.
- **Inputs:**
  - Spec: `docs/specs/ui/prompt-turn-duration.md` — `What` and `Scenarios`.
  - Plan: `docs/plans/prompt-turn-duration/plan.md` — Frontend "Component", Tests table.
  - Patterns: `PromptDuration` in `apps/web/components/task/prompt-history-panel-content.tsx` (hourglass + `formatPromptDuration` with `t()` unit keys); existing `MessageMetaInfo` styling in `message-actions.tsx`.
  - The store already resolves `turn` via `useMessageTurnAndUsage`. To seed test turns, capture the store with `useAppStoreApi()` (from `@/components/state-provider`; `useAppStore` is a hook without `getState`) inside a seeding wrapper component rendered within `StateProvider` (or via `renderHook`), then call `api.getState().addTurn(turn)` inside `act()`. `addTurn` is a ROOT-level store action (see `apps/web/lib/state/slices/session/turn-actions.ts` and its use in `turn-actions.test.ts`) — never `turns.addTurn`. The `assistantMessage()` helper in the test file is `author_type: "agent"`; add a user-message factory with `turn_id`.
  - Mobile parity (required, not optional): the duration is one more inline-flex span in the existing action row — it must not change the row's composition in a way that clips or wraps on narrow widths, must not add a scroll container, and must keep the row reachable for coarse pointers at every width. Fine-pointer layouts retain the existing `group-hover`/`focus-within` reveal. Run `/mobile-parity` before finishing and record the rendered mobile verification in `## Results`; the mobile E2E scenario lives in task-03.
- **Output contract:** summary; files changed; tests run with counts; eslint/typecheck results; blockers/risks; update task status and `plan.md` checkbox.

## Results

- TDD red: `cd apps/web && pnpm vitest run components/task/chat/messages/message-actions.test.tsx lib/prompt-history.test.ts` failed as expected: completed and `0s` duration spans were absent.
- Verification: `cd apps/web && pnpm vitest run components/task/chat/messages/message-actions.test.tsx lib/prompt-history.test.ts` passed: 2 files, 50 tests.
- Verification: `cd apps/web && pnpm exec eslint components/task/chat/messages/message-actions.tsx components/task/chat/messages/message-actions.test.tsx lib/prompt-history.ts lib/prompt-history.test.ts` passed with no warnings.
- Verification: `cd apps/web && pnpm run typecheck` passed.
- Mobile parity: content-only inline addition to the existing action row; nearest mobile exemplar is `mobile-prompt-history-panel.spec.ts`. The existing below-`sm` `opacity-100` reveal, scroll owner, and touch behavior are unchanged. Rendered Pixel 5 verification passed with `cd apps/web && pnpm e2e:raw tests/task/mobile-prompt-turn-duration.spec.ts --project=mobile-chrome`: the duration was visible, opaque, unwrapped, and contained in its action row.
- Generated artifacts: release-notes/changelog pretypecheck output only; no generated source artifacts to remove.
- Fixup verification: `cd apps/web && pnpm vitest run components/task/chat/messages/message-actions.test.tsx lib/prompt-history.test.ts` — passed: 2 files, 51 tests. The new regression confirms the action row stays visible for coarse pointers at tablet widths.
- Fixup verification: focused ESLint, `cd apps/web && pnpm run typecheck`, and `cd apps/web && pnpm run i18n:check` — passed.
