---
spec: docs/specs/executors/requirements/ssh-executor.md
issue: https://github.com/kdlbs/kandev/issues/2413
created: 2026-08-08
status: implemented
---

# Implementation Plan: SSH Workspace Preparation

## Overview

Issue #2413 is caused by a split launch pipeline: `SSHPreparer.Prepare` reports a successful
validation-only step, while `SSHExecutor.CreateInstance` creates the remote task directory and starts
`agentctl` without resolving or executing the profile prepare script. The only current root checkout
is the special `RemoteContributions[""]` path, so an ordinary provider-backed repository reaches the
agent as an empty directory. The repair gives SSH the same prepare/cleanup contract as Sprites while
preserving SSH host-key, credential-forwarding, and resumable-workspace boundaries.

---

## Backend

### SSH default and script resolution

- Add an idempotent `ssh` case to
  `apps/backend/internal/agent/runtime/lifecycle/default_scripts.go`. It must initialize the primary
  checkout at the remote task workspace root, verify a reused checkout's credential-free `origin`,
  run `{{repository.setup_script}}`, and leave feature-branch selection to the unconditional
  `KandevBranchCheckoutPostlude`.
- Add SSH prepare/cleanup resolution beside the executor (prefer a focused
  `executor_ssh_scripts.go` rather than growing `executor_ssh.go`). Reuse `scriptengine` providers for
  workspace, repository, worktree, Git identity, GitHub auth, and remote contribution bindings. Map
  agentctl install/start placeholders to the explicit SSH lifecycle so a profile script cannot start
  an unauthenticated duplicate controller.
- Execute the resolved prepare script through the selected remote login shell with the effective
  profile/repository environment explicitly configured for that executor. Keep secret values on
  stdin, bound execution with a timeout, bound retained output, and return a generic checkout error
  where Git stderr could contain credentials. Preserve the narrower existing allowlist for the agent
  subprocess; prepare-script access must not accidentally broaden long-lived agent environment.

### Launch ordering and postcondition

- Refactor `SSHExecutor.CreateInstance` in
  `apps/backend/internal/agent/runtime/lifecycle/executor_ssh.go` into the observable order: connect
  and verify host; upload/resolve `agentctl`; create the task workspace; upload credentials; execute
  preparation; verify that an attached primary repository is a matching Git checkout; create the
  session runtime directory; preflight the selected agent binary; then launch/forward `agentctl` from
  the remote workspace.
- Remove the post-controller ordinary/special-case gap by routing contribution setup through the same
  resolved prepare pipeline. Preserve exact-head validation and source-upstream behavior from
  `executor_ssh_remote_contribution.go`; consolidate or delete that SSH-only script only after its
  existing security and resume tests are represented in the shared path.
- A failed or cancelled preparation must close the SSH client and leave no remote controller, port
  forward, or tracked session. A reused checkout must preserve local descendants and untracked files.

### Terminal cleanup

- Resolve and run `MetadataKeyCleanupScript` over the tracked SSH connection in `StopInstance` only
  when `shouldRunExecutorCleanup(instance.StopReason)` is true. Run it against
  `MetadataKeySSHRemoteTaskDir` with the same non-secret providers as preparation, then stop the remote
  controller and close forwarding/SSH even if cleanup fails.
- Keep any cleanup environment needed after launch only in the in-memory `sshSessionState`; rebuild it
  from the resume request after backend recovery and never persist resolved secret values.
- Ordinary user Stop and `StopReasonBackendShutdown` must not run cleanup. Preserve the remote task
  directory under all current SSH stop paths; this repair does not add automatic remote deletion.

---

## Tests

- **What:** `DefaultPrepareScript("ssh")` is populated, contains the repository setup placeholder,
  has no inline task-branch checkout, and safely reuses a matching workspace.
  **File:** `apps/backend/internal/agent/runtime/lifecycle/default_scripts_test.go` and focused SSH
  script tests.
  **How:** execute the resolved script against temporary bare repositories; assert root checkout,
  credential-free `origin`, task branch, setup marker, preserved local descendant, and conflicting
  origin rejection.
