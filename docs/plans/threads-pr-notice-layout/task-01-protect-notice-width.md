---
id: "01-protect-notice-width"
title: "Protect shared PR notice width"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-MOBILE-TASK-ACTION-BAR-001
acceptance_criteria:
  - AC-UI-MOBILE-TASK-ACTION-BAR-001.1
  - AC-UI-MOBILE-TASK-ACTION-BAR-001.2
  - AC-UI-MOBILE-TASK-ACTION-BAR-001.3
  - AC-UI-MOBILE-TASK-ACTION-BAR-001.4
  - AC-UI-MOBILE-TASK-ACTION-BAR-001.6
  - AC-UI-MOBILE-TASK-ACTION-BAR-001.7
  - AC-UI-MOBILE-TASK-ACTION-BAR-001.8
  - AC-UI-MOBILE-TASK-ACTION-BAR-001.9
system_design:
  - ../../specs/ui/system-design/mobile-task-action-bar.md
---

# Task 01: Protect shared PR notice width

## Summary

Give terminal PR notices enough width before adjacent actions consume the row.
Prove the correction in narrow desktop Threads tiles and preserve phone behavior.

## In scope

Shared banner flex allocation, the passthrough parent if required, targeted
desktop and mobile browser regressions, and existing component compatibility.

## Out of scope

PR state detection, archive policy, new copy, toolbar redesign, stored
preferences, and Threads admission rules.

## Acceptance

1. A new browser test fails on the current desktop tile with visible utilities
   because ordinary notice text collapses. It passes after the allocation fix.
2. Both terminal variants remain readable and contained at the specified tile
   widths. Wide desktop preserves compact inline layout when enough space exists.
3. Phone hit targets, empty state, Archive cancellation, and task-scoped
   dismissal retain their established behavior in chat and passthrough hosts.

## UI preview

UI-01 from the [combined preview](plan.md#ui-preview): a narrow desktop notice
contains readable text, Archive, and dismiss. Utilities and Done wrap beneath
when necessary. Phone retains its full-width notice, right-aligned Done row,
and subsequent utility row. Wide desktop retains a single row when it fits.
No notice space remains after dismissal. Maps to criteria .1 through .4 and .9.

## Implementation sequence

1. Install workspace dependencies. Add the desktop geometry test and run it
   against the current build. Record the actual text width and expected failure.
2. Update shared notice allocation as specified in the paired design. Check
   passthrough's nested wrapper without changing its interaction owners.
3. Add phone Threads coverage. Run the commands below sequentially. Inspect
   desktop and phone captures, then record results in this work order and plan.

## Verification

Run from the repository root. The guarded E2E runner builds current sources.
Do not overlap browser suites or override its worker budget.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run components/task/chat/pr-archive-banners.test.tsx components/task/chat/chat-status-bar.test.tsx components/task/passthrough-toolbar.test.tsx)
(cd apps/web && NODE_OPTIONS=--max-old-space-size=8192 pnpm run typecheck)
(cd apps/web && pnpm exec eslint components/task/chat/pr-archive-banners.tsx components/task/passthrough-toolbar.tsx e2e/tests/task/threads-pr-notice-helpers.ts e2e/tests/task/threads-pr-notice-layout.spec.ts e2e/tests/task/mobile-threads-pr-notice-layout.spec.ts)
(cd apps/web && pnpm e2e:run --project chromium tests/task/threads-pr-notice-layout.spec.ts tests/chat/chat-status-bar.spec.ts tests/cli-mode/passthrough-toolbar.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/task/mobile-threads-pr-notice-layout.spec.ts tests/chat/mobile-chat-status-bar.spec.ts tests/cli-mode/mobile-passthrough-composer.spec.ts)
(cd apps/web && pnpm run i18n:ratchet)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `apps/web/components/task/chat/pr-archive-banners.tsx`
- `apps/web/components/task/passthrough-toolbar.tsx` only if parent allocation fails
- `apps/web/e2e/tests/task/threads-pr-notice-layout.spec.ts` (new)
- `apps/web/e2e/tests/task/mobile-threads-pr-notice-layout.spec.ts` (new)
- `apps/web/e2e/tests/task/threads-pr-notice-helpers.ts` (shared geometry assertions)
- This work order and `plan.md` for execution status and results

## Dependencies

None. Existing mobile action-bar and action-order plans are implemented.

## Risks

