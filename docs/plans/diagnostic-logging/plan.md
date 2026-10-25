---
spec: docs/specs/platform/requirements/diagnostic-logging.md
created: 2026-07-30
status: implemented
---

# Implementation Plan: Diagnostic logging

## Overview

Establish the fixed daily backend files first, because every later endpoint and
bundle depends on their path, retention, and sink semantics. Add bounded error-
toast ingestion and align every launcher. In parallel, implement frontend toast
reporting and the backend diagnostic-bundle job/protocol. Then replace the
browser memory-only console capture with bounded local three-day retention and
the System Logs download workflow. Finally remove legacy ring-buffer/viewer
paths, migrate Improve Kandev, update docs, and teach agents to download,
extract, and grep source-selectable bundles. The follow-up extension preserves
that standard flow while adding an optional sanitized runtime index and an
explicit, session-selected ACP source fetched on demand only in debug mode.

The implementation follows
[ADR-2026-07-30-file-backed-diagnostic-bundles](../../decisions/2026-07-30-file-backed-diagnostic-bundles.md).

---

## Backend daily files and sinks

- In `apps/backend/internal/common/logger/logger.go`, preserve `NewLogger` for
  agentctl and focused callers, and add a backend-specific constructor whose
  file and stdout cores have independent level enablers.
- Add a backend daily writer for
  `<ResolvedHomeDir>/logs/backend-logs.log`: create the directory owner-only,
  append on same-UTC-day restarts, roll at UTC midnight or first startup after
  a boundary, and create new active files as `0600`.
- Name completed files `backend-logs-YYYY-MM-DD.log`. Keep the current UTC day
  plus the preceding two UTC days, deleting only strictly recognized older
  daily filenames and preserving unrelated neighbors.
- Persist the active file's UTC-day identity rather than inferring it only from
  mtime, and inject a clock for deterministic rollover tests. Serialize
  write/rollover/close so concurrent entries cannot be lost or split across
  the wrong day.
- Make rollover crash-recoverable with an owner-only transaction journal
  containing source day/identity/size, destination, and copied offset. Resume
  an interrupted merge exactly once and never replace an existing dated file.
- Give file and stdout independent priority-aware asynchronous queues. Enforce
  the spec's entry/byte/reserved capacities and 256 KiB encoded-entry maximum;
  expose per-sink loss counters without recursively logging each drop. Emit a
  compact recovery summary only after that sink can accept it.
- Expose a read-only sink-statistics snapshot for manifest construction
  (accepted/dropped by sink, level, and reason); this is not another log buffer.
- Drain both queues for at most two seconds on graceful close. Give terminal
  DPanic/Panic/Fatal entries the bounded flush plus capped direct-stderr path
  defined by the spec.
- In `apps/backend/internal/backendapp/main.go`, resolve the fixed path from
  `cfg.ResolvedHomeDir()`, construct the dual-sink logger, and print the
  absolute path directly to stdout before normal structured startup logs.
- In `apps/backend/internal/common/config/config.go`, remove the destination
  and lumberjack rotation fields/defaults. Keep `logging.level` and
  `logging.format`; `logging.level` becomes the file threshold.
- Do not expand the existing process-wide ring-buffer contract. Its consumers
  are migrated and the buffer is removed after the bundle workflow lands.
- File open/create and retention-cleanup failures must not prevent product
  startup. Start with stderr/stdout sinks, report the intended path, and retry
  file activation every 30 seconds.

## Error-toast ingestion

- Add `apps/backend/internal/system/frontenderrors/handler.go` with
  `POST /api/v1/system/logs/frontend-errors`.
- Define a typed request and normalization helper in the logs package. Limit
  the body to 64 KiB, validate source and visible text, UTF-8-safely bound each
  field including the optional 128-byte task ID, and record truncation.
- Log a fixed `frontend error toast` message at error level with client values
  only as structured fields, then return `204`.
