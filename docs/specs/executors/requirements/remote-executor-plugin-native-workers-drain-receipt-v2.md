---
status: draft
system: executors
created: 2026-10-31
owners:
  - kandev
---

# Remote executor plugins: drain receipt version 2 requirements

## Overview

Continuation of [native worker capabilities](remote-executor-plugin-native-workers.md) (REQ-EXECUTORS-NATIVE-WORKER-005). Version 1 of `drain_receipt` sends `not_attempted` only for a failed launch that never started an agentctl handshake, so every other cleanup of an environment whose host never obtained a credential is `unreachable` and a provider must retain the environment. Version 2 reuses the same value with a wider, still positive, meaning. A provider and a host that negotiate version 1 behave exactly as before. The terms of the native worker document apply.

## Requirements

### REQ-EXECUTORS-NATIVE-WORKER-010: Drain receipt version 2

**Intent:** An environment whose host never held an agentctl credential is cleaned up and released, instead of staying quarantined for a runtime that provably cannot exist.

#### Acceptance criteria

- **AC-EXECUTORS-NATIVE-WORKER-010.1:** The capability `drain_receipt` shall accept versions 1 and 2. A provider declares exactly one version in its manifest, the echo of the provider's profile validation shall name that same version, and the host shall pin the declared version in the environment's negotiated set. A host shall decide every drain outcome from the version pinned in the stored record, never from the current registry or manifest.
- **AC-EXECUTORS-NATIVE-WORKER-010.2:** Under version 2, `not_attempted` shall mean that the host never held an agentctl credential for this runtime, so no agentctl instance, agent or shell can have been created through agentctl. It shall not mean that nothing was provisioned: the provider's resource, its allocations and its worker state may exist, and the provider shall still verify them before it reports absence.
- **AC-EXECUTORS-NATIVE-WORKER-010.3:** Under version 2, when no credential is available and no handshake was ever started, the host shall send `not_attempted` for every cleanup reason (`task_cleanup`, `task_environment_reset`, `recovery_cleanup`, `bootstrap_consumed`, `incomplete_bootstrap`, `launch_failed` and the launch-stage reasons), and shall persist the outcome with the cleanup claim before the destroy call.
- **AC-EXECUTORS-NATIVE-WORKER-010.4:** Under version 2 with `durable_bootstrap` negotiated, when no credential is available, no credential was ever recorded and no handshake was ever rejected, the host shall send `not_attempted` even though a handshake was started, because a handshake that failed before a token existed (end of stream, deadline, refused connection, or a crash before the token was persisted) left the host without a credential.
- **AC-EXECUTORS-NATIVE-WORKER-010.5:** The host shall record two sticky envelope fields: `credential_recorded`, set in the same checkpoint that records the reference to the agentctl token secret, and `handshake_rejected`, set when agentctl rejects a handshake (the bootstrap nonce is already spent). Neither field is ever cleared. A version 2 environment records `credential_recorded` as `false` when it is admitted; a record without `credential_recorded` is not known to be credential-free and is treated as strict (AC-EXECUTORS-NATIVE-WORKER-010.6).
- **AC-EXECUTORS-NATIVE-WORKER-010.6:** The host shall stay strict, sending `unreachable` and never `not_attempted`, when `handshake_rejected` is set, when `credential_recorded` is set but the token secret is missing or unreadable, when the provider did not negotiate `durable_bootstrap` and a handshake was started, and for a record without `credential_recorded` that has a handshake started. The host shall not infer these states from metadata.
- **AC-EXECUTORS-NATIVE-WORKER-010.7:** Under version 2 a provider shall accept `not_attempted` with a non-empty cleanup claim for any cleanup reason as proof that no agentctl runtime exists, and shall skip only the drain. It shall still require that the job or resource is this operation's and stopped, every allocation is terminal, the node contract holds, the worker state is removed and terminal evidence is present, and it shall quarantine the environment when any of them fails or when the host sends `unreachable`, `uncertain` or `drained` without a valid receipt. A provider that negotiated version 1 shall keep the narrow rule of REQ-EXECUTORS-NATIVE-WORKER-005.
- **AC-EXECUTORS-NATIVE-WORKER-010.8:** Under version 1 the host and the provider shall behave exactly as before this requirement, for new and for retained records.
