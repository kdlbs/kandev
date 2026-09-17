---
id: "01-import-profile-selection"
title: "Implement workflow import profile selection"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-IMPORT-PROFILES-001
acceptance_criteria:
  - AC-TASKS-IMPORT-PROFILES-001.1
  - AC-TASKS-IMPORT-PROFILES-001.2
  - AC-TASKS-IMPORT-PROFILES-001.3
  - AC-TASKS-IMPORT-PROFILES-001.4
  - AC-TASKS-IMPORT-PROFILES-001.5
  - AC-TASKS-IMPORT-PROFILES-001.6
  - AC-TASKS-IMPORT-PROFILES-001.7
  - AC-TASKS-IMPORT-PROFILES-001.8
  - AC-TASKS-IMPORT-PROFILES-001.9
system_design:
  - ../../specs/tasks/system-design/workflow-import-profile-selection.md
---

# Task 01: Implement workflow import profile selection

## Summary

Deliver the manual import flow from preview through profile selection to persisted workflows.
Use TDD for the service, HTTP contract, and client state, then verify both browser compositions.

## In scope

- Implement the preview and explicit JSON import contracts in the linked design.
- Wire eligible profile lookup with caller context and exact-ID validation.
- Preserve all session targets and validate the selected batch before persistence.
- Implement shared import state and desktop/phone selection surfaces.
- Add translated loading, empty, conflict, retry, and success states.
- Update public import documentation with browser selection and legacy API distinctions.

## Out of scope

The exclusions in the [plan](plan.md#scope) apply.
Do not create profiles, change profile configuration, or alter sync behavior.

## Acceptance

1. The supplied Feature structure imports after explicit selection for Implement. PR still targets that step after reload.
2. Exact matches proceed automatically. Missing, stale, unauthorized, and malformed selections cannot cause partial validation writes or guessed replacements.
3. Desktop and phone users complete the flow, recover from errors, and retain selections through valid retries. Locale and focused regression checks pass.

## ASCII UI preview

These excerpts use the labels from the [full preview](plan.md#ascii-ui-preview).
They cover AC-TASKS-IMPORT-PROFILES-001.1, .3, .6, .8, and .9.

```text
UI-01 Desktop:
Choose agent profiles
Feature > Implement
Requested: Codex / gpt-5.6-luna / agent-full-access
[Choose a profile v]
[Back] [Cancel]                         [Import]

UI-02 Phone:                     UI-03 Phone picker:
< Back   Choose profiles   X     < Back     Implement
Feature > Implement              [Search profiles]
Requested agent/model/mode       [Profile / agent / model / mode]
[Choose a profile >]             [Another profile]
(scrolling step list)            (scrolling candidate list)
[Import] + safe-area padding
```

UI-03 replaces UI-02 within one drawer. Back preserves selections.
Headers and primary actions stay fixed. Each active view has one vertical scroll region.
Import stays disabled while a required selection is empty or submission is pending.
An empty catalog provides settings access and Retry. A profile conflict identifies the affected step inline.
Compare rendered desktop and phone views with these structural requirements during E2E verification.

## Verification

Run each command from the repository root. The managed E2E runner rebuilds the application.
Use the recorded exact suite names for the new unit tests.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/backend && go test ./internal/workflow/service ./internal/workflow/controller ./internal/workflow/handlers -count=1)
(cd apps/backend && go test ./internal/backendapp -run 'TestBuildAgentProfileMatcher|TestWorkflowImportProfileCatalog' -count=1)
(cd apps/web && pnpm exec vitest run app/settings/workspace/use-workflow-import.test.ts app/settings/workspace/workflow-import-profile-selection.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec eslint app/settings/workspace/workspace-workflows-client.tsx app/settings/workspace/workspace-workflows-dialogs.tsx app/settings/workspace/use-workflow-import.ts app/settings/workspace/workflow-import-profile-selection.tsx app/actions/workspaces.ts lib/types/http.ts)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm e2e:run --project chromium tests/workflow/workflow-import-export.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/workflow/mobile-workflow-import-export.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

The new backend wiring test must be named `TestWorkflowImportProfileCatalog` or use that prefix.
The new Vitest files and E2E file must exist before their commands run.
If implementation requires additional focused files, update the command list to cover them.
No broad QA or review gate is part of this task.

## Files likely touched

- `apps/backend/internal/workflow/service/service.go`
- New `apps/backend/internal/workflow/service/import_profile_selection.go` and corresponding test file
- `apps/backend/internal/workflow/controller/controller.go`
- `apps/backend/internal/workflow/handlers/handlers.go`
- New `apps/backend/internal/workflow/handlers/import_profile_selection.go` and corresponding test file
- `apps/backend/internal/backendapp/services.go` and `agent_profile_matcher_test.go`
- `apps/web/app/actions/workspaces.ts`
- `apps/web/lib/types/http.ts`
- `apps/web/app/settings/workspace/workspace-workflows-client.tsx`
- `apps/web/app/settings/workspace/workspace-workflows-dialogs.tsx`
- New `apps/web/app/settings/workspace/use-workflow-import.ts` and corresponding test file
- New `apps/web/app/settings/workspace/workflow-import-profile-selection.tsx` and corresponding test file
- `apps/web/src/locales/*/workflows.json`
- `apps/web/e2e/tests/workflow/workflow-import-export.spec.ts`
- New `apps/web/e2e/tests/workflow/mobile-workflow-import-export.spec.ts`
- `docs/public/workflow-import-export.md`
- This plan, work order, and paired design for final status and evidence

## Dependencies

None. Work is sequential within this vertical slice.

## Risks

The import helper is shared with sync. Preserve its current contract.
The current profile hook also applies runtime health and feature filters. It must not silently redefine server import eligibility.
Generic API errors can discard response details. Verify structured conflicts through the actual HTTP action.
Do not claim transaction atomicity for storage failures beyond the existing import contract.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/tasks/requirements/workflow-import-profile-selection.md)
- [System design](../../specs/tasks/system-design/workflow-import-profile-selection.md)
- `apps/backend/AGENTS.md` and `apps/web/AGENTS.md`
- Existing workflow import service tests and `e2e/tests/workflow/workflow-import-export.spec.ts`
- `components/kanban/mobile-menu-sheet.tsx` for viewport and scroll geometry
- `components/settings/workflow-session-target-surfaces.tsx` for profile rows
- `/tdd`, `/e2e`, `/mobile-parity`, and `/docs-maintainer`

