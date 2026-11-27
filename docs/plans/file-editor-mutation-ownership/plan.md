---
created: 2026-10-03
status: done
requirements:
  - REQ-UI-FILE-EDITOR-MUTATION-001
system_design:
  - ../../specs/ui/system-design/file-editor-mutation-ownership.md
legacy_specs: []
---

# Implementation Plan: File Editor Mutation Ownership

## Overview

Keep pending save/delete replies inside their originating editor lifetime.
One sequential work order supplies faithful permanent RED evidence, the local
ownership correction and targeted GREEN validation. Design handoff precedes
permanent tests and production edits; implementation needs a later explicit
release. No agents or extra tasks/sessions are authorized.

## Evidence and specification reconciliation

Baseline actual main: `5f08b1b3e3c17bb2688d7473f2a9ec32f3eed7cb`.
The parent supplied a completed RED production-hook proof: A's deferred save
resolved after B occupied the same repo/path, and B's `originalContent` became
A's `v2` instead of `session B disk`. Parent handle 53870 is joined, exit 1.
Preserve `/tmp/kandev-editor-session-save-repro.test.ts`; do not replay it.

`performSaveFile` captures A's request snapshot, awaits transport, then rereads
global `openFiles` by repo/path alone and updates baseline/hash, LSP and panel.
`deleteFileAction` follows the same unguarded pattern after remote success:
`closeFileEditorPanel` finds whichever current pinned/preview panel matches that
key. A success after B navigation therefore closes B's panel. This second cause
is established by source trace; permanent deferred delete RED remains Task 01.

`useFileEditorEffects` clears/restores global buffers after session changes.
The existing [navigation requirement](../../specs/ui/requirements/task-navigation-responsiveness.md)
and [Files freshness design](../../specs/ui/system-design/file-browser-reply-freshness.md)
establish stale-view protection, but do not specify mutation replies and editor
replacement. The new durable pair owns reusable editor presentation only;
filesystem and task lifecycle remain with their current systems. It is not an
incident-specific repair specification. Assumption check: intended ownership,
preserved behavior, implementation release and resource limits are settled.
There is no unresolved product decision.

Related-package inventory: `task-navigation-responsiveness` and
`file-browser-reply-freshness` retain their existing work-order scopes/results;
this work adds no read-coordination or tree-refresh change to those packages.
No ADR is needed for the local guard extension; the design records alternatives.

## Scope

### In scope

- Success/failure/finally ownership for save/delete across navigation and return.
- Hook disposal, buffer reopen/replacement, panel-host replacement and originally
  absent delete panels, including replacement saving indication.
- Same-session typing, clean saves, repo independence, current failures, LSP
  save boundary and current remote reload behavior.
- Real hook/store/panel integration regressions and owning documentation.
- Tablet TaskCenterPanel's separate mutation handler and immediate tab lifetime
  and saving-state glue, with real hook/local-state deferred regressions.

### Out of scope

- Backend, API, general coordinator, new dependency or runtime/harness changes.
- General refresh races, new same-editor concurrency policy or remote rollback.
- Layout/copy/touch/navigation changes, browser/build/full-suite runs,
  speculative cleanup and foreign process/cache/worktree changes.

## Technical approach

Task 01 owns `hooks/use-file-save-delete.ts`, immediate `use-file-editors.ts`
glue, `FileEditorState` and file-state actions only as needed for stable buffer
incarnations. The paired design defines visit/buffer/host ownership, per-owner
pending markers and guarded remote-hash publication. Preserve public editor
action signatures, repo-scoped panel IDs and `savingFiles` Set consumption.

The authorized review correction extends the same rule to tablet file tabs in
`task-center-panel-restoration.ts`, `task-center-panel-file-tabs.ts` and the
immediate TaskCenterPanel consumer. Run only the newly affected tablet suites
after faithful navigation save/delete RED; no unchanged Dockview replay.

Use a new `hooks/use-file-editors.mutation-ownership.test.tsx` with real
`useFileEditors`, AppStore/StateProvider and `useDockviewStore`. Mock transport
and external services, not the state actions under test. Panel doubles must
emit remove events so the real hook's removal/persistence wiring is exercised.
Retain/adapt the existing repo-threading tests. A small
`lib/state/dockview-file-state.test.ts` proves installation versus update lifetime.

