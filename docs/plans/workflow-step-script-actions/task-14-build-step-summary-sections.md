---
id: 14-build-step-summary-sections
title: Build expandable step summary sections
status: done
wave: 11
depends_on:
  - 12-restore-inline-workflow-tabs
  - 13-prove-revised-inline-experience
plan: plan.md
requirements:
  - REQ-TASKS-WORKFLOW-STEP-SCRIPT-006
  - REQ-TASKS-WORKFLOW-STEP-SCRIPT-008
acceptance_criteria:
  - AC-TASKS-WORKFLOW-STEP-SCRIPT-006.1
  - AC-TASKS-WORKFLOW-STEP-SCRIPT-006.4
  - AC-TASKS-WORKFLOW-STEP-SCRIPT-006.5
  - AC-TASKS-WORKFLOW-STEP-SCRIPT-008.2
  - AC-TASKS-WORKFLOW-STEP-SCRIPT-008.3
  - AC-TASKS-WORKFLOW-STEP-SCRIPT-008.4
  - AC-TASKS-WORKFLOW-STEP-SCRIPT-008.6
  - AC-TASKS-WORKFLOW-STEP-SCRIPT-008.7
  - AC-TASKS-WORKFLOW-STEP-SCRIPT-008.8
  - AC-TASKS-WORKFLOW-STEP-SCRIPT-008.9
  - AC-TASKS-WORKFLOW-STEP-SCRIPT-008.10
system_design:
  - ../../specs/tasks/system-design/workflow-step-script-actions.md
---

# Task 14: Build expandable step summary sections

## Summary

Replace the step editor tabs with independent Agent, Instructions, Automation,
Board behavior, and Advanced sections. Show live draft summaries and edit
actions inline while retaining the familiar workflow card and step strip.

## In scope

- Rebase the existing PR branch on main before implementation.
- Preserve all existing step options, profile/session selection, restrictions,
  page save ownership, and event action serialization.
- Separate instructions from agent settings; retain prompt templates and
  autocomplete. Publish prompt edits immediately so collapse cannot lose edits.
- Show readable profile/session, prompt, action count/destination, board, and
  advanced summaries, with dirty markers based on values rather than labels.
- Keep event groups visible while editing an action. Show unused lifecycle
  groups collapsed, with touch-safe explicit reorder controls.
- Preserve session-option resolution while Advanced is collapsed.
- Translate new copy and update public documentation and design artifacts.

## Out of scope

- Runtime scripts, trigger behavior, workflow wire-format, or topology changes.
- New editor routes, canvas layout, or a separate phone workflow hierarchy.

## Acceptance

1. Several sections can stay open together; closing a section or selecting
   another step never discards draft edits.
2. Live summaries identify configured behavior without raw destination IDs.
   Changed commands mark Automation dirty even when the action count is stable.
3. Desktop and phone authors can edit, reorder, save, and reload actions.
   Phone row targets are at least 44px with no page overflow.

## ASCII UI preview

