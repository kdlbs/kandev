---
id: "01-row-keyboard"
title: "Restore row keyboard ownership"
status: done
wave: 1
depends_on: []
plan: plan.md
requirements:
  - REQ-TASKS-KEYBOARD-ACTIONS-003
acceptance_criteria:
  - AC-TASKS-KEYBOARD-ACTIONS-003.1
  - AC-TASKS-KEYBOARD-ACTIONS-003.2
  - AC-TASKS-KEYBOARD-ACTIONS-003.3
system_design:
  - ../../specs/tasks/system-design/task-keyboard-actions.md
---

# Task 01: Restore row keyboard ownership

## Summary and scope

Use TDD at the real sidebar row boundary, then preserve native nested button
activation in `components/task/task-item.tsx`. Inspect the actual
`task-switcher-row.tsx`, `task-switcher-context-menu.tsx`,
`task-switcher-archive-confirmation.tsx`, and phone picker before writing the
regressions. Preserve all other work and avoid edits to those adapters unless
new evidence establishes they are indispensable. Exclude shared confirmation
changes, pointer gestures, unrelated menus and visual redesign.

## Acceptance

1. Real nested action and inline confirmation buttons own Enter/Space without
   row selection; existing action dispatch and confirmation outcomes work.
2. Row navigation, noninteractive title-origin events, modifier and selection
   semantics, repeats, already-prevented events, pending removal and subtask
   toggle retain their existing behavior.
3. Independently authored rendered RED/control and focused desktop and phone
   Chromium evidence meet the plan's tests and E2E scenarios.

## Existing surfaces

On desktop, the existing trailing Task actions button opens the task menu
without activating its row. On phone, the task title opens the existing picker,
whose visible row action opens the existing sheet with keyboard or touch. The
picker retains its fixed header, scrolling list and safe-area handling. On
coarse non-phone layouts, the existing inline archive Cancel and Archive
buttons own their activation. The surfaces and copy remain unchanged. See the
[plan description](plan.md#existing-surfaces).

## Verification and sequencing

After the separate implementation and exclusive heavy-resource grants, use the
pinned pnpm 9.15.9, Node 24.21.0, and Go 1.26.0 paths provided in the task.
Install once with `cd apps && pnpm install --frozen-lockfile`. Run the following
sequentially, retaining each native process handle, exit status and actual test
count. Give every `run-quiet` invocation a distinct task-owned
`KANDEV_RUN_QUIET_DIR`; only E2E tags use `--summary`.

1. Author `components/task/task-item-keyboard-ownership.test.tsx` with real
   `TaskItem`/`TaskMenuButton` and providers. Run before production changes:
   `cd apps/web && pnpm exec vitest run --project browser-locales components/task/task-item-keyboard-ownership.test.tsx --maxWorkers=1 --no-file-parallelism`.
   Record both causal failures and the click/contextmenu and row-Enter controls.
   New unreviewed tests use HappyDOM browser-locales; inspect the reviewed
   project mapping for any existing test also run. Do not call this Chromium.
2. Apply the minimal row guard after recording the rendered causal RED and
   controls, then rerun the focused component regression:
   `cd apps/web && pnpm exec vitest run --project browser-locales components/task/task-item-keyboard-ownership.test.tsx --maxWorkers=1 --no-file-parallelism`.
3. Author and run the desktop and phone Playwright GREEN tests described in
   the plan. Use actual focused buttons and real Enter/Space presses, with
   route/selection, row-key and phone-touch controls. Run desktop:
   `pnpm --dir apps/web e2e:run --host --shards 1 --project chromium tests/task/sidebar-action-keyboard.spec.ts tests/task/sidebar-inline-archive-keyboard.spec.ts -- --workers=1 --retries=0`.
   This first GREEN run builds fixed production assets. Run phone separately:
   `pnpm --dir apps/web e2e:run --host --shards 1 --project mobile-chrome tests/task/mobile-sidebar-action-keyboard.spec.ts -- --workers=1 --retries=0`.
   The phone run may add `--no-build` before `--` only after verifying unchanged
   production asset inputs and fingerprints; otherwise use its normal build.
   Wrap each with `scripts/run-quiet e2e --summary --` and a unique directory;
   verify intended discovery, actual joins, counts and route/menu outcomes.
4. Keep the projects sequential, with one worker and zero retries. Record the
   native Chromium results separately from the HappyDOM component evidence.
5. Run scoped ESLint over every changed TS/TSX file, including helpers and
   tests: `cd apps/web && pnpm exec eslint --max-warnings 0 <actual-changed-ts-files>`.
   Run `cd apps/web && pnpm run typecheck` only after a fresh grant for its
   resource cap. Ordinary corrections within the approved cap are covered;
   unknown, authorization, resource, OOM or out-of-scope failures require a
   checkpoint rather than an unchanged retry.
6. Stage actual owned additions before the i18n ratchets; then run from
   `apps/web`: `pnpm run i18n:check` and `pnpm run i18n:ratchet`. Run
   `python3 scripts/list-docs.py validate`,
   `python3 scripts/lint-spec-files.py --all`, and `git diff --check` from the
   repository root. Record the current actual documentation coverage. Use normal
   active hooks and no bypass for later delivery.

Implementation and local checks were authorized in a later separate release.
The actual rendered RED had 2 expected failures and 6 passing controls; the
final component suite passed 9 tests in HappyDOM. Desktop Chromium passed 2
and mobile Chromium passed 1 with actual keyboard input and zero retries.
Scoped ESLint, 6144 MiB typecheck, i18n check and ratchet passed. The initial
desktop browser attempt failed its test-only return-to-row focus sequence after
both menu keys succeeded; the revised initial-traversal control passed. No
public documentation update was needed. Delivery remains subject to the
separately stated task gates.

## Dependencies and risks

No prerequisite work order. Native Chromium must prove keyboard-generated
clicks; HappyDOM keydown/default assertions cannot. Inline archive requires the
real non-phone coarse and resolved-classification path; phone archive is a
dialog, so do not mislabel it as inline. Active/multi-selected row state and
pending removal must not be inferred from menu visibility alone.
