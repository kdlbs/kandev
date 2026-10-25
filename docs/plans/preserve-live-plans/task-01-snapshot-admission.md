---
id: "01-snapshot-admission"
title: "Preserve observed plan state across delayed reads"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-DOCUMENTS-003
system_design:
  - ../../specs/tasks/system-design/plan-write-lifecycle.md
acceptance_criteria:
  - AC-TASKS-DOCUMENTS-003.4
  - AC-TASKS-DOCUMENTS-003.8
---
# Task 01: Snapshot admission

## Summary

Fence older snapshots after observed state and newer reads. Preserve fresh reads
and real editor external-sync/own-save semantics.

## In scope

Current Plan hook, session slice token admission, immediate notification action
bookkeeping, real transport-boundary hook/draft and panel/editor tests.

## Out of scope

Backend/schema/security, transport ordering, Query migration, global conflict
ordering, dirty-draft policy and revision pagination.

## Acceptance

- Live update/create/delete survives old success/null/error with metadata,
  comment identity and unread state intact.
- Latest-read admission spans consumers; tasks/providers are isolated. Retired
  completion cannot clear newer loading/errors or affect a different task view.
- Fresh success/null/error and later refresh work. Desktop/phone editor consumes
  accepted content. Own-save and size rejection behavior remain green.

## Verification

```bash
(cd apps/web && pnpm exec vitest run hooks/domains/session/use-task-plan.refresh.test.tsx hooks/domains/session/use-task-plan.test.ts hooks/domains/session/use-plan-draft.save-ack.test.tsx components/task/task-plan-panel.snapshot.test.tsx components/task/task-plan-panel.save-ack.test.tsx components/task/task-plan-panel.refresh.test.tsx components/task/task-plan-panel.session-switch.test.tsx components/task/task-plan-panel-draft.test.ts components/editors/tiptap/tiptap-plan-editor.test.tsx lib/state/slices/session/task-plans.test.ts lib/ws/handlers/task-plans.test.ts)
(cd apps/web && NODE_OPTIONS=--max-old-space-size=8192 pnpm run typecheck)
(cd apps/web && pnpm exec eslint --max-warnings 0 hooks/domains/session/use-task-plan.ts hooks/domains/session/use-task-plan.test.ts hooks/domains/session/use-task-plan.refresh.test.tsx components/task/task-plan-panel.snapshot.test.tsx lib/state/slices/session/session-slice.ts lib/state/slices/session/types.ts lib/state/app-state-types.ts)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `apps/web/hooks/domains/session/use-task-plan.ts`
- `apps/web/hooks/domains/session/use-task-plan.refresh.test.tsx`
- `apps/web/lib/state/slices/session/session-slice.ts`
- `apps/web/lib/state/slices/session/types.ts`
- `apps/web/lib/state/app-state-types.ts`
- `apps/web/components/task/task-plan-panel.snapshot.test.tsx`

## Dependencies

None.

## Risks

Seen/loading/error admission and provider/lifecycle isolation.

## Parallelism

`sequential`

## Inputs

REQ-TASKS-DOCUMENTS-003.4/.8, Plan snapshot admission in owning design, existing
use-task-plan and real panel save-ack fixtures.

## Results

Permanent RED before production: the four initial hook/store/draft/registered
notification cases failed with nine behavioral assertions: update/delete/create
rewind, unread consumption and older completion clearing replacement loading.
No fixture exception. `/tmp/attempt8-red.log` records that run. Initial GREEN:
four cases passed. Final expanded regression: 119 tests in 11 files passed,
including real Plan panel/TipTap update/delete for desktop and phone. No browser
or production incident evidence is claimed.

Changed-file ESLint passed with zero warnings. Refactored token bookkeeping to
meet the existing 100-line function limit. Catalog validation (395 decisions,
1689 specifications), full spec lint, and diff whitespace checks passed. The
exported `.github/scripts/pr-docs.cjs` `validateCoverage` preflight, supplied
current changed paths plus this plan/work order and its owning requirement/design
contents, returned `ok=true,status=covered,errors=[]`.

Typecheck initially exceeded Node's default 4 GiB heap. The 8 GiB run exposed
missing duplicated AppState declarations and test payload/mock typing; these
were corrected and the test/lint commands rerun successfully. Final typecheck
passed with `NODE_OPTIONS=--max-old-space-size=8192`. Normal commit hooks are required; their receipt accompanies the parent handoff.
Parent actual-diff validation passed and independently reran the original 117
tests. Delivery was subsequently authorized; PR #4651 is open. Exact-current-head
CI, semantic review disposition and protected merge remain delivery gates.


## First review follow-up

Corrected the requirement-mapping table conjunction and clarified that the
creation test models an earlier absence response; the same token admission
rejects non-null stale data in the existing update/delete cases. Retained the
task-ID predicate: a previously captured refetch callback can be invoked after
navigation, when its closure task differs from the current view. Added a real
hook/store regression proving that neither starting nor failing that outgoing
read clears/replaces the current view's error. Temporarily removing only this
predicate failed both assertions; restoring production code passed. That follow-up did not change production
logic. Its local targeted regression contained 118 tests in the
same eleven suites. Remote CI/review evidence must be refreshed after the push.


## Committed-view review correction

A fresh aggregate review identified that a suspended navigation render could
mutate the view ref even though the current task remained committed. The new
real-provider Suspense/startTransition regression failed with the current task's
read error missing. Moving only the view identity update into `useLayoutEffect`
preserves committed ownership before fetch effects run; the same case passed.
Task-ID matching, task round trips, retained callbacks, store token admission,
background settlement and own-save policy remain unchanged. Final focused
regression passed 119 tests in eleven suites. Typecheck with the documented
8 GiB heap, changed-file ESLint, catalog/spec lint and whitespace checks passed.
The correction uses normal commit hooks. Exact-head remote CI/reviews remain pending after push.

The prior current-head CI cycle passed 63 checks with 19 skipped. Its predecessor's
two E2E failures were investigated without changing expectations: both exact tests
passed individually with retries disabled, followed by 10 passing tests across
both complete mobile specs in one managed worker on fresh production artifacts.
This is CI investigation evidence, not Playwright coverage of the Plan ordering
race. New production changes invalidate prior exact-head remote evidence.
