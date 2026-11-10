---
created: 2026-10-21
status: done
requirements:
  - REQ-UI-FILE-EDITOR-MUTATION-001
system_design:
  - ../../specs/ui/system-design/file-editor-mutation-ownership.md
legacy_specs: []
---

# Implementation Plan: Save and workspace observation reconciliation

## Overview

Keep a newer observed agent snapshot accessible after an older save receipt.
The parent authorized bounded implementation without a separate design handoff.
One sequential work order covers reconciliation and user-flow proof.

## Scope and cause

`publishSavedFile` clears all remote state on successful settlement, even after
`syncOpenFileFromWorkspace` has published a different agent snapshot. The actual
hook/store regression fails on the missing marker/content/hash on base
`21949f6362e6830b9fb9162d2ca6ba41c5900331`. This proves local settlement ordering,
not a production data-loss incident or arbitrary backend write chronology.
Own only save/refresh/reload publication, tests and associated docs. Exclude
backend overwrites, API/schema changes, general file-action races and panels.

## Technical approach

Use the transient workspace observation object described in the owning design.
Capture at save admission, recheck current state at settlement, distinguish an
independent observation from an echo/preexisting warning, and preserve existing
session/visit/incarnation/host/repository ownership and disk/live LSP semantics.

## Tests

`hooks/use-file-editors.save-sync.test.tsx` protects AC-UI-FILE-EDITOR-MUTATION-001.10
with actual sync production, hook and store, deferred transport and real hashing.
Cover distinct and matching updates, preexisting conflict, reverse ordering,
typing, failure, accepted clean observation and reload preparation. Existing
ownership/sync tests retain AC .1-.9 coverage.

## E2E tests and mobile parity

Desktop: a deferred real save response, real agent disk edit, actual tab sync,
retained Reload and explicit reload to agent text. Phone: existing
`task/mobile-file-viewer.spec.ts` primary Files opening flow. Phone and coarse
pointer tablet use separate viewers and do not use Dockview background sync.
This state-only change preserves all existing controls, composition, touch,
scrolling and navigation. No ASCII preview is needed for unchanged markup.

## Work orders

- [x] [Task 01: Reconcile save receipts](task-01-reconcile-save.md)

## Verification results

- RED: actual-hook/store test failed after late save response cleared the newer
  marker/content/hash (1 failed, 24 skipped). Deferred Reload preparation also
  failed by applying old text over a newer sync snapshot (37 passed, 1 failed).
- GREEN: six targeted suites passed, 88 tests. Original ownership scenarios
  reuse extracted fixtures without changing their assertions.
- Typecheck passed with `NODE_OPTIONS=--max-old-space-size=8192`; the default
  4 GiB heap exhausted before completion. The first increased-heap run found a
  missing `path`/`size` in the new mock response; corrected and reran successfully.
- Changed-file ESLint with zero warnings and Prettier check passed.
- Managed desktop E2E built backend, production Vite assets and fixture plugin.
  The first browser run reached Reload but hit a task-title/Save-button selector
  ambiguity. A narrowed Save locator passed (1 test, 7.3s) using that fresh build.
  It held the real successful save receipt, verified submitted bytes on disk,
  typed more, wrote distinct agent bytes, activated the real sync producer,
  released the receipt, then used Reload and verified displayed and disk bytes.
- Sequential mobile E2E reused the same fresh build and passed the existing
  All-files-to-viewer tapping flow (1 test, 5.9s). This proves the independent
  phone entry path; it does not claim phone uses Dockview reconciliation.
- Spec catalog and full specification lint passed. Public-doc unit validator
  passed 63 tests; live validator accepted 54 pages. Documentation coverage
  preflight returned `covered`, no errors. Diff whitespace check passed.
- Public docs updated: `docs/public/developer-tools.md` (how-to guidance).
- No new UI copy, API, schema, backend change, delegate or new architecture
  boundary. Parent actual-diff validation and an independent six-suite rerun passed.
  Delivery CI, semantic reviews and merge evidence are tracked in the PR and
  parent task messages; implementation completion does not waive those gates.

## Risks

