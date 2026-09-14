---
id: "01-remove-pending-archives"
title: "Remove pending archives from Threads"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-THREADS-ACTIONS-001
  - REQ-TASKS-THREADS-ACTIONS-002
  - REQ-TASKS-THREADS-ACTIONS-003
  - REQ-TASKS-THREADS-ACTIONS-004
acceptance_criteria:
  - AC-TASKS-THREADS-ACTIONS-001.5
  - AC-TASKS-THREADS-ACTIONS-002.5
  - AC-TASKS-THREADS-ACTIONS-003.1
  - AC-TASKS-THREADS-ACTIONS-003.2
  - AC-TASKS-THREADS-ACTIONS-003.3
  - AC-TASKS-THREADS-ACTIONS-003.4
  - AC-TASKS-THREADS-ACTIONS-003.5
  - AC-TASKS-THREADS-ACTIONS-003.6
  - AC-TASKS-THREADS-ACTIONS-003.7
  - AC-TASKS-THREADS-ACTIONS-003.8
  - AC-TASKS-THREADS-ACTIONS-004.6
system_design:
  - ../../specs/tasks/system-design/threads-task-actions.md
---

# Task 01: Remove Pending Archives from Threads

## Summary

Connect existing pending archive intent to Threads query admission, so the
outgoing column unmounts at acceptance. Preserve existing recovery and handle
failed readmission without reactivating a consumed deep link.

## In scope

- Render-time archive exclusion and consistent query counts/limit refill.
- Real-store regression tests for pending, failure, cascade, deep links,
  independent operations, selection and event/response ordering.
- Desktop and phone E2E coverage, rendered inspection, scoped engineering
  guidance and the existing public Threads usage paragraph.

## Out of scope

Backend behavior, deletion timing, shared removal coordinator redesign, new
state persistence, session management, saved-view preference changes, new copy
or geometry, and unrelated task surfaces.

## Acceptance

1. An accepted archive removes every covered admitted column before HTTP or
   cleanup completes. Confirm-cancel and pending-delete controls retain current
   behavior. Query scope, deep links, counts and caps honor the exclusion.
2. Success stays removed through either event/response order; rejection restores
   only currently eligible tasks. Surviving columns, newer menus/selections,
   consumed focus requests, workspace and saved view remain coherent.
3. Targeted unit and desktop/mobile browser checks pass, with real pending
   interval assertions, phone picker/pagination/swipe evidence, and inspected
   desktop/phone renders. Usage docs describe the implemented timing and failure.

## ASCII UI preview

