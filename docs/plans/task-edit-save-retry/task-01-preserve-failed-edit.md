---
id: "01-preserve-failed-edit"
title: "Preserve failed task edits"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-EDIT-SAVE-RETRY-001
acceptance_criteria:
  - AC-TASKS-EDIT-SAVE-RETRY-001.1
  - AC-TASKS-EDIT-SAVE-RETRY-001.2
  - AC-TASKS-EDIT-SAVE-RETRY-001.3
  - AC-TASKS-EDIT-SAVE-RETRY-001.4
  - AC-TASKS-EDIT-SAVE-RETRY-001.5
  - AC-TASKS-EDIT-SAVE-RETRY-001.6
system_design:
  - ../../specs/tasks/system-design/task-edit-save-retry.md
---

# Task 01: Preserve failed task edits

## Summary

Independently prove the real current-edit draft loss, then correct both edit
catch paths and verify retry without changing other save stages. No work begins
until a later explicit ROOT reviewed-package implementation INTERRUPT to the
same primary; no heavy command begins before ROOT grants the global lease.

## In scope

- A permanent transport-backed rendered regression through real editor wiring.
- Minimal catch-path correction in `task-create-dialog-submit.tsx`.
- Affected existing regression files, frontend gates, recovery-doc paragraph,
  lifecycle/status results and actual receipt reporting.

## Out of scope

