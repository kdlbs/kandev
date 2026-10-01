---
created: 2026-10-01
status: implemented
requirements:
  - REQ-TASKS-MANAGED-CLONE-RELOCATION-001
system_design:
  - ../../specs/tasks/system-design/managed-clone-relocation.md
legacy_specs: []
---

# Implementation Plan: Resume unchanged legacy managed clones

## Overview

Restore launch, resume, and workspace access when the registered repository and
its task checkout still use the same healthy legacy clone. The task system owns
this repair because it owns admission and continuity of the existing checkout.

The September 28 change `adc5d67f55` computes a workspace destination for every
eligible provider row and validates it before establishing whether the registered
source changed. A missing candidate therefore rejects an intact legacy checkout.
The live incident was reproduced through `session.recover`; Git verified the
original checkout and the absent computed destination independently.

## Scope

In scope: shared admission, strict identity checks, and backend regressions.
Out of scope: database migrations, automatic manual Git-registration rewrites,
new relocation support for filters/submodules, UI changes, and bulk repair.

## Technical approach

Add one shared unchanged-source inspection in
`apps/backend/internal/worktree/managed_clone_legacy_reuse.go`, called from
`managed_clone_relocation.go` by linked and
main checkout admission before canonical destination validation. Follow the
system design's exact managed candidate, origin, and Git-directory proof.
Retain `ManagedCloneRelocationPaths` as layout discovery and retain the current
executor, lifecycle, and task-service proof wiring. No nil-proof shortcut based
only on the absence of the destination is acceptable.

| Shape | Expected behavior | Evidence |
| --- | --- | --- |
| GitHub/GitLab, owner/name or provider legacy clone | Reuse exact registered clone | Real-Git admission tests |
| Same legacy source with destination absent or present | Reuse without transfer | Path/branch/index/content assertions |
| Registered workspace destination changed | Existing guarded relocation | Existing relocation regressions |
| Missing registered destination, wrong origin, foreign path | Refuse without mutation | Negative and mixed-slot tests |
| Other providers or non-host executors | Existing handling | Existing package suites |

## Tests

Add `managed_clone_legacy_reuse_test.go` with
`TestManagerAdmitRecoveryReusesRegisteredLegacyClone` and
`TestManagerAdmitRecoveryRejectsInvalidLegacyReuse`. Cover both legacy layouts,
GitHub/GitLab, main/linked checkout, destination absent/present, dirty/staged and
ignored files, a configured filter, and an initialized submodule. Assert unchanged
HEAD, branch, index, file bytes, inventory, and absent destination after reuse.
Add a mixed-slot case containing one reusable slot and one invalid slot.

## End-to-end evidence

Add `executor_legacy_clone_recovery_test.go` with
`TestPreflightSessionWorktreeRecoveryReusesRegisteredLegacyClone`, using real Git
and persisted repository/environment records. Prove the real preflight consumes
the generated relocation proof and preserves session identity and inventory.
Run the existing lifecycle and task-service suites for workspace restoration.
No browser interaction or rendered UI changes are needed for this backend guard.

## Work orders

- [x] [Task 01: Preserve unchanged legacy checkout admission](task-01-preserve-legacy-admission.md)

## Verification results

Completed on 2026-10-01.

- Red: all eight initial real-Git legacy layout/provider/main-linked cases failed
  with `managed destination clone identity is invalid` before production edits.
- Green: all new legacy reuse/content/refusal/cancellation/case/symlink tests pass.
- SQLite-backed executor preflight passes with real cloner proof and unchanged
  persisted environment and session metadata.
- Focused worktree/executor/lifecycle admission suites passed. Existing clean and
  dirty relocation, restart, multi-repository refusal, origin parsing, and content
  preservation suites passed (separate worktree run: 12.718s).
- Full repoclone suite passed (17.929s); full task-service suite passed (55.158s).
- Full five-package command was run twice, first in the host environment and
  then with isolated HOME/Git configuration and canonical temporary paths.
  Remaining worktree/executor/lifecycle failures were macOS `/dev/fd/3` directory
  handling, a shell timeout fixture, Unix socket path length, and Docker daemon
  preparation. Representative failures in every affected package were reproduced
  with a Go overlay restoring the original production file. They are recorded
  baseline limitations, not passing checks; Linux CI retains the full suite gate.
- Changed-package golangci-lint: zero issues.
- Catalog validation, specification lint, 36 spec-linter tests, 62 public-doc
  validator tests, validation of 47 published pages, PR documentation coverage
  preflight, and `git diff --check` passed.

The initial case-alias fixture was corrected to keep request and persisted
repository paths consistent; the final case and symlink alias regressions pass.
No live installation or database was modified during implementation.


## Delivery

After the implementation handoff and passing checks, commit and open the PR.
The user has authorized the PR and explicitly requires the `ghp` alias for
personal-account authentication. Resolve that alias and verify the account before
publication; do not substitute the default `gh` account. No delegation authorized.

## Risks

- An overly broad reuse bypass could accept a foreign clone or hide a missing
  registered destination. Require exact filesystem and origin proof.
- macOS path casing must use real directory identity, not global lowercasing.
- A valid slot must never hide an invalid sibling in the selected environment.
