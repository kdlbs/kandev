---
spec: docs/specs/ui/task-layout-profiles.md
created: 2026-08-03
status: completed
---

# Implementation Plan: Conditional PR Details Tab

## Overview

Restore review-driven PR Details visibility without removing PR Details from the reusable layout editor. The built-in Default and compact layouts will no longer contain the panel; the existing review synchronization hook will own a conditional, inactive panel only while the active task has a linked GitHub PR or GitLab MR. Explicitly configured/restored panels keep their placement and empty state, and existing saved layouts are not migrated.

The confirmed root cause is `f8c363f72` (`feat(layout): add layout-owned PR Details panel`): it added `pr-detail` to both built-in layouts and replaced the prior auto-show hooks with parameter-only synchronization. Diagnostic bundle `kandev-diagnostic-logs (5).zip` reproduced the regression: a task created with `use_worktree=false` and no linked PR had no saved environment layout, so the default builder inserted `pr-detail` immediately.

## Frontend

### Built-in layout definitions

- Update `apps/web/lib/state/layout-manager/presets.ts` so `defaultLayout()` and `compactLayout()` omit `pr-detail`; keep `pr-detail` in `REUSABLE_PANEL_IDS` and `PANEL_REGISTRY` so custom and built-in overrides can add it.
- Update the built-in Default description in `apps/web/lib/layout/layout-profiles.ts` to list only the panels actually present by default.
- Adjust preset/profile/merge tests that currently require PR Details in the code-defined defaults while retaining validation that canonical `pr-detail` is editable reusable content.

### Conditional review synchronization

- Extend `apps/web/components/task/dockview-review-panel-sync.ts` with a pure `resolveConditionalReviewPanelAction()` decision and an apply path that distinguishes a layout-owned canonical panel from one automatically added for review context.
- Mark automatically created panels with Dockview param `autoAddedForReview: true`, add them inactive beside the live Agent/session group with the existing center-group fallback, and update their provider/key through the current `resolveCanonicalReviewParams()` path.
- When review identity becomes empty, close only a marked conditional panel; clear identity and retain an unmarked layout-owned panel.
- Restore `wasPRPanelOffered()` / `markPRPanelOffered()` and the session-storage key `kandev.pr-panel-offered.<sessionId>` in `apps/web/lib/local-storage.ts`. Mark a review-bearing canonical panel as offered whether it came from the layout or conditional insertion, so closing it prevents immediate re-creation during the same session. Explicit PR/MR opening remains available.
- Preserve the double-animation-frame live-identity guard, restoration/maximize guards, GitHub-over-GitLab precedence, and the current rule that review synchronization never moves an existing panel.

No backend, API, persistence-schema, mobile task-layout, or feature-flag changes are required.

## Tests

- **Built-in omission:** `apps/web/lib/state/layout-manager/presets.test.ts` and `apps/web/lib/layout/layout-profiles.test.ts` assert Default/compact omit `pr-detail` while reusable-layout validation still accepts it.
- **Conditional lifecycle:** `apps/web/components/task/dockview-review-panel-sync.test.ts` covers add-on-linked-review, inactive Agent-group placement, live task/session identity, restoration/maximize suppression, provider/key updates, removal of `autoAddedForReview` panels, preservation of explicit panels, and dismissal suppression. `apps/web/lib/local-storage.test.ts` covers the restored session flag.
- **Preset transitions:** update only the affected expectations in `apps/web/lib/state/layout-manager/merger.test.ts`; existing task-specific PR Details panels remain valid content when switching presets.

## E2E Tests

- **Scenario:** GIVEN a fresh Default-layout task with no linked review, WHEN its desktop workbench opens, THEN no PR Details tab exists. Update `apps/web/e2e/tests/task/task-default-layout.spec.ts` and `apps/web/e2e/tests/pr/pr-detail-layout.spec.ts`.
- **Scenario:** GIVEN that task, WHEN a PR is linked, THEN an inactive PR Details tab appears beside Agent and renders the linked PR without changing the selected Agent tab. Cover in `apps/web/e2e/tests/pr/pr-detail-layout.spec.ts`.
- **Scenario:** GIVEN the auto-added tab is closed, WHEN PR synchronization repeats, THEN it stays closed for the session. Cover in `apps/web/e2e/tests/pr/pr-detail-layout.spec.ts`.
- **Scenario:** GIVEN the user edits Default on desktop or mobile, WHEN they add PR Details and save, THEN fresh/reset tasks retain the configured panel and its empty state without review data. Update `apps/web/e2e/tests/settings/layout-profiles.spec.ts` and `apps/web/e2e/tests/settings/mobile-layout-profiles.spec.ts`.

## Mobile design contract

- Desktop outcome: review-less tasks using code-defined Default/compact omit PR Details; linked reviews conditionally add it beside Agent; profiles may make it persistent in any supported group.
- Mobile task entry and composition: unchanged. Phone tasks continue to use the existing bottom-nav Review destination and dedicated `mobile-pr-review-panel` / `mobile-mr-review-panel`, covered by current mobile PR and GitLab parity specs.
- Mobile settings entry: `Settings > General > Layouts`, using the existing responsive Layout editor. `mobile-layout-profiles.spec.ts` will now add PR Details from the existing touch-accessible panel menu before moving/saving it.
- Nearest shipped exemplar: the existing mobile Layouts editor and `MobilePickerSheet`; no new surface, scroll owner, safe-area behavior, or touch control is introduced.
- Mobile verification: the updated settings E2E proves the panel can be added and arranged by touch without document-level horizontal overflow. Existing task-review mobile E2E remains the evidence for linked-review access.

## Public documentation

- Update the explanation section in `docs/public/sessions-and-review.md` to say Default omits PR Details, linked reviews add it conditionally, and Layouts can make it persistent or place it explicitly.
- Run both public-doc validators; no navigation or screenshot inventory change is required.

## Verification Results

Completed.

- Focused Vitest suite: passed, `86` tests.
- Web typecheck: passed.
- Web lint: passed with zero warnings.
- Chromium E2E: task default `2` passed, PR lifecycle `3` passed, Layouts settings `4` passed.
- Mobile Chromium E2E: Layouts settings `2` passed.
- Public docs validators: `58` tests passed; `41` published pages validated.
- `git diff --check`: passed.

## Implementation Waves And Parallel Candidates

Wave 1:

- [x] [Task 01 - Conditional panel behavior](task-01-conditional-panel-behavior.md)

Wave 2 (depends on Wave 1):

- [x] [Task 02 - E2E and public documentation](task-02-e2e-and-public-docs.md)

Execution is sequential in the primary conversation. No task is marked parallel-safe because Task 02 validates and documents Task 01's behavior.

## Risks

- Dockview persists panel params in environment layouts. The conditional marker must survive reloads without converting an explicitly configured panel into an auto-owned panel.
- Task and environment switches run through delayed Dockview reconciliation. The hook must re-read live task/session/workspace state before adding, removing, or updating a panel.
- Closing a conditional panel must suppress only automatic re-creation for that session; explicit add/open actions and layout-editor placement must continue to work.
- Existing saved layouts that already contain PR Details remain unchanged by design, so users may need Reset Layout or a manual edit to adopt the new built-in omission.

## Out of scope

- Migrating or rewriting saved user profiles and task-specific Dockview layouts.
- Changing PR/MR association, detection, polling, or backend storage.
- Changing mobile/tablet task review navigation or content.
- Removing PR Details from the reusable layout editor.
