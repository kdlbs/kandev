---
spec: ../../specs/ui/prompt-turn-duration.md
created: 2026-08-20
status: done
---

# Implementation Plan: Prompt Turn Duration on Message Hover

## Overview

The per-message hover action row (`MessageActions`) already resolves the message's turn via `useMessageTurnAndUsage`. We add a small exported duration helper to `lib/prompt-history.ts` that reuses the panel's nanosecond arithmetic, render an hourglass + compact duration in the row when the prompt's turn is completed, and cover the behavior with unit, component, and E2E tests. Order: helper + unit tests first, UI + component tests second, E2E last.

---

## Frontend

### Duration helper — `apps/web/lib/prompt-history.ts`

Add and export:

```ts
export function messageTurnDurationSeconds(message: Message, turn: Turn | null): number | null
```

- Returns `null` unless `message.author_type === "user"` AND `message.turn_id` is a non-empty string AND `turn` is non-null AND `turn.id === message.turn_id` AND `turn.session_id === message.session_id` AND `turn.completed_at` parses. The identity match mirrors the panel's composite `session_id:id` resolution in `turnCompletionByPrompt`: the helper receives the turn from the caller, so it must verify the turn is the message's own (a mismatched id or session returns `null`, table-tested).
- Otherwise returns `max(0, floor((completedNs − messageCreatedNs) / 1e9))`, where both ns values come from the existing private `epochNanoseconds()` helper (already used by `buildPromptHistoryEntries`). Unparseable `message.created_at` → `null` (never `NaN`).
- This reuses only the panel's timestamp parsing, floor, and clamp arithmetic for the completion bound (`Turn.completed_at − message.created_at`). It deliberately does NOT apply the panel's "earlier of turn completion and next prompt" bound: this row shows the resulting turn's duration only after completion. For two prompts sharing one completed turn the two rows' durations therefore differ (each measured from its own `created_at`), unlike the panel's rows.

### Component — `apps/web/components/task/chat/messages/message-actions.tsx`

