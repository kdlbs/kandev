---
created: 2026-10-13
status: done
requirements:
  - REQ-UI-FILE-MOVE-SELECTION-001
system_design:
  - ../../specs/ui/system-design/file-move-selection.md
legacy_specs: []
---

# Implementation Plan: Directory-Covering File Moves

## Overview

The one sequential work order normalizes move roots before destination naming
and dispatch, with mounted command regressions and real directory hierarchy
evidence. Local implementation and checks are complete. Hosted delivery state
and current instructions are maintained in the external task plan.

## Evidence and assumption check

Clean branch `feature/move-selected-folder-a81`, HEAD
`7d1adc8976aac9f5f27628f9474d8614d5f0e498` at design start.
Source inspection confirms `useDragAndDrop` forwards all selected paths;
`executeMoveFiles` computes one target per loaded selected node and concurrently
renames them. A folder and its child therefore become separate mutations.

ROOT supplied two causal mounted RED cases for both selection orders and three
passing folder-only/child-only/disjoint controls, with five captured actual WS
rename commands replayed using native `os.rename` on isolated filesystems.
Child-first flattened the child into the destination; folder-first produced
ENOENT for the redundant child operation. This is captured-command plus native
filesystem replay evidence, not live backend RPC or browser E2E evidence.
ROOT's protected `/tmp/kandev-root-nested-file-move-discovery-20261013-y5wwwb4h`
is reference-only: do not read, copy, replay, change, or clean it. Supplied
qualification reports the implicated frontend/backend blobs identical between
tested `c4e09a9f525043c613ff4596ff406c237d7f1c0e` and design HEAD; we do not
reopen the prior child or rerun its evidence.

Confirmed: any selected directory covers all selected descendants at every
depth, independent of selection order. Independent root order, segment
boundaries, directory collision rules and individual feedback stay intact.
Verified: one production coordinator caller and one target-computation consumer;
unused `moveNodesInTree` is outside the active command path. Chosen boundary:
private normalization in `file-browser-move.ts` before `computeMoveTargets`.
No material unresolved question.

UI owns selection-to-command construction; Workspaces still owns filesystem
authority. Adjacent path-scope, keyboard-scope, transfer, file-entry-mutation and
reply-freshness contracts do not specify directory coverage. Add the smallest
[owning requirement](../../specs/ui/requirements/file-move-selection.md) and
[design](../../specs/ui/system-design/file-move-selection.md); do not extend
task-navigation responsiveness `.15`/`.16` for this different failure.

Related settlement and reply-freshness packages are completed compatibility
inputs. Keep their statuses, tests and historical results intact. No linked
package is reopened. No ADR is needed for a private normalization step within
the existing coordinator.

## Scope

### In scope

- Directory-covered move roots before naming/dispatch, preserving root order.
- Independent mounted selection/real operation/WS regressions and minimal
  fixture count seam; retain every existing settlement test.
- One meaningful desktop backend/disk RED and final GREEN, serial builds,
  one worker, retries zero, actual task checkout and sentinel bytes.

### Out of scope

- Bulk API/transaction, backend policy or `os.Root` changes; ACK/reply/tree races.
- Same-folder mixed-selection repair; delete/rename/upload companions; source,
  path, selection or lifetime hardening; layout, copy, screenshots, phone features.
- New dependencies/lock changes, installs/builds/heavy during design, full local
  suites, unsolicited audits, foreign cleanup, ROOT evidence or archived tasks.

## Technical approach

