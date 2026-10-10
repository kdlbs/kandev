---
status: draft
system: platform
created: 2026-09-10
updated: 2026-10-10
owners:
  - kandev
---

# Durable agent delivery requirements

## Overview

The platform system owns delivery between the backend and agentctl. This contract does not replace browser subscription recovery or native harness persistence.

This draft defines proposed behavior. It does not claim that the current implementation provides these guarantees.

## Terminology

- Journal: An agentctl disk store for submissions and normalized events, with bounded memory use and explicit storage-exhaustion handling.
- Inbox: Backend SQL records that durably receive journal events.
- Acknowledged cursor: Highest contiguous sequence committed to the backend inbox.
- Projected cursor: Highest contiguous sequence applied to canonical product state.

## Requirements

### REQ-PLATFORM-DURABLE-AGENT-DELIVERY-001: Durable journal

**Intent:** Events that agentctl accepts must survive a supported process restart.

**User story:** As an operator, I want durable transport records, so that a disconnect does not erase accepted output.

#### Acceptance criteria

- **AC-PLATFORM-DURABLE-AGENT-DELIVERY-001.1:** When durable delivery is active, agentctl must commit normalized events before publication and recover committed records after a process crash.
- **AC-PLATFORM-DURABLE-AGENT-DELIVERY-001.2:** When storage is unavailable, corrupt, locked, or full, agentctl must refuse unsafe admissions and expose a typed error. It must not silently discard unacknowledged records.
- **AC-PLATFORM-DURABLE-AGENT-DELIVERY-001.3:** When an instance stops during journal access, the operation shall finish safely or return a typed unavailable result. Other instances shall remain operational, and committed records shall remain recoverable.
- **AC-PLATFORM-DURABLE-AGENT-DELIVERY-001.4:** Before usable disk capacity is exhausted, agentctl shall expose delivery pressure and preserve capacity for control and recovery records. The fixed 256 MiB stream and 2 GiB journal thresholds shall not cancel ongoing work while usable disk capacity remains. At actual storage exhaustion or persistence failure, bounded flow control and exact-owner cancellation may protect recoverable state. Status, acknowledgment, replay, and Stop shall remain available where storage permits. Persistence failure shall preserve uncertainty and shall not establish process termination.

### REQ-PLATFORM-DURABLE-AGENT-DELIVERY-002: Executor storage lifetime

**Intent:** Durability guarantees must match the actual executor storage lifetime.

**User story:** As an operator, I want explicit storage limits, so that I know which failures permit recovery.

#### Acceptance criteria

- **AC-PLATFORM-DURABLE-AGENT-DELIVERY-002.1:** When an executor advertises durable delivery, its journal must survive agentctl replacement within the retained environment. Native harness state must remain separate.
- **AC-PLATFORM-DURABLE-AGENT-DELIVERY-002.2:** When an environment loses its journal, Kandev must report unavailable delivery history and an uncertain active submission. Cleanup must preserve live or unacknowledged records.
- **AC-PLATFORM-DURABLE-AGENT-DELIVERY-002.3:** Normal Kandev shutdown shall stop local/worktree agents by default. Explicit local survival remains optional. Backend shutdown or disconnection shall preserve supported remote agentctl and agent processes independently of that local setting. Backend return shall authenticate and reconnect to the recorded remote execution; explicit Stop and provider environment loss remain separate events.

### REQ-PLATFORM-DURABLE-AGENT-DELIVERY-003: Idempotent prompt submission

**Intent:** A transport retry must not run the same logical prompt twice.

**User story:** As a user, I want safe prompt delivery, so that a lost response does not repeat tool actions.

#### Acceptance criteria

- **AC-PLATFORM-DURABLE-AGENT-DELIVERY-003.1:** When the backend repeats an accepted submission identifier with the same payload hash, agentctl must return its durable state without another harness dispatch.
- **AC-PLATFORM-DURABLE-AGENT-DELIVERY-003.2:** When a crash leaves dispatch uncertain, Kandev must block automatic resend. A reused identifier with a different hash must fail without harness dispatch.

### REQ-PLATFORM-DURABLE-AGENT-DELIVERY-004: Cursor-based event delivery

**Intent:** Reconnect must restore committed events in order with bounded memory.

**User story:** As a user, I want missing output after reconnect, so that the conversation remains complete.

#### Acceptance criteria

- **AC-PLATFORM-DURABLE-AGENT-DELIVERY-004.1:** When a compatible backend reconnects with a valid cursor, agentctl must replay subsequent committed events in sequence before joining live delivery.
- **AC-PLATFORM-DURABLE-AGENT-DELIVERY-004.2:** When a cursor is invalid, expired, or from another stream, agentctl must return a typed error. It must not skip history or create an unbounded queue.
- **AC-PLATFORM-DURABLE-AGENT-DELIVERY-004.3:** When a backend adopts a surviving agent, recovery must preserve the original stream identity and replay position. Adoption must not replace the conversation.

