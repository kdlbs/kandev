# 0047 — Plugins read conversation content via a capability-gated Host RPC

- Status: accepted
- Date: 2026-07-21
- Area: backend, protocol
- Related: [0043 — Plugin host data API](0043-plugin-host-data-api.md) (the pattern
  this extends), [docs/specs/plugins/spec.md](../specs/plugins/spec.md) ("Host data API")

## Context

The Host data API (ADR 0043) lets a plugin read kandev's domain data — tasks,
sessions, workspaces, workflows, agent profiles, repositories — over a
capability-gated `Host` gRPC channel. It deliberately stopped short of message
content: `Sessions()` exposes session *metadata* (identity, agent, timestamps,
`acp_session_id`, code stats) but no transcript.

A downstream "My Daily Standup" plugin needs to summarize yesterday's
user/agent conversations and surface issues worth mentioning. Today a plugin
**cannot** read historical message content at all:

- The only place message content is exposed is the `message.added` bus event
  (`internal/task/service/service_events.go` `publishMessageEvent`). Live event
  capture is a poor fit for "summarize yesterday" — a plugin would have to be
  running and recording continuously, and gets nothing for the window before it
  was installed.
- The message repository has no task-scoped or time-range query. `ListMessages`
  is session-scoped only; nothing filters by task id or a `created_at` window.

Reusing the ADR 0043 escape hatch (open the SQLite file directly) is the exact
failure ADR 0043 exists to close: schema coupling, breaks on Postgres, no
scoping, only possible because the subprocess is unsandboxed.

## Decision

Add one read RPC to the existing `service Host`, following ADR 0043 exactly.

1. **New RPC + DTO.** `ListMessages(ListMessagesRequest) returns
   (ListMessagesResponse)` on `service Host` in
   `apps/backend/proto/kandev/plugin/v1/plugin.proto`. `Message` DTO fields:
   `id`, `session_id`, `task_id`, `turn_id`, `author_type`, `content`, `type`,
   `created_at` (RFC3339). `MessageFilter`: `session_ids`, `task_ids`, `since`,
   `until` (RFC3339, `since` inclusive / `until` exclusive), `types`. Opaque
   cursor pagination (`Page`/`PageInfo`) like every other reader.

2. **SDK accessor.** `Host.Messages() MessageReader` with
   `List(ctx, MessageFilter, Page) ([]Message, *PageInfo, error)`, plus
   Go-native `Message`/`MessageFilter` mirrors in `pkg/pluginsdk/data_types.go`
   and an `UnimplementedHostData.Messages()` default. No proto types leak past
   the package boundary.

3. **Capability gate `api_read:messages`.** A new `resourceMessages` const and a
   `Messages()` accessor in `internal/plugins/host_data.go` that returns a real,
   service-backed reader when the manifest declares `api_read:messages`, else a
   denied reader whose `List` returns gRPC `PermissionDenied` with
   `capability 'api_read:messages' not declared` — identical to the other
   resources. A non-RFC3339 `since`/`until` returns gRPC `InvalidArgument`.

4. **Content is sanitized; system prompts never leave kandev.** The reader maps
   `models.Message` → DTO through `messageModelToDTO`, which runs
   `sysprompt.StripSystemContent` on `content` — the same `<kandev-system>`
   stripping the `message.added` event applies. Raw system content and the
   event path's `raw_content`/`has_hidden_prompts` are **not** exposed.
   `author_type` is only ever `user` or `agent`: there is no `system` author in
   kandev's model — system context is inline markup inside an otherwise
   user/agent message, and it is stripped at read time.

5. **Reads go through the service layer.** A new
   `Service.ListMessagesForPlugin(ctx, models.PluginMessageFilter)` on the task
   service delegates to a new `MessageRepository.ListMessagesForPlugin` /
   SQLite `ListMessagesForPlugin` — a single filtered query
   (`task_session_id IN`, `task_id IN`, `type IN`, `created_at >= since`,
   `created_at < until`), ordered `created_at ASC, id ASC`, with SQL
   `LIMIT`/`OFFSET`. The reader requests `page-limit+1` rows to derive
   `HasMore` without a second count query; `NextCursor` is `offset+limit`,
   matching the other readers' opaque-offset cursor. The `internal/plugins`
   package reaches this via a narrow `messageDataSource` interface wired by
   `Service.SetDataSources`, never importing the task service directly (the same
   import-cycle avoidance the existing readers use).

## Consequences

- A plugin can read a window of conversation content (e.g. "everything on this
  workspace's tasks since yesterday 00:00") without touching the DB file, and
  without the plugin having to be running when the messages were created.
- The `Message` proto is a public contract: fields are additive-only thereafter.
- The new query is the one net-new piece of data access; every other layer is
  the established ADR 0043 pattern (accessor + denied reader + narrow data
  source + DTO mapper + in-process/wire capability tests).
- Scoping stays ADR 0043 v1: reads are global to the instance, filters narrow
  results, and the single server-side data-source hook remains the place a
  future per-plugin/per-user restriction would attach.

## Alternatives considered

- **Subscribe to `message.added` and accumulate.** Rejected: only captures
  messages created while the plugin runs, needs durable plugin-side storage, and
  gives nothing for the pre-install / "yesterday" window the feature is about.
- **Expose the raw message row (including `raw_content`).** Rejected: leaks
  kandev's injected system prompts to plugins. The event path already strips
  them; the read path must match.
- **A session-only reader (reuse `ListMessages`).** Rejected: the standup use
  case is task/time scoped ("yesterday, across these tasks"), which the
  session-only query cannot express without an N+1 fan-out and client-side time
  filtering. One filtered query is simpler and cheaper.