- Add bounded in-memory token buckets: per identity 60/minute with burst 20,
  process-wide 300/minute with burst 100. Return `429` plus `Retry-After` and
  keep limiter identity state TTL-bounded.
- Register the route on the authenticated non-admin System group.

## Launchers and CLI contracts

- In the native Go launcher, separate file and stdout thresholds, forward
  backend stdout in every mode, and retain captured startup-failure output.
- Add internal `KANDEV_CONSOLE_LOG_LEVEL`: `warn` normally and in debug, `info`
  only for `--verbose`. `KANDEV_LOG_LEVEL` remains the file threshold (`info`
  normally, `debug` for `--debug`, unless explicitly overridden).
- Apply the same contract to the TypeScript development/release supervisors.
  When release stdout is piped for failure capture, tee it to parent stdout.
- Update native and TypeScript CLI help so `--debug` promises detail in the
  backend file while `--verbose` remains the stdout-info opt-in.

---

## Frontend toast reporting

### Best-effort report client

- Add `apps/web/lib/api/domains/frontend-error-log-api.ts` with the request
  type, environment capture, safe text/error extraction, route task-ID
  derivation, and a fire-and-forget call through the authenticated API client.
- Capture full URL, browser identity fields, language, platform, viewport,
  client timestamp, emission stack, and underlying `Error` details.
- Schedule the request only after delegating to the original toast implementation
  and never await it. Bound local context work, use a two-second deadline,
  and catch every rejection. Do not log it to `console.error`, retry it, or
  emit another toast.

### Toast seams

- Add `apps/web/lib/toast/sonner.ts`, re-export the Sonner API used by Kandev,
  and wrap `toast.error` so it delegates first and schedules reporting
  immediately afterward in the same turn.
- Mechanically replace application `toast` imports from `sonner` with the
  Kandev wrapper. Keep the shared toaster renderer on upstream Sonner.
- Add a source-scan assertion so application call sites cannot bypass the seam.
- In `apps/web/components/toast-provider.tsx`, report error creation and
  transitions into the error variant without changing IDs, timers, ordering,
  ARIA behavior, placement, styling, or dismissal.

---

## Diagnostic bundle backend

### Job and archive service

- Add a dedicated `apps/backend/internal/system/logbundle` package rather than
  growing the existing file-list handler. It owns job state, temporary working
  directories, archive construction, expiry cleanup, and a clock/filesystem
  seam for tests.
- Bundle jobs are identity-owned, use unguessable IDs, create owner-only files,
  have a five-minute collecting/building deadline, expire 15 minutes after
  becoming ready/partial, and have states `collecting`, `building`, `ready`,
  `partial`, `failed`, and `expired`.
- Move collection, copy, JSONL writing, and ZIP construction to a bounded
  background worker. Allow one active job per identity, eight collecting/queued
  jobs process-wide, and one build process-wide. Coalesce the same source set;
  return `429` for a different identity-local active request and `503` for
  global saturation, both with `Retry-After: 5`.
- Backend capture copies only recognized active/dated log files inside the
  rolling three-day window into `backend/`, preserving bytes and names. Apply a
  160 MiB backend budget newest-first; include only the newest byte range when
  a file exceeds the remaining budget.
- Accept at most four browser profiles, 20 MiB each and 80 MiB total frontend
  data. Cap uncompressed archive payload at 256 MiB and temporary job data at
  384 MiB, checking available space before admission. Use ZIP `Store`, stream
  1 MiB chunks, and yield between chunks rather than spending CPU on deflate.
- Runtime metadata formerly exposed by debug export goes into
  `manifest.json`. Manifest construction is deterministic and records sources,
  included filenames, browser/connection counts, storage modes, truncation,
  timeouts, warnings, and expiry without listing another user's identity.
- ZIP entry names are server-owned. Reject symlinks and non-regular backend
  files, and enforce containment before opening any source or working path.

### Frontend request/response protocol

