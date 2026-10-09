---
status: draft
system: cli
requirements:
  - REQ-CLI-PASSTHROUGH-PROMPT-DELIVERY-001
---

# Passthrough Prompt Delivery System Design

## Purpose and boundaries

This design owns how a prompt that is already selected for a CLI passthrough
session becomes bytes on that session's PTY. It owns the framing of the body,
the size and pacing of the writes, and the separation of the submit keystroke.

It does not own the prompt text, its system wrapper, its queue position, or its
delivery mode; those arrive from the tasks system. It does not own PTY process
startup, terminal mirroring, or operator keystrokes, which belong to the
interactive process runner. It consumes the per-agent passthrough capability
record but does not own the agent catalog.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-CLI-PASSTHROUGH-PROMPT-DELIVERY-001` | [Components and responsibilities](#components-and-responsibilities), [Data and contracts](#data-and-contracts), [Control flow](#control-flow), [Failure and recovery](#failure-and-recovery), [Security](#security) |

## Components and responsibilities

- **Passthrough capability record.** Per-agent configuration that states the
  submit sequence, the submit delay, and whether the agent accepts
  bracketed-paste framing. It is the only input that varies delivery behavior
  between agents; delivery code never branches on an agent identity.
- **Stdin chunk planner.** The single function that converts a prompt and a
  capability record into an ordered list of PTY writes. It is pure: no PTY, no
  clock, no I/O, so every framing and sizing rule is unit-testable. It is the
  only place that knows about framing markers and the host read budget.
- **Delivery callers.** The initial-prompt injector, the workflow step prompt
  handler, and the session prompt path each obtain the planned chunks and write
  them in order, honoring each chunk's pre-write delay. They add no framing of
  their own.
- **PTY writer.** The lifecycle manager's passthrough stdin call and the
  interactive runner behind it write one chunk per call to the PTY master and
  surface write errors to the caller.

## Data and contracts

- **Planned chunk.** An ordered pair of the bytes to write and the delay to
  apply before writing them. The first chunk carries no delay. This type already
  exists and is unchanged.
- **Framing markers.** The bracketed-paste start and end sequences. They are
  applied to the body only, never to the submit keystroke.
- **Raw-safe write size.** A byte budget for one write on the unframed path,
  chosen below the smallest host single-read capacity Kandev supports, so a
  receiving TUI never has to absorb more than one read worth of unframed input
  at a time. Splits fall on rune boundaries so no write ends mid-character.
- **Inter-chunk delay.** The pause applied before each continuation write on the
  unframed path, sized so the receiving TUI drains one read before the next
  arrives.
- **Submit delay.** The existing per-agent pause before the submit keystroke,
  which keeps that keystroke a discrete input event rather than trailing bytes
  of the body.

## Control flow

1. A caller resolves the session's capability record and asks the planner for
   the chunks of a prompt.
2. If framing is selected, the planner neutralizes framing terminators inside the body.
3. When the agent accepts bracketed-paste framing and the body is multi-line or
   larger than the raw-safe write size, the planner emits the framed body as one
   chunk. Small single-line bodies stay unframed, preserving today's bytes.
4. When the agent does not accept bracketed-paste framing, the planner emits the
   body as consecutive chunks no larger than the raw-safe write size, each
   continuation chunk carrying the inter-chunk delay.
5. When `SubmitDelay > 0`, the planner appends the submit keystroke as its own
   chunk carrying the submit delay. When `SubmitDelay == 0`, it appends the
   submit sequence to the final body chunk.
6. The caller writes each chunk in order, waiting for the chunk's delay with the
   caller context first.

Ordering is total: the PTY is a single stream, and the caller writes
sequentially, so the body writes and submit keystroke are sent in order. The
unframed pacing reduces burst loss but cannot confirm what the TUI consumed.

## Failure and recovery

A failed write aborts the remaining chunks for that prompt. The session prompt
path returns the failure to its caller, which reverts session state and surfaces
the error, so a half-typed prompt is never reported as delivered. The initial
prompt injector logs the failure and stops rather than retrying, because a retry
would duplicate the already-written prefix in the agent's input.

Delivery does not read the terminal back, so it cannot confirm what the TUI
absorbed. Correctness therefore rests on framing and sizing rather than on
acknowledgement. The 30 ms unframed interval is an empirical pacing policy, not
a universal lossless-delivery guarantee. Any change to these rules needs
evidence from a real TUI.

## Security

Prompt bodies are untrusted for terminal framing. A body that contains a framing
terminator would end the paste early and let the remainder reach the TUI as
terminal input outside the prompt, which in a CLI agent means unintended
commands. The planner therefore neutralizes framing terminators in the body
before wrapping it. It performs no other rewriting: the body is agent-visible
content and must otherwise arrive byte-for-byte.

## Observability

Delivery callers log the byte length of what they write: the initial prompt
injector logs the injected description length, and the session prompt path logs
the prompt length at debug level. The planner itself logs nothing, because it is
pure.

The authoritative delivery check is comparison, not logging: the stored message
text against the receiving agent's own transcript of what it was sent. That
comparison is what exposed the original defect, a loss of exactly N x 1022
bytes from the start of every long body. Per-delivery logging of chunk count and
framing is not implemented; it would let an operator distinguish a mis-planned
delivery from one the TUI mishandled without a transcript.
