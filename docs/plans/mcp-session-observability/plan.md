---
spec: docs/specs/platform/requirements/mcp-session-observability.md
decision: docs/decisions/2026-07-30-session-owned-mcp-observability.md
created: 2026-07-30
status: draft
---

# Implementation Plan: Session MCP Attachment Observability

## Overview

Replace the toolbar's profile-derived MCP list with a versioned,
session-owned attachment report. Agentctl will emit honest configuration and
local-server observations through its existing agent event stream; lifecycle
and orchestrator will attach immutable Kandev ownership, persist a bounded
history, broadcast live updates, and hydrate reloads. The frontend will disclose
per-server state from that report in a neutral desktop popover or mobile drawer.

Release diagnostics remain structured and sanitized. Raw ACP frames stay
development-only, while endpoint tests and bounded agent output are explicit
user actions. `acpdbg` receives a sentinel MCP probe for integration work.

## Confirmed root causes and protocol limits

- `useSessionMcp` reads profile configuration and defaults to `["kandev"]`,
  including after a profile-config fetch failure. `McpIndicator` therefore
  labels intended configuration as active runtime state.
- Structured ACP sessions inject Kandev's HTTP and SSE variants, then filter
  by advertised or assumed transport support before sending `session/new` or
  `session/load`. The filter currently returns only the surviving servers, so
  its omission reasons are lost.
- Passthrough materialization is a separate path. If a passthrough agent has no
  `MCPStrategy`, `applyPassthroughMCP` silently returns without exposing any
  servers even when discovery says the agent supports MCP.
- ACP advertises MCP transport capabilities and accepts `mcpServers`, but does
  not return a portable connected-server list. A successful ACP session request
  proves delivery acceptance, not connection.
- Agentctl hosts Kandev's task MCP endpoint, and the existing `mcp-go` version
  exposes hooks for session registration, initialize, `tools/list`, tool calls,
  errors, and unregister. Those hooks can prove local endpoint behavior without
  raw frame logging.
- Third-party profile MCP servers connect directly to the agent, so Kandev
  cannot automatically observe their initialize or `tools/list`.
- Agent stderr is already retained in a bounded in-memory buffer and reachable
  through the internal `agent.stderr` stream request. It is not currently
  exposed as an authorized, on-demand user diagnostic.
- `acpdbg` sends an empty `mcpServers` array and its runner keeps the original
  empty `RunConfig.Workdir` after allocating a temporary child directory.
  Consequently `session/new` can receive an empty `cwd`, and the tool cannot
  test whether a real agent attaches to an injected MCP server.

---

## Status contract

### Attachment history

Add a versioned contract shared across the agentctl stream, lifecycle events,
persistence, boot hydration, and frontend state:

- `MCPAttachmentHistory`
  - schema version;
  - current `attachment_attempt_id`;
  - current attempt plus at most two previous attempts.
- `MCPAttachmentAttempt`
  - backend-owned `task_id`, `session_id`, `execution_id`, and
    `attachment_attempt_id`;
  - `agent_id`, `agent_profile_id`, and optional `acp_session_id`;
  - start, update, supersession, and session-acceptance times;
  - bounded evidence timeline;
  - report-level sanitized agent session error when it cannot be mapped to one
    server.
- `MCPServerAttachment`
  - name, source (`kandev` or `profile`), and transport;
  - sanitized target description;
  - strongest display status;
  - stable filter/failure reason code and bounded summary;
  - latest connection ID, tool count, and evidence timestamps where observable;
  - latest endpoint-test result, stored separately from attachment evidence.

Create a new attachment attempt before every ACP `session/new`,
`session/load`, or `session/reset` delivery and before every passthrough process
start. Publishing that attempt immediately supersedes the old current attempt,
so a failed or slow reset cannot retain Active status.

The display reducer follows the spec:

- `tools_list_observed` → Active;
- `initialize_observed` → Connected;
- delivered or accepted without observable connection → Delivered /
  connection unverified;
- explicit server-specific error → Failed;
- policy/capability/strategy omission → Filtered or Unavailable.

Tool use enriches Active state and does not define another toolbar color.
Disconnect remains visible in detail; a later connection ID can restore the
current connection evidence.

### Sanitization and bounds

- Keep at most three attempts and 64 evidence events per attempt.
- Store MCP names, stable reason codes, timestamps, counts, and ownership IDs.
- Reduce network targets with URL parsing to `scheme://host[:port]`, dropping
  user info, path, query, and fragment.