- Add `POST /api/v1/system/logs/bundles`,
  `GET /api/v1/system/logs/bundles/:id`,
  `POST /api/v1/system/logs/bundles/:id/frontend`, and
  `GET /api/v1/system/logs/bundles/:id/download`.
- A frontend source sends `system.logs.capture_requested` through the WebSocket
  hub only to connected clients matching the HTTP caller's identity. Extend
  the hub with an identity-targeted send that returns the number of queued
  connections; do not use a global broadcast.
- Collect for at most 15 seconds. Track targeted/responded connection counts,
  accept sequential ≤1 MiB chunks, cap each browser at 10,000 entries/20 MiB,
  and cap each job at four browsers/80 MiB.
- Require an opaque per-tab `capture_stream_id`. The first accepted chunk
  atomically claims its browser ID; acknowledge later streams for that browser
  without decoding/appending entries. Validate sequence only within the winning
  stream so tab interleaving cannot mix snapshots.
- Validate every frontend entry again on the backend. Write one server-named
  JSONL file per distinct browser profile.
- Backend-only jobs skip WebSocket capture and can build immediately.
- Register the bundle worker/expiry loop with explicit start/stop ownership and
  leak tests per backend goroutine conventions.

---

## Browser console retention and capture

### Local store

- Replace the memory-only implementation behind `apps/web/lib/logger/buffer.ts`
  with a storage facade: IndexedDB is authoritative, while the 500-entry
  memory ring remains the fallback.
- Preserve the existing interceptor seams. Normalize console arguments into
  bounded JSON-safe data off the intercepted console call path and add URL,
  recognized route task ID, browser installation ID, serialized size, and
  storage mode.
- The interceptor creates a reference-free preview: inspect at most 20
  arguments, keep capped primitives and `Error` fields, and represent other
  values by safe type descriptors without retaining objects, walking
  prototypes, or invoking getters.
- Enforce a 500-entry/2 MiB staging queue. Drain at most 50 entries/256 KiB per
  transaction after 250 ms or in idle time with a one-second timeout fallback;
  never await IndexedDB opening, serialization, pruning, or quota work from a
  console call. Shed debug/info before warn/error and retain loss counters.
- Retain three UTC days and enforce 10,000 entries/20 MiB per browser profile,
  with 64 KiB per entry. Prune expired records first, then oldest-first.
- Store one opaque random browser installation ID per browser profile. All tabs
  share it; never expose it as a ZIP filename or authorization credential.
- Partition entries by the current authenticated Kandev identity and snapshot
  only that partition. Apply entry/byte caps across all partitions so account
  churn cannot create unbounded browser storage.
- Make initialization, capture, snapshot, pruning, quota failure, and memory
  fallback testable without relying on a real browser database.

### WebSocket responder and uploads

- Add a narrow top-level hook/bridge that handles
  `system.logs.capture_requested`, snapshots local history, chunks it under the
  advertised limit, uploads sequentially, and always sends a final `done`
  chunk when possible.
- Every tab creates a random `capture_stream_id` for its response. Multiple tabs
  may answer; the backend atomically accepts one stream per browser ID, so the
  client does not need cross-tab leader election.
- Put bounded loss/persistence/truncation counters in the final `done` chunk's
  `capture_metadata`; upload no more than 8 KiB and let the backend validate it.
- Capture/upload failures stay silent in the console to avoid recursion. The
  backend's partial manifest is the diagnostic surface.

---

## System Logs user workflow

- Replace `LogViewer` with one focused diagnostic-bundle card. Remove tail,
  copy, refresh, file table, badges, and individual file actions.
- Explain in visible plain language that the combined ZIP contains install-wide
  backend logs plus locally retained browser console events, full URLs,
  console arguments, stacks, and runtime metadata; disclose the three-day,
  four-browser, and fixed byte limits and tell users to review before sharing.
- The primary **Download diagnostic bundle** action requests both sources,
  polls/subscribes to job progress, downloads ready or partial ZIPs, and shows
  why a partial bundle lacks frontend data.