UI-01, excerpt from the [combined preview](plan.md#ascii-ui-preview).
Coverage: `003.1`-`003.8` and `004.6` under the AC prefix in frontmatter.

```text
Accept Archive B
Desktop: [A chat] [B chat] [C chat] -> [A chat] [C chat]
Phone:   [B chat, 2/3]             -> [C chat, 2/2]
Last:    [B chat, no other tasks]  -> [existing empty state]
Failure: [surviving deck]          -> [eligible B returns + error]
```

The outgoing subtree must be absent. The phone keeps one snapped conversation,
the existing fixed header, transcript scroll owner, safe-area behavior and
touch controls. Failed readmission uses normal arrival ordering and does not
steal focus. Sketch spacing and labels are illustrative.

## Verification

Run from repository root. Install once before the first frontend command in
this fresh worktree. Use `/tdd`; first reproduce with an unchanged production
query and a deferred archive request. Existing `lateArchiveOutcome` provides
the smallest browser stimulus: assert absence before `pending.continue()`.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run app/threads/threads-page-client.test.tsx app/threads/threads-page-archive.test.tsx lib/threads/thread-view-query.test.ts lib/threads/stable-order.test.ts lib/threads/thread-selection-fallback.test.ts components/threads/threads-board.test.tsx components/threads/thread-column-activation.test.tsx components/threads/use-thread-focus-request.test.ts hooks/use-task-menu-actions.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec eslint app/threads/threads-page-client.tsx app/threads/threads-page-client.test.tsx app/threads/threads-page-archive.test.tsx lib/threads/thread-view-query.ts lib/threads/thread-view-query.test.ts components/threads/threads-board.tsx components/threads/threads-board.test.tsx components/threads/use-thread-column-activation.ts components/threads/thread-column-activation.test.tsx components/threads/use-thread-focus-request.ts components/threads/use-thread-focus-request.test.ts e2e/tests/task/threads-task-actions.spec.ts e2e/tests/task/mobile-threads-task-actions.spec.ts e2e/tests/task/threads-task-actions-helpers.ts e2e/tests/task/threads-task-actions-edge-helpers.ts e2e/tests/task/threads-pending-archive-helpers.ts --max-warnings 0)
(cd apps/web && pnpm exec prettier --check app/threads/threads-page-client.tsx app/threads/threads-page-client.test.tsx app/threads/threads-page-archive.test.tsx lib/threads/thread-view-query.ts lib/threads/thread-view-query.test.ts components/threads/threads-board.tsx components/threads/threads-board.test.tsx components/threads/use-thread-column-activation.ts components/threads/thread-column-activation.test.tsx components/threads/use-thread-focus-request.ts components/threads/use-thread-focus-request.test.ts e2e/tests/task/threads-task-actions.spec.ts e2e/tests/task/mobile-threads-task-actions.spec.ts e2e/tests/task/threads-task-actions-helpers.ts e2e/tests/task/threads-task-actions-edge-helpers.ts e2e/tests/task/threads-pending-archive-helpers.ts)
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --project chromium tests/task/threads-task-actions.spec.ts -- --retries=0)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/task/mobile-threads-task-actions.spec.ts tests/task/mobile-threads-swipe.spec.ts -- --retries=0)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
```

Run browser commands sequentially; each managed invocation builds current
production assets. Use test-controlled request/response barriers and existing
causal waits, no arbitrary sleeps. Restore settings and release held routes on
failure. Record RED/GREEN separately, actual discovered/passed counts and
screenshots. If implementation adds another test or changes a helper's scope,
update the exact command list before marking the work complete.

## Files likely touched

Under `apps/web/`:

- `app/threads/threads-page-client.tsx` and `.test.tsx`.
- `app/threads/threads-page-archive.test.tsx` (real-store/coordinator integration).
- `lib/threads/thread-view-query.ts` and `.test.ts`.
- `components/threads/threads-board.tsx` and `.test.tsx` (focus request lifetime).
- `components/threads/use-thread-focus-request.ts` and `.test.ts` (visual
  dismissal versus initial activation lifetime and late hydration).
- `components/threads/use-thread-column-activation.ts` and `thread-column-activation.test.tsx`
  (retain surviving visibility and editor mounts across removal/readmission).
- `e2e/tests/task/threads-task-actions-edge-helpers.ts`.
- `e2e/tests/task/threads-task-actions-helpers.ts` (wait for persisted archive
  independently of immediate column removal).
- `e2e/tests/task/threads-pending-archive-helpers.ts` (desktop/phone pending scenarios).
- `e2e/tests/task/threads-task-actions.spec.ts` and `mobile-threads-task-actions.spec.ts`.
- `components/threads/AGENTS.md`.

Documentation: `docs/public/sessions-and-review.md`, the paired design's status,
this work order and `plan.md` results. The owning index already links the
requirement/design pair; no duplicate capability documents are needed.

## Dependencies

None. Existing task-action and task-removal implementations are present at the
recorded source revision; preserve their behavior and historical plan results.

## Risks

Premature exclusion release, stale deep-link refocus, surviving-column rerank,
and tests that miss the interval before cleanup. See the plan's test matrix.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/tasks/requirements/threads-task-actions.md),
  especially deterministic recovery and archive acceptance.
- [System design](../../specs/tasks/system-design/threads-task-actions.md),
  especially pending archive exclusion and failure behavior.
- `apps/web/AGENTS.md`, `components/threads/AGENTS.md`, `/mobile-parity`, `/e2e`.
- Existing query/order tests, `use-task-menu-actions.test.ts`, `ThreadActionsPage`,
  `lateArchiveOutcome`, `holdMutationResponse`, and mobile swipe helpers.

## Results

Implemented and verified on 2026-09-12 after the user's "go for it".

### Implementation

The page derives archive-only exclusions from the existing removal coordinator,
and the query applies them before every admission/count path. No optimistic
snapshot mutation, new store, API, copy, confirmation rule or delete timing
was added. Query derivation is a local hook to keep the page within the
function-length limit.

The board retains consumed URL request identity across temporary absence and
does not use a retired request as its detail fallback. The viewport observer
now follows the board element's lifetime: existing callback refs reconcile
membership without clearing surviving visibility or remounting editors.

### RED evidence

- Before production changes, the focused query/board run failed four new
  assertions (59 controls passed), and all four new real-store lifecycle cases
  failed because outgoing columns stayed mounted.
- A stronger consumed-link assertion exposed the old detail fallback replacing
  the surviving chat. Two additional observer tests reproduced replacement of
  visible B with fallback A during removal and readmission.
- A fresh browser build with the archive exclusion disabled reproduced both
  held-request regressions: expected zero outgoing columns, observed one.
- The first full desktop run passed six cases and exposed two outdated test
  assumptions. The success label is `Archived 1 task.`, and immediate DOM
  departure no longer proves persisted archival. The existing all-actions
  helper now polls backend persistence independently of column absence.

### Final checks

The initial implementation checks passed (extended fixup results appear below):

- Frozen-lockfile dependency installation succeeded.
- Vitest: 117 passed across eight files, including real-store/coordinator/WS
  integration, query exclusions, focus recovery and activation budgets.
- Typecheck, changed-file ESLint (`--max-warnings 0`), Prettier and i18n ratchet:
  passed.
- Chromium: 8 discovered and 8 passed, retries disabled (1.8 minutes).
- Mobile-chrome: 10 discovered and 10 passed across the task-action and swipe
  specs, retries disabled (2.4 minutes). Coverage includes touch confirmation,
  picker/pagination exclusion, rejection recovery, native swiping and the
  existing 320/360/700/820-pixel geometry checks.
- The successful-archive browser probe observed departure without any DOM
  reappearance through lifecycle events and HTTP settlement. Both projects
  also verified the empty state after reload.
- Public-doc validator test passed and 46 published pages validated.
  All 36 specification linter tests and the all-spec check passed.
  Whitespace checks passed.

The sandbox denied the default Go cache and local socket creation. Final E2E
runs used the approved isolated runner outside that sandbox, prefixed with
`KANDEV_SERVER_HOST=127.0.0.1 GOCACHE=/tmp/threads-archive-go-cache.pom5F8`.
Both invocations rebuilt production assets and ran sequentially with one
worker. The commands additionally used
`--output /tmp/threads-archive-evidence.4vbQSF/desktop` and
`--output /tmp/threads-archive-evidence.4vbQSF/mobile`, respectively. Fixtures
owned and cleaned their temporary backends, worktrees and data; no developer
instance was mutated.

Inspected both projects' `archive-pending.png`, `archive-rejected.png` and
`last-archive-pending.png` beneath those output roots. Pending chat removal,
survivor drafts, restored-column ordering, phone pagination and empty-state
composition are correct. An additional phone header assertion passed before
and after the last archive, with its refreshed empty-state screenshot inspected
under `/tmp/threads-archive-evidence.4vbQSF/mobile-header`. That test-only check
reused the same fresh production build:

```bash
KANDEV_SERVER_HOST=127.0.0.1 GOCACHE=/tmp/threads-archive-go-cache.pom5F8 pnpm e2e:run --no-build --project mobile-chrome tests/task/mobile-threads-task-actions.spec.ts -- --grep 'pending last archive' --retries=0 --output /tmp/threads-archive-evidence.4vbQSF/mobile-header
```

One test was discovered and passed (12 seconds); the modified helper's ESLint
and Prettier checks also passed. The build retained existing bundle-size/dynamic-import
warnings and optional macOS-sidecar signing warnings on Linux.

Public docs updated: `docs/public/sessions-and-review.md` (how-to guide).
The owning design is current and the work package is complete. At the
implementation handoff, no commit, push or PR had been requested or performed.

### PR review remediation

Greptile identified a pre-observer activation race. Two board regressions
reproduced it: pointer and keyboard focus retired the visual mark and unmounted
the requested non-first conversation (28 existing cases passed). The extracted
`useThreadFocusRequest` now keeps that activation fallback until its consumed
target departs, without reviving it on failed-archive readmission. The regressions
assert the same conversation node stays mounted. Three additional hook controls
cover late hydration, interaction before hydration, and a genuinely new request.

Claude's compatibility suggestion is addressed by documenting the explicit
URL-key requirement and legacy optional default. CodeRabbit's aggregate
suggestions are addressed by removing the stale public-doc deferral and adding
the exact Prettier command above. No further public-doc change is needed:
the existing how-to already promises uninterrupted conversation use and focus
preservation; this repair enforces that contract.

Final local remediation verification on 2026-09-12:

- The updated nine-file Vitest command passed all 122 tests.
- Typecheck, changed-source ESLint with zero warnings, the explicit 16-file
  Prettier check, and i18n ratchet passed.
- Fresh managed Chromium run: all eight archive/action regressions plus one
  disposable PR capture passed (nine tests, 1.8 minutes).
- Mobile-chrome reused that fresh build: all ten archive/action/swipe
  regressions plus one disposable PR capture passed (11 tests, 2.3 minutes).
  Both runs used one worker, retries disabled, `--host`, `CAPTURE_PR_ASSETS=1`
  and the previously recorded loopback/cache overrides. Capture specs were
  removed afterward; screenshot publication uses a separate orphan media ref,
  never the feature branch.
- All 19 harness-validator tests, all 196 harness files, all 36 spec-validator
  tests, the all-spec check, the public-doc validator test and all 46 public
  pages passed. Scoped guidance remains within its line budget.

Current-head CI, review-thread resolution, the minimum five-minute new-comment
watch, and current-base integration validation are tracked on the live PR/task
after the remediation commit. Local checks do not substitute for those gates.
