# ADR-2026-08-04-mid-turn-steering: Deliver Operator Input Into a Generating Turn

**Status:** accepted
**Date:** 2026-08-04
**Area:** backend, frontend, protocol

## Context

While a Claude session is genuinely generating, an operator who spots a wrong
turn can only queue a message (it lands after the decision it would have
prevented) or cancel (losing the turn's work). Claude Code's own terminal and
desktop surfaces accept typed input during generation and act on it at the next
tool boundary. Kandev could not, so operators arriving from those surfaces
experienced it as worse at the moment correction matters most.

Two prior decisions bracketed this. [ADR 0049](0049-fine-grained-foreground-idle-busy-signal.md)
recorded that true mid-turn steering "needs ACP concurrent-prompt support and
per-agent capability gating and is tracked separately." The
[coarse-running ADR](2026-07-28-coarse-running-busy-signal.md) then made every
`RUNNING` session busy-by-default behind the `claudeBackgroundPromptHandoff`
experiment.

Direct investigation of the shipped stack changed the premise. The Claude CLI
folds a stream-json message pushed mid-turn into the running turn at the next
tool boundary (one `result` answered two prompts; the model obeyed the second).
The `@agentclientprotocol/claude-agent-acp` bridge accepts a concurrent
`session/prompt` and hands the turn off: the predecessor RPC resolves `end_turn`
before the steered answer exists and with zeroed usage, while the answer and the
real usage land on the successor. Background work launched in the predecessor
survives the boundary and can later wake a model cycle with no prompt in flight.
All three are captured as wire fixtures under
`apps/backend/internal/agentctl/server/adapter/transport/acp/testdata/`.

So the concurrent-prompt precondition ADR 0049 named is already satisfied by the
shipped bridge, and the missing piece is a *trigger* that fires while the
foreground is still generating — `markPromptHandoff` previously fired only on
`EventTypeForegroundIdle`.

## Decision

Add mid-turn steering as an additive, flag-gated delivery path.

- **Trigger.** The operator's send is itself the handoff trigger. A steer
  transfers the existing prompt-gate token to the successor while the
  predecessor `session/prompt` is still open, reusing the ADR-0049 machinery
  (`tryTransferPromptTurn`, `claimPromptTurnCompletion`,
  `protectActiveBackgroundWorkForHandoff`) rather than adding a second
  concurrency path. Synthetic `ScheduleWakeup` prompts can neither trigger nor
  consume a steer.

- **Attribution.** The predecessor's early `end_turn` must not be presented as
  its own answer or emit a completion, and must not sweep tool ownership,
  consume the successor's usage, or clear its trace. This is the same
  generation-keyed suppression ADR 0049 specified, now reused for the
  steer trigger.

- **Capability gate is negotiated, not named.** Eligibility comes from the ACP
  `initialize` advertisement `agentCapabilities._meta.claudeCode.promptQueueing`,
  replacing the two shipped agent-name comparisons
  (`supportsPromptHandoff`, `claudeBackgroundPromptHandoffEnabledForSession`).
  This aligns the code with ADR 0049's own rejected alternative — "an agent
  identity does not prove that the installed adapter/provider version emits the
  lifecycle frames Kandev expects" — and it re-gates the shipped
  foreground-idle experiment on the same signal, narrowing eligibility rather
  than widening it.

- **Delivery is opportunistic; deferral is a first-class success.** The
  advertisement asserts only that the agent accepts a concurrent prompt. Whether
  the agent *folds* the prompt into the running turn is decided by the agent CLI
  beneath the bridge, is not advertised over the protocol, and — critically —
  the CLI's version is not observable over ACP (`agentInfo.version` is the
  bridge's). An installation whose bridge is current but whose CLI is too old to
  fold reports itself steer-capable and defers every steer. That is a supported
  configuration: it degrades to exactly today's queue behavior. Kandev must not
  gate on, infer, or warn about a CLI version it cannot see, and the composer
  affordance therefore promises *delivery*, never folding.

- **Ordering.** A steer is admitted only when the session has no queued messages
  (it never jumps the queue) and at most one steer is in flight per session; a
  second attempt queues. A steer does not take the foreground claim, which the
  predecessor still owns.

- **Flag.** Ships behind the default-off, high-risk, restart-required
  `features.claudeMidTurnSteering` runtime toggle
  (`KANDEV_FEATURES_CLAUDE_MID_TURN_STEERING`), independent of
  `claudeBackgroundPromptHandoff`, and retained as a kill-switch after rollout.

This supersedes the mid-turn-steering non-goal in ADR 0049 for the
steer-capable case. The full behavior is specified in
[docs/specs/platform/requirements/mid-turn-steering.md](../specs/platform/requirements/mid-turn-steering.md).

## Consequences

Operators on a steer-capable, flag-enabled session can correct a running turn
without cancelling it. The composer switches from "queue" to a delivery
affordance, and the message reaches the agent immediately.

The risk is real and is why the feature stays behind a kill-switch: the fold is
undocumented, marked `@internal` in the CLI, and not advertised over ACP, so it
can regress without a changelog entry. The mitigations are that the negotiated
gate asserts only the concurrent-prompt precondition, both outcomes (folded and
deferred) are specified as correct, the observed wire shapes are pinned as
fixtures, and two overlapping RPCs reuse the generation-keyed attribution that
already shipped for foreground-idle handoff.

Public docs record the runtime flag: `docs/public/configuration.md` carries a
`claudeMidTurnSteering` row in both config-key tables, matching how
`claudeBackgroundPromptHandoff` is documented. The row states the default-off,
high-risk, experimental nature and that folding is the agent's decision.

## Alternatives Considered

- **Add a second gate slot / concurrency path.** Rejected: the bridge already
  tolerates two overlapping prompts with one logical owner, so the work is
  attribution and triggering, not concurrency.
- **Gate steering on a Claude CLI version floor.** Rejected: the CLI version is
  not observable over ACP, and gating on the bridge version gates the wrong
  component. Opportunistic delivery with a first-class deferred outcome is the
  only honest contract.
- **Adopt an agent-specific steer primitive (e.g. Codex `turn/steer`).**
  Rejected for this iteration: it would add a provider-specific protocol
  surface, whereas the fold uses only the existing prompt method. Left open for
  a future agent-specific build.
- **Keep the agent-name whitelist and just add steering.** Rejected: it would
  perpetuate the exact anti-pattern ADR 0049 rejected and would trust a
  name-matching bridge too old to advertise the capability.
