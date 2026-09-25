---
id: "01-stabilize-invalidation"
title: "Stabilize preview invalidation"
status: complete
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-WORKFLOW-MOVE-PREVIEW-001
  - REQ-TASKS-WORKFLOW-MOVE-PREVIEW-002
acceptance_criteria:
  - AC-TASKS-WORKFLOW-MOVE-PREVIEW-001.7
  - AC-TASKS-WORKFLOW-MOVE-PREVIEW-001.8
  - AC-TASKS-WORKFLOW-MOVE-PREVIEW-002.1
  - AC-TASKS-WORKFLOW-MOVE-PREVIEW-002.2
  - AC-TASKS-WORKFLOW-MOVE-PREVIEW-002.3
  - AC-TASKS-WORKFLOW-MOVE-PREVIEW-002.4
system_design:
  - ../../specs/tasks/system-design/workflow-move-preview.md
---

# Task 01: Stabilize preview invalidation

## Summary

Replace broad record invalidation with prediction-specific revisions. Prove
that harmless updates preserve visible success while real changes still refresh.

## In scope

- Typed revision projections, semantic candidate ordering, and map normalization.
- All regression cases in the [plan test matrix](plan.md#tests).
- Desktop and existing touch-drawer browser evidence, preserving move scenarios.
- Update this work order and plan with actual verification results.

## Out of scope

Backend policy/API changes, persistent caching, stale-result rendering, new copy,
layout changes, production instance mutation, and new navigation surfaces.

## Acceptance

1. The composed hook regression fails before the fix because a harmless update
   clears success or issues another request. After the fix it retains success
   and the request count across repeated updates in every existing store source.
2. Positive regressions prove relevant changes still invalidate, including
   candidate-order changes and mixed eligible/terminal/completion-follow-up
   inventories. Preserve cancellation, retry, queue, and generation guards.
3. Desktop and touch E2E keep the disclosure and selected details stable, then
   prove a real model update refreshes. Existing move, focus, target-size, and
   overflow assertions pass against freshly built assets.

## Implementation sequence

Use `/tdd`. First add the negative revision test and the composed request test
named in the plan; record the expected behavioral failures. Add positive field
and timestamp-order cases before removing broad inputs. Implement explicit
projections in the existing revision module, extracting a small adjacent helper
only if source-size limits require it. Follow the design's metadata and ordering
rules; use backend selectors as read-only references, not targets for edits.

Extend existing browser suites. The unit regression is the RED gate if the
browser fixture cannot reliably hold the pre-fix lifecycle; record that reason.
Run all commands below and record actual counts. Inspect desktop and touch
screenshots against UI-01/UI-02 without changing existing composition.

## ASCII UI preview

UI-01 and UI-02 excerpts; [full views and states](plan.md#ascii-ui-preview).
AC-TASKS-WORKFLOW-MOVE-PREVIEW-001.8 and -002.1 through .3.

```text
Desktop hover                 Touch drawer
+-------------------------+   +------------------------------+
|        Move here        |   | Move to                   x  |
|         Options         |   | Implement                    |
|   [step capabilities]   |   | [Options]       [Move here]  |
|-------------------------|   | Reuse current session        |
|  Reuse current session  |   | Astra                        |
|       Astra (i)         |   | [details stay open]          |
+-------------------------+   +------------------------------+
Harmless update: both previews retain their current result.
Real input change: Checking session... -> fresh result or Retry.
```

Retain the existing drawer scroll owner, safe-area clearance, 44px touch targets,
and row alignment. Text is illustrative data or existing localized copy.

## Verification

Run from repository root. Install dependencies once if this worktree has no
workspace install. Run browser commands sequentially; managed runners rebuild.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run hooks/domains/kanban/use-workflow-move-preview-revision.test.ts hooks/domains/kanban/use-workflow-move-preview.test.ts components/task/workflow-move-preview.test.tsx components/task/workflow-stepper.test.tsx components/task/workflow-move-proceed-button.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec eslint hooks/domains/kanban/use-workflow-move-preview-revision.ts hooks/domains/kanban/use-workflow-move-preview-revision.test.ts hooks/domains/kanban/use-workflow-move-preview.ts hooks/domains/kanban/use-workflow-move-preview.test.ts)
(cd apps/web && pnpm e2e:run --project chromium tests/workflow/workflow-move-preview.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/workflow/mobile-workflow-move-preview.spec.ts)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
```

Add any extracted helper and changed suite to the lint/test commands before
completion. No new user-facing text is planned; if needed, localize it and run
the repository i18n checks. Record request counts, visible status, and screenshot
comparison. No live instance may be used for browser mutations.

## Files likely touched

- `apps/web/hooks/domains/kanban/use-workflow-move-preview-revision.ts`
- `apps/web/hooks/domains/kanban/use-workflow-move-preview-revision.test.ts`
- `apps/web/hooks/domains/kanban/use-workflow-move-preview.test.ts`
- `apps/web/hooks/domains/kanban/use-workflow-move-preview.ts` only if composed evidence requires it
- `apps/web/e2e/tests/workflow/workflow-move-preview.spec.ts`
- `apps/web/e2e/tests/workflow/mobile-workflow-move-preview.spec.ts`
- An adjacent shared E2E helper if both suites need the same stimulus/capture code
- This plan and work order for results

## Dependencies

None. Existing preview endpoint and consumers are already implemented.

## Risks

See [plan risks](plan.md#risks). Do not remove timestamp sensitivity without
proving candidate-order changes remain observable. Do not silence genuine
invalidations with a longer debounce or a persistent cache.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/tasks/requirements/workflow-move-preview.md).
- [Design](../../specs/tasks/system-design/workflow-move-preview.md#semantic-revision-inputs).
- Existing revision and request-hook suites, renderer/stepper/proceed-button suites.
- `workflow_move_preview.go`, `workflow_move_preview_settings.go`, and
  `workflow_session_target.go` under `apps/backend/internal/orchestrator/`.
- Existing E2E specs and `apps/web/e2e/helpers/session-store.ts` for store access.
- `/tdd`, `/mobile-parity`, `/e2e`, and scoped frontend guidance.

## Results

Completed on 2026-09-18.

- TDD RED was confirmed for the new harmless-update and composed-hook cases.
- GREEN focused Vitest passed 74 tests across the revision, request-hook,
  renderer, stepper, and proceed-button suites.
- The revision now projects prediction inputs, normalizes configuration maps,
  and encodes current/original session identity plus reusable candidate order.
  Task descriptions, status-summary bookkeeping, read cursors, command counts,
  profile version churn, and timestamp changes that preserve candidate order do
  not invalidate the preview. Routing, model, options, connection, context,
  eligibility, and candidate-order changes still invalidate it.
- Touch disclosure hover and focus handlers are disabled while the existing
  coarse-pointer drawer is active. This keeps the drawer and its details open
  through harmless store rerenders while preserving explicit tap and Escape
  behavior. Fine-pointer hover and focus behavior is unchanged.
- Desktop Chromium workflow move-preview E2E passed 3 tests. Mobile-chrome
  workflow move-preview E2E passed 1 test. Each stability flow observed one
  initial request, no request or loading state across three harmless updates,
  then a second request and visible loading for a model change before the new
  model appeared.
- Typecheck, changed-file ESLint, `pnpm run build:e2e`, `git diff --check`,
  `python3 scripts/list-docs.py validate`, `python3 scripts/lint-spec-files.test.py`
  (36 tests), and `python3 scripts/lint-spec-files.py --all` passed.
- Review remediation: task-session and indexed-session projections now calculate
  current, original, and candidate-order dependencies per source. A duplicate
  logical session is not counted twice, while timestamp changes in either source
  remain observable when they change a real candidate order. The regression
  covers separate timestamp updates, convergence, marked-original provenance,
  and a genuine two-candidate reorder.
- Review remediation: profile revisions now include full settings-profile mode
  and config options, with the normalized profile `updatedAt` as a scoped
  fallback. The regression dispatches the real `agent.profile.updated` handler
  for a mode/config-options-only edit, then proves an unrelated profile event
  and global version bump do not invalidate the referenced preview.
- Review remediation: an initial-target profile from the durable
  `workflow_initial_session` snapshot remains a revision dependency after the
  historical session is deleted. The regression proves the missing-session
  state and a real profile event invalidate the open preview.
- Review remediation: desktop and touch negative assertions use the shared
  `dwell(..., "negative-assertion", ...)` observer window instead of treating a
  request timeout as proof that no late refresh occurred.
