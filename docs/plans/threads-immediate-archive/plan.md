---
created: 2026-09-11
status: done
requirements:
  - REQ-TASKS-THREADS-ACTIONS-001
  - REQ-TASKS-THREADS-ACTIONS-002
  - REQ-TASKS-THREADS-ACTIONS-003
  - REQ-TASKS-THREADS-ACTIONS-004
system_design:
  - ../../specs/tasks/system-design/threads-task-actions.md
legacy_specs: []
---

# Implementation Plan: Immediate Threads Archive Removal

## Overview

Remove an archived task's conversation as soon as the user accepts Archive.
Use the existing task-removal intent to exclude its column while cleanup runs,
then let the current deck recovery select a survivor or show the empty state.
One work order owns the correction, targeted tests, and matching usage docs.

The task system owns this outcome because archive intent, mutation settlement
and visible recovery belong to the existing
[Threads task actions requirement](../../specs/tasks/requirements/threads-task-actions.md).
The [paired design](../../specs/tasks/system-design/threads-task-actions.md)
reuses the deck's ordering and viewport contracts. No new ADR is needed for
this use of the existing removal and presentation boundaries.

## Evidence and requirement conformance

Read-only trace at `20efe4855`:

1. `useTaskManagementFlow` and `useTaskMenuActions({ stayOnListing: true })`
   initiate the existing shared archive operation.
2. `hooks/task-removal-coordinator.ts:coordinateTaskRemovalBatch` calls
   `beginOperation` before invoking the mutation. The operation records the
   task IDs, including captured cascade descendants, in `taskRemoval`.
3. `app/threads/threads-page-client.tsx:ThreadsPageClient` subscribes to
   workflow snapshots and saved views, but not task-removal intent.
   `queryThreadView` has no pending exclusion input. Therefore an unchanged
   snapshot still yields the same column while archive is pending.
4. `ThreadColumn` continues to mount `ThreadConversation` and `TaskChatPanel`.
   Successful snapshot pruning or an authoritative archive event eventually
   removes it. Until then, chat can expose intermediate cleanup state.
5. `lateArchiveOutcome` already holds an archive request before forwarding it,
   but asserts column absence only after `pending.continue()`. Moving the
   absence assertion before that release is the smallest browser regression.

This confirms the missing presentation boundary through source, not a captured
browser replay. The user-reported blocked composer and warning are the symptom;
their precise runtime event sequence was not recorded in this design turn.

The previous contract specified eventual recovery but omitted immediate archive
departure. `AC-TASKS-THREADS-ACTIONS-003.7` and `.8` define that timing and
membership. `AC-TASKS-THREADS-ACTIONS-003.4` now permits provisional departure and requires
failed archives to restore eligible rows without claiming success or stealing
newer selection. The prior prohibition on treating failure as success remains.
This changes visible timing, not archive persistence or cleanup semantics.

The original [Threads task actions package](../threads-task-actions/plan.md)
remains a completed historical delivery. Its results are not this repair's
verification. The existing [task-removal coordinator design](../../specs/tasks/system-design/removal-navigation.md)
is reused; none of its other entry points is being redesigned.

## Scope

In scope: immediate pending archive exclusion; known consented cascade targets;
query counts and limit refill; consumed deep-link focus; surviving/empty
recovery; failed-request readmission; desktop/mobile proof and usage text.

Out of scope: delete timing, new archive APIs, cleanup scheduling, worktree
recovery, new persisted state, saved-view semantics, task-detail/Office/preview
navigation, new confirmation rules, UI geometry changes, and broad refactors.

## Technical approach

- In `ThreadsPageClient`, derive pending archive task IDs from the existing
  `taskRemoval.operationsByToken` subscription. Use operation lifetime through
  reconciliation; a successful response alone is not a reason to reopen chat.
- Add optional `excludedTaskIds` to `ThreadViewQueryOptions`; exclude before
  all query admission paths, including explicit scopes, column limits and
  temporary deep links. Keep exclusion out of `queryFingerprint` and the
  stable-order reset key. Other query callers retain their defaults.
- Keep the board's action provider mounted and feed one consistent candidate
  projection to columns, header counts, phone picker and pagination. Existing
  `useThreadSelectionRecovery` owns successor/predecessor/empty behavior.
- Make `useRetiringFocusMark` distinguish a URL request's identity from its
  resolved visible target. A temporarily excluded task returning after failure
  must not reactivate an already consumed deep link. Add a narrow optional
  board prop for the request identity if needed; preserve current callers.
- Reuse coordinator success/failure cleanup and localized toasts. Do not
  optimistically mutate shared task snapshots or introduce API calls in Threads.
- Update the pending-exclusion note in `components/threads/AGENTS.md` and the
  Threads paragraph in `docs/public/sessions-and-review.md` with implementation.
  Public docs are deferred because this turn changes design intent only.

Source paths above are relative to `apps/web/` unless prefixed with `docs/`.

## ASCII UI preview

UI-01: Accepted archive of B. Opening/cancelling confirmation keeps the before
state. The current chat persistence during cleanup is established by source.

```text
Desktop before / pending today     Desktop immediately after acceptance
+-------+-------+-------+          +-----------+-----------+
| A     | B ... | C     |          | A         | C ...     |
| chat  | chat  | chat  |   --->   | chat      | chat      |
| input | input | input |          | input     | input     |
+-------+-------+-------+          +-----------+-----------+
          Archive B                 B column is absent

Phone before                      Phone immediately after acceptance
+----------------------+          +----------------------+
| Threads / view  2/3  |          | Threads / view  2/2  |
| B          Open ...  |          | C          Open ...  |
| Conversation B      |   --->   | Conversation C      |
| Composer B          |          | Composer C          |
+----------------------+          +----------------------+

Last column archived: [Threads / view] + [existing empty state]
Archive rejected:     [eligible task returns] + [existing error toast]
```

