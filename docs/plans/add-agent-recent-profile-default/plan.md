---
created: 2026-08-30
status: complete
requirements:
  - REQ-AGENTS-PROFILE-RECENT-USE-001
  - REQ-AGENTS-PROFILE-RECENT-USE-002
  - REQ-AGENTS-PROFILE-RECENT-USE-003
system_design:
  - ../../specs/agents/system-design/profile-recent-use.md
legacy_specs: []
---

# Implementation Plan: Add Agent Recent Profile Default

## Overview

Make an ordinary Add Agent dialog select the newest compatible
`task_session` profile. Preserve handoff targets, manual choices, eligibility,
and context isolation. Implement the selection resolver first. Then prove the
complete desktop and mobile flows.

## Confirmed defect

The recent-use feature orders Add Agent options with `task_session` history.
The dialog still initializes its value from the current task session. The
shared combobox moves that selected value to the first row. As a result, the
current session masks the separate recent-use order and remains the launch
default.

The smallest reproduction is:

1. Start a task with profile A.
2. Start another task session with profile B.
3. Open Add Agent on a task whose current session uses profile A.
4. Observe that profile A is selected and first, although profile B is first in
   `task_session` history.

## Scope

### In scope

- Select the first compatible `task_session` profile for ordinary Add Agent
  and new-session dialogs.
- Treat the recent-use default as the selector-backed launch choice.
- Preserve handoff targets and later manual profile choices.
- Preserve current-session and source-order fallback behavior when history has
  no compatible profile.
- Add focused unit, desktop Playwright, and mobile Playwright evidence.

### Out of scope

- Changes to recency persistence, limits, API shapes, or WebSocket events.
- Changes to `task_create`, `quick_chat`, or `config_chat` defaults.
- Changes to workflow, automation, settings, or Office selectors.
- Dialog markup, copy, overlay type, touch targets, scrolling, or breakpoints.

## Technical approach

### Initial profile resolver

Extract a pure resolver beside `new-session-dialog.tsx`. The resolver receives
the compatible profiles, `task_session` IDs, the current session profile, and
an optional handoff target. It returns the selected profile and selection
source.

Use this priority: handoff target, compatible recent profile, compatible
current-session profile, then the first compatible source profile. Mark
handoff, recent-use, and manual selections as explicit. Keep legacy fallbacks
non-explicit.

Keep selection provenance in the dialog state. Compatibility or recency
updates must not replace a manual choice. If recent-use state is unavailable,
the dialog must not block. It uses the existing current-session then
source-order fallback.

### Launch semantics

Continue to use `profile_explicit` in the current session-launch request. Set
it to `true` for a recent-use default. This rule makes the launched profile
match the profile that the dialog shows, including on a profile-pinned workflow
step.

Do not change backend request shapes or workflow resolution. The existing
explicit-profile path already owns this precedence.

### Desktop and mobile behavior

Both viewports use `NewSessionDialog`, `AgentSelector`, and the same launch
handler. The presentation stays unchanged. Desktop entry uses the task header
Add Agent action. Mobile entry uses the sessions picker and its New Agent
action. Each flow must show the recent profile and launch it.

## Tests

- `AC-AGENTS-PROFILE-RECENT-USE-001.4`, `.6`, `.7`, and `.8`: add pure resolver
  tests in
  `apps/web/components/task/new-session-profile-selection.test.ts`.
- `AC-AGENTS-PROFILE-RECENT-USE-001.6` and `.8`: update
  `apps/web/components/task/new-session-dialog.test.tsx` and
  `new-session-form-actions.test.ts` for selection provenance and
  `profile_explicit` request behavior.
- `AC-AGENTS-PROFILE-RECENT-USE-002.1`: make sure that opening, cancelling, or
  manually changing the dialog does not record use before a successful launch.

The first RED test is
`selects a compatible task-session recent profile over the current session`.
It must fail because the current implementation selects the current session.

## E2E tests

- `AC-AGENTS-PROFILE-RECENT-USE-001.2`, `.4`, `.6`, and `.8`: extend
  `apps/web/e2e/tests/session/new-session-dialog.spec.ts`. Start profile B in
  one task-session context. Open Add Agent on a profile-A task. Assert that B is
  selected, first, and used by the created session.
- Add the same user outcome to
  `apps/web/e2e/tests/session/mobile-new-session-dialog.spec.ts`. Use the
  existing mobile sessions picker and touch interactions.
- Poll `listTaskSessions` for the effective `agent_profile_id`. UI labels are
  secondary evidence.

## Work orders

- [x] [Task 01: Resolve the recent Add Agent profile](task-01-resolve-recent-profile.md)
- [x] [Task 02: Prove desktop and mobile recency](task-02-prove-desktop-mobile-recency.md)

## Verification results

- `cd apps && pnpm --filter @kandev/web test -- --run components/task/new-session-profile-selection.test.ts components/task/new-session-dialog.test.tsx components/task/new-session-form-actions.test.ts lib/agent-profile-recent-use.test.ts` passed (34 tests).
- `cd apps/web && pnpm run typecheck` passed.
- `cd apps/web && pnpm e2e:run --project chromium tests/session/new-session-dialog.spec.ts -- --grep "uses task-session recency"` passed (1 test).
- `cd apps/web && pnpm e2e:run --project mobile-chrome tests/session/mobile-new-session-dialog.spec.ts -- --grep "uses task-session recency"` passed (1 test).
- Review remediation added explicit coverage for unavailable and empty recent-use state, invalid manual selections, and incompatible handoff fallback. The recency-specific E2E controls now use `NewSessionDialogPage` and assert one matching first option on desktop and mobile.

## Risks

- A recent-use default becomes an explicit profile choice. It can override a
  workflow-step profile that previously replaced the unchanged dialog value.
- Late store or compatibility updates can overwrite a manual choice without a
  selection-source guard.
- The selected-first combobox pass can hide an ordering regression unless the
  browser test asserts both the trigger and the option list.
- E2E setup must use two tasks or reactivate the original session. Otherwise,
  the newest session already uses the recent profile and masks the defect.
