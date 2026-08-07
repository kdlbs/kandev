---
spec: docs/specs/ui/context-window-unmeasured-state.md
created: 2026-08-07
status: completed
---

# Implementation Plan: Context Window Unmeasured State

## Overview

Make the context-window hover honest before the first positive ACP usage sample arrives. The
existing session-runtime entry and ACP/WebSocket pipeline remain the source of truth; the frontend
will render used === 0 as an unmeasured state rather than a measured 0%. A focused component
test plus the existing desktop and mobile context-window E2E surfaces will prove the pending state
and the unchanged measured state.

## Backend/API

No backend, ACP adapter, WebSocket, persistence, or store-schema changes are planned. The
existing ContextWindowEntry contract already carries the window size, current usage, source, and
compaction count. This plan deliberately treats a reliable used === 0 entry as "no positive
sample has been observed for display" and confines that interpretation to the context hover.

## Frontend

### Pending context-window presentation

- Update apps/web/components/task/chat/token-usage-display.tsx so the existing reliable-data
  branch distinguishes used === 0 from positive usage without changing the existing used > size
  guard.
- Keep the ring trigger visible and empty while pending, but use a translated pending accessible
  label instead of interpolating 0%.
- In the tooltip, render translated "—%" and "— of {{size}} tokens" values plus a short pending
  status sentence. Keep the source and compaction rows in their current positions.
- Leave the positive, exact-full, and impossible-report branches behaviorally unchanged.
- Extend apps/web/components/task/chat/token-usage-display.test.tsx with a pending-state case that
  opens the tooltip, asserts the accessible label and pending copy, and proves numeric 0% and
  0 of are absent. Retain the existing positive, source, compaction, and dismissal coverage.

### Translation catalog

- Add the pending accessible label, percentage/token placeholders, and pending explanation to
  apps/web/src/locales/en/task.json.
- Regenerate or update apps/web/src/locales/pseudo/task.json through the repository's existing
  i18n workflow; do not hardcode new visible strings in the component.
- Run both the i18n key check and ratchet because the changed component and catalogs are user-facing
  frontend files.

### Mobile design contract

- **Desktop outcome:** the current context ring and tooltip gain only a clear pre-sample content
  state; the measured state keeps its current compact layout.
- **Mobile entry point and exemplar:** the same context ring remains the entry point and remains
  tap-pinnable. apps/web/e2e/tests/chat/mobile-context-window-source.spec.ts is the nearest
  shipped mobile exemplar.
- **Hierarchy and presentation:** the ring, pending percentage, token row, source, and compaction
  row remain in the existing tooltip. No drawer, route, breakpoint-specific copy branch, new
  scroll owner, or persistent mobile preference is introduced.
- **Shared behavior:** desktop and mobile consume the same ContextWindowEntry, translation keys,
  pending predicate, and tooltip dismissal behavior.
- **Mobile proof:** the mobile E2E scenario taps the ring, asserts the pending content inside the
  pinned tooltip, and checks that the document has no horizontal overflow.

## Tests

- **What:** the pending branch shows an unmeasured state for used === 0, while positive, exact
  full, and impossible reports retain existing behavior.
  - **File:** apps/web/components/task/chat/token-usage-display.test.tsx
  - **How:** render the component with the existing mocked session hook, open the pinnable
    tooltip, assert translated accessible/content values, and exercise the existing regression
    cases.
- **What:** the new translation keys are present in both catalogs and comply with i18n rules.
  - **Files:** apps/web/src/locales/en/task.json, apps/web/src/locales/pseudo/task.json
  - **How:** run the focused web i18n check and ratchet.
- **Integration boundary:** no backend integration test is needed because no transport or
  persistence behavior changes. The managed desktop and mobile browser tests below exercise the
  rendered store-to-tooltip path.

## E2E Tests

- **Scenario:** **GIVEN** a seeded session whose reliable context window has used: 0, **WHEN**
  a desktop user opens the context control, **THEN** the trigger announces pending usage and the
  tooltip shows em-dash usage values, the known window size, the source, and no numeric 0%/0 of
  usage.
  - **Files:** apps/web/e2e/tests/chat/context-window-source-helpers.ts,
    apps/web/e2e/tests/chat/context-window-source.spec.ts
  - **What to verify:** pending content and the existing positive-sample scenario.
- **Scenario:** **GIVEN** the same pending session on the mobile-chrome project, **WHEN** the user
  taps the context ring, **THEN** the pending content remains visible in the tap-pinned tooltip
  and the page has no horizontal overflow.
  - **File:** apps/web/e2e/tests/chat/mobile-context-window-source.spec.ts
  - **What to verify:** touch entry, pending content, and document containment.

## Verification Results

Implemented on `feature/investigate-deepseek-882` (2026-08-07):

- Focused Vitest `components/task/chat/token-usage-display.test.tsx` — 12 tests passed (11 retained
  + 1 new pending-state case).
- `pnpm --filter @kandev/web run typecheck` — passed.
- `pnpm --filter @kandev/web run i18n:check` — keys OK, pseudo in sync; `i18n:ratchet` — clean.
- Managed desktop E2E `pnpm e2e:run tests/chat/context-window-source.spec.ts` — 2 passed (pending +
  positive-sample regression).
- Managed mobile-chrome E2E `pnpm e2e:run --project mobile-chrome
  tests/chat/mobile-context-window-source.spec.ts` — 2 passed (pending + positive-sample touch
  scenarios, overflow check green).

## Implementation Waves And Parallel Candidates

Execution is sequential in the primary conversation because the browser scenarios depend on the
component's pending-state contract and translation keys.

- [x] [Task 01: Render the unmeasured context state](task-01-render-unmeasured-context-state.md) —
  done
- [x] [Task 02: Cover pending state on desktop and mobile](task-02-context-window-unmeasured-e2e.md) —
  done

## Open Questions

None. The design intentionally uses the existing reliable used === 0 state and does not expand
the ACP/session metadata contract.

## Risks and boundaries

- A provider could theoretically report a legitimate zero-token sample. The chosen product
  behavior still prioritizes avoiding a false "measured zero" claim; the first positive sample
  transitions the display to numeric usage.
- The pending state must not accidentally suppress source or compaction information, or alter the
  parent tooltip's touch/keyboard dismissal behavior.
- The new em-dash and pending copy must be translated so pseudo-locale rendering remains a valid
  completeness check.

