---
id: "01-restore-opener-focus"
title: "Restore plugin modal opener focus"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLUGINS-ACTION-UX-001
  - REQ-PLUGINS-ACTION-UX-003
acceptance_criteria:
  - AC-PLUGINS-ACTION-UX-001.1
  - AC-PLUGINS-ACTION-UX-001.5
  - AC-PLUGINS-ACTION-UX-003.6
  - AC-PLUGINS-ACTION-UX-003.7
  - AC-PLUGINS-ACTION-UX-003.8
system_design:
  - ../../specs/plugins/system-design/plugin-action-ux.md
---

# Task 01: Restore plugin modal opener focus

## Summary

Capture each modal's opener before the host publishes the instance.
Restore focus through the dialog and drawer close lifecycle without moving
focus behind a surviving overlay. Prove the reported desktop paths and phone equivalents.

## In scope

- Host-only opener metadata in the manager's shared `open` path, including native task-link instances.
- Both content primitives' `onCloseAutoFocus` handlers and callback-time eligibility checks.
- Regression cases for each normal closure route, unavailable openers, concurrent overlays, and owner cleanup.
- Packaged fixture launchers and desktop/mobile browser evidence listed in the plan.
- Internal contract consistency and the short public author focus-return note.

## Out of scope

- Shared UI primitive changes, new modal options, SDK changes, plugin repository releases, or navigation redesign.
- New stores, persistence, runtime flags, dependencies, or a replacement overlay state machine.
- Restoring removed controls by identity or changing drawer opening autofocus.

## Acceptance

1. The permanent Escape regression fails on the current source because focus reaches `BODY`, then passes with the correction.
2. Both presentations restore eligible openers and preserve remaining overlay focus in every component edge case named in the plan.
3. All three reported desktop surfaces and phone parent-surface flows pass packaged-browser checks with existing scenarios preserved.

## ASCII UI preview

### UI-01: Desktop Action, open and closed states

```text
[Details*] --Enter--> [Plugin details: content focus]
Escape -> closed -> [Details*] -> Tab continues
```

### UI-02: Phone parent surface and plugin modal

```text
App menu / Status drawer: [Details*]
  -> plugin dialog or requested drawer
  -> close -> [Details*] in still-open parent
```

