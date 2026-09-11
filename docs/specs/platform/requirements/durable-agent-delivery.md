---
status: draft
system: platform
created: 2026-09-10
owners:
  - kandev
---

# Durable agent delivery requirements

## Overview

The platform system owns delivery between the backend and agentctl. This contract does not replace browser subscription recovery or native harness persistence.

This draft defines proposed behavior. It does not claim that the current implementation provides these guarantees.

## Terminology

- Journal: A bounded agentctl store for submissions and normalized events.
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

### REQ-PLATFORM-DURABLE-AGENT-DELIVERY-002: Executor storage lifetime

**Intent:** Durability guarantees must match the actual executor storage lifetime.

**User story:** As an operator, I want explicit storage limits, so that I know which failures permit recovery.

#### Acceptance criteria

- **AC-PLATFORM-DURABLE-AGENT-DELIVERY-002.1:** When an executor advertises durable delivery, its journal must survive agentctl replacement within the retained environment. Native harness state must remain separate.
- **AC-PLATFORM-DURABLE-AGENT-DELIVERY-002.2:** When an environment loses its journal, Kandev must report unavailable delivery history and an uncertain active submission. Cleanup must preserve live or unacknowledged records.

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

### REQ-PLATFORM-DURABLE-AGENT-DELIVERY-005: Idempotent backend projection

**Intent:** Replay must not duplicate messages, turn completion, or workflow effects.

**User story:** As a user, I want one result per event, so that reconnect does not repeat task actions.

#### Acceptance criteria

- **AC-PLATFORM-DURABLE-AGENT-DELIVERY-005.1:** When the backend acknowledges an event, the event must already exist in its durable inbox. Duplicate delivery must produce one canonical message effect.
- **AC-PLATFORM-DURABLE-AGENT-DELIVERY-005.2:** When projection restarts after a crash, turn transitions and workflow intents must remain idempotent. Stale owners must not change current session state.

### REQ-PLATFORM-DURABLE-AGENT-DELIVERY-006: Disconnect reconciliation

**Intent:** A broken stream is not proof that a prompt failed.

**User story:** As a user, I want accurate reconnect status, so that Kandev does not restart work that already ran.

#### Acceptance criteria

- **AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.1:** When the transport disconnects, Kandev must reconcile the original submission and stream before declaring a terminal outcome or starting another prompt.
- **AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.2:** When reconciliation cannot establish the outcome, Kandev must show an uncertain state, keep Stop available, and prevent automatic queue dispatch or tool replay.

### REQ-PLATFORM-DURABLE-AGENT-DELIVERY-007: Compatible rollout

**Intent:** Compatible installations must receive durable delivery without an operator toggle or misleading fallback.

**User story:** As an operator, I want explicit capability negotiation, so that mixed versions do not silently lose durability.

#### Acceptance criteria

- **AC-PLATFORM-DURABLE-AGENT-DELIVERY-007.1:** When a peer lacks the protocol or an executor does not support retained storage, Kandev must identify legacy delivery. Existing persisted schema must remain available.
- **AC-PLATFORM-DURABLE-AGENT-DELIVERY-007.2:** Recovery must pass crash, replay, and desktop/mobile tests before release. Supported rollback must retain journal data and prevent unsafe active-stream downgrade.
- **AC-PLATFORM-DURABLE-AGENT-DELIVERY-007.3:** When compatible peers use a supported retained environment, durable delivery must activate automatically. The functionality must not require a feature flag.
- **AC-PLATFORM-DURABLE-AGENT-DELIVERY-007.4:** When a supported durable environment has a journal error, Kandev must block unsafe admission. It must not downgrade to legacy delivery.

## Out of scope

- Exactly-once execution of external tools, MCP requests, or model calls.
- Survival of an erased executor volume or an unsupported shared filesystem.
- Keeping agent processes alive after an executor intentionally terminates them.
- Replacing backend SQL transcripts or the existing browser subscription contract.