See the [plan exclusions](plan.md#scope). Do not read/import/copy/replay the
protected parent candidate, change the reset effect, add production test seams,
new dependencies, a global draft store, backend changes, or a browser suite.

## Acceptance

1. Independent native RED fails on ordinary title/instructions retention through
   real submit/reset/API/provider/editor wiring; current controls succeed in the
   same collection. Compile/import/matcher errors never qualify as product RED.
2. Both edit variants retain the exact current draft after failure, report the
   failure, clear busy state, preserve confirmed data, and close only after the
   normal successful retry. Recognized/partial outcomes and admission are intact.
3. All exact affected gates below pass after the final change; actual results,
   coverage limits, joins, group cleanup and heavy RETURN are recorded. Specs
   become active/current and plan implemented only after conformity is verified.

## ASCII UI preview

UI-01, [full preview](plan.md#ascii-ui-preview), AC-001.1/.2/.4/.6:

```text
[existing editor stays open on desktop and phone]
 Title:        [  Retry title v1  ]
 Instructions: [  Retry instructions v1\nSecond line  ]
 [existing failure toast]
 [Cancel]                              [Update]
```

This is a state correction; retain existing phone full-height surface, desktop
inset surface, scroll ownership, touch targets, copy and locked fields. Mobile
pure-state exception permits the rendered integration; no browser proof claimed.

## Regression design and RED/GREEN

New file `apps/web/components/task-create-dialog-save-retry.test.tsx`. Render
actual `SidebarTaskEditDialog` -> `TaskCreateDialog` with real `StateProvider`,
`createAppStore`, `ToastProvider`, required real router/UI contexts, real form
state/reset/inputs and production submit/API client. Seed a complete pending or
started task and read-side metadata; inject only external transport responses.
Do not borrow mocks from the existing submit/shell tests. Unexpected requests
fail the fixture; account for frontend-error reporting as a separate known
transport route, never as a task PATCH. Unknown fixture boundary or unrelated
runtime dependency checkpoints ROOT before an alternative or new package.

Saved oracles: title `Confirmed title`, instructions `Confirmed instructions`.
Raw drafts: title `  Retry title v1  `, instructions
`  Retry instructions v1\nSecond line  `. Type via actual rendered controls.
Assert DOM/editor values equal those literals after failure; wire payload stays
trimmed (`Retry title v1`, `Retry instructions v1\nSecond line`). Include an
editable empty-instructions case to protect clearing, without changing rules.
Started tasks change title only and submit no unlocked prompt/repository data.

Cases (test names, annotate corresponding ACs):

- `keeps the raw title after an ordinary edit PATCH fails`: unstarted form
  submit returns HTTP 500 `{error: "Task edit rejected"}` without error_code.
  Title assertion first, remaining open/busy settled and confirmed title intact.
- `keeps raw instructions after an ordinary edit PATCH fails`: separate test
  independently asserts multiline instructions; include explicit empty string.
- `keeps edits after a transport rejection`: reject the task PATCH promise
  with `TypeError("Task edit network failure")`; actual client/hook catches it.
- `retries the current draft and closes after acknowledgement`: fail, revise
  title to `  Retry title v2  ` and instructions to `  Retry instructions v2  `,
  retry via the existing action, assert exact trimmed retry PATCH and normal
  acknowledged consumer/callback outcome. Failure must not publish success.
- `retains title on started update-only failure`: actual Update control routes
  update-only; instructions remain confirmed/locked; ordinary failure retains
  raw title and retry succeeds without agent launch.
- `retains editable drafts on the update-only alternative`: pending editor's
  real split-menu Update alternative with a valid configured agent, unchanged
  runner and ready dependencies; failure cannot start that agent. Observe real
  current title/instructions and valid retry rather than mocked setters.
- Controls: `closes and acknowledges an ordinary successful edit`,
  `retains drafts and refreshes a stale branch policy`, and
  `explicit cancel retains confirmed values`. Stale-policy route returns actual
  typed HTTP 400 `branch_policy_stale`; observe fresh read/correctable draft.

Run the new file ONCE as RED including its controls. Retain individual outcomes;
an initial failed title assertion never proves a separate body assertion passed.
Then minimally change both caught edit failures to retain openness before
existing policy refresh/toast, remove only dead classifier code, and run the
same file with affected compatibility files ONCE as GREEN. Existing runner,
repository, dependency and launch-after-save cases in the submit test are the
controls for their independent confirmed-stage meanings; do not rewrite them
to match a broad unsaved guarantee. Test changes after final GREEN require their
affected rerun; unchanged passing controls receive no separate replay.

## Verification

All commands are rooted independently. Heavy commands run sequentially under
ROOT's lease, never overlapping sibling work. Use GNU timeout with kill-after
10s; `NODE_OPTIONS=--max-old-space-size=2048`, one Vitest worker, no file parallelism.
Timeout is failure, not a passing receipt. No Go/build/Playwright/full suite.

ROOT resource exception, 2026-10-08: the original typecheck exceeded 2048 MiB
and exited 134. ROOT qualified that failure and regranted exclusive heavy86 for
exactly one recovery run of the same `pnpm run typecheck`, GNU timeout 6m and
kill-after 10s, with only `NODE_OPTIONS=--max-old-space-size=4096` changed. All
other commands retain the 2048 MiB bound. No cache/install/runtime/config/source
or TypeScript flag changes are authorized. A new resource/timeout/transport or
unknown failure checkpoints ROOT before another alternative. The original
failure remains failed; recovery is recorded separately.

The recovery exposed actionable own-fixture compiler diagnostics rather than
another resource failure. ROOT's conditional permission admitted their scoped
correction and required affected verification under the same 4096 MiB bound.
No automatic retry of an unchanged resource failure is authorized.

If and only if workspace dependencies are absent, ONE pinned frozen install:

```bash
(cd apps && timeout --kill-after=10s 6m env NODE_OPTIONS=--max-old-space-size=2048 corepack pnpm@9.15.9 install --frozen-lockfile)
```

Keep the lockfile unchanged. Install/resource/transport/timeout/unknown failures
checkpoint ROOT before retry, cache wipe, alternate command or dependency work.

RED (before production edits):

```bash
(cd apps/web && timeout --kill-after=10s 3m env NODE_OPTIONS=--max-old-space-size=2048 pnpm exec vitest run components/task-create-dialog-save-retry.test.tsx --maxWorkers=1 --no-file-parallelism)
```

GREEN plus affected compatibility:

```bash
(cd apps/web && timeout --kill-after=10s 5m env NODE_OPTIONS=--max-old-space-size=2048 pnpm exec vitest run components/task-create-dialog-save-retry.test.tsx components/task-create-dialog-submit.test.tsx components/task-create-dialog-reset-effects.test.ts components/task-create-dialog-setup.test.ts --maxWorkers=1 --no-file-parallelism)
(cd apps/web && timeout --kill-after=10s 3m env NODE_OPTIONS=--max-old-space-size=2048 pnpm exec eslint --max-warnings 0 components/task-create-dialog-submit.tsx components/task-create-dialog-save-retry.test.tsx)
(cd apps/web && timeout --kill-after=10s 6m env NODE_OPTIONS=--max-old-space-size=2048 pnpm run typecheck)
(cd apps/web && timeout --kill-after=10s 3m env NODE_OPTIONS=--max-old-space-size=2048 pnpm run i18n:check)
(cd apps/web && timeout --kill-after=10s 3m env NODE_OPTIONS=--max-old-space-size=2048 pnpm run i18n:ratchet)
```

If existing test source changes to fix a causal fixture/lint issue, add ONLY those
changed test paths to affected eslint coverage and record the exact amended
command. No new copy is planned; if copy changes, all seven locales and normal
locale checks are required within a ROOT-dispositioned scope change.

Lightweight design/docs gates from repo root (no install):

```bash
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check -- docs/specs/tasks docs/plans/task-edit-save-retry
```

After ROOT dispositions the design-turn missing-Node checkpoint, run this exact
read-only reference preflight from repo root, without GitHub `run()` or status
mutation. It reads draft worktree contents and cannot claim published-head/PR
evidence:

```bash
node <<'NODE'
const fs = require('node:fs');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const paths = [
  'docs/specs/tasks/requirements/task-edit-save-retry.md',
  'docs/specs/tasks/system-design/task-edit-save-retry.md',
  'docs/plans/task-edit-save-retry/plan.md',
  'docs/plans/task-edit-save-retry/task-01-preserve-failed-edit.md',
];
const fileContents = Object.fromEntries(paths.map(p => [p, fs.readFileSync(p, 'utf8')]));
const result = validateCoverage({
  changedFiles: paths.map(filename => ({ filename, status: 'added' })),
  fileContents,
});
console.log(JSON.stringify(result, null, 2));
if (!result.ok) process.exitCode = 1;
NODE
```

Inspect `ok`, `errors[]`, accepted references and exact paths. At implementation
also include the actual changed production/test/public-doc paths in
`changedFiles` while keeping this same complete referenced-document input; record
the exact resulting command. Never synthesize committed-head coverage from this
draft preflight.

After the small Troubleshooting how-to paragraph is implemented:

```bash
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check
git status --short
```

## Receipts, caps, and cleanup

Retain every original native tool response, start/chunk/session, actual terminal
chunk/exit, owned PID/process group and wait/reap. Poll each retained handle to
actual completion; never discard a handle or infer from a duplicate/wrapper log.
Receipt files may live in a task-owned temp directory, never the parent's
protected proof. Each long command must have a known owned group and a fresh
post-join descendant/group absence check. Explicitly join/reap all owned
descendants before ROOT heavy RETURN. Preserve foreign refs/caches/processes,
the platform worktree and dependencies.

Clean rendered fixtures, fetch/global stubs, editor instances, store
subscriptions and any fixture-owned timers in test teardown. Flush/join the
known error-report transport lifecycle and restore timer state; never add sleeps
or weaken assertions to accommodate it. Avoid new test-only production exports.
On a crash, durable receipts identify original live handles; continue them
without replay. Routine causal fixture/lint corrections may affect only scoped
files. Resource/timeout/transport/unknown/outscope failures checkpoint ROOT
before any alternative. Delivery after heavy RETURN follows the platform plan's
single original hosted wait, frozen head, full-current-head semantic coverage,
and separate ROOT MERGE grant.

## Files likely touched

- `apps/web/components/task-create-dialog-submit.tsx`
- `apps/web/components/task-create-dialog-save-retry.test.tsx` (new)
- `docs/public/tasks-and-workflows.md` (small recovery paragraph)
- This work order, its plan, and paired spec lifecycle/results.

Read-only first-party context: `task-create-dialog-setup.ts`,
`task-create-dialog-state.ts`, `task-create-dialog-reset-effects.ts`,
`task-create-dialog-form-body.tsx`, `task-create-dialog-footer.tsx`,
`task/task-session-sidebar-edit.tsx`, `task/task-actions-menu-dialogs.tsx`,
`task/mobile/session-task-switcher-sheet-dialogs.tsx`, `lib/api/domains/kanban-api.ts`,
`lib/api/client.ts`, and the three affected compatibility files above.

## Dependencies and parallelism

No prerequisite work order. Later ROOT implementation release and heavy lease
are separate required gates. Sequential; no delegation.

## Risks

Real first-party fixture hydration may reveal an unknown requirement; checkpoint
ROOT instead of substituting a predicate-only or mocked setter proof. Typed
partial commits must keep their acknowledged-field reconciliation. Do not claim
a browser/mobile geometry check, atomic multi-stage save, or server rollback.

## Inputs

- [Requirement](../../specs/tasks/requirements/task-edit-save-retry.md), all six ACs.
- [Design](../../specs/tasks/system-design/task-edit-save-retry.md), all sections.
- [Plan](plan.md), evidence qualifications and binding design/delivery barriers.
- Scoped `apps/web/AGENTS.md`, `/tdd`, `/mobile-parity`, `/docs-maintainer`.

## Results

Implemented and locally verified after ROOT's later reviewed-package implementation release to the
same primary. Independently authored the full first-party rendered regression;
seven causal draft-retention RED failures with three positive controls, followed
by 62 affected tests GREEN. Scoped fixture lint cleanup required a new-file-only
rerun: 10/10 passed. No parent candidate was read/copied/imported/replayed.

The two edit catches now retain the dialog before the unchanged branch-policy
refresh and error toast; only the dead error classifier was removed. Added the
public Troubleshooting recovery paragraph. Reset and confirmed partial stages
are untouched. No browser geometry coverage is claimed.

Original typecheck `a79481` / session `82607` / actual terminal `7834b5` exited
134 at the reviewed 2 GiB heap cap. Heavy86 was returned, ROOT qualified the
failure and regranted the documented 4 GiB exception. The recovery produced only
own-fixture compiler diagnostics, corrected under ROOT's conditional permission.
Final typecheck `bc7748` / session `2466` / `7ec77b` passed; the original failure
remains failed. Final new-file GREEN `93dd15` / `65348` / `ed9907`: 10/10.
The unchanged 52 compatibility cases retain their original GREEN receipt.

Final scoped ESLint and both i18n gates passed. Public documentation validator
tests passed 62/62 and validated 47 pages; actual seven-path reference preflight
returned `covered`, `ok: true`, `errors: []`. Active/current artifact catalog and
specification lint passed. All six ACs conform through the real integration,
first-party mobile consumer audit and existing partial-stage controls. Normal
hooks, publication and hosted/merge delivery remain externally pending. All completed original heavy
handles were joined with actual wait/reap and empty owned groups. Detailed
receipts and noncausal fixture failures are preserved in the platform plan and
[manifest results](plan.md#verification-results).
