---
id: "01-responsive-actions"
title: "Make Storage actions fit their section"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-SYSTEM-PAGE-STORAGE-ACTION-LAYOUT-001
acceptance_criteria:
  - AC-SYSTEM-PAGE-STORAGE-ACTION-LAYOUT-001.1
  - AC-SYSTEM-PAGE-STORAGE-ACTION-LAYOUT-001.2
  - AC-SYSTEM-PAGE-STORAGE-ACTION-LAYOUT-001.3
  - AC-SYSTEM-PAGE-STORAGE-ACTION-LAYOUT-001.4
  - AC-SYSTEM-PAGE-STORAGE-ACTION-LAYOUT-001.5
  - AC-SYSTEM-PAGE-STORAGE-ACTION-LAYOUT-001.6
  - AC-SYSTEM-PAGE-STORAGE-ACTION-LAYOUT-001.7
  - AC-SYSTEM-PAGE-STORAGE-ACTION-LAYOUT-001.8
  - AC-SYSTEM-PAGE-STORAGE-ACTION-LAYOUT-001.9
  - AC-SYSTEM-PAGE-STORAGE-ACTION-LAYOUT-001.10
  - AC-SYSTEM-PAGE-STORAGE-ACTION-LAYOUT-001.11
  - AC-SYSTEM-PAGE-STORAGE-ACTION-LAYOUT-001.12
  - AC-SYSTEM-PAGE-STORAGE-ACTION-LAYOUT-001.13
  - AC-SYSTEM-PAGE-STORAGE-ACTION-LAYOUT-001.14
system_design:
  - ../../specs/system-page/system-design/storage-action-layout.md
---

# Task 01: Make Storage actions fit their section

## Summary

Make Analyze and Run now fit the actual Settings content width. Keep both
controls readable and usable through job-state changes and viewport changes.

## In scope

- Write a rendered overlap regression before changing the layout.
- Add available-width layout rules in `StorageActions` and its page root.
- Add opt-in full-width sizing for the disabled tooltip wrapper.
- Check phone composition, wrapped labels, target sizing, and keyboard actions.

## Out of scope

- Changes to maintenance logic, permissions, API requests, or confirmation.
- Changes to other Storage cards, global control sizing, or Settings navigation.
- New copy, dependencies, motion, or persistence.

## Acceptance

1. All linked criteria pass through rendered layout, visual, and activation checks.
2. Both enabled and disabled controls fit their cells in every tested state.
3. Existing operation tests pass, and other wrapper consumers keep their layout.

## ASCII UI preview