- **AC-PLATFORM-DURABLE-AGENT-DELIVERY-004.4:** While detached and within journal capacity, a durable producer shall continue committing output without waiting for an absent stream consumer. Reattachment shall deliver the complete retained tail, including terminal events.
- **AC-PLATFORM-DURABLE-AGENT-DELIVERY-004.5:** A supported remote agentctl and its active agent shall continue independent work during prolonged backend disconnection, including a week-long outage, while the remote environment and usable disk capacity remain available. Unacknowledged output shall not expire because of backend absence or crossing the former fixed journal limits. Memory use shall remain bounded.
- **AC-PLATFORM-DURABLE-AGENT-DELIVERY-004.6:** On backend return, Kandev shall automatically authenticate and reconnect to the same surviving remote execution, replay retained output incrementally, and join live delivery without a prompt, duplicate tool execution, or replacement conversation. Explicit Stop remains separate from backend shutdown. Operations requiring an unavailable backend may wait without fabricating completion.

### REQ-PLATFORM-DURABLE-AGENT-DELIVERY-005: Idempotent backend projection

**Intent:** Replay must not duplicate messages, turn completion, or workflow effects.

**User story:** As a user, I want one result per event, so that reconnect does not repeat task actions.

#### Acceptance criteria

- **AC-PLATFORM-DURABLE-AGENT-DELIVERY-005.1:** When the backend acknowledges an event, the event must already exist in its durable inbox. Duplicate delivery must produce one canonical message effect.
- **AC-PLATFORM-DURABLE-AGENT-DELIVERY-005.2:** When projection restarts after a crash, turn transitions and workflow intents must remain idempotent. Stale owners must not change current session state.
- **AC-PLATFORM-DURABLE-AGENT-DELIVERY-005.3:** When an adopted turn completes, preceding conversation output must be preserved before completion releases subsequent work. Repeated recovery must not repeat completion effects.
- **AC-PLATFORM-DURABLE-AGENT-DELIVERY-005.4:** After connection or execution replacement, acknowledgments shall use the current authenticated owner and recover their pending cursor from durable backend state. A failed acknowledgment shall retry without requiring another event or prompt. Late work from an obsolete connection shall not override its replacement.
- **AC-PLATFORM-DURABLE-AGENT-DELIVERY-005.5:** Once backend persistence permits acknowledgment, journal retention shall converge to that cursor even while the stream is quiet or full. Recovery shall prune only verified acknowledged data, preserve native conversation identity, and expose stalled acknowledgment progress. Increasing capacity shall not substitute for retention recovery.

### REQ-PLATFORM-DURABLE-AGENT-DELIVERY-006: Disconnect reconciliation

**Intent:** A broken stream is not proof that a prompt failed.

**User story:** As a user, I want accurate reconnect status, so that Kandev does not restart work that already ran.

#### Acceptance criteria