- Reduce stdio targets to `filepath.Base(command)`.
- Pass error summaries through the existing routing/error sanitizer, cap them
  at 256 bytes, and never fall back to raw structured objects.
- Never persist headers, environment values, command arguments, prompts,
  content, tool arguments/results, raw ACP frames, or agent stderr.
- On-demand stderr is capped at the smaller of 50 lines or 16 KiB, remains
  ephemeral, and is excluded from Copy sanitized diagnostics.

---

## Backend

### Agentctl evidence production

- Add typed MCP evidence fields and constants under
  `apps/backend/internal/agentctl/types/streams/`.
- Give `process.Manager` a non-blocking typed update publisher so API delivery
  code and MCP hooks use the same ordered `/agent/stream` channel as adapter
  events. Do not create a second event stream.
- Change ACP transport filtering to return both the selected server list and a
  decision per input server. Record agent-advertised versus assumed transport
  support and stable reason codes without storing secrets.
- In `handleWSNewSession`, `handleWSLoadSession`, and reset handling:
  - start and publish a new attempt before delivery;
  - publish configured/filter/delivered evidence;
  - publish session acceptance with the returned ACP session ID;
  - publish a sanitized report-level or server-specific explicit error when
    available.
- Install `mcp-go` hooks in the in-session Kandev MCP server. Attribute
  initialize, `tools/list`, call, error, register, and unregister events to the
  instance's task/session/execution/attempt ownership and the hook session's
  opaque connection ID.
- Ensure the server factory passes `InstanceConfig.InstanceID`, the status
  publisher, and the active attempt source into the MCP server. Tool handlers
  continue injecting task/session identity from server context.
- Update passthrough materialization to publish resolved, filtered,
  materialized/delivered, start-accepted, and explicit failure evidence. A nil
  strategy produces an honest unavailable reason instead of a silent no-op.

### Lifecycle, persistence, and live delivery

- Normalize the agentctl MCP event in lifecycle without treating it as prompt
  activity or chat content. Add the lifecycle payload and domain event subject.
- Stamp every accepted event with the live `AgentExecution` ownership. Reject
  or ignore evidence whose execution or attachment attempt is no longer
  current.
- Add `SessionMetaKeyMCPAttachmentHistory` and typed JSON rehydration helpers.
- In the orchestrator handler, reduce each event into the bounded history and
  write only that metadata key atomically. Persistence failures are logged and
  do not alter session state.
- Publish `session.mcp_status_updated` on the session-scoped event bus subject
  and register it with the WebSocket session broadcaster.
- Add task-detail boot state `mcpAttachments.bySessionId`, using the persisted
  history so reload does not wait for a live event.
- Add a frontend session-runtime slice, hydration merge, WebSocket handler, and
  tests. Removing a session clears its attachment report.

### Release diagnostic operations

- Add session-scoped WebSocket requests:
  - `session.mcp_test_endpoint` with `session_id` and `server_name`;
  - `session.mcp_recent_agent_output` with `session_id`.
- Keep the existing raw-gateway rejection of `mcp.*` actions. These new
  `session.*` operations pass through ordinary dispatcher registration and the
  gateway's session authorization backstop, plus handler-level authorization.
- Resolve the requested server name from the current persisted/effective
  attachment configuration. Reject unknown names; never accept a caller URL,
  command, headers, args, or environment.
- Execute the bounded initialize + `tools/list` test through the owning live
  agentctl instance so network and stdio checks run inside the session's
  executor. Close the temporary client/process on success, error, cancellation,
  and timeout.
- Store only the sanitized test result in the attachment report, explicitly
  classified as endpoint reachability rather than agent attachment.
- Reuse `GetAgentStderr` for the output request, cap it to 50 lines/16 KiB in
  the backend response, do not persist it, and return a clear unavailable
  response when the execution is not live.
- Reuse the existing reset-context recovery path in the UI. Do not add an
  automatic retry, process restart, or state transition based on missing
  evidence.

---

## Frontend

### Store-derived status

- Replace `useSessionMcp(agentProfileId)` with a session-ID-based hook that
  reads `mcpAttachments.bySessionId`. It may show a neutral loading or
  unavailable state while no report exists, but must not synthesize Kandev as
  Active from profile configuration.
- Keep configuration context in the status report itself. The component never
  fetches profile MCP JSON directly.
- Define a pure display reducer for row status, colors, checklist text,
  timestamp formatting, and copy-safe diagnostics. Unit-test unknown/newer
  reason codes with a conservative fallback.

