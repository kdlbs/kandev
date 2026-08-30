---
created: 2026-08-30
status: implemented
requirements:
  - REQ-WORKSPACES-EMPTY-REMOTE-REPOSITORIES-001
  - REQ-WORKSPACES-EMPTY-REMOTE-REPOSITORIES-002
  - REQ-WORKSPACES-WORKTREE-BASE-REFRESH-001
system_design:
  - ../../specs/workspaces/system-design/empty-remote-repositories.md
  - ../../specs/workspaces/system-design/worktree-base-refresh.md
legacy_specs: []
---

# Implementation Plan: Empty Remote Repositories

## Overview

This package lets Kandev launch task worktrees from empty remote repositories. It keeps remote publication behind existing explicit Push and Create PR actions.

Implementation starts with typed remote-state evidence and a marked local baseline. The Git publication flow then consumes that baseline through task runtime credentials.

Frontend error mapping, desktop and mobile evidence, and public documentation complete the delivery package.

## Scope

### In scope

- Classify an authenticated zero-ref advertisement as an empty remote.
- Create a deterministic empty local baseline and exact marker ref.
- Create normal task worktrees from the local baseline.
- Publish the baseline before the task branch during explicit Push and Create PR actions.
- Preserve local and remote history when another actor initializes the remote.
- Show localized recovery errors through existing desktop and mobile Changes surfaces.
- Document empty-remote launch and first publication.

### Out of scope

- Create a hosted repository.
- Add project templates or initial files.
- Publish a remote ref during task launch.
- Force, reset, merge, or rebase during bootstrap recovery.
- Add change-request support for another provider.
- Add a new page, dialog, drawer, setting, or database column.

## Technical approach

### Remote-state evidence and local baseline

Add a typed `RemoteRefState` result to strict clone and refresh boundaries. Carry `has_refs`, `empty`, or `unknown` into the worktree request.

Add `internal/gitbootstrap` for the deterministic empty commit and marker-ref contract. Create the base and marker refs in one local ref transaction.

Update `internal/worktree.Manager` to accept only authenticated `empty` evidence. Create the marked baseline under the existing repository lock before normal branch planning.

Keep non-empty missing-branch, authentication, network, timeout, cancellation, and ancestry errors fail-closed.

### Explicit first publication

Add one `GitOperator` helper that validates the marker and advertises the selected remote with task runtime credentials.

If the remote remains empty, push the marked base without force. Then use the existing task-branch push path.

Reuse the helper in ordinary Push and Create PR. Call provider change-request creation only after both refs exist.

Return bounded error codes for a changed remote, failed base publication, and failed task-branch publication after base success.

### User-visible recovery

Extend the shared Git operation result types with the bounded error codes. Map them to localized Changes toasts.

Reuse the existing Changes split button, change-request dialog, mobile task layout, and mobile menu treatment. Add no new responsive composition.

Add English, Portuguese, Simplified Chinese, and Traditional Chinese catalog entries. Generate both Traditional Chinese variants through the repository script.

### End-to-end evidence

Add a disposable empty Git remote fixture. Create a task through the normal task flow and make one project change.

The desktop scenario proves launch, commit, Push, base-first publication, and task-branch publication. The phone scenario proves that the existing Changes touch path reaches the same result.

The nearest mobile exemplar is the existing task layout and responsive Changes menu. The workflow uses the current scroll owner, safe-area handling, and touch targets.

### Public documentation

Update the task workflow guides with the empty-remote behavior and publication boundary. Correct the existing local-repository text that still describes an unborn `main` branch.

Classify both pages as how-to guides. Keep provider credentials and recovery limits visible beside the relevant steps.

## Tests

- `AC-WORKSPACES-EMPTY-REMOTE-REPOSITORIES-001.1` and `001.6`: strict refresh tests distinguish zero refs from missing branch and unavailable remote.
- `AC-WORKSPACES-EMPTY-REMOTE-REPOSITORIES-001.2` through `001.5`: real-Git worktree tests inspect the empty tree, marker, branch, and remote refs.
- `AC-WORKSPACES-EMPTY-REMOTE-REPOSITORIES-001.7`: multi-repository preparation tests isolate the marker and baseline.
- `AC-WORKSPACES-EMPTY-REMOTE-REPOSITORIES-002.1` through `002.4`: `GitOperator` tests prove base-first Push and Create PR without force.
- `AC-WORKSPACES-EMPTY-REMOTE-REPOSITORIES-002.5` through `002.7`: race and partial-result tests prove safe recovery and stable error codes.
- `AC-WORKSPACES-EMPTY-REMOTE-REPOSITORIES-002.9`: provider tests prove that change-request creation starts after both pushes.
- `AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.8`: worktree tests prove that only typed zero-ref evidence permits the marked baseline.

## E2E tests

- `apps/web/e2e/tests/git/empty-remote-repository.spec.ts` covers task launch, commit, Push, remote `main`, and the task branch.
- `apps/web/e2e/tests/git/mobile-empty-remote-repository.spec.ts` covers the same Push outcome through the phone Changes menu.
- The mobile scenario uses the configured `mobile-chrome` Pixel 5 project. It asserts the user result, not only control visibility.

## Work orders

- [x] [Task 01: Seed Empty Remote Worktrees](task-01-seed-empty-remote-worktrees.md)
- [x] [Task 02: Publish the Empty Remote Base](task-02-publish-empty-remote-base.md)
- [x] [Task 03: Surface Publication Recovery](task-03-surface-publication-recovery.md)
- [x] [Task 04: Prove the Empty Remote Workflow](task-04-prove-empty-remote-workflow.md)
- [x] [Task 05: Document the Empty Remote Workflow](task-05-document-empty-remote-workflow.md)

## Verification results

Completed.

- Task 01: `go test ./internal/gitbootstrap ./internal/repoclone ./internal/worktree ./internal/orchestrator/executor` (954 tests passed).
- Task 02: `go test ./internal/agentctl/server/process ./internal/agentctl/server/api ./internal/agent/handlers ./internal/orchestrator/handlers` (1,573 tests passed).
- Task 03: focused Vitest suite (22 tests passed), `pnpm run i18n:check`, and `pnpm run typecheck` passed.
- Task 04: desktop and `mobile-chrome` Playwright scenarios passed.
- Task 05: public-doc tests (61 passed), public-doc validation (41 pages), and `git diff --check -- docs/public` passed.
- Repository specification lint passed with `python3 scripts/lint-spec-files.py --all`.
- Review round: marker retirement, marked-base recovery, base mismatch, and credential-safe probe regressions passed; backend lint reports 0 issues.
- Review fixup coverage: absence-lease publication, immutable baseline refspec, post-publication race detection, local-baseline recreation, refreshed local-base fallback, deterministic Git identity, stdout/stderr separation, and credential-safe diagnostics are covered by focused regressions.

## Risks

- A weak empty-remote probe can confuse a missing branch or credential error with zero refs.
- Marker heuristics can publish user history. Implementation must compare exact refs.
- Another actor can initialize the remote between advertisement and base push.
- Baseline publication can succeed before task-branch publication fails.
- Provider and executor credential routes can expose different read and write authority.
- Multi-repository task roots can route a marker or Push to the wrong repository without exact scoping.
