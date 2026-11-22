# ADR-2026-11-22-plugin-external-chat-task-control: Plugin task control from external chat surfaces

**Status:** proposed
**Date:** 2026-11-22
**Area:** protocol

## Context

Chat-bridge plugins such as
[`kandev-plugin-discord`](../specs/integrations/discord.md) map an external
conversation (a Discord channel) to a Kandev task. They mirror agent output,
deliver chat messages to the task, and let operators stop work, answer agent
permission requests, and approve workflow review steps from chat. The request
and its acceptance criteria are tracked in
[kdlbs/kandev#5420](https://github.com/kdlbs/kandev/issues/5420).

Most of that runs on the shipped Host API: `Tasks()`, `Messages().Send/List`,
`Interactions().ListPending/Get`, exact `StopTaskRun`, and admin `actions`.
Five gaps remain:

1. Exact `StopTaskRun` ends the execution. Nothing lets a plugin interrupt only
   the current turn, as the native `agent.cancel` action does through
   `orchestrator.Service.CancelAgent`.
2. Exact `RespondPermission`/`AnswerClarification` require a single-use human
   response receipt. Only an authenticated native UI request
   (`POST /api/plugins/host/interactions/response-receipts`) by a user with
   `session.control` can obtain one
   ([coordination platform decision](2026-09-25-plugin-coordination-platform.md)).
   A person in an external chat can never answer.
3. The native Approve button (`POST /api/v1/sessions/:id/approve`, served by
   `task/service.Service.ApproveSession`) resolves the step's configured
   transition, moves the task, and sets `review_status=approved`. A plugin can
   only call `Tasks().Move`, which records no approval and does not resolve
   the configured target.
4. The `pluginsdk.Message` read DTO carries no provenance. A bridge cannot
   recognize the messages it delivered itself without subscribing to the
   high-volume `message.*` events.
5. `newMessageEvent` adds `raw_content` (the unstripped message) whenever a
   message holds hidden `<kandev-system>` content, and plugin event delivery
   forwards bus payloads unfiltered. The public plugin docs state that plugins
   never see injected system content.

Items 1 to 3 change who may act on a running task and need a maintainer
decision. Items 4 and 5 are read-surface repairs.

## Decision

### Decided

- **No `raw_content` in plugin deliveries.** The plugin event deliverer
  removes `raw_content` from every `message.*` payload before it builds the
  `pluginsdk.Event`. `content` stays stripped and
  `metadata.has_hidden_prompts` stays. WebSocket clients and other bus
  subscribers keep receiving `raw_content`, because the native UI needs it to
  render hidden prompts.
- **`Message.Source`.** `pluginsdk.Message` gains `Source`, read only from the
  persisted `metadata.source` (for example `plugin:<id>`) and empty when
  absent. It is populated on `Messages().List` and on exact message
  observations. No other metadata key is exposed.
- **Non-goals.** Plugins get no interrupt-by-message, no arbitrary workflow
  moves beyond the existing `Tasks().Move`, no Office approval-stage
  decisions, and no receipts minted without an explicit grant.

### Pending maintainer decision

The following are proposed and must not be implemented until the maintainers
answer on #5420.

- **`CancelTurn`.** An exact execution command in
  `pluginsdk.HostExecutionCommands`, under `api_write: execution` and the
  workspace grant `host.v2.write:execution`. It is admitted and fenced exactly
  like `StopTaskRun`: observed session resource version, execution ID, and
  management claim. It delegates to `orchestrator.Service.CancelAgent`, so the
  session ends `WAITING_FOR_INPUT` and stays promptable. The open question is
  whether turn cancellation may be plugin-initiated at all or remains a native
  human action.
- **External human-response authority.** One of three variants:
  - **(a) Delegated grant with external actor attestation (requested).** A
    workspace manager grants a new Host v2 delegation (working name
    `host.v2.delegate:human_responses`) in Settings > Plugins > Workspace
    access, with reason, approval revision, audit, and revocation. With it, a
    plugin requests a single-use, five-minute receipt bound to one interaction
    revision and the exact response payload. The request carries an
    attestation `{provider, subject, display_name}`. The Host stores the
    attestation on the resolved interaction and in the audit history, labeled
    as plugin-attested. The rule that plugins never mint approval for, or
    substitute, a Kandev human identity (#5202) is unchanged; the attestation
    names an external person only.
  - **(b) Account linking.** An external identity is linked to a Kandev user
    once, through an authenticated native flow. Receipts are minted for the
    linked user only while that user holds `session.control`. Attribution is a
    real Kandev identity, at the cost of a linking flow, link storage, and
    unlink and revocation semantics.
  - **(c) Host-verified provider signatures.** The Host verifies the
    provider's signed interaction payload (Discord Ed25519) and mints the
    receipt itself. The plugin cannot forge an actor, but the Host gains
    provider-specific verification code and key configuration, and the plugin
    must forward raw signed interaction bodies, which a gateway-based bot does
    not receive.
- **Step approval.** `ApproveStep(session_id, receipt)` reuses
  `task/service.Service.ApproveSession`, the same path as the native Approve
  button, and is gated by the authority chosen above with a `step_approval`
  receipt kind. `GetStepApproval(session_id)` is a read under
  `api_read: sessions` that reports whether approval is awaited and the
  source and target steps. Nothing awaiting approval returns
  `FailedPrecondition`; a replay returns `ALREADY_APPLIED` and never moves the
  task twice. Office approval stages return `Unimplemented`.

When the maintainers answer, this ADR is updated to `accepted` with the
selected variant and the rejected alternatives move to the section below.

## Consequences

- Plugins lose access to `raw_content`. A third-party plugin that read it must
  use the stripped `content`. No official plugin reads it.
- `Message.Source` adds a field to the public SDK and wire DTO; adopting it
  requires a `pluginsdk` release and a raised `min_kandev_version` in plugins.
- Every pending addition follows the exact-command conventions: resource
  version fencing, idempotency keys, approval revision, manifest digest, and
  Host v2 workspace grants. Missing grants return `PermissionDenied`.
- Variant (a) weakens the invariant that only the native UI obtains response
  receipts. It stays bounded by an explicit per-workspace grant, revocation,
  single-use payload-bound receipts, and an audited attestation that never
  names a Kandev user.

## Alternatives Considered

- **Deep links only.** Chat commands open the Kandev UI instead of acting.
  This works today but defeats the mobile and chat use case.
- **Plugin-minted receipts without a grant.** Rejected: any installed plugin
  could answer for humans.
- **`Tasks().Move` for step approval.** Rejected: it skips `review_status` and
  the configured-transition resolution, so the result diverges from the native
  Approve button.
- **`message.added` subscriptions for echo suppression.** Rejected: streaming
  emits one event per chunk and floods the bounded plugin event queue.
- **Strip `raw_content` at the publisher.** Rejected: the native UI renders
  hidden prompts from it, so only the plugin delivery boundary changes.
