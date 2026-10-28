---
status: draft
system: executors
created: 2026-10-28
owners:
  - kandev
---

# Remote executor plugins: native worker recovery and proof requirements

## Overview

Continuation of [native worker capabilities](remote-executor-plugin-native-workers.md): destroy- and recover-scoped checkpoints (REQ-EXECUTORS-NATIVE-WORKER-006, moved here for the file size limit with its IDs unchanged), a backend restart during launch resumes the admitted operation, and a provider-neutral crash barrier lets the fixture provider prove every capability through the real plugin process. The terms of that document apply. Both are opt-in; a provider declaring none of these capabilities keeps today's behavior.

## Requirements

### REQ-EXECUTORS-NATIVE-WORKER-006: Destroy- and recover-scoped checkpoints

**Intent:** Provider progress during cleanup and recovery is recorded durably under the same fences as launch.

#### Acceptance criteria

- **AC-EXECUTORS-NATIVE-WORKER-006.1:** When a provider requires `destroy_checkpoints`, the host shall accept `CheckpointExecutorResource` while that provider holds an active cleanup claim or recovery claim, with phases `cleanup_progress` and `recovery_progress`. The phase `drain_receipt` shall be writable only by the host.
- **AC-EXECUTORS-NATIVE-WORKER-006.2:** The host shall validate each such callback against the active claim (the environment and the claim's operation ID, the `cleanup_claim` that destroy requests already carry), the callback's operation ID, the environment, the ownership generation, the dispatch generation, and the plugin envelope revision. A callback from a released claim, a replaced generation, or a superseded dispatch shall be rejected and shall not change inventory.
- **AC-EXECUTORS-NATIVE-WORKER-006.3:** Provider state in a checkpoint shall pass the closed resource-state schema, shall fit the 32 KiB envelope, and shall contain no secret field.
- **AC-EXECUTORS-NATIVE-WORKER-006.4:** A cleanup claim shall accept only the phase `cleanup_progress` and a recovery claim only `recovery_progress`. Each callback shall carry a provider `sequence` that is greater than the last accepted one for that claim. A callback with an equal sequence and an equal payload digest shall succeed without a write; with a different digest, a lower sequence, an unknown phase, or a missing sequence it shall be rejected (`conflict` for the first two, `invalid_config` for the rest) and change nothing.
- **AC-EXECUTORS-NATIVE-WORKER-006.8:** A callback shall be checked in this order: shape (`invalid_config`), claim identity (`not_found`), sequence (an equal sequence with an equal digest succeeds without a write even if the revision has moved; a lower sequence or a different digest is `conflict`), then the revision CAS. The claim kind (`cleanup` or `recovery`), last sequence, and digest shall be recorded in the envelope in the transaction that takes the claim; the claim table gains no column.
- **AC-EXECUTORS-NATIVE-WORKER-006.5:** When two callbacks observe the same revision, exactly one shall commit and the other shall receive `conflict`. This shall hold on SQLite and PostgreSQL.
- **AC-EXECUTORS-NATIVE-WORKER-006.6:** The host shall settle an environment absent only after the provider confirms absence and the latest accepted checkpoint has been persisted. A missing checkpoint shall never be read as absence.
- **AC-EXECUTORS-NATIVE-WORKER-006.7:** After a crash between a provider side effect and its checkpoint, recovery shall re-issue destroy idempotently from the recorded state, and the provider shall return progress or confirmed absence. The host shall not allocate a replacement.

Requirements 007 (durable bootstrap continuation and ready publication) and 008 (crash barrier and fixture proof) are in [native worker recovery and proof](remote-executor-plugin-native-workers-recovery.md); they keep their IDs.

### REQ-EXECUTORS-NATIVE-WORKER-007: Durable bootstrap continuation and ready publication

**Intent:** A backend restart during launch resumes the admitted operation instead of destroying or duplicating it.

#### Acceptance criteria

- **AC-EXECUTORS-NATIVE-WORKER-007.1:** When a provider requires `durable_bootstrap`, the host shall persist the bootstrap nonce as a host-side secret reference before the provision call, and the agentctl authorization the same way before instance creation. Neither value shall appear in the inventory envelope, a checkpoint, progress, a log, provider state, or a browser response.
- **AC-EXECUTORS-NATIVE-WORKER-007.2:** When the backend restarts with a persisted nonce and no agentctl authorization, recovery shall repeat the authenticated handshake under the original operation. When agentctl accepts it (the nonce is unspent), recovery shall create or adopt the session's instance by verifying owner, workspace descriptor, and port, and continue to readiness without allocating a second environment.
- **AC-EXECUTORS-NATIVE-WORKER-007.3:** The host shall publish an environment from `creating` to `ready` in one guarded transaction under the recovery claim, after verifying the original operation, task and session, environment owner and generation, runtime identity, workspace descriptor, and repository inventory. Repeating publication shall be idempotent, and a mismatch shall publish nothing, adopt nothing, and retain the environment with a named reason.
- **AC-EXECUTORS-NATIVE-WORKER-007.4:** A canceled recovery or a temporary transport failure shall leave the environment `creating` and retryable while its owner exists.
- **AC-EXECUTORS-NATIVE-WORKER-007.5:** When agentctl rejects the repeated handshake as consumed, the nonce was spent but the authorization was not persisted, and recovery shall run cleanup with reason `bootstrap_consumed`. The accept or reject reply of agentctl is the only evidence that separates this case from AC-EXECUTORS-NATIVE-WORKER-007.2; the host also records `handshake_started` before each handshake for diagnosis. Recovery shall not allocate a replacement and shall not publish `ready`.
- **AC-EXECUTORS-NATIVE-WORKER-007.9:** An archive, delete, or reset that no-ops because recovery holds the claim shall record `cleanup_requested` in the envelope, and the existing cleanup sweep shall run the cleanup after recovery releases the claim, so the lost race loses no cleanup.
- **AC-EXECUTORS-NATIVE-WORKER-007.6:** When archive, delete, or reset races recovery for one environment, exactly one shall hold the claim; the other shall no-op, and no cleanup state shall be lost.
- **AC-EXECUTORS-NATIVE-WORKER-007.7:** When a provider does not require `durable_bootstrap`, a non-ready record without saved authorization shall be cleaned up as `incomplete_bootstrap` exactly as today.

### REQ-EXECUTORS-NATIVE-WORKER-008: Provider-neutral crash barrier and fixture proof

**Intent:** Every capability is proven through the real plugin process boundary without a vendor.

#### Acceptance criteria

- **AC-EXECUTORS-NATIVE-WORKER-008.1:** The host shall expose named crash barriers at: after provider side effect before checkpoint, after drain before destroy, after handshake before authorization persistence, before ready publication, after destroy confirmation before owner release, after the pending placement is persisted, before a placement poll commits, after a cancel is marked, and after a migration step is recorded. Names shall be provider-neutral.
- **AC-EXECUTORS-NATIVE-WORKER-008.2:** Barriers shall be armable only when `KANDEV_E2E_MOCK=true`. With any other profile, arming shall be refused and barrier points shall have no effect.
- **AC-EXECUTORS-NATIVE-WORKER-008.3:** An armed barrier shall fire at most once per arming, for the first caller that reaches it, and shall either block until released or terminate the backend process, as the test selects. Arming a name that is already armed shall be refused with `conflict`, and releasing a barrier that is not armed shall do nothing. After termination, a restarted backend shall recover under REQ-EXECUTORS-NATIVE-WORKER-006 and 007.
- **AC-EXECUTORS-NATIVE-WORKER-008.4:** The packaged fixture provider shall declare every capability in AC-EXECUTORS-NATIVE-WORKER-001.1 and a conformance test through the managed plugin process shall exercise each capability and each barrier.
- **AC-EXECUTORS-NATIVE-WORKER-008.5:** The fixture shall use a local test transport and require no network, cluster, or credentials.

## Out of scope

- Provider-specific recovery and native-state migration.
- A vendor harness; it lives in the vendor plugin repository.

## Implementation plans

- [Native worker capabilities for executor plugins](../../../plans/executor-plugin-native-workers/plan.md)
