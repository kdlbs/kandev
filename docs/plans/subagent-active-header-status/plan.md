---
spec: docs/specs/ui/subagent-observability.md
created: 2026-08-25
status: done
---

# Implementation Plan: Subagent active header status

## Overview

The collapsed subagent header already has a `GridSpinner` while `isSubagentEffectivelyActive` is true, but `task:working` is gated on `isActive && !hasExpandableContent`. A live wave with nested children therefore looks like a settled type + description row. This plan keeps Working + spinner on every active header, adds a red `IconX` after failed or cancelled finishes, and shows a muted completion check after success. Identity chips stay settled-card metadata. Unit tests cover the new states; existing `/e2e:subagent` specs assert the settled header. No backend or mock-agent change.

## Frontend

### `apps/web/components/task/chat/messages/tool-subagent-message.tsx`

- Delete `showInlineWorking`. Render `t("task:working")` whenever `isActive`, next to the existing `GridSpinner`, including when `hasExpandableContent` is true.
- Pass `metadata?.status` into `SubagentHeader`. When not active, if `normalizeToolCallStatus(status)` is `error` or `cancelled`, render a shrink-0 red `IconX` (`h-3.5 w-3.5 text-red-500`) with `aria-label` `task:failed` or `task:cancelled`. Pattern: `ExecuteStatusIcon` in `tool-execute-message.tsx`, but reuse `task:failed` / `task:cancelled` instead of command keys, and never render `IconCheck`.
- Keep `SubagentMetaRow` and the UUID/model/duration chip timing (`!isActive`). Preserve terminal nested payload precedence in `isSubagentEffectivelyActive`.

### Mobile design contract

- Desktop and mobile share the same header. No overlay, drawer, or navigation change.
- Nearest exemplar: `ExecuteStatusIcon` plus the existing subagent header (`min-h-11` expandable hit target below `sm`).
- Status glyph stays `shrink-0` so type, description, and Working truncate first.
- Parity proof: `mobile-subagent.spec.ts` asserts the settled header at phone width.

## Tests

- **What:** active card with children still shows Working + Loading spinner; complete card shows one Completed check; failed/cancelled show the labelled red mark.
  **File:** `apps/web/components/task/chat/messages/tool-subagent-message.test.tsx`.
  **How:** extend `ToolSubagentMessage` / expansion cases. Active + `childMessages` must show `task:working` and `role=status` name `Loading`. Complete + children must not. Failed/cancelled fixtures assert `getByLabelText` for Failed / Cancelled and no spinner.

## E2E Tests

- **Scenario:** GIVEN `/e2e:subagent` completes, WHEN the card is visible, THEN there is no Loading status, no Working copy, one Completed check, and the metadata row is visible.
  **Files:** `apps/web/e2e/tests/chat/subagent.spec.ts` and `apps/web/e2e/tests/chat/mobile-subagent.spec.ts`.
- Do not add a mock-agent scenario. The existing `scenarioSubagent` window is ~150ms; active-with-children and failed/cancelled are unit-tested.

## Verification Results

- `pnpm --filter @kandev/web test -- components/task/chat/messages/tool-subagent-message.test.tsx` — 52 passed.
- `pnpm exec eslint` + `pnpm run typecheck` on the two unit files — pass.
- `pnpm e2e:run --host --project chromium -- tests/chat/subagent.spec.ts` — 1 passed.
- `pnpm e2e:run --host --project mobile-chrome -- tests/chat/mobile-subagent.spec.ts` — 1 passed.

## Implementation Waves And Parallel Candidates

Wave 1:

- [x] [task-01-header-status-ui](task-01-header-status-ui.md)

Wave 2:

- [x] [task-02-subagent-status-e2e](task-02-subagent-status-e2e.md)

Sequential. E2E asserts the settled header produced by task 01.

## Risks

- Fresh worktrees need `cd apps && pnpm install --frozen-lockfile` before any pnpm command.
- E2E runs the production Vite build. Rebuild after the UI change (`pnpm e2e:run` does this; a stale `apps/web/dist` will miss the header).
- Do not invent `task:subagentFailed` keys. Reuse `task:working`, `task:failed`, `task:cancelled`, and `common:loadingIndicatorLabel`.

## Open Questions

None.
