# ADR-2026-09-27-explicit-same-turn-steering: Explicit same-turn delivery beside the queue

**Status:** accepted
**Date:** 2026-09-27
**Area:** protocol

## Context

Kandev's existing steering is based on ACP concurrent prompts, whose provider may
fold or defer the input. It preserves queue order by queueing behind pending work.
Codex app-server exposes turn/steer with an expected active-turn identity, a
stronger targeting contract. The user requested sending to the active turn
without waiting for the Kandev queue, while keeping a shared multi-protocol UI.

## Decision

Normalize steering capability as same_turn, provider_managed, or unsupported.
Use an explicit Send now delivery intent for same_turn providers and a separate
Queue for later choice. A deliberate same-turn send may bypass queued next-turn
messages without changing their order or membership. Existing automatic/ACP
steering keeps its existing queue-first policy and independent release gate.

Pin delivery to the original session incarnation, execution, and root turn.
Reject stale targets rather than steering their successors. Serialize only the
bounded dispatch/acknowledgement interval; do not wait for root completion.
No implicit queue or new-turn fallback is allowed for this explicit intent.
Ambiguous delivery is retained as uncertain and never automatically resent.

The runtime delivery contract belongs to Platform, with adapter-specific mapping
in the native transport and shared composer behavior in the same vertical design.
It is independent of background-work visibility and does not enable a child-agent
composer. Same-turn rollout has its own off-by-default gate; native Codex also
requires its existing native profile gate. Do not repurpose the Claude flag.

## Consequences

Users can correct active work while preserving planned next-turn messages.
The UI must distinguish accepted from acted-on input and retain uncertain
outcomes. Durable receipts and immutable targeting add work but prevent
accidental duplicate instructions or delivery to a replacement conversation.
Older clients/providers remain on their original path, and future protocols can
advertise their actual semantics without UI branches by agent name.

## Alternatives Considered

- Always queue first: preserves global submission order but prevents the requested
  correction of active work when the queue is nonempty.
- Treat native steering as ACP concurrent prompting: obscures the stronger turn
  precondition and risks unnecessary prompt-generation handoff/completion handling.
- Interrupt and restart: loses additive steering semantics and can repeat work.
- Automatically enqueue rejected or uncertain sends: changes user intent and may
  duplicate a message already accepted by the provider.

## Related design

[Explicit same-turn steering](../specs/platform/system-design/explicit-turn-steering.md).
