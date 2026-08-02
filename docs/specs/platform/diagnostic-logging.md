---
status: building
created: 2026-07-30
owner: tbd
---

# Diagnostic logging

Decision: [ADR-2026-07-30-file-backed-diagnostic-bundles](../../decisions/2026-07-30-file-backed-diagnostic-bundles.md)

Implementation plan: [diagnostic-logging](../../plans/diagnostic-logging/plan.md)

## Why

Users can see frontend failures that leave no backend evidence, and support
cannot rely on one known artifact containing recent backend and browser
diagnostics. The product needs a clearly disclosed bundle that users can
download and agents can inspect without continuously uploading browser console
history or returning an unbounded log export.

## What

### Backend files and terminal output

- Every backend process targets
  `<Kandev home>/logs/backend-logs.log`, where `<Kandev home>` is the resolved
  `KANDEV_HOME_DIR` or its `~/.kandev` default, and writes there whenever the
  path is available.
- Backend startup prints the absolute log-file path to stdout before the
  backend becomes ready.
- A normal run writes `info` and higher entries to the file and `warn` and
  higher entries to stdout.
- A debug run writes `debug` and higher entries to the file while stdout
  remains `warn` and higher.
- An explicitly verbose run writes `info` and higher entries to both the file
  and stdout.
- `logging.level` / `KANDEV_LOG_LEVEL` remains the supported override for the
  file threshold, and `logging.format` remains supported.
  `logging.outputPath`, `logging.maxSizeMb`, `logging.maxBackups`,
  `logging.maxAgeDays`, and `logging.compress` are removed; the diagnostic
  path and daily retention policy are not configurable.
- `backend-logs.log` represents the current UTC calendar day. At UTC midnight,
  the backend atomically rolls it to `backend-logs-YYYY-MM-DD.log` and creates
  a new owner-readable, owner-writable active file.
- A restart during the same UTC day appends to `backend-logs.log`. On the first
  startup after a day boundary, the previous active file is rolled to the date
  it represents before new entries are written.
- Cleanup retains a rolling three-day UTC window: the current active day and
  at most the two preceding dated daily files. Older recognized daily files
  are removed; unrelated files in the log directory are preserved.
- Each daily backend file accepts at most 256 MiB. Once the active file reaches
  that bound, entries that would exceed it are dropped without blocking
  application work and are reflected in file-sink loss statistics. UTC
  rollover resumes normal writes into the next day's file.
- Rollover is crash-recoverable. A small owner-only rollover journal records
  the source day, source identity/size, destination, and copied offset before
  an existing dated destination is extended. Recovery resumes that transaction
  exactly once and never replaces an existing dated file.
- The structured backend in-memory ring buffer is removed. The System Logs
  page, Improve Kandev, and agent diagnostics use the files and bundles
  described below.

### Error-toast reporting

- Every error toast displayed through either supported frontend toast system
  is reported immediately to the backend and written as an `error` entry named
  `frontend error toast`.
- Toast reporting includes the visible toast text plus rich browser context:
  the browser URL origin and pathname with query and fragment removed, browser
  identity fields, viewport dimensions, a client timestamp, the toast
  implementation source, the current task ID when it can be derived from a
  recognized task route or query parameter, a toast-emission call stack when
  available, and underlying error metadata when the caller still provides it.
- Toast reporting is best effort. A reporting failure does not suppress,
  replace, duplicate, or otherwise alter the original toast and does not
  create a retry toast.
- Frontend report bodies and individual fields are bounded before they reach
  the logger. The backend never treats a client-supplied value as a log level
  or log message template.
- The endpoint has an in-memory token bucket per authenticated identity
  allowing 60 reports per minute with a burst of 20, plus a process-wide
  bucket allowing 300 reports per minute with a burst of 100. Exhausted
  buckets return `429 Too Many Requests` with `Retry-After`; the frontend
  silently discards that response. Independent byte buckets admit at most
  64 KiB per minute per identity with a 64 KiB burst and 256 KiB per minute
  process-wide with a 256 KiB burst, measured from the bounded request body.

### Browser console history

- The frontend intercepts `console.debug`, `console.info`, `console.warn`, and
  `console.error`, plus `window.error` and `unhandledrejection`, without
  changing their normal browser behavior.
