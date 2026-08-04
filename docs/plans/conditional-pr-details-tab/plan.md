---
spec: docs/specs/ui/task-layout-profiles.md
created: 2026-08-03
status: completed
---

# Implementation Plan: Conditional PR Details Tab

## Overview

Restore review-driven PR Details visibility without removing PR Details from the reusable layout editor. The built-in Default and compact layouts no longer contain the panel; the review synchronization hook owns an inactive runtime panel only while the active task has a linked GitHub PR or GitLab MR. A custom Default's canonical panel is retained as placement metadata but is hidden without a review, and existing saved layouts are not migrated.

The confirmed root cause is `f8c363f72` (`feat(layout): add layout-owned PR Details panel`): it added `pr-detail` to both built-in layouts and replaced the prior auto-show hooks with parameter-only synchronization. Diagnostic bundle `kandev-diagnostic-logs (5).zip` reproduced the regression: a task created with `use_worktree=false` and no linked PR had no saved environment layout, so the default builder inserted `pr-detail` immediately.

## Frontend

### Built-in layout definitions

- Update `apps/web/lib/state/layout-manager/presets.ts` so `defaultLayout()` and `compactLayout()` omit `pr-detail`; keep `pr-detail` in `REUSABLE_PANEL_IDS` and `PANEL_REGISTRY` so custom and built-in overrides can add it.
- Update the built-in Default description in `apps/web/lib/layout/layout-profiles.ts` to list only the panels actually present by default.
- Adjust preset/profile/merge tests that currently require PR Details in the code-defined defaults while retaining validation that canonical `pr-detail` is editable reusable content.

### Conditional review synchronization

- Extend `apps/web/components/task/dockview-review-panel-sync.ts` with a pure `resolveConditionalReviewPanelAction()` decision and a placement resolver for the user's custom Default layout.
- Add linked-review panels inactive at the custom Default's configured group and tab index when that live group exists; otherwise use the live Agent/session group with the existing center-group fallback. Update provider/key through the current `resolveCanonicalReviewParams()` path.
- When review identity becomes empty and review data has hydrated, close every canonical panel, including one materialized from an explicit saved layout. Preserve the saved layout itself as the future placement template.
- Restore `wasPRPanelOffered()` / `markPRPanelOffered()` and the session-storage key `kandev.pr-panel-offered.<sessionId>` in `apps/web/lib/local-storage.ts`. Mark a review-bearing canonical panel as offered whether it came from the layout or conditional insertion, so closing it prevents immediate re-creation during the same session. Explicit PR/MR opening remains available.
- Preserve the double-animation-frame live-identity guard, restoration/maximize guards, GitHub-over-GitLab precedence, and the current rule that review synchronization never moves an existing panel.

No backend, API, persistence-schema, mobile task-layout, or feature-flag changes are required.

## Tests

- **Built-in omission:** `apps/web/lib/state/layout-manager/presets.test.ts` and `apps/web/lib/layout/layout-profiles.test.ts` assert Default/compact omit `pr-detail` while reusable-layout validation still accepts it.
- **Conditional lifecycle:** `apps/web/components/task/dockview-review-panel-sync.test.ts` covers add-on-linked-review, inactive Agent-group placement, custom saved-group placement, live task/session identity, restoration/maximize suppression, provider/key updates, removal of both automatic and explicit panels without a review, and dismissal suppression. `apps/web/lib/local-storage.test.ts` covers the restored session flag.
- **Preset transitions:** update only the affected expectations in `apps/web/lib/state/layout-manager/merger.test.ts`; existing task-specific PR Details panels remain valid content when switching presets.

## E2E Tests

