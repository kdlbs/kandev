---
status: draft
system: platform
requirements:
  - REQ-PLATFORM-DIAGNOSTIC-LOGGING-001
created: 2026-07-30
owners:
  - tbd
---
# Diagnostic logging System Design Part 2

## Purpose and boundaries

This design preserves the technical source detail for `REQ-PLATFORM-DIAGNOSTIC-LOGGING-001` during migration.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-PLATFORM-DIAGNOSTIC-LOGGING-001` | [Migrated source detail](#migrated-source-detail) |

## Migrated source detail

## API surface

### `POST /api/v1/system/logs/frontend-errors`

The authenticated browser submits:

```json
{
  "client_timestamp": "2026-07-30T12:34:56.789Z",
  "source": "sonner",
  "task_id": "d7e9e92e-eeb3-41e3-86d9-b6eca5760ac1",
  "title": "Failed to save settings",
  "description": "The backend rejected the update",
  "url": "http://localhost:38429/settings/system/logs?tab=current#tail",
  "user_agent": "Mozilla/5.0 ...",
  "language": "en-US",
  "platform": "Linux x86_64",
  "viewport": {
    "width": 1440,
    "height": 900
  },
  "stack": "Error: error toast emitted\n    at ...",
  "error": {
    "name": "TypeError",
    "message": "Failed to fetch",
    "stack": "TypeError: Failed to fetch\n    at ..."
  }
}
```

- `source` is `sonner` or `toast-provider`.
- `title` and `description` are optional individually, but at least one must
  contain visible text.
- `client_timestamp` is RFC 3339 when supplied. The backend log timestamp
  remains authoritative.
- `task_id` is optional and capped at 128 bytes. The frontend derives it only
  from recognized `/t/:taskId`, `/tasks/:taskId`, `/office/tasks/:taskId`, or
  `taskId` query-parameter routes; other pages omit it.
- `url`, `user_agent`, `language`, `platform`, `viewport`, `stack`, and
  `error` are best-effort client observations and may be absent.
- The request body is capped at 64 KiB. Before logging, the backend
  UTF-8-safely truncates title, description, error message, and URL to 8 KiB
  each; task ID to 128 bytes; user agent and platform/language fields to 2 KiB
  each; and stacks to 16 KiB each. It adds `truncated: true` when any value was
  shortened.
- A valid report returns `204 No Content`.
- Invalid JSON, an unsupported source, or a report without visible text
  returns `400 Bad Request`. A body over 64 KiB returns
  `413 Request Entity Too Large`.
- An exhausted identity or process-wide report bucket returns
  `429 Too Many Requests` with `Retry-After`.

The endpoint logs one structured `error` entry. Client fields remain fields;
they cannot replace the fixed `frontend error toast` message.

### Diagnostic bundle jobs

`POST /api/v1/system/logs/bundles` accepts:

```json
{
  "sources": ["backend", "frontend", "runtime", "acp"],
  "session_ids": ["session-uuid-1", "session-uuid-2"]
}
```

- `sources` is a non-empty unique subset of `backend`, `frontend`, `runtime`,
  and `acp`. `session_ids` is rejected unless `acp` is selected. Selecting
  `acp` requires one to ten unique session IDs.
- The backend resolves each ACP session to an authorized task and current or
  retained debug source. A non-admin may select only sessions on tasks they
  own; an admin may select any eligible session. Unknown, foreign, or duplicate
  IDs fail the request without revealing whether a foreign session exists.
- `acp` is rejected unless backend-authoritative ACP debug capture is enabled.
  A frontend runtime debug flag alone cannot enable or authorize ACP export.
- A valid request returns `202 Accepted` with `id`, `status`, `reused`,
  `build_deadline`, and nullable `expires_at`. `expires_at` is populated when
  the job becomes ready/partial. An equivalent non-expired job for the same
  identity, source set, and normalized ACP session set returns that job with
  `reused: true`.
- Backend-only jobs can become ready immediately. Jobs containing `frontend`
  collect browser evidence for at most 15 seconds. Jobs containing `acp`
  collect selected executor evidence for at most 30 seconds; frontend and ACP
  collection may overlap under the global job bounds.
- An invalid or empty source set returns `400 Bad Request`.
- A different request while that identity owns an active job returns
  `429 Too Many Requests`; process-wide job saturation returns
  `503 Service Unavailable`. Both include `Retry-After: 5`.

`GET /api/v1/system/logs/capabilities` returns backend-authoritative bundle
capabilities for the current identity:

```json
{
  "sources": ["backend", "frontend", "runtime"],
  "acp_debug_enabled": false,
  "acp_max_sessions": 10
}
```

- `acp` appears in `sources` whenever ACP debug capture is enabled. Candidate
  count may be zero; the UI still shows the debug action and its empty state.
- This endpoint reveals capability only. It does not return paths, frames,
  another user's session IDs, or the value of environment variables.

`GET /api/v1/system/logs/acp-sessions` returns at most 500 eligible sessions,
newest first, and is available only while ACP debug capture is enabled:

```json
{
  "sessions": [
    {
      "task_id": "task-uuid",
      "task_title": "Investigate browser disconnect",
      "session_id": "session-uuid",
      "agent": "claude-acp",
      "provider": "anthropic",
      "model": "sonnet",
      "status": "running",
      "executor_type": "local_docker",
      "last_activity_at": "2026-08-02T12:00:00Z",
      "acp_availability": "reachable"
    }
  ]
}
```

- `acp_availability` is `host_retained`, `reachable`, or `unavailable`.
  Unavailable sessions remain selectable only when a retained host file might
  still be discovered during collection; otherwise the control disables them
  with an explanation.
- Non-admin responses contain only caller-owned sessions. Admin responses may
  contain all eligible sessions. The task title is returned only for the
  authorized ACP-picker row and is not copied into an archive. Descriptions,
  messages, prompts, tool payloads, file content, user identity, and log paths
  are never returned.

Reachable executor-side `agentctl` instances expose an internal
`GET /api/v1/debug/acp/:session/export?max_bytes=N` route only while
`KANDEV_DEBUG_AGENT_MESSAGES=true`:

- The response is a bounded ZIP containing only recognized retained raw and
  normalized ACP JSONL files for that exact session. `N` is clamped to the
  backend's remaining ACP budget.
- The route never accepts a filesystem path or task/user identity and never
  returns unrelated session files. Unknown sessions return `404`; disabled
  debug capture returns `404`; unreadable or expired evidence returns `410`.
- The lifecycle/runtime client is the only bundle consumer. Browser clients
  never call executor-side `agentctl` directly, and the backend revalidates ZIP
  entry names, byte bounds, and selected-session ownership before inclusion.

When frontend collection starts, the backend sends authenticated WebSocket
notification `system.logs.capture_requested` to every connected frontend for
the requesting identity:

```json
{
  "bundle_id": "01J...",
  "capture_deadline": "2026-07-30T12:35:15Z",
  "max_chunk_bytes": 1048576,
  "max_browser_profiles": 4
}
```

Each frontend snapshots its local three-day store and uploads sequential chunks
to `POST /api/v1/system/logs/bundles/:id/frontend`:

```json
{
  "browser_id": "opaque-browser-id",
  "capture_stream_id": "opaque-per-tab-response-id",
  "chunk_index": 0,
  "done": false,
  "storage_mode": "indexeddb",
  "capture_metadata": null,
  "entries": []
}
```

- Each request body is capped at 1 MiB; each entry remains capped at 64 KiB.
- The first accepted chunk atomically binds a browser ID to its
  `capture_stream_id` for that job. Chunks must be sequential within that
  stream. Chunks from other tabs using the same browser ID are acknowledged
  without parsing or appending their entries, so concurrent tabs cannot produce
  a mixed snapshot.
- A caller may upload only to its own unexpired bundle job.
- The backend accepts at most 10,000 entries and 20 MiB per browser profile.
- The backend accepts at most four profiles and 80 MiB of frontend data for the
  whole job. Later profiles receive `429` and are recorded as omitted.
- The final `done` chunk includes at most 8 KiB of server-validated
  `capture_metadata`: dropped counts by level/reason, persistence failures,
  entry truncations, and storage mode. Earlier chunks omit it.
- Accepted chunks return `204 No Content`; invalid ordering or bounds return
  `400` or `413`; unknown/expired jobs return `404` or `410`.

`GET /api/v1/system/logs/bundles/:id` returns the caller-owned job state:
`collecting`, `building`, `ready`, `partial`, `failed`, or `expired`, plus
source/capture counts, selected ACP session counts and aggregate availability,
warnings, expiry, and a download URL when available.

`GET /api/v1/system/logs/bundles/:id/download` returns
`application/zip` for `ready` and `partial` jobs. It returns `409 Conflict`
while work is pending, `404` for unknown jobs, and `410 Gone` after expiry.

`POST /api/v1/system/improve-kandev/bundle/lease` accepts a caller-owned
ready/partial `bundle_id` plus the caller's current Improve Kandev
`bundle_dir`. It verifies both artifacts belong to the authenticated identity,
copies the ZIP as `diagnostic-bundle.zip`, and returns that owner-only path.
Pending, foreign, expired, or invalid artifacts are rejected without copying.
Bootstrap directories carry a server-written owner marker outside the ZIP; a
path prefix or caller-supplied directory alone never proves ownership.

The existing log list, tail, individual-file download, and dev debug-export
routes are removed.

## Permissions

All diagnostic routes use the existing authenticated install-user boundary. Any
signed-in user may create and download a bundle, matching the current System
Logs access rule. A user can inspect, upload to, or download only bundle jobs
created under the same identity. With authentication disabled, the synthetic
single-user identity keeps local behavior unchanged.

Raw ACP selection adds a stricter task-ownership boundary. A non-admin may list
and include ACP evidence only for sessions whose task owner matches the
authenticated identity. An admin may list and include any eligible session.
Authentication-disabled local installs use the synthetic admin identity. The
backend authorizes every selected session before contacting host or executor
storage; possessing or guessing a session ID does not grant access.

Frontend capture notifications and uploads never cross identities. Bundle
ownership is derived from the authenticated request, not a client-supplied user
or browser ID.

Task-mode MCP bundle requests derive identity from the immutable current
execution task through the existing MCP identity scoper. The tool does not
accept a caller-supplied identity, task/session pair, destination, or reusable
credential.

Backend and bundle files use owner-only permissions (`0600`) where Unix
permission bits are supported. Diagnostic archives may contain install-wide
backend activity, full URLs, console arguments, stacks, paths, browser
metadata, and user-visible backend error text. The page must present that
disclosure before the download action.

ACP-inclusive archives carry a distinct high-sensitivity disclosure and may
contain prompts, responses, tool arguments/results, file content, MCP payloads,
environment-derived values, and secrets. Standard bundles never read raw ACP
files or persisted message/transcript tables. The runtime index is authorized
with the same task/session rules and excludes message bodies and user identity.

## Failure modes

- If the log directory cannot be created or the active file cannot be opened,
  backend startup prints the intended path and a warning to stderr, continues
  with stdout/stderr logging, and retries the file sink every 30 seconds.
  Retention-cleanup failures are warnings and do not prevent startup.
- If size or day rotation fails, the backend preserves the existing files and
  does not exceed the total retention budget. It reports the error to stderr
  and retries the file sink after 30 seconds.
- If browser persistence fails, capture continues in the bounded memory
  fallback and the bundle manifest marks the degraded storage mode.
- If no eligible frontend is connected, a frontend-only bundle becomes
  `partial` with no frontend file; an `all` bundle still includes backend files.
- If some frontend connections time out, disconnect, reject the request, or
  exceed bounds, successful distinct-browser captures remain and the manifest
  lists aggregate omissions without exposing another user's identity.
- If ACP debug capture is disabled, an ACP request fails validation before a
  job is created. If a selected host file or executor becomes unavailable after
  authorization, other selected sources continue and the bundle becomes
  `partial` with a per-session availability warning in the manifest.
- If executor ACP export times out, exceeds its clamp, returns invalid ZIP
  paths, or cannot be revalidated, that session is omitted. The backend does
  not retry continuously, does not copy other sessions, and does not fail a
  valid backend/frontend/runtime source.
- If the runtime index query fails, other selected sources continue and the
  bundle becomes `partial`; it never falls back to a broader unscoped query.
- If ZIP construction fails, the job becomes `failed`, temporary partial files
  are removed, and the UI surfaces a normal error toast.
- If a backend or browser diagnostic queue is saturated, lower-priority entries
  may be omitted and the manifest records the loss; the product path that
  produced the log continues without waiting.
- If the bundle worker is busy, the request receives a busy response or reuses
  the caller's equivalent active job. It does not create unbounded concurrent
  archive or capture work.
- If the browser report endpoint receives a network, authentication,
  validation, or server failure, the original error toast remains unchanged
  and the reporting promise is discarded without retry.
- Unsupported or non-text toast content is omitted from the corresponding text
  field; reporting proceeds only when visible text can be extracted.
- Daily rollover uses UTC calendar boundaries and never uses client timestamps
  to select a backend file.

## Persistence guarantees

- Backend segments preserve the newest accepted entries across retained UTC
  days, up to the fixed 256 MiB total budget. High volume shortens the available
  time window instead of removing later evidence.
- Three UTC days is the maximum backend-log age. It is not a guaranteed
  allocation for each day. Files outside that window are removed at startup
  and after successful rotation.
- Browser console history remains in the browser profile for three UTC days,
  subject to its entry/byte caps. It is not server-persistent before capture.
- Raw ACP files retain their existing debug-only executor/host retention: at
  most 48 hours by default, bounded by per-file rotation and total file count.
  Bundle creation reads selected files on demand and does not create permanent
  backend ACP storage. Executor teardown may make evidence unavailable before
  the retention age.
- The runtime index is generated per bundle from authorized current database
  state, capped to the diagnostic window/row limit, and is not persisted as a
  separate product record.
- A frontend bundle can contain only histories returned by browser profiles
  connected during its 15-second collection window.
- Bundle jobs and ZIPs survive frontend navigation. Collecting/building jobs
  are cancelled after five minutes; ready/partial archives expire 15 minutes
  after becoming downloadable. They are not durable product data.
- Improve Kandev's task-context ZIP is a separate internal lease on the shared
  archive builder and remains available for up to 24 hours.
- No toast report is stored in SQLite or queued in browser storage for retry.
- Concurrent backend processes sharing one Kandev home are unsupported.

## Scenarios

- **GIVEN** no explicit logging override, **WHEN** Kandev starts normally,
  **THEN** stdout prints the absolute backend log path, the file contains
  `info+`, and stdout contains only `warn+` backend entries.
- **GIVEN** Kandev starts with debug logging, **WHEN** it emits debug through
  error entries, **THEN** all are in the file while only warn/error are on
  stdout.
- **GIVEN** a same-UTC-day restart, **WHEN** the backend opens its log,
  **THEN** it appends to the active segment and does not replace closed
  segments.
- **GIVEN** logging exceeds 256 MiB, restarts after rotation, or upgrades from
  an oversized legacy file, **WHEN** the backend resumes, **THEN** it continues
  with newest bounded content and the next sequence.
- **GIVEN** the configured log directory is temporarily unwritable, **WHEN**
  Kandev starts, **THEN** product startup succeeds with a stderr warning and
  the backend retries file activation every 30 seconds.
- **GIVEN** more than three UTC days of backend files, **WHEN** cleanup runs,
  **THEN** only the current and two preceding days remain.
- **GIVEN** the gateway receives close code `1000` or an unexpected close code,
  **WHEN** the read pump handles it, **THEN** only the unexpected close creates
  a `WebSocket read error` entry with a stack trace.
- **GIVEN** an error toast on a recognized task route, **WHEN** its report is
  accepted, **THEN** one backend error entry includes its visible text,
  browser context, and `task_id`.
- **GIVEN** console activity over three days, **WHEN** browser retention runs,
  **THEN** expired and oldest-over-cap entries are removed without uploading
  retained entries.
- **GIVEN** IndexedDB is slow or unavailable, **WHEN** an application emits a
  console entry, **THEN** the original console behavior returns without waiting
  for persistence and bounded staging/fallback preserves best-effort evidence.
- **GIVEN** a backend file sink is slow or blocked, **WHEN** application code
  emits logs, **THEN** logging does not block that code; lower-priority entries
  are shed first and the eventual bundle reports any loss.
- **GIVEN** stdout is blocked while the file sink is writable, **WHEN** backend
  logs are emitted, **THEN** the stdout queue may shed entries without stalling
  or dropping entries accepted by the independent file queue.
- **GIVEN** two users sign into Kandev from the same browser profile, **WHEN**
  the second user requests a bundle, **THEN** its frontend files contain only
  entries captured under the second user's identity partition.
- **GIVEN** a signed-in user opens System Logs, **WHEN** the page renders,
  **THEN** it names the frontend/backend event classes, states that the standard
  bundle does not read stored session or agent messages, warns about incidental
  emitted log content, and exposes one Customize bundle action without a tail
  or file table.
- **GIVEN** ACP debug capture is disabled, **WHEN** a user opens System Logs,
  **THEN** no ACP download action or ACP custom source is offered and a crafted
  ACP bundle request is rejected.
- **GIVEN** ACP debug capture is enabled, **WHEN** a user selects ACP debug
  messages in **Customize bundle**, **THEN** the session picker opens with the
  high-sensitivity disclosure and collection cannot start until at least one
  authorized session is selected.
- **GIVEN** the ACP picker lists an authorized session, **WHEN** a user reviews
  it, **THEN** its task title links to the task in a new tab and select-all
  includes no more than the allowed number of eligible sessions.
- **GIVEN** a non-admin can observe another user's session ID, **WHEN** they
  list ACP candidates or submit that ID directly, **THEN** the session is not
  disclosed or included and the response does not reveal its existence.
- **GIVEN** an admin selects two reachable sessions and one stopped unavailable
  executor session, **WHEN** the debug bundle finishes, **THEN** raw and
  normalized files for the reachable sessions are included, the unavailable
  session is omitted, and the bundle is downloadable as `partial` with an
  explicit manifest warning.
- **GIVEN** the runtime index is selected without ACP, **WHEN** a bundle is
  built, **THEN** it contains only authorized bounded session/runtime metadata
  and no task titles, prompts, responses, tool payloads, files, or message
  bodies.
- **GIVEN** two tabs from one browser profile answer a bundle request, **WHEN**
  their chunks interleave, **THEN** the first capture stream owns that browser
  ID and the ZIP contains one internally consistent frontend JSONL snapshot.
- **GIVEN** one browser answers and another times out, **WHEN** collection
  expires, **THEN** the combined ZIP is downloadable as `partial` with backend
  files, the successful frontend file, and a manifest warning.
- **GIVEN** retained backend files exceed 160 MiB, **WHEN** a bundle is built,
  **THEN** it includes newest byte ranges within the budget, becomes `partial`,
  and records each source offset and omitted byte count in the manifest.
- **GIVEN** an agent needs only task-scoped backend evidence, **WHEN** it
  calls `get_diagnostic_bundle_kandev` for `backend`, **THEN** the current task
  owner scopes the job, no frontend capture is triggered, and the tool returns
  an executor-local ZIP path the agent can extract and exact-grep.
- **GIVEN** an agent requests frontend evidence while no browser is connected,
  **WHEN** collection finishes, **THEN** it receives a partial ZIP whose
  manifest states that no frontend capture was available.
- **GIVEN** a phone user downloads diagnostics, **WHEN** collection completes,
  **THEN** the same disclosure, progress, partial-state warning, and archive
  download remain available without horizontal scrolling or desktop-only
  controls.
- **GIVEN** a phone user customizes a bundle, **WHEN** they select sources and
  ACP sessions, **THEN** an inset bottom drawer keeps disclosure and actions
  reachable with one internal scroll owner, safe-area clearance, ≥44px targets,
  and the same validation/result as desktop.

## Out of scope

- Continuous browser-log upload, hosted telemetry, or crash-reporting service.
- Continuous central upload or permanent backend retention of raw ACP frames.
- Including raw ACP in the standard bundle, Improve Kandev bundle, or
  task-mode `get_diagnostic_bundle_kandev` response.
- Exporting database contents, task titles/descriptions, chat transcripts,
  stored agent messages, workspace files, secrets/configuration, or environment
  dumps as standalone bundle sources.
- Preserving backend or frontend diagnostic history beyond three UTC days.
- A server-side free-text/regex log search or unbounded JSON log export.
- Viewing, copying, refreshing, or individually downloading log files from the
  System Logs page.
- Retrying failed error-toast reports.
- Changing toast copy, placement, duration, or styling.
- Adding a setting for log path, retention, browser capture, or bundle bounds.
- Supporting configurable size-based, per-process, or local-time backend
  rotation.
- Changing the browser reconnection policy or increasing the fixed 256 MiB
  total backend-log budget.