### Responsive status disclosure

- Replace `McpIndicator` with a semantic button and shared
  `McpStatusContent`.
- **Desktop:** use a controlled Popover-style disclosure. Pointer hover and
  keyboard focus open it transiently; click pins it for actions; outside
  interaction, Escape, or a second trigger click closes it. Keep focus order
  and `aria-expanded` correct.
- **Touch/coarse pointer:** use `useTouchDrawer` and the shared inset Drawer
  treatment. A `.tap()` on a 44px trigger opens the bottom drawer; no
  hover-only behavior remains.
- Keep the plug icon neutral in every state. Put green/amber/red/gray only in
  the disclosed server rows.
- Show the current Kandev session, execution, and attempt in diagnostics, but
  keep opaque IDs secondary to server name and plain-language status.
- Provide the checklist and actions from the spec. Fetch recent output only
  after explicit activation and warn before rendering it. Copy diagnostics is
  built from the sanitized report and test result only.
- Link to the existing MCP settings surface. Show the existing Reset context
  flow only when its current eligibility rules allow it and retain its
  confirmation copy.
- Use the same component in structured and passthrough chat toolbars; status
  truth comes from the active session report.

### Mobile design contract

- **Desktop outcome:** a neutral toolbar button opens a compact hover/focus
  popover; clicking pins a scroll-bounded list with per-server status and
  diagnostics.
- **Mobile entry point:** a 44px neutral plug button in the existing
  horizontally scrollable composer toolbar.
- **Mobile outcome:** a safe-area-aware inset bottom drawer containing the same
  server list, checklist, and actions. Rows wrap long names and summaries, the
  drawer owns its internal scroll, and the document has no horizontal
  overflow.
- **Nearest shipped exemplar:** `useTouchDrawer` plus the Drawer/Popover split
  in GitHub task credential and PR status surfaces.
- **Shared logic:** one store selector, display reducer, status content
  component, diagnostic request layer, and copy serializer across desktop and
  touch.

---

## Developer probe

- Fix `acpdbg.NewRunner` to retain the resolved temporary working directory in
  `r.cfg.Workdir`, so ACP `cwd` matches the spawned child.
- Add an `mcp-probe` command that starts a temporary streamable-HTTP sentinel,
  injects it into `session/new`, waits within the existing command timeout, and
  summarizes:
  - advertised MCP transports;
  - server delivered;
  - initialize observed;
  - `tools/list` observed and count;
  - optional sentinel tool use when an explicit prompt is supplied.
- Give sentinel connections opaque IDs and write their lifecycle markers as
  JSONL metadata alongside the raw ACP frames.
- Keep `--exec`, registry command resolution, inherited/stripped environment,
  workdir, stderr capture, and timeout behavior aligned with ordinary probes.
- Update `cmd/acpdbg/README.md` and the `acp-debug` skill with exact syntax,
  output fields, interpretation, and the warning that no observation is not a
  portable agent failure.

---

## Tests

- **Wire contract and reducer:** event kinds, status precedence, attempt
  supersession, bounds, target sanitization, and typed JSON rehydration.
- **ACP delivery:** HTTP/SSE capability selection, filtered reasons, delivery
  events, session acceptance, explicit errors, and reset attempt rotation.
- **Kandev endpoint observation:** separate connections and attempts emit
  initialize, `tools/list`, tool-call, error, and unregister evidence with no
  tool payloads or agent-provided ownership IDs.
- **Passthrough:** strategies emit delivered evidence; nil strategy and
  materialization failures are unavailable/failed rather than active.
- **Persistence:** orchestrator rejects stale attempts, retains three attempts
  and 64 events each, persists sanitized data, broadcasts live state, and
  hydrates task detail after reload.
- **Diagnostics:** arbitrary targets are rejected, authorization runs before
  dependencies, network and stdio tests are bounded and cleaned up, successful
  tests do not promote attachment state, stderr is capped and never persisted.
- **Frontend state:** boot hydration, WS replacement, active-session
  separation, session removal, status reduction, diagnostic copy redaction,
  desktop disclosure state, keyboard operation, and touch Drawer selection.
- **acpdbg:** resolved temp cwd reaches `session/new`; fake ACP clients that
  connect, ignore, list, call, or error produce distinct summaries.

## E2E tests

