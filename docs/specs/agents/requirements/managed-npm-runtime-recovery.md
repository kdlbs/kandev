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

Kandev repairs this error without requiring the end user to operate npm. The
same behavior applies to host capability probes and to agent launches on local
PC, local Docker, and remote SSH executors.

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
- **AC-AGENTS-MANAGED-RUNTIME-RECOVERY-001.6:** When a host capability probe reports the same strict npm `ETARGET` evidence, Kandev shall repair the trusted execution tree and retry the same probe once with online-preferred metadata before it publishes a failed capability status.
- **AC-AGENTS-MANAGED-RUNTIME-RECOVERY-001.7:** Capability-probe recovery and failure shall not change a persisted profile's selected model, fallback model, mode, active runtime version, or enabled state.
- **AC-AGENTS-MANAGED-RUNTIME-RECOVERY-001.8:** When the host capability-probe retry succeeds, Kandev shall publish the recovered capability catalogue without requiring a task launch, restart, or user action.

### REQ-AGENTS-MANAGED-RUNTIME-RECOVERY-002: Scoped executor-local repair

**Intent:** Repair only the cache entry that belongs to the failed trusted runtime.

#### Acceptance criteria

- **AC-AGENTS-MANAGED-RUNTIME-RECOVERY-002.1:** Kandev shall resolve the npm cache with the failed agent process environment on its execution host.
- **AC-AGENTS-MANAGED-RUNTIME-RECOVERY-002.2:** Kandev shall remove only the deterministic execution tree for the trusted exact package specification.
- **AC-AGENTS-MANAGED-RUNTIME-RECOVERY-002.3:** Kandev shall preserve the configured npm registry, the global npm cache, and unrelated execution trees.
- **AC-AGENTS-MANAGED-RUNTIME-RECOVERY-002.4:** Cache repair shall reject broad roots, path-like package values, symbolic links, and paths outside the npm execution cache.
- **AC-AGENTS-MANAGED-RUNTIME-RECOVERY-002.5:** Cancellation and backend shutdown shall stop repair before a replacement process starts.

### REQ-AGENTS-MANAGED-RUNTIME-RECOVERY-003: Project-independent resolution and policy errors

**Intent:** A task repository shall not control how Kandev resolves its managed agent runtime, and a remaining npm release-date restriction shall have an actionable failure.

#### Acceptance criteria

- **AC-AGENTS-MANAGED-RUNTIME-RECOVERY-003.1:** When Kandev prepares, probes, or launches a built-in managed npm runtime, the task repository's project `.npmrc`, including `min-release-age` and `before`, shall not affect runtime package resolution. This shall hold for host utility probes and local PC, local Docker, and remote SSH launches.
- **AC-AGENTS-MANAGED-RUNTIME-RECOVERY-003.2:** Isolation shall preserve the agent's workspace working directory, exact selected package and version, configured registry, explicit npm environment overrides, ACP arguments, and existing npm execution-cache identity. Native and passthrough commands shall remain unchanged.
- **AC-AGENTS-MANAGED-RUNTIME-RECOVERY-003.3:** When npm reports a release-date-qualified `ETARGET` for the exact selected managed package from configuration that still applies, during startup, a capability probe, or a Settings update, Kandev shall report a distinct, actionable npm policy failure. It shall not invalidate the execution cache or attempt an online-preferred retry for that failure.
- **AC-AGENTS-MANAGED-RUNTIME-RECOVERY-003.4:** The release-date failure shall retain bounded, sanitized technical details and one recovery explanation on desktop and phone. The explanation shall identify `min-release-age` or `before` as settings to check without claiming that cache repair was attempted. A Settings update error shall identify the policy without exposing the raw npm date. Unrelated packages, malformed diagnostics, and generic ACP disconnects shall not receive this classification.

## Out of scope

- Automatic version rollback or selection of another package version.
- Registry replacement, dependency substitution, or global npm cache cleanup.
- Native runtimes, passthrough commands, unrelated npm errors, and a second online retry.
- Automatic cache repair and retry for Sprites, remote Docker, Kubernetes, and future executors without the same authenticated executor-local repair contract. Kandev still classifies a policy failure when bounded diagnostics are available for the exact trusted package.
