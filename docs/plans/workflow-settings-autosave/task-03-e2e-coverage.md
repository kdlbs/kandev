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
cd apps/web && pnpm e2e:run tests/workflow/workflow-settings.spec.ts tests/workflow/mobile-workflow-settings.spec.ts
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