- Entries remain local to the browser until a diagnostic bundle explicitly
  requests them. Kandev does not continuously stream console logs to the
  backend.
- Browser history is stored in IndexedDB for a rolling three-day UTC window.
  Each browser profile has an opaque random installation ID shared by its tabs.
- Entries are partitioned by the current authenticated Kandev identity. A
  capture returns only that identity's partition; signing into another account
  in the same browser profile cannot expose the previous account's entries.
  Auth-disabled mode uses the existing synthetic single-user scope.
- The browser store keeps at most 10,000 entries and 20 MiB after
  serialization across all identity partitions. Each entry is capped at
  64 KiB. Age is pruned first, then the oldest entries are evicted until both
  bounds hold.
- Entries contain a client timestamp, level, source, message, bounded
  JSON-safe arguments, available stack, full URL, and a task ID derived from a
  recognized task route when available.
- Console-call staging never retains an arbitrary caller object. It preserves
  primitive values and bounded `Error` fields. Other objects, functions, DOM
  values, and proxies become reference-free type descriptors without walking
  prototypes or invoking getters. At most 20 arguments are inspected; strings
  are capped at 4 KiB, error messages at 8 KiB, and stacks at 16 KiB before the
  overall 64 KiB entry cap.
- If IndexedDB is unavailable or fails, the frontend falls back to the existing
  500-entry in-memory buffer. A resulting bundle identifies the degraded
  capture in its manifest.

### Performance and resource contract

- Diagnostic capture is best effort and is never on an application-critical
  path. Emitting a backend log, displaying a toast, or invoking a browser
  console method must not wait for filesystem I/O, IndexedDB I/O, a network
  request, ZIP creation, or another browser tab.
- File and stdout use independent bounded asynchronous queues so a blocked
  terminal cannot stall the authoritative file. The file queue accepts at most
  8,192 entries or 8 MiB and reserves 2,048 entries or 2 MiB for `warn+`. The
  stdout queue accepts at most 2,048 entries or 2 MiB and reserves 512 entries
  or 512 KiB for `warn+`. An encoded entry over 256 KiB is dropped. Producers
  never wait for sink I/O; `debug`/`info` is shed before reserved capacity is
  used, and even `warn+` is dropped rather than blocking when its queue is full.
  Each sink records atomic loss counters by level and reason.
- Graceful shutdown drains both queues for at most two seconds, then records
  any remaining entries as lost. `DPanic`, `Panic`, and `Fatal` are terminal
  exceptions: the logger attempts the same bounded drain and writes one capped
  final entry directly to stderr before the process terminates.
- Toast reporting begins after the original toast has been handed to its UI
  library and is never awaited. It performs bounded context collection, uses a
  two-second request deadline, and has no retry queue. "Immediately" means the
  report is scheduled in the same toast-emission turn, not that rendering waits
  for an HTTP round trip.
- Browser interception does only constant-time staging work on the console
  call path, where constant-time means bounded by the fixed argument and field
  limits above. Its reference-free staging queue holds at most 500 entries or
  2 MiB. It persists at most 50 entries or 256 KiB per transaction, scheduled
  after 250 ms or during browser idle time with a one-second timeout fallback.
  IndexedDB opening, serialization, pruning, and quota handling never run
  synchronously in the intercepted console call. A full staging queue drops
  lower-priority `debug`/`info` entries first and records loss metadata; it
  never delays the original console call.
- Bundle collection, JSONL writing, cleanup, and download preparation run
  outside the route handler. An identity owns at most one active job, at most
  eight jobs may collect or wait process-wide, and one archive build runs
  process-wide. An equivalent source request reuses the existing job. A
  different request from an identity with an active job returns `429`; global
  saturation returns `503`; both include `Retry-After: 5`.
- A job accepts at most four browser profiles, 20 MiB per profile and 80 MiB
  total frontend data. Without ACP, backend payload remains capped at 160 MiB
  and frontend payload at 80 MiB. When ACP is selected, the per-source budgets
  are 96 MiB backend, 48 MiB frontend, 96 MiB ACP, and 2 MiB runtime index.
  Omitted sources do not transfer their budget to another source. Backend and
  ACP files are selected newest-first; when a selected file exceeds its
  remaining source budget, only its newest bytes are included and the manifest
  records the source byte range. Total uncompressed archive payload remains
  capped at 256 MiB and a job may use at most 384 MiB of temporary disk.
  Creation fails safely when that temporary-space budget is unavailable.
