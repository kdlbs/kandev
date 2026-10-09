---
id: "03-e2e-and-public-docs"
title: "End-to-end coverage and public documentation"
status: done
wave: 2
depends_on:
  - "01-backend-visibility-input"
  - "02-shared-reveal-control"
plan: "plan.md"
requirements:
  - REQ-WORKSPACES-HIDDEN-FOLDERS-001
acceptance_criteria:
  - AC-WORKSPACES-HIDDEN-FOLDERS-001.1
  - AC-WORKSPACES-HIDDEN-FOLDERS-001.3
  - AC-WORKSPACES-HIDDEN-FOLDERS-001.4
  - AC-WORKSPACES-HIDDEN-FOLDERS-001.5
  - AC-WORKSPACES-HIDDEN-FOLDERS-001.10
system_design:
  - ../../specs/workspaces/system-design/hidden-folder-browsing.md
---

# Task 03: End-to-end coverage and public documentation

## Summary

Prove the user-visible outcome through a real directory browser, and make the
public documentation match what ships. A hidden directory that a user can reach
in the product but not find in the docs is still a gap.

## Scope

- A desktop Playwright spec that opens a real directory browser, activates the
  control, sees a hidden entry, enters it, and selects it.
- A separate mobile Playwright spec for the coarse-pointer target and pinned
  slot, plus narrow fine-pointer coverage in the desktop spec.
- A public documentation update for the control, in the page that already
  documents the browser folder picker and the repo-less starting folder.
- Confirmation that the spec asserts the default-off listing too, so the
  no-regression case is covered by the same journey.

## Exclusions

- No new test project and no change to the E2E shard budget.
- No detailed native-picker instructions, symlink browsing, or tilde handling.
  Public docs state that the native picker uses the operating system's control.
- No production or test code change unless a failure here proves a defect, which
  is reported rather than absorbed.

## Implementation acceptance conditions

1. The desktop-width spec shows a home listing with no hidden entry, activates
   the control, sees the hidden entry, enters it, and confirms the selected
   path is the hidden directory's canonical path.
2. The phone-width spec shows the control meeting the coarse-pointer target and
   remaining in the pinned slot while the breadcrumb scrolls.
3. The public documentation states the control's existence and default for
   in-app directory browsers. It names the native-picker exception.

## Verification commands

```bash
cd apps/web
pnpm e2e:run e2e/tests/task/directory-browser-hidden-folders.spec.ts
pnpm e2e:run --project mobile-chrome e2e/tests/task/mobile-directory-browser-hidden-folders.spec.ts
pnpm run i18n:check
cd ../..
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
```

## Likely files

- `apps/web/e2e/tests/task/directory-browser-hidden-folders.spec.ts`
- `apps/web/e2e/tests/task/mobile-directory-browser-hidden-folders.spec.ts`
- `apps/web/e2e/tests/task/mobile-add-workspace-sources.spec.ts` for the existing
  folder-picker journey helpers
- `docs/public/desktop-app.md`, and the public task documentation that covers
  the repo-less starting folder

## Dependencies and risks

- This work order depends on tasks 01 and 02; an end-to-end failure before both
  land is expected, not a defect.
- Local runners enforce one worker per shard and a memory-aware shard budget, so
  this spec runs in the existing task shard rather than a new project.
- The public documentation must not claim symlink browsing or tilde expansion.
  If the docs page turns out to describe the browser in a way this change
  contradicts, update the requirement rather than the sentence.

## Results

Done.

Two specs were written rather than one, because the `mobile-chrome` Playwright
project only collects `mobile-*.spec.ts`, and a phone test in a desktop-named
file would have been measured at desktop geometry.

Verification:

| Command | Result |
| --- | --- |
| `pnpm e2e:run e2e/tests/task/directory-browser-hidden-folders.spec.ts` | 4 passed |
| `pnpm e2e:run --project mobile-chrome e2e/tests/task/mobile-directory-browser-hidden-folders.spec.ts` | 2 passed (Pixel 5) |
| `pnpm exec playwright install chromium` | installed the build this Playwright version expects (1228); the cached 1243 was not usable |
| `pnpm run typecheck` | clean |
| `pnpm exec eslint` on both specs | clean |

