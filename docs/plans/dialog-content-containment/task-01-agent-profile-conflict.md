---
id: "01-agent-profile-conflict"
title: "Contain agent conflict details"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/requirements/dialog-content-containment.md"
design: "../../specs/ui/system-design/dialog-content-containment.md"
---

# Task 01: Contain Agent Conflict Details

## Outcome

Long agent-profile conflict lists scroll inside the dialog while the title and
permitted deletion decisions remain visible on desktop and phone.

## In scope

- Add desktop and mobile-Chrome RED cases using the structured profile DELETE
  409 response with enough task references to exceed the viewport.
- Convert `AgentProfileDeleteConflictDialog` to the bounded three-row layout.
- Add stable dialog and conflict-body selectors.
- Keep phone footer actions full-width and at least 44 pixels high.

## Exclusions

- Do not change conflict parsing, grouping, force-delete rules, or requests.
- Do not add archive, delete, disable, reassignment, or bulk remediation.
- Do not change the simple confirmation or shared AlertDialog primitive.

## Applicable requirements

- `REQ-UI-DIALOG-CONTENT-CONTAINMENT-001`
- `AC-UI-DIALOG-CONTENT-CONTAINMENT-001.1` through
  `AC-UI-DIALOG-CONTENT-CONTAINMENT-001.9`

## Implementation acceptance

1. The long conflict response creates one body scroll range while the dialog,
   title, and footer stay inside desktop and phone viewports.
2. The final conflict is reachable; Cancel/Delete Anyway stay visible and
   hit-testable when allowed; hard blockers still show Cancel only.
3. Short content, action ordering, dismissal, confirmation, and requests remain
   unchanged.

## TDD sequence

1. Add stable selectors and long-conflict desktop/phone browser cases.
2. RED: record the current outer overflow or unreachable footer geometry.
3. GREEN: apply the dynamic cap, auto/minmax/auto grid, body overflow, and phone
   action sizing.
4. Run focused component, E2E, type, lint, i18n, and diff checks.

## Verification

```bash
(cd apps && pnpm --filter @kandev/web test -- components/settings/agent-profile-delete-dialog.test.tsx)
(cd apps/web && pnpm e2e:run --host -- tests/settings/agent-profile-delete.spec.ts --grep "keeps conflict actions visible with many tasks")
(cd apps/web && pnpm e2e:run --host --project mobile-chrome -- tests/settings/mobile-agent-profile-delete.spec.ts --grep "keeps conflict actions visible with many tasks")
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec eslint components/settings/agent-profile-delete-dialog.tsx components/settings/agent-profile-delete-dialog.test.tsx e2e/tests/settings/agent-profile-delete.spec.ts e2e/tests/settings/mobile-agent-profile-delete.spec.ts)
(cd apps/web && pnpm run i18n:ratchet)
git diff --check
```

## Files likely touched

- `apps/web/components/settings/agent-profile-delete-dialog.tsx`
- `apps/web/components/settings/agent-profile-delete-dialog.test.tsx`
- `apps/web/e2e/tests/settings/agent-profile-delete.spec.ts`
- `apps/web/e2e/tests/settings/mobile-agent-profile-delete.spec.ts`
- `docs/plans/dialog-content-containment/plan.md`
- `docs/plans/dialog-content-containment/task-01-agent-profile-conflict.md`

## Dependencies

None.

## Parallelism

`sequential`. This work order owns one responsive RED-GREEN cycle.

## Output contract

Record RED/GREEN geometry, focused results, files changed, cleanup, blockers,
and residual risks. Reconcile this work order and the plan with the final diff.

## Results

- RED: The new long structured 409 conflict cases exposed the unbounded dialog
  and missing stable body/footer seams on desktop and mobile.
- GREEN: The conflict details now use a capped AlertDialog with an
  auto/minmax/auto grid and one scrolling description body. Cancel and the
  permitted Delete Anyway action remain in the footer, while routing-tier hard
  blockers still omit Delete Anyway.
- Focused component coverage and the desktop/mobile conflict geometry cases
  passed. The final combined run included the existing deletion scenarios and
  the long-task case.
- No conflict parsing, force-delete request, action ordering, or dismissal
  behavior changed.
