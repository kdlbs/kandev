---
id: "04-repository-gear"
title: "Repository gear and responsive settings"
status: done
wave: 4
depends_on: ["03-remote-preparation"]
plan: plan.md
requirements:
  - REQ-TASKS-REMOTE-OPTIONS-001
  - REQ-TASKS-REMOTE-OPTIONS-002
  - REQ-TASKS-REMOTE-OPTIONS-003
acceptance_criteria:
  - AC-TASKS-REMOTE-OPTIONS-001.1
  - AC-TASKS-REMOTE-OPTIONS-001.2
  - AC-TASKS-REMOTE-OPTIONS-001.3
  - AC-TASKS-REMOTE-OPTIONS-001.4
  - AC-TASKS-REMOTE-OPTIONS-002.1
  - AC-TASKS-REMOTE-OPTIONS-002.2
  - AC-TASKS-REMOTE-OPTIONS-002.3
  - AC-TASKS-REMOTE-OPTIONS-002.4
  - AC-TASKS-REMOTE-OPTIONS-003.2
system_design:
  - ../../specs/tasks/system-design/remote-repository-options.md
---

# Task 04: Repository gear and responsive settings

## Summary and scope

Own `apps/web/components/task-create-dialog-remote-repo-chip.tsx`, a new
`task-create-dialog-repository-options.tsx`, `task-create-dialog-types.ts`,
`task-create-dialog-repositories-state.ts`, `task-create-dialog-helpers.ts`,
shared create-dialog callers, API client/types and capability hook, all related
unit tests, the two task E2E files from the plan, and locale catalogs. Follow
shared state patterns; fetch capability data in a domain hook, not a component.

Use the row key for applied settings and a separate transient draft for the open
panel. Show the gear after branch and before Remove; use a drawer on phones.
Reuse existing dialog dismissal handling and leave the parent form open on
settings dismissal. Reset on repository identity changes, preserve on branch
changes, and omit policy from independent new tasks and repository-set defaults.

Update `docs/public/tasks-and-workflows.md` as a how-to under `/docs-maintainer`
only once behavior is implemented. Document supported combinations and the
remaining timeout limitation. Exclude runtime redesign and repository defaults.

## Dependencies

Complete the preceding work order; no parallel execution is planned.

## Implementation acceptance

1. Desktop and phone users apply options to one remote row, see its summary, and create a task with the persisted policy while other rows retain their own settings.
2. Cancel/reset, repository changes, executor capability changes, invalid paths, keyboard dismissal, and repeated task creation satisfy the ownership/validation criteria.
3. Rendered phone tests prove drawer composition, reachable actions, measured touch targets, scroll containment, and no desktop state corruption; translations pass.

## Verification

Use TDD: reproduce the required failure before implementation, then verify the
real behavior. New files named below are created by this work order. Commands
run from the repository root; install once with `(cd apps && pnpm install
--frozen-lockfile)` before any package command if this worktree lacks dependencies.

```sh
(cd apps/web && pnpm exec vitest run components/task-create-dialog-checkout-options.test.ts components/task-create-dialog-remote-repo-chip.test.tsx components/task-create-dialog-remote-repo-chips.test.tsx components/task-create-dialog-helpers.multi-repo.test.ts)
(cd apps/web && pnpm run typecheck && pnpm run i18n:check)
make -C apps/backend build
make -C apps/backend e2e-plugin-package
(cd apps/web && pnpm run build:e2e)
(cd apps/web && pnpm e2e:run --project=chromium tests/task/remote-repository-options.spec.ts tests/task/create-task-remote-repo.spec.ts)
(cd apps/web && pnpm e2e:run --project=mobile-chrome tests/task/mobile-remote-repository-options.spec.ts tests/task/mobile-create-task-remote-repo.spec.ts)
```

## ASCII UI preview

