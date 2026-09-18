---
status: current
system: agents
requirements:
  - REQ-AGENTS-SESSION-CEILING-001
  - REQ-AGENTS-SESSION-CEILING-002
---

# Session Concurrency Ceiling System Design

## Purpose and boundaries

The orchestrator owns one admission controller shared by manual starts,
resumes, workflow starts, queue drains, and dynamic relaunches. The controller
does not own task metadata or provider execution. It returns a reservation or a
typed refusal. The task repository owns the durable deferred-launch record.

This design defines the implemented opt-in behavior. The composition root
resolves a disabled default, saved Settings value, or explicit startup
environment override before automatic launch consumers start. The
[delivery plan](../../../plans/session-ceiling-opt-in/plan.md) records the
implementation and verification. The admission/replay contracts below remain
in force when enabled.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-AGENTS-SESSION-CEILING-001` | [Admission and replay](#admission-and-replay), [Failure and recovery](#failure-and-recovery) |
| `REQ-AGENTS-SESSION-CEILING-002` | [Settings contract](#settings-contract), [Settings surface](#settings-surface), [Persistence](#persistence) |

## Components and responsibilities

| Component | Responsibility |
| --- | --- |
| `sessionCeilingController` | Counts persisted `STARTING`/`RUNNING` sessions and process-local reservations. It admits, refuses, confirms, and releases launches. |
| Orchestrator launch seams | Pass the launch origin and complete replay payload to the controller. They consume a reservation only after launch success. |
| `deferCeilingRefusal` | Merges the ceiling-owned keys into task `deferred_launch` with compare-and-set semantics. |
| Ceiling sweep | Lists tasks with ceiling records, validates eligibility, and dispatches the stored launch kind. |
| Task repository and service | Store and update the shared deferred record. Prompt edits update both legacy top-level data and the nested ceiling payload. |
| Executor callbacks | Confirm or release only when the callback execution still owns the session row. |

## Data and contracts

The shared record uses `ceiling_deferred`, `ceiling_launch_kind`,
`ceiling_launch_payload`, `ceiling_launch_origin`, `ceiling_reason_code`, and
`ceiling_queued_at`. The payload is nested so replay does not confuse a resume,
prompt ensure, or dynamic relaunch with a task start. A different pending
payload returns `ErrCeilingLaunchConflict`; the existing record stays unchanged.

The composition root resolves capacity before starting the orchestrator or other
automatic launch consumers. A new typed `internal/system/sessioncapacity` service
uses the existing install-wide `internal/system/settings.Store`. It exposes the
settings API and updates the same orchestrator admission controller used by all
launch seams. No second limiter or runtime feature-flag entry is introduced.

### Settings contract

The store key is `session_capacity`. Its JSON value has `enabled: bool`
and `max_sessions: int`. The absent-row default is `{enabled: false,
max_sessions: 5}`. Five is an editable form suggestion, not an active default
ceiling. A saved maximum must be between 1 and 2147483647 inclusive. The bound
keeps the value portable across supported Go integer widths and JSON clients.
Disabled state retains the maximum. All writes validate the complete record.

Effective admission capacity is zero when disabled, otherwise `max_sessions`.
Precedence is valid explicit `KANDEV_MAX_CONCURRENT_SESSIONS` > saved setting >
disabled. Read the environment once at startup; zero is a valid locked override.
Blank, negative, malformed, or overflowing values are ignored with a warning.
They do not lock Settings or restore the old CPU-derived default. Accept the
same whitespace and integer syntax as the existing parser within the bound.

Add `GET` and `PATCH /api/v1/system/session-capacity/settings` through
`internal/system/system.go`. Follow `queuesettings.RegisterRoutes`: reads use
the existing system group and writes use its admin group. PATCH supports partial
updates; omitted fields retain their saved value. Reject null, wrong types,
invalid bounds, and malformed requests with 400. A valid environment override
rejects writes with 409. Existing auth middleware returns 403 for member writes.
Load/save errors return 500 and leave the controller unchanged.

Responses contain `settings: {enabled, max_sessions}` and
`effective: {enabled, max_sessions, source, locked}`. The effective maximum is
zero when disabled; `source` is `default`, `setting`, or `environment`. With an
environment override, the form renders effective values and the lock reason,
while retaining the saved values separately in the response. No pending-restart
state exists for Settings saves. Environment changes still require restart.

Use the queue-settings service as the local example for typed resolution,
partial updates, and save-before-apply. Reuse the raw settings store and its
consistent-read/CAS support rather than copying generic storage infrastructure.
Serialize settings writes and application to the live target. A successful
write is applied before responding. A validated target setter is infallible;
storage failure cannot change admission. Concurrent PATCHes preserve omitted
fields. GET uses the service's serialized resolution path.

Add a typed capacity value to the orchestrator's existing `ServiceConfig` and
pass the resolved value from `internal/backendapp/orchestrator.go`. The
constructor defaults to zero and no longer independently reads the environment.
`internal/backendapp/main.go` wires the settings service to that same
orchestrator through a narrow setter interface. Resolve persisted configuration
before any automatic launch worker starts, including after restart.

Update the `KANDEV_MAX_CONCURRENT_SESSIONS` inventory exclusion in
`internal/common/config/catalog.go`: it remains outside YAML, but is now an
environment override of a live install setting, not an environment-only ceiling.

### Settings surface

Place a Session capacity section after Message Queue in
`components/settings/task-behavior-settings.tsx`, at
`/settings/preferences/task-behavior`. Use `SettingsTarget` and register it in
`lib/settings-discovery/catalog/preferences.ts` so search and deep links work.
Use the target ID `setting-session-capacity`. Queue banners link to this target
under the [task-owned presentation contract](../../tasks/system-design/queued-session-ownership.md#limit-scope-and-configuration-navigation).
The section belongs to agent admission despite its Settings placement.

Add `system/session-capacity-settings.tsx`, a focused draft hook, API functions
in `lib/api/domains/settings-api.ts`, and DTOs in `lib/types/system.ts`.
Use `SettingsCard`, `SettingsSection`, `Switch`, `Input`, and the existing
`useSettingsSaveContributor` page-level Save changes/Reset behavior. The enabled
switch and maximum are one atomic contributor. Keep edits made during an
in-flight save, following the message-queue contributor's submitted-value guard.

Show the switch first, then a numeric maximum only when enabled, then current
effective state and concise scope/help text. Off means "No session limit".
Loading and load failure disable edits; load failure offers Retry. Save failure
preserves the draft and previous effective state. Invalid enabled maximum blocks
save with inline feedback. Turning the draft off can save despite an invalid
unsaved maximum, using the last valid saved maximum. Members and env-locked
installs see read-only effective values with an explanation.

Use the existing Message Queue card and
`e2e/tests/system/mobile-message-queue-settings.spec.ts` as the shipped mobile
exemplar. Mobile enters through home navigation > Settings > Task Behavior.
This small form stays inline in the Settings route; it needs no new drawer.
The existing `settings-scroll-container` remains the only scroll owner, and the
existing save bar owns dynamic viewport and safe-area behavior. Fields stack
at full available width on phones; desktop maximum input can use its existing
bounded width. Share draft state and mutations across viewports.

Use `settingsControlClassName` and `settingsActionClassName` for 28px desktop
controls and at least 44px phone/coarse-pointer targets. Give the switch a real
44px touch wrapper, as the queue auto-merge control does. Preserve focus, visible
labels, numeric mobile keyboard, and screen-reader error associations. Add all
copy to the five locale catalogs; generate the Traditional Chinese pair with
the existing script. The [plan previews](../../../plans/session-ceiling-opt-in/plan.md#ascii-ui-preview)
define structural composition and states.

## Admission and replay

The flow is:

```text
launch request
  -> orchestrator seam
  -> sessionCeilingController.admit
  -> reservation, or typed refusal
  -> deferCeilingRefusal(task CAS) when automatic refusal
  -> executor launch and callback
  -> confirm/release reservation
  -> ceiling sweep replays the stored kind when capacity is available
