---
id: "01-clear-window"
title: "Restore output after local clear"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-BACKGROUND-WORK-004
acceptance_criteria:
  - AC-AGENTS-BACKGROUND-WORK-004.1
  - AC-AGENTS-BACKGROUND-WORK-004.2
  - AC-AGENTS-BACKGROUND-WORK-004.5
system_design:
  - ../../specs/agents/system-design/background-work.md
---

# Task 01: Restore output after local clear

## Summary

Make a locally cleared detail view show later retained output even when its
rolling window is full. First independently reproduce the rendered failure
through the actual store producer, then implement bounded client bookkeeping.

## In scope

- Actual panel, provider, hook and append action regression with only the HTTP
  list mocked; control with the same append and no Clear view. For the separate
  phone check, use the real chip and store with a phone breakpoint and the same
  HTTP boundary; verify opening the drawer and its retained output.
- Client-only window position in JS code units, identity/generation scoping,
  repeated clear, search, auto-scroll and truncation behavior.
- Direct store-action tests for append, accepted identical snapshot and omitted
  output update, different accepted replacement, run change, rejected update,
  list removal, clear, purge, re-addition, and isolation of other workloads.
- Existing phone drawer output at phone width, without introducing phone clear.

## Out of scope

Protocol/backend/producer cap changes, flag rollout, settings, stale-event or
save companion work, UTF-8 truncation, new product copy, styling, or mobile flow.

## Acceptance

1. A pre-fix user-action test fails specifically because the fresh marker is
   missing from the rendered output while the real capped store retains it;
   the no-clear control passes. The identical test passes after the fix.
2. Direct store-action tests prove the sidecar transitions in the owning design;
   pre-cap and rolling appends, rollover past the clear point, repeated clears,
   search, truncation notice, auto-scroll, and session/workload/run isolation
   preserve their intended visible behavior. A multibyte case proves no wire-byte
   offset is used as a JS index.
3. Desktop renders the corrected view and the existing phone drawer still shows
   shared output at phone width. No mobile clear action is claimed or added.

## ASCII UI preview

