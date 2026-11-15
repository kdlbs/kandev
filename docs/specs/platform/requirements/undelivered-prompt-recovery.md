---
status: active
system: platform
---

# Undelivered prompt recovery

## Scope

Platform owns delivery admission and recovery evidence. This extends
[durable delivery](durable-agent-delivery.md) for failures before admission.
Existing native recovery remains responsible for work with uncertain effects.

### REQ-PLATFORM-UNDELIVERED-PROMPT-001: Recovery without replay

A failed submission must not permanently prevent use of its conversation when
trusted evidence proves that the agent never admitted it.

#### Acceptance criteria

- **AC-PLATFORM-UNDELIVERED-PROMPT-001.1:** A proven pre-admission rejection shall settle as failed. It shall not create an unknown-outcome block. A missing acceptance response alone shall remain uncertain.
- **AC-PLATFORM-UNDELIVERED-PROMPT-001.2:** Explicit Resume shall use the existing missing-peer retirement contract for eligible SQL-only interrupted submissions. Historical uncertainty shall remain unknown. Incomplete, conflicting, stale, or unavailable evidence shall preserve the block.
- **AC-PLATFORM-UNDELIVERED-PROMPT-001.3:** An open unknown-outcome block shall prevent user sends, automatic retries, queue dispatch, and steering from creating or dispatching another submission. Stop and supported recovery shall remain available.
- **AC-PLATFORM-UNDELIVERED-PROMPT-001.4:** Recovery shall preserve the native conversation and retained output. It shall never resend the failed instruction. Repeated recovery shall have no duplicate effects. A distinct instruction can run after all admission restrictions are satisfied.
- **AC-PLATFORM-UNDELIVERED-PROMPT-001.5:** Desktop and phone shall provide the existing Resume action for eligible recovery. Busy state, failures, and remaining restrictions shall remain visible. Recovery shall not require database editing.

## Compatibility and exclusions

Unknown effects remain unknown. Explicit Resume can retire matching uncertain
journal work through the existing contract. It does not claim that work failed.
Absent proof does not authorize a force-clear operation or replacement conversation.
Office and automation retain their existing recovery authority.
This extension does not repair provider DNS or ACP adapter failures.

## Delivery

[System design](../system-design/undelivered-prompt-recovery.md).
[Implementation plan](../../../plans/undelivered-prompt-recovery/plan.md).