- Reuse `SystemPageShell`, `Card`, `Button`, and existing action-feedback/status
  patterns. The route remains `/settings/system/logs`.

### Mobile design contract

- Desktop and phone use the same one-dimensional card and shared state/action.
  There is no table, sidebar, overlay, or alternate navigation to adapt.
- The existing System settings route is the mobile entry point. The disclosure
  precedes the primary action; status/warnings follow it.
- Use the normal page as the single scroll owner. The action has at least a
  44px active height, remains full-width when helpful on phones, and requires
  no hover or wide-viewport interaction.
- Add a `mobile-chrome` E2E scenario proving disclosure, touch activation,
  progress/partial behavior, download availability, and no horizontal overflow.

---

## Custom bundles, runtime index, and ACP debug evidence

### Backend source and permission contracts

- Extend `apps/backend/internal/system/logbundle` source normalization and job
  identity to support `runtime` and `acp` plus a normalized ACP session-ID set.
  Preserve `backend`/`frontend` behavior and make coalescing key off both source
  and selected-session sets.
- Add authenticated capabilities and ACP-candidate endpoints under
  `/api/v1/system/logs`. Cap candidate rows at 500, return only allow-listed
  session/runtime fields, and derive candidate visibility through existing task
  authorization: owners see their sessions, admins may see all.
- Add a runtime-index provider that emits `runtime/sessions.json` from at most
  500 authorized sessions in the three-day diagnostic window plus explicitly
  selected ACP sessions. Use a dedicated DTO so task titles, descriptions,
  identities, messages, prompts, tool data, files, arbitrary metadata, and
  configuration cannot enter by accident.
- Gate ACP capability, candidate listing, and job creation on the backend's
  effective `KANDEV_DEBUG_AGENT_MESSAGES` state. A client-side debug flag is
  presentation only and never authorizes collection.
- Keep `manifest.json` mandatory. Add requested/included runtime and ACP source
  fields, selected/available/omitted session counts, executor availability,
  byte ranges, deadlines, and warnings without recording user identities or
  filesystem paths.

### On-demand ACP export

- Add an agentctl internal export route beside the existing debug ACP tail. It
  is registered only while ACP debug logging is enabled, accepts an exact
  session and clamped byte budget, and produces a bounded ZIP of recognized
  raw/normalized retained JSONL files for only that session.
- Add a runtime/lifecycle client seam that locates the selected execution and
  streams the export from reachable local, Docker, Sprites, or remote agentctl
  instances. Host-resident standalone logs use the same recognized-filename
  reader. Do not centralize frames before a request.
- Authorize all selected sessions before I/O. Fetch at most two executors
  concurrently, collect at most ten sessions for 30 seconds total, stream in
  fixed chunks, and cancel/close every partial response on deadline or job
  cancellation.
- Revalidate returned ZIP structure, regular-file status, per-entry/session
  ownership, JSONL filenames, and bytes before copying into server-numbered
  `acp/session-NN/` paths. Treat stopped/unreachable/expired/invalid sessions as
  partial rather than falling back to another session or broader directory.
- Preserve the existing 256 MiB archive and 384 MiB temporary-disk caps.
  Standard bundles retain 160 MiB backend and 80 MiB frontend budgets;
  ACP-inclusive jobs use 96 MiB backend, 48 MiB frontend, 96 MiB ACP, and
  2 MiB runtime budgets with no redistribution from omitted sources.

### Logs page customization

- Replace the single action copy in
  `apps/web/components/settings/system/log-viewer.tsx` with a shared bundle
  request controller and one **Customize bundle** entry point.
- Fetch backend capabilities instead of inferring ACP availability from
  `window.__KANDEV_DEBUG`. The customizer defaults to backend+frontend,
  offers runtime as an opt-in, and exposes ACP as an opt-in only when the
  backend allows it. Selecting ACP loads candidates on demand and requires one
  to ten eligible sessions before submit. In debug mode it remains available
  with an explanatory empty state when the caller has no eligible session.
