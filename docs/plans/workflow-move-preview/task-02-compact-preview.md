---
id: "02-compact-preview"
title: "Compact preview and move verification"
status: completed
wave: 2
depends_on:
  - "01-move-decision-api"
plan: "plan.md"
requirements:
  - REQ-TASKS-WORKFLOW-MOVE-PREVIEW-001
  - REQ-TASKS-WORKFLOW-MOVE-PREVIEW-002
acceptance_criteria:
  - AC-TASKS-WORKFLOW-MOVE-PREVIEW-001.1
  - AC-TASKS-WORKFLOW-MOVE-PREVIEW-001.2
  - AC-TASKS-WORKFLOW-MOVE-PREVIEW-001.3
  - AC-TASKS-WORKFLOW-MOVE-PREVIEW-001.4
  - AC-TASKS-WORKFLOW-MOVE-PREVIEW-001.5
  - AC-TASKS-WORKFLOW-MOVE-PREVIEW-001.6
  - AC-TASKS-WORKFLOW-MOVE-PREVIEW-001.7
  - AC-TASKS-WORKFLOW-MOVE-PREVIEW-002.1
  - AC-TASKS-WORKFLOW-MOVE-PREVIEW-002.2
  - AC-TASKS-WORKFLOW-MOVE-PREVIEW-002.3
  - AC-TASKS-WORKFLOW-MOVE-PREVIEW-002.4
system_design:
  - ../../specs/tasks/system-design/workflow-move-preview.md
---

# Task 02: Compact preview and move verification

## Summary

Render the shared prediction in the step hover and mobile drawer. Verify the
visible prediction against actual moves using isolated backend fixtures.

## In scope

Domain client/hook, scoped invalidation, late-response guards, two-line renderer,
inline details, same draft for preview/move, all locales, focused component and
browser coverage, and public workflow guidance.

## Out of scope

Workflow editing, new routing rules, board drag/bulk-move previews, mandatory
confirmation, or direct provider probing.

## Acceptance

1. UI-01 and UI-03 remain compact, keyboard accessible, localized, and accurate
   through overrides, model changes, option changes, unknowns, and transport races.
2. UI-02 exposes equivalent predictions and inline details with 44px touch actions,
   one scroll owner, focus return, and viewport containment.
3. Browser tests verify preview versus actual recipient/model, including new and
   reused sessions. Documentation explains predictions and execution-time checks.

## ASCII UI preview

