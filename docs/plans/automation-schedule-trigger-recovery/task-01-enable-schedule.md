---
id: "01-enable-schedule"
title: "Enable a disabled schedule"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-OFFICE-AUTOMATIONS-SETTINGS-001
acceptance_criteria:
  - AC-OFFICE-AUTOMATIONS-SETTINGS-001.22
  - AC-OFFICE-AUTOMATIONS-SETTINGS-001.26
  - AC-OFFICE-AUTOMATIONS-SETTINGS-001.28
  - AC-OFFICE-AUTOMATIONS-SETTINGS-001.29
  - AC-OFFICE-AUTOMATIONS-SETTINGS-001.30
system_design:
  - ../../specs/office/system-design/automations-settings-03.md
---

# Task 01. Enable a disabled schedule

## Summary

Add a recovery button beside the disabled schedule reason in the shared
Run detail disclosure.

## In scope

- Use `scheduleBinding` to select the trigger shown in Run detail.
- Call `updateTrigger(trigger.id, { enabled: true })` from the action.
  Pass the existing activity refresh through the Runs rail and drawer.
- Disable the action during the write and allow retry after an error.
- Localize the action in every supported locale. Add a sentence about the
  action and catch-up behavior to the public Schedule section.

## Out of scope

Automation editor changes, new API actions, and scheduler changes.

## Acceptance

- The action appears beside the disabled schedule reason in the desktop rail
  and phone drawer. The phone button has a touch target of at least 44 px.
- The action appears only in Activity Run detail, where a failed refresh cannot
  unmount a Configure editor draft.
- A late write from automation A cannot refresh or replace automation B after
  B has loaded.
- Activation sends one partial trigger update. Run detail displays the
  refreshed saved status. Reloading shows that the schedule is enabled.
- A failed update displays an error and leaves the action available for retry.

## ASCII UI preview

UI-01: Disabled schedule recovery in the existing Run detail disclosure.

```text
Schedule trigger is switched off
[Turn on schedule]
```

The action sits in the desktop rail or phone drawer. See the [full preview](plan.md#ascii-ui-preview)
for the entry points and acceptance criteria.

## Mobile design contract

- **Desktop outcome:** recover the configured schedule from Run detail in the Runs rail.
- **Phone entry and exemplar:** open the Runs drawer from its header control. Reuse the existing `RunsDrawer`; its inset bottom-drawer interaction follows the geometry and scroll pattern in `components/kanban/mobile-menu-sheet.tsx`.
- **Hierarchy and surface:** keep the disabled-schedule reason first and the single primary recovery action directly below it in the existing Run detail disclosure. Schedule recovery is an occasional one-step repair, so it stays in the temporary Runs drawer instead of adding a route or full-height surface.
- **Scroll and touch:** the drawer retains its existing content scroll owner. Run detail stays in the fixed drawer header area. The action has a 44px minimum hit target on phones and coarse pointers.
- **Shared behavior:** desktop and phone use the same trigger update and activity refresh callback; only the existing rail/drawer presentation changes.
- **Mobile proof:** open the Runs drawer, enable the schedule, verify refreshed and persisted status, and assert the action's rendered hit target and drawer containment.

## Verification

Start with `turns on a disabled schedule from Run detail` in
`automation-detail-page.test.tsx`. Make the first write fail, then retry.
Assert the trigger ID and `{ enabled: true }` payload. Assert the next status
after a successful read. Keep recovery unavailable on Configure and keep the
editor mounted. Defer an automation A write, load automation B, then resolve A
and assert B's loaded snapshot remains in place.

For the browser cases, seed an enabled automation and a disabled scheduled
trigger with a future occurrence. Activate the action from Run detail.
Assert the status updates live and after reload. Check the saved trigger ID,
cron, timezone, and enabled flag with `automation.get`. Check the button's
touch height with a 14px root font and drawer containment on the phone.

Run from the repository root. These commands passed for this implementation.
The phone check reuses the desktop E2E build.

```bash
pnpm --dir apps/web test components/runs/automation-detail-page.test.tsx # passed: 29 tests
pnpm --dir apps/web test components/runs/use-automation-activity.test.ts # passed: 9 tests
pnpm --dir apps/web run typecheck # passed
pnpm --dir apps/web run lint # passed
pnpm --dir apps/web run build # passed
pnpm --dir apps/web i18n:pseudo # passed: 37 namespaces, 13994 messages
pnpm --dir apps/web run i18n:check # passed
pnpm --dir apps/web e2e:run --host --shards 1 --project chromium tests/automations-run-detail.spec.ts -- --grep 'schedule trigger recovery' # passed: 1 test
pnpm --dir apps/web e2e:run --host --shards 1 --no-build --project mobile-chrome tests/mobile-automation-detail.spec.ts -- --grep 'schedule trigger recovery' # passed: 1 test; 44px minimum with 14px root font
(cd apps/backend && go test -trimpath ./internal/automation -run '^(TestScheduledTriggerUpdate_DisabledAndEmpty|TestCronScheduler_UnchangedScheduleCatchUp)$' -count=1) # passed
python3 scripts/list-docs.py validate # passed: 463 decisions and 2098 specifications
python3 scripts/lint-spec-files.py --all # passed
node --test scripts/validate-public-docs.test.mjs # passed: 63 tests
node scripts/validate-public-docs.mjs # passed: 56 published pages
pnpm --dir apps/web exec prettier --check components/runs/automation-detail-page.test.tsx components/runs/automation-detail-page.tsx components/runs/use-automation-activity.ts e2e/tests/automations-run-detail.spec.ts e2e/tests/mobile-automation-detail.spec.ts # passed
pnpm --dir apps/web exec prettier --check ../../docs/plans/automation-schedule-trigger-recovery/task-01-enable-schedule.md # passed
git diff --check # passed
```

## Files likely touched

- `apps/web/components/runs/run-detail-disclosure.tsx`, `runs-rail.tsx`,
  `runs-drawer.tsx`, and `automation-detail-page.tsx`
- `apps/web/components/runs/automation-detail-page.test.tsx`
- `apps/web/src/locales/*/automations.json`
- `apps/web/e2e/tests/automations-run-detail.spec.ts` and
  `mobile-automation-detail.spec.ts`
- `docs/public/automation-and-mcp.md`

## Dependencies

The existing trigger update action and automation activity refresh.

## Risks

The existing scheduler can admit a missed occurrence when a schedule resumes.

## Parallelism

`sequential`

## Inputs

- [Automation settings requirements](../../specs/office/requirements/automations-settings.md)
- [Schedule trigger recovery design](../../specs/office/system-design/automations-settings-03.md#schedule-trigger-recovery)

## Results

The page tests cover the recovery action, write-in-progress state, visible
failure and retry, refreshed saved status, a deferred A write after B loads,
the Activity-only action scope, and an unsaved Configure draft across a failed
post-enable refresh. The page suite passes (29 tests). The activity hook suite
passes (9 tests); its manual refresh test awaits the asynchronous `act` scope.

The targeted Go scheduler tests pass. Desktop E2E passes after the managed
runner builds production assets; mobile E2E passes against that build and
checks persistence, drawer containment, and a 44px action hit target.

Web typecheck, build, lint, and i18n checks pass. Desktop and mobile recovery
E2E pass; mobile measures at least 44px with a 14px root font. Public-doc tests
and validation, spec validation and lint, Prettier, and `git diff --check` pass.