Phone viewport tests alone miss the desktop tile defect. Assert viewport width,
tile width, visible controls, and message geometry independently. Preserve
wide inline composition and the passthrough wrapper's empty state.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/ui/requirements/mobile-task-action-bar.md), especially .9
- [System design](../../specs/ui/system-design/mobile-task-action-bar.md#narrow-desktop-notice-allocation)
- `apps/web/components/threads/AGENTS.md`
- `apps/web/e2e/tests/task/threads-presentation-helpers.ts`
- `apps/web/e2e/tests/chat/mobile-chat-status-bar.spec.ts`
- `apps/web/e2e/tests/chat/chat-status-bar.spec.ts`
- [Existing mobile plan](../mobile-task-action-bar/plan.md)
- [Existing workflow placement plan](../mobile-composer-action-wrapping/plan.md)

## Results

Implementation authorized and completed on 2026-10-23.

The regression failed before production changed: the text span measured 0px,
but its longest ordinary word required 50.23px. Evidence:
`/tmp/kandev-run.e2e.YnWzvf0r.log`. The first green run passed all four
merged/closed Columns/Grid scenarios in 56.5s:
`/tmp/kandev-run.e2e.BUyrTyyY.log`.

The production change adds bounded minimum width and a preferred flex basis
to the existing shared banner. No passthrough parent change was required.

Completed checks:

- Workspace dependency install passed.
- The specified Vitest command passed 57 tests across three files.
- Typecheck passed with `NODE_OPTIONS=--max-old-space-size=8192`. The initial
  default 4 GiB Node heap exhausted memory before completion.
- ESLint and Prettier checks passed for the implementation and new test files.
- `pnpm run i18n:ratchet` passed.
- Catalog validation, specification lint, and `git diff --check` passed.
- Existing desktop chat and passthrough scenarios passed all 16 tests.
  The combined run also passed the four geometry scenarios. Its new translation
  case initially attempted a cookie on `about:blank`; the fixture now uses the
  isolated backend URL. The final desktop rerun passed all five Threads tests
  in 52.8s, including translated text and wide inline layout:
  `/tmp/kandev-run.e2e.qjWH2eTr.log`.

Local E2E environment: the inherited launcher configuration selected an
unreachable bind address, and Chromium required Nix shared libraries. The
temporary `/tmp/kandev-notice-test-env.sh` unsets `KANDEV_INTERNAL_CONFIG_FILE`,
`KANDEV_INTERNAL_CONFIG_HOME_FILE`, `KANDEV_INTERNAL_AGENTCTL_STARTUP_CONFIG`,
and `KANDEV_PARENT_PIPE_FD`. It sets `KANDEV_SERVER_HOST=127.0.0.1` and
`LD_LIBRARY_PATH` to installed library directories. These overrides apply only
to the test subprocess. No runtime or repository configuration was modified.

Browser commands run through `scripts/run-quiet e2e --summary --` after sourcing
that temporary file. The first green run rebuilt the backend, E2E web bundle,
and fixture plugin. Subsequent runs use `--no-build` with those same artifacts;
the production source did not change afterward. New test files run from source.

The first mobile run passed eight cases, including the new Threads test and
all notice geometry checks. Two existing terminal-output cases failed because
the host only configured DejaVu Sans. A temporary
`FONTCONFIG_FILE=/tmp/kandev-notice-fonts.conf` adds installed Liberation Mono
for the monospace family. Both failed cases then passed in 16.0s:
`/tmp/kandev-run.e2e.YgzktD2v.log`. No terminal source or test was changed.
The full mobile command passed all 10 tests in 1.1m with that environment
override: `/tmp/kandev-run.e2e.0TogjAmA.log`.

Final evidence totals: 57 component tests, 21 desktop browser scenarios, and
10 mobile browser scenarios passed. The final desktop Threads rerun covers
all five new desktop cases after the last test edit. The final mobile run
covers the new phone case and both existing mobile suites. Documentation
coverage preflight passed with zero errors. The change is ready for publication.

Desktop Columns and Grid screenshots were inspected. They show readable
notice text above compact utility controls. The phone screenshot preserves
the short full-width notice, Archive/dismiss targets, and workflow row above
utilities. Captures remain outside the tracked source tree.

## PR compatibility follow-up

The current base moved archive notices inside `chat-status-bar-actions`.
The original desktop geometry assertion treated that container as a sibling
below the notice. A synthetic merge reproduced four failures even though the
notice remained readable. The updated assertions measure utility controls and
Proceed directly. They require each group to fit beside or below the notice
without overlap or horizontal overflow, and retain wide inline alignment.
This permits the valid partial wrap at 480px while protecting 360px and 384px.

The three component suites passed 59 tests on the combined code. All five
updated desktop scenarios passed both there and on the PR branch:
`/tmp/kandev-run.e2e.pnCGUCb8.log` and
`/tmp/kandev-run.e2e.PEm8Cwlw.log`. Prettier and targeted ESLint passed.

All 12 mobile compatibility cases passed on the combined code, including the
new Threads case: `/tmp/kandev-run.e2e.FG6nWLMN.log`. CodeRabbit's summary
requested docstrings for the new helpers; each helper now states its contract.
No production behavior changed during this follow-up.