- ACP collection is request-driven, permits at most ten selected sessions,
  fetches from at most two reachable executor-side `agentctl` instances at a
  time, and has a 30-second total collection deadline. It never continuously
  copies protocol frames to the backend. ACP collection, like frontend capture,
  runs outside product-critical paths and may produce a partial bundle.
- Log payloads use ZIP `Store` rather than CPU-heavy compression. Copy and ZIP
  writing use 1 MiB chunks and yield between chunks. Collection/build jobs have
  a five-minute hard lifetime; ready/partial archives expire 15 minutes after
  becoming downloadable.
- The bundle manifest records capture, persistence, queue, and archive
  truncation/loss counters. Diagnostic failures and pressure must be visible in
  the artifact without creating recursive log traffic.

### System Logs page and bundle contents

- `/settings/system/logs` no longer renders a log tail, file table, individual
  file downloads, copy action, or refresh action.
- The page states that a standard bundle does not read stored chat history,
  session transcripts, agent responses, prompts, tool messages, database rows,
  or workspace files. This is a source boundary, not a redaction guarantee:
  incidental content already emitted into a selected log may still appear.
- The frontend disclosure names `console.debug`, `console.info`,
  `console.warn`, `console.error`, uncaught JavaScript errors, unhandled promise
  rejections, bounded browser/runtime context, and user-visible error toasts
  that are reported into backend logs.
- The backend disclosure names structured application/runtime, HTTP/service,
  executor, integration, startup/shutdown, warning/error, identifier, path,
  performance, and diagnostic-loss events. Backend inclusion copies emitted
  file lines; it does not query product tables or construct transcripts.
- The page exposes one always-visible **Customize bundle** action. It opens
  source selection with backend and frontend selected by default; the optional
  runtime index and ACP evidence start unselected. Collection, preparing,
  ready/partial, busy-with-retry, and failure states remain on the page without
  navigating away.
- When the backend reports that raw ACP capture is enabled, ACP is an optional
  source in that same customizer. Selecting it requires the user to choose one
  or more eligible sessions before collection starts. ACP is never silently
  included by a one-click action. The source remains available when no eligible
  session exists; the picker then shows an explanatory empty state and keeps
  submission disabled.
- The ACP selection surface explicitly warns that raw and normalized protocol
  frames can contain full prompts, agent responses, tool arguments/results,
  workspace file content, MCP payloads, environment-derived values, and
  secrets. The standard no-transcript disclosure never appears to cover an
  ACP-inclusive bundle.
- ACP candidate rows show their authorized Kandev task title as a link that
  opens that task in a new tab. The title is available only to identify a
  picker row; it is not written to the runtime index, manifest, or archive.
  The picker provides explicit select-all and clear-selection controls. Select
  all includes the first eligible sessions within the backend's maximum rather
  than exceeding the collection limit.
- `manifest.json` is always included and is not a selectable source. The
  optional runtime index contains only task/session IDs, agent/provider/model,
  status, timestamps, executor type, and ACP availability. It excludes task
  titles, descriptions, prompts, responses, tool payloads, file content, and
  stored message bodies.
- Desktop renders source/session customization in a modal dialog. Phone uses
  an inset bottom drawer patterned after the shipped mobile picker/menu
  surfaces, with one internally scrolling body, a fixed disclosure/header,
  safe-area-aware actions, no horizontal overflow, and at least 44px touch
  targets. Both viewports share selection, validation, job, and download logic.
- The combined ZIP uses this layout:

```text
manifest.json
backend/backend-logs.log
backend/backend-logs-YYYY-MM-DD.log
frontend/browser-01.jsonl
frontend/browser-02.jsonl
runtime/sessions.json
acp/session-01/raw-acp.jsonl
acp/session-01/normalized-acp.jsonl
```

- `backend/` contains the recognized active and dated files still inside the
  three-day retention window. File bytes are copied without reformatting.
- `frontend/` contains one JSON Lines file per distinct responding browser
  profile. Multiple tabs from one browser profile are deduplicated. Client
  values never determine archive paths or filenames.
