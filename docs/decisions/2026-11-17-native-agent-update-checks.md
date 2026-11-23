# ADR-2026-11-17-native-agent-update-checks: Let Agents Declare Native Update Checks

**Status:** proposed
**Date:** 2026-11-17
**Area:** backend

## Context

Runtime update status uses npm or GitHub release metadata and stable-only
SemVer comparison. For a harness-owned updater, such as Oh My Pi (`omp`), that
metadata cannot represent the harness's configured channel or installation
method. A canary installation newer than the stable release is reported as
`unknown`. A canary installation behind the canary release is not detected.

[ADR-2026-09-26](2026-09-26-harness-owned-runtime-updates.md) rejected parsing
`omp update --check` because its exit status does not distinguish outcomes and
human-readable output is a fragile contract. Kandev maintainers have since asked
the agent runtime to use the check that the harness provides
([kdlbs/kandev#5156](https://github.com/kdlbs/kandev/issues/5156)). The updater
already resolves the configured channel and installation method, and its check
mode returns before it acquires an install lock or changes the installation.
Like every `omp` start, it still initializes settings and local state. The
`omp acp` probe that Kandev already runs has the same side effects.

## Decision

A built-in agent can declare an optional **native update check**: trusted argv
and a result adapter in built-in agent metadata. The check capability is
separate from permission to install or update automatically.

- Agents without a native check keep the existing npm and GitHub lookup.
- For an agent that declares a native check, the check is the only source of
  update status and preview latest-version information. Kandev does not also
  query npm stable metadata, and it does not pass native results through the
  stable-only comparator.
- Kandev runs the check directly, without a shell, on the Kandev host. It uses
  the same environment and executable resolution as the agent's trusted update
  command and does not add channel or installation flags. Execution has a
  timeout, an output limit that stops the run, cancellation, and process-tree
  containment, because the checker can start package-manager probes.
- The adapter returns `update_available`, `up_to_date`, or `unknown` and
  reports current version, latest version, and channel when the output contains
  them. Exit status alone never determines availability. A result that cannot
  be recognized, conflicts with itself, or describes a different installation
  from the observed runtime is `unknown` with a stable reason code.
- Native results use the shared status cache, single-flight lookup, scheduler,
  and notification pipeline. Their cache identity includes the agent, check
  argv, resolved executable, observed runtime version, an agent-declared
  update-configuration fingerprint, and an invalidation generation. A check
  that started before an update job ended cannot populate the cache.
- Installation remains a user-approved maintenance job that runs the agent's
  existing update command, followed by the existing ACP probe.

Oh My Pi declares `omp update --check`. Its adapter recognizes upstream text
markers and treats any other output as `unknown`.

## Consequences

Status follows the channel that `omp update` would install, including canary
prereleases. The update notification and Settings action use the existing
paths without a separate update mechanism.

Status now depends on the harness's human-readable output. Upstream wording
changes degrade to `unknown` with a reason. They never produce `up_to_date`
or an incorrect version. Adapter tests and upstream source references are
part of the contract. Agents that add native checks need the same tests.

Status requests can start a host process on a cache miss. Process limits and
the shared cache restrict this to one check for each agent and configuration in
each cache window. Checks run only for enabled agents that are available on
the host.

This decision supersedes the status-lookup part of ADR-2026-09-26 for agents
that declare a native check. Its update, probe, no-rollback, and approval rules
remain unchanged.

## Alternatives Considered

- **Keep the npm stable reference.** Rejected because it cannot report
  configured-channel status and leaves current canary installations unknown.
- **Read OMP's channel setting and query npm or the build service directly.**
  Rejected because it duplicates the updater's installation-method and channel
  resolution and can disagree with the result of `omp update`.
- **Require a machine-readable flag from upstream before adding support.**
  This is preferred if one becomes available, and the adapter can then switch to
  it. Waiting would leave the existing gap unsolved.
- **Enable automatic in-place updates when a check reports an update.** Rejected
  by maintainers. Installation remains user-triggered, and declaring a check
  does not grant update authority.
