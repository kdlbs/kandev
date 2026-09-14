---
created: 2026-09-13
status: completed
requirements:
  - REQ-PLATFORM-GIT-SUBPROCESS-ADMISSION-002
  - REQ-PLATFORM-WORKSPACE-GIT-STATUS-001
system_design:
  - ../../specs/platform/system-design/git-subprocess-execution.md
  - ../../specs/platform/system-design/workspace-git-status.md
legacy_specs: []
---

# Implementation Plan: Noninteractive Git execution

## Overview

Enforce application Git prompt suppression after callers assemble their environments, then bound helper cleanup and uncovered network operations.
Prove selected credential compatibility and task recovery before updating public guidance.
The five work orders were implemented sequentially on the verified PR #3635 descendant and validated with focused backend, web, build, documentation, and desktop/mobile E2E checks.

## Evidence and root cause

Source inspected at `bd7974dc8`, matching the supplied report's build.
`NewGitCommand` constructs Git without prompt controls. All shared runner variants execute caller commands without final preparation.
`GitOperator.runGitCommandWithEnvironment` replaces the environment after construction, making constructor-only enforcement insufficient.
Local worktree, repository-fetch, and tracker helpers apply inconsistent controls; the tracker uses `GIT_ASKPASS=echo`.

A temporary real-Git experiment on 2026-09-13 used a controlling PTY, isolated HOME/global/system config, and `credential.helper=!false`.
`git credential fill` for `example.invalid` printed a username prompt and was still waiting after 1.039 seconds.
With terminal prompting disabled, deny-only askpass, and GCM noninteractive mode, Git failed in 0.047 seconds.
The parent killed only its owned waiting process group. No network, credentials, or live instance were used.
The failure diagnostic also quotes the username prompt, so substring absence alone is not a correct regression assertion.

This confirms the terminal-fallback mechanism and missing enforcement boundary.
The original task-navigation subprocess, regression commit, whole-backend outage, and provider-trigger hypothesis remain unconfirmed.
The supplied GitHub incident and invalid-token observations are reported context, not independently verified facts in this package.

## Merged dependency: PR #3635