- `apps/web/e2e/tests/chat/mcp-status.spec.ts`
  - create two sessions in one task with different attachment reports;
  - hover and keyboard-focus the neutral desktop trigger;
  - pin it by click and verify Active, Connected, Delivered/unverified,
    Failed, and Filtered rows;
  - switch session tabs and verify evidence never leaks between agents;
  - reload and verify persisted status hydrates;
  - run a mocked endpoint test, reveal bounded output, and confirm copied
    diagnostics omit that output and sensitive target parts.
- `apps/web/e2e/tests/chat/mobile-mcp-status.spec.ts`
  - tap the 44px trigger under `mobile-chrome`;
  - verify the inset Drawer contains the same rows and actions;
  - exercise one diagnostic action;
  - assert safe-area layout, internal scrolling, and no horizontal overflow.

E2E setup may seed bounded attachment metadata and diagnostic responses through
E2E-only helpers, but assertions must operate through the rendered UI. Backend
integration tests remain responsible for proving real MCP hook events.

---

## Public documentation

- Update `docs/public/agents-and-profiles.md` troubleshooting to distinguish
  configured, delivered, connected, and tools loaded.
- Update `docs/public/automation-and-mcp.md` with the toolbar diagnostic flow,
  third-party observability limit, endpoint-test interpretation, and release
  privacy boundary.
- Update `docs/public/add-agent-cli.md` with the sentinel `acpdbg` workflow and
  the requirement that new passthrough agents report an unavailable strategy
  honestly.
- Update `docs/public/feature-status.md` if its profile MCP observability row
  still describes only configuration.
- Update `apps/backend/cmd/acpdbg/README.md` and
  `.agents/skills/acp-debug/SKILL.md` with the developer command contract.

---

## Implementation waves and parallel candidates

Implementation remains in the user-started primary session unless the user
later authorizes native implementation agents.

Wave 1:

- [x] [Task 01: Establish the attachment evidence contract](task-01-attachment-evidence-contract.md)

Wave 2:

- [ ] [Task 02: Observe per-attempt MCP attachment](task-02-observe-mcp-attachment.md)
- [ ] [Task 06: Extend acpdbg sentinel probing](task-06-acpdbg-sentinel-probe.md)

Tasks 02 and 06 are parallel-safe if explicitly delegated later. Task 02 owns
agentctl API/adapter/process/MCP server and lifecycle passthrough files. Task 06
owns `internal/agent/acpdbg`, `cmd/acpdbg`, and its developer documentation.
Both consume Task 01's terminology but do not edit the same implementation
files.

Wave 3:

- [ ] [Task 03: Persist session attachment reports](task-03-persist-attachment-reports.md)

Wave 4:

- [ ] [Task 04: Add release diagnostic operations](task-04-release-diagnostic-operations.md)

Wave 5:

- [ ] [Task 05: Build the responsive MCP status surface](task-05-responsive-mcp-status-surface.md)

Wave 6:

- [ ] [Task 07: Prove responsive status flows](task-07-responsive-status-e2e.md)

Wave 7:

- [ ] [Task 08: Document release troubleshooting](task-08-release-troubleshooting-docs.md)

## Required final verification

After every task's targeted RED/GREEN evidence is recorded:

1. `make -C apps/backend test`
2. `cd apps && pnpm install --frozen-lockfile`
3. `cd apps/web && pnpm run typecheck`
4. `cd apps && pnpm --filter @kandev/web lint`
5. `cd apps && pnpm --filter @kandev/web e2e:run tests/chat/mcp-status.spec.ts -- --project=chromium`
6. `cd apps && pnpm --filter @kandev/web e2e:run tests/chat/mobile-mcp-status.spec.ts -- --project=mobile-chrome`
7. `node --test scripts/validate-public-docs.test.mjs`
8. `node scripts/validate-public-docs.mjs`

## Risks and non-goals

- Direct third-party MCP connectivity remains unobservable without a proxy or
  provider extension. The UI must preserve Delivered/unverified as a normal,
  honest state.
- Some agents attach lazily, so no timeout converts missing initialize into
  failure.
- MCP hooks can be concurrent. The reducer must be idempotent, preserve
  per-connection ordering where meaningful, and reject stale attempts.
- Status reporting must not block the agent update channel or MCP request path;
  a dropped live event can be logged, while later snapshots and persistence
  repair the current display.
- Endpoint tests can start stdio processes and make network requests, but only
  after explicit user action, only from stored effective configuration, and
  under strict timeout/cleanup.
- This plan does not proxy third-party MCP traffic, add a provider-specific ACP
  connected-server extension, enable release raw-frame logging, or
  automatically reset/restart sessions.