See the [complete before/after previews](plan.md#ascii-ui-preview).
These excerpts retain labels UI-01, UI-02, and UI-03 and cover criteria 001.1
through 001.11. The asterisk marks the primary action and is not rendered text.

```text
UI-01: Wide section, idle
Explanation                 [ Analyze ]   [ Run now* ]

UI-02: Constrained section, idle
Explanation
[        Analyze        ]   [        Run now*        ]

UI-03: Phone, idle
Explanation
[                Analyze                ]
[                Run now*               ]
```

Each phone control has a 44px minimum target. Under 24rem section width, a
non-phone section also uses separate action rows. Job-state and disabled
controls retain the same composition. Compare running and completed labels
with UI-04 in the full preview. The existing Settings page owns scrolling.

## Implementation sequence

1. Read the linked contracts, `apps/web/AGENTS.md`, `/tdd`, `/e2e`, and
   `/mobile-parity`. Mark only this work order `in_progress`.
2. Add the failing overlap regression to the new desktop Playwright spec.
   Use existing selectors for the initial RED assertion. A missing selector
   does not prove the layout bug.
3. Implement the available-width grid and opt-in wrapper sizing. Preserve
   translation keys, operation handlers, job-state markers, and focus order.
4. Add the remaining state, keyboard, phone, and coarse-pointer cases.
5. Run the checks below. Compare rendered views with the previews. Record
   actual results and update the lifecycle only after all checks pass.

## Rendered cases

- Desktop fine pointer: normal wide layout and the reported constrained
  layout with the Settings sidebar present. Check viewport widths around
  640px and 768px. Check section widths just below, at, and above 24rem and
  52rem. Use viewport changes for real Settings layouts. Cap the section width
  for exact container boundaries that the desktop sidebar cannot produce by
  viewport resizing alone.
- State changes: idle, queued, running, succeeded, failed, and member-disabled.
  Route analysis and cleanup operations to disposable mock jobs. Include the
  existing longer translated status labels, using the application's language
  setting. Restore any changed persisted setting in `afterEach` or `finally`.
- Phone: configured Pixel 5 and a 320px viewport. Keep one control per row.
  Measure each target and its wrapper. Tap Analyze and Run now and assert the
  corresponding mocked request and visible result. Cover narrow fine-pointer
  phone behavior in the desktop spec too.
- Tablet: use the mobile project's coarse-pointer context at 820px width.
  Check the actual section width and matching one/two-column composition.
- Interaction: use real Tab and Enter/Space input. Resize while focus or a job
  is active. Check retained focus, state, control order, and enabled handlers.

Assert control bounds against their cell and section, not only the viewport.
Check non-overlap with the copy and sibling control. Check actual label
clipping and `document.documentElement.scrollWidth <= clientWidth`.
For enabled controls, check `elementFromPoint` at the control center before
activation. Measure 28px single-line fine-pointer desktop height within 1px.
Measure at least 44px height for phone and coarse-pointer controls.

Capture the narrow desktop and phone states with `prCapture`. Inspect them
against the preview hierarchy and shared icon, radius, and variant styling.
No new animation needs verification.

## Verification

Run from the repository root. Install dependencies once if this worktree lacks
`apps/node_modules`. Do not run the desktop and mobile E2E commands concurrently.

The typecheck uses an 8 GiB Node heap because the default 4 GiB heap failed
in this worktree. E2E runs use host mode and a loopback server bind.
On NixOS, set `LD_LIBRARY_PATH` to the installed Chromium runtime libraries
before E2E runs. The passing runs used systemd, libxcb, and the local Kandev
Playwright library bundle. Nix store paths depend on the installed generation.

```bash
test -d apps/node_modules || (cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm test components/settings/system/storage)
(cd apps/web && pnpm exec eslint --max-warnings 0 components/settings/system/storage/storage-maintenance-settings.tsx components/settings/system/storage/storage-action-button.tsx e2e/helpers/storage-actions-layout.ts e2e/tests/system/storage-actions-layout.spec.ts e2e/tests/system/mobile-storage-actions-layout.spec.ts)
(cd apps/web && NODE_OPTIONS=--max-old-space-size=8192 pnpm run typecheck)
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/web && KANDEV_SERVER_HOST=127.0.0.1 pnpm e2e:run --host --project chromium tests/system/storage-actions-layout.spec.ts tests/system/storage-maintenance.spec.ts)
(cd apps/web && KANDEV_SERVER_HOST=127.0.0.1 pnpm e2e:run --host --project mobile-chrome tests/system/mobile-storage-actions-layout.spec.ts tests/system/mobile-storage-maintenance.spec.ts)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Run the documentation cross-reference preflight described in the plan too.
Record the initial failing geometry assertion and the final passing commands.
The managed E2E runner rebuilds production assets. Do not substitute stale
assets or a visibility-only check for geometry evidence.

## Files likely touched

- `apps/web/components/settings/system/storage/storage-maintenance-settings.tsx`
- `apps/web/components/settings/system/storage/storage-action-button.tsx`
- `apps/web/e2e/tests/system/storage-actions-layout.spec.ts`
- `apps/web/e2e/tests/system/mobile-storage-actions-layout.spec.ts`
- `apps/web/e2e/helpers/storage-actions-layout.ts`

Update this work order, its plan, and the linked contracts for final results.

## Dependencies

None. Existing shared button and settings primitives supply control behavior.

## Risks

- Full-width wrapper styles must remain opt-in for the two top-level actions.
- Automatic height must override the matching phone and coarse-pointer variants.
- A visually contained control can still have an overlapping active hit area.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/system-page/requirements/storage-action-layout.md).
- [System design](../../specs/system-page/system-design/storage-action-layout.md).
- Existing `storage-maintenance-settings.test.tsx` coverage for job labels,
  active-job disabling, permission gates, and operation handlers.
- Existing `storage-maintenance.spec.ts` and `mobile-storage-maintenance.spec.ts`
  for isolated storage setup, request mocks, screenshots, and operation flows.
- `CleanupActions` in `tool-payload-retention-card.tsx` for phone action rows.

## Results

Implemented and verified on 2026-11-03 in the primary session.

### Regression evidence

Before production changes, the 1024px browser regression failed on the actual
button edge. Expected right edge: at most 887.359375px. Observed: 952.71875px.
The button exceeded its cell by about 65px. The same test passed after the fix.

The initial two runs stopped on environment setup: a non-loopback server bind,
then missing Chromium library paths on NixOS. The fixture became usable with
`KANDEV_SERVER_HOST=127.0.0.1` and `LD_LIBRARY_PATH` pointing to existing Nix
libraries. These settings affect only local test commands.

The first keyboard test assumed one Tab from Host reached Analyze. The existing
Radix tab panel is an intermediate stop. The corrected test traverses that
panel with real Tab input before checking focus retention and Enter/Space.

### Check results

| Command from the repository root | Result |
| --- | --- |
| `(cd apps && pnpm install --frozen-lockfile)` | Passed. Worktree dependencies installed. |
| `(cd apps/web && pnpm test components/settings/system/storage)` | 127 tests passed across 15 files. |
| ESLint command in Verification, including both component files and all three new E2E files | Passed with zero warnings. The keyboard spec was checked again after its final edit. |
| `(cd apps/web && NODE_OPTIONS=--max-old-space-size=8192 pnpm run typecheck)` | Passed. The default 4 GiB heap exhausted its limit. |
| `(cd apps/web && pnpm run i18n:ratchet)` | Passed. Both modified production files are clean. |
| `pnpm --dir apps exec prettier --check web/components/settings/system/storage/storage-action-button.tsx web/components/settings/system/storage/storage-maintenance-settings.tsx web/e2e/helpers/storage-actions-layout.ts web/e2e/tests/system/storage-actions-layout.spec.ts web/e2e/tests/system/mobile-storage-actions-layout.spec.ts` | Passed. |
| `scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --host --no-build --project chromium tests/system/storage-actions-layout.spec.ts tests/system/storage-maintenance.spec.ts` | 13 passed in 1.2 minutes, with no retries. |
| `scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --host --project mobile-chrome tests/system/mobile-storage-actions-layout.spec.ts tests/system/mobile-storage-maintenance.spec.ts` | Nine passed in 29.3 seconds, with no retries. |

Both E2E commands used the local Nix library settings and loopback bind above.
`CAPTURE_PR_ASSETS=true` enabled screenshots. The production assets were rebuilt
after the last production edit. The final desktop run reused that build after
a test-only keyboard correction. The phone run rebuilt current assets again.

### Rendered evidence

Desktop checks cover viewport widths 639, 640, 641, 767, 768, 769, 1024, and
1400px. Exact section boundaries cover 383, 384, 385, 831, 832, and 833px.
They check contained buttons and wrappers, visible gaps, unclipped labels,
28px desktop height, 44px touch minimums, hit targets, and page overflow.

Job cases cover idle, queued, running, failed, succeeded, and member-disabled
states. Portuguese labels use the actual Display language setting. Phone
cases cover the configured Pixel 5, 320px width, and an 820px coarse-pointer
tablet. Both mocked operations complete through their normal UI handlers.

Visual inspection confirms the preview hierarchy and existing icon, radius,
and variant styling. The narrow desktop actions sit below the explanation.
Phone actions occupy separate full-width rows. Wide controls remain beside
the explanation. The screenshots are available for file previews:

- `.pr-screenshots/storage-action-layout/desktop-constrained.png`
- `.pr-screenshots/storage-action-layout/desktop-wide.png`
- `.pr-screenshots/storage-action-layout/desktop-translated.png`
- `.pr-screenshots/storage-action-layout/phone.png`
- `.pr-screenshots/storage-action-layout/phone-completed.png`

The saved desktop log is
`.tmp/storage-layout-logs/kandev-run.e2e.KweHsROR.log`.
The saved phone log is
`.tmp/storage-layout-logs/kandev-run.e2e.jKVLy9zI.log`.

PR review corrections split the acceptance criteria and aligned Verification
with the recorded host mode, loopback bind, Nix libraries, and Node heap.
Only documentation and test annotations changed. Documentation validation,
focused lint, formatting, and diff checks passed again.

Final specification, reference, and diff validation is recorded in the plan.

### Accessibility review follow-up

CodeRabbit found that active spinners exposed the default English label.
The initial fix translated that label and stabilized each button's name.
The final component keeps one persistent translated status beside each button
and hides the decorative spinner. Existing catalogs cover all locales.
The button and geometry helpers also have brief invariant comments.

The Portuguese browser regression first failed with `Loading` instead of
`A analisar...`. Its RED log is `/tmp/kandev-run.e2e.IWIo4ttI.log`.
The extended desktop and phone cases now check stable button names and
translated active statuses. Both suites passed without retries:

| Command from the repository root, with the Verification environment | Result |
| --- | --- |
| `scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --host --project chromium tests/system/storage-actions-layout.spec.ts tests/system/storage-maintenance.spec.ts` | 13 passed in 1.3 minutes. Log: `/tmp/kandev-run.e2e.d9DUrqmI.log`. |
| `scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --host --no-build --project mobile-chrome tests/system/mobile-storage-actions-layout.spec.ts tests/system/mobile-storage-maintenance.spec.ts` | Nine passed in 46.3 seconds. Log: `/tmp/kandev-run.e2e.5aSazten.log`. |

The phone run reused the current desktop build. No production edits followed
that build. All 127 Storage unit tests passed again. Typecheck, focused lint,
formatting, the i18n ratchet, and the full i18n check passed. Specification and
documentation-reference checks passed. CI and the review watch remain external
delivery checks until the current PR head is terminal.

### Terminal status review follow-up

The next review found that terminal results lost their live announcement when
the spinner disappeared. `StorageJobButton` now owns both the button and its
persistent status region. One derived label supplies the visible text and the
announcement through queued, running, failed, and succeeded states.

The new regression failed because no status region remained after failure.
Its RED log is `/tmp/kandev-run.e2e.Cf1iZ5pi.log`. The desktop loop also checks
the visible Portuguese failure labels requested in the aggregate review.
Phone coverage checks failures, retries, and completion announcements.

Both Verification browser commands passed again after the final production edit:

- Desktop: 13 passed in 1.3 minutes. Log: `/tmp/kandev-run.e2e.jCoH6LZF.log`.
- Phone/tablet: nine passed in 33.9 seconds, reusing the current build.
  Log: `/tmp/kandev-run.e2e.7v45CXv9.log`.
- All 127 Storage unit tests, typecheck, focused lint, formatting, the i18n
  ratchet, catalog validation, specification lint, and reference checks passed.

The change adds only a visually hidden announcement outside each control.
Visible labels, geometry, and operation behavior retain their existing contract.
The published screenshots remain visually accurate. Remote CI and the final
review watch remain pending until the pushed head has terminal evidence.

### Live-region retention coverage

The browser cases now mark each idle status node before the first operation.
They verify the marker after active, failed, completed, and permission updates.
The phone case also retains the marker through viewport changes. A replacement
node cannot satisfy this check by rendering the same text.

Only test helpers, assertions, and delivery records changed. The product code
is unchanged from the terminal-status fix. The focused checks passed:

| Command from the repository root, with the Verification environment | Result |
| --- | --- |
| `scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --host --no-build --project chromium tests/system/storage-actions-layout.spec.ts` | Three passed in 10.9 seconds. Log: `/tmp/kandev-run.e2e.7gTzepGW.log`. |
| `scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --host --no-build --project mobile-chrome tests/system/mobile-storage-actions-layout.spec.ts` | Two passed in 7.0 seconds. Log: `/tmp/kandev-run.e2e.EtgxgA3l.log`. |

Both runs used the current production build. Focused lint and formatting passed.
Remote CI and the final review watch remain pending.