Implement [effective-root normalization](../../specs/ui/system-design/file-move-selection.md#effective-root-normalization)
inside `apps/web/components/task/file-browser-move.ts:executeMoveFiles`.
Collect selected inventory directories before a stable strict-ancestor filter.
Pass effective roots to existing `computeMoveTargets`; all naming, captured
nodes, requests, result publication and refresh naturally use those mappings.
Leave `file-browser.tsx`, target naming, unused `moveNodesInTree`, operations
hook, transport, and backend logic unchanged unless an actual scoped finding
requires review. Do not extract a new production test API.

## ASCII UI preview

UI-01: Desktop Files, expanded source selected with its child, drop on `dest`.
Before follows source and ROOT-supplied command evidence; outcome depends on
which independent rename reaches disk first.

```text
BEFORE: Files pane                 AFTER: Files pane
[x] src/                          dest/
  [x] a.txt                         src/
dest/                                a.txt
Drop -> two renames               Drop -> one src -> dest/src rename
dest/a.txt + incomplete src        Relative subtree and bytes retained
OR src moves + child ENOENT
```

UI-02: Phone Files bottom-nav entry, shipped browse/open composition.

```text
BEFORE                            AFTER
[Files header / actions]          [Files header / actions]
v src/                            v src/ (or current moved hierarchy)
  a.txt        [...]                a.txt        [...]
[bottom nav: Files]               [bottom nav: Files]
Tap a.txt -> file viewer          Tap a.txt -> file viewer
[path] [Close]                    [path] [Close]
Close -> Files                    Close -> Files
```

Phone is a focused list or viewer, not simultaneous squeezed panes. Existing
`file-tree-scroll` owns tree scrolling; header/bottom nav retain existing
clearance and safe areas, and touch actions remain explicit. The curated task
layout exemplar and actual `session-mobile-layout.tsx` provide the composition.
Only hierarchy/command semantics change (AC `.1`-.5); anatomy and phone entry
value remain (AC `.6`). Spacing is illustrative. No new touch DnD promise.

## Tests

`components/task/file-browser-move-selection.test.tsx` is the primary new
contract owner. See the work order's literal command matrix for `.1`-.5.
Use the actual mounted browser, real selection hook, real operations and WS
helper. Only external transport and necessary DOM geometry are substituted.
Existing settlement coverage protects individual outcomes and `.15`/`.16`;
responsive component checks protect the existing phone wiring. Expected commands
and counts must be independently specified, not computed by the implementation.

## E2E tests

Add `selected directory carries selected and unselected descendants` in
`e2e/tests/task/file-tree-drag-drop.spec.ts`, `chromium` project. Use child-first
selection, native HTML5 events, observed real requests/replies and on-disk
hierarchy/bytes in the actual task checkout. This distinguishes real backend
hierarchy from the fixture's synthetic server tree (AC `.1`, `.2`, `.5`).
Run this one scenario causally RED before the correction. After the final edit,
run the full owning spec including its existing file-only and self-drop controls
GREEN. Each run uses fresh managed production assets, one shard/worker, no retry.

Mobile-parity's shared state/data exception applies. No viewport-dependent logic
or touch behavior changes; mounted regressions and
`file-browser-responsive.test.tsx` are the bounded mobile validation. Existing
`mobile-file-viewer.spec.ts` opening/closing cases substantiate shipped value,
not move coverage, and are not scheduled for replay. No screenshots/layout work.

## Work orders

- [x] [Task 01: Normalize effective move roots](task-01-normalize-move-roots.md)

Task01 only, sequential.

## Verification results

The independently authored mounted RED had ten causal failures, including both
selection orders dispatching two requests instead of one, with three passing
folder-only/child-only/disjoint controls. The single fresh desktop RED reached
real backend replies and passed hierarchy/byte assertions before failing on the
extra child request; it was not a setup or timeout failure. Both original runs
were joined before the five-line production correction.

Final affected mounted GREEN: 54/54 across five suites, including 13 new selection
cases and all existing settlement coverage. Fresh full owning desktop GREEN:
3/3, one worker and retries zero. The new scenario observes exactly
`src -> dest/src`, a successful real response, source absence, exact selected
and unselected nested bytes, intact destination sentinel, no flattened child,
and rendered nested entries. It uses the resolved actual task checkout.

Required typecheck passed once with the bounded process-local heap. Scoped
zero-warning ESLint, final Prettier, i18n catalog/copy checks, and staged ratchets
passed. The i18n ratchet reports 0 added/2 modified eligible files; the staged
new `.test.tsx` and E2E `.spec.ts` are excluded from that copy scan, not claimed
as scanned. The sleep ratchet reports 0 added/1 modified E2E file clean. One
frozen install reused existing cached dependencies and changed no lockfile.

Final catalog validates 381 decisions/1591 specifications; full-spec lint passes.
The work order records exact commands and cross-reference/diff gates. Design
future-production coverage also passed for nine anticipated paths; final actual
path coverage is recorded in the external execution ledger before commit.
No unchanged validator unit suite, full local product suite, mobile E2E,
screenshot capture or backend policy change was added.

Original command metadata/logs, native join identities, RED qualification,
desktop GREEN request/reply JSON and current-owned cleanup:
`/tmp/kandev-move-selected-folder-design-20261013-6stsdtfv/`.
All validation groups are empty; the observed desktop fixture/backend/browser
processes are no longer live and its temporary root is absent. Managed parent
processes and foreign resources were outside cleanup ownership and untouched.

## Documentation impact

Public-doc inspection covers `docs/public`, root README and screenshot catalog.
This fixes existing move construction without changing a public command, API,
workflow step, terminology or screenshot. Speculative design stays internal;
no public page edit or unchanged public-validator unit run is planned.

## Execution and delivery

Use the existing Node24.18.0/corepack pnpm9.15.9 and Go1.26.0 toolchains for the
bounded, serialized commands in the work order. A fresh worktree gets one frozen
apps install only when its normal dependencies are absent; no new dependency,
lockfile, or tracked heap configuration is needed. Complete the final checks
and record results/statuses before committing and publishing through the
repository's ordinary commit, push, PR and PR-fixup skills. Mutable session,
resource-grant, hosted review and merge instructions remain in the external
task plan and execution checkpoint.

## Risks

- Filtering against an incomplete directory set makes child-first selection fail.
- Plain string prefix comparison wrongly suppresses `src-other`.
- Normalizing after target naming lets covered children consume collision names.
- Synthetic tree relocation alone cannot prove actual disk hierarchy; desktop
  E2E must inspect the materialized task checkout and bytes.
- Permanent fixture count defaults must preserve all settlement cases.
