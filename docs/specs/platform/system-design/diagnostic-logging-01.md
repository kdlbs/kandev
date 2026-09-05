---
status: draft
system: platform
requirements:
  - REQ-PLATFORM-DIAGNOSTIC-LOGGING-001
created: 2026-07-30
updated: 2026-08-27
owners:
  - tbd
---

# Diagnostic logging System Design Part 1

## Purpose and boundaries

This design preserves the technical source detail for `REQ-PLATFORM-DIAGNOSTIC-LOGGING-001` during migration.

## Requirement mapping

| Requirement                           | Design section                                    |
| ------------------------------------- | ------------------------------------------------- |
| `REQ-PLATFORM-DIAGNOSTIC-LOGGING-001` | [Migrated source detail](#migrated-source-detail) |

## Migrated source detail

Decisions: [bundles](../../../decisions/2026-07-30-file-backed-diagnostic-bundles.md),
[bounded logs](../../../decisions/2026-08-22-preserve-newest-bounded-backend-logs.md).

Browser retention: [requirements](../requirements/browser-console-retention.md) and
[system design](../system-design/browser-console-retention.md).

Implementation plan: [diagnostic-logging](../../../plans/diagnostic-logging/plan.md)

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
- Normal browser close code `1000` is lifecycle-only; unexpected codes remain
  error entries with stacks.
- `logging.level` / `KANDEV_LOG_LEVEL` remains the supported override for the
  file threshold, and `logging.format` remains supported.
  `logging.outputPath`, `logging.maxSizeMb`, `logging.maxBackups`,
  `logging.maxAgeDays`, and `logging.compress` are removed; the diagnostic
  path and daily retention policy are not configurable.
- `backend-logs.log` is the current-day active segment, capped at 16 MiB. Full
  files become `backend-logs-YYYY-MM-DD-NNNNNN.log`.
- Active, closed, and legacy files share a 256 MiB budget; rotation evicts old
  closed files so logging continues with newest evidence.
- At UTC midnight, the active file becomes the prior day's final segment and a
  new file opens. Cleanup removes recognized files older than three UTC days;
  this is a maximum age, not a per-day allocation.
- Legacy singleton files remain readable and count toward the budget. An
  oversized legacy active file is converted to bounded newest segments.
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

The browser retention limits, persistence guarantees, write serialization, and
incremental maintenance contract are authoritative in
[Browser console retention](../requirements/browser-console-retention.md) and its
paired [system design](../system-design/browser-console-retention.md).

- The frontend intercepts `console.debug`, `console.info`, `console.warn`, and
  `console.error`, plus `window.error` and `unhandledrejection`, without
  changing their normal browser behavior.
- Entries remain local to the browser until a diagnostic bundle explicitly
  requests them. Kandev does not continuously stream console logs to the
  backend.
- Each browser profile has an opaque random installation ID shared by its tabs.
- Entries are partitioned by the current authenticated Kandev identity. A
  capture returns only that identity's partition; signing into another account
  in the same browser profile cannot expose the previous account's entries.
  Auth-disabled mode uses the existing synthetic single-user scope.
- Entries contain a client timestamp, level, source, message, bounded
  JSON-safe arguments, available stack, full URL, and a task ID derived from a
  recognized task route when available.
- Console-call staging never retains an arbitrary caller object. It preserves
  primitive values and bounded `Error` fields. Other objects, functions, DOM
  values, and proxies become reference-free type descriptors without walking
  prototypes or invoking getters. At most 20 arguments are inspected; strings
  are capped at 4 KiB, error messages at 8 KiB, and stacks at 16 KiB before the
  overall 64 KiB entry cap.
- Persistence failure and memory fallback behavior follow
  `AC-PLATFORM-BROWSER-CONSOLE-RETENTION-001.6`. A resulting bundle identifies
  the degraded capture in its manifest.

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
  IndexedDB opening, serialization, retention maintenance, and quota handling never run
  synchronously in the intercepted console call. A full staging queue drops
  lower-priority `debug`/`info` entries first and records loss metadata; it
  never delays the original console call.
- A browser debug producer that scans a growing collection coalesces its own
  derived payload before it calls `console.debug`. The
  `messages:process` producer keeps the latest inputs and emits one trailing
  sample at most once per 250 ms. Later updates replace the pending sample.
  This bounds message counting, grouping summaries, console formatting, and
  browser-log staging during a stream without removing the final latest-state
  diagnostic.
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
backend/backend-logs*.log
frontend/browser-01.jsonl
frontend/browser-02.jsonl
runtime/sessions.json
acp/session-01/raw-acp.jsonl
acp/session-01/normalized-acp.jsonl
```

- `backend/` contains recognized active, segmented, and legacy files inside the
  three-day retention window. Candidates are selected newest-first, and file
  bytes are copied without reformatting.
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
