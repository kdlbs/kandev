---
id: "01-settle-file-moves"
title: "Settle each file move without replacing newer tree state"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-TASK-NAVIGATION-RESPONSIVENESS-001
acceptance_criteria:
  - AC-UI-TASK-NAVIGATION-RESPONSIVENESS-001.3
  - AC-UI-TASK-NAVIGATION-RESPONSIVENESS-001.5
  - AC-UI-TASK-NAVIGATION-RESPONSIVENESS-001.6
  - AC-UI-TASK-NAVIGATION-RESPONSIVENESS-001.15
  - AC-UI-TASK-NAVIGATION-RESPONSIVENESS-001.16
system_design:
  - ../../specs/ui/system-design/file-browser-reply-freshness.md
---

# Task 01: Settle Each File Move Without Replacing Newer Tree State

## Summary

Write permanent rendered native-DnD regressions through the real production
operations, provider/store, tree, cache and subscription. Replace the existing
whole-selection optimistic move/rollback with independent confirmed publication
and one final reconciliation of affected paths through the existing refresh
owner. Keep endpoint semantics and request concurrency unchanged.

## In scope

- Captured exact mappings, accepted outcomes, failure reporting, full settlement
  despite rejection, and preservation of intervening authoritative changes.
- Immediate tree/cache publication and narrow reuse of current subscription
  guards/tickets for final affected-folder refresh.
- Faithful component regressions and targeted resource-capped verification.

## Out of scope

Backend batch API, remote rollback/atomicity, disk data-loss claims, new request
concurrency policy, global editor lifetime or writer audit, other file actions,
layout/copy/touch/navigation, cache framework/retention redesign, dependencies,
new flags/settings, full suites/browser/build/E2E, live data, workers/sessions.
Never replay/copy/import/change/delete the protected ROOT proof.

## Acceptance

1. The two named causal mixed-outcome cases fail before production changes at
   their final rendered-location assertions, with genuine accepted rename
   evidence and initial reads causally settled. After correction the complete
   regression matrix passes through actual production wiring (`.15`, `.16`).
2. Each request settles independently and every dispatched sibling is joined.
   Success updates only its still-current captured mapping against the latest
   tree; failure never restores earlier state. Reconciliation uses the actual
   subscription's current-owner/per-folder tickets and preserves newer data,
   including when a read fails. Exact collision targets, payloads and existing
   localized error feedback retain their contracts (`.5`, `.15`, `.16`).
3. The same store's retained tree shows settled outcomes while a remount's read
   is held. Current cache scope/budgets, read freshness, stable aggregate result
   and desktop/phone composition remain valid. All task-defined affected checks
   pass and results distinguish RED, GREEN and documentation evidence
   (`.3`, `.5`, `.6`, `.16`).

## Inputs and owned files

