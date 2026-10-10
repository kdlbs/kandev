---
created: 2026-10-08
status: done
requirements:
  - REQ-UI-TASK-NAVIGATION-RESPONSIVENESS-001
system_design:
  - ../../specs/ui/system-design/file-browser-reply-freshness.md
legacy_specs: []
---

# Implementation Plan: Preserve Successful File Moves

## Overview

Keep accepted files at their confirmed destinations when siblings fail in a
multiple-selection move. The one sequential work order adds faithful rendered
regressions and corrects settlement in the existing tree/cache publication and
subscription refresh boundary. ROOT reviewed the four-file design after the
actual design handoff and released implementation in this same primary.
Local validation is recorded below; publication, hosted evidence and the
separately authorized merge remain external task-plan gates.

## Evidence and root cause

Audited clean HEAD/main: `4d24f78d5fbabf0b36bcbaacf410f46ab2fd4c27`.
`file-browser.tsx:executeMoveFiles` optimistically moves the selection, then
`Promise.all` restores the whole captured tree on any false result or rejection.
The real operations hook normalizes both remote `success: false` and transport
rejection to `false`. Consequently an independently accepted rename A reappears
at its source when B fails, even after an actual subscribed authoritative refresh
already showed A at its destination. This is a client projection/cache defect;
there is no evidence of disk data loss, remote rollback, or atomic batch semantics.

ROOT's read-only proof lives at
`/tmp/kandev-root-file-move-partial-discovery-20261008/candidate.test.tsx`, regular
`0400`, SHA256 `2428ef1503997d7e99441da4b794e81b140318c1fe0b4247fbf6e59deb1a1df3`.
`qualified-proof.json` SHA256
`5009cc0fc355de456d7cfdfe732048033b20b73501a309b8ca998c3f6719ef8d` and
`final-local-return.json` SHA256
`68c738d230a83d95093e329e216aaae3bb65bf46ac91db60ecab9d033f939b44`
confirm original handle97487/start1f0d08 ACTUALLYJOINEDeb9c14, wrapper exit0,
actual Vitest exit1, two causal mixed RED and all-success/all-failure controls
PASS, PID group2481266 gone and temporary source absent. Earlier wire/provider
and unsettled-initial-read failures are historical, not RED. No replay, copy,
import, modification, chmod, or deletion of proof is authorized; ROOT owns release.
ROOT reports proof-base b734 to current main changed only sidebar merge/test;
the move chain is byte-identical. Permanent evidence must be independently authored.

## Ownership and assumption check

UI owns the current/retained Files-tree projection and reply publication; the
existing `REQ-UI-TASK-NAVIGATION-RESPONSIVENESS-001` and focused
[file-browser reply design](../../specs/ui/system-design/file-browser-reply-freshness.md)
own this lifecycle. Workspaces owns actual filesystem mutation/authorization.
Adjacent path-scope, editor mutation, chat context, and byte-transfer contracts
cover different outcomes. This adds only missing settlement criteria `.15`/`.16`
and their design, preserving `.3`/`.5`/`.6`. No new incident spec or ADR is needed.

Confirmed: accepted files stay, failed outcomes reconcile independently, newer
authoritative changes survive, exact targets and endpoint semantics remain.
Agent design choice within that scope: pending rows stay at the last known
location rather than optimistically moving every file and attempting inverses.
Success publishes narrowly; one final affected-folder refresh reconciles unknown
remote outcomes. No material question remains.

Related packages: [navigation restoration](../task-navigation-responsiveness/task-03-progressive-file-trees.md)
and [Files reply freshness](../file-browser-reply-freshness/plan.md) are done;
retain their scopes/results and existing folder/cache guards. This package does
not reopen restoration, search, overview, editor, or sidebar work. The large
navigation design is unchanged; its existing focused reply supplement is extended.

## Scope

### In scope

- Real multiple-selection native DnD and independent rename settlement.
- Latest-tree success publication, truthful failure feedback, affected-folder
  reconciliation through the existing owner/tickets, immediate retained cache.
- Permanent transport-only rendered regressions and exact affected validation.

### Out of scope

- Backend batch API, remote rollback/atomicity, disk data-loss claims, changing
  request concurrency, global editor lifetime, broad writer audits or retries.
