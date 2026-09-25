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
  "updated_at": "RFC3339 | null",
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

Status codes are the same on all three routes: `404` when no such executor
exists or it is soft-deleted, and `400` when the id names an executor that is
not of type `ssh`. The probe route adds `409` when the executor is `ssh` but its
`status` is not `active`; probing a deactivated executor on request would
contradict the rule that its retained record stays unchanged, so the route
refuses rather than inventing a third behavior.

**Do not reuse `resolveSSHTarget`'s mapping wholesale.** That helper
(`internal/ssh/handlers.go`) answers `400` both for a wrong executor type *and*
for a failure to project the executor's config into a target — and a
configuration that will not resolve, or that carries no pinned fingerprint, is
exactly the case `AC-EXECUTORS-SSH-REACHABILITY-002.5` says must return an
outcome. A probe route built on it would answer `400` for precisely the broken
executors whose `config` reading matters most. Only the wrong-type branch maps
to `400` here; an unresolvable configuration is a `200` carrying reason
`config`.

`GET` on an eligible executor with no record returns the `unknown` shape with
null timestamps, not `404`. `updated_at` is null there too: the shape is
synthesized, no row was written, and there is no version to report. A client
treats a null `updated_at` as older than any real record, so a synthesized
placeholder can never displace a record that actually exists. That covers an executor that exists and has simply
not been probed yet; a nonexistent, soft-deleted or non-`ssh` id takes the
status above, so a wrong id cannot be rendered to the user as "reachability
unknown".

`GET /api/v1/ssh/reachability` returns one entry for every SSH executor that is
not soft-deleted, **ordered by `executor_id` ascending** — the same total order
the pass uses, and the slice keys by id anyway, so no tiebreak is needed.
Executors whose `status` is not `active` are included, carrying their retained
record, because `AC-EXECUTORS-SSH-REACHABILITY-001.17` preserves it and the
settings page for a deactivated executor still renders it. An executor never
probed appears in the `unknown` shape rather than being omitted, so the client
never has to distinguish "absent from the list" from "has no record".

The immediate-probe route coalesces: a per-executor
`golang.org/x/sync/singleflight` group keyed by executor id makes overlapping
requests share one probe and one result. **The shared probe does not run on any
caller's request context.** It runs on a context derived from the reachability
package's own, carrying the probe deadline, so a caller that disconnects — a
closed tab, a cancelled fetch — neither cancels the probe nor fails the other
caller waiting on it, and the result is persisted either way. It is **not**
`context.Background()`: the probe is registered on the package `WaitGroup` and
cancelled by `Stop` like every other probe, which is what the engine design's
drain contract and its `goleak` guard require. A caller whose request already ended receives
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

When `probing_enabled` is `false` the effective interval is `0` and there is no
cadence to follow: the page fetches once on open and again after an immediate
probe, and runs no timer at all. Reading the interval literally in that state
would be a zero-delay refetch loop against the one record known not to be
changing.

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
carries the later **`updated_at`**.

`updated_at`, not `checked_at`, is the reconciliation key, and the distinction is
load-bearing. The server orders *observations* by `checked_at`, which is when a
probe completed. A client orders *record versions*, and one record version has
no `checked_at` at all: the configuration reset clears it. Reconciling on
`checked_at` would make a reset — whose payload carries `checked_at: null` —
compare as older than every result it supersedes, so the client would discard it
and keep showing `reachable` or `unreachable` for an executor the user had just
re-pointed, in exactly the window this capability exists to illuminate.
`updated_at` advances on every write, the reset included, so it orders all of
them.

**Settings page.** `apps/web/app/settings/executors/ssh/[executorId]/page.tsx`
gains `apps/web/components/settings/ssh-reachability-card.tsx` alongside the
existing `SSHConnectionCard` and `SSHSessionsCard`. It shows the state, probed
host, the reason and message when unreachable, the failure count, the age of the
last completed and last successful probe (or that none is recorded), the stale
marker, the probing-is-off notice, and the immediate-probe button. It owns the
once-per-effective-interval refetch above and is the only component to refetch.

**Launch surfaces.** The backend always emits `session.launch.warning` for the
launched session; the session view renders it there, and a client that
initiated the launch itself also renders it inline at the initiation point,
naming the host and the last-success age. The gateway caches the latest warning
for each session and replays it to a later subscriber until the session reaches
`RUNNING` or is removed. The launch-failure attribution renders what the
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

**Where the warning is produced.** In
`internal/agent/runtime/lifecycle/manager_launch.go`, the shared launch path
starts a bounded, best-effort read immediately before its single
`rt.CreateInstance` call. The read runs asynchronously, so a slow repository
cannot delay the launch. `CreateInstance` remains free of any record read, and a
read failure produces no warning rather than blocking or retrying.

**One mechanism, not two.** Rather than branch on whether a human is watching,
the warning is *always* published as a `session.launch.warning` event carrying
`executor_id`, `host`, `state`, `reason`, `last_success_at` (null when none has
been recorded), and a timestamp. A client that initiated that launch
interactively renders the same event inline as well. The gateway keeps the
latest warning per session and replays it to a later subscriber while the
backend is running. Every other path — a dependency chain, a workflow
transition, or an autostart — uses the same event path. One producer and one
payload keep all launch paths aligned.

**When it is raised.** Only when the record's state is `unreachable`. While
periodic probing is enabled, any `unreachable` record qualifies. While probing
is disabled, only a record whose `checked_at` falls within three times the
*default* interval qualifies, per `AC-EXECUTORS-SSH-REACHABILITY-003.2`. An
operator who switched the poller off and then ran an immediate probe seconds ago
holds a current measurement and should be warned from it —
`AC-EXECUTORS-SSH-REACHABILITY-001.28` keeps that action working precisely so
that reading exists. A record retained from before the poller was switched off
measures nothing, and warning from it would train the user to distrust the
warning that matters.

**The launch-failure message** names the target host and the reason classified
from **that launch's own connection attempt**, not the stored record, which may
predate the attempt by a full interval. Kandev never intercepts or edits agent
output; it adds its own attribution alongside.