## Results

Implemented the browser workflow import profile-resolution flow end to end.
The backend now previews eligible global profiles, validates explicit profile
bindings and revisions before persistence, and preserves direct profile IDs and
session targets. The web client now provides localized desktop and phone
selection surfaces, retry and conflict recovery, and generation-safe import
state. Public import documentation describes the preview, browser envelope,
profile conflicts, and raw YAML behavior.

Review fixups preserve a non-null empty profile catalog, invalidate canceled
previews and late file reads, keep final submission ownership through draft
edits, and keep the selection surface mounted during pending submission.
Storage failures compensate by deleting already-created steps and the new
workflow. Focused regressions cover empty-catalog HTTP serialization,
malformed binding validation, partial-step rollback, rendered desktop and
phone empty/loading states, deferred cancellation and reopen, late file-read
results, stable busy status, in-flight edit/repeated-submit protection, coarse
pointer tablet drawers, picker Escape navigation, and the 767 px touch-target
boundary.

Verification passed:

- Backend focused workflow, controller, handler, and profile-catalog tests,
  including the empty eligible-catalog HTTP array regression.
- Focused frontend Vitest suite: 15 tests passed, including rendered desktop
  and phone empty/loading states, the deferred final-submit dialog path,
  deferred cancellation and reopen, late file-read results, stable busy
  status, and in-flight edit/repeated-submit protection.
- Focused backend import suite passed, including malformed binding rejection,
  workflow-field rollback, and partial-step rollback.
- Frontend typecheck, ESLint, i18n checks, and E2E production build.
- Desktop Chromium workflow import E2E: 8 tests passed.
- Mobile Chromium workflow import E2E: 1 test passed, including picker Escape
  navigation, coarse-pointer drawer behavior, and the 767 px control boundary.
- Public-doc validation, specification validation and lint, and `git diff --check`.
