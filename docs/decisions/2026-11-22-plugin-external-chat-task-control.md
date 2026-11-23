# ADR-2026-11-22-plugin-external-chat-task-control: Plugin task control from external chat surfaces

**Status:** accepted
**Date:** 2026-11-22
**Area:** protocol

## Context

Chat-bridge plugins such as
[`kandev-plugin-discord`](../specs/integrations/system-design/discord.md) map an external
conversation (a Discord channel) to a Kandev task. They mirror agent output,
deliver chat messages to the task, and let operators stop work, answer agent
permission requests, and approve workflow review steps from chat. The request
is [kdlbs/kandev#5420](https://github.com/kdlbs/kandev/issues/5420); the
maintainer decision is
[#5420 (comment)](https://github.com/kdlbs/kandev/issues/5420#issuecomment-7083766483).

Most of that runs on the shipped Host API: `Tasks()`, `Messages().Send/List`,
`Interactions().ListPending/Get`, exact `StopTaskRun`, and admin `actions`.
Five gaps remained:

1. Exact `StopTaskRun` ends the execution. Nothing let a plugin interrupt only
   the current turn, as the native `agent.cancel` action does through
   `orchestrator.Service.CancelAgent`.
2. Exact `RespondPermission`/`AnswerClarification` require a single-use human
   response receipt. Only an authenticated native UI request
   (`POST /api/plugins/host/interactions/response-receipts`) by a user with
   `session.control` could obtain one
   ([coordination platform decision](2026-09-25-plugin-coordination-platform.md)).
   A person in an external chat could never answer.
3. The native Approve button (`POST /api/v1/sessions/:id/approve`, served by
   `task/service.Service.ApproveSession`) resolves the step's configured
   transition, moves the task, and sets `review_status=approved`. A plugin
   could only call `Tasks().Move`, which records no approval and does not
   resolve the configured target.
4. The `pluginsdk.Message` read DTO carried no provenance, so a bridge could
   not recognize the messages it delivered itself without subscribing to the
   high-volume `message.*` events.
5. `newMessageEvent` adds `raw_content` (the unstripped message) whenever a
   message holds hidden `<kandev-system>` content, plugin event delivery
   forwards bus payloads unchanged, and the public docs claimed plugins never
   see injected system content.

Issue #5202 asks for native human approval of plugin tool operations bound to
an operation digest, with the rule that agents and plugins never mint
approval or substitute a human name. Item 2 crosses the same boundary, so both
must use one receipt model.

## Decision

### Turn cancellation

`CancelTurn` is an exact execution command in
`pluginsdk.HostExecutionCommands`, under `api_write: execution` and the
workspace grant `host.v2.write:execution`. Admission and fencing are the same
as `StopTaskRun`: request ID, idempotency key, observed session resource
version, observed execution ID, management claim fence, approval revision, and
manifest digest. It delegates to `orchestrator.Service.CancelAgent`, so the
session ends `WAITING_FOR_INPUT`, stays promptable, and the queue is not
drained. It returns `APPLIED` when a turn was interrupted, `NO_CHANGE` when no
turn was active, `CONFLICT` for a stale session or execution, and
`ALREADY_APPLIED` on replay.

### External human responses: linked Kandev users behind an admin grant

A plugin relays a human decision only for a **Kandev user** who linked the
external identity, and only in a workspace whose manager granted the
installation that authority.

- **Grant.** The manifest declares `api_write: [human_responses]`, which maps
  to the Host v2 capability `host.v2.write:human_responses`. A workspace
  manager approves it per installation in Settings > Plugins > Workspace
  access, with a reason, an approval revision, and the existing audit history,
  and can revoke it. The Host checks the grant when a receipt is requested and
  again when it is consumed.
- **Link.** A signed-in Kandev user asks Kandev for a link code on the plugin's
  settings page. The code is random, single-use, valid for ten minutes, bound
  to that user and that plugin installation, and stored only as a digest. The
  user gives it to the external surface (for example `/link <code>` in
  Discord), and the plugin redeems it with `LinkExternalIdentity(code,
  provider, subject, display_name)`. Redemption requires the manifest
  declaration above; it creates one durable link
  `{installation, provider, subject} -> Kandev user`. A subject links to at
  most one user and a user to at most one subject per provider and
  installation. The user can revoke their own links and an instance admin can
  revoke any link of an installation from the same page. Uninstalling the
  plugin deletes its links.
- **Receipt.** With the grant and an active link, the plugin calls
  `RequestHumanResponseReceipt` with the workspace, provider, subject, target
  (an interaction, or a session for step approval), observed resource version,
  and exact response payload. The Host resolves the linked user, verifies the
  user is active and holds `session.control` on the workspace, verifies the
  target is still pending at that revision, and mints a receipt. The receipt
  has the same shape as a native receipt: single-use, five minutes, bound to
  the workspace, target, revision, and payload digest. A delegated receipt is
  also bound to the requesting installation and to the link. Unlinked
  subjects, revoked links, and users without `session.control` are denied;
  the plugin then offers a deep link to Kandev instead of an action.
- **Consumption.** The existing exact `RespondPermission`,
  `AnswerClarification`, and the new `ApproveStep` accept the receipt
  unchanged. At consumption the Host re-checks the installation binding, that
  the link is still active, and that the user still holds `session.control`,
  so a revocation takes effect inside the five-minute window. The response
  runs under the linked user's identity, never a plugin-chosen one.
- **Attribution and audit.** Every delegated receipt is recorded durably with
  the installation, workspace, Kandev user, provider, subject, target kind and
  ID, payload digest, issue time, and consumption time. The resolved
  interaction (`Interactions().Get`) carries `ResolvedBy {user_id,
  display_name, via}`, for example "alice via discord", and the permission
  resolution audit records the Kandev user as the actor with source `plugin`.

### One receipt model with #5202

A **human response receipt** is the only proof that a person decided. It
always binds: the authenticated Kandev user who decided; the surface that
carried the decision (the native UI, or a plugin installation plus the
external provider); the workspace; the target kind, target ID, and target
revision; the exact payload digest; an expiry; and single use. The Host
derives the user from an authenticated session or from an active link; a
plugin never names the user. Plugin-relayed approvals (this decision) and
plugin-tool approvals (#5202) differ only in target kind: #5202 adds a target
kind whose revision is the operation digest. Neither path lets a plugin mint a
receipt on its own authority.

### Workflow step approval

`ApproveStep` is an exact command under `api_write: execution` and
`host.v2.write:execution` that also requires a `step_approval` receipt from the
flow above. It reuses `task/service.Service.ApproveSession`, the path behind
the native Approve button, so the task moves to the same configured target,
`task.moved` and `on_enter` fire, and `review_status` becomes `approved`.
Following the exact-command convention, nothing awaiting approval is the typed
result `CONFLICT` with reason `step_not_awaiting_approval` (the command form
of `FailedPrecondition`); a replay returns `ALREADY_APPLIED`, and it never
moves a task twice. It carries no management-claim fence: like an
interaction response, it is a human decision.

`GetStepApproval(session_id)` is an exact read under `api_read: sessions` and
`host.v2.read:sessions` that returns whether approval is awaited, the source
and target steps, and the resource version that the receipt and
`ApproveStep` bind to. Approval is awaited when the session is not running or
starting, `review_status` is not `approved`, and either `review_status` is
`pending` or the current step has an `on_turn_complete` transition with
`requires_approval=true` and the session's latest turn completed in the
current step entry. Turns already record the step and the step-transition
entry they started in, so a turn from an earlier visit to the same step does
not count and no timestamp comparison is needed. The target step uses the same
resolver as `ApproveSession`. An Office approval stage (`stage_type:
approval`) returns `Unimplemented`.

### Message provenance and event payloads

- `pluginsdk.Message.Source` carries the persisted `metadata.source` (for
  example `plugin:<id>`) and nothing else, on `Messages().List` and on exact
  message observations.
- `message.*` event payloads keep `raw_content` so existing consumers do not
  break; plugin delivery forwards the bus payload as it is. `Messages().List`
  stays stripped. The public docs state that `message.*` payloads may include
  `raw_content` with the unstripped `<kandev-system>` content.

### Non-goals

Interrupt-by-message, arbitrary workflow moves beyond the existing
`Tasks().Move`, Office approval-stage decisions, plugin-asserted identities
without a link, provider-specific signature verification in the Host, and
rendering response attribution in the native chat UI.

## Consequences

- A chat operator can stop a turn, answer a permission request or question,
  and approve a review step, and Kandev records which Kandev user acted and
  through which surface.
- A workspace that never grants `host.v2.write:human_responses` is unaffected.
  A user who never links an identity cannot be acted for.
- The Host stores identity links and delegated receipt records in the plugin
  store. Link codes and receipts stay in memory: a restart invalidates
  unredeemed codes and unconsumed receipts, which only costs a retry.
- New proto RPCs and DTO fields require a `pluginsdk` minor release; plugins
  raise `min_kandev_version` to use them.
- `raw_content` remains visible to plugins that subscribe to `message.*`.

## Alternatives Considered

- **(a) Delegated grant with a plugin-attested external actor.** Rejected: an
  unverifiable attestation would let a plugin approve under any name, which
  conflicts with #5202. Its admin grant is kept as the gate in front of
  linking.
- **(c) Host-verified provider signatures.** Rejected: it puts
  Discord-specific verification in the Host and still does not identify the
  Kandev user who acts.
- **Deep links only.** Chat commands open the Kandev UI instead of acting.
  It remains the fallback for unlinked users.
- **`Tasks().Move` for step approval.** Rejected: it skips `review_status` and
  the configured-transition resolution.
- **`message.added` subscriptions for echo suppression.** Rejected: streaming
  emits one event per chunk and floods the bounded plugin event queue.
- **Strip `raw_content` from plugin deliveries.** Rejected by the maintainers
  to avoid breaking existing consumers; the docs were corrected instead.