Structural requirements: remove the column/chat subtree, retain survivor order,
keep one phone conversation and the existing header, and show no outgoing
composer or archive warning during pending cleanup. Widths/copy in the sketch
are illustrative. Returning failed tasks use normal arrival order and do not
take focus from the current surviving reader.

The phone entry is the existing visible overflow and inset task-actions drawer;
`mobile-thread-column-header.tsx` and `task-management-drawer.tsx` are the
closest shipped exemplars. The confirmation closes into the snapped deck,
which is the primary content surface. Existing horizontal deck scrolling,
internal transcript scrolling, dynamic viewport sizing, safe areas and touch
targets remain in place. No new scroll container or action is introduced.

## Tests

All numeric AC suffixes below expand to `AC-TASKS-THREADS-ACTIONS-*`.

| Evidence | Coverage |
| --- | --- |
| `app/threads/threads-page-archive.test.tsx`: `removes an archived column before the request settles` with the real store/deferred archive; archive/delete distinction; response/event ordering cannot remount success | `001.5`, `002.5`, `003.7` |
| `lib/threads/thread-view-query.test.ts`: pending target excluded from every candidate list/count, explicit scope/deep link, capped refill, captured descendants | `003.2`, `003.3`, `003.8` |
| Page/board tests: independent A/B operations, rejection readmission, foreign workspace/view changes, already archived/deleted rejection, consumed deep link plus newer menu/selection | `002.5`, `003.1`, `003.4`, `003.5`, `003.8`, `004.6` |
| Existing `stable-order`, `thread-selection-fallback`, board and column-activation tests: survivor position, selected sibling/draft preservation, empty focus and one phone detail | `003.1`-`003.6`, `004.6` |
| Existing `hooks/use-task-menu-actions.test.ts`: pending intent, listing route, duplicate guards and request settlement continue working | `002.5`, `003.4`, `003.7` |

Use a narrowly mocked heavy chat boundary to observe mount/unmount in a rendered
page test, while retaining the real store, query and archive coordinator. Do
not use only final-state assertions or mock the whole query being tested.

## E2E tests

Extend the existing `threads-task-actions.spec.ts` (`chromium`) and
`mobile-threads-task-actions.spec.ts` (`mobile-chrome`). Share stimulus helpers
in `threads-task-actions-edge-helpers.ts` and `threads-pending-archive-helpers.ts`;
reuse `ThreadActionsPage`, settings cleanup and the production mock-agent seed path.

Both projects cover: request held before backend dispatch; real archive
lifecycle events with HTTP response held; last-column empty state; rejection
and retry; deep-linked target; independent survivor/newer menu; confirmation
cancellation and confirmation-disabled acceptance. Unit tests cover the full
cascade/limit/workspace matrix. Assert pending absence before releasing each
barrier. Arm a scoped DOM observer before acceptance to detect outgoing chat
reappearance, and release all held routes in `finally`.

Reuse `holdMutationResponse` for the post-backend barrier. Assert the task is
archived after success and remains absent after reload. On phone, use actual
touch entry, check pagination and picker membership, and swipe the surviving
deck. Inspect desktop/phone pending screenshots during the focused run.

## Work orders

- [x] [Task 01: Remove pending archives from Threads](task-01-remove-pending-archives.md)

Sequential, one work order; no delegation or additional worktree prerequisite.
Its verification block contains the exact runnable commands.

## Verification results

Design checkpoint (2026-09-11):

- `python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- `python3 scripts/lint-spec-files.py --all`: all specification files passed.
- `git diff --check`: passed for tracked changes; `git diff --no-index --check`
  against `/dev/null` passed for both new plan files.
- `git status --short -- docs/specs docs/plans/threads-immediate-archive`:
  exactly two modified specifications and the new plan directory.
- Read-only package validation: four documents, 22 relative links, all 15
  referenced requirement/acceptance IDs, and explicit source/test paths resolve.

At the design checkpoint, production and permanent tests were unchanged,
dependencies were not installed, and product/browser tests were not run.
Implementation was subsequently authorized by "go for it" on 2026-09-12.

Implementation completed on 2026-09-12 in the primary session, without
delegation or additional platform tasks/sessions. The
[work order](task-01-remove-pending-archives.md#results) records behavioral
RED/GREEN evidence, corrected test assumptions, exact environment overrides,
and inspected screenshots.

- Focused frontend suite: 117 tests passed across eight files.
- Fresh managed browser runs: 8 Chromium tests and 10 mobile-chrome tests
  passed with one worker and retries disabled, including native swiping.
- Typecheck, changed-file ESLint with zero warnings, Prettier and the i18n
  new-code ratchet passed.
- Public-doc validator test passed; all 46 published pages validated.
  Specification linter tests passed (36), all specs validated, and whitespace
  checks passed.
- Public how-to documentation now describes immediate departure and failed
  readmission. Scoped Threads guidance records query exclusion, focus request
  identity and retention of surviving viewport observations.

## Risks

- Filtering after column admission leaves incorrect counts or prevents refill.
- Including pending state in the reset key reranks surviving columns.
- Restoring a task can re-trigger a consumed deep link and steal focus.
- Releasing exclusion before snapshot cleanup can flash the archived chat.
- A test that checks only after releasing HTTP can pass with the defect intact.
