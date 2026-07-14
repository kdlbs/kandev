---
id: "03-e2e-coverage"
title: "Workflow autosave E2E coverage"
status: done
wave: 3
depends_on: ["01-autosave-state", "02-responsive-layout"]
plan: "plan.md"
spec: "../../specs/workflow-settings-autosave/spec.md"
---

# Task 03: Workflow Autosave E2E Coverage

## Acceptance

- Desktop E2E proves workflow creation, metadata edits, and step edits persist without Save.
- Mobile E2E proves required controls are fully within the viewport and the document has no horizontal overflow.
- Page-object helpers describe autosave status rather than manual saving.

## Verification

```bash
cd apps/web && pnpm e2e:run tests/workflow/workflow-settings.spec.ts
cd apps/web && pnpm e2e:run --project mobile-chrome --no-build tests/workflow/mobile-workflow-settings.spec.ts
```

## Files Likely Touched

- `apps/web/e2e/pages/workflow-settings-page.ts`
- `apps/web/e2e/tests/workflow/workflow-settings.spec.ts`
- `apps/web/e2e/tests/workflow/mobile-workflow-settings.spec.ts`

## Inputs

- All spec Scenarios and completed Tasks 01-02.
- Existing workflow fixture/API helpers.

## Output Contract

Report scenarios covered, exact E2E command and result, artifacts inspected, blockers, and update this task plus `plan.md` to done.

## Completion Report

- Scenarios: template/custom creation, workflow-name autosave, step add/edit autosave, workflow/step profile autosave, removal of manual Save, and mobile reachability with the step editor open.
- Results: the focused desktop autosave/creation runs and the full two-test mobile Chrome file passed locally; mutation-specific response waits prevent an existing `Saved` label from satisfying persistence assertions early.
- Artifacts: Playwright traces/screenshots were inspected during failure triage; no persistent PR asset was required.
- Blockers: PR CI later failed before test execution because the shared runtime image lacked the Playwright browser revision requested by the lockfile.