See the [combined preview](plan.md#ascii-ui-preview).

### UI-01: Desktop, selected remote row and expanded settings

```text
[raycast/extensions] [main v] [gear*] [x]
  On demand, 1 folder

  Repository settings: raycast/extensions
  Applies to this task only
  Download       [Download on demand v]
  Folders        [Selected folders v]
  [extensions/my-extension                 ]
  [packages/shared                         ]
  One directory per line; invalid lines are identified.
  [Reset]                  [Cancel] [Apply]
```

The gear is always visible on a resolved remote row; `*` illustrates a subtle
non-default indicator. The panel is bounded and the parent dialog stays open.
Root/ancestor files are included by directory-based sparse checkout; shared
subdirectories can be added explicitly. No success claim is based on this sketch.

### UI-02: Phone, gear opens an inset bottom drawer

```text
[raycast/extensions]
[main v]              [gear*] [x]
On demand, 1 folder

+--------------------------------+
| Repository settings            |
| raycast/extensions             |
| Applies to this task only      |
|--------------------------------|
| Download                       |
| [Download on demand v]         |
| Folders                        |
| [Selected folders v]           |
| Directories, one per line      |
| [extensions/my-extension    ]  |
| [packages/shared            ]  |
| [Reset]                        |
|--------------------------------|
| [Cancel]               [Apply] |
+--------------------------------+
```

Header and safe-area footer are fixed; only the middle region scrolls. Existing
mobile picker-sheet geometry is the exemplar. Controls have 44px touch targets;
ordinary desktop controls retain 28px density. Folder edits use this same drawer,
not another stacked picker. Label width and ASCII spacing are illustrative.
Control grouping, row ownership, scroll ownership, and action order are required.

### UI-03: Validation and capability states

```text
[../outside]
Use a folder path inside this repository.
                              [Apply disabled]

Download on demand unavailable
This executor cannot fetch missing files after setup.
[Standard v]
```

Empty repository row: no gear until a repository URL is selected or submitted.
Capability loading: retain current applied settings, disable Apply for custom settings until current
identity/executor eligibility is known. Capability error: Retry without resetting
folder inputs. Changing executors does not silently clear incompatible settings.
These views cover AC-001.1 through AC-001.4, AC-002.2/002.4, and AC-003.2.

## Risks

Preserve task identity, provider authorization, credential redaction, existing
branch/contribution semantics, and user work. A capability or environment blocker
is not a passing result; record it and do not advertise unverified support.

## Results

Implemented task-only row options, desktop popover, phone drawer, translated
copy, per-line path validation, support-check retry, and payload propagation.

Passed: 74 frontend regression unit tests; two capability hook tests; ten existing
desktop picker browser tests plus the new two-repository settings test; six
existing phone picker tests plus the new drawer test. Final drawer smoke passed
after the capability retry change. The desktop test verifies create/read
persistence and fresh-task defaults. Typecheck, focused ESLint, i18n checks, and
public docs validation passed.

Observed and fixed before completion: missing gear, phone row overflow after
adding the gear, stale capability responses, and failed capability-request retry.
Browser geometry assertions now wait for drawer animation; multi-row assertions
wait for closed popovers to unmount. No fixed-duration sleeps were added.
Real software-keyboard behavior and explicit 767/768px measurements remain
unverified; the browser checks cover desktop and a phone viewport.

### UI refinement, 2026-09-17

User feedback: hide the gear until a repository is selected and match existing
Kandev form styling. Empty rows now omit the gear. Shared Select controls replace
native radios, using the task-priority selector as the exemplar. The desktop
panel is 320px wide with text-xs labels and tighter spacing; the phone retains
its drawer, one scroll region, and touch-sized controls. Copy is unchanged.

Validation: the empty-row regression failed before the fix; all 30 chip unit
tests passed after it. Updated desktop and mobile checkout settings E2E passed.
Focused ESLint, typecheck, spec validation, and whitespace checks passed. The
isolated manual-test frontend was refreshed on port 48479; port 9998 was untouched.
