---
status: active
system: agents
created: 2026-11-05
owners:
  - kandev
---

# Copilot Native Compaction Requirements

## Overview

Copilot can report actual compaction outcomes through an opt-in native ACP notification stream.
Kandev uses those outcomes to restore meaningful compaction counting without interpreting child-context usage drops as parent compactions.
Agents owns this vertical contract because provider attribution and outcome evidence determine which observations consumers can trust.
Task storage and the context-window surface consume that evidence.

The existing [count surface](../../ui/requirements/context-compaction-count.md) retains its presentation and lifetime-total contract.
The [standard ACP timeline](acp-session-events.md) remains a separate input path.
This requirement defines the additional Copilot provider contract, including its visible outcomes.

## Terms

- **Accepted compaction:** A parent completion with a valid attempt identity, explicit success, and an applied outcome.
- **Observed total:** The retained session count plus accepted native increments. It is not a complete provider-history claim.
- **Root observation:** A valid native envelope with no child `agentId` property, bound to the expected native conversation.
- **Unavailable:** The current runtime cannot establish compatible native-event collection.
- **Partial:** Kandev detected missing or unusable counting evidence. This limitation persists for the session lifetime.

`/compact` compacts the same conversation and can produce an accepted compaction.
Reset Context creates a fresh native conversation and does not itself increment the count.

## Requirements

### REQ-AGENTS-COPILOT-NATIVE-COMPACTION-001: Attributed native observations

**Intent:** Native parent outcomes provide evidence that usage drops alone cannot supply.

#### Acceptance criteria

- **AC-AGENTS-COPILOT-NATIVE-COMPACTION-001.1:** Only the Copilot provider shall subscribe to and interpret its native session-event notifications.
- **AC-AGENTS-COPILOT-NATIVE-COMPACTION-001.2:** An accepted parent `session.compaction_complete` shall add one compaction, including when no preceding start was observed.
- **AC-AGENTS-COPILOT-NATIVE-COMPACTION-001.3:** Starts, child events, failures, cancellations, non-applied outcomes, and missing or malformed success/outcome/attempt fields shall add nothing.
- **AC-AGENTS-COPILOT-NATIVE-COMPACTION-001.4:** A wrong conversation, invalid origin attribution, or unsupported notification shall not change the count or active context gauge.
- **AC-AGENTS-COPILOT-NATIVE-COMPACTION-001.5:** Valid manual and automatic compactions shall use the same success rule. Trigger labels shall not substitute for successful applied outcomes.
- **AC-AGENTS-COPILOT-NATIVE-COMPACTION-001.6:** Missing event data, including provider omission markers, shall never imply success. A potentially lost parent completion shall make coverage partial.

### REQ-AGENTS-COPILOT-NATIVE-COMPACTION-002: Durable counting identity

**Intent:** Delivery retries and recovery cannot multiply one accepted provider operation.

#### Acceptance criteria

- **AC-AGENTS-COPILOT-NATIVE-COMPACTION-002.1:** One attempt shall increment at most once for its owning Kandev session, provider, and native conversation, across duplicate notifications and delivery streams.
- **AC-AGENTS-COPILOT-NATIVE-COMPACTION-002.2:** Accepted identity, count mutation, and durable event projection shall commit together or leave all three unchanged.
- **AC-AGENTS-COPILOT-NATIVE-COMPACTION-002.3:** Backend restart, journal replay, and recovery without lifecycle callbacks shall preserve accepted counts and deduplication. Late client updates shall not lower a newer count.
- **AC-AGENTS-COPILOT-NATIVE-COMPACTION-002.4:** Native replacement shall reject newly arriving stale observations. Previously journaled evidence shall retain its original owner and count once without changing replacement context usage.
- **AC-AGENTS-COPILOT-NATIVE-COMPACTION-002.5:** Prior lifetime totals shall remain intact and readable.
- **AC-AGENTS-COPILOT-NATIVE-COMPACTION-002.6:** Context reset and model change shall preserve deduplication receipts and shall not add a compaction.
- **AC-AGENTS-COPILOT-NATIVE-COMPACTION-002.7:** Task transfer and receipt retirement shall preserve deduplication receipts and shall not add a compaction.
- **AC-AGENTS-COPILOT-NATIVE-COMPACTION-002.8:** History compaction shall preserve deduplication receipts and shall not add a compaction.

