---
spec: docs/specs/ui/requirements/acp-model-configuration-summary.md
created: 2026-07-29
status: complete
---

# Implementation Plan: Model-only Agent Tab Title

## Overview

PR [#2021](https://github.com/kdlbs/kandev/pull/2021) made unnamed agent tabs follow the
authoritative live model across model switches and task-detail reloads. Its title resolver also
appends every non-model ACP config value, producing labels such as
`GPT-5.6-Terra / Default / High / Off`. Narrow the derived tab title to the model display name while
preserving the PR's live-state precedence, reload behavior, and custom-name override.

## Frontend

### Agent tab title resolver

- Update `apps/web/components/task/session-tab-title.ts` so `resolveModelTitle` resolves only the
  provider-supplied display name for the chosen model.
- Continue using the live model config option to identify and display the authoritative current
  model before the active model, agent-profile fallback, and start-time snapshot fallbacks.
- Keep user-supplied session names as the highest-precedence title.
- Do not change `ModelConfigSelector`, the task-chat compact changed-value summary, tooltips, or
  agent-profile selector labels.

### Mobile design contract

`SessionTab` is mounted only by `DockviewDesktopLayout`; `TaskLayout` routes phone and
coarse-pointer tablet viewports to `SessionMobileLayout` and `SessionTabletLayout`. This copy-only
change therefore has no mobile tab surface, navigation, touch, scroll, or geometry impact. The
nearest mobile exemplar is the existing task mobile model selector, whose option access and compact
summary remain unchanged. No new mobile Playwright case is required; the focused desktop rendered
check proves the only affected surface.

## Tests

- **What:** A derived tab title contains the model display name and excludes non-model ACP config
  values while preserving custom-name and fallback precedence.
  **File:** `apps/web/components/task/session-tab-title.test.ts`.
  **How:** Vitest table/fixture coverage around `resolveSessionTabTitle`, including live model,
  mode, and effort options.

## E2E Tests

- **Scenario:** GIVEN an unnamed agent tab with provider mode and effort options, WHEN the user
  switches to `Mock Smart`, THEN the tab text is exactly `Mock Smart`; WHEN the page reloads, THEN
  the tab still contains only `Mock Smart`.
  **File:** `apps/web/e2e/tests/chat/model-selector-error.spec.ts`.
  **What to verify:** The stable session-ID tab locator has the exact model-only label before and
  after reload, while the session model selector continues to show the selected model.

## Implementation

- [x] [Task 01: Render model-only agent tab titles](task-01-render-model-only-title.md)

## Open Questions

None.
