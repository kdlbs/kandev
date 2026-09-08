---
status: draft
system: office
requirements:
  - REQ-OFFICE-AGENT-RECOVERY-001
  - REQ-OFFICE-AGENT-RECOVERY-002
  - REQ-OFFICE-AGENT-RECOVERY-003
---

# Office Agent Recovery System Design

## Purpose and boundaries

Office owns Office agent identity, the `status` and `pause_reason` fields on an
agent row, the transition table that governs them, and the scheduler gates that
read them. This design covers the operator-facing path that drives one of those
transitions, across both runtime boundaries: the web control and the existing
HTTP endpoint it calls.

Adjacent contracts this design uses but does not own:

- The Office agent status transition table and the status endpoint's validation
  (`internal/office/agents`). Consumed as-is; this design adds no transition.
- The auto-pause recovery flow (`MarkAgentPausedFixed`), which owns the
  failure-counter reset, inbox dismissal, and an attempted run re-queue, and
  which writes the agent's status back unchanged — so while that status is
  `paused`, the re-queue it attempts is refused. Not invoked here.
- The Office inbox's failure sources. They are not called by this design, but
  they READ the field it writes: both the consolidated auto-paused entry and the
  suppression of the individual failed-run entries are selected by matching
  `pause_reason` against the auto-pause prefix. Clearing `pause_reason` therefore
  changes what the inbox lists, with no inbox code on this path. The
  operator-visible shape of that change, and the ordering guidance it creates
  between this control and "Mark fixed", are specified in the requirement's
  "Consequences of the named exclusions". An implementation must not add
  dismissal writes to compensate.
