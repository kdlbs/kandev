---
status: active
system: agents
created: 2026-08-24
owners:
  - kandev
---

# Managed npm runtime recovery requirements

## Overview

Managed npm runtimes use exact reviewed package versions. Stale npm metadata
can hide a published version and stop the agent before ACP initialization.

Kandev retries recoverable setup failures without requiring the end user to operate npm. Strict ETARGET recovery applies to host capability probes and task launches.
Broader bounded setup recovery applies to task launches on local PC, local Docker,
and remote SSH executors.

Managed runtime package resolution is independent of the task repository's
project npm configuration. A repository release-age policy must not prevent an
operator-selected, successfully validated runtime from starting.

## Terminology

- **Managed npm runtime:** A built-in ACP runtime that Kandev launches through an exact npm package version.
- **Execution tree:** The deterministic npm `_npx` directory for one exact package specification.
- **Executor-local:** An operation that runs in the environment that hosts the agent process and its npm cache.

## Requirements

### REQ-AGENTS-MANAGED-RUNTIME-RECOVERY-001: Transparent executor recovery

**Intent:** Restore a managed runtime after stale npm metadata without changing the selected package, version, executor, or session.

**User story:** As a Kandev user, I want runtime repair to occur automatically, so that I do not operate npm on an execution host.

#### Acceptance criteria

- **AC-AGENTS-MANAGED-RUNTIME-RECOVERY-001.1:** When a supported executor reports strict npm `ETARGET` evidence before ACP initialization, Kandev shall retry the same runtime once.
- **AC-AGENTS-MANAGED-RUNTIME-RECOVERY-001.2:** The supported executors shall be local PC, local Docker, and remote SSH.
- **AC-AGENTS-MANAGED-RUNTIME-RECOVERY-001.3:** A successful retry shall continue the original session without a failure card or user action.
- **AC-AGENTS-MANAGED-RUNTIME-RECOVERY-001.4:** The retry shall preserve the trusted package, exact version, registry, command prefix, ACP arguments, model, permissions, executor, and session identity.
- **AC-AGENTS-MANAGED-RUNTIME-RECOVERY-001.5:** When the retry fails, Kandev shall report the npm preparation error and offer one **Retry runtime** action.
- **AC-AGENTS-MANAGED-RUNTIME-RECOVERY-001.6:** When a host capability probe reports the same strict npm `ETARGET` evidence, Kandev shall retry the same probe once with online-preferred metadata, without deleting its shared execution tree before it publishes a failed capability status.
- **AC-AGENTS-MANAGED-RUNTIME-RECOVERY-001.7:** Capability-probe recovery and failure shall not change a persisted profile's selected model, fallback model, mode, active runtime version, or enabled state.
- **AC-AGENTS-MANAGED-RUNTIME-RECOVERY-001.8:** When the host capability-probe retry succeeds, Kandev shall publish the recovered capability catalogue without requiring a task launch, restart, or user action.

### REQ-AGENTS-MANAGED-RUNTIME-RECOVERY-002: Scoped executor-local repair

**Intent:** Repair only the cache entry that belongs to the failed trusted runtime.

#### Acceptance criteria

- **AC-AGENTS-MANAGED-RUNTIME-RECOVERY-002.1:** Kandev shall resolve the npm cache with the failed agent process environment on its execution host.
- **AC-AGENTS-MANAGED-RUNTIME-RECOVERY-002.2:** When an explicit maintenance operation removes cached files, Kandev shall remove only the deterministic execution tree for the trusted exact package specification. Automatic startup and capability-probe retries shall not delete an execution tree.
- **AC-AGENTS-MANAGED-RUNTIME-RECOVERY-002.3:** Kandev shall preserve the configured npm registry, the global npm cache, and unrelated execution trees.
- **AC-AGENTS-MANAGED-RUNTIME-RECOVERY-002.4:** Cache repair shall reject broad roots, path-like package values, symbolic links, and paths outside the npm execution cache.
- **AC-AGENTS-MANAGED-RUNTIME-RECOVERY-002.5:** Cancellation and backend shutdown shall stop repair before a replacement process starts.

### REQ-AGENTS-MANAGED-RUNTIME-RECOVERY-003: Project-independent resolution and policy errors

**Intent:** A task repository shall not control how Kandev resolves its managed agent runtime, and a remaining npm release-date restriction shall have an actionable failure.

#### Acceptance criteria