Excerpt from [Desktop: independent step sections](plan.md#desktop-independent-step-sections):

```text
| > Agent           Task default | Auto-start agent                      |
| > Instructions    Implement the requested change                       |
| v Automation      3 actions | On completion: Review                     |
|   v On task entry                                          2 actions   |
|     [1. Run script: pnpm install]                                      |
|       Command [pnpm install_______________________________________]    |
|       Timeout [300]  Failure [Block v]  Move up / down / Remove         |
|       Collapse                                                        |
|     [2. Auto-start agent]                                             |
|   v On turn complete                                       1 action    |
|     [1. Move to step: Review]                                         |
| > Board behavior  Manual moves | Command panel | No WIP limit           |
| > Advanced        Default settings                                     |
```

Excerpt from [Phone: inline sections and touch-safe controls](plan.md#phone-inline-sections-and-touch-safe-controls):

```text
| > Agent                           |
|   Task default | Auto-start agent  |
| v Automation                      |
|   3 actions | Completion: Review   |
|   v On task entry        2 actions |
|     [1. Run script]                |
|     Command                       |
|     [pnpm install______________]   |
|     [Up] [Down] [Remove]           |
|     [Collapse]                     |
|     [2. Auto-start agent]          |
| > Board behavior                  |
|   Manual moves | No WIP limit      |
```

These views cover AC-TASKS-WORKFLOW-STEP-SCRIPT-008.2 through .4, .7 and .8.
Section state is local; edit state remains owned by the workflow draft.

## Verification

Run commands from the repository root:

```bash
(cd apps/web && pnpm exec vitest run lib/workflows/workflow-step-section-summary.test.ts components/settings/workflow-editor components/settings/workflow-step-prompt-section.test.tsx lib/workflows/workflow-editor-view-model.test.ts components/settings/workflow-step-mutations.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec eslint components/settings/workflow-editor components/settings/workflow-pipeline-editor-panels.tsx components/settings/workflow-step-prompt-section.tsx lib/workflows/workflow-step-section-summary.ts e2e/pages/workflow-settings-page.ts e2e/tests/workflow)
(cd apps/web && pnpm run i18n:check && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --project chromium tests/workflow/workflow-editor.spec.ts tests/workflow/workflow-settings.spec.ts tests/workflow/workflow-step-autocomplete.spec.ts tests/workflow/workflow-cycle-guardrails.spec.ts tests/workflow/workflow-task-completion.spec.ts -- --retries=0)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/workflow/mobile-workflow-editor.spec.ts tests/workflow/mobile-workflow-settings.spec.ts tests/workflow/mobile-workflow-cycle-guardrails.spec.ts tests/workflow/mobile-workflow-task-completion.spec.ts tests/workflow/mobile-workflow-cancel-completion.spec.ts -- --retries=0)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check
```

Summary unit tests cover live/default values, missing profiles/destinations,
ordered transitions, and value-based dirty markers. Browser tests cover independent
expansion, inline editing, save/reload, step switching, profile/session options,
prompts, board settings, completion/cycle guards, and phone geometry.

## Files likely touched

- `apps/web/components/settings/workflow-editor/`
- `apps/web/components/settings/workflow-pipeline-editor-panels.tsx`
- `apps/web/components/settings/workflow-step-prompt-section.tsx`
- `apps/web/lib/workflows/workflow-step-section-summary.ts`
- `apps/web/e2e/pages/workflow-settings-page.ts`
- `apps/web/e2e/tests/workflow/` and workflow locale catalogs.
- Workflow requirements, system design, ADR, plan, and public guide.

## Dependencies

Tasks 12 and 13 provide the existing inline editor and runtime verification.

## Risks

- Hidden session-option resolution must still block an unsafe save.
- Unmounting a collapsed editor must not discard a debounced prompt.
- New summaries must not depend on stale saved values or leak internal IDs.

## Parallelism

`sequential`. The primary session owns implementation and delivery.

## Inputs

- User-selected summary-row design and explicit implementation/push instruction.
- REQ-TASKS-WORKFLOW-STEP-SCRIPT-006 and -008; inline editor system design.
- Existing workflow draft, action catalog, mobile controls, and save coordinator.

## Results

Rebased the existing PR branch onto main at
`48adb0ce73960c5b4fcba968367645a5dc680b26` without conflicts. Replaced tabs with
five independent summary sections and inline action editors. Preserved every
step control, page-local drafts, shared save, and synchronized inspection.
Advanced keeps capability-resolution ownership while collapsed. Prompt edits
publish immediately before collapse or step selection can unmount the editor.

TDD evidence: summary tests failed against the empty implementation; the dirty
marker regression failed with unchanged action counts; the desktop browser
scenario rejected the old tabbed layout before implementation.

Verification:

- Focused Vitest: 5 files, 17 tests passed.
- Final web typecheck, targeted ESLint with zero warnings, i18n check, and
  new-code i18n ratchet passed.
- Phone workflow E2E: all five listed specs passed, 13 tests with retries disabled.
  Covered template/action targets, independent expansion, immediate prompts,
  ordered scripts, save/reload, original-session options, cycle guards, and WIP.
- Desktop workflow E2E: all five listed specs passed, 31 tests with retries
  disabled against the same assets as the phone suite. The final pass measured
  28px desktop action controls; the phone pass measured at least 44px targets.
- Desktop and phone screenshots were inspected against the approved previews.
- Public documentation tests: 62 passed; 47 published pages validated.
- Documentation catalog, specification lint, local PR-documentation coverage
  preflight, and diff checks passed. Task 14 covers the revised delivery slice.

Implementation and task checks are complete. The primary session handoff records
the normal hook receipt and exact leased push to PR #3440.

### Conflict and CI remediation

Merged current main while preserving workflow script startup reconciliation,
attempt-scoped cancellation cleanup ownership, and agent event continuity fields.
Updated the cleanup regression for task-scoped callbacks. Workflow profile tests
now open the Agent and Advanced summary sections through the existing page helpers.
No rendered UI or public behavior changed in this remediation.

Local verification: orchestrator, executor, and watcher race tests passed;
changed-scope Go lint reported zero issues; web typecheck and changed-test ESLint
passed; five frontend test files passed all 18 tests. The four failed or flaky CI
browser cases passed three consecutive repetitions, 12 tests with retries disabled.
Phone editor and settings coverage passed nine tests with retries disabled.
Harness and specification validation passed. Exact pushed-head CI remains a
delivery check owned by the primary session, not a claim made by this work order.