- The Office live-update broadcaster. It carries no event for this endpoint;
  see [Failure and recovery](#failure-and-recovery).

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-OFFICE-AGENT-RECOVERY-001` | [Components and responsibilities](#components-and-responsibilities), [Control flow](#control-flow) |
| `REQ-OFFICE-AGENT-RECOVERY-002` | [Failure and recovery](#failure-and-recovery) |
| `REQ-OFFICE-AGENT-RECOVERY-003` | [Components and responsibilities](#components-and-responsibilities), [Security](#security) |

## Components and responsibilities

**Agent detail identity strip (web).** The agent detail layout renders a
persistent identity strip above the tab navigation, on every agent sub-route.
It already renders the status indicator and status label, so the recovery
control and the pause-reason text belong beside the state they describe, and
placing them in the layout rather than in one tab satisfies
AC-OFFICE-AGENT-RECOVERY-001.1 without repeating the control per tab.

**Office agent API client (web).** Owns the single status mutation. The web
codebase has no caller of the agent status endpoint today; the only existing
caller in the repository is the end-to-end test helper. This design adds the
production client function rather than reaching for the general agent update
endpoint — see [Data and contracts](#data-and-contracts) for why that
distinction is load-bearing.

**Office agent store slice (web).** Holds the workspace's agent rows and is the
single read source for the identity strip, the agent list, and the dashboard.
Patching the recovered agent here is what makes AC-OFFICE-AGENT-RECOVERY-001.5
hold across those surfaces without a reload.

**Office agent status endpoint (backend, existing, unchanged).** Validates the
transition, persists status and pause reason, and returns the updated agent.

**Office agent list (web, existing, unchanged).** Applies no status filter, so a
`paused` or `stopped` agent is already listed and already links to its detail
surface. AC-OFFICE-AGENT-RECOVERY-003.1 is a regression guard over existing
behavior, not new work.

## Data and contracts

Request:

```text
PATCH /api/v1/office/agents/:id/status
{ "status": "idle" }
```

Response `200`:

```text
{ "agent": { ... , "status": "idle" } }
```

**`pause_reason` is absent from that body, not empty.** The agent model carries
`pause_reason` with `omitempty` on a plain string, so the cleared value is
omitted from the JSON entirely rather than serialised as `""`. Any consumer of
this response must treat an absent `pause_reason` as empty. The web layer's
existing agent normalizer already does this — it coerces a missing string field
to `""` — so the design's requirement is that the response be read **through
that normalizer** rather than by reaching into the raw JSON body, where the
field would come back `undefined` and a naive patch would leave the previous
pause reason on screen in violation of AC-OFFICE-AGENT-RECOVERY-001.7. The same
`omitempty` treatment applies to `status`, which is why the success path must
also assert on the normalized value rather than on key presence.

Three further properties of this contract shape the design:

1. **The endpoint is the only validating writer.** The general agent update
   endpoint (`PATCH /api/v1/office/agents/:id`) also accepts a `status` field,
   but it assigns it directly and never consults the transition table. The
   recovery control must use the `/status` endpoint so that an illegal
   transition is refused rather than silently written.

2. **Omitting `pause_reason` clears it.** The request field is a plain string,
   and the persistence layer preserves the stored `pause_reason` only when the
   *incoming* status is `working`. A recovery request therefore writes an empty
   pause reason as a side effect of writing `idle`, which is what
   AC-OFFICE-AGENT-RECOVERY-001.7 requires. The same statement clears
   `working_run_id`.

3. **The response body is the authority.** The handler returns the agent it just
   wrote, so the acting client never has to guess and never has to re-fetch to
   satisfy AC-OFFICE-AGENT-RECOVERY-001.4.

Statuses whose transition set contains `idle`: `paused`, `stopped`,
`working`, `pending_approval`, and `idle` itself (a same-status write is
accepted as a no-op). Restricting the control to `paused` and `stopped` per
AC-OFFICE-AGENT-RECOVERY-001.2 is therefore a **presentation** decision, not a
constraint the endpoint enforces — the control is not shown for `working` or
`pending_approval` even though the endpoint would accept those requests. An
implementation must gate on the rendered status rather than assume the backend
will refuse.

## Control flow

1. The identity strip reads the agent from the store. When the status is
   `paused` or `stopped`, it renders the recovery control; when `pause_reason`
   is non-empty, it renders that text.
2. Activation marks the request in flight, which disables further activation
   (AC-OFFICE-AGENT-RECOVERY-002.1).
3. The API client issues the `PATCH` with the constant target `idle`. The target
   is not derived from the rendered status, which is what makes a stale render
   harmless (AC-OFFICE-AGENT-RECOVERY-002.5) and a repeat request a no-op
   (AC-OFFICE-AGENT-RECOVERY-002.3).
4. On `200`, the client patches the store from the normalized response agent's
   `status` and `pause_reason`, where an absent `pause_reason` normalizes to the
   empty string (see [Data and contracts](#data-and-contracts)). No optimistic
   write precedes this.
5. On failure, the store is not touched, an error is surfaced, and the in-flight
   flag clears so the operator can retry.

No step queues a run, touches the failure counter, or dismisses an inbox entry,
which is how AC-OFFICE-AGENT-RECOVERY-001.8 is satisfied — by omission, and the
omission is deliberate.

## Failure and recovery

The endpoint answers every service-layer failure with `400` and an `error`
string, including a target agent that cannot be resolved. An implementation must
not distinguish "no such agent" from "illegal transition" by status code; both
are the failed-request path of AC-OFFICE-AGENT-RECOVERY-002.2 and
AC-OFFICE-AGENT-RECOVERY-002.6.

Transport failures and non-`2xx` responses are handled identically: displayed
state unchanged, error surfaced, control re-enabled. Because the request is
idempotent, an operator who retries after an ambiguous timeout cannot make
things worse.

**A concurrent "Mark fixed" can undo a completed recovery.** Auto-pause recovery
reads the agent row and writes the status it read back, so one that began before
a recovery landed can complete after it and restore `paused`. The recovery
response was still truthful when it was sent, and this design keeps
AC-OFFICE-AGENT-RECOVERY-001.4 by rendering it; the acting client is simply not
told about the later write, for the same reason it is not told about any other:
the endpoint publishes no event. The control reappears on the next refetch and
the operator can activate it again. Guarding the write is the other flow's to
fix, not this one's.

**No event is published.** The status endpoint emits nothing on the event bus
and writes no activity entry. The constant `office.agent.status_changed` is
declared in both the events package and the Office service package but has never
had a publisher or a websocket forwarding rule — the working-status broadcaster
documents this and deliberately publishes `office.agent.updated` instead, which
the web layer handles by refetching the agent list. The recovery control adds no
publisher, so a second operator's open view keeps the stale status until it
refetches for some other reason. The requirement scopes the live-update
guarantee to the acting client for exactly this reason. Adding a publisher would
close the gap and is a self-contained backend change, but it is a contract
change to the endpoint and is out of scope here.

## Persistence

None added. The status endpoint's existing single-statement update owns the
write. The agent runtime row is DB-resident and not part of exported YAML
configuration, so config reconciliation does not revert an operator's recovery
across a restart.

## Security

The status handler applies its role check only to callers presenting an agent
JWT: an agent may change its own status, and only an admin-role agent may change
another's. Requests without a bearer token are treated as operator/admin
requests and pass through. The recovery control is such a request, so it needs
no new permission and introduces no new trust boundary
(AC-OFFICE-AGENT-RECOVERY-003.2). The pause reason is operator-authored or
system-authored diagnostic text already returned by the agent list endpoint;
rendering it on the detail surface exposes nothing that surface's viewer could
not already read.

## Observability

The backend path is unchanged and logs nothing for this transition today. The
operator-visible signals are the rendered status, the presence or absence of the
recovery control, and the pause-reason text — which is what the acceptance
criteria assert against. If a run-time record of who recovered an agent is
wanted, it belongs with the endpoint as an activity entry, alongside the missing
event publisher, not in the web layer.

## Related decisions

- [Agent stall, user-controlled recovery](../../../decisions/2026-07-29-agent-stall-user-controlled-recovery.md)
- [Provider-neutral agent error recovery](../../../decisions/2026-08-08-provider-neutral-agent-error-recovery.md)