## Tests

| Criteria | Permanent evidence in mutation-ownership hook suite |
| --- | --- |
| `.1` | A save success/rejection/exception after B; A-B-A return; unmount; no stale toast/LSP/panel or tab persistence write |
| `.2` | Close/reopen identical key; replaced buffer/API; pending delete with initially absent editor |
| `.3` | Deferred save after v3 typing keeps v3 dirty, baseline v2/hash and LSP `(v2,v3)`; clean save clears dirty title |
| `.4` | Deferred pinned and preview delete after B survives; current successful delete emits removal and drops only owned buffer; current rejected/throwing mutations preserve editor and report errors |
| `.5` | Same path in two repos remains independent; A finally cannot clear B pending spinner; reopen does not inherit A spinner |
| `.6` | Current remote reload; deferred local hash after retirement preserves replacement editor |

The permanent navigation-save and navigation-delete cases must fail on the
uncorrected production implementation before GREEN. Use selected RED cases once;
do not replay parent proof or passing prefix suites. Cover all new/modified tests
in one targeted GREEN invocation with one worker and a bounded Node heap.

## Mobile and rendered verification

The state/data exception in `/mobile-parity` applies: no layout, navigation,
touch, scrolling or viewport-dependent interaction changes. Existing desktop
Dockview and phone focused Files/document compositions remain intact. Tests
exercise the shared publication boundary; no new ASCII composition, Playwright
test or browser build is warranted for this correction.

## Work orders

- [x] [Task 01: Bind mutation completion to its editor owner](task-01-bind-mutation-owner.md)

## Verification results

Initial implementation completed after the later explicit release. One frozen pnpm
installation completed (923 packages, lockfile unchanged). The permanent RED
selected only navigation save and pinned/preview delete: three assertion
failures proved baseline/hash corruption and replacement-panel removal. One
additional selected RED proved replacement-host spinner inheritance (expected
zero, received one). The final GREEN command in Task 01 passed six files and
57 tests, including 24 real hook/store/panel cases and the buffer-lifetime test.

Changed-file lint, typecheck, i18n check and new-code ratchet passed on the final
implementation. The typecheck generators left no generated-file diff. No new
copy, viewport markup, browser build, E2E or broad suite was added. Design checks
passed 343 decisions/1319 specifications and 36 spec-linter tests; final docs and
actual-diff coverage results are recorded in Task 01. All local command handles
are joined. Hosted PR/review/merge evidence and delivery cleanup are tracked in
the live Kandev task plan; local completion does not assert those future gates.

The parent subsequently authorized the tablet consumer omission identified in
PR 4177. Two permanent production-hook navigation regressions failed before
its correction; only the newly affected tablet suites then ran (three files,
27 tests passed, including 14 ownership cases). Tablet changed-file lint and
typecheck passed. Three valid test/requirement precision findings were also
corrected. Task 01 records exact commands/handles; no unchanged Dockview replay,
second install, optional polish, browser/build or broad suite was performed.

## Documentation and delivery

Internal docs only: no commands, APIs, labels, configuration, screenshots or
operator workflow change. The public editor capability remains the same.
Use actual PR coverage evaluation with the changed work order and linked pair.
Normal hooks/checks precede ready PR publication; one owned `scripts/pr-await`
monitor must be joined. Current-head configured full semantic review, required
hosted gates and finding disposition precede expected-head squash. CodeRabbit
App 347564 full review is sufficient under caller authorization. Do not rebase
only for advancing main or run a duplicate optional review.

Completion requires actual GitHub MERGED SHA/content/remote verification and all
owned handles joined/cleanup complete. Parent receives queued notifications at
design handoff, PR publication, blockers/recovery and final verified cleanup;
parent owns archive and the next child.

## Risks

- StrictMode and multiple hook consumers must retire only their own action
  lifetime; global restoration/persistence behavior must remain intact.
- Whole-object or hash equality would mishandle typing or identical reopen.
- An unguarded finally could hide a replacement save's pending indication.
- Remote success remains real even when its originating UI consumer retires.
