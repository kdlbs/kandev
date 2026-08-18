# Session-bound recurring wakes

## What

Kanban-profile agents can list, upsert, and delete recurring wake schedules
for only their current task session. A schedule has a stable marker, exact
prompt, five-field cron/descriptor/@every expression, IANA timezone, and
future RFC3339 expiry. Expired rows remain listable and upserting the same
marker reactivates its stable identity.

## Why

Coordinators need scheduled follow-up without creating duplicate Kanban tasks
or relying on an external cron host.

## Behaviour

The backend delivers the configured prompt to the same session. Idle sessions
resume through the normal queue path; running or starting sessions receive one
coalesced server message and are not interrupted. A delayed tick fires at most
once and advances to the next future occurrence. Terminal targets record an
error; transient failures remain inspectable for later retry.

## Boundary

The feature does not change Automations or Office routines, and cannot create
tasks or target another task/session. See ADR-2026-08-17-session-bound-recurring-wakes.
