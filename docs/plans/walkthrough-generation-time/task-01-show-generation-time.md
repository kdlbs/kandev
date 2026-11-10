---
id: "01-show-generation-time"
title: "Show walkthrough generation time"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-WALKTHROUGH-TIME-001
acceptance_criteria:
  - AC-TASKS-WALKTHROUGH-TIME-001.1
  - AC-TASKS-WALKTHROUGH-TIME-001.2
  - AC-TASKS-WALKTHROUGH-TIME-001.3
  - AC-TASKS-WALKTHROUGH-TIME-001.4
  - AC-TASKS-WALKTHROUGH-TIME-001.5
system_design:
  - ../../specs/tasks/system-design/walkthrough-generation-time.md
---

# Task 01: Show Walkthrough Generation Time

## Summary

Expose the current saved generation time in the shared walkthrough card.
Use the existing timestamp and localization helpers. Verify desktop and phone behavior.

## In scope

- Timestamp component, card integration, and all seven locale catalogs plus generated pseudo copy.
- Focused component tests and existing desktop/mobile walkthrough E2E suites.
- One short timestamp explanation in the public walkthrough guide.

## Out of scope

Backend changes, PR webpages, chat summaries, relative timers, and navigation redesign.

## Acceptance

1. The card renders a localized generation time from `updated_at`, and replacements update the visible value.
2. Desktop and phone show complete metadata without clipping controls. Invalid values omit only the timestamp.
3. Targeted tests pass, and public documentation explains the timestamp's meaning.

## ASCII UI preview

UI-01: Open walkthrough metadata. See the [full preview](plan.md#ascii-ui-preview).
Desktop and phone share this order:

```text
[Walkthrough]  Step 1 of 5, Line 24
 Generated Oct 28, 2026, 10:42 AM      [X]
[Current step                         v]
```

Wrap the metadata inside the phone bottom sheet. Omit its row for invalid or missing values.
Metadata stays outside the drag handle and above scrolling content.
The date and spacing are illustrative. Criteria .1, .3, .4, and .5 apply.

## Verification

Use TDD for formatting, invalid-value behavior, and reactive updates.
Install dependencies once from `apps/` before package commands in a fresh worktree.
Run these commands from the repository root. E2E commands run sequentially and build fresh assets.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && TZ=UTC pnpm exec vitest run components/diff/walkthrough-generation-time.test.tsx components/diff/walkthrough-step-card.test.ts)
(cd apps/web && NODE_OPTIONS=--max-old-space-size=8192 pnpm run typecheck)
(cd apps/web && pnpm exec eslint components/diff/walkthrough-generation-time.tsx components/diff/walkthrough-generation-time.test.tsx components/diff/walkthrough-step-card.tsx e2e/tests/review/walkthrough.spec.ts e2e/tests/review/mobile-walkthrough.spec.ts)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --project chromium tests/review/walkthrough.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/review/mobile-walkthrough.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Use `pnpm run i18n:zh-hant` from `apps/web` to generate Traditional Chinese translations.
Compare rendered desktop and phone captures with UI-01 during the focused E2E runs.
Record discovered test counts, command results, and any visual mismatch before marking this work order done.

## Files likely touched

- `apps/web/components/diff/walkthrough-generation-time.tsx` and its `.test.tsx` sibling (new).
- `apps/web/components/diff/walkthrough-step-card.tsx`.
- `apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw,ja,ko}/diff.json`.
- `apps/web/e2e/tests/review/walkthrough.spec.ts` and `mobile-walkthrough.spec.ts`.
- `docs/public/sessions-and-review.md` and this package's status records.

## Dependencies

None.

## Risks

Read `updated_at`, not `created_at`. Guard invalid timestamps before invoking the formatter.
Long locale output must wrap without reducing the close button's touch target.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/tasks/requirements/walkthrough-generation-time.md).
- [System design](../../specs/tasks/system-design/walkthrough-generation-time.md).
- Existing `formatDateTime`, `WalkthroughStepInner`, and desktop/mobile walkthrough E2E helpers.
- `apps/web/AGENTS.md`, `/tdd`, `/mobile-parity`, `/better-ui`, and `/e2e` guidance.

## Results

Implementation and all task-defined verification completed on 2026-10-28.

- Installed workspace dependencies with the frozen lockfile.
- RED: the new component tests failed three visible-output assertions against an empty component.
- GREEN: the exact targeted Vitest command passed 10 tests across two files after the final source edit.
- Typecheck passed with the documented 8 GB Node heap. The initial 4 GB attempt exhausted its heap.
- Targeted ESLint passed without warnings after removing an unnecessary single-use variable.
- `i18n:check` and `i18n:ratchet` passed. Existing catalog-orphan notices remain informational.
- Traditional Chinese and pseudo catalogs were generated through the repository scripts.
- Final desktop managed E2E passed all 10 tests (1.3 minutes). It covers persisted time, navigation, reload, feedback, and live replacement.
- Final phone managed E2E passed all 10 tests (1.6 minutes), including English, Portuguese, and landscape navigation.
- Public documentation validator tests passed 63 tests. Public validation passed for 54 pages.
- Specification catalog validation and full specification lint passed. Final status validation and documentation coverage preflight also passed.

### Local browser environment

