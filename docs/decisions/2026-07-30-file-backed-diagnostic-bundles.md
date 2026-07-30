# ADR-2026-07-30-file-backed-diagnostic-bundles: File-backed diagnostic bundles

**Status:** accepted
**Date:** 2026-07-30
**Area:** backend, frontend, infra, protocol, workflow

## Context

Frontend error toasts and console failures can be the only user-visible
evidence of a problem, while backend diagnostics are split between terminal
output, optional files, an in-memory ring buffer, a System Logs viewer, and a
dev-only debug export. Kandev already captures a bounded browser console buffer
and uploads it on demand for Improve Kandev, but that path is not a clear
user-downloadable artifact or a reusable agent workflow. The design has
meaningful privacy, retention, transport, and access alternatives.

## Decision

The backend owns an always-on current-day file at
`<ResolvedHomeDir>/logs/backend-logs.log`, rolls it at UTC midnight to
`backend-logs-YYYY-MM-DD.log`, appends across same-day restarts, and retains
the current plus two preceding UTC days. Normal/debug/verbose file and stdout
thresholds remain defined by the diagnostic-logging spec. Destination and
lumberjack rotation settings are removed.

The backend structured ring buffer, Logs-page tail and file table, individual
file downloads, and dev-only log export are removed. Files become the backend
source of truth. Runtime metadata moves into a diagnostic bundle manifest.

Diagnostic collection is best effort and deliberately stays off application
critical paths. Backend file and stdout sinks drain independent bounded,
priority-aware queues so a blocked terminal cannot stall the authoritative
file. Startup log-path failures degrade to stderr and background retry rather
than preventing Kandev from starting. Browser interception stages only
reference-free bounded values and persists compact batches asynchronously
during idle time. Toast reporting is scheduled after the toast is handed to the
UI library and is never awaited. Bundle capture and archive construction have
fixed per-job byte/profile limits plus a bounded, coalescing background worker
so one export cannot create unbounded CPU, disk, memory, or connection work.

The browser intercepts every console level plus uncaught errors and rejections,
but retains entries locally in bounded IndexedDB for three UTC days. It never
continuously uploads console history. When an authenticated bundle job requests
frontend evidence, the backend sends a WebSocket capture notification only to
connections for the requesting identity. Responding tabs upload bounded chunks;
the first per-tab capture stream to claim a browser-profile installation ID
owns that upload so interleaved tabs cannot mix snapshots. Local records are
partitioned by authenticated Kandev identity so an account switch inside one
browser profile cannot disclose the previous user's history.

Users download one clearly disclosed combined ZIP from System Logs. It contains
separate `backend/` and `frontend/` directories plus `manifest.json`. Frontend
capture may time out without blocking backend evidence; the bundle then becomes
partial and records the omission. Any signed-in user retains the existing
install-wide Logs-page access boundary, while bundle job ownership and frontend
capture remain identity-scoped.

Agents use the same source-selectable bundle API. They request `backend`,
`frontend`, or `all`, extract into a fresh temporary directory, and grep the
artifact locally—normally by exact task ID first. Kandev does not add a
server-side free-text log query. Task-session agents use an owner-scoped MCP
tool that derives identity from the current execution and materializes the ZIP
into that execution instead of exposing a reusable credential. Improve Kandev
creates an authenticated browser-owned bundle first, then leases a verified
copy into its task context. Public bundle jobs expire 15 minutes after becoming
downloadable; Improve Kandev keeps its copied artifact for the existing
24-hour cleanup window.

The observable contract lives in
[`docs/specs/platform/diagnostic-logging.md`](../specs/platform/diagnostic-logging.md).

## Consequences

Users and agents receive the same inspectable artifact, backend logs survive
same-day restarts, and large log searches happen with standard local tools
instead of an unbounded HTTP response. Backend memory no longer duplicates the
file sink. Frontend history stays browser-owned unless a user or agent
explicitly requests it.

Frontend-only evidence requires a connected browser during the collection
window. IndexedDB adds a bounded local persistence surface, WebSocket
request/HTTP chunk response adds a short-lived capture protocol, and bundles
need expiry and cleanup ownership. A combined ZIP may be partial, so consumers
must inspect the manifest before assuming both sources are present.

Some lower-priority logs can be deliberately omitted during sustained resource
pressure. The manifest exposes queue, persistence, and archive loss so support
does not mistake a best-effort bundle for complete evidence. Diagnostic bundle
requests can be temporarily busy or coalesced instead of competing with normal
product activity.

Fixed byte/profile caps mean a very large retained backend file or a fifth
browser profile may be represented only partially. Newest backend bytes are
preferred and the manifest records exact included ranges. ZIP payloads are
stored without CPU-heavy compression, trading a larger download for predictable
application performance.

Any signed-in user can still download install-wide backend diagnostics. This
preserves current behavior but carries broader sensitive content once browser
logs are included, so the page disclosure and identity-scoped frontend capture
are mandatory.

The System Logs page becomes a focused action rather than a live diagnostic
viewer. Mobile and desktop share the workflow, with a touch-sized primary
action and no wide table.

## Alternatives Considered

- **Continuously stream every frontend log to the backend.** Rejected because it
  uploads sensitive browser history without an explicit diagnostic request and
  adds persistent network/storage overhead.
- **Upload only the current 500-entry memory buffer.** Rejected because it
  cannot match the selected three-day diagnostic window across reloads.
- **Have the initiating frontend attach logs directly to the ZIP request.**
  Rejected in favor of a backend-requested capture protocol that also supports
  agent-triggered frontend bundles and multiple connected browser profiles.
- **Keep the backend ring buffer for the page and debug endpoint.** Rejected
  because the always-on file sink is now authoritative and duplicate memory
  retention creates inconsistent evidence windows.
- **Add a filtered JSON log-query endpoint for agents.** Rejected because agents
  can select a smaller source bundle, extract it, and use mature local search
  tools without maintaining a second parser/query contract.
- **Expose only one all-sources bundle.** Rejected because agents often need
  backend evidence only and should not trigger frontend capture unnecessarily.
- **Let agents call the HTTP API with a reusable user token.** Rejected because
  in-session MCP already resolves the task owner and can materialize an
  identity-scoped artifact without exposing a broad credential to the agent.
- **Make bundles admin-only.** Rejected to preserve the existing authenticated
  System Logs boundary selected for this feature.
- **Keep the tail viewer beside bundle download.** Rejected because it retains
  duplicate endpoints, state, and UI without adding evidence absent from the
  files.
- **Use local-time retention and rollover.** Rejected because UTC is
  deterministic across machines and daylight-saving transitions.