- **What:** SSH preparation resolves supported placeholders, receives only the approved environment,
  runs before controller launch, and fails closed on non-zero exit, timeout, absent checkout, or
  cancellation.
  **File:** focused tests beside `executor_ssh_scripts.go` plus narrow executor orchestration tests.
  **How:** fake command/session seams record ordering and output without dialing a user host.
- **What:** terminal cleanup runs for archive/delete reasons, skips Stop/backend shutdown, resolves
  the remote workspace, and never prevents controller teardown after failure.
  **File:** focused SSH lifecycle tests beside `executor_ssh.go`.
  **How:** fake SSH command and controller-stop seams assert invocation order and error handling.
- **What:** contribution checkouts keep target `origin`, exact source SHA, source upstream, and local
  descendant reuse after consolidation.
  **File:** existing/focused tests in
  `apps/backend/internal/agent/runtime/lifecycle/executor_ssh_connection_test.go` or the new script
  test file.
  **How:** temporary target/source bare repositories with URL rewrites.

## E2E Tests

- **Scenario:** GIVEN a provider-backed repository and the default SSH profile, WHEN a task starts,
  THEN the remote task directory is a Git checkout on the task branch, contains the repository file,
  and is the workspace used by `agentctl` and the agent.
  **File:** `apps/web/e2e/tests/ssh/launch-task.spec.ts` or a focused sibling SSH workspace spec.
  **What to verify:** real SSH transport, remote `git rev-parse --show-toplevel`, repository content,
  session metadata, and controller log location.
- **Scenario:** GIVEN custom prepare and cleanup scripts, WHEN the task starts and is later archived
  or deleted, THEN the prepare marker exists before the agent completes and the cleanup marker appears
  only during terminal teardown.
  **File:** the same focused SSH workspace spec.
  **What to verify:** profile persistence, placeholder resolution, preparation failure gating, cleanup
  reason gating, and retained remote workspace evidence.
- Extend `apps/web/e2e/helpers/http-git-server.ts` only as needed to expose a disposable provider URL
  reachable from the SSH container. Keep the target isolated, credential-free, and torn down by the
  owning fixture.

## Public Documentation

- Update `docs/public/executors.md` to describe automatic primary checkout, prepare/cleanup script
  execution, failure behavior, and retained workspace cleanup semantics.
- Update the SSH row in `docs/public/feature-status.md` so it no longer claims repository and script
  preparation are unsupported.
- Treat `docs/public/executors.md` as a how-to/reference hybrid whose dominant type is how-to;
  `docs/public/feature-status.md` remains reference.

## Verification Results

- Task 01 targeted SSH lifecycle tests: `rtk go test ./internal/agent/runtime/lifecycle -run 'Test.*SSH.*(Prepare|Workspace|Cleanup|Contribution|Origin|Checkout)' -count=1` — 23 passed.
- Task 01 full lifecycle package: `rtk go test ./internal/agent/runtime/lifecycle -count=1` — 1,141 passed.
- Task 01 race-focused run: `rtk go test -race ./internal/agent/runtime/lifecycle -run 'TestSSH|TestShouldRunExecutorCleanupIncludesCascadeTerminalReasons' -count=1` — 47 passed.
- Task 02 container-backed SSH regression: `rtk env KANDEV_E2E_CONTAINERS=1 rtk pnpm e2e:run --no-build --project containers tests/ssh/launch-task.spec.ts` — 9 passed.
- Task 03 public docs tests: `rtk node --test scripts/validate-public-docs.test.mjs` — 58 passed; `rtk node scripts/validate-public-docs.mjs` — 41 pages validated.

## Implementation Waves And Parallel Candidates

Wave 1:

- [x] [Task 01: SSH workspace lifecycle](task-01-ssh-workspace-lifecycle.md)

Wave 2:

- [x] [Task 02: SSH container regression](task-02-ssh-container-regression.md)

Wave 3:

- [x] [Task 03: Public documentation alignment](task-03-public-documentation-alignment.md)

Execution is sequential in the primary conversation. The tasks share behavior contracts and are not
marked parallel-safe.