- Add exact translated copy describing frontend and backend event classes,
  stating that standard bundles do not read stored chat/session/agent messages,
  and warning that incidental text already emitted to logs may remain.
- When ACP is selected, replace the standard reassurance with the explicit raw
  protocol warning covering prompts, responses, tools, files, MCP data,
  environment-derived values, and secrets. Show each session's authorized task
  title as a new-tab task link, per-session availability, and explicit
  select-all/clear controls that honor the server's maximum; explain why
  unavailable sessions cannot be collected. Keep task titles out of the
  runtime index and all archive metadata.
- Keep all selection, validation, polling, partial/error status, and download
  logic shared. Do not add source preferences to persistent settings or a
  database; each customizer opening starts from the same defaults.

### Mobile design contract

- Desktop uses the existing design-system `Dialog`; phone uses the shipped
  inset `Drawer` pattern from `mobile-menu-sheet`/`mobile-picker-sheet`. The
  Settings > Logs route and diagnostic card remain the entry point.
- The disclosure/header and bottom actions stay fixed; the source/session list
  is the only `min-h-0 overflow-y-auto` scroll owner. Clear the bottom safe area
  and keep document horizontal overflow at zero.
- Stack card actions on phones, keep each primary/secondary trigger and drawer
  row at least 44px high, preserve visible labels, and avoid hover-only details.
- Add desktop and `mobile-chrome` E2E flows proving standard download,
  customization, ACP session validation, high-sensitivity disclosure, partial
  unavailable-session behavior, safe-area/overflow geometry, and equivalent
  final bundle contents.

---

## Agent diagnostic materialization

- Register task-mode `get_diagnostic_bundle_kandev` with a required
  `sources` enum (`backend`, `frontend`, or `all`) and no identity, task,
  session, path, or token arguments.
- Forward the current MCP server's task/session IDs. The existing in-session MCP
  identity scoper resolves the task owner before the backend handler creates a
  bundle; external credentialed MCP callers retain their authenticated identity.
- Wait for ready/partial within the job's hard lifetime, then materialize the
  ZIP through the existing lifecycle/agentctl file-transfer seam to
  `.kandev/diagnostics/<bundle-id>.zip` inside the current execution workspace,
  mode `0600`. Never trust a caller-selected destination.
- Return the executor-local path plus manifest source/completeness/loss summary.
  A backend-only call never sends a frontend WebSocket capture request.
- Clean materialized files on session/task workspace cleanup. The repository's
  ignored `.kandev/` directory keeps the artifact out of git status.
- Keep `scripts/kandev-logs` as a host-side fallback: no credential in
  auth-disabled mode; under auth it reads a PAT only from
  `KANDEV_API_TOKEN`, sends it as Bearer auth, and never prints it.

---

## Legacy-path removal and Improve Kandev migration

- Remove the backend structured ring-buffer core/package and
  `Logger.BufferSnapshot`.
- Remove System Logs list/tail/individual-download handlers, service state,
  frontend API methods/hooks/store fields/components, and their tests.
- Remove dev-only `/api/v1/system/debug/export` and update
  `scripts/kandev-logs` to create/download source-selectable bundle jobs.
- Migrate Improve Kandev's “Include logs” path to the public authenticated
  all-sources job. After bootstrap returns, the initiating browser creates and
  waits for ready/partial, then calls
  `POST /api/v1/system/improve-kandev/bundle/lease` with its job ID and bootstrap
  directory. The backend verifies both belong to the same identity, copies
  `diagnostic-bundle.zip`, and returns the path appended to the task
  description. Bootstrap itself never waits for the 15-second collection.
- Bind every bootstrap directory to its authenticated creator with a
  server-written owner marker outside the archive. Prefix/path validation is
  containment defense, not authorization.
- Update Improve Kandev's disclosure from “small in-memory ring buffer” to exact
  bundle contents and partial/truncation behavior.
- Reuse archive construction and validation while keeping the longer explicit
  Improve Kandev lease separate from 15-minute public bundle jobs.

