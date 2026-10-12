---
created: 2026-09-27
status: complete
requirements:
  - REQ-TASKS-MANAGED-CLONE-RELOCATION-001
  - REQ-TASKS-MANAGED-CLONE-RELOCATION-002
  - REQ-TASKS-MANAGED-CLONE-RELOCATION-003
system_design:
  - ../../specs/tasks/system-design/managed-clone-relocation.md
legacy_specs: []
---

# Implementation plan: Managed clone relocation

## Overview

The July 2026 workspace clone change left some older task worktrees attached to
their original clones. A later repository-identity check rejects those tasks on
resume. This package adds guarded relocation for a clean worktree, then adds an
explicit recovery path for a dirty one. The existing Git identity check remains
the final launch gate.

The reproduction has two clones of the same provider repository. A task
worktree belongs to clone A. `repositories.local_path` points to clone B.
`validateLaunchWorkspaceAdmission` returns `ErrReuseWorktreeUnavailable` before
agent startup. The original checkout and its files still exist.

## Scope

### In scope

- Host Worktree task environments with a verified managed source-clone change.
- Exact branch and commit transfer, including unpushed commits.
- Exclusive clean relocation and explicit dirty relocation with retained data.
- A typed recovery action, desktop and phone presentation, and public Git guidance.

### Out of scope

- Local or remote executor repair, deleted branches, and lost Git objects.
- Bulk background migration and automatic removal of original checkouts.
- Changes to provider conversation identity or Git credential scope.

## Technical approach

Task 01 adds a nullable source-clone identity to the canonical environment
repository row, writes it during new worktree materialization, and proves
legacy identity from Git registration before any backfill. It classifies a
managed clone change before read-only lifecycle admission. It uses the existing
environment recovery claim, filesystem claim, and exact-slot compare-and-swap
to replace a verified clean worktree. It imports the branch and exact commit
from the verified source clone into the current workspace clone. It retains
the original checkout and refuses a dirty, busy, foreign, or ambiguous source.

Task 02 extends the guarded operation with a stamped, authorized
`relocate_and_resume` action for dirty worktrees. It reuses the metadata
recovery snapshot path, retains originals, and tells the user that staging
choices do not transfer. The recovery model removes actions that cannot
repair this mismatch. The same action works from desktop and phone.

The source clone and selected repository must agree on provider, host, owner,
and repository identity. An origin string alone does not prove ownership.
For GitHub and GitLab managed clones, apply the same proof. If a provider lacks
an authoritative identity or local object transfer fails, refuse relocation
and retain the original. A local source repository is outside this operation.

The decision boundary is recorded in
[ADR-2026-09-27-managed-clone-relocation-boundary](../../decisions/2026-09-27-managed-clone-relocation-boundary.md).

## ASCII UI preview

`UI-01` is the session recovery card after a verified dirty clone mismatch.
The current card offers Resume, Start fresh, and Restore read-only workspace.
None repairs this cause. The proposed card has one repair action.

```text
UI-01 desktop | task session | managed clone mismatch
+--------------------------------------------------------------+
| Workspace needs repair                                       |
| This task's files remain in its original checkout.          |
| [Move files and resume]  [Technical details]                |
+--------------------------------------------------------------+

Confirmation dialog
+--------------------------------------------------------------+
| Move this task to the current repository clone?              |
| The original files and a snapshot remain. Staging choices   |
| do not transfer. The conversation continues.                 |
|                                      [Cancel] [Move and resume]|
+--------------------------------------------------------------+
```

```text
UI-01 phone | existing session recovery view
+----------------------------------+
| Workspace needs repair           |
| Files remain in the old checkout.|
| [Move files and resume]           |
| [Technical details]              |
+----------------------------------+

Inset bottom confirmation drawer
+----------------------------------+
| Move to current clone?           |
| Original and snapshot remain.    |
| Staging choices do not transfer. |
| [Cancel]                         |
| [Move and resume]                 |
+----------------------------------+
```

The structure and action order are required. The wording is illustrative and
must use localized keys. The card keeps one scroll owner. The phone action and
drawer controls need 44-pixel touch targets and safe-area clearance. Existing
mobile session recovery cards provide the entry point and hierarchy. This
preview covers `AC-TASKS-MANAGED-CLONE-RELOCATION-002.1`, `.002.3`, and `.003.2`.

## Tests

| Criteria | Evidence |
| --- | --- |
| 001.1-001.2 | Real-Git two-clone test proves exact branch, unpushed commit, original retention, and successful resume. |
| 001.3-001.4 | Busy, foreign, ambiguous, multi-slot, and executor-type refusal tests compare Git state and inventory before and after. |
| 002.1-002.4 | Dirty snapshot tests cover staged, untracked, ignored, deleted, symlink, restart, and stale stamp cases. |
| 003.1-003.2 | Recovery model tests and desktop/phone E2E prove action eligibility and the same successful outcome. |

## E2E tests

Extend `apps/web/e2e/tests/session/session-resume-recovery.spec.ts` for desktop.
Extend `apps/web/e2e/tests/session/mobile-session-resume-recovery.spec.ts` for
phone. Both seed separate managed clones, preserve a dirty original, submit the
explicit action, and verify the task's Files path and session conversation.

## Work orders

- [x] [Task 01: Keep clean worktrees resumable](task-01-clean-relocation.md)
- [x] [Task 02: Recover dirty worktrees](task-02-dirty-recovery.md)

## Verification results

The backend build, legacy-layout and source-removal relocation regressions,
full worktree/repoclone/lifecycle/executor packages, SQL guard, and persistence
race conformance passed. The strict lifecycle execution regression passed with
the worktree manager unavailable. Frontend typecheck, recovery UI tests (79
tests), i18n checks, desktop recovery E2E (2 tests), and mobile recovery E2E (5
tests) passed. The desktop and phone dirty-relocation E2E cases passed with
retries disabled using the legacy owner/name fixture. Public documentation
checks, specification validation, spec lint, and `git diff --check` passed.
Final PR fixup also passed the complete lifecycle and task-service Go packages,
the desktop dirty-relocation, CLI fallback, and both TUI restart E2Es, plus the
mobile clarification, mobile entry-recovery, and automation-confirmation cases.
The follow-up inventory regression pins an explicitly authorized dirty
relocation to the exact worktree selected by its session when task branch
settings have changed. The complete executor and worktree packages, changed-code
backend lint, backend build, frontend E2E build, frontend typecheck, desktop
dirty-relocation E2E, and mobile clarification E2E passed after this fix.
The mobile sidebar archive and mobile thread-action E2Es also passed after
isolating update-notice placement from ordinary action toasts. The SSH
repository-secret container E2E passed in a focused local run.
PostgreSQL-specific migration and concurrency checks were not run because
`KANDEV_TEST_POSTGRES_DSN` was unset.

## Risks

The [convergence fix package](../managed-clone-recovery-convergence/plan.md)
adds regression coverage for inspection contention and recovery error projection.
The completed results above remain the evidence for this original package.

- Local Git object transfer needs strict origin and workspace proof. A lookalike
  repository cannot become a source of code or credentials.
- A dirty file copy cannot preserve staging choices. The original and snapshot
  remain available, and the user must choose the transfer.
- Multi-repository publication is not one filesystem transaction. The claim,
  inventory reload, and restart record must make partial progress safe.