UI-01 and UI-02, AC-004.1/004.2/004.5 (see [full preview](plan.md#ascii-ui-preview)):

```text
Desktop detail after clear + append:   Phone existing output drawer:
+---------------------------+          +-----------------------+
| Search | Pause | Clear    |          | Back   Build watcher  |
| [earlier output truncated]|          | fresh marker          |
| fresh marker              |          +-----------------------+
+---------------------------+
```

Desktop toolbar is above its output scroller; phone has one output scroller and
no Clear view control. Text and spacing are illustrative, not new copy or geometry.

## Verification

After a separate implementation interrupt and HEAVY grant, install once from
`apps/` with `pnpm install --frozen-lockfile` in the fresh worktree. Prepend
`/home/jcfs/.local/share/mise/installs/pnpm/9.15.9`,
`/home/jcfs/.local/share/mise/installs/node/24.21.0/bin`, and
`/home/jcfs/.local/share/mise/installs/go/1.26.0/bin` to PATH in that order.
Use an owned unique run-quiet directory and retain/join each native handle.
Before production edits run this exact suite and record collected count, causal
assertion failure, control pass, and actual exit. After the fix rerun it, the
new direct action suite, and the existing panel and purge suites. The two new
unreviewed suites are `browser-locales`; both existing suites are `browser`
according to `vitest-reviewed-projects.json`, not `node`. Confirm nonzero
discovery for each project:

```bash
(cd apps/web && pnpm exec vitest run --project browser-locales --maxWorkers=1 --no-file-parallelism components/task/chat/background-work/background-work-clear-window.test.tsx lib/state/slices/session-runtime/background-work-clear-window.test.ts)
(cd apps/web && pnpm exec vitest run --project browser --maxWorkers=1 --no-file-parallelism components/task/chat/background-work/background-work.test.tsx lib/state/slices/session-runtime/purge-session.test.ts)
(cd apps/web && pnpm exec eslint --max-warnings 0 components/task/chat/background-work/background-work-panel.tsx components/task/chat/background-work/background-work-clear-window.test.tsx lib/state/slices/session-runtime/session-runtime-slice.ts lib/state/slices/session-runtime/types.ts lib/state/slices/session-runtime/background-work-clear-window.test.ts lib/state/slices/session-runtime/background-output-window.ts)
(cd apps/web && pnpm run typecheck)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check -- docs/specs docs/plans/background-work-clear-window
```

Before the first production edit run the `browser-locales` command with only the
rendered test path (the store test may not exist yet). Adapt scoped ESLint inputs
to include every actual changed TS/TSX file, including added helpers and types.
Use the separately approved memory budget for typecheck. Do not treat zero test discovery,
setup failure, or a helper-only assertion as RED or GREEN. Record actual results
and any phone-rendering blocker, not planned counts.

After implementation and only under the later delivery authorization, stage
the actual owned frontend changes and all four package files before either
ratchet; include any additional owned helpers or types in the explicit `git add`
and ESLint inputs after inspecting the diff. Do not stage unrelated changes.
For the expected file set:

```bash
git add -- apps/web/components/task/chat/background-work/background-work-panel.tsx apps/web/components/task/chat/background-work/background-work-clear-window.test.tsx apps/web/lib/state/slices/session-runtime/session-runtime-slice.ts apps/web/lib/state/slices/session-runtime/types.ts apps/web/lib/state/slices/session-runtime/background-work-clear-window.test.ts apps/web/lib/state/slices/session-runtime/background-output-window.ts docs/specs/agents/requirements/background-work.md docs/specs/agents/system-design/background-work.md docs/plans/background-work-clear-window/plan.md docs/plans/background-work-clear-window/task-01-clear-window.md
git diff --cached --name-status
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/web && pnpm run e2e:sleep-ratchet)
```

Run the PR-documentation coverage preflight against the staged owned paths and
staged versions of the four referenced artifacts (the pure evaluator does not
contact GitHub):

```bash
node <<'NODE'
const { execFileSync } = require('node:child_process');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const paths = execFileSync('git', ['diff', '--cached', '--name-only', '-z'], {encoding: 'utf8'}).split('\0').filter(Boolean);
const docs = [
  'docs/plans/background-work-clear-window/task-01-clear-window.md',
  'docs/plans/background-work-clear-window/plan.md',
  'docs/specs/agents/requirements/background-work.md',
  'docs/specs/agents/system-design/background-work.md',
];
const fileContents = Object.fromEntries(docs.map(p => [p, execFileSync('git', ['show', `:${p}`], {encoding: 'utf8'})]));
const result = validateCoverage({changedFiles: paths.map(filename => ({filename, status: 'modified'})), fileContents});
console.log(JSON.stringify(result, null, 2));
if (!result.ok || result.status !== 'covered') process.exitCode = 1;
NODE
```

This preflight assumes no staged deletions or renames; if the owned diff contains
either, supply its actual status and previous filename to the evaluator rather
than presenting it as modified. Confirm the staged paths include every intended
addition and no unrelated files. Retain and join each native handle and run in
the unique owned run-quiet directory; don't infer an exit from another run.

## Files likely touched

- `apps/web/components/task/chat/background-work/background-work-panel.tsx`
- `apps/web/components/task/chat/background-work/background-work-clear-window.test.tsx`
- `apps/web/lib/state/slices/session-runtime/session-runtime-slice.ts` and its
  client state types if needed for bounded bookkeeping.
- `apps/web/lib/state/slices/session-runtime/background-output-window.ts` and
  `background-work-clear-window.test.ts` for owning action and cleanup coverage.

## Dependencies

None. One sequential work order; implementation requires the later separate
interrupt and HEAVY grant.

## Risks

Output replacements are not appends, and identical strings may recur. Avoid
content-overlap guesses; transport offsets use different units. Existing phone
presentation lacks clear and is not a real Chromium test in HappyDOM.

## Parallelism

Sequential.

## Inputs

- [Requirements](../../specs/agents/requirements/background-work.md),
  [design](../../specs/agents/system-design/background-work.md), and
  [normalization decision](../../decisions/2026-09-27-provider-neutral-background-work.md).
- Existing producer, detail view, chip drawer, and component test patterns.

## Results

The independent pre-fix `browser-locales` rendered suite collected two tests:
the clear case failed at the displayed fresh-marker assertion while the real
store retained it at the full cap, and the no-clear control passed (exit 1).
After implementation the two new `browser-locales` suites passed 10 tests;
the existing `browser` panel and purge suites passed 7 tests. Scoped ESLint
passed for all six changed TS/TSX files. Canonical web typecheck passed with
`NODE_OPTIONS=--max-old-space-size=6144`. HappyDOM verifies rendered behavior,
not Chromium geometry or a phone clear control. Staged ratchets and the
documentation preflight passed as recorded in the plan.

## Review correction: hook ownership

Greptile comment 4395153358, thread `PRRT_kwDOQ2-eWs6w60d8`, identifies that
the store-subscribing `useClearedOutput` is co-located in the component despite
`apps/web/AGENTS.md` requiring hooks under `hooks/domains/`. Move the same hook
body to `apps/web/hooks/domains/session/use-cleared-background-output.ts` and
import it into the panel. Preserve its selection, anchor, return values, and
rendered behavior. This is a focused ownership correction, not a new contract;
the real panel/store rendered regression already exercises the hook. Do not add
a mirror unit test or broaden snapshot reconciliation. The separate correction
implementation and HEAVY grant was received before the move.

After that grant, use the pinned PATH, task-owned run-quiet directory and actual
handle joins described above. Run the following affected checks after the move,
with no zero-test success and no test-only replacement of the actual hook:

```bash
(cd apps/web && pnpm exec vitest run --project browser-locales --maxWorkers=1 --no-file-parallelism components/task/chat/background-work/background-work-clear-window.test.tsx)
(cd apps/web && pnpm exec vitest run --project browser --maxWorkers=1 --no-file-parallelism components/task/chat/background-work/background-work.test.tsx)
(cd apps/web && pnpm exec eslint --max-warnings 0 components/task/chat/background-work/background-work-panel.tsx hooks/domains/session/use-cleared-background-output.ts)
(cd apps/web && NODE_OPTIONS=--max-old-space-size=6144 pnpm run typecheck)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Before staged ratchets, review and stage only actual owned correction paths,
including the new hook and changed component and design/work-order records;
include any additional actual changed TS/TSX in scoped ESLint. For this move:

```bash
git add -- apps/web/components/task/chat/background-work/background-work-panel.tsx apps/web/hooks/domains/session/use-cleared-background-output.ts docs/specs/agents/system-design/background-work.md docs/plans/background-work-clear-window/plan.md docs/plans/background-work-clear-window/task-01-clear-window.md
git diff --cached --name-status
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/web && pnpm run e2e:sleep-ratchet)
git diff --cached --check
```

Rerun the staged PR-documentation preflight from the verification section with
the current staged work order and its referenced committed/staged artifacts,
then use normal active hooks. Keep the original hosted observer and its deadline
through the correction; do not start another observer or retry CI without a
separate grant. Correction results: hook body matched the original apart from
the export and imports; the `browser-locales` rendered suite passed 7 tests and
the existing `browser` panel suite passed 4 tests. Scoped ESLint and canonical
web typecheck with the 6 GiB heap passed. Staged ratchets, coverage preflight,
catalog/spec/diff checks, and normal hooks are delivery checks for this correction.