```

Manual origins bypass refusal and write a manual-override audit entry. Automatic
origins never become manual during replay. The sweep clears a record only after
successful dispatch. A repeated refusal leaves the original timestamp and
payload in place.

With effective capacity zero, `admit` returns an ordinary admission before the
population lookup can refuse it. Retain reservation ownership needed by launch
callbacks and by a later enable operation. Do not mark a manual override or
create a ceiling warning. This also covers a failed population lookup; disabled
must not accidentally fail closed. Observations may still report an unknown
population without blocking launches.

`Service.SetSessionCapacity` updates the existing controller under
its admission mutex. All capacity reads, including observations, use that mutex.
Do not replace the controller or clear its reservations when changing the value.
Already admitted launches remain admitted if the new limit is lower. New
automatic admissions see the new value after the setter returns.

After releasing the controller mutex, disabling or increasing capacity calls
`signalCeilingSweep`. Keep the sweeper running when capacity is zero, including
on startup, so existing durable deferrals can recover. Replay still checks task,
entry, workflow, and payload ownership; capacity changes do not clear records
directly. The 20-second sweep remains the recovery backstop. Existing historical
manual-override messages remain history; they are not an effective-limit badge.
Every applied capacity change also requests an observation/status refresh through
the existing projection path, including a decrease. Do not let an old count imply
that a disabled limit still blocks work while confirmed replay is pending.

The task-owned [queued session ownership design](../../tasks/system-design/queued-session-ownership.md)
plans explicit passive-inspection classification at the caller boundary. Opening
a conversation is not a manual override. Actual explicit execution retains this
design's manual admission rule; workflow parking eligibility remains task-owned.

## Failure and recovery

Reservation ownership is session-keyed. A stale process-start callback first
checks the persisted session execution identity; it cannot release a successor's
reservation. A dynamic relaunch reports three outcomes: succeeded, deferred, or
failed. A deferred detached relaunch leaves the automation run and its durable
record for the sweep. Queue dispatch treats a seam-3 refusal as retryable so the
original message remains in the queue.

Office automatic starts do not use workflow-step auto-start eligibility when the
sweep evaluates a `start` record. This keeps Office scheduling ownership in the
Office path.

## Persistence

`deferred_launch` is updated with a read-compare-write retry loop. The ceiling
keys share the task record with other launch intents and are removed as a group
after replay or a terminal drop. A prompt edit preserves unrelated keys and
updates the nested replay payload.

The install setting uses the existing `settings` table and SQL-dialect support;
no table migration or per-task backfill is required. An absent key means disabled
even on an upgrade from the CPU-derived default. Never persist that old derived
value during upgrade. An explicitly configured environment limit remains an
opt-in. Saved UI values survive removal of the environment override.

A settings storage read failure or corrupt persisted record must be surfaced
before automatic workers start, rather than silently discarding a saved limit.
At runtime, a read/write failure leaves the last applied setting in place and
returns a visible error. No background worker is added for settings refresh.

## Security

The controller receives launch data from trusted backend paths. The nested
payload is excluded from plugin host data. No prompt or environment value is
executed during replay; it is passed to the same existing launch seam after
task and session eligibility checks.

## Observability

Admission decisions, refusal reasons, reservation expiry, replay drops, and
manual overrides use the existing orchestrator logs and task status messages.
The stored reason and population snapshot make a refusal diagnosable after a
restart.

Log applied capacity and its source on initialization and successful changes.
Reuse existing observation and queue-status projection paths. Never log full
environment dumps or launch payloads for a settings change.

## Related decisions

[ADR 0018](../../../decisions/0018-runtime-settings-overrides.md) supplies the
existing environment precedence and install-admin convention. This operational
limit follows the live Message Queue settings pattern; it does not extend the
boolean release-toggle registry. A separate ADR is unnecessary for this local
settings extension.
