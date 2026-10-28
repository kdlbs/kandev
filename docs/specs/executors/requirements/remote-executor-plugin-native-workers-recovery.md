---
status: draft
system: executors
created: 2026-10-28
owners:
  - kandev
---

# Remote executor plugins: native worker recovery and proof requirements

## Overview

Continuation of [native worker capabilities](remote-executor-plugin-native-workers.md): a backend restart during launch resumes the admitted operation, and a provider-neutral crash barrier lets the fixture provider prove every capability through the real plugin process. The terms of that document apply. Both are opt-in; a provider declaring none of these capabilities keeps today's behavior.

## Requirements

### REQ-EXECUTORS-NATIVE-WORKER-007: Durable bootstrap continuation and ready publication

**Intent:** A backend restart during launch resumes the admitted operation instead of destroying or duplicating it.

#### Acceptance criteria

- **AC-EXECUTORS-NATIVE-WORKER-007.1:** When a provider requires `durable_bootstrap`, the host shall persist the bootstrap nonce as a host-side secret reference before the provision call, and the agentctl authorization the same way before instance creation. Neither value shall appear in the inventory envelope, a checkpoint, progress, a log, provider state, or a browser response.
- **AC-EXECUTORS-NATIVE-WORKER-007.2:** When the backend restarts with a persisted nonce and no agentctl authorization, recovery shall repeat the authenticated handshake under the original operation. When agentctl accepts it (the nonce is unspent), recovery shall create or adopt the session's instance by verifying owner, workspace descriptor, and port, and continue to readiness without allocating a second environment.
- **AC-EXECUTORS-NATIVE-WORKER-007.3:** The host shall publish an environment from `creating` to `ready` in one guarded transaction under the recovery claim, after verifying the original operation, task and session, environment owner and generation, runtime identity, workspace descriptor, and repository inventory. Repeating publication shall be idempotent, and a mismatch shall publish nothing, adopt nothing, and retain the environment with a named reason.
- **AC-EXECUTORS-NATIVE-WORKER-007.4:** A canceled recovery or a temporary transport failure shall leave the environment `creating` and retryable while its owner exists.
- **AC-EXECUTORS-NATIVE-WORKER-007.5:** When agentctl rejects the repeated handshake as consumed, the nonce was spent but the authorization was not persisted, and recovery shall run cleanup with reason `bootstrap_consumed`. The accept or reject reply of agentctl is the only evidence that separates this case from AC-EXECUTORS-NATIVE-WORKER-007.2; the host also records `handshake_started` before each handshake for diagnosis. Recovery shall not allocate a replacement and shall not publish `ready`.
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