- Import `IconHourglass` from `@tabler/icons-react`, and `formatPromptDuration` + `messageTurnDurationSeconds` from `@/lib/prompt-history` (extend the existing `prompt-history` import if one exists; otherwise add).
- In `MessageActions`, derive `const durationSeconds = messageTurnDurationSeconds(message, turn)` using the `turn` already resolved by `useMessageTurnAndUsage`.
- When `durationSeconds !== null`, render, after `<MessageMetaInfo>` (row's right end):

```tsx
<span
  data-testid="message-turn-duration"
  className="inline-flex items-center gap-1 whitespace-nowrap shrink-0 text-[10px] text-muted-foreground/60 font-mono"
>
  <IconHourglass className="h-3 w-3 shrink-0" aria-hidden="true" />
  {formatPromptDuration(durationSeconds, {
    s: t("task:durationUnitSeconds"),
    m: t("task:durationUnitMinutes"),
    h: t("task:durationUnitHours"),
  })}
</span>
```

  (`t` from the existing `useTranslation()`; text size/style matches the row's timestamp/model meta; hourglass matches the Prompt history panel's icon treatment.)

- No change to `chat-message.tsx` / `agent-message-content.tsx`: `MessageActions` is the shared row; the `group` hover wrapper already exists. Fine-pointer layouts keep the existing hover/focus reveal. `useTouchDrawer` keeps the row visible for coarse pointers at every width, so touch users do not depend on hover.

---

## Tests

| Behavior (spec scenario) | File | How |
|---|---|---|
| Completed turn → floored seconds from prompt send to completion | `apps/web/lib/prompt-history.test.ts` | Table-driven cases on `messageTurnDurationSeconds`: `5.9s → 5`, whole seconds, `0s` for sub-second |
| Running/never-completed turn → `null` | same | `turn` without `completed_at` |
| Unparseable `completed_at` → `null` (no `NaN`) | same | `completed_at: "not-a-time"` |
| Unparseable `created_at` → `null` | same | message with bad `created_at` |
| Agent message → `null` | same | `author_type: "agent"` with completed turn |
| No / empty `turn_id` or `turn: null` → `null` | same | absent `turn_id`; `turn_id: ""` (empty string) with a completed turn passed in; absent turn |
| Mismatched turn id or session → `null` | same | `turn.id !== message.turn_id`, or `turn.session_id !== message.session_id` |
| Clock skew → clamped to `0` | same | completion earlier than prompt send |
| Row shows hourglass + formatted duration for completed user prompt | `apps/web/components/task/chat/messages/message-actions.test.tsx` | Capture the store with `useAppStoreApi()` (hook, no `getState` — use a seeding wrapper component inside `StateProvider` or `renderHook`) and call `api.getState().addTurn(turn)` in `act()` (`addTurn` is ROOT-level, not `turns.addTurn`); render `MessageActions` with a user `Message` carrying `turn_id`; assert `getByTestId("message-turn-duration")` text (e.g. `5s`) and that its `querySelector("svg")` carries `aria-hidden` and `h-3 w-3` |
| Row shows no duration for a running turn | same | turn without `completed_at`; assert `queryByTestId("message-turn-duration")` is null |
| Deterministic multi-unit duration renders un-wrapped | `apps/web/components/task/chat/messages/message-actions.test.tsx` | User message whose turn completed 323s after `created_at` → text `5m 23s` (localized units) AND the span has `whitespace-nowrap` + `shrink-0` classes (only deterministic no-wrap proof; E2E wall-clock is uncontrolled) |
| `0s` duration still renders | same | Completed turn with `completed_at` before or within <1s of `created_at` → `message-turn-duration` present with localized `0s` (guards against a truthiness render guard; the UI guard must be `durationSeconds !== null`) |
| Row shows no duration for agent messages | same | `assistantMessage()` (existing helper) with completed turn seeded; assert absent |
| Existing timestamp/other row behavior unchanged | same | existing suites keep passing |

## E2E Tests

- **Scenario:** a settled session's user prompt row shows the turn duration on hover, formatted like the panel.
- **File:** `apps/web/e2e/tests/task/prompt-turn-duration.spec.ts` (new; modeled on `prompt-history-panel.spec.ts` — `createTaskWithAgent`, poll `settled` via `DONE_STATES`, open `/t/:id`, `SessionPage` helpers).
- **What to verify:** capture the seeded prompt's persisted message id BEFORE navigating by `expect.poll`-ing `apiClient.listSessionMessages(sessionId)` until the seeded user prompt appears (pre-navigation timing from `mobile-prompt-history-panel.spec.ts`, which reads messages ONCE after the settle poll — the `expect.poll` is an added robustness requirement so a terminal session state cannot precede message observability and strand the test); scope the row to the ACTIVE chat (``const chat = session.activeChat(); const row = chat.locator(`#msg-${messageId}`)`` with `toHaveCount(1)` — `#msg-<id>` exists on every mounted `MessageRow` and inactive dockview chat panels stay mounted); HOVER INSIDE the `.group` container, NOT the outer `#msg-<id>` — `row.getByTestId("user-message-bubble")` (or `.group` first match), because the full-width wrapper's center can fall outside the right-aligned bubble and `group-hover` would never fire (see task-03); assert the action row's computed opacity is `0` before hover and `1` after hover with `toHaveCSS` (NOT `toBeVisible` — Playwright ignores `opacity: 0`; `toHaveCSS` polls through the `transition-opacity` transition); assert `message-turn-duration` text matches `DURATION_SHAPE = /^\d+s$|^\d+m \d+s$|^\d+h \d+m \d+s$/` (wall-clock elapsed time is uncontrolled; shape-only, as the panel spec does); ALSO prove keyboard reveal INDEPENDENTLY of hover — move the mouse OUTSIDE the row after the hover assertions, assert the action row's opacity returns to `0`, THEN focus an existing action button inside the user row and assert the duration parent's computed opacity is `1` with the duration text present (the row reveals via `focus-within:opacity-100`; without the pointer-reset, a focused-while-hovered button would pass purely from `group-hover` and a focus-within regression would go unnoticed). Reuse the exact regex comment convention.
- **Build prerequisite (mandatory):** E2E runs the PREBUILT backend and web bundle — `fixtures/backend.ts` spawns `apps/backend/bin/kandev`, `build:e2e` only rebuilds the Vite bundle, and `global-setup.ts` fails fast on missing/stale backend artifacts. BEFORE any `pnpm e2e:raw`, run `make -C apps/backend build`, `make -C apps/backend e2e-plugin-package`, and `cd apps/web && pnpm run build:e2e` (`apps/web/e2e/README.md`). Desktop spec runs with `--project=chromium`; see task-03 for the exact block.
- **Mobile parity:** the duration is one more inline-flex span inside the existing action row (`flex items-center gap-2`); it changes no scroll container or touch interaction. Fine-pointer layouts keep the existing `group-hover`/`focus-within` reveal. `useTouchDrawer` keeps the row visible for coarse pointers at every width. Mobile spec `apps/web/e2e/tests/task/mobile-prompt-turn-duration.spec.ts` (mobile-chrome project, Pixel 5; modeled on `mobile-prompt-history-panel.spec.ts`) asserts the settled prompt's `message-turn-duration` renders inside the active chat at phone width and at a 700px coarse-pointer viewport, and that the action row (the duration's parent) has computed `opacity: 1` in both cases. It also asserts the duration span has computed `white-space: nowrap` (multi-unit durations must not wrap), the ACTION ROW has `scrollWidth <= clientWidth`, and the duration's bounding rect stays within the action-row rect; the outer `#msg-<id>` row check is only an additional assertion, because the user-message bubble and its wrappers are `overflow-hidden` (`chat-message.tsx`). The mobile spec can only assert no-wrap STYLING — E2E wall-clock is uncontrolled and normally sub-minute; the deterministic `5m 23s` no-wrap/geometry proof is task-02's component test (see Tests table). Implementer must run `/mobile-parity` and record rendered mobile verification in task-02's results.
- Mobile E2E coverage is MANDATORY, not discretionary (task-03 acceptance 3 requires `mobile-prompt-turn-duration.spec.ts`). `/mobile-parity` runs during implementation (task-02) and its rendered verification is recorded in task-02's results.

