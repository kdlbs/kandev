---
status: draft
system: plugins
requirements:
  - REQ-PLUGINS-CHAT-BRIDGE-HOST-001
  - REQ-PLUGINS-CHAT-BRIDGE-HOST-002
  - REQ-PLUGINS-CHAT-BRIDGE-HOST-003
  - REQ-PLUGINS-CHAT-BRIDGE-HOST-004
  - REQ-PLUGINS-CHAT-BRIDGE-HOST-005
  - REQ-PLUGINS-CHAT-BRIDGE-HOST-006
  - REQ-PLUGINS-CHAT-BRIDGE-HOST-007
  - REQ-PLUGINS-CHAT-BRIDGE-HOST-008
created: 2026-11-23
owners:
  - Fclem
---

# Chat-Bridge Host Controls System Design

## Purpose and boundaries

This design carries the technical detail behind
`docs/specs/plugins/requirements/chat-bridge-host-controls.md`. The decision
record and rejected alternatives are in
`docs/decisions/2026-11-22-plugin-external-chat-task-control.md`.

The Host adds four surfaces to the existing exact-command model: turn
cancellation, linked accounts, human-response receipts, and step approval. It
adds one read field, `Message.Source`. It does not change the task system's
move semantics or the native approval rules.

## Exact command pattern

Every command carries a request ID, an idempotency key, the observed resource
version, the approval revision, and the manifest digest. The Host admits the
command in the durable command store before any effect, then completes it with
a typed result: `APPLIED`, `NO_CHANGE`, `CONFLICT`, `DENIED`, `NOT_FOUND`,
`UNAVAILABLE`, or `ALREADY_APPLIED` on replay. A replay never repeats the
effect.

## Turn cancellation (REQ-002)

`CancelTurn` runs under the same management-claim fence as `StopTaskRun`. The
controller first checks the session version and that the live execution is the
observed one. It then reads a cancel target, the live execution ID and the
active turn ID, through `orchestrator.Service.ObserveCancelTarget`.

`CancelAgentIfTarget` carries that target in the context to `CancelAgent`. The
orchestrator compares it again after it takes the session's in-flight cancel
guard and before it prepares the cancellation. A mismatch returns
`ErrCancelTargetChanged`, which the adapter maps to a conflict. The execution
ID alone is not enough, because one execution serves many turns.

## Linked accounts (REQ-004)

Link codes live in memory, stored by SHA-256 digest, with a ten-minute life and
one use. Links live in `plugin_identity_links`. Uninstalling a plugin deletes
its links.

`LinkExternalIdentity` checks, in order: the manifest declares `human_responses`,
the request is well formed, and `requireHumanResponsesGrant` finds an active,
non-tombstoned approval of `host.v2.write:human_responses` whose manifest digest
still matches the installed manifest. The request carries no workspace, and a
link belongs to an installation and a user, so one grant in any workspace opens
linking. The per-workspace grant is checked again whenever a receipt is
requested or spent. The grant check runs before the code is redeemed, so a
denied attempt leaves the code unspent.

The settings card reads links through `usePluginIdentityLinks`. The user
redeems a code in another app, so the hook refreshes the list quietly when the
window regains focus.

## Receipts and attribution (REQ-005)

A receipt is held in memory: single use, five minutes, bound to the
installation, link, user, workspace, target, revision, and payload digest. When
it is minted, the Host writes its audit row to `plugin_delegated_human_responses`.

Consumption follows a write-ahead order:

1. Spend the receipt in memory and recheck the link, grant, and
   `session.control`.
2. Write the claim, `claimed_at`, on the audit row. If this write fails, refuse
   the response. A delivery with no audit trail could not be attributed.
3. Deliver the native response as the linked user.
4. On failure, release the claim. On success, write `consumed_at`.

If step 4 loses the `consumed_at` write, the claim still identifies the
delivered receipt. When a resolved interaction is read and no consumed row
exists, `claimedResolution` accepts the newest claim for that target only if the
interaction resolved between `claimed_at` and the receipt's expiry. It then
repairs `consumed_at`. The native response is never repeated.

The permission audit records the linked user with `Source: plugin` and
`ActorKind: linked_identity`. A dismissal goes through
`orchestrator.Service.CancelAgentPermission`, so it carries the same
attribution as a selected option.

## Step approval (REQ-006)

`GetStepApprovalStatus` reads the session, task, current step, and next step,
and computes whether the step awaits approval. `ApproveAwaitingStep` runs the
caller's guard on the observed status, then calls the native `ApproveSession`
logic with the observed step ID.

The move uses `MoveTaskOptions.ExpectedStepID`. The task repository checks that
the task still occupies that step inside the write transaction that moves it,
with the same WIP admission and completion admission as an ordinary move. A
lost race returns `applied=false`, surfaced as `ErrStepApprovalChanged`, and the
task row is untouched. This check does not rebase the task from the stored row,
because the caller's task carries the move's own metadata and position.

The adapter maps `ErrStepApprovalChanged` to `step_approval_changed` and
`ErrStepNotAwaitingApproval` to `step_not_awaiting_approval`.

If the move commits and the review-status write then fails, `approveSession`
returns `ErrReviewStatusNotRecorded` with the result. `ApproveAwaitingStep`
retries the status write once and reports the approval as applied. Native
callers of `ApproveSession` still see the error.

## Message source (REQ-007)

`Message.Source` is populated from `metadata.source` only. The field is part of
the proto, the gRPC client and server, and the exact message observation.

## Persistence and required stores

The `plugin-command-receipts` store owns the command intent, receipt, and result
tables, the identity link table, and the delegated-response audit table. Its
constructor `state.NewCommandStore` creates all five. The store has a descriptor
in the required-store catalog, a fixed adapter in the conformance suite, and a
boot-time record, so a missing table fails before readiness.

## Failure modes

- A missing grant denies linking, receipt minting, and receipt spending.
- A revoked link between mint and spend denies the spend.
- A restart drops codes and unspent receipts. A plugin requests a new receipt.
- A lost `consumed_at` write is recovered from the claim on the next read.
- A failed release after a failed delivery leaves an in-doubt claim. It is
  honored only inside the receipt's lifetime window.

## Verification

Backend tests cover the receipt lifecycle, the grant gate, the write-ahead claim
and its recovery, the step-guarded move, the turn fence, and the store
conformance suite on SQLite and PostgreSQL.