---

## Tests

- **Daily writer and dual sinks:** fake-clock Go tests cover normal/debug/
  verbose thresholds, same/cross-day restarts, midnight rollover, concurrent
  writes, `0600`, strict cleanup matching, rollover retry, blocked I/O,
  independent-sink isolation, exact capacities, priority shedding, loss
  accounting, bounded close/fatal behavior, startup fallback/retry, journaled
  crash points, collision recovery, and producer non-blocking behavior.
- **Configuration:** Go tests prove removed destination/rotation keys are
  ignored and level/format remain.
- **Toast endpoint:** Gin tests cover auth, valid sources, optional task ID,
  malformed/oversized bodies, missing text, UTF-8 truncation, and fixed
  server-owned message/level, per-identity/global token buckets, TTL cleanup,
  `429`, and `Retry-After`.
- **Launchers:** native and TypeScript tests cover every file/stdout level
  matrix and warning stdout forwarding.
- **Toast seams:** Vitest covers environment/route capture, silent failure,
  delegate-before-report ordering, legacy transitions, non-awaited scheduling,
  two-second deadline, and direct-import prevention.
- **Bundle service:** Go tests cover identity ownership, source selection,
  backend allow-list/containment, manifest determinism, ZIP layout, chunk
  ordering/bounds, browser dedupe, partial/timeout states, expiry cleanup,
  frontend identity isolation, interleaved capture streams, exact coalescing/
  `429`/`503` capacity, per-source/job/temp byte caps, newest-range truncation,
  disk-space refusal, five-minute cancellation, and start/stop leak safety.
- **Runtime index and capabilities:** Go handler/service tests cover debug
  capability gating, owner/admin session visibility, foreign-ID non-disclosure,
  source/session validation, coalescing keys, allow-listed runtime DTO fields,
  500-row/three-day bounds, and failure-to-partial behavior.
- **ACP exporter and collector:** Agentctl tests cover debug-disabled `404`,
  exact-session file matching, raw+normalized rotation selection, clamped
  newest-byte ranges, malicious paths, and bounded streaming. Runtime/logbundle
  tests cover host and executor sources, owner/admin authorization before I/O,
  ten-session/two-fetch/30-second limits, cancellation, partial unavailable
  sessions, per-source budgets, archive renaming, manifest mapping, and no
  continuous-copy goroutine.
- **Browser store:** Vitest with an IndexedDB test implementation covers
  three-day/entry/byte pruning, 64 KiB entry limits, cyclic/unserializable
  values, reference-free object descriptors, argument/field caps, shared
  browser ID, identity partitioning/account switches, quota failure, memory
  fallback, snapshots, exact staging/batch limits, overflow priority, and the
  invariant that slow persistence cannot delay the intercepted console call.
- **Capture bridge:** Vitest covers WS request handling, per-tab stream IDs,
  sequential chunks, done marker, interleaved duplicate-tab safety, and silent
  upload failures.
- **System Logs UI:** component tests cover disclosure, job states, partial
  warning, download, and accessible action.
- **Custom Logs UI:** Vitest covers standard source defaults, capability-driven
  ACP visibility, custom source validation, required/owned session selection,
  runtime-index selection, standard versus ACP disclosure, translated copy,
  shared polling/download state, unavailable sessions, and desktop dialog versus
  phone drawer presentation.
- **Legacy removal:** source scans reject the old tail/list/debug-export APIs
  and backend ring-buffer imports.
- **Improve Kandev:** backend/frontend tests cover reuse of the all-sources ZIP
  job, owner-verified lease, non-blocking bootstrap, partial archive, and
  task-description path.
- **Agent diagnostics:** Go/MCP integration tests cover task-owner identity,
  source selection, no caller-selected path, local and remote materialization,
  cleanup, backend-only no-capture behavior, and manifest summary.

## E2E tests