### REQ-AGENTS-COPILOT-NATIVE-COMPACTION-003: Root context gauge and independent accounting

**Intent:** The chat-input gauge describes the parent conversation while accounting retains its existing inputs.

#### Acceptance criteria

- **AC-AGENTS-COPILOT-NATIVE-COMPACTION-003.1:** On a supported native-event runtime, the context gauge shall use valid root `session.usage_info` observations and ignore child observations.
- **AC-AGENTS-COPILOT-NATIVE-COMPACTION-003.2:** Ordinary mixed usage notifications shall not overwrite that root gauge or increment the Copilot compaction count.
- **AC-AGENTS-COPILOT-NATIVE-COMPACTION-003.3:** Enabling root-only reporting or replacing the native conversation shall invalidate any incompatible gauge baseline until a fresh root reading arrives.
- **AC-AGENTS-COPILOT-NATIVE-COMPACTION-003.4:** Filtering gauge observations shall preserve token and cost accounting. `isInitial`, token thresholds, and tool-card lifetimes shall not select context ownership.

### REQ-AGENTS-COPILOT-NATIVE-COMPACTION-004: Honest coverage and compatible presentation

**Intent:** Users can distinguish received evidence from an unavailable or incomplete provider total.

#### Acceptance criteria

- **AC-AGENTS-COPILOT-NATIVE-COMPACTION-004.1:** The existing context-window surface shall show the retained total and explain that new Copilot increments use observed successful parent events.
- **AC-AGENTS-COPILOT-NATIVE-COMPACTION-004.2:** Unsupported or unqualified runtimes shall show counting as unavailable and retain prior totals. Silence shall not establish a successful subscription or zero actual compactions.
- **AC-AGENTS-COPILOT-NATIVE-COMPACTION-004.3:** Known omitted evidence or delivery gaps shall persist partial coverage. Later valid events shall not imply that the missing history became complete.
- **AC-AGENTS-COPILOT-NATIVE-COMPACTION-004.4:** Help shall disclose retained historical estimates and potentially missed events. The total shall not be described as wholly confirmed or complete.
- **AC-AGENTS-COPILOT-NATIVE-COMPACTION-004.5:** Desktop pointer/keyboard and phone touch shall expose the same total, coverage, and explanation through the existing context-window surface.
- **AC-AGENTS-COPILOT-NATIVE-COMPACTION-004.6:** Compatible parent starts and completions shall reuse the existing compaction timeline identity. Timeline rendering shall not independently increment the counter.
- **AC-AGENTS-COPILOT-NATIVE-COMPACTION-004.7:** Other providers and standard ACP timeline events shall retain their current counting behavior. Older agentctl processes shall remain usable with native counting unavailable.

## Compatibility evidence

The [issue investigation](https://github.com/kdlbs/kandev/issues/5089#issuecomment-6672451872) captured two child calls with no compaction and one applied manual compaction on Copilot 1.0.103.
The 1.0.103 generated schema contains optional `compactionAttemptId` and `outcome`, required `success`, and a `threshold` trigger for automatic compaction.
The 1.0.88 schema contains neither the attempt identity nor the outcome field, so it cannot satisfy this requirement.
The managed pin is 1.0.103. Qualification used version-pinned schemas and sanitized fixtures; no live automatic-compaction capture or live account call was available for this implementation.

## Exclusions

- Re-enabling mixed-usage inference, historical backfill, or repairing old false increments.
- Reading native private event logs, changing billing, or recovering provider-dropped events from guessed context changes.
- Counting subagent compactions in the parent total or adding controls that initiate compaction.
- Replacing standard ACP support or changing other providers' count semantics.

## Implementation plans

- [Native Copilot compaction counting](../../../plans/copilot-native-compaction-count/plan.md).
