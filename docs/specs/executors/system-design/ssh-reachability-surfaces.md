---
status: draft
system: executors
requirements:
  - REQ-EXECUTORS-SSH-REACHABILITY-002
  - REQ-EXECUTORS-SSH-REACHABILITY-003
created: 2026-09-13
owners:
  - tbd
---
# SSH Host Reachability Surfaces System Design

## Purpose and boundaries

The [engine design](ssh-reachability.md) owns how a reachability record is
produced, decided and stored. This document owns how that record reaches a
user: the HTTP and WebSocket contracts, the web components that render it, and
the launch interaction. Nothing here decides reachability; every surface renders
state the engine defines.

The task-card indicator and the launch path's record *write* are deferred. Both
are specified in the requirement document's `## Out of scope`, and their
criterion IDs are retired rather than reused.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-EXECUTORS-SSH-REACHABILITY-002` | [API and event contracts](#api-and-event-contracts), [Frontend components](#frontend-components) |
| `REQ-EXECUTORS-SSH-REACHABILITY-003` | [Launch interaction](#launch-interaction) |

## API and event contracts

Reachability is its own resource, not fields on the executor DTO, so a record
changing every interval does not invalidate the executor list payload.

| Route | Purpose |
| --- | --- |
| `GET /api/v1/ssh/reachability` | Every SSH executor's current record in one request. |
| `GET /api/v1/ssh/executors/:id/reachability` | One executor's record. |
| `POST /api/v1/ssh/executors/:id/reachability/probe` | Run one probe now, persist it, return the record. |

The record shape, mirrored in `apps/web/lib/types/http-ssh.ts`:

```json
{
  "executor_id": "…",
  "state": "unknown | reachable | unreachable",
  "reason": "'' | config | network | timeout | auth | host_key | unknown",
  "message": "…",
  "consecutive_failures": 0,
  "host": "…",
  "checked_at": "RFC3339 | null",
  "last_success_at": "RFC3339 | null",
  "probing_enabled": true,
  "probe_interval_seconds": 60,
  "persisted": true
}
```

`reason` is empty exactly when the most recent completed probe succeeded, or
when none has completed; a `reachable` or `unknown` state can therefore carry a
non-empty reason left by a below-threshold failure. It is a member of the type,
not an absent field, so a typed client is not surprised by the success shape. `probing_enabled` is `false` when the effective
interval is `0`, distinguishing "not probed yet" from "probing is off" without
inferring it from an absent timestamp. `probe_interval_seconds` carries the
**effective** interval after clamping, because the client cannot otherwise
satisfy the three-times-the-interval staleness rule or set its own refresh
cadence; without it both silently become hardcoded guesses that drift from the
operator's configuration.

Status codes on the probe route: `404` when no such executor exists, `400` when
it is not of type `ssh` (both matching the existing `resolveSSHTarget` mapping
in `internal/ssh/handlers.go`), and `409` when it is `ssh` but its `status` is
not `active`. Probing a deactivated executor on request would contradict the
rule that its retained record stays unchanged, so the route refuses rather than
inventing a third behavior. `GET` on an executor with no record returns the
`unknown` shape with null timestamps, not `404`.

The immediate-probe route coalesces: a per-executor
`golang.org/x/sync/singleflight` group keyed by executor id makes overlapping
requests share one probe and one result. **The shared probe does not run on any
caller's request context.** It runs on a background context carrying only the
probe deadline, so a caller that disconnects — a closed tab, a cancelled fetch —
neither cancels the probe nor fails the other caller waiting on it, and the
result is persisted either way. A caller whose request already ended receives
nothing. A probe that ran but was not stored — the write failed, or a guard
refused it — still answers `200` with the outcome and `persisted: false`: the
reading is true even when its durability is not, and the client cannot act on the
difference. Only a failure to run the probe at all is an error status.

Change propagation reuses the existing event bus to WebSocket bridge. A new
event `executor.reachability.changed` is published only when `state` or `reason`
differs from the stored record, subscribed in
`internal/gateway/websocket/task_notifications.go` alongside
`events.ExecutorUpdated`. Publishing per probe would put one message per executor
per interval on every client forever; publishing on change keeps a steady host
silent. The payload is the record above. The configuration reset uses the same
rule, so a state change has one propagation path.

Because a silent steady state means `checked_at` ages in the client, the
settings page refetches **its own single record** once per effective interval
while open. That is a per-view refresh by the one client looking, not a
broadcast, so a steady host still produces no per-interval traffic to any other
client.

## Frontend components

Everything here is `apps/web`. No component defines reachability; each renders
state the backend owns.

**Types and client.** The record type lands in `apps/web/lib/types/http-ssh.ts`
next to `SSHTestResult`; the three routes are wrapped in
`apps/web/lib/api/domains/ssh-api.ts`, which already wraps SSH settings.

**Store slice.** A `reachability` slice keyed by executor id, one record each.
It hydrates from `GET /api/v1/ssh/reachability` and applies
`executor.reachability.changed` events in place. Both inputs race — a refetch
issued before an event can answer after it — so the slice keeps whichever payload
carries the later `checked_at`, giving the client the same last-write-wins rule
the store uses.

**Settings page.** `apps/web/app/settings/executors/ssh/[executorId]/page.tsx`
gains `apps/web/components/settings/ssh-reachability-card.tsx` alongside the
existing `SSHConnectionCard` and `SSHSessionsCard`. It shows the state, probed
host, the reason and message when unreachable, the failure count, the age of the
last completed and last successful probe (or that none is recorded), the stale
marker, the probing-is-off notice, and the immediate-probe button. It owns the
once-per-effective-interval refetch above and is the only component to refetch.

**Launch surfaces.** The pre-launch warning renders where the launch is
initiated, naming the host and the last-success age. With no interactive user —
a dependency chain, a workflow transition, an autostart — there is no prompt, so
the warning is written to the launched session's own event stream, where the user
finds it on opening the card. The launch-failure attribution renders what the
backend reports; the frontend adds no text of its own.

**Cross-cutting.** All copy goes through `t()` with keys in all five locales;
reason tokens render through a translated label map, never raw. State is text as
well as color. Every surface is verified against `/mobile-parity`.

## Launch interaction

The launch path is unchanged in its decision-making: `CreateInstance` reads no
reachability record and nothing consults one to decide whether to proceed. A
probe is evidence about the recent past, and a host can recover before a
launch.

The launch path writes no reachability record either: the poller is the only
writer. Making a launch's own dial a probe observation is deferred, so a launch
that fails against a dead host updates no record, and the settings page can lag
that host by up to the failure threshold times the effective interval.

Two user-visible additions. The pre-launch warning names the host and the age of
the last successful probe, or says none has been recorded, and does not block.
It is suppressed entirely while probing is disabled: a retained record is not a
current measurement, and warning from one would train the user to distrust the
warning that matters.
The launch-failure message names the target host and the reason classified from
**that launch's own connection attempt**, not the stored record, which may
predate the attempt by a full interval. Kandev never intercepts or edits agent
output; it adds its own attribution alongside.