The mobile spec first failed waiting for `files-workspace-actions`, because a
phone opens Files as a drawer from its own button and uses a drawer rather than a
dialog. The navigation was corrected against the proven
`mobile-add-workspace-sources` flow.

The desktop specs assert the default-off listing, the reveal plus re-list, entry
into the hidden directory, and sequential Tab access to the switch. A narrow
fine-pointer case checks the 44px target and a center click. The mobile spec
checks the coarse-pointer target and a touch-driven reveal.

All six E2E cases wait for `primary_executor_type` before opening the picker.
This prevents the Add folder action from racing task setup.

Code review found that the plan's UI-01 claim, "the breadcrumb scrolls while the
control stays fixed", was not proven by any test. A second phone case now walks
a seven-segment path and asserts the control stays flush against the popover's
trailing edge, that the breadcrumb genuinely overflows and scrolls, and that
there is no document-level horizontal overflow. That case first failed while
measuring an animating popover, which reports a narrower box and a scaled
control; it now waits for finite animations and asserts the trailing gap is zero
rather than merely unchanged.

Public documentation: `docs/public/tasks-and-workflows.md` describes the control
next to the source table, and `docs/public/desktop-app.md` distinguishes the
in-app browser from the operating system picker. Neither mentions symbolic-link
browsing or tilde expansion.


### 2026-10-06 CI fixture remediation

The desktop helper now waits for the session page and activates Files before
opening workspace actions. CI had selected Changes, leaving the Files control
mounted but invisible. A controlled inactive-panel reproduction failed at the
click; activating Files passed with the same bounded click budget. Temporary
reproduction controls were removed. Product behavior and public documentation
are unchanged.

Checks from `apps/web` on the combined main/PR tree:

- `pnpm e2e:run --host --no-build --project chromium -- tests/task/directory-browser-hidden-folders.spec.ts tests/chat/message-queue.spec.ts tests/session/long-prepare-panels.spec.ts tests/workflow/workflow-session-targeting.spec.ts --grep 'Directory browser hidden folders|Send Now resumes Auto-run|file tree and terminal keep waiting|delivers a source-step target' --retries=0`: seven passed, including all four directory-browser cases.
- `pnpm e2e:run --host --no-build --project mobile-chrome -- tests/task/mobile-directory-browser-hidden-folders.spec.ts --retries=0`: two passed.
- `pnpm exec eslint e2e/tests/task/directory-browser-hidden-folders.spec.ts` and `pnpm exec prettier --check e2e/tests/task/directory-browser-hidden-folders.spec.ts`: passed.
- `pnpm run typecheck` and `pnpm run lint`: passed.

Hosted verification remains pending until the remediation head passes CI.


### Phone geometry CI follow-up, 2026-10-06

Run `37460626206`, shard 6 job `112264886098`, failed the reveal-control
height assertion by 1.7129 pixels while the popover's opening scale animation
was still running. The coarse-pointer case now waits for the same finite
animation settlement already used by the neighboring breadcrumb case before
measuring geometry. It exercises a one-second duration on the existing
production animation to make this timing regression deterministic. The
44-pixel target, one-pixel tolerance, touch interactions, and test deadline
are unchanged. Production CSS and behavior are unchanged.

- RED: the slower production opening animation failed the original sizing assertion by 2.1582 pixels, with retries disabled.
- GREEN: `cd apps/web && pnpm e2e:run --host --no-build --shards 1 --project mobile-chrome tests/task/mobile-directory-browser-hidden-folders.spec.ts -- --repeat-each 3 --retries 0 --trace=retain-on-failure`: all six cases passed in 46.8 seconds, covering three independent repetitions of both phone cases.
- Focused Prettier and ESLint passed. Hosted verification follows the next pushed head.

- Desktop parity: `cd apps/web && pnpm e2e:run --host --no-build --shards 1 --project chromium tests/task/directory-browser-hidden-folders.spec.ts -- --retries 0`: all four cases passed in 30.9 seconds.
