---
id: "01-session-owned-file-search"
title: "Keep file search with its committed owner"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-COMPOSER-FILE-SEARCH-OWNERSHIP-001
acceptance_criteria:
  - AC-UI-COMPOSER-FILE-SEARCH-OWNERSHIP-001.1
  - AC-UI-COMPOSER-FILE-SEARCH-OWNERSHIP-001.2
  - AC-UI-COMPOSER-FILE-SEARCH-OWNERSHIP-001.3
  - AC-UI-COMPOSER-FILE-SEARCH-OWNERSHIP-001.4
  - AC-UI-COMPOSER-FILE-SEARCH-OWNERSHIP-001.5
  - AC-UI-COMPOSER-FILE-SEARCH-OWNERSHIP-001.6
system_design:
  - ../../specs/ui/system-design/composer-file-search-ownership.md
---

# Task 01: Keep file search with its committed owner

## Summary

In the same mounted real composer, a repeated query after committed session
navigation must read and show the new session's files. Correct only local file
cache identity and settled-owner glue, backed by faithful actual-component and
wire regression tests. ROOT's later explicit interrupt admitted this work order
with exclusive local-heavy ownership, retained through publication and joins.

## In scope

- Private `fetchFileResults` / `useMentionItems` glue in existing
  `apps/web/components/task/chat/tiptap-input.tsx`.
- New `apps/web/components/task/chat/tiptap-input-file-search-ownership.test.tsx`:
  real exported component, actual StateProvider/createAppStore, real editor,
  mention suggestion/menu, real API/WebSocketClient with only transport deferred.
- Source/consumer audit limited to the chain and boundaries in the paired design.
- Exact regression/control matrix, scoped checks, actual receipts, normal delivery
  and accurate draft/active/current/pending/done/implemented lifecycle updates.

## Out of scope

All [plan exclusions and resource/hosted gates](plan.md) apply. In particular,
do not touch `hooks/use-inline-mention.ts`, generic suggestion architecture,
backend/WS/SDK/store, draft/writer/focus, layout/mobile/touch/navigation/copy or
ranking/recency. No proof replay, broad local audit, Go/browser/E2E/build/mock
server, new agent/task/tab/session/model or global runtime/cache change.

## Acceptance

1. All `.1` through `.6` criteria have actual real-component/wire evidence,
   including rendered new-session rows and retired-settlement/current reuse
   controls. New permanent causal regression fails for the missing beta read
   before the correction and passes afterward; positive alpha case also passes.
2. Production diff stays in private file-search/cache glue, preserves public
   input/return/insertion, full exact query/limit20, current fallback/non-file
   sources/ranking, and committed lifetime with independent-editor behavior.
3. All task checks have joined terminal receipts and owned PID/group cleanup;
   package statuses and actual PR-path documentation coverage are accurate.
   Hosted and merge gates from the plan remain separately enforced.

## Sequential execution

1. After ROOT's later interrupt, read actual four files and task plan/user edits;
   record implementation admission and exclusive local-heavy identity. Inspect
   current HEAD/source/dependency/consumer baseline read-only without replaying
   ROOT proof. Checkpoint conflicting source changes rather than rebasing for
   main movement. Mark this order `in_progress` only after admission.
2. If dependencies are missing, one conditional pinned frozen installation from
   `apps/`; preserve lockfile and shared caches. Read `/tdd`. Use Node24 existing
   command-local Bash login=false PATH, no global runtime edits.
3. Write the targeted permanent suite from current fixture dependencies. Real
   provider loads prompts, real editor public imperative `clear` / `insertText`
   admits `@`; only wire is replaced. Restore previous client/global/timers,
   settle owned deferred IDs, cleanup component and disconnect after each case.
4. Run causal RED once with one worker, Node4GiB and original120s bound. Require
   actual current positive plus causal regression failure, not setup/timeout or
   private/copied predicate. Capture actual receipt; do not weaken rendered-row
   or exact wire assertion to get a pass. Unknown/transport/resource failures
   checkpoint ROOT with failed receipt; no auto recovery.
