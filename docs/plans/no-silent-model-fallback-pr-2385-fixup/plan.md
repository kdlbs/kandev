---
spec: docs/specs/agents/requirements/no-silent-model-fallback.md
created: 2026-08-09
status: done
---

# Implementation Plan: PR 2385 Fallback Follow-up

## Overview

The contributor's latest commit resolves four of the five earlier review items:
Office routing is workspace-owned again, mixed fallback PATCH requests persist,
profile-picker reasons are readable without hover, and `.coderabbit.yaml` is
gone. The remaining backend work prevents profile layers from retrying a model
decision that the start-model policy already handled. The frontend follow-up
groups the two fallback choices into one self-describing disclosure with an
intentional desktop and touch layout.

## Backend

### Preserve handled model-policy outcomes

In `apps/backend/internal/agent/runtime/lifecycle/session.go`, replace the
`modelSet` result with an outcome that independently records the effective
model, the model actually applied, and whether the policy handled model
selection. `InitializeAndPromptWithLayers` passes the handled state to
`applyProfileSessionLayers`, which records an applied model only when one was
actually applied and never makes a second `SetModel` call after auto-fallback
best-effort failure or `sessionmodel.IsMethodNotFound`.

The explicit fallback path retains its intentional ordered attempts when the
advertised list is unknown: a rejected start model may be followed once by the
configured fallback. The profile layer repeats neither call.

## Frontend

### Shared fallback-settings disclosure

Add a small domain presentation component in
`apps/web/components/settings/model-fallback-settings-shell.tsx`. It owns the
collapsed header, effective-mode summary, aggregate dirty decoration,
responsive option grid, and shared info-help interaction, while callers retain
their existing model-picker implementations.

Use the shell from both:

- `apps/web/components/settings/profile-model-fields.tsx`, the full agent
  profile settings page shown in the request.
- `apps/web/components/agent/cli-profile-fallback-fields.tsx`, the lighter
  inline profile editor that exposes the same persisted settings.

The section starts collapsed. Its header reports strict mode, automatic mode,
or the configured explicit fallback. Expanded content uses two equal columns at
`md` and one column below `md`. Both choices remain visible; when automatic
fallback is enabled, the explicit fallback switch and picker are disabled but
the configured value is preserved.

Each option keeps concise visible helper copy. Its info-icon button opens a
fine-pointer tooltip on hover/focus and, using `useTouchDrawer`, an inset bottom
drawer with the same localized detail on coarse pointers. The disclosure
trigger and coarse-pointer help buttons have at least a 44px active dimension.
The grid remains the page's normal vertical content and introduces no new
scroll owner.

### Localization

Add the disclosure title, mode summaries, disabled-state hint, accessible
labels, and detailed help copy to the `settings` catalogs for `en`, `pseudo`,
`pt-pt`, and `zh-cn`. Domain values such as model IDs remain untranslated.

## Tests

- **What:** Session initialization never retries a handled model-policy
  outcome in the profile layer.
  **File:** `apps/backend/internal/agent/runtime/lifecycle/session_test.go`.
  **How:** Extend the recording agentctl coverage for successful application,
  auto-fallback best-effort failure, method-not-supported, and the ordered
  explicit-fallback path; assert call lists and original-configuration model
  snapshots.
- **What:** The disclosure starts closed, summarizes its effective mode,
  expands accessibly, marks hidden dirty state, and disables rather than removes
  the explicit fallback controls while automatic fallback is active.
  **Files:**
  `apps/web/components/settings/model-fallback-settings-shell.test.tsx`,
  `apps/web/components/settings/profile-form-fields.test.tsx`, and
  `apps/web/components/agent/cli-profile-editor.test.tsx`.
  **How:** Render both callers with stateful form data, exercise the trigger and
  switches, and assert semantics, preserved values, disabled state, and desktop
  tooltip content.

## E2E Tests

- **Scenario:** Given an agent profile on a desktop viewport, when the settings
  page loads and the user expands Fallback settings, then the two option cards
  share a row, both explanations are available by pointer/keyboard, and the
  selected strategy can be saved.
  **File:** update
  `apps/web/e2e/tests/settings/no-silent-model-fallback.spec.ts`.
  **What to verify:** initial collapsed state, effective-mode summary,
  horizontal card alignment by bounding boxes, tooltip portal content, mutual
  exclusion, and persistence after reload.
- **Scenario:** Given the same profile on the Pixel 5 project, when the user
  taps Fallback settings and an info icon, then the options form one vertical
  flow and the localized explanation opens in a touch drawer.
  **File:** update
  `apps/web/e2e/tests/settings/mobile-no-silent-model-fallback.spec.ts`.
  **What to verify:** `.tap()` interaction, stacked card geometry, 44px help
  target, drawer content/dismissal, setting persistence, and no document-level
  horizontal overflow.

## Verification Results

Complete. Each task records its exact commands and outcomes in `## Results`.

## Implementation Waves And Parallel Candidates

Wave 1 (parallel candidates; user authorization required for delegation):

- [x] [task-01-preserve-model-policy-outcome](task-01-preserve-model-policy-outcome.md)
- [x] [task-02-build-fallback-settings-disclosure](task-02-build-fallback-settings-disclosure.md)

These tasks are parallel-safe because their Go lifecycle and React settings
files are disjoint and they share no schema, generated contract, lockfile, or
package configuration. This does not authorize subagents; execution remains in
the primary conversation unless the user explicitly requests delegation.

Wave 2:

- [x] [task-03-cover-responsive-fallback-settings](task-03-cover-responsive-fallback-settings.md)

## Risks And Boundaries

- The PR is external and may advance again. Before implementation, fetch and
  compare the remote PR head, preserving contributor changes.
- `handled` and `applied` are intentionally separate. Treating them as one flag
  either reintroduces the duplicate call or falsely records the configured
  model as active after best-effort failure/method-not-supported.
- Tooltips are supplementary. Visible helper text and the touch drawer remain
  required so the setting is understandable without hover.
- This follow-up does not change Office routing, database fields, API payloads,
  or the task-create profile-picker behavior fixed by the contributor.
- Public documentation is unaffected; this is a settings composition change
  and a lifecycle correctness repair within the existing feature contract.