Read the [manifest](plan.md), [requirement](../../specs/ui/requirements/task-navigation-responsiveness.md)
and [File-move settlement design](../../specs/ui/system-design/file-browser-reply-freshness.md#file-move-settlement),
plus `apps/web/AGENTS.md`, `/tdd`, `/mobile-parity` and `/docs-maintainer`.

Production ownership is bounded to:

- `apps/web/components/task/file-browser.tsx`: existing native gesture consumer;
  import the production execution helper rather than grow this610-line file.
- `apps/web/components/task/file-browser-move.ts`: extracted actual move path,
  using existing `file-tree-utils.ts` functions and the exact captured mapping.
- `apps/web/components/task/file-browser-refresh.ts`: ROOT-approved corrective
  seam to invalidate affected existing FolderRefreshes tickets using the same
  changed-folder grouping, with no new registry or reads.
- `apps/web/components/task/file-browser-hooks.ts`: immediate shared refresh
  callback/current-tree ownership through `useFileChangeSubscription` and
  `useFileBrowserTree`, including stable result dependencies.

Permanent regression ownership:

- `apps/web/components/task/file-browser-move-settlement.test.tsx`.
- `apps/web/components/task/file-browser-move-settlement.test-helpers.tsx` only
  if fixture size requires extraction; fixture only, used by that real suite.
- Existing `file-browser-render-identity.test.tsx` or immediately affected typed
  tree-hook fixtures only for necessary production-result compatibility.

Reference production `hooks/use-file-operations.ts`, `lib/ws/workspace-files.ts`,
`file-browser-tree-state.ts`, `file-browser-tree-cache.ts`,
`file-browser-refresh.ts`, `file-tree-utils.ts`, `StateProvider` and toast/
tooltip providers; these are real test dependencies, not targets for redesign.
If implementation requires a change outside this immediate boundary, persist
root-cause evidence and checkpoint ROOT before expanding. Preserve others' edits.
No test-only production export, cache reset, mocked internal owner or guard mirror.

## Regression matrix

Use describe `file move settlement`, with exact causal test names below. Use
unique task/session/environment identities per fixture and a current server-tree
model updated only by accepted transport requests. Record exact rename payloads,
request outcome and authoritative read generations separately from DOM results.

| Case | Required observations | AC |
| --- | --- | --- |
| `retains accepted file after authoritative refresh before sibling failure` | A accepted; actual event starts real tree reads; destination A rendered/source A absent BEFORE B is released false; final A stays at destination and B at source | `.15`, `.16` |
| `retains accepted file without a workspace event` | Independent rename accepts A, B false; no notification; real final affected reads settle; mixed paths and failure toast correct | `.15` |
| Completion reversal and accepted source reversal | Failure first and success first; A or B accepted; no result/order dependence or abandoned pending sibling | `.15` |
| Rejected transport | Real rename request rejects, actual `useFileOperations` returns false and emits existing per-file error; accepted sibling and batch feedback preserved | `.15`, `.16` |
| All success / all failure | All accepted paths at exact destinations; all failed unchanged absent newer data; no false batch success or accepted-path undo | `.15` |
| Unrelated authoritative changes | Actual event/read introduces a row, removes a row and changes surviving metadata before sibling failure; all changes survive final settlement and cache readback | `.16` |
| Newer affected-path data | Replaced/removed/relocated source or occupied destination from an actual read is never overwritten by captured data; authoritative final reads establish locations | `.16` |
| Final read ordering / failure | Hold settlement read, publish newer same-folder event reply, release old read; old data cannot win. Rejected read preserves live accepted/unrelated rows, without inventing authoritative absence | `.5`, `.16` |
| Exact collision mapping and directory descendants | Existing destination basename yields one captured deduplicated target; payload and final row agree. An accepted directory keeps loaded descendant path prefixes, including after a sibling settles first and final reads reject | `.15`, `.16` |
| Actual retained tree | Remount ONLY browser within same providers/store, hold new root/folder replies, observe reconciled paths before reads settle; release all afterward | `.3`, `.16` |
| Current owner compatibility | Existing context guards remain used; hold settlement across disposal or replacement context and prove no replacement tree/cache publication | `.5` |

Mount real `FileBrowser` with real `useFileOperations` callbacks under
`StateProvider`, `ToastProvider` and `TooltipProvider`. Use actual selection
clicks and native `dragStart`/`drop` `DataTransfer` payload handling. Mock only
external WS/API endpoints, stable connection subscription and bounded DOM
geometry necessary for the real virtualizer; restore geometry after every test.
Use the actual `session.workspace.file.changes` listener and real production
refresh path, not direct test calls to `applyFileChanges` for the new regressions.
No hook/tree/store/cache/toast/tooltip/multi-select mocks, test predicate mirror,
source-text assertion, or missing-wire-method failure qualifies.

Settle the initial root/destination reads and active session hydration before
selection; do not classify an unsettled initial-read control as RED. Causal
waits name request/event publication. Every deferred request, read and fixture
promise must settle; clear listeners, unmount consumers and join test-owned
timers in teardown. Do not add arbitrary sleeps or leave a promise behind.
Transport rejection represents unknown remote outcome: do not infer disk
rollback from `false`; exercise current authoritative reconciliation.

## Implementation sequence

After the later explicit ROOT implementation grant and local-heavy lease:

1. Re-read actual primary/plan and user-edited artifacts. Record current HEAD,
   source changes from the audited base and lease/handles. Mark only Task01
   `in_progress`. Do not reset/rebase to old proof or make a synthetic merge.
2. Independently author the real rendered fixture and two named causal cases.
   Run the anchored RED command. Record actual assertion failures and accepted
   wire evidence; fixture/provider/initial-read errors are not RED.
3. Implement the design's local production helper and shared refresh callback.
   Capture exact mappings once. Leave pending rows at their last known paths;
   publish still-owned confirmed success functionally; failures do not mutate
   tree state. Catch each callback failure and settle all siblings before one
   final affected-path refresh. Reuse existing localized aggregate failure copy.
4. Complete the permanent matrix and affected compatibility fixtures. Run the
   single GREEN/affected command, then lint/typecheck/i18n/docs/coverage once.
   Correct only causal own fixture/compiler/lint defects and rerun affected
   checks; do not replay passing groups. Record actual results and all joins.
5. Mark Task01 `done` and manifest implementation complete only after all checks
   and traceability pass. Existing active/current spec statuses remain correct.
   Later publication and separate ROOT merge grant follow live-plan gates.

## Resource execution contract

ROOT released implementation after actual design handoff and four-file review
on 2026-10-08. The one GLOBAL local-heavy79 lease is granted; MERGE NONE.
Commands run serially from repo root in `/bin/bash`,
`login=false`, after the explicit single GLOBAL local-heavy lease.
Pin existing Node24.21.0 in PATH; project-pinned pnpm9.15.9 only. One conditional
frozen install from apps is permitted only when local dependencies are absent.
No alternate Node download, dependency symlink, lockfile or package-manager path.

Command blocks are payloads for the owning bounded runner. For EACH process
persist original native handle, wrapper and child PID/group, command/cwd,
UTC start/cutoff/log, wrapper versus actual exit and ACTUALLYJOIN evidence.
Use an owned group with GNU timeout/kill10s; poll original handles to actual
completion (each tool wait at most60s); verify owned groups absent. Serial
execution does not authorize killing foreign work. No lost handle replacement,
automatic retry after resource/timeout/transport/unknown/out-of-scope failures,
cache wipe, full suite/browser/build/E2E/backend work or passing replay.
Those failures persist a ROOT checkpoint and END WAITING. Routine own causal
fixture/compiler/lint corrections need only minimum changes and affected checks.

## Verification

Initialize PATH/heap for every admitted shell (do not assume shell state persists):

```bash
export PATH="/home/jcfs/.local/share/mise/installs/node/24.21.0/bin:$PATH"
export NODE_OPTIONS=--max-old-space-size=4096
node --version
corepack pnpm@9.15.9 --version
```

Version readback must be24.21.0/9.15.9. If absent/mismatched, checkpoint ROOT;
no automatic installation/retry. Conditional install at most once, only later:

```bash
if [ ! -d apps/node_modules ] || [ ! -d apps/web/node_modules ]; then
  (cd apps && timeout --kill-after=10s 600s corepack pnpm@9.15.9 install --frozen-lockfile)
fi
```

Run each payload as a separate retained process under the resource contract:

```bash
# Causal RED before production changes; exact full test-name anchor.
(cd apps/web && timeout --kill-after=10s 120s corepack pnpm@9.15.9 exec vitest run --project browser-locales --maxWorkers=1 --no-file-parallelism components/task/file-browser-move-settlement.test.tsx -t '^file move settlement (retains accepted file after authoritative refresh before sibling failure|retains accepted file without a workspace event)$')

# One final GREEN/affected run, explicit paths, one worker. No full suite.
(cd apps/web && timeout --kill-after=10s 180s corepack pnpm@9.15.9 exec vitest run --project browser-locales --maxWorkers=1 --no-file-parallelism components/task/file-browser-move-settlement.test.tsx components/task/file-browser-refresh-freshness.test.ts components/task/file-browser-apply-changes.test.ts components/task/file-browser-tree-state.test.tsx components/task/file-browser-tree-cache.test.ts components/task/file-tree-utils.test.ts components/task/file-browser-render-identity.test.tsx)

# Exact owned source/test paths; helper fixture included only if created.
(cd apps/web && timeout --kill-after=10s 120s corepack pnpm@9.15.9 exec eslint --max-warnings 0 components/task/file-browser.tsx components/task/file-browser-move.ts components/task/file-browser-hooks.ts components/task/file-browser-move-settlement.test.tsx components/task/file-browser-move-settlement.test-helpers.tsx)
(cd apps/web && timeout --kill-after=10s 120s corepack pnpm@9.15.9 exec prettier --check components/task/file-browser.tsx components/task/file-browser-move.ts components/task/file-browser-hooks.ts components/task/file-browser-move-settlement.test.tsx components/task/file-browser-move-settlement.test-helpers.tsx)

# Normal pretypecheck generates required files; inspect collateral, no build.
(cd apps/web && timeout --kill-after=10s 300s corepack pnpm@9.15.9 run typecheck)
(cd apps/web && timeout --kill-after=10s 120s corepack pnpm@9.15.9 run i18n:check)
(cd apps/web && timeout --kill-after=10s 120s corepack pnpm@9.15.9 run i18n:ratchet)

# Cheap docs gates, design and final delivery; no product tests.
timeout --kill-after=10s 60s python3 scripts/list-docs.py validate
timeout --kill-after=10s 60s python3 scripts/lint-spec-files.py --all
git diff --check
git status --short -- docs/specs/ui/requirements/task-navigation-responsiveness.md docs/specs/ui/system-design/file-browser-reply-freshness.md docs/plans/preserve-successful-file-moves
```

If an optional fixture or immediate typed test changes, add its exact existing
path to ESLint/Prettier and the affected Vitest command, record why, and inspect
actual collected paths. Never widen with wildcards or all-worker flags. Normal
active commit hooks remain enabled; formatter failure means re-stage and a new
commit, no bypass/amend. No new user-facing copy is planned; any required copy
must use existing translations or receive all seven catalogs/zh-hant generation.

The real PR-doc coverage oracle below has a60s cap and no GitHub/network calls.
It consumes actual changed/untracked files, not fabricated triggering paths.
Design legitimately reports `exempt`; direct work-order validation proves
references separately. Implementation must report `covered` for actual changes.

```bash
timeout --kill-after=10s 60s node <<'NODE'
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');
const { createRequire } = require('node:module');
const { execFileSync } = require('node:child_process');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const split = value => value.split('\0').filter(Boolean);
const changedPaths = [...new Set([
  ...split(execFileSync('git', ['diff', '--name-only', '-z', 'HEAD'], { encoding: 'utf8' })),
  ...split(execFileSync('git', ['ls-files', '--others', '--exclude-standard', '-z'], { encoding: 'utf8' })),
])];
const documents = [
  'docs/specs/ui/requirements/task-navigation-responsiveness.md',
  'docs/specs/ui/system-design/file-browser-reply-freshness.md',
  'docs/plans/preserve-successful-file-moves/plan.md',
  'docs/plans/preserve-successful-file-moves/task-01-settle-file-moves.md',
];
const fileContents = Object.fromEntries(documents.map(name => [name, fs.readFileSync(name, 'utf8')]));
const result = validateCoverage({ changedFiles: changedPaths.map(filename => ({ filename, status: 'modified' })), fileContents });
console.log(JSON.stringify(result));
if (!result.ok || result.errors.length) process.exitCode = 1;
const filename = path.resolve('.github/scripts/pr-docs.cjs');
const context = { require: createRequire(filename), module: { exports: {} }, process, console, Buffer, URL, fileContents, workOrderPath: documents[3] };
vm.runInNewContext(fs.readFileSync(filename, 'utf8') + '\nglobalThis.validation = validateWorkOrder(workOrderPath, fileContents, new Set());', context, { filename });
console.log(JSON.stringify(context.validation));
if (context.validation.errors.length) process.exitCode = 1;
if (changedPaths.some(name => name.startsWith('apps/web/')) && result.status !== 'covered') process.exitCode = 1;
NODE
```

This is read-only use of the unchanged repository validator, including its
private reference function. Do not add a test API or synthetic changed source.
If foreign edits exist, preserve them and load their real document closure
rather than filter them into a misleading PASS. Before later staging, identify
all owned/unowned paths and retain generated collateral only when actually owned.

## Mobile and public documentation

The [manifest audit](plan.md#mobile-e2e-and-public-documentation) applies the
state/data exception. Desktop/phone share tree settlement/cache without changed
markup, geometry, touch, scroll, copy or navigation. No new composition preview
or mobile Playwright run is required. Public docs describe the unchanged Files
entry points and usable rows; internal intent/results belong in this package.
No public docs edit or public validator is needed for this correction.

## Dependencies and parallelism

Dependencies: None. Parallelism: `sequential`, SAME primary, no delegates,
recursive tasks, new sessions/tabs or model changes. ROOT owns admission and
local-heavy/publication/observer/merge gates in the live task plan and manifest.
No completion claim until actual merged evidence and joined owned cleanup.

## Risks

Preserve exact collision mapping and loaded descendant prefixes. A broader
inverse or stale-node insertion can overwrite newer authoritative data. Final
reconciliation must share event ordering, and failed reads must not erase rows.
Transport rejection is not evidence that disk stayed unchanged. Fixture initial
reads and every deferred sibling must settle for RED/GREEN to be causal.

## Original published results

Implemented the actual gesture consumer, extracted settlement helper and shared
subscription refresh callbacks. Accepted publication checks the latest source
identity, destination parent and exact captured target. It preserves untouched
sibling identities, loaded descendants and existing cache publication. False
outcomes never replace tree state; all siblings settle before one affected read.

| Verification | Actual result |
| --- | --- |
| Anchored two-case RED command above | Both causal final DOM assertions failed before production edits, after genuine accepted wire evidence and settled initial reads. RED03 actualVitest1/wrapper0. Early own wire/selection failures were NOT RED. |
| Original explicit seven-file affected GREEN command above | Six existing suites44/44 PASS. New suite had15 PASS and2 own fixture failures (asymmetric matcher/depth-one directory response); corrected only affected new suite afterward. |
| `vitest run --project browser-locales --maxWorkers=1 --no-file-parallelism components/task/file-browser-move-settlement.test.tsx -t '^file move settlement '` from apps/web, cap120s/Node4GiB | Final18/18 PASS. Includes completion orders, false/rejection and lost-reply convergence, subscription supersession, authoritative metadata/cache, read failures, exact directory collision/descendants and retired owner. |
| Directory after earlier sibling, rejected final reads | Additional RED05 failed at accepted destination DOM assertion; local helper retains untouched node identities. GREEN04 includes its PASS. RED04's missing-row click was not an assertion-qualified RED. |
| Owned-file ESLint command above plus exact fixture helper, cap120s/Node4GiB | PASS, zero warnings after own fixture size/string and typed fixture corrections. Minimum affected-path reruns only. |
| Owned-file Prettier command above plus fixture helper, cap120s | PASS. Active hooks remain required. |
| `corepack pnpm@9.15.9 run typecheck` from apps/web, cap300s/Node4GiB | PASS including normal pretypecheck generation. Own fixture completeness/ID brands corrected with real defaultState and existing ID constructors. No generated collateral changed. |
| `corepack pnpm@9.15.9 run i18n:check` from apps/web, cap120s | PASS: seven catalogs/pseudo, Trans indices, plurals, module-scope translation, public punctuation and non-JSX copy.435 existing orphan entries are advisory. No copy added. |
| `corepack pnpm@9.15.9 run i18n:ratchet` from apps/web, cap120s | PASS before and after staging:2 added +2 modified guarded files clean, including the new production/fixture helpers. Guard643 entries intact. |
| Final catalog/spec lint, actual changed-file PR-doc coverage and private work-order oracle above, cap60s each | PASS: catalog363 decisions/1437 specifications, all-spec lint, actual nine changed/untracked paths `covered`/errors[], direct work-order oracle errors[]. Diff whitespace clean. |

All commands use pinned Node24.21.0/pnpm9.15.9 and serial retained originals.
Conditional frozen install ran once because dependencies were absent. Full
argv/cwd/start/cutoff/log/native handles/actual versus wrapper exits and
ACTUALLYJOIN/group absence live in
`/tmp/kandev-child79-execution-20261008/*.json`. This receipt directory is external
evidence, not a production fixture or protected proof copy. No full suite,
browser/build/E2E/backend test, foreign cleanup or protected-proof replay.

Publication/hosted observer/merge evidence remains in the external task plan;
MERGE NONE until the separate ROOT serial grant. Mobile and public documentation
assessment above remains valid.

### Corrective review work

Greptile4215879422 identifies a valid older-read race in confirmed publication.
ROOT explicitly released its correction in this same owner/work order, including
`file-browser-refresh.ts`'s immediate ticket seam. Before any corrective test,
the original observer was ownership-verified and actually joined after the
ROOT-authorized stop (exit143, no CI verdict); all review reads also joined.

Add the meaningful real-rendered case `supersedes pre-acceptance reads while a
sibling is pending and final reads fail`. Hold actual subscribed root/destination
replies before acceptance, accept A through the actual operations transport,
release old replies while B remains pending, and then fail final reads as B
fails. Assert accepted DOM/cache paths and unrelated pending folder metadata.
Existing later-authoritative-change controls must remain valid. Invalidate only
affected current tickets before queueing accepted tree publication; do not fetch
at acceptance, blanket-retire tickets, or add a second registry/abort policy.

Exact corrective payloads (each under the retained runner, serially):

```bash
# RED cap120s, before the ticket seam changes.
corepack pnpm@9.15.9 exec vitest run --project browser-locales --maxWorkers=1 --no-file-parallelism components/task/file-browser-move-settlement.test.tsx -t '^file move settlement supersedes pre-acceptance reads while a sibling is pending and final reads fail$'
# GREEN cap180s, only immediate affected concerns.
corepack pnpm@9.15.9 exec vitest run --project browser-locales --maxWorkers=1 --no-file-parallelism components/task/file-browser-move-settlement.test.tsx components/task/file-browser-refresh-freshness.test.ts components/task/file-browser-apply-changes.test.ts components/task/file-browser-render-identity.test.tsx
# Changed source/test lint and formatting, cap120s each.
corepack pnpm@9.15.9 exec eslint --max-warnings 0 components/task/file-browser-refresh.ts components/task/file-browser-hooks.ts components/task/file-browser-move.ts components/task/file-browser-move-settlement.test.tsx
corepack pnpm@9.15.9 exec prettier --check components/task/file-browser-refresh.ts components/task/file-browser-hooks.ts components/task/file-browser-move.ts components/task/file-browser-move-settlement.test.tsx
```

These run from `apps/web` with the same pinned Node4GiB configuration. The
existing typecheck/i18n/docs payloads and caps apply. Final coverage consumes
ALL actual PR-base-to-working-tree paths (base `4d24f78d5fbabf0b36bcbaacf410f46ab2fd4c27`),
including the original committed implementation and this correction, rather
than only the corrective delta. No triggering path is fabricated.

Use cap120s/Node4GiB anchored new-case RED, then cap180s/one-worker GREEN of only
the changed rendered suite and immediately affected refresh/apply/render-identity
suites. Run exact changed ESLint/Prettier, normal web typecheck, i18n, docs and
actual all-path documentation coverage under the existing caps. Reconcile final
counts/results and live PR validation before a normal NEW commit/push. No broad
44-test replay, backend/browser/build/E2E, optional cleanup, amend or rebase.
Corrective RED06 qualifies the accepted-write/old-read ordering: initial reads
settled, actual rename A accepted and destination row present before releasing
older root/destination replies; the destination assertion then failed while B
remained pending (actual Vitest1/wrapper0). GREEN05 passed44/44 in four focused
files:19 rendered cases plus25 refresh/apply/render-identity controls. The new
case also publishes unrelated folder metadata and retains accepted DOM/cache
paths when final reads reject. This is not a replay of the original six-suite44.

Minimum test-only lint corrections split the added case into a same-prefix
suite and name repeated literals. The fourth suite crossed the enforced string
threshold, so naming its unchanged prefix was necessary for zero-warning lint;
optional naming/unused-utility cleanup remains deferred. Affected ESLint and
Prettier pass. Normal web typecheck (including unchanged generated release/changelog files),
i18n checks and staged ratchet pass (2 added +3 modified guarded files,643-entry
allowlist intact). Catalog363/1437 and all-spec lint pass. Actual ALL10 PR paths
report `covered`/errors[] and direct work-order validation errors[].
Prior results above remain historical for the first published head. External
task plan retains all original handles and gates.


Corrective local implementation and verification are complete. The preserved
external receipts record RED06 actual1, GREEN05 actual0, all affected checks
actual0, normal publication and review/observer gates. Hosted and merged
completion remain separately gated; no CI verdict is inferred from the stopped
original observer. No new dependency installation or backend checks occurred.