5. Implement the minimal structured full session/query completed cache and
   committed owner/latest lookup guards. Guard cache and returned file candidates.
   Preserve exact API and non-file fallback. No speculative pending dedup/shared
   coordinator. Run scoped GREEN and meaningful controls, then scoped static
   gates serially. Repair own fixture/lint issues only with affected reruns.
6. Reconcile results and lifecycle once all required local gates pass. Promote
   requirement to `active`, design to `current`, order to `done`, plan to
   `implemented` only when their local contract is actually implemented and
   verified; unresolved design mismatch stays truthful. Hosted delivery is still
   pending until independently proven, and final task completion awaits merge.
7. Retain local-heavy through normal active hooks, new commit, push, READY PR
   and exact publication verification. Return it only after every retained local
   and publication handle is actually joined and owned PID/groups are gone,
   BEFORE starting the hosted collector. Then apply the plan's hosted gate and
   ROOT-only merge release. Do not self-admit hosted reruns/merge or discard
   collector handles. Preserve platform worktree, dependencies, evidence and
   ROOT proof through archive release.

## Regression matrix

Use actual outbound request IDs and distinct paths, scoped to each mounted menu.

| Case | Required observation |
| --- | --- |
| Current alpha positive | Exact alpha/queryunique/limit20; unique-alpha.ts rendered |
| Completed alpha to beta, same query | Same editor DOM instance; beta request and unique-beta.ts rendered, alpha absent |
| Same-owner repeat | Completed current path shown with no additional request |
| Query controls | Different query reads; empty and literal __empty__ distinct; exact text/limit20 |
| Old success before beta success | No old file in beta lookup; beta then renders and reuses its own entry |
| Old success after beta success | Beta entry survives; reopen same beta query proves reuse and beta row |
| Old success while beta pending | Old cannot become beta cache; beta response proves genuine files |
| Old rejection after/before beta success | No cache clearing/poisoning; current beta reuse remains valid |
| Same-session different-query races | New lookup wins cache publication; superseded reply cannot poison later reuse |
| Current cache hit while older miss pending | Hit retires old lookup; later old reply cannot replace completed entry |
| Null and return transitions | No null RPC/files, null-to-B fresh; A-to-B-to-A cannot revive original A work |
| Ordinary rerender / two editors | Same-lifetime reuse; independent files and owner retirement |
| Unmount / StrictMode | No retired publication, no unhandled rejection/timer/client leak; new lifetime usable |
| Current fallback/non-file controls | Empty success reusable; error not cached; no-client fallback; task/Plan/prompt ranking preserved |

Retired settlement is qualified to returned file paths and reusable cache, not
all generic menu callbacks. Do not assert unsupported automatic open-menu refresh
or submission/draft policy. If broader popup lifecycle becomes causally necessary,
persist the evidence and END WAITING for ROOT before changing the scope. A faithful
keyboard input control is optional strengthening within the same fixture.

## Verification

Only the light specification/reference/whitespace commands below are admitted
in the design turn. Everything involving pnpm or implementation waits for release.
Run commands from repo root in Bash login=false; each cwd below is independent.

```bash
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
python3 scripts/list-docs.py specs --system ui --text composer-file-search-ownership --format paths
git diff --check -- docs/specs/ui docs/plans/composer-file-search-ownership
git status --short -- docs/plans/composer-file-search-ownership
```

After later exclusive release, set command-local PATH to the existing Node24:

```bash
export PATH="/home/jcfs/.nvm/versions/node/v24.18.0/bin:$PATH"
node --version
if [ ! -d apps/node_modules ]; then
  (cd apps && corepack pnpm@9.15.9 install --frozen-lockfile)
fi
```

RED and GREEN use this same exact targeted command, with external owned process
watchdog original120s and Node4GiB. Record the start, absolute cutoff, subprocess
PID/group, log, retained exec handle, exit, actual join and process-gone evidence.
The runner must kill/join only its own group on timeout, then checkpoint ROOT.
Keep tests driven by causal transport replies; fake timers are fixture controls,
not a replacement for outer wall-clock timeout. Do not discard a session handle.