- Update `apps/web/e2e/tests/system/logs-page.spec.ts` using TDD: navigate through
  the System Logs UI, assert the full disclosure, request a bundle, answer the
  backend capture request through the real frontend bridge, and verify the
  downloaded ZIP exposes separate backend/frontend entries and a manifest.
- Add `apps/web/e2e/tests/system/mobile-logs-bundle.spec.ts` for the
  `mobile-chrome` project: use `.tap()`, verify the ≥44px action, partial-state
  feedback when capture is unavailable, downloadable backend evidence, and no
  document horizontal overflow.
- Extend the desktop spec with debug-disabled standard/custom behavior and a
  debug-enabled seeded session catalogue. Verify that **Download with ACP…**
  cannot submit without a session, the strong warning is visible, selected raw
  and normalized evidence plus the sanitized runtime index appear in the ZIP,
  and unavailable sessions yield an explicit partial result.
- Extend the mobile spec to open Customize and ACP selection through the inset
  drawer, complete the same source/session workflow, assert ≥44px controls,
  internal scrolling, viewport containment, safe-area clearance, focus return,
  and zero document horizontal overflow.
- Use `pnpm e2e:run` so production backend/web artifacts are rebuilt before the
  focused specs run.

---

## Public documentation

- Update `docs/public/configuration.md` to remove destination/lumberjack
  settings and document fixed daily files and the three-day UTC policy.
- Update `docs/public/contributing.md` for startup path output and the
  diagnostics-bundle workflow.
- Update `docs/public/docker.md`, `docs/public/operations.md`, and
  `docs/public/k8s.md` for resolved file paths, container stdout thresholds,
  source-selectable bundle downloads, browser availability, and sensitive data.
- Document that the System Logs page downloads backend plus frontend evidence,
  that frontend history stays local until requested, and that partial bundles
  must be checked via `manifest.json`.
- Update CLI/operations/configuration/remote-cloud docs to distinguish the
  standard message-table-free source boundary from raw ACP's explicit
  high-sensitivity content, owner/admin selection rules, host versus executor
  availability, runtime index fields, fixed ACP limits, and partial behavior.
- Update `.agents/skills/debug` guidance to request standard sources first and
  use ACP only after a human-selected debug bundle is available. Continue exact
  task-ID/session-ID grep and never describe standard no-transcript language as
  applying to an ACP-inclusive archive.

---

## Debug harness

- Update `.agents/skills/debug/SKILL.md` and its instance/browser references to
  use `get_diagnostic_bundle_kandev` in task sessions and request `backend`
  first for backend/live-instance issues, `frontend` for browser evidence, or
  `all` only when correlation requires both.
- Teach agents to create a fresh temporary directory, download the ZIP, extract
  it safely, inspect `manifest.json`, exact-grep task ID first, then broaden by
  session ID, route/error text, and time window.
- Treat a zero task-ID match as evidence, not proof: global/startup lines and
  frontend pages outside task routes may omit it.
- Update `scripts/kandev-logs` to drive bundle jobs rather than debug export or
  the removed stderr-tail convention. Under auth it reads only the
  `KANDEV_API_TOKEN` environment variable. Never extract over an existing
  directory.

---

## Verification Results

Tasks 01–11 record the implemented standard bundle results in their individual
task files. Tasks 12–17 now record the implemented custom source/runtime-index,
on-demand ACP, consolidated customizer UI, E2E, and privacy-guidance work. No
database migration or browser schema migration is required by this extension.

---

## Implementation waves and parallel candidates

The default is sequential execution in the primary conversation.

Wave 1:

- [x] [Task 01: Backend log sinks](task-01-backend-log-sinks.md)

Wave 2:

- [x] [Task 02: Frontend-error endpoint](task-02-frontend-error-endpoint.md)
- [x] [Task 03: Launcher contracts](task-03-launcher-contracts.md)

Task 02 and Task 03 are parallel-safe after Task 01. Parallel execution still
requires explicit user authorization.

Wave 3:

