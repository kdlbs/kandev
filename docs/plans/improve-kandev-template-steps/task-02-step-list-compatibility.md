---
id: "02-step-list-compatibility"
title: "Normalize workflow step lists"
status: done
wave: 2
depends_on:
  - "01-template-provisioning"
plan: "plan.md"
requirements:
  - REQ-WORKSPACES-IMPROVE-KANDEV-001
acceptance_criteria:
  - AC-WORKSPACES-IMPROVE-KANDEV-001.1
  - AC-WORKSPACES-IMPROVE-KANDEV-001.11
  - AC-WORKSPACES-IMPROVE-KANDEV-001.13
system_design:
  - ../../specs/workspaces/system-design/improve-kandev.md
---

# Task 02: Normalize workflow step lists

## Summary

Return an array from the steps endpoint and tolerate historical null lists in
the client. Preserve the existing shared dialog and populated-step normalization.

## In scope

- In `Controller.ListStepsByWorkflow`, normalize a successful nil result to an
  empty slice before JSON serialization. Preserve real errors.
- In `workflow-api.ts`, map `(response.steps ?? [])` and return an array.
  Keep input nullability at the transport boundary.
- Add HTTP empty-array and client null/omitted/empty/populated/error tests.
- Run the existing desktop and phone Improve Kandev E2E suites.

## Out of scope

Template repair, new dialog markup or copy, locale keys, new browser fixtures,
and viewport-specific interactions. Task 01 owns bootstrap correctness.

## Acceptance

1. The successful empty HTTP response is `{"steps":[]}`. Populated responses,
   authorization denials, and storage errors retain their behavior.
2. The client accepts null, missing, and empty lists. Populated steps retain
   defaults and lifecycle normalization. Failed requests still reject.
3. Existing desktop/phone create and issue-only flows pass against fresh builds.
   A bootstrap error continues to block task submission.

## ASCII UI preview

`UI-01: Bootstrap result`, from the [plan](plan.md#ascii-ui-preview).
Criteria .1, .11, and .13 apply.

```text
Desktop footer / Phone Utilities
  Improve Kandev -> report kind -> title/description -> task actions
  Preparation error -> task submission remains blocked
```

The existing phone dialog supplies the mobile exemplar. Shared response
normalization changes no composition, scrolling, focus, or touch behavior.
Existing desktop/mobile rendered tests plus unit coverage prove the change.
There are no new labels or pixel requirements.

## Implementation sequence

1. Add `TestListWorkflowStepsEmptyArray` and the `listWorkflowSteps` fetch tests.
2. Run them before production edits and require failure on null encoding and `.map`.
3. Apply the two boundary normalizations.
4. Run the commands below. Record actual test results and browser discovery counts.

## Verification

Run from the repository root. Install dependencies once in a fresh worktree.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/workflow/controller ./internal/workflow/handlers -count=1)
mapfile -t repair_go_files < <(git ls-files --modified --others --exclude-standard -- 'apps/backend/*.go' 'apps/backend/**/*.go')
bash scripts/lint-go-changed "${repair_go_files[@]}"
(cd apps/web && pnpm exec vitest run lib/api/domains/workflow-api.test.ts components/improve-kandev-dialog.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run lint)
(cd apps/web && pnpm e2e:run --project chromium tests/improve-kandev.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/mobile-improve-kandev.spec.ts)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Run the managed browser commands sequentially. They rebuild current backend and
Vite assets with isolated data. Keep the runner's worker budget.
Also run the [documentation preflight](plan.md#documentation-preflight).

## Files likely touched

- `apps/backend/internal/workflow/controller/controller.go`
- `apps/backend/internal/workflow/handlers/steps_list_test.go` (new)
- `apps/web/lib/api/domains/workflow-api.ts`
- `apps/web/lib/api/domains/workflow-api.test.ts`

## Dependencies

Task 01. Empty-list normalization stops the crash but does not create missing steps.

## Risks

Apply the fallback only to successful responses. Do not hide database, permission,
or network errors as empty lists.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/workspaces/requirements/improve-kandev.md), criteria .1, .11, .13.
- [Design](../../specs/workspaces/system-design/improve-kandev.md#list-contract-and-shared-client).
- Existing API fetch mocks, `setupStepRouter`, and desktop/mobile E2E fixtures.
- Scoped frontend/backend guidance and `/mobile-parity` data-normalization exception.

## Results

The HTTP endpoint now returns an empty array for an empty workflow and continues
to report storage errors. The client accepts null, omitted, and empty lists,
retains populated-step normalization, and continues to reject failed requests.

The regression tests failed before the two boundary fixes and now pass. The
race-enabled controller and handler tests pass. Changed Go files pass
`bash scripts/lint-go-changed` with zero issues. The focused web suite passes
22 tests; typecheck and lint pass. Desktop Improve Kandev E2E passes 17 tests,
and phone E2E passes its one test. Both E2E runs completed their backend and
Vite builds. Documentation catalog validation, specification lint,
documentation coverage, and `git diff --check` pass.