```bash
(cd apps/web && NODE_OPTIONS=--max-old-space-size=4096 corepack pnpm@9.15.9 exec vitest run --project browser-locales components/task/chat/tiptap-input-file-search-ownership.test.tsx --maxWorkers=1 --no-file-parallelism)
```

After meaningful GREEN, run each gate serially once under the same Node24 PATH.
Retain/join any yielded handles; no broad passing replay. Normal typecheck includes
its project pretypecheck, and no substitute tsc bypass is allowed.

```bash
(cd apps/web && NODE_OPTIONS=--max-old-space-size=4096 corepack pnpm@9.15.9 exec eslint --max-warnings 0 components/task/chat/tiptap-input.tsx components/task/chat/tiptap-input-file-search-ownership.test.tsx)
(cd apps/web && NODE_OPTIONS=--max-old-space-size=4096 corepack pnpm@9.15.9 run typecheck)
(cd apps/web && NODE_OPTIONS=--max-old-space-size=4096 corepack pnpm@9.15.9 run i18n:check)
(cd apps/web && NODE_OPTIONS=--max-old-space-size=4096 corepack pnpm@9.15.9 run i18n:ratchet)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

For documentation coverage use the actual local changed-path inventory, including
untracked artifacts, and real file contents with the existing
`.github/scripts/pr-docs.cjs` `validateCoverage` export. Validate this work order,
its sibling plan backlink, all six AC IDs in the owning requirement, and design
requirement mapping. Require `ok=true`, `errors=[]`, actual source triggering path
covered after implementation. Repeat against actual PR changed files at delivery;
never use a fabricated reduced inventory as actual PR evidence. At design, report
actual four-file result separately from prospective source-scope preflight.

The local actual-inventory preflight is dependency-free and can run during design
under Node24. Before commit it must include the real source/test diff; after PR
creation use the provider's exact-head actual PR file inventory instead of an
empty post-commit working diff.

```bash
PATH=/home/jcfs/.nvm/versions/node/v24.18.0/bin:$PATH node <<'JS'
const fs = require('node:fs');
const cp = require('node:child_process');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const tracked = cp.execFileSync('git', ['diff', '--name-status', '--no-renames', 'HEAD'], { encoding: 'utf8' })
  .trim().split('\n').filter(Boolean).map(line => {
    const [code, filename] = line.split('\t');
    return { filename, status: code === 'D' ? 'removed' : code === 'A' ? 'added' : 'modified' };
  });
const added = cp.execFileSync('git', ['ls-files', '--others', '--exclude-standard'], { encoding: 'utf8' })
  .trim().split('\n').filter(Boolean).map(filename => ({ filename, status: 'added' }));
