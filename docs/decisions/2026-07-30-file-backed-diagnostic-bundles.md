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
the current plus two preceding UTC days. Each daily file has a fixed 256 MiB
write bound so sustained or hostile diagnostic traffic cannot consume
unbounded disk before midnight. Normal/debug/verbose file and stdout thresholds
remain defined by the diagnostic-logging spec. Destination and lumberjack
rotation settings are removed.

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
UI library and is never awaited. Automatic toast reports remove URL query and
fragment data and are guarded by both count and byte token buckets. Bundle
capture and archive construction have fixed per-job byte/profile limits plus a
bounded, coalescing background worker so one export cannot create unbounded
CPU, disk, memory, or connection work.

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

The standard bundle never reads persisted chat/session message tables or raw
agent protocol captures. Its UI names the frontend and backend event classes
and explains that this source boundary cannot remove incidental content already
emitted into a selected log. `manifest.json` remains mandatory. A separate
optional `runtime/` source provides a bounded authorized task/session index
(IDs, agent/provider/model, status, timestamps, and executor type) without
titles, prompts, responses, tool payloads, file content, identities, or stored
message bodies.

Raw ACP is a distinct debug-only, explicit-consent source. It is available only
when backend-authoritative ACP message capture is enabled, always requires the
user to select one or more sessions, and carries a stronger disclosure that raw
and normalized frames can contain prompts, responses, tool inputs/results,
workspace file content, MCP payloads, environment-derived values, and secrets.
Non-admin users may select only sessions on tasks they own; admins may select
any eligible session. Auth-disabled local installations use their synthetic
admin identity.

Kandev does not continuously centralize ACP frames. For selected sessions the
bundle worker reads retained host files or fetches a bounded export on demand
from reachable executor-side `agentctl` instances, revalidates every entry and
archive path, and records unavailable/stopped sessions as partial. The request
selects at most ten sessions, uses at most two concurrent executor fetches, and
has a 30-second ACP collection deadline. Standard jobs keep their existing
160 MiB backend/80 MiB frontend budgets. ACP-inclusive jobs reserve 96 MiB
backend, 48 MiB frontend, 96 MiB ACP, and 2 MiB runtime while retaining the
256 MiB uncompressed archive and 384 MiB temporary-disk bounds.

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

Raw ACP and the runtime index are intentionally not added to the task-mode MCP
or Improve Kandev source aliases. ACP selection remains a human-reviewed
download flow because its content and permission boundary differ materially
from the standard support bundle.

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

The fixed daily file bound limits worst-case disk growth but can omit later
same-day evidence after sustained pressure. Count and byte limits reduce how
quickly automatic browser reports consume that budget, while UTC rollover
restores capacity without operator action.

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
action and no wide table. Desktop customization uses a dialog; phone uses an
inset, safe-area-aware bottom drawer with one internal scroll owner. Debug mode
shows one **Customize bundle** action. It starts with backend and frontend
evidence selected, while the runtime index and ACP evidence remain explicit
opt-ins. Selecting ACP opens the required session selection in that same
customizer.

Raw ACP is intentionally more sensitive than the standard bundle. The UI and
API cannot truthfully promise that an ACP-inclusive archive excludes agent or
session messages. Maintainers gain exact wire evidence, but users must make a
session-scoped choice and review a stronger warning before collection.

On-demand executor reads can omit evidence from stopped, removed, unreachable,
or non-debug executors. This avoids continuous network/disk overhead and limits
backend custody of raw frames, at the cost of some historical completeness.

## Alternatives Considered

- **Continuously stream every frontend log to the backend.** Rejected because it
  uploads sensitive browser history without an explicit diagnostic request and
  adds persistent network/storage overhead.
- **Continuously centralize all debug ACP frames.** Rejected because raw frames
  can contain secrets and full work content, would add hot-path network/storage
  overhead, and would retain sensitive executor data without an explicit user
  request.
- **Put ACP in every debug-mode bundle automatically.** Rejected because the
  standard no-transcript disclosure would become misleading and a single click
  could export unrelated sessions. ACP requires explicit session selection.
- **Offer only normalized ACP events.** Rejected because maintainers sometimes
  need raw wire evidence to diagnose adapter normalization, while normalized
  events can still contain sensitive message/tool content. The UI therefore
  includes both behind the same strong warning.
- **Read only host-resident ACP files.** Rejected because common Docker and
  remote executors retain debug files inside their agentctl environment.
  Bounded on-demand export covers reachable selected sessions without making
  raw-frame streaming continuous.
- **Export task/session database records as a debug source.** Rejected in favor
  of a small allow-listed runtime index that excludes titles, message bodies,
  tool payloads, file content, identities, and arbitrary configuration.
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
- **Rely on three-day retention without a daily byte bound.** Rejected because
  retention limits age but a single current-day file can still exhaust disk.
- **Make the daily byte bound configurable.** Rejected to keep diagnostic
  resource behavior predictable and avoid restoring the removed rotation
  configuration surface.