[PR #3635](https://github.com/kdlbs/kandev/pull/3635) merged on 2026-09-13 at 11:19:46 UTC as `9cc146ee21c296d553ae684531219a5c711a4213`.
The merge commit was the inspected `origin/main` head when this package began. The working checkout includes this commit and the implementation changes.
Verify that `git merge-base --is-ancestor 9cc146ee21c296d553ae684531219a5c711a4213 HEAD` succeeds before compatibility tests.
Recheck upstream changes if main advances again.

The merged [host bridge package](https://github.com/kdlbs/kandev/blob/9cc146ee21c296d553ae684531219a5c711a4213/docs/plans/executor-host-gh-bridge/plan.md) is implemented, not another pending work order.
It owns `REQ-INTEGRATIONS-GITHUB-AUTHENTICATION-002`, criteria `.1` through `.7`.
The [merged credential design](https://github.com/kdlbs/kandev/blob/9cc146ee21c296d553ae684531219a5c711a4213/docs/specs/integrations/system-design/github-authentication-02.md#executor-host-cli-bridge) is the compatibility authority.
No change to our observable requirements or five-task dependency order is needed.

The merge adds credential routing and environment composition. It does not modify `common/subproc/shared.go` or `git_command.go`.
The shared execution defect is the target of this package. The implementation consumes the composed environment without rebuilding host eligibility or helper registration.

Preserve existing user-helper order, optional marker-owned host helpers, token precedence, and managed-mode exclusion.
Environment duplicate removal applies to repeated assignments of the same environment variable, not repeated Git configuration entries at different indexes.
The existing optional helper chain is allowed; the prohibition on new fallback means no new routing after authentication failure.
Do not remove the host bridge because its shell function is not a direct OpenSSH command; SSH normalization applies only to SSH settings.

An installed bridge can recover on a later command when its credential store or service recovers.
If bridge registration was skipped, existing launch/resume/prepared-start evaluation can install it after recovery, without restarting the backend.
Do not promise that every existing instance acquires a previously absent bridge on its next arbitrary Git command.

## Requirement conformance

Platform owns shared execution safety across callers.
Reuse `AC-PLATFORM-WORKSPACE-GIT-STATUS-001.16` through `.18` for comparison behavior.
Add `REQ-PLATFORM-GIT-SUBPROCESS-ADMISSION-002` to the existing shared Git requirement because universal prompt and helper-cleanup criteria were missing.
The existing accepted transport ADR remains authoritative. Its credential scope and no-fallback policy do not change.
The assumption check found no material unanswered product choice: the prompt explicitly settles scope, recovery, exclusions, and planning handoff.

## Scope

### In scope

- Final prompt enforcement and selected-scope credential compatibility.
- Finite network budgets after admission, owned helper cancellation, pipe cleanup, and slot release.
- Required error completion and optional comparison recovery on desktop and phone.
- Targeted TDD, source audit, scoped engineering guidance, and public recovery guidance.

### Out of scope

- Credential changes, automatic retries, transport changes, and new authentication UI.
- Launcher-wide environment changes or deliberately interactive task terminals.
- Publication, live-instance mutation, credential changes, and provider operations.
- Attribution of the original incident to an unproven subprocess or provider event.

## Technical approach

The [execution design](../../specs/platform/system-design/git-subprocess-execution.md) defines the shared preparation and lifecycle boundary.
Preserve the [GitHub credential authority](../../specs/integrations/system-design/github-authentication-02.md) and existing clone isolation.
Move only reusable process primitives below agentctl; keep the unrelated agent lifecycle stable through compatibility wrappers.
Do not replace stdin or pass sensitive credentials through logs.

### Initial caller inventory

Task 03 completed the repository-wide inventory and applied the shared execution boundary to each discovered application-owned network path.

| Caller / path under apps/backend/internal | Finding and treatment |
| --- | --- |
| orchestrator/executor/executor_host_gh_bridge.go (merged dependency) | Keep optional host probing and helper registration unchanged; add compatibility evidence through final Git execution |
| agent/runtime/environment/environment.go and agentctl/server/process/manager.go (merged dependency) | Consume canonical indexed configuration; preserve overlay/complete replacement and stale generated-helper removal |
| common/subproc/shared.go | All runner variants now use final preparation and bounded lifecycle handling |
| agentctl/server/process/git.go | Explicit instance environment and operation-specific AfterAcquire budgets are preserved |
| agentctl/server/process/workspace_git_cmd.go | Tracker environment, index, lock behavior, prompt/SSH controls, and the 10-second budget are preserved |
| agentctl/server/process/workspace_git_diff.go | Manual admission uses the shared stream lifecycle and closes the reader on cancellation before Wait |
| worktree/manager_git.go | Manual admitted paths use the shared lifecycle and 500 ms WaitDelay while retaining manager budgets |
| task/service/repository_fetch.go | The 30-second refresh, singleflight, cooldown, and shared prompt controls are preserved |
| task/service/fresh_branch.go | The 30-second AfterAcquire budget, best-effort fetch, and required checkout errors are preserved |
| agent/runtime/lifecycle/env_preparer_local.go | Fetch and checkout use finite post-admission budgets with the caller context |
| repoclone/clone.go | Clone, fetch, and submodule operations retain scoped helper authority with finite budgets |
| gitbootstrap/gitbootstrap.go | Network reachability is covered without changing local initialization behavior |
| office/configloader/git.go | The 30-second AfterAcquire path inherits final policy and cleanup |
| backendapp and submodule callers | Generic command construction and network verbs are covered by the completed inventory |
| task/gitinit/command_descriptor.go | Local descriptor-based ExecGit exception; preserve security and process ownership |

The completed budget inventory is summarized below. A shorter caller lifetime always wins over these post-admission budgets.

| Caller group | Network operations | Post-admission budget | Error owner |
| --- | --- | --- | --- |
| `agentctl/server/process` Git operator | fetch, push, remote inspection | 30 seconds for ordinary operations; five minutes for clone, push, and submodule operations | Required API result with sanitized error and released operation lock |
| Workspace tracker and comparison materialization | fetch and target refresh | Existing 10-second tracker command budget | Optional unavailable comparison state; local Changes remains usable |
| Worktree manager | fetch and pull | Existing configured fetch/pull budgets, 60 seconds by default; 30-second inspect commands | Existing lifecycle and fallback policy |
| Runtime, task, repository, and Office setup | fetch, checkout, clone, submodule | 30 seconds for ordinary fetch/checkout; five minutes for clone/submodule | Existing required setup errors or documented best-effort fetch handling |
| Backend materialization and repository state | fetch, pull, clone, push, submodule, `ls-remote` | 30 seconds for ordinary operations; five minutes for clone/push/submodule | Existing caller-specific required or optional result |

Uncovered ordinary fetch, pull, checkout, and `ls-remote` paths use the 30-second default. Uncovered clone, submodule, and push paths use the five-minute default.
Existing explicit operation budgets remain intact, and earlier parent cancellation wins.

## Tests

The following table records the behavioral evidence implemented by the work orders.

| Acceptance criteria | Behavioral evidence |
| --- | --- |
| ADMISSION-002.1, .2 | Task 01: `TestManagedGitCredentialFillPTY`, `TestGitFinalEnvironment`, `TestGitCredentialHTTPSelectedScope`, `TestGitSSHCommandPreservation` |
| ADMISSION-002.3, .4 | Task 02: `TestGitHelperCancellationReleasesSlot`, `TestGitDescendantPipeCleanup`, `TestWindowsManagedGitJobCleanup`, `TestCapDiffOutputCancellationClosesReader` |
| ADMISSION-002.2 through .6 | Task 03: `TestGitOperatorAuthenticationFailureAndRecovery`, `TestLocalCheckoutFetchDeadline`, `TestCloneDeadlineAfterAdmission`, existing queue-cancellation regressions |
| WORKSPACE-GIT-STATUS-001.16 through .18 | Task 04: authentication fixture with available local status, unavailable comparison, and later recovery |

Prefixes above abbreviate `AC-PLATFORM-`; work orders contain full IDs and exact paths/commands.
Protect PTY/helper tests with outer timeouts and owned-process cleanup. Use local HTTP and synthetic credentials only.
Task 03 also runs the merged executor, runtime-environment, configuration, and lifecycle regressions.
Native Windows must execute environment, helper, and job-lifecycle tests. Linux cross-compilation cannot replace that evidence.

## E2E tests

The implementation extends `apps/web/e2e/tests/git/fork-pr-comparison-target.spec.ts` in `chromium` and `mobile-fork-pr-comparison-target.spec.ts` in `mobile-chrome`.
Reuse `fork-pr-comparison-target-helpers.ts`, adding real authentication-failure fixture behavior.
Desktop opens Changes through its session tab. Phone taps Changes and uses the existing mobile panel.
Both retain the existing unavailable-target notice and can open local file changes before authentication recovers.
No rendered UI change is proposed, so no new composition or ASCII preview is required.
Task 04 owns exact installation, build, and guarded runner commands. Run the two projects sequentially.

## Companion packages

The merged host bridge package above adds compatibility inputs for Tasks 01, 03, and 05. Its completed statuses and historical results remain unchanged.

[Git admission](../git-subprocess-admission/plan.md), [Noninteractive comparison Git](../noninteractive-comparison-target-git/plan.md), and [Fork comparison targets](../fork-pr-comparison-targets/plan.md) are prior implemented packages.
Their recorded checks remain historical evidence for their original scope.
This package owns the broader runner enforcement and additional authentication-failure scenarios; it does not relabel their old results as new validation.
Workspace scalability, dependency exclusion, and mixed-change packages have no changed task scope.

## Work orders

- [x] [Task 01: Enforce final Git prompt policy](task-01-prompt-policy.md)
- [x] [Task 02: Bound Git helpers and streaming cleanup](task-02-owned-cleanup.md)
- [x] [Task 03: Cover network callers and failure recovery](task-03-network-budgets.md)
- [x] [Task 04: Prove desktop and phone task access](task-04-comparison-recovery.md)
- [x] [Task 05: Document Git failure and recovery](task-05-guidance.md)

## Verification results

Package authoring: temporary PTY mechanism reproduced as recorded above.
Implementation: completed on the verified PR #3635 descendant. Focused backend packages, the shared subprocess race test, web lint, Vite build, TypeScript typecheck, and Linux build passed.
The full backend `make lint` command passes after updating pre-existing Office test fixtures to discard the unused `QueueRun` outcome.
Cross-platform evidence: Windows subprocess, agentctl, and common lifecycle test binaries cross-compiled successfully. Native Windows execution was unavailable in this workspace, so the native Windows requirement remains a platform CI responsibility.
Browser evidence: the desktop Chromium and mobile Chrome comparison-recovery specs passed against the disposable local HTTP authentication fixture.
Documentation: public-doc validation, specification catalog validation, specification lint, and `git diff --check` passed.

## Risks

- Windows suspended-start job handling and Git shell askpass need native execution evidence.
- Caller process attributes may conflict with session isolation; fail before start rather than weakening ownership.
- Arbitrary custom helpers may ignore prompt controls or escape process ownership; never kill shared daemons to compensate.
- Constructor audit alone cannot prove direct Start coverage. Task 02 covers the lifecycle bypasses.
- Previously implicit clone/push lifetimes become finite; preserve established budgets and document uncovered defaults.
- The original incident call site remains unknown; universal execution-boundary coverage addresses the confirmed defect without assigning blame.

Package reconciliation with merged PR #3635: source and contract comparison completed; the implementation consumes the merged host bridge environment without adding a credential or transport fallback.