- [x] [Task 04: Toast reporting](task-04-toast-reporting.md)
- [x] [Task 07: Diagnostic bundle backend](task-07-diagnostic-bundle-backend.md)

Task 04 owns frontend toast files; Task 07 owns backend bundle/gateway files.
They are parallel-safe after their dependencies with explicit authorization.

Wave 4:

- [x] [Task 08: Browser logs and bundle UI](task-08-browser-logs-and-bundle-ui.md)

Wave 5:

- [x] [Task 10: Agent diagnostic materialization](task-10-agent-diagnostic-materialization.md)

Wave 6:

- [x] [Task 09: Remove legacy diagnostics](task-09-remove-legacy-diagnostics.md)

Wave 7:

- [x] [Task 05: Public logging documentation](task-05-public-logging-docs.md)
- [x] [Task 06: Debug workflow guidance](task-06-debug-workflow-guidance.md)

Task 05 and Task 06 are parallel-safe after Tasks 09 and 10 because public docs
and agent-harness files are disjoint. Parallel execution still requires
explicit user authorization.

Wave 8:

- [x] [Task 11: Merge-risk hardening](task-11-merge-risk-hardening.md)

Task 11 is a sequential PR-remediation task covering the current-main lint
integration, bounded automatic toast ingestion, the fixed daily file bound,
URL privacy hardening, and allocation-free IndexedDB pruning.

Wave 9:

- [x] [Task 12: Custom bundle contracts and runtime index](task-12-custom-bundle-contracts.md)

Wave 10:

- [x] [Task 13: On-demand ACP collection](task-13-on-demand-acp-collection.md)

Task 13 depends on Task 12's source/session and manifest contracts. Both tasks
are sequential because they share logbundle job/archive state and permission
boundaries.

Wave 11:

- [x] [Task 14: Bundle customizer UI](task-14-bundle-customizer-ui.md)

Wave 12:

- [x] [Task 15: Custom bundle E2E](task-15-custom-bundle-e2e.md)
- [x] [Task 16: Diagnostic privacy documentation](task-16-diagnostic-privacy-docs.md)

Task 15 follows the completed backend/frontend behavior. Task 16 is
parallel-safe with Task 15 because it owns public docs and debug guidance while
Task 15 owns Playwright fixtures/specs; parallel execution still requires
explicit user authorization.

Wave 13:

- [x] [Task 17: Bundle customizer refinement](task-17-bundle-customizer-refinement.md)

## Risks

- Frontend console data is highly sensitive. It remains browser-local until an
  explicit request, but combined bundle disclosure and identity routing must be
  unambiguous.
- IndexedDB quota/private-mode failures must degrade to bounded memory without
  breaking console behavior or bundle creation.
- WebSocket capture and HTTP chunk upload span two transports; ownership,
  expiry, disconnect, duplicate tabs, and late chunks need deterministic tests.
- ZIP creation must not follow symlinks, accept client paths, zip-slip on
  extraction guidance, or grow beyond frontend/backend bounds.
- Raw ACP can contain prompts, responses, tools, files, MCP payloads, and
  secrets. Capability gating, explicit session selection, owner/admin
  authorization before I/O, high-sensitivity copy, and server-owned archive
  paths are release blockers.
- Executor-side ACP evidence may disappear with stopped or removed containers.
  On-demand collection must report this as partial without centralizing frames
  continuously or blocking session/agent work.
- The runtime index must be built from a field allow-list, not generic model or
  metadata serialization, so future message-bearing fields cannot enter by
  accident.
- Desktop Dialog and phone Drawer must share state/validation while retaining
  one phone scroll owner, safe-area clearance, ≥44px targets, and no horizontal
  overflow.
- Any authenticated user retains install-wide backend log access by explicit
  product choice.
- Removing the ring buffer affects Logs, debug export, Improve Kandev, logger
  construction, scripts, tests, and user copy; migration must be atomic.
- Task-ID filtering is best effort. Logs before task resolution or outside task
  routes require session, route, error-text, or time-window correlation.