The host uses Nix. Chromium required its installed shared-library directories in `LD_LIBRARY_PATH`.
The library path was assembled in `/tmp/walkthrough-browser-library-path` and checked with Chromium's `--version` command.
The test runner also required `KANDEV_SERVER_HOST=127.0.0.1` and removal of inherited
`KANDEV_INTERNAL_CONFIG_FILE` and `KANDEV_INTERNAL_CONFIG_HOME_FILE` values.
These overrides affected only test processes. No environment workaround entered production source.

The successful desktop invocation was:

```bash
LD_LIBRARY_PATH="$(cat /tmp/walkthrough-browser-library-path)" KANDEV_SERVER_HOST=127.0.0.1 CAPTURE_PR_ASSETS=1 scripts/run-quiet e2e --summary -- env -u KANDEV_INTERNAL_CONFIG_FILE -u KANDEV_INTERNAL_CONFIG_HOME_FILE pnpm --dir apps/web e2e:run --project chromium tests/review/walkthrough.spec.ts
```

Final desktop log: `/tmp/kandev-run.e2e.pybpvOI8.log`.
Final phone log: `/tmp/kandev-run.e2e.w6mEKXMi.log`.
The phone invocation uses the same command and overrides, with project `mobile-chrome` and file `tests/review/mobile-walkthrough.spec.ts`.
The earlier attempts stopped during fixture readiness or Chromium startup before timestamp assertions ran.
Both final viewport commands build fresh application assets and run one worker.

### Visual comparison

Inspected final desktop and phone captures. They match UI-01: timestamp below progress, a separate close target, and clear navigation.
The landscape test exposed footer clipping from a separate metadata row during implementation.
The timestamp now shares header height with the existing close target, and the full phone suite passes.
Local capture copies are retained in `/tmp/walkthrough-time-captures/`.
No actionable UI-polish findings remain in the changed header.


## Timestamp presentation refinement

The user requested timezone text on hover only and left alignment with the badge.
The timestamp now uses a native title for the full date, time, and timezone.
The visible label uses the same date and time without the timezone.
An explicit left alignment prevents inherited centering.

Refinement verification passed on 2026-10-28. The earlier full-suite results above describe the initial implementation.

- RED: three output assertions failed before removing the visible timezone.
- GREEN: all 10 focused component tests passed, including localized hover text.
- The focused desktop E2E test passed after a fresh build. Log: `/tmp/kandev-run.e2e.TEnm2xUA.log`.
- Both focused phone E2E tests passed, covering English, Portuguese, and landscape containment. Log: `/tmp/kandev-run.e2e.qjwQNWuI.log`.
- Desktop and phone assertions confirm that the timestamp and badge share the same left edge.
- Updated desktop and phone screenshots show the expected alignment and omit the visible timezone.
- Targeted ESLint, public documentation validation, specification validation, and whitespace checks passed.

## Badge text alignment correction

The user clarified that the timestamp must align with the text inside the badge, including its padding and border inset. The previous assertions compared outer element edges. Desktop and phone assertions now measure the actual badge text with a DOM Range. The timestamp inset uses the same spacing token as the badge padding, plus its 1-pixel border.

- RED: the updated desktop assertion failed with a 9-pixel difference. Log: `/tmp/kandev-run.e2e.EpLVvslc.log`.
- GREEN: the focused desktop test passed after a fresh build. Log: `/tmp/kandev-run.e2e.uJCci1pC.log`.
- Targeted ESLint, Prettier, specification validation, and whitespace checks passed.
- Both focused phone tests passed, including English, Portuguese, and landscape containment. Log: `/tmp/kandev-run.e2e.0RStFgud.log`.
- Inspected updated desktop and phone screenshots. The timestamp begins at the badge text, and navigation remains visible.

## PR review follow-up

CodeRabbit requested an exact saved-value assertion for live replacement. The replacement E2E now captures the task-scoped `task.walkthrough.updated` event before the second emission and compares its `updated_at` with the rendered `datetime`, while retaining the changed-value assertion.

The focused replacement test passed with retries disabled (`--grep "a re-emitted walkthrough" --retries=0`). Log: `/tmp/kandev-run.e2e.OP1gToHE.log`. Targeted ESLint and formatting passed. Production behavior and the existing requirements are unchanged.

The follow-up review aligned the replacement event wait with the existing 45-second agent-turn budget, keeping the pre-armed wait and task filter. The focused replacement test passed again with retries disabled (`/tmp/kandev-run.e2e.dAR9YB9b.log`), and targeted lint and formatting passed.

## CI follow-up

E2E shard 16 exposed an existing mobile cancellation test race. The trace showed the command drawer still moving when the test tapped Cancel Turn, so Configuration Chat moved under the tap position. The test now uses the existing finite-animation helper before interacting with the drawer.

Validation used a clean merge of PR head `91b9bcf4cea9` and main `da9ff6332863`, plus this test change. Both cancellation tests passed three times with retries disabled (6 passes; `/tmp/kandev-run.e2e.8JqUQlMd.log`). All seven mobile spec files from the affected CI process then passed with retries disabled (22 passes; `/tmp/kandev-run.e2e.i9AN6CZb.log`). Targeted ESLint and Prettier passed. This correction changes test synchronization only.