- **AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.1:** When the transport disconnects, Kandev must reconcile the original submission and stream before declaring a terminal outcome or starting another prompt.
- **AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.2:** When reconciliation cannot establish the outcome, Kandev must show an uncertain state, keep Stop available, and prevent automatic queue dispatch or tool replay.
- **AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.3:** When adoption cannot establish the active submission, Kandev must block new dispatch and replacement. Stop must remain available during reconciliation.
- **AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.4:** After bounded initial reconciliation expires, later state-only reconciliation shall remain possible for the same owned submission without resend.
- **AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.5:** When retained evidence settles an uncertain submission, Kandev shall resolve only its matching delivery block after projection and authoritative outcome settlement. Other recovery blocks shall remain effective.
- **AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.6:** Reconnecting and uncertain session state shall survive browser reload and backend restart. It shall not appear idle and ready for new work while delivery remains unresolved.
- **AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.7:** When a session has an uncertain prompt, workspace-only inspection shall remain available through an existing owned workspace execution. Cold workspace restoration may proceed when no saved execution or delivery recovery identity can be replaced. It shall not overwrite a pinned recovery source or candidate, start an agent, resend a prompt, or resolve uncertainty.
- **AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.8:** When a user clicks the existing Resume action in Quick Chat or task chat, Kandev shall use explicit session recovery rather than an ordinary launch blocked by an interrupted prompt. Automatic opening and focus shall remain subject to recovery admission.
- **AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.9:** A successful explicit native resume shall preserve the native conversation and record its recovery action without resending the interrupted instruction or declaring its historical unknown outcome terminal. Failed recovery shall retain the recovery block.
- **AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.10:** Workspace-only restoration shall never resolve a prompt recovery block, including when a request carries a recovery action. Native-state loss and unresolved live durable work shall retain their existing distinct recovery rules.
- **AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.11:** A read-only workspace fallback without a composer recovery owner shall expose the existing explicit Resume action with details collapsed. The action shall honor busy state and remain reachable on desktop and phone.
- **AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.12:** Successful explicit native resume shall match every unresolved SQL submission to authenticated journal evidence by ID, session, incarnation, generation, and payload hash before retiring interrupted-unknown work. It shall confirm matching retirement before resolving the database recovery block. Acknowledged uncertainty shall retain its historical payload and outcome, remain non-replayable, and survive journal reopen and backend adoption. Live work, ownership mismatch, or unavailable SQL or journal evidence shall keep recovery blocked.
- **AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.16:** Retry connection shall report progress and a specific result, including when the original execution is absent. Retained evidence shall remain usable without that execution. Missing or ambiguous evidence shall produce an actionable blocked result without resend.
- **AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.17:** After confirmed termination of the original owned process, an authorized user can explicitly continue an interrupted session with a new instruction. Kandev shall retain the prior uncertainty, preserve native identity when recoverable, and reject stale recovery requests. Repeated requests shall not dispatch another instruction. Unknown process ownership shall block continuation. **Superseded by AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.20 through 006.24 for restart recovery.**
- **AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.18:** Recovery actions shall use aligned desktop controls and stacked phone controls. Phone and coarse-pointer targets shall measure at least 44 pixels. Retry results, Stop limitations, and continuation choices shall remain visible and keyboard-accessible. **Superseded by AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.20 through 006.24 for restart recovery.**
- **AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.19:** Users can resume selected sessions interrupted by a shared runtime failure in one operation. Each eligible session shall retain its Kandev session and native conversation identities. The operation shall report progress and results per session, preserve independent admission and ownership checks, and exclude already active or completed work. Retrying the operation shall not repeat accepted continuation instructions. Missing native state shall block that session without creating a replacement conversation. **Superseded by AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.20 through 006.24 for restart recovery.**
- **AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.20:** After backend restart, Kandev shall automatically reconcile eligible interrupted sessions without browser interaction. Surviving agents shall retain their process, conversation, and active turn.
- **AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.21:** After confirmed process termination, recovery shall load the same saved native conversation without sending any prompt. It shall preserve session identity, workspace, retained output, and the interrupted submission's uncertain outcome. It shall not create a user message, replay tools, or drain queued work.
- **AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.22:** Desktop and phone shall not show a restart-resume form, session-selection list, acknowledgment checkbox, or continuation-instruction field. Unresolved failures shall remain inspectable within the affected session, without blocking unrelated sessions or the application layout.
- **AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.23:** Recovery shall verify current ownership, process termination, native identity, and independent admission restrictions. Missing or conflicting evidence shall preserve a blocked session. Archived, completed, Office-owned, and automation-owned sessions shall not enter ordinary automatic chat recovery.
- **AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.24:** Repeated restart or interrupted recovery shall reuse durable progress without duplicate restore or prompt dispatch. Resolved recovery metadata shall not trigger another recovery attempt or stale-owner warning. One blocked candidate shall not prevent other eligible candidates from recovering.

### REQ-PLATFORM-DURABLE-AGENT-DELIVERY-007: Compatible rollout

**Intent:** Compatible installations must receive durable delivery without an operator toggle or misleading fallback.

**User story:** As an operator, I want explicit capability negotiation, so that mixed versions do not silently lose durability.

#### Acceptance criteria

- **AC-PLATFORM-DURABLE-AGENT-DELIVERY-007.1:** When a peer lacks the protocol or an executor does not support retained storage, Kandev must identify legacy delivery. Existing persisted schema must remain available.
- **AC-PLATFORM-DURABLE-AGENT-DELIVERY-007.2:** Recovery must pass crash, replay, and desktop/mobile tests before release. Supported rollback must retain journal data and prevent unsafe active-stream downgrade.
- **AC-PLATFORM-DURABLE-AGENT-DELIVERY-007.3:** When compatible peers use a supported retained environment, durable delivery must activate automatically. The functionality must not require a feature flag.
- **AC-PLATFORM-DURABLE-AGENT-DELIVERY-007.4:** When a supported durable environment has a journal error, Kandev must block unsafe admission. It must not downgrade to legacy delivery.
- **AC-PLATFORM-DURABLE-AGENT-DELIVERY-007.5:** When a backend adopts a surviving agent, it must establish delivery capability before accepting new work. An undiscovered capability must not imply legacy support.

- **AC-PLATFORM-DURABLE-AGENT-DELIVERY-007.6:** Reattachment to a surviving initialized agent shall preserve its existing session without ACP initialization or prompt dispatch. Failed reattachment shall not force-stop that process through new-start cleanup.

## Out of scope

- Exactly-once execution of external tools, MCP requests, or model calls.
- Survival of an erased executor volume or an unsupported shared filesystem.
- Keeping agent processes alive after an executor intentionally terminates them.
- Replacing backend SQL transcripts or the existing browser subscription contract.

## Implementation records

- [Journal shutdown and interrupted-session recovery repair](../../../plans/agentctl-journal-shutdown-recovery/plan.md).
