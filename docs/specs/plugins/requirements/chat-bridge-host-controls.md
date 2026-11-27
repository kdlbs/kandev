---
status: draft
system: plugins
created: 2026-11-23
owners:
  - Fclem
---

# Chat-Bridge Host Controls Requirements

> Technical design and decisions:
> `docs/specs/plugins/system-design/chat-bridge-host-controls.md` and
> `docs/decisions/2026-11-22-plugin-external-chat-task-control.md`.

## Overview

A chat-bridge plugin, such as the Discord plugin, mirrors a task into an
external chat surface. People in that surface need to stop a turn, answer an
agent's permission and clarification prompts, and approve a workflow review
step. The plugin must do this on behalf of a real Kandev user, never through a
bot account with broad authority.

The maintainers agreed the shape on
[#5420](https://github.com/kdlbs/kandev/issues/5420#issuecomment-7083766483).
This system owns the Host contract for it: exact turn cancellation, linked
accounts behind an admin grant, human-response receipts, workflow step
approval, and message provenance.

## Terminology

- **Installation:** one installed copy of a plugin.
- **Grant:** a workspace manager's approval of the Host v2 capability
  `host.v2.write:human_responses` for one installation, with a reason and an
  approval revision, revocable and audited.
- **Link:** a durable record that one external account (provider and subject),
  as seen by one installation, belongs to one Kandev user.
- **Link code:** a short-lived, single-use code that a signed-in Kandev user
  creates and gives to the external surface to prove ownership.
- **Receipt:** a single-use, five-minute authority minted for one linked user,
  one target, one observed revision, and one exact response payload.
- **Awaiting approval:** a session whose current workflow step the native
  Approve action would move forward.

## Requirements

### REQ-PLUGINS-CHAT-BRIDGE-HOST-001: Decision record

The design for plugin-driven task control is recorded before it ships.

- **AC-PLUGINS-CHAT-BRIDGE-HOST-001.1:** When the decision is accepted, an ADR under
  `docs/decisions/` shall record turn cancellation, linked-account human
  responses behind an admin grant, step approval, message source, the
  `raw_content` decision, and the non-goals.
- **AC-PLUGINS-CHAT-BRIDGE-HOST-001.2:** When the ADR exists, `docs/decisions/INDEX.md`
  shall list it.

### REQ-PLUGINS-CHAT-BRIDGE-HOST-002: Exact turn cancellation

A plugin can interrupt the current turn of a session without ending the
session.

- **AC-PLUGINS-CHAT-BRIDGE-HOST-002.1:** When a plugin with `host.v2.write:execution`
  calls `CancelTurn` with the observed session version and execution ID, the
  system shall apply the same admission and fencing as `StopTaskRun`.
- **AC-PLUGINS-CHAT-BRIDGE-HOST-002.2:** When a turn is running, the system shall
  interrupt it without terminating the agent process, leave the session
  promptable, and return `APPLIED`.
- **AC-PLUGINS-CHAT-BRIDGE-HOST-002.3:** When no turn is running, the system shall
  return `NO_CHANGE`; when the session version or execution ID is stale, it
  shall return `CONFLICT`; when the idempotency key repeats, it shall replay
  the first outcome.
- **AC-PLUGINS-CHAT-BRIDGE-HOST-002.4:** When the observed turn ends and another
  turn starts before the cancellation is applied, the system shall return
  `CONFLICT` and shall not interrupt the newer turn.
- **AC-PLUGINS-CHAT-BRIDGE-HOST-002.5:** When a plugin runs in the runtime tests, it
  shall be able to call `CancelTurn` end to end, and the command shall appear
  in capability discovery like its sibling commands.

### REQ-PLUGINS-CHAT-BRIDGE-HOST-003: Event payload content visibility

A plugin knows which message fields carry unstripped system content.

- **AC-PLUGINS-CHAT-BRIDGE-HOST-003.1:** When a plugin subscribes to `message.*`
  events, the event payload shall keep the `raw_content` field, so existing
  consumers do not break.
- **AC-PLUGINS-CHAT-BRIDGE-HOST-003.2:** When a plugin must not handle system
  content, the public documentation shall direct it to `content` or
  `Messages().List`, which stay stripped.

### REQ-PLUGINS-CHAT-BRIDGE-HOST-004: Linked accounts

A Kandev user can link an external account to their identity for one plugin
installation.

- **AC-PLUGINS-CHAT-BRIDGE-HOST-004.1:** When a manifest declares
  `api_write: [human_responses]`, the system shall request
  `host.v2.write:human_responses`, and only a workspace manager shall be able to
  grant or revoke it; a plugin shall not grant itself.
- **AC-PLUGINS-CHAT-BRIDGE-HOST-004.2:** When a signed-in user requests a link code
  for an installed plugin that declares `human_responses`, the system shall
  issue a random, single-use code that is valid for ten minutes, bound to that
  user and installation, and stored only as a digest.
- **AC-PLUGINS-CHAT-BRIDGE-HOST-004.3:** When a plugin calls `LinkExternalIdentity`
  and the installation holds the grant in at least one workspace, the system
  shall redeem the code for the calling installation only and create or replace
  the link for that subject and that user.
- **AC-PLUGINS-CHAT-BRIDGE-HOST-004.4:** When no workspace currently grants the
  installation `host.v2.write:human_responses`, the system shall deny the link
  and leave the code unspent.
- **AC-PLUGINS-CHAT-BRIDGE-HOST-004.5:** When a code is unknown, expired, used, or
  presented by another installation, the system shall deny the link.
- **AC-PLUGINS-CHAT-BRIDGE-HOST-004.6:** When a user lists links, the system shall
  return their own links, or every link of the installation for an instance
  admin; a user shall revoke their own link and an admin any link, and
  uninstalling the plugin shall delete its links.
- **AC-PLUGINS-CHAT-BRIDGE-HOST-004.7:** When the user opens the plugin settings of
  an installation that declares `human_responses`, the page shall show a
  "Linked chat accounts" card with the code generator, the link list, and
  revoke actions, and the card shall refresh when the page regains focus.

### REQ-PLUGINS-CHAT-BRIDGE-HOST-005: Human response receipts and attribution

A plugin can relay a permission or clarification answer as a linked user.

- **AC-PLUGINS-CHAT-BRIDGE-HOST-005.1:** When a plugin requests a receipt, the system
  shall require the workspace grant, an active link, an active linked user with
  `session.control` on the workspace, and a target still pending at the observed
  revision; otherwise it shall return a typed denial, conflict, or not-found.
- **AC-PLUGINS-CHAT-BRIDGE-HOST-005.2:** When a receipt is minted, it shall be bound
  to the installation, link, user, workspace, target, revision, and payload
  digest, last five minutes, and be usable once.
- **AC-PLUGINS-CHAT-BRIDGE-HOST-005.3:** When a receipt is spent through
  `RespondPermission` or `AnswerClarification`, the system shall recheck the
  installation, link, grant, and `session.control`, and run the response as the
  linked user; another installation shall not spend it.
- **AC-PLUGINS-CHAT-BRIDGE-HOST-005.4:** When a response is delivered, the system
  shall record the receipt durably before delivery, so the audit trail
  identifies the receipt even if the completion marker is lost afterward, and
  shall refuse delivery when that record cannot be written.
- **AC-PLUGINS-CHAT-BRIDGE-HOST-005.5:** When a delivery fails, the system shall
  release the receipt record so a later resolution by someone else is never
  attributed to the linked user.
- **AC-PLUGINS-CHAT-BRIDGE-HOST-005.6:** When an interaction was resolved with a
  delegated receipt, `Interactions().Get` shall return `ResolvedBy` with the
  user, display name, and provider, and the permission audit shall record the
  linked user with source `plugin`, including when the response was a dismissal.
- **AC-PLUGINS-CHAT-BRIDGE-HOST-005.7:** When the grant is missing or revoked, the
  link is missing or revoked, the receipt is replayed, the payload differs, the
  interaction is in another workspace, or the manifest lacks the declaration,
  the system shall refuse the response and make no change.

### REQ-PLUGINS-CHAT-BRIDGE-HOST-006: Workflow step approval

A plugin can read whether a step awaits approval and approve it as a linked
user.

- **AC-PLUGINS-CHAT-BRIDGE-HOST-006.1:** When a plugin with `api_read: sessions`
  calls `GetStepApproval`, the system shall return whether the session awaits
  approval and the from and to step IDs and names.
- **AC-PLUGINS-CHAT-BRIDGE-HOST-006.2:** When the session is not running or starting,
  its review status is not approved, and either the review status is pending or
  a turn completed in a step that requires approval, the system shall report
  that the session is awaiting approval.
- **AC-PLUGINS-CHAT-BRIDGE-HOST-006.3:** When a plugin calls `ApproveStep` with a
  `step_approval` receipt, the system shall approve through the native Approve
  path, so step-entry actions run and the review status becomes approved, and
  return the from and to step IDs; a final step shall approve without moving.
- **AC-PLUGINS-CHAT-BRIDGE-HOST-006.4:** When nothing is awaiting approval, the
  system shall return a conflict with reason `step_not_awaiting_approval`; when
  the idempotency key repeats, it shall return `ALREADY_APPLIED` without moving
  the task twice; an Office approval stage shall be unimplemented.
- **AC-PLUGINS-CHAT-BRIDGE-HOST-006.5:** When the task leaves the observed step
  between the read and the write, the system shall return a conflict with
  reason `step_approval_changed` and shall not advance the task.
- **AC-PLUGINS-CHAT-BRIDGE-HOST-006.6:** When the task move commits but the
  review-status write fails, the system shall report the approval as applied, so
  a replayed idempotency key never stores a failure for a move that happened.

### REQ-PLUGINS-CHAT-BRIDGE-HOST-007: Message source

A plugin can tell which surface wrote a message.

- **AC-PLUGINS-CHAT-BRIDGE-HOST-007.1:** When a message has a persisted
  `metadata.source`, `Message.Source` shall carry it, and shall be empty
  otherwise; no other metadata shall be exposed.
- **AC-PLUGINS-CHAT-BRIDGE-HOST-007.2:** When a plugin lists messages or reads an
  exact message observation, `Source` shall be present and survive the wire
  round trip.

### REQ-PLUGINS-CHAT-BRIDGE-HOST-008: Public plugin documentation

Plugin authors can learn the new contract without reading Host code.

- **AC-PLUGINS-CHAT-BRIDGE-HOST-008.1:** When the contract ships,
  `docs/public/plugins-authoring.md` shall cover `CancelTurn`, the linked-account
  flow, `ApproveStep`, `GetStepApproval`, and `Message.Source` with examples.
- **AC-PLUGINS-CHAT-BRIDGE-HOST-008.2:** When the contract ships,
  `docs/public/plugins-manifest.md` shall document the `human_responses` write
  resource and its grant, and the `raw_content` payload behavior.

## Exclusions

- Interrupting a turn by sending a message.
- Moving a task to an arbitrary step.
- Deciding an Office approval stage.
- Plugin-minted receipts without a grant.
- Verifying a provider's request signatures in the Host.