- **Scenario:** GIVEN a fresh Default-layout task with no linked review, WHEN its desktop workbench opens, THEN no PR Details tab exists. Update `apps/web/e2e/tests/task/task-default-layout.spec.ts` and `apps/web/e2e/tests/pr/pr-detail-layout.spec.ts`.
- **Scenario:** GIVEN that task, WHEN a PR is linked, THEN an inactive PR Details tab appears beside Agent and renders the linked PR without changing the selected Agent tab. Cover in `apps/web/e2e/tests/pr/pr-detail-layout.spec.ts`.
- **Scenario:** GIVEN the auto-added tab is closed, WHEN PR synchronization repeats, THEN it stays closed for the session. Cover in `apps/web/e2e/tests/pr/pr-detail-layout.spec.ts`.
- **Scenario:** GIVEN the user edits Default and moves PR Details, WHEN a fresh/reset task has no review, THEN the tab is hidden; WHEN a PR is linked, THEN the inactive tab appears in the saved group. Cover the runtime transition in `apps/web/e2e/tests/settings/layout-profiles.spec.ts`; retain mobile settings coverage for touch-accessible placement editing.

## Mobile design contract

- Desktop outcome: review-less tasks never show PR Details; linked reviews add it beside Agent for the built-in Default or in the group configured by a custom Default.
- Mobile task entry and composition: unchanged. Phone tasks continue to use the existing bottom-nav Review destination and dedicated `mobile-pr-review-panel` / `mobile-mr-review-panel`, covered by current mobile PR and GitLab parity specs.
- Mobile settings entry: `Settings > General > Layouts`, using the existing responsive Layout editor. `mobile-layout-profiles.spec.ts` will now add PR Details from the existing touch-accessible panel menu before moving/saving it.
- Nearest shipped exemplar: the existing mobile Layouts editor and `MobilePickerSheet`; no new surface, scroll owner, safe-area behavior, or touch control is introduced.
- Mobile verification: the settings E2E proves placement can be edited by touch without document-level horizontal overflow. Existing task-review mobile E2E remains the evidence that review content itself is already association-gated.

## Public documentation

- Update the explanation section in `docs/public/sessions-and-review.md` to say review association owns visibility while Layouts controls the linked tab's placement.
- Run both public-doc validators; no navigation or screenshot inventory change is required.

## Verification Results

Completed.

- Focused Vitest suite: passed, `101` tests; corrected review synchronizer file passed `20` tests after lint cleanup.
- Web typecheck: passed.
- Web lint: passed with zero warnings.
- Web i18n consistency checks: passed.
- Chromium E2E: task default `2` passed, PR lifecycle `3` passed, Layouts settings `4` passed; the clarified configured-placement transition was rerun independently and passed (`1` test).
- Mobile Chromium E2E: Layouts settings `2` passed.
- Public docs validators: `58` tests passed; `41` published pages validated.
- `git diff --check`: passed.
- PR review fixup: terminal GitHub sync failures settle the review loading state only after the
  six-retry budget is exhausted, and conditional panel synchronization requires complete options;
  focused hook and panel tests passed (`29` tests).

## Implementation Waves And Parallel Candidates

Wave 1:

- [x] [Task 01 - Conditional panel behavior](task-01-conditional-panel-behavior.md)

Wave 2 (depends on Wave 1):

- [x] [Task 02 - E2E and public documentation](task-02-e2e-and-public-docs.md)

Execution is sequential in the primary conversation. No task is marked parallel-safe because Task 02 validates and documents Task 01's behavior.

## Risks

- Dockview can materialize `pr-detail` from saved environment layouts before review hydration. Removal must wait until review data settles so a linked panel is not discarded during loading.
- Task and environment switches run through delayed Dockview reconciliation. The hook must re-read live task/session/workspace state before adding, removing, or updating a panel.
- Closing a conditional panel must suppress only automatic re-creation for that session; explicit add/open actions and layout-editor placement must continue to work.
- Existing saved layouts that already contain PR Details remain unchanged on disk, but their runtime tab is still hidden while no review is linked.

## Out of scope

- Migrating or rewriting saved user profiles and task-specific Dockview layouts.
- Changing PR/MR association, detection, polling, or backend storage.
- Changing mobile/tablet task review navigation or content.
- Removing PR Details from the reusable layout editor.
