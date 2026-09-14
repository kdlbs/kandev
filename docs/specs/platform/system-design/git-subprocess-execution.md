---
status: current
system: platform
created: 2026-09-13
requirements:
  - REQ-PLATFORM-GIT-SUBPROCESS-ADMISSION-002
owners:
  - kandev
---

# Managed Git execution

## Purpose and boundaries

Platform owns this contract because admission, subprocess lifetime, and prompt suppression serve all application Git callers.
Credential selection remains with the [GitHub authentication design](../../integrations/system-design/github-authentication-02.md).
Comparison state remains with [Workspace Git Status](workspace-git-status.md).
This design applies the accepted transport ADR; it does not select a new credential authority.

## Requirement mapping

| Criteria under REQ-PLATFORM-GIT-SUBPROCESS-ADMISSION-002 | Design section |
| --- | --- |
| .1, .2 | Final preparation |
| .3, .4 | Execution and cleanup |
| .5, .6 | Failure and recovery |

## Final preparation

`internal/common/subproc` owns final preparation immediately before Git starts.
All classified Run, Output, CombinedOutput, and AfterAcquire variants apply the same policy.
`NewGitCommand` remains the construction seam. Constructor defaults cannot enforce this contract because callers replace `cmd.Env`.
Manually admitted commands use the same preparation and lifecycle helper before Start or Run.
`GitInteractive` selects admission priority; it does not allow credential interaction.

Preparation copies `cmd.Environ()`. An explicit empty environment stays explicit; only a nil environment inherits the parent.
Collapse duplicate assignments of the same environment key deterministically, using the last value and platform environment-key comparison rules.
Different indexed Git entries can intentionally repeat the same configuration key. Preserve their values and order, including empty helper resets.
Override prompt keys after that normalization. Preserve unrelated entries, working-directory behavior, stdin, and indexed `GIT_CONFIG_*` entries.
Never add ambient credentials to an explicit instance environment or reset a credential-helper chain.

PR #3635's host bridge is part of the selected executor environment, not a new fallback introduced by prompt preparation.
Preserve its `kandev-host-gh-bridge` marker, quoted absolute CLI executable, and inherited helpers before it.
Explicit public/enterprise tokens retain precedence. Managed helper failure must never invoke this optional host route.
Remote executors and unrelated hosts remain outside host-bridge eligibility.
The integration layer owns HOME/GH_CONFIG_DIR/XDG_CONFIG_HOME selection and source-aware indexed composition.
Consume the resulting environment without re-probing credentials or merging it with the ambient host again.
Preserve agentctl overlay versus complete-block replacement, including intentional removal of a previous generated bridge.
SSH command normalization must not inspect or rewrite credential-helper shell functions.

Enforce `GIT_TERMINAL_PROMPT=0`, `GCM_INTERACTIVE=Never`, and `GCM_GUI_PROMPT=0`.
Set Git and SSH askpass to a deny-only command which emits no credential and exits unsuccessfully.
Use a shell-builtin denial (`exit 1`) through Git's shell command handling, including Git for Windows.
Native Windows tests must prove invocation and denial; a Unix `/bin/false` path is not the Windows contract.
Set `SSH_ASKPASS_REQUIRE=never` as defense in depth. Do not use `echo` as an artificial credential.
Prompt preparation must be idempotent, including SSH normalization.

Move the existing direct-OpenSSH normalization from `workspace_git_cmd.go` into the shared layer.
Preserve quoted executable paths, identities, proxy options, and host configuration.
Place `BatchMode=yes` before inherited options. Repeated preparation must not append repeated batch options.
Resolve direct `GIT_SSH` executables without silently discarding their path when `GIT_SSH_COMMAND` is absent.
Unsupported shell wrappers retain the ADR's safe-default behavior. Do not build another general shell parser.
Do not disable host-key verification or accept unknown hosts automatically.

## Execution and cleanup

Operation owners select finite budgets. Existing AfterAcquire budgets remain authoritative.
Migrate uncovered network callers to builders so queue time does not consume the command budget.
Keep earlier parent deadlines and distinguish admission cancellation from execution failure.
Do not impose the existing 30-second generic helper default on every clone or push.

Shared Git lifecycle support must own Start, cancellation, and Wait, including output capture variants.
On Unix, detach ordinary managed Git into its own session without a controlling terminal.
Its process group is owned by the command and can be terminated on cancellation.
Preserve compatible attributes, including Linux parent-death behavior. Reject incompatible terminal/group attributes before starting; never target the launcher's group.
Compose caller cancellation cleanup rather than silently discarding it.

Windows uses a kill-on-close Job Object installed while Git is suspended, before descendants can start.
Reuse the implementation in `internal/agentctl/server/winproc` by moving the required generic lifecycle code to a common package.
Keep compatibility wrappers for existing agentctl consumers. The shared layer must not import the agentctl server.
Job installation failure kills the owned suspended process and returns an error; it cannot leave a suspended orphan.

Use a maximum 500 ms pipe WaitDelay, preserving a shorter caller value.
Bound owned-tree termination/reaping to two seconds, then return any cleanup failure.
Normal completion also closes owned lifecycle resources. Never terminate shared credential daemons or unrelated shells.
A helper which deliberately escapes ownership is outside process-tree containment; deadline and pipe closure still bound the caller.

`capDiffOutput` must close its reader when execution is canceled, including while draining with `io.Copy` before Wait.
Keep output caps, partial-output semantics, and exactly-once admission release.
The descriptor-based `task/gitinit` ExecGit path remains a local-only exception with its existing process ownership and descriptor security.
The source audit records that narrow exception and rejects new unprepared direct execution paths.

## Failure and recovery

Keep existing API/WS shapes, error wrapping, redaction, locks, singleflight cleanup, and cooldown behavior.
Required operations finish with their existing error result. Optional materialization publishes unavailable comparison data while local Changes remains usable.
Desktop and phone retain their current navigation and unavailable notice. No new rendered UI is planned.
Never substitute a same-named local comparison ref after remote failure.

Each later operation resolves its effective credentials through the existing authority.
No global failure latch, token mutation, new retries, or transport changes are added.
An already-installed host helper reads its selected credential store on later commands and can recover within the same instance.
If optional bridge registration was skipped, launch, resume, or prepared-start re-evaluates eligibility through the existing integration path.
This requires no backend restart, but does not imply automatic helper installation on every Git command.
Provider request failure alone does not establish token revocation.
An explicit fresh Git-status request may schedule one new attempt for an unavailable comparison target, so a recovered credential service can restore comparison data without a backend restart. This is a user-triggered re-evaluation, not a timer or retry loop.
When that attempt refreshes a moving comparison target, Git may force-update only the deterministic internal comparison ref for that exact target branch. It never force-updates `origin`, the checkout upstream, or the push route.

## Persistence and observability

No schema, runtime flag, startup setting, or persisted failure state changes.
Existing admission metrics continue to distinguish queueing from execution.
Logs and returned errors must exclude fixture credentials and real tokens.

## Related decisions and delivery

- [Deterministic noninteractive transport](../../../decisions/2026-08-31-deterministic-noninteractive-git-transport.md)
- [Class-aware admission](../../../decisions/2026-08-02-class-aware-git-subprocess-admission.md)
- [Implementation plan](../../../plans/noninteractive-git-execution/plan.md)

- [Merged host bridge contract at PR #3635](https://github.com/kdlbs/kandev/blob/9cc146ee21c296d553ae684531219a5c711a4213/docs/specs/integrations/system-design/github-authentication-02.md#executor-host-cli-bridge)
