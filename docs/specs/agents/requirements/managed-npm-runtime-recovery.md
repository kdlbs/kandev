---
status: draft
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
same behavior applies on local PC, local Docker, and remote SSH executors.

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

### REQ-AGENTS-MANAGED-RUNTIME-RECOVERY-002: Scoped executor-local repair

**Intent:** Repair only the cache entry that belongs to the failed trusted runtime.

#### Acceptance criteria

- **AC-AGENTS-MANAGED-RUNTIME-RECOVERY-002.1:** Kandev shall resolve the npm cache with the failed agent process environment on its execution host.
- **AC-AGENTS-MANAGED-RUNTIME-RECOVERY-002.2:** Kandev shall remove only the deterministic execution tree for the trusted exact package specification.
- **AC-AGENTS-MANAGED-RUNTIME-RECOVERY-002.3:** Kandev shall preserve the configured npm registry, the global npm cache, and unrelated execution trees.
- **AC-AGENTS-MANAGED-RUNTIME-RECOVERY-002.4:** Cache repair shall reject broad roots, path-like package values, symbolic links, and paths outside the npm execution cache.
- **AC-AGENTS-MANAGED-RUNTIME-RECOVERY-002.5:** Cancellation and backend shutdown shall stop repair before a replacement process starts.

## Out of scope

- Automatic version rollback or selection of another package version.
- Registry replacement, dependency substitution, or global npm cache cleanup.
- Native runtimes, passthrough commands, unrelated npm errors, and a second online retry.
- Sprites, remote Docker, Kubernetes, and future executors without the same authenticated executor-local repair contract.