`*` means keyboard focus. Geometry, grouping, and scroll owners remain as shipped.
See the [full preview](plan.md#ascii-ui-preview) and AC-PLUGINS-ACTION-UX-003.6 through .8.

## TDD sequence

1. Add `returns focus to the opening Action after Escape` to the real host component suite.
2. Run the targeted unit command and require the final opener-focus assertion to fail with `BODY`.
3. Add the remaining manager and component edge cases from the plan.
4. Add test-only packaged fixture launchers without replacing existing Action toggles.
5. Add browser regressions, record their expected focus failures, then implement the minimum host correction.
6. Run the commands below sequentially and record results in this work order and the plan.

## Verification

Run from the repository root. A fresh worktree requires
`(cd apps && pnpm install --frozen-lockfile)` once before these commands.
The managed runner rebuilds the frontend, backend, and packaged fixture.
Run desktop and mobile separately because the runner accepts one project.

```bash
(cd apps/web && pnpm exec vitest run lib/plugins/modal-manager.test.ts components/plugins/plugin-modal-host.test.tsx)
(cd apps/web && pnpm exec eslint lib/plugins/modal-manager.ts components/plugins/plugin-modal-host.tsx lib/plugins/modal-manager.test.ts components/plugins/plugin-modal-host.test.tsx e2e/tests/plugins/plugin-action-ux.spec.ts e2e/tests/plugins/mobile-plugin-action-ux.spec.ts --max-warnings 0)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --project chromium tests/plugins/plugin-action-ux.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/plugins/mobile-plugin-action-ux.spec.ts tests/plugins/mobile-plugin-modal.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
node <<'NODE'
const fs = require('node:fs');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const workOrder = 'docs/plans/plugin-modal-focus-restoration/task-01-restore-opener-focus.md';
const paths = [
  workOrder,
  'docs/plans/plugin-modal-focus-restoration/plan.md',
  'docs/specs/plugins/requirements/plugin-action-ux.md',
  'docs/specs/plugins/system-design/plugin-action-ux.md',
];
const result = validateCoverage({
  changedFiles: [
    { filename: workOrder, status: 'added' },
    { filename: 'apps/web/lib/plugins/modal-manager.ts', status: 'modified' },
    { filename: 'apps/web/components/plugins/plugin-modal-host.tsx', status: 'modified' },
  ],
  fileContents: Object.fromEntries(paths.map(p => [p, fs.readFileSync(p, 'utf8')])),
});
console.log(JSON.stringify(result, null, 2));
if (!result.ok || result.status !== 'covered') process.exitCode = 1;
NODE
```

Require nonzero test discovery and zero discovery errors for both browser projects.
The evaluator command checks prospective implementation coverage and all artifact references.
No broad verification suite or local review phase is required.

## Files likely touched

- `apps/web/lib/plugins/modal-manager.ts`
- `apps/web/lib/plugins/modal-manager.test.ts`
- `apps/web/components/plugins/plugin-modal-host.tsx`
- `apps/web/components/plugins/plugin-modal-host.test.tsx`
- `apps/web/e2e/fixtures/plugins/prompt-history-plugin/bundle.js`
- `apps/backend/cmd/plugin-fixture/fixture-package/ui/bundle.js`
- `apps/web/e2e/tests/plugins/plugin-action-ux.spec.ts`
- `apps/web/e2e/tests/plugins/mobile-plugin-action-ux.spec.ts`
- `docs/public/plugins-authoring.md`
- `docs/plans/plugins/PLUGIN-API.md`
- `docs/specs/plugins/requirements/plugin-action-ux.md`
- `docs/specs/plugins/system-design/plugin-action-ux.md`
- This work order and sibling `plan.md`.

## Dependencies

None. Read the existing Action package for compatibility context.

## Risks

- Deferred primitive cleanup can race a newer modal or owner bulk cleanup.
- Mobile parent traps must retain focus inside their active surface.
- Unload and responsive remount can remove the captured opener.

## Parallelism

`sequential`. No subagents are authorized.

## Inputs

- [Requirements](../../specs/plugins/requirements/plugin-action-ux.md), especially 001.1, 001.5, and 003.6 through .8.
- [System design](../../specs/plugins/system-design/plugin-action-ux.md#host-modal-focus-lifecycle).
- [Issue and diagnosis](plan.md#evidence-and-confirmed-cause).
- [Original Action plan](../plugin-action-ux/plan.md).
- Existing manager, component, fixture-installation, and mobile Action test patterns.
- Accepted [additive Action decision](../../decisions/2026-09-25-additive-plugin-action-chrome.md).

## Results

Implemented host-only opener capture in the shared modal-manager open path and
safe focus restoration for both dialog and drawer close lifecycles. The host
keeps focus within the focused or topmost surviving dialog surface, including
newer portaled nonmodal surfaces, when an opener is unavailable or a newer
surface remains open. Regression coverage includes Escape, close button, handle
close, unavailable and hidden openers, nested and concurrent modals, owner
cleanup, and nondismissible behavior. A real Radix Popover regression proves
that background cleanup preserves both foreground popover visibility and
focus. Fake timers flush deferred FocusScope cleanup before overlap assertions,
including the delayed old-close-after-successor ordering.

The packaged fixture now exposes focus launchers for the reported workspace,
task, and status Actions. Desktop and phone browser tests pass, including
continued keyboard navigation and focus retention in phone parent surfaces.
The fixture package build keeps the web source and packaged bundle identical.

All verification commands listed above passed. Unit tests passed 37/37;
Chromium passed 4/4 and mobile Chrome passed 5/5. The frontend production build
completed as part of both managed browser runs. Public docs tests passed 62/62,
the public docs validator accepted 47 pages, specification validation accepted
360 decisions and 1425 specifications, and the spec linter tests passed 36/36.
Typecheck, targeted ESLint, Prettier, i18n ratchet, PR documentation coverage,
and `git diff --check` passed. The initial permanent regression failed against
the uncorrected implementation with focus on `BODY`, and the Popover regression
failed when background cleanup dismissed the focused surface, as expected.
