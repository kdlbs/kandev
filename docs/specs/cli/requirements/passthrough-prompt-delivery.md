---
status: draft
system: cli
created: 2026-09-22
owners:
  - kandev
---

# Passthrough Prompt Delivery Requirements

## Overview

In CLI passthrough mode Kandev runs a vendor CLI inside a PTY and types prompts
into it, because that CLI exposes no structured prompt channel. Every other
delivery surface hands the agent a framed message; passthrough hands it
keystrokes, so the prompt is only as reliable as the terminal path that carries
it. A host terminal delivers input to the reader in fixed-size reads, and a TUI
that cannot absorb a burst that large drops whole reads without reporting an
error. The operator sees a complete message in Kandev, the agent answers a
truncated one, and nothing in either surface marks the difference.

The CLI system owns this contract because it owns how Kandev drives a vendor CLI
through a PTY and the compatibility behavior that path needs. What is delivered
and when it is delivered stay with their own owners: the tasks system owns the
message API and its delivery modes, and the platform system owns delivery into a
generating turn.

## Terminology

- **CLI passthrough session:** An agent session whose profile sets
  `cli_passthrough`, where Kandev runs the vendor CLI as a PTY child and mirrors
  that terminal to the operator.
- **Prompt body:** The prompt text Kandev delivers, excluding the submit
  keystroke.
- **Submit keystroke:** The byte sequence that makes the receiving TUI start a
  turn for the delivered body.
- **Bracketed paste:** The terminal convention in which `ESC[200~` and `ESC[201~`
  delimit pasted text so a TUI treats the whole burst as one paste rather than a
  stream of keystrokes.
- **Single-read capacity:** The number of bytes a host terminal hands its reader
  per read. It is a host constant, not a Kandev setting, and is smaller than a
  typical prompt.

## Requirements

### REQ-CLI-PASSTHROUGH-PROMPT-DELIVERY-001: Passthrough Prompt Delivery Integrity

**Intent:** A prompt that Kandev accepts for a passthrough session must reach
the agent as the same text the operator or calling agent sent. Silent partial
delivery is worse than a failed delivery: the agent acts on a fragment, the
answer looks coherent, and the loss is invisible in every Kandev surface.

**User story:** As an operator running an agent in CLI passthrough mode, I want
every prompt to arrive whole, so that the agent answers what was actually sent.

#### Acceptance criteria

- **AC-CLI-PASSTHROUGH-PROMPT-DELIVERY-001.1:** When Kandev delivers a prompt to
  a CLI passthrough session, the receiving agent shall receive the prompt body
  in full and unmodified, at any body length.
- **AC-CLI-PASSTHROUGH-PROMPT-DELIVERY-001.2:** When a prompt body exceeds the
  host's single-read capacity, delivery shall preserve body order and shall
  present the body to the receiving TUI as one continuous input rather than as
  unrelated input events it has to rejoin.
- **AC-CLI-PASSTHROUGH-PROMPT-DELIVERY-001.3:** When the prompt body has been
  delivered, the submit keystroke shall arrive as a discrete input event, so the
  receiving TUI starts exactly one turn for that body.
- **AC-CLI-PASSTHROUGH-PROMPT-DELIVERY-001.4:** When the receiving agent is
  configured as unable to accept bracketed-paste framing, delivery shall keep
  AC-CLI-PASSTHROUGH-PROMPT-DELIVERY-001.1 true by pacing the body within the
  host's single-read capacity instead of writing one oversized burst.
- **AC-CLI-PASSTHROUGH-PROMPT-DELIVERY-001.5:** When the prompt body contains
  byte sequences that would end the delivery framing early, delivery shall
  neutralize those sequences so no part of the body reaches the TUI as terminal
  input outside the prompt.
- **AC-CLI-PASSTHROUGH-PROMPT-DELIVERY-001.6:** When a prompt originates from the
  initial task description, an operator composer message, a workflow step, or an
  agent-to-agent message, delivery integrity shall be identical, because all
  passthrough prompt sources share one delivery contract.
- **AC-CLI-PASSTHROUGH-PROMPT-DELIVERY-001.7:** When delivery of a body cannot
  complete, Kandev shall report the failure to the caller rather than leaving a
  partially typed prompt as a successful delivery.

## Out of scope

- Prompt transport for non-passthrough sessions, which uses the structured agent
  protocol and not a terminal.
- Operator keystrokes typed directly into the mirrored terminal view, which the
  terminal input path owns.
- Which prompt is delivered, its wrapping, its queue position, and its delivery
  mode, owned by the tasks system.
- Delivery into an already generating turn, owned by the platform system's
  mid-turn steering contract.
- How the receiving agent interprets or answers the prompt once it has it.
- Message storage and rendering in Kandev surfaces, owned by the tasks and UI
  systems.
