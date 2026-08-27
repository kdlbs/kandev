---
status: draft
system: agents
requirements:
  - REQ-AGENTS-PROFILE-RECENT-USE-001
  - REQ-AGENTS-PROFILE-RECENT-USE-002
  - REQ-AGENTS-PROFILE-RECENT-USE-003
---

# Agent Profile Recent Use System Design

## Purpose and boundaries

The agent system owns a portable, bounded order of recently used agent profile
IDs for operational selection contexts. Task and chat launchers report
successful use; the user service persists and synchronizes the projection; the
frontend applies it after existing eligibility filters. Workspace defaults,
task/session creation, and chat lifecycles remain owned by their current
systems.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-AGENTS-PROFILE-RECENT-USE-001` | [Frontend ordering](#frontend-ordering) |
| `REQ-AGENTS-PROFILE-RECENT-USE-002` | [Recording flow](#recording-flow) |
| `REQ-AGENTS-PROFILE-RECENT-USE-003` | [Persistence](#persistence), [Synchronization](#synchronization) |

## Components and responsibilities

- The user repository stores one bounded record per user and supported context.
- The user service validates context and profile ID, applies move-to-front
  semantics, retries revision conflicts, suppresses no-op writes, and publishes
  user-routed events.
- User HTTP handlers expose a focused read and record contract independent of
  the general settings PATCH.
- The boot-state builder loads all context rows once for an authenticated boot.
- The WebSocket user broadcaster routes the compact update to the writing
  user's connected clients.
- A separate frontend store field owns hydrated recent-use state and rejects
  stale per-context revisions.
- Shared option-ordering logic applies a context history after existing
  eligibility filtering and before the combobox prioritizes its selected value.
- Operational launchers report only the final successful profile for their
  declared context.

## Data and contracts

### Contexts

The backend and frontend use the closed set `task_create`, `task_session`,
`quick_chat`, and `config_chat`. Unknown values are rejected. Configuration
selectors do not receive a context.

### Persistence record

```sql
CREATE TABLE IF NOT EXISTS user_agent_profile_recent_use (
    user_id TEXT NOT NULL,
    context TEXT NOT NULL,
    profile_ids TEXT NOT NULL DEFAULT '[]',
    revision BIGINT NOT NULL DEFAULT 0,
    updated_at TIMESTAMP NOT NULL,
    PRIMARY KEY (user_id, context),
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);
```

`profile_ids` is an ordered JSON array containing at most ten distinct,
non-empty IDs. The composite primary key is sufficient for all reads and
writes; no secondary index is needed. With four contexts, each user has at most
four rows and forty retained IDs. The table is separate from `users.settings`,
so a recency update does not rewrite the settings JSON or increment its
revision.

### HTTP

- `GET /api/v1/user/agent-profile-recent-use` returns all supported context
  records for the authenticated user.
- `PUT /api/v1/user/agent-profile-recent-use/:context` accepts
  `{ "agent_profile_id": "..." }` and returns the resulting context record.

Wire records use `context`, `profile_ids`, `revision`, and `updated_at`. The
mutation body uses the existing request-size safeguards with a narrow cap and
rejects unknown contexts, empty IDs, or profile IDs longer than 255 bytes with
`400`.

### Boot state

Authenticated boot payloads contain a separate `agentProfileRecentUse` state
with records keyed by context and `loaded: true`. A read failure does not fail
the app boot; it leaves the field unloaded so the client can use source order
and recover through the focused GET endpoint.

### WebSocket

A successful changed mutation publishes
`user.agent_profile_recent_use.updated` with the changed context record. The
event includes the user ID only for server-side routing; the broadcaster strips
it before delivery. Reusing the already-first profile publishes nothing.

## Recording flow

1. A task, subtask, task-session, quick-chat, or configuration-chat launch
   completes successfully.
2. The caller resolves the effective profile ID from the backend response,
   falling back to the submitted ID only where the response omits it.
3. Supersession checks run before recording. A discarded quick-chat or
  configuration-chat launch exits without a recency mutation.
4. Deferred task-create launches carry an explicit, selector-backed attribution
   marker. HTTP and WebSocket task/subtask selectors set it; programmatic MCP
   profile input does not. The marker's creator identity remains in the
   server-owned portion of the deferred intent and is not client-editable or
   returned in task DTOs.
5. The frontend starts the focused mutation without awaiting it in the primary
  launch/navigation path.
6. The service reads the context row. If the requested ID is already first, it
  returns the current record with no database update or event.
7. Otherwise it removes any prior occurrence, prepends the ID, truncates the
  array to ten, and conditionally writes the next revision. A conflicting
  revision rereads and retries up to the existing bounded CAS attempt limit.
8. The response and compact event update frontend state only when their
  revision is newer than the stored revision.

Mutation failure is logged and leaves the last committed order intact. It does
not surface as a launch failure and does not delete successful work.

## Frontend ordering

The pure ordering helper builds a rank map for at most ten IDs and makes one
stable pass over the eligible profile list. Its complexity is `O(P + 10)` for
`P` eligible profiles and it does not mutate source arrays.

Operational consumers pass one context:

- standard task and subtask creation: `task_create`
- add agent, new session, and handoff: `task_session`
- quick chat: `quick_chat`
- configuration chat: `config_chat`

The existing eligibility filter runs first. Remembered eligible profiles are
ordered by history; unseen eligible profiles retain source order. The existing
combobox selected-first pass runs last. Search continues to operate on the same
eligible option set. Unknown or stale IDs never create placeholder options.

Selectors for settings, defaults, automations, and Office assignments omit a
context and retain source order.

This change is state/data normalization inside existing selector compositions.
Desktop and mobile reuse the same store, filtering, ordering, search, selection,
and launch handlers. It does not change overlay type, navigation, scroll owner,
safe-area behavior, touch targets, or responsive breakpoints. Existing mobile
combobox and quick-chat surfaces therefore remain the mobile exemplar, and
focused unit/component coverage satisfies the mobile-parity exception.

## Failure and recovery

- Missing recent-use state falls back to source order without blocking a
  selector.
- Invalid stored JSON fails that context read and is logged; other contexts and
  the app boot remain usable.
- A stale boot or WebSocket record cannot replace a newer per-context revision.
- Stale profile IDs are ignored during ordering and are removed naturally as
  later successful uses truncate the bounded list.
- A record request failure is best effort. The next successful record or boot
  restores backend-owned state; no browser retry queue becomes durable.

## Persistence

The user repository's base schema and replay-safe migrations both create the
new table for SQLite and PostgreSQL. User deletion cascades to context rows;
repository tests verify that behavior with foreign-key enforcement enabled.
There is no migration from browser storage or `users.settings` because neither
previously owned this history.

The row update uses revision-based compare-and-swap. The revision begins at one
for a created context and increases only when the ordered IDs change. The
timestamp is server-generated UTC metadata and does not determine ordering.

## Security

Handlers derive the user ID from the authenticated request context and never
accept a user ID in the URL or body. Reads, mutations, boot state, and events
are isolated to that user. The closed context set, narrow body cap, ten-ID
retention cap, and 255-byte profile-ID cap bound storage controlled by a client.

Deferred task-create attribution is a server-owned subfield of the deferred
launch intent. The task service strips client-provided deferred intents and
attribution during create, preserves the existing intent during generic task
metadata replacement, and only accepts the explicit attribution flag from the
HTTP/WebSocket selector handlers. Public task projections redact the creator
identity and internal marker. MCP may request a deferred profile launch, but it
does not carry selector attribution and therefore cannot reorder task-create
history.

## Observability

Repository, validation, and event publication failures use the existing user
service logger with context but without emitting the full profile history.
No new metric is required for this bounded preference path. Focused tests cover
no-op suppression, CAS retry, caps, isolation, boot hydration, and event
routing.

## Related decisions

- [ADR 0041: Backend-Owned Portable User Settings](../../../decisions/0041-backend-owned-portable-user-settings.md)
- [ADR 0028: Backend-Owned Task-Create Last-Used Preferences](../../../decisions/0028-task-create-last-used-source-of-truth.md)
- [ADR 2026-08-27: Store Agent Profile Recency in Bounded Context Rows](../../../decisions/2026-08-27-bounded-agent-profile-recency.md)
- [ADR 2026-08-27: Protect Deferred Launch Attribution](../../../decisions/2026-08-27-protect-deferred-launch-attribution.md)