- Layout/copy/touch/navigation, settings, flags, dependencies, cache frameworks,
  cache wipe, passing replay, full suites, browser/build/E2E or live instances.
- Delegates, recursive tasks, new sessions/tabs, model changes, foreign cleanup.

## Technical approach

Follow [File-move settlement](../../specs/ui/system-design/file-browser-reply-freshness.md#file-move-settlement).
Extract existing execution into production `file-browser-move.ts` only to keep
`file-browser.tsx` below the current 600-line limit (currently610). Compute exact
mappings once; settle all outcomes; publish successful mappings with the existing
functional tree setter and current-owner guard. Failed and superseded mappings
never replace the tree. Wire the settlement's affected paths into the actual
subscription's guarded `applyFileChanges`/`FolderRefreshes` path. Keep
`useFileTreeState`/`FileBrowserTreeCache` publication and budgets unchanged.
No production exports solely for tests.

## Tests

The [work order](task-01-settle-file-moves.md#regression-matrix) maps `.15`/`.16`
to `components/task/file-browser-move-settlement.test.tsx`. Required assertions
cover rendered locations, accepted remote evidence, real error toasts, exact
payloads and retained-tree readback, with initial reads settled causally.
Existing refresh/apply/cache/tree utilities/render-identity checks protect `.3`,
`.5`, `.6` and the immediately reused path. No copied predicate proves acceptance.

## Mobile, E2E and public documentation

Nearest shipped phone surface: `session-mobile-layout.tsx` renders
`TaskFilesPanel` in focused Files bottom navigation; it uses the same browser,
operations, subscription and cache. Desktop retains Dockview Files. The mobile
language guide's shared-logic rule and state/data exception apply: no composition,
touch targets, scrolling, navigation or viewport behavior changes. Targeted
real-provider rendered tests suffice; do not claim a touch DnD capability or
browser geometry proof. No new phone surface or ASCII layout preview is needed.

Existing `e2e/tests/task/file-tree-drag-drop.spec.ts` and
`file-tree-multi-select.spec.ts` cover desktop reachability; existing
`mobile-file-tree-chat-context.spec.ts` covers the phone Files action surface,
not partial moves. They are context, not newly executed race proof. No local
browser/build/E2E/full-suite run is planned or permitted without a reviewed
causal extension from ROOT. Visual verification is not run: no geometry change,
explicit bounded ROOT scope, and rendered client integration coverage targets
settlement. Screenshots/browser runs were not authorized for this state-only
correction; no visual or touch behavior is claimed.

Public docs audit searched `docs/public`, root README and `docs/screenshots.md`.
`developer-tools.md` describes Files/worktree paths;
`tasks-and-workflows.md` describes usable rows during refresh and phone Files.
Neither promises atomic moves or whole-selection rollback. This plan restores
truthful existing behavior with no public command, API, copy, workflow step, or
screenshot change. Internal docs updated; no public page or public validator is
needed. Revisit only if implementation introduces an actual public contract.

## Work orders

- [x] [Task 01: Settle each file move without replacing newer tree state](task-01-settle-file-moves.md)

Dependency order: Task01 only, `done` for local implementation/checks, sequential, same primary.
ROOT released the reviewed package after the actual design END, with local-heavy79
ONLY granted. Merge remains separately gated. Receipt:
`/tmp/kandev-root-child79-reviewed-design-20261008.json`.

## Verification results

The final corrective affected run passes44/44 in four focused files:19 rendered
integration cases and25 existing refresh/apply/render-identity controls. The
original six existing suites44/44 retain their historical passing evidence;
unchanged tree/cache/utility suites were not replayed. Two independently authored
mixed cases initially qualified causal RED. The corrective old-read ordering
also qualified causal RED after accepted wire/DOM evidence and settled initial
reads. Its GREEN proves affected older reads cannot erase accepted A while B is
pending, unrelated folder metadata still publishes, and failed final reads
preserve accepted DOM/cache state. Later-authoritative controls remain valid.

Typecheck, zero-warning changed ESLint, Prettier, i18n catalog/copy checks and
staged ratchet pass. Final catalog363 decisions/1437 specifications, all-spec
lint and actual ALL10 PR-path documentation coverage PASS (`covered`, errors[]);
direct work-order validation has errors[]. Routine lint corrections stayed in
the owned regression suite. All local originals are retained and actually joined
with owned groups absent before publication/lease return.

Task01 local implementation/checks are complete; exact commands and historical
versus corrective evidence are in the work order and execution ledger. Hosted
and merged completion remain external gates. No full suite, browser/build/E2E
or backend checks were run.

## Resource and delivery boundaries

Task `edc92b45-aaf5-4ba4-b63d-dd39246b5e2c`, session
`6d3b1a10-4425-4581-9b93-2a49985f8a76`; ROOT
`14825981-b175-411d-999a-31ddc2aa5fc3`. Preserve the live task plan's system
marker, question barrier, user edits, title ownership, handles and gates.
No child-to-ROOT interrupt, queue retries or acknowledgement waits. ROOT reads
this primary/plan directly; callbacks are optional.

After later implementation release, require one GLOBAL local-heavy lease.
Run serially with existing Node24.21.0 pinned in PATH, bash login=false and
project pnpm9.15.9; one conditional frozen apps install only if absent. Retain
original handles, PIDs/groups, UTC start/cutoff and actual exit to ACTUALLYJOIN.
Resource/timeout/transport/unknown/out-of-scope failure checkpoints ROOT, with
no automatic retry. Correct routine causal own fixtures/compiler/lint minimally
and rerun only affected checks. Normal active hooks, no bypass/amend.

After READY publication freeze SHA except a real finding; no main-only rebase,
synthetic merged testing or optional polish. Verify canonical repository
association `16026b06-bd79-47c0-aed1-dc7ca95f63d9` complete/errors[] and all five
PR automations false. Explicitly return local-heavy with all handles joined,
groups absent, clean local/upstream/remote before ONE original90m all-terminal
collector (GNU91m, kill10s, cadence60s). Retain/join that original handle;
replacement or hosted retry requires explicit ROOT grant; budgets are workflow
and job NAME across heads. All six actual required contexts and actual
Backend/Frontend/E2E parents must be terminal SUCCESS, current complete/errors[],
with zero actionable or hidden human changes-requested reviews. Authenticated
configured CodeRabbitApp347564 substantive FULL CURRENTHEAD ALLFILES automatic
review suffices; actual skip/gap permits one necessary request, no ACK/progress
or optional second wait. Ground every finding's disposition.

Only a real backend finding admits the one exact PR-base full-CHANGED fixup
check: GOMAX2/GOMEM1GiB/concurrency2/allowserial/CLI5m/GNU6m/kill10s. Docs-only
work never replays passing backend checks. Merge NONE until a separate ROOT
serial grant. Then normal expected-head squash, no admin/bypass; verify actual
mergedSHA/tree/parent/all blobs and remote inclusion, only owned cleanup/all
joins. Preserve managed worktree/dependencies/shared caches/foreign refs and
processes/protected proof. ROOT owns independent verification, archive and
absent-board readback, checksum-proved proof release and refill.

## Risks

- Recomputing destinations after dispatch can change accepted collision names.
- An inverse edit or whole-tree replacement can erase a newer authoritative row.
- Separate refresh ticket registries allow an older settlement read to overwrite
  a newer event; sharing the actual owner is required.
- A rejected transport response is unknown disk outcome, not rollback proof.
- Pending rows now remain at their last known location; acceptance/refresh moves
  them. This visible timing choice is explicit and introduces no new control.

## Approved causal review correction

ROOT source-verified Greptile4215879422 on the published implementation and
released the sole corrective local-heavy79 lease. This repairs the already
approved accepted-results contract. Before a sibling settles, an affected read
started before acceptance must not restore the old location. Supersede its
existing folder ticket at confirmed success, before tree publication, using
shared changed-folder grouping. Preserve unrelated tickets and genuine later
workspace data; keep one final reconciliation and the current/cache guards.

ROOT explicitly extended immediate ownership to `file-browser-refresh.ts`
for this affected-ticket invalidation seam and existing hook glue. The original
observer was ownership-verified, stopped and actually joined with exit143;
that stop has no CI verdict and cancelled/retried no hosted job. All review read
handles joined before corrective tests. The original18/44 verification is
historical for the first published head; corrective RED/GREEN and affected checks
passed as recorded in Task01. Optional naming/constant/unused-utility polish remains deferred.
No main-only rebase or test replay after child77's normal main merge. Publication,
new-head observer and merge still follow ROOT's external gates.