- **AC-AGENTS-MANAGED-RUNTIME-RECOVERY-003.1:** When Kandev prepares, probes, or launches a built-in managed npm runtime, the task repository's project `.npmrc`, including `min-release-age` and `before`, shall not affect runtime package resolution. The npm project root shall stay outside the task workspace and mounted agent session state. This shall hold for host utility probes and local PC, local Docker, and remote SSH launches.
- **AC-AGENTS-MANAGED-RUNTIME-RECOVERY-003.2:** Isolation shall preserve the agent's workspace working directory, exact selected package and version, configured registry, explicit npm environment overrides, ACP arguments, and existing npm execution-cache identity. Native and passthrough commands shall remain unchanged.
- **AC-AGENTS-MANAGED-RUNTIME-RECOVERY-003.3:** When npm reports a release-date-qualified `ETARGET` for the exact selected managed package from configuration that still applies, during startup, a capability probe, or a Settings update, Kandev shall report a distinct, actionable npm policy failure. It shall not invalidate the execution cache or attempt an online-preferred retry for that failure.
- **AC-AGENTS-MANAGED-RUNTIME-RECOVERY-003.4:** The release-date failure shall retain bounded, sanitized technical details and one recovery explanation on desktop and phone. The explanation shall identify `min-release-age` or `before` as settings to check without claiming that cache repair was attempted. A Settings update error shall identify the policy without exposing the raw npm date. Unrelated packages, malformed diagnostics, and generic ACP disconnects shall not receive this classification.

### REQ-AGENTS-MANAGED-RUNTIME-RECOVERY-004: Bounded startup recovery

**Intent:** Bursts of managed agent launches recover from temporary setup failures without leaving otherwise recoverable sessions FAILED.

#### Acceptance criteria

- **AC-AGENTS-MANAGED-RUNTIME-RECOVERY-004.1:** Before conversation creation or restoration, a supported managed npm launch shall retry once after a recognized transient npm setup failure. This includes temporary network, registry, installation-contention, and download-integrity failures. The selected package, version, environment, registry, workspace, and session shall remain unchanged.
- **AC-AGENTS-MANAGED-RUNTIME-RECOVERY-004.2:** A managed npm process that exits unexpectedly before the initialize handshake completes shall qualify for the same single retry. Only complete, genuinely empty stderr may use the unexplained-exit path; an unclassified npm code or incomplete diagnostic collection shall prevent automatic retry. A closed-pipe message alone shall not establish eligibility. Unknown process ownership or exit evidence shall prevent automatic retry.
- **AC-AGENTS-MANAGED-RUNTIME-RECOVERY-004.3:** Authentication, authorization, release-age policy, incompatible runtime, unavailable disk space, and invalid configuration failures shall not trigger automatic retry. Intentional stop, cancellation, shutdown, and deadline expiration shall take precedence. Failure after conversation creation, restoration, or prompt dispatch shall not use this setup retry.
- **AC-AGENTS-MANAGED-RUNTIME-RECOVERY-004.4:** All eligible setup failure categories shall share one retry budget per launch operation. Retry shall use a short randomized delay and the existing deadline. Confirmed cleanup of the failed process tree shall precede replacement. Cleanup failure shall prevent replacement. Concurrent healthy launches shall remain independent.
- **AC-AGENTS-MANAGED-RUNTIME-RECOVERY-004.5:** During recovery, the task and session shall retain their startup state and expose retry progress. Success shall continue the same session and send its accepted prompt once. Exhaustion shall produce one actionable failure with the sanitized final cause and attempt count, while preserving authentication recovery and normal later-phase failure handling. Desktop and phone shall provide the same recovery outcome.
- **AC-AGENTS-MANAGED-RUNTIME-RECOVERY-004.6:** Automatic recovery shall preserve shared npm execution trees and sibling processes. An exact-package ETARGET retry shall refresh online metadata without Kandev deleting the shared tree. Capability-probe ETARGET recovery shall follow the same non-destructive rule.
- **AC-AGENTS-MANAGED-RUNTIME-RECOVERY-004.7:** Delayed exit, error, and disconnect events from a failed attempt shall not fail or stop its replacement. Concurrent launch tests shall include a healthy sibling, a recovered sibling, and an exhausted sibling.
- **AC-AGENTS-MANAGED-RUNTIME-RECOVERY-004.8:** Diagnostics shall distinguish npm setup failure from an unexplained early process exit. They shall record attempt, outcome, and bounded exit evidence without exposing credentials or registry URLs. Final UI text shall not attribute an unexplained exit to npm or claim unperformed cache repair.

## Out of scope

- Automatic version rollback or selection of another package version.
- Registry replacement, dependency substitution, or global npm cache cleanup.
- Native runtimes, passthrough commands, unclassified npm errors without eligible early-exit evidence, and a second automatic retry.
- Automatic cache repair and retry for Sprites, remote Docker, Kubernetes, and future executors without a validated executor-local launch and stop contract. Kandev still classifies a policy failure when bounded diagnostics are available for the exact trusted package.

## Implementation package

- [Managed npm startup resilience](../../../plans/managed-npm-startup-resilience/plan.md)