An observation is evidence available to the frontend, not proof of disk write
ordering. Equal-content refresh no-ops must stay no-ops. Never retain an old
warning solely because a save was once pending; never lose successful-save proof.

## Review remediation

Greptile identified that a conflicting observation matching the original disk
bytes/hash could be mistaken for an accepted baseline. A real-hash hook/store
regression failed on the old baseline/hash and false dirty state. Acceptance now
also requires no current remote warning; existing accepted-snapshot controls
continue to pass. That first remediation passed 89 tests across six suites.
An optional review comment now documents snapshot identity at the comparison.

CodeRabbit identified shared seeded-repository contamination in the desktop
E2E. A baseline-commit assertion reproduced it. The fixture now creates its
file only in an explicitly selected Worktree executor checkout, verifies
source/task paths differ,
and asserts the shared source commit and status remain unchanged at cleanup.
No hard reset or other mutation of the shared source checkout is needed.

Remediation typecheck and zero-warning changed-file lint passed. The production
build was refreshed after the acceptance correction. The isolation assertion
initially caught the default Local executor's shared checkout; explicitly
selecting the existing Worktree profile then passed the actual regression,
screenshot capture, and neighboring file-creation scenario (3 tests, 19.0s,
retries disabled, managed one-worker budget). Both regression runs asserted
source commit/status preservation. Current-head CI and review results remain
external delivery gates recorded in the PR and parent messages.


### Accepted baseline provenance

A fresh full CodeRabbit review of the first remediation found an additional
AC .10 ordering: Reload or a clean refresh accepts X, then typing and another
agent warning Z arrive before the older receipt. The latest observation alone
cannot prove which baseline was actually accepted. Four actual-producer REDs
covered both acceptance paths and both distinct X and a return to the real
original baseline/hash. A further RED covered explicit Reload of a preexisting
snapshot while saving; a matching-save-echo control remains intact.

The store now retains the last accepted workspace snapshot separately from the
latest observation. Only real clean/matching refresh and Reload publication
advance acceptance; warning-only publication preserves it. Receipt selection
requires that this acceptance changed after admission and still matches the
live baseline. A later matching save echo supersedes an older acceptance. This
is bounded transient evidence per editor, never a history, API or persisted field.

The six focused suites pass 95 tests. Typecheck and zero-warning changed-file
lint passed. A fresh production build, extended regression/capture, and neighbor
scenario passed 3 tests with retries disabled (23.3s). The desktop browser regression also exercises Reload while the
receipt is pending, then typing and a later warning; typing back to the accepted
baseline must disable Save after the delayed receipt, proving its dirty baseline
through the actual UI. Final browser/review/CI results are recorded externally.


### CI test-contract remediation

Current-head E2E exposed two preexisting test assumptions in the failed shard.
The mobile task recovery test dispatched `online` about 217 ms after an initial
refresh, within the intentional 250 ms foreground coalescing episode. Freezing
browser Date before navigation reproduced the missing-GET failure with retries
disabled. Moving Date into a new episode before the synthetic event passed;
timers and production refresh behavior remain unchanged.

The plugin recovery test expected exactly four rows after one continuation,
although live message projection can retain a fifth independently of the
page size. Its CI screenshot showed all five legitimate prompts; production
merges by message ID. The scoped test correction checks the older seeded
message, retention of recovered/live messages, and complete continuation to
all five unique seeded IDs. No plugin production behavior, public contract, requirement or system
design changes are needed. Final local and delivery checks are recorded below
and in the PR rather than waived by an isolated pass.


Local CI remediation validation: the exact selected 34-test partition ran
sequentially with retries disabled. Both corrected cases passed; a separate
sidebar reveal-cue assertion failed, so this is recorded as 33 passed/1 failed,
not a green partition. The cue case then passed alone (6.3s), and its complete
8-test sidebar file passed without retries (28.1s). No sidebar source or test
was changed and the local intermittent result remains a delivery limitation.
Changed-file ESLint, spec/catalog validation, whitespace checks and trusted-base
documentation preflight (`covered`, no errors) passed. Fresh remote CI for the
remediation head is still required.
