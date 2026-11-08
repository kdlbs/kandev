---
status: draft
system: agents
created: 2026-10-29
requirements:
  - REQ-AGENTS-PERMISSION-CONTROL-INTEGRITY-003
---

# Automatic Permission Presentation System Design

## Purpose and ownership

Agents owns this design because automatic permission policy determines whether
an interaction needs a person. The design extends
[permission control integrity](agent-permission-control-integrity.md#auto-approve-selection)
without changing live authority or decision ordering.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| `REQ-AGENTS-PERMISSION-CONTROL-INTEGRITY-003` | Message contract, failure and replay, shared presentation and attention, desktop and phone |

## Presentation contract

This amendment covers criteria 003.12-003.17. Automatic candidates currently
create ordinary pending messages before resolution. Chat displays those
messages until the approved status arrives. Preserve the durable ordering and
carry presentation intent in the first message instead.

## Message contract

Add a typed `permission_presentation` metadata value: `automatic`,
`interactive`, or `unavailable`. An absent or unknown value retains legacy
interactive behavior. This value controls presentation only. Live agentctl
requests remain the authority under
[the permission authority ADR](../../../decisions/2026-08-11-live-agent-permission-authority.md).

Introduce internal `PermissionRequestMessageOptions` with `Decision` and
`Presentation` fields in place of the existing optional decision argument.
`handlePermissionRequest` derives `automatic` only from the existing
`AutoApprovalPending` candidate event. `messageCreatorAdapter` writes it during
the idempotent create, before message publication. It does not preapprove the
request or move presentation data into `PermissionResolutionAudit`.

Keep `message -> claim -> provider delivery -> finalization`. Accepted status
and audit remain authoritative even when the row retains its original
`automatic` presentation. Older already-approved events still create approved
audit records. Unmarked requests retain their current behavior.

## Failure and replay

| Observation | Presentation result |
| --- | --- |
| Automatic attempt still active | Ordinary tool progress, no permission input |
| Accepted decision | Existing completed tool and hidden approved request |
| Failure before claim, exact live request still pending, no audit claim | Guarded promotion to `interactive` |
| Withdrawn or replaced request | Existing expired handling, no answer controls |
| Dispatch claim with uncertain delivery, or failed finalization | `unavailable` feedback, no second answer |

Use the full task/session/request/pending identity for presentation updates.
Promotion must conditionally match an automatic row without a resolution
claim. Revalidate the live generation first. A concurrent claim or replacement
defeats promotion. Preserve terminal status and every audit field.

Automatic resolution claims with an automatic-presentation guard, so a row
already promoted to a person's fallback cannot be claimed automatically when the
original candidate event is replayed. If live enumeration fails, an automatic
claim's read-back fails, or the provider withdraws or replaces the request,
settlement records non-actionable unavailable feedback instead of leaving the
row hidden as automatic. The same bounded cleanup runs after the originating
request context is cancelled. A competing dispatching resolver or accepted
outcome keeps ownership, including an accepted finalization whose read-back
failed.

An in-progress competing resolver is not a failed decision. Do not overwrite
its outcome. For a stopped attempt with uncertain delivery, use existing
localized unavailable/recovery feedback and keep Stop available. Do not expire
a running tool solely to change its presentation.

Replay uses the existing idempotent event and claim paths. Duplicate creation
must preserve a later fallback or terminal state. A durable dispatch claim
continues to block another delivery. This repair adds no automatic recovery
decision, timeout-based approval, or retry scheduler. A storage outage retains
existing error handling until a durable update succeeds.

## Shared presentation and attention

Add a dependency-neutral classifier under `apps/web/lib/permissions/`.
`parsePermission`, transcript filtering, and permission attention selectors
consume it. Standalone requests and permission state attached to `tool_call`,
`tool_execute`, or Kandev tool rows must agree. Automatic rows must not mount
approval controls, amber styling, or accessibility announcements for input.
Unavailable rows use existing localized feedback without approval controls.
When an attached permission row is deduplicated into its tool row, that tool row
renders the same non-actionable feedback instead of dropping the explanation.

Preserve current-turn and newest-permission authority. In SQL projections and
browser selectors, select the current row first, then classify its
presentation. Filtering candidates before ranking can resurrect an older
pending request. Cover both an older manual row superseded by an automatic row
and a separate session with a current manual request.

Update `pendingActionsBySessionQuery` and `ListPendingInteractions` consistently.
These projections describe input owed by a person. Do not filter
`ListPendingAgentPermissions`, which must remain the live inventory used for
automatic resolution. Retain SQLite/Postgres parity and existing authorization.

Persisted metadata feeds both HTTP hydration and WebSocket updates. Reuse
message freshness ordering so stale snapshots cannot rehide a promoted prompt
or reopen an approved one. Never infer intent from a profile toggle, tool
title, provider name, or a browser timer.

## Desktop and phone

Reuse the existing transcript, tool rows, and `PermissionActionRow`. No new
drawer, dialog, navigation, or scroll region is needed. Task chat is the entry
point on both layouts. Automatic approval leaves its normal tool row in place.
Interactive fallback uses the existing inline choices and localized labels.
Retain the phone transcript scroll owner, composer safe-area behavior, touch
targets, and keyboard access. No focus transfer occurs for automatic rows.

## Verification and compatibility

Use deterministic barriers around automatic resolution in backend/component
tests. Observe browser DOM mutations before prompt submission through tool
completion. An assertion made only after idle cannot detect this defect.
Cover standalone and attached rows, fallback, claim uncertainty, hydration,
replay, mixed sessions, and unchanged manual approvals on desktop and phone.

The marker is additive message metadata. It needs no SQL schema migration,
provider RPC extension, new profile setting, or runtime flag. Known automatic
candidates work across transports that already emit the candidate fields.
Other provider shapes keep interactive behavior. This local projection change
preserves the existing authority ADR and needs no separate architecture decision.


## Implementation plan

[Automatic permission presentation repair](../../../plans/automatic-permission-presentation/plan.md)
owns delivery and its regression evidence.