- `runtime/sessions.json` is present only when the runtime source is requested.
  It contains at most 500 authorized sessions updated within the backend
  three-day diagnostic window, newest first. When ACP sessions are selected,
  their rows are included even if they fall outside that window.
- `acp/` is present only for explicitly selected sessions while ACP debug
  capture is enabled. It contains raw and normalized retained JSONL frames
  collected on demand from host-resident files or reachable executor-side
  `agentctl` instances. Archive directories are server-numbered; client-supplied
  session IDs never become ZIP paths. `manifest.json` maps each directory to
  its authorized task/session and records unavailable or truncated files.
- `manifest.json` identifies requested and included sources, capture time,
  Kandev version/commit, OS/architecture, uptime and bounded runtime metrics,
  backend filenames, frontend browser/connection counts, storage mode, omitted
  or truncated data, selected ACP session availability and byte ranges, runtime
  index counts, capture timeouts, and archive expiry.
- Bundle archives and working files are owner-only and live in a Kandev-owned
  temporary directory. User/API bundle jobs expire 15 minutes after creation.
- If frontend or ACP capture is unavailable or incomplete, a multi-source
  bundle still becomes downloadable with backend files. Its state is `partial`,
  and the manifest plus UI state explains each unavailable source/session.
- Any backend/ACP byte-range truncation, frontend profile omission, runtime
  index truncation, queue loss, or archive-cap truncation also makes the bundle
  `partial`; the manifest records exact source byte ranges and aggregate loss
  counters.

### Agent diagnostics

- Agents use the same bundle API and request only `backend`, `frontend`, or
  `all`. A backend-only request never waits for a browser.
- The task-mode MCP tool does not expose raw ACP or runtime-index selection.
  ACP capture is an explicit human download flow because its stronger content
  and permission contract must be reviewed before collection.
- Task-session agents normally use the task-mode
  `get_diagnostic_bundle_kandev` MCP tool. It accepts only a source selection;
  it derives the task, session, and authenticated owner from the MCP dispatch
  context, creates the identity-owned job, waits for ready/partial, and
  materializes the ZIP as an owner-only file under the execution workspace's
  ignored `.kandev/diagnostics/` directory. It returns that local path and a
  manifest summary; it never accepts a user ID, task ID, output path, or token.
- `scripts/kandev-logs` remains the host-side fallback. Authentication-disabled
  instances need only a port. Authenticated use reads a PAT exclusively from
  `KANDEV_API_TOKEN`; it never accepts or prints a token on the command line.
- Agent guidance downloads the selected ZIP, extracts it into a fresh temporary
  directory, searches an exact task ID first, and broadens to session ID,
  route/error text, or a bounded time window when needed.
- The dev-only `/api/v1/system/debug/export` log response is removed. Runtime
  metadata moves to `manifest.json`; no replacement unbounded JSON log query is
  introduced.
- Improve Kandev uses the same archive builder instead of writing snapshots
  from the removed backend ring buffer. When **Include logs** is selected, the
  initiating browser creates an authenticated `all` job and waits for
  ready/partial before task submission. It then calls an Improve Kandev lease
  endpoint with that caller-owned bundle ID and bootstrap directory; the
  backend verifies common ownership, copies the ZIP to
  `diagnostic-bundle.zip`, and returns the path appended to the task
  description. The bootstrap route never waits for frontend collection.
  The task-context copy retains the existing 24-hour cleanup window so a newly
  launched agent can read it.

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
- If midnight rollover fails after startup, the backend emits an error to
  stderr and its existing sinks, continues writing to the active file, and
  retries rollover on the next write without dropping the triggering entry.
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

- `backend-logs.log` preserves entries accepted by the file queue during the
  current UTC day until the fixed 256 MiB daily bound. Pressure drops, startup
  fallback gaps, bounded-shutdown loss, and archive truncation are explicit in
  counters and bundle manifests.
- Dated archives preserve the two preceding UTC calendar days. Files outside
  that rolling window are deleted at startup and after successful rollover.
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
  **THEN** it appends without losing earlier entries.
- **GIVEN** the configured log directory is temporarily unwritable, **WHEN**
  Kandev starts, **THEN** product startup succeeds with a stderr warning and
  the backend retries file activation every 30 seconds.
- **GIVEN** more than three UTC days of backend files, **WHEN** cleanup runs,
  **THEN** only the current and two preceding days remain.
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
