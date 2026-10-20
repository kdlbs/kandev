---
status: current
system: tasks
requirements:
  - REQ-TASKS-AGENT-ERROR-DISMISSAL-001
created: 2026-10-18
owners:
  - kandev
---

# Session Agent Error Dismissal System Design

## Purpose and boundaries

Tasks owns the durable `task_sessions.metadata.last_agent_error` acknowledgment.
This design corrects only its repository dismissal operation. The existing
[launch recovery design](task-launch-failure-recovery.md) owns launch actions;
[Git push dismissal](git-push-error-dismissal.md) owns a different message-level
acknowledgment. Platform retains the shared
[store portability contract](../../platform/system-design/postgres-domain-store-parity.md).
No shared persistence interface, schema, provider protocol, or UI changes.

## Requirement mapping

| Requirement / criteria | Design sections |
| --- | --- |
| REQ-TASKS-AGENT-ERROR-DISMISSAL-001; AC-TASKS-AGENT-ERROR-DISMISSAL-001.1 | Atomic metadata update, Validation boundary |
| AC-TASKS-AGENT-ERROR-DISMISSAL-001.2, AC-TASKS-AGENT-ERROR-DISMISSAL-001.3 | Message and occurrence guards |
| AC-TASKS-AGENT-ERROR-DISMISSAL-001.4, AC-TASKS-AGENT-ERROR-DISMISSAL-001.5 | Existing caller and presentation contracts, Failure behavior |

## Existing caller and presentation contracts

`TaskHandlers.httpDismissLastAgentError` serves
`POST /api/v1/task-sessions/:id/last-agent-error/dismiss` with the existing
optional `stamp` body and session DTO response. `Service.DismissLastAgentError`
loads the session, calls `authorizeTaskID`, loads `models.LastAgentError`, and
checks a nonempty request stamp with `MatchesStamp`. Missing error and stale
stamp remain successful no-ops. An empty stamp retains its existing meaning.

The service calls the repository with the observed error and dismissal time.
Only a changed row causes the existing `events.TaskSessionErrorChanged`
publication (`active: false`, existing task/session/stamp/dismissed-at fields)
and session reread. Publication failure remains logged and nonfatal; a
repository no-op retains the service's existing pre-read response. This work
does not fix that response during concurrent replacement.

`LastAgentErrorNotice` in
`apps/web/components/task/chat/message-list-shared.tsx` calls the existing API
client and hides only in its success callback. Its current-error stamp check,
store update, failure logging, localized labels, keyboard behavior, and desktop
and coarse-pointer layouts remain intact. Persistent readers continue to use
`models.LoadLastAgentError` and `LastAgentError.IsDismissed`; dismissal remains
an acknowledgment, separate from recovery evidence.

## Atomic metadata update

Keep `Repository.DismissLastAgentError` in
`apps/backend/internal/task/repository/sqlite/session.go`. Its payload remains
a copy of the supplied `models.LastAgentError` with `DismissedAt` set, serialized
through `json.Marshal`; it replaces the same modeled error object as today.
Unknown members inside that object are not newly guaranteed. Unrelated
top-level metadata survives. Successful writes retain the existing `updated_at`
behavior and `RowsAffected`-derived boolean; database errors propagate.

Use the active `r.db.DriverName()` and existing `metadataKeyUpdateQuery` and
`metadataKeyUpdateArgs` from `metadata_launch_error_cas.go` for the base update.
Those helpers already normalize SQL NULL, JSON null, and empty metadata for the
setter, bind a single key and JSON payload, and preserve other keys:

| Driver | Existing setter shape | Nested extraction |
| --- | --- | --- |
| SQLite | `json_set(..., ?, json(?))` | `dialect.JSONExtractPath` emits dotted `json_extract` |
| pgx | `jsonb_set(..., ARRAY[?]::text[], ?::jsonb, true)::text` | `dialect.JSONExtractPath` emits nested `->` / `->>` |

Append the dismissal predicates to that update. Bind the key as
`models.SessionMetaKeyLastAgentError`; do not interpolate the payload or caller
values. Rebind the complete statement once with `r.db.Rebind` after assembly.
Do not use `dialect.JSONExtract` with a dotted key: PostgreSQL interprets that
as one literal key. `dialect.JSONSet` embeds a string value and is not a setter
for this bound JSON object.

The existing update is a single conditional statement, not a read-modify-write
transaction. Keep the guards in the write so replacement rows cannot pass a
check performed earlier in Go. No extra transaction, advisory lock, retry loop,
or whole-metadata replacement is needed. Remove this method's obsolete exact
`sqlite-date-function` entry from `internal/db/sqlguard/exemptions.json`; the
portable operation must pass the existing guard without that exemption.

## Message and occurrence guards

Preserve the session ID and exact stored-message equality predicates. Extract
`last_agent_error.message` and `last_agent_error.occurred_at` with
`dialect.JSONExtractPath`. Continue binding the expected time as
`expected.OccurredAt.UTC().Format(time.RFC3339Nano)`.

- SQLite retains the existing disjunction: exact timestamp text equality, or
  `julianday(stored_time) = julianday(?)`. Do not replace this with `datetime`,
  which would lose fractional-second discrimination.
- PostgreSQL uses exact timestamp text equality, or equality between the
  extracted explicitly zoned time cast to `timestamptz` and the bound expected
  time cast to `timestamptz`. Native PostgreSQL subsecond precision applies; do
  not truncate to whole seconds, add a tolerance, or cast through a naive UTC
  column helper. The JSON value carries its own explicit zone.

For example, `.310Z`, `.31Z`, and `.310000+00:00` match; an explicit nonzero
offset for the same instant matches; `.311Z` must not match `.310Z` merely
because both fall in the same second. SQLite's existing date-function precision
is preserved; this correction does not establish nanosecond equality across
engines. Missing paths and SQL/JSON null do not satisfy the predicates.

Do not replace these guards with stamp-only matching or add stamp equality to
the repository predicate. The service's stamp check and the repository's
message/time identity remain at their existing boundaries. This does not claim
to close a separate same-message/same-time but changed-stamp race.

## Failure behavior

Keep malformed persisted metadata outside this repair. A cast or database
failure must propagate without reporting a successful acknowledgment; no
fallback may hide the error. The normal service model decoder already rejects
invalid occurrence times. Do not add broad repair, parser compatibility, or
new public error handling to accommodate malformed rows.

## Validation boundary

Add independently authored repository behavior tests for both engines, using
real metadata writers, `DismissLastAgentError`, `GetTaskSession`, and
`models.LoadLastAgentError` / `IsDismissed`. Test SQL results and retained data,
not generated query text alone. Cross-dialect cases distinguish message-only
and time-only replacements, equivalent fractions and offsets, missing targets,
and retained rich modeled diagnostics and unrelated metadata. A stale no-op
must preserve `updated_at` as well as the error. The SQL-NULL metadata case
uses a nullable raw-row snapshot: the existing session reader cannot scan that
historical shape, and changing that reader is outside this correction. Existing
service tests protect
authorization, stamp handling, and success integration; their source boundaries
are unchanged.

Use `testutil.PostgresDSNFromEnv` and `testutil.OpenIsolatedPostgres` for a unique
schema on every PostgreSQL fixture connection. Tests remain serial and own no
foreign schemas. The [single work order](../../../plans/postgres-agent-error-dismissal/task-01-port-dismissal.md)
defines fixture ownership, exact RED/GREEN and package commands, and cleanup.
The [plan](../../../plans/postgres-agent-error-dismissal/plan.md) records coverage
and qualification provenance. No new runtime metric or event is introduced.