UI-01 desktop and UI-02 phone excerpts; see [full previews](plan.md#ascii-ui-preview)
for UI-03 loading/error, all variants, structural notes, and criterion mappings.

```text
Desktop                       Phone drawer
+---------------------------+ +------------------------------+
|        Move here          | | Move to                   x  |
|         Options           | |------------------------------|
|    [step capabilities]    | | Implement                    |
|---------------------------| | [Options]      [Move here]   |
|   Reuse current session   | |------------------------------|
|    Astra -> Luna +1 (i)    | |    Reuse current session     |
+---------------------------+ |     Astra -> Luna +1 (i)     |
                              +------------------------------+
```

The approved footer also appears below the Move action in the next-step
options popover and drawer above chat. It uses that action's destination and draft.

Collapsed summaries stay two centered lines. Details expand inline; phone rows share
one internal scroll region. The +1 denotes a non-model setting change here.
AC-TASKS-WORKFLOW-MOVE-PREVIEW-002.1 through .4 apply.

## Verification

Commands run from repository root. Bootstrap this worktree before the first
pnpm check. Managed E2E commands build fresh artifacts and run sequentially.
The new test files below are implementation outputs, not existing tests.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm test hooks/domains/kanban/use-workflow-move-preview.test.ts components/task/workflow-move-preview.test.tsx components/task/workflow-stepper.test.tsx)
(cd apps/web && pnpm run typecheck && pnpm run lint && pnpm run i18n:check && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run tests/workflow/workflow-move-preview.spec.ts tests/workflow/workflow-session-targeting.spec.ts tests/workflow/workflow-step-move-overrides.spec.ts -- --project=chromium)
(cd apps/web && pnpm e2e:run tests/workflow/mobile-workflow-move-preview.spec.ts tests/workflow/mobile-workflow-session-targeting.spec.ts tests/workflow/mobile-workflow-step-move-overrides.spec.ts -- --project=mobile-chrome)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
```

Inspect rendered desktop and phone screenshots from these tests against UI-01
through UI-03. Record actual commands/results and any deviation. Add any other
changed unit suite to the targeted command before completion.

## Files likely touched

- `apps/web/lib/api/domains/kanban-api.ts`.
- `apps/web/hooks/domains/kanban/use-workflow-move-preview.ts`, its tests, and
  `use-workflow-move-preview-revision.ts` (new).
- `apps/web/components/task/workflow-move-preview.tsx` and `.test.tsx` (new).
- `apps/web/components/task/workflow-stepper.tsx` and `.test.tsx`.
- `apps/web/components/task/workflow-step-disclosure.tsx` and disclosure actions.
- `apps/web/src/locales/` for en, pt-pt, zh-cn, zh-hk, zh-tw; generate Traditional Chinese with i18n:zh-hant.
- `apps/web/e2e/tests/workflow/workflow-move-preview.spec.ts` and `mobile-workflow-move-preview.spec.ts` (new).
- `docs/public/tasks-and-workflows.md` and paired feature documents/work-order results.

## Dependencies

Task 01 and its typed response. Reuse existing stepper, move options, session
targeting, and mobile drawer tests; do not simulate all routing in frontend mocks.

## Risks

Disclosure remounts, pinned viewed tabs, WS updates, and draft changes can race.
Preserve move-pending ownership and never render a response from another scope.

## Parallelism

`sequential`

## Inputs

[Design](../../specs/tasks/system-design/workflow-move-preview.md): Presentation,
Mobile, Freshness and failures. Use mobile-parity, TDD, E2E, and docs-maintainer
skills during implementation. No implementation delegation is authorized.

## Results

Implemented the shared preview client, debounced request hook, compact renderer,
desktop disclosure integration, and coarse-pointer drawer integration. The same
normalized options draft feeds the preview and the eventual move. All five web
locales include the new copy, and public workflow guidance describes the
advisory prediction and execution-time recheck.

Verification passed:

- `(cd apps && pnpm install --frozen-lockfile)`
- Focused Vitest: 5 files, 44 tests passed.
- `pnpm run typecheck`, `pnpm run lint`, `pnpm run i18n:check`, and `pnpm run i18n:ratchet`.
- `make -C apps/backend build` and `pnpm run build:e2e`.
- Desktop workflow E2E: 12 tests passed, including recipient reuse, fresh
  sessions, retained model overrides, targeting, and move options.
- Mobile workflow E2E: 5 tests passed, including the preview drawer, session
  targeting, and move-option regressions. The preview scenario uses the
  repository's coarse-pointer tablet context because the Pixel 5 full-page task
  route intentionally uses `SessionMobileLayout`, which has no desktop task
  stepper. The exercised `CompactWorkflowStepDisclosure` branch is the shared
  touch drawer used by the task preview and tablet task surfaces.
- `python3 scripts/list-docs.py validate`
- `python3 scripts/lint-spec-files.test.py`
- `python3 scripts/lint-spec-files.py --all`
- `node --test scripts/validate-public-docs.test.mjs`
- `node scripts/validate-public-docs.mjs`
- `git diff --check`

The managed build reports existing Darwin signing, deprecated Vite chunk
configuration, and large-chunk warnings; all build and browser commands exit
successfully.

## Review remediation results

The request hook now receives a store-derived revision containing relevant task,
source and destination step, session, workflow, profile, workspace, and
connection state, including per-session model and fallback data. It clears a
successful result before refetching on live updates and retains cancellation
and generation guards. Drawer rows all enqueue their previews; the shared queue
limits active requests to two and allows later rows to resolve after earlier
requests finish.

The renderer maps known notice, field, and built-in value codes to localized
copy, including a dedicated no-session dispatch state, while retaining a
generic fallback for unknown diagnostics and safe data labels for provider
options. A non-English DTO-shape test covers missing snapshot, ambiguous rule,
and inapplicable-session notices.

Remediation verification passed on 2026-09-15:

- Focused Vitest: 3 files, 13 tests passed.
- `cd apps/web && pnpm run typecheck && pnpm run lint && pnpm run i18n:check && pnpm run i18n:ratchet`

Final post-remediation verification passed on 2026-09-15:

- Focused Vitest: 6 files, 50 tests passed.
- `pnpm run build:e2e`, `pnpm run typecheck`, `pnpm run lint`,
  `pnpm run i18n:check`, and `pnpm run i18n:ratchet`.
- Desktop workflow move-preview E2E: 2 tests passed.
- Mobile workflow move-preview E2E: 1 test passed.