---

## Verification Results

- `cd apps/web && pnpm vitest run lib/prompt-history.test.ts` — passed: 1 file, 39 tests.
- `cd apps/web && pnpm vitest run components/task/chat/messages/message-actions.test.tsx lib/prompt-history.test.ts` — passed: 2 files, 50 tests.
- Focused ESLint and `cd apps/web && pnpm run typecheck` — passed.
- `make fmt && make -C apps/backend build && make -C apps/backend e2e-plugin-package && cd apps/web && pnpm run build:e2e` — passed.
- Desktop and Pixel 5 mobile duration E2E specs, plus `pnpm run e2e:sleep-ratchet` — passed.
- Fixup verification: `cd apps/web && pnpm vitest run components/task/chat/messages/message-actions.test.tsx lib/prompt-history.test.ts` — passed: 2 files, 51 tests. The new component regression confirms coarse-pointer action rows omit the `sm` hover-only opacity classes.
- Fixup verification: `make -C apps/backend build`, `make -C apps/backend e2e-plugin-package`, and `cd apps/web && pnpm run build:e2e` — passed. Desktop duration E2E and the Pixel 5 mobile duration E2E passed; the mobile spec also passed at a 700px coarse-pointer viewport. `pnpm run e2e:sleep-ratchet` passed.

---

## Implementation Waves And Parallel Candidates

Small feature; three sequential tasks, no parallel candidates (each depends on the previous):

```
Wave 1:
- [x] [task-01-duration-helper](task-01-duration-helper.md)

Wave 2:
- [x] [task-02-message-actions-ui](task-02-message-actions-ui.md)

Wave 3:
- [x] [task-03-e2e](task-03-e2e.md)
```

## Open Questions

None.
