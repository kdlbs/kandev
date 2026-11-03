---
status: draft
system: agents
created: 2026-11-01
owners:
  - kandev
---

# Subagent Provider Lifecycle Requirements

## Overview

Agents report delegated work in provider-specific ways. Users need one subagent card
that shows who was delegated, what is still running, and what came back. The agents system
owns this contract because it interprets provider wire behavior. Chat consumes the
provider-neutral subagent payload. This extends
[Codex subagent wire compatibility](codex-subagent-wire-compatibility.md), which remains
authoritative for recognizing Codex spawn frames.

## Terminology

- **Subagent card:** The existing chat card for one delegated child.
- **Control operation:** A tool call that acts on an existing child, such as waiting for,
  messaging, reading, listing, resuming, or closing it.
- **Unattributed child call:** A tool call that a provider reports as belonging to a child
  when the wire provides no safe link to one card.

## Requirements

### REQ-AGENTS-SUBAGENT-LIFECYCLE-001: Codex child outcomes

**Intent:** A Codex subagent card reflects the child's reported end state and result.

#### Acceptance criteria

- **AC-AGENTS-SUBAGENT-LIFECYCLE-001.1:** When a completed wait reports a terminal state
  for a known child, the system shall update that child's card with the state.
- **AC-AGENTS-SUBAGENT-LIFECYCLE-001.2:** When that report includes a message, the card
  shall show it as the subagent result.
- **AC-AGENTS-SUBAGENT-LIFECYCLE-001.3:** When a wait names several children, the system
  shall update each known child's own card, and shall leave any unnamed sibling unchanged.
- **AC-AGENTS-SUBAGENT-LIFECYCLE-001.4:** When a close completes for a known child that has
  no terminal state, the card shall show the child as shut down.
- **AC-AGENTS-SUBAGENT-LIFECYCLE-001.5:** A wait that names no states, a state for an
  unknown child, or a non-terminal state shall not create a card or move a card out of a
  terminal state.
- **AC-AGENTS-SUBAGENT-LIFECYCLE-001.6:** A control operation shall not create a subagent
  card or background-work attestation.

### REQ-AGENTS-SUBAGENT-LIFECYCLE-002: Copilot subagent cards

**Intent:** Copilot subagents appear as subagent cards, without inventing a parent link the
wire does not provide.

#### Acceptance criteria

- **AC-AGENTS-SUBAGENT-LIFECYCLE-002.1:** When Copilot starts a delegated agent, the chat
  shall show a subagent card with its reported type, name, description, and prompt.
- **AC-AGENTS-SUBAGENT-LIFECYCLE-002.2:** When a foreground subagent completes, the card
  shall show its result text.
- **AC-AGENTS-SUBAGENT-LIFECYCLE-002.3:** When a subagent is launched in the background, the
  card shall show the background state and the reported child identity when available.
- **AC-AGENTS-SUBAGENT-LIFECYCLE-002.4:** A child call shall nest under a card only when its
  child identity matches a background launch, or when exactly one started foreground card has
  no child identity yet. A child identity keeps that link for its later calls.
- **AC-AGENTS-SUBAGENT-LIFECYCLE-002.5:** When a safe link does not exist, the call shall stay
  an ordinary top-level tool row. The system shall not choose a parent by arrival order.
- **AC-AGENTS-SUBAGENT-LIFECYCLE-002.6:** Non-Copilot agents with similar input fields shall
  not gain Copilot recognition.

### REQ-AGENTS-SUBAGENT-LIFECYCLE-003: Claude background and resumed subagents

**Intent:** Claude's background and resumed subagents show a truthful state and their
final report, using only evidence available on the existing tool-call frames.

#### Acceptance criteria

- **AC-AGENTS-SUBAGENT-LIFECYCLE-003.1:** When a prompt ends normally and is not handed off,
  background cards started by that prompt shall show as completed.
- **AC-AGENTS-SUBAGENT-LIFECYCLE-003.2:** A cancelled or failed prompt, a handed-off prompt,
  or a different prompt's completion shall not mark those cards completed.
- **AC-AGENTS-SUBAGENT-LIFECYCLE-003.3:** When a background card is completed this way and the
  subagent's final report can be read, the card shall show it as the result. When it cannot
  be read, the card shall complete without a result and without a visible error.
- **AC-AGENTS-SUBAGENT-LIFECYCLE-003.4:** When a follow-up message names the child identity of
  an existing card, that card shall show it was resumed and active, and its existing child
  calls shall remain nested. When the follow-up's prompt completes as in 003.1, the card
  shall return to completed.
- **AC-AGENTS-SUBAGENT-LIFECYCLE-003.5:** A follow-up naming an unknown child shall not change
  any card.
- **AC-AGENTS-SUBAGENT-LIFECYCLE-003.6:** Marking or resuming a card shall not change background
  counts, prompt admission, or session activity beyond what the provider's existing lifecycle
  events report.

### REQ-AGENTS-SUBAGENT-LIFECYCLE-004: Control operation labels

**Intent:** Users can read subagent control operations without knowing provider tool names.

#### Acceptance criteria

- **AC-AGENTS-SUBAGENT-LIFECYCLE-004.1:** When Codex, Copilot, or Claude reports a recognized
  control operation, the chat shall show a localized label for it, not the raw tool name.
- **AC-AGENTS-SUBAGENT-LIFECYCLE-004.2:** A similarly named tool from another provider, or an
  unrecognized operation, shall keep its existing title.
- **AC-AGENTS-SUBAGENT-LIFECYCLE-004.3:** Control operations shall remain visible as tool rows
  with their status on desktop and phone.

## Out of scope

- ACP native child sessions and Claude's non-schema `subagent_spawned` frames. Enabling them
  drops the parent card, and a live resumed child lost its tool calls.
- Separating Copilot child replies from assistant text. The chunks carry no child identity.
- A per-child finish signal for Claude's parallel background subagents. The provider does not
  report one on the wire.
- Synthetic grouping of unattributed child calls.
- Changing OpenCode, Cursor, or Auggie subagent presentation.