const changedFiles = [...tracked, ...added];
const docs = [
  'docs/specs/ui/requirements/composer-file-search-ownership.md',
  'docs/specs/ui/system-design/composer-file-search-ownership.md',
  'docs/plans/composer-file-search-ownership/plan.md',
  'docs/plans/composer-file-search-ownership/task-01-session-owned-file-search.md',
];
const fileContents = Object.fromEntries(docs.map(p => [p, fs.readFileSync(p, 'utf8')]));
const result = validateCoverage({ changedFiles, fileContents });
console.log(JSON.stringify(result, null, 2));
if (!result.ok || result.errors.length) process.exitCode = 1;
JS
```

## Files likely touched

- `apps/web/components/task/chat/tiptap-input.tsx` (private file-search glue only).
- `apps/web/components/task/chat/tiptap-input-file-search-ownership.test.tsx` (new).
- `docs/specs/ui/requirements/composer-file-search-ownership.md` (lifecycle).
- `docs/specs/ui/system-design/composer-file-search-ownership.md` (lifecycle/actual design).
- `docs/plans/composer-file-search-ownership/plan.md` (status/results).
- `docs/plans/composer-file-search-ownership/task-01-session-owned-file-search.md` (status/results).

No package config, transport, store, new helper module or public docs edit is
planned. Checkpoint ROOT before an out-of-scope causal change.

## Dependencies

No preceding work order. Explicit later ROOT implementation interrupt with
exclusive local-heavy; independently proven hosted gates; later serial merge
grant. Accepted ROOT proof remains read-only and cannot be replayed.

## Risks

- Full session/query key without ownership guards still admits late poisoning.
- Ref updates during render would make abandoned work authoritative; commit-only
  ownership and effect replay require targeted actual-component controls.
- Tests must prove menu rows after beta reply, not only request counts.
- Generic TipTap popup cancellation is deliberately not promised by this scope.

## Parallelism

`sequential`. Same primary session only; no delegation or model change.

## Inputs

- [Requirement](../../specs/ui/requirements/composer-file-search-ownership.md), `.1` through `.6`.
- [Design](../../specs/ui/system-design/composer-file-search-ownership.md), all sections.
- [Plan](plan.md), evidence identities and resource/hosted/merge checkpoints.
- Current `TipTapInput`, provider/editor/menu/API/client and consumer source.
- Existing `apps/web/hooks/domains/session/use-session-search-ownership.test.tsx`
  deferred-wire cleanup pattern, without copying its different search policy.
- Read-only accepted ROOT archive/receipt/classification/log/current-main audit.

## Results

Historical design checks passed on 2026-10-06 at HEAD
`8f533fc85a1b5f0658bae75905fc64ff80c13a87`: catalog validated 351 decisions /
1373 specifications; all specification files passed lint; the new pair is
catalog-discoverable. Dependency-free Node24 documentation coverage reports the
actual four-file design inventory exempt, errors[], and a separately labelled
prospective six-path implementation preflight covered, errors[], with this work
order's requirement/design/AC references accepted. The prospective result is not
actual production or PR coverage. Receipt:
`/tmp/kandev-child46-file-search-design-coverage.json`.

After ROOT's later implementation release, one frozen dependency install passed.
Final owned production correction and 21 real-component/provider/editor/menu/
API/WebSocketClient cases satisfy `.1` through `.6`; only deferred wire transport
is substituted. Actual candidate menu files and exact wire identity remain the
causal regression assertions. Retired empty/success/error, current cache reuse,
query sentinel, null/ABA, independent editors, StrictMode/unmount and current
fallback controls pass. Fixture synthetic drafts are saved/restored through real
storage APIs, and actual editor teardown is flushed before clearing owned timers.

| Command / evidence | Actual result |
| --- | --- |
| Conditional pnpm9.15.9 frozen install | PASS2.266s; once only |
| Permanent scoped RED after draft-fixture repair | 8 causal failures / 11 PASS; missing beta expected2actual1; exit1/6.505s |
| Exact final Vitest suite after fixture workflowId repair | 21 PASS; exit0/6.667s; one worker; original120s; Node4GiB |
| Scoped changed-source/test ESLint and later affected-test repairs | PASS; final affected test exit0/1.403s |
| Normal pnpm typecheck including pretypecheck | PASS exit0/8.665s after own missing workflowId fixture repair |
| pnpm i18n:check | PASS exit0/9.118s |
| pnpm i18n:ratchet | PASS exit0/1.750s |

Exact receipts and input hashes are in the [plan implementation results](plan.md).
All native handles actually joined; owned process groups/watchdogs gone before
each next run. First RED attempt was owned draft-fixture contamination; the first
typecheck diagnostic was the new fixture's missing required workflowId. Neither
was represented as causal RED or resource/timeout failure. Repairs stayed within
the authorized fixture/lint boundary and reran only affected checks.

Final light docs/reference/actual six-path coverage/whitespace passed in 1.856s;
receipt `/tmp/kandev-child46-docs.json`, actual coverage inventory
`/tmp/kandev-child46-local-contracts.json`: covered/errors[], owning references
accepted. Catalog351 decisions/1373 specifications; spec lint, pair discovery,
links, untracked-aware whitespace and proof preservation passed. Native86156
actually joined, group920801/watchdog920798 gone. This local order is done.

Publication gates remain pending. No hosted run/review/CI verdict/merge has
occurred; heavy stays with child46 through normal active hooks/new conventional
commit/push/READY PR/publication joins, then returns before the hosted collector.
