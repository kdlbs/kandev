---
status: draft
system: agents
requirements:
  - REQ-AGENTS-BACKGROUND-WORK-001
  - REQ-AGENTS-BACKGROUND-WORK-002
  - REQ-AGENTS-BACKGROUND-WORK-003
  - REQ-AGENTS-BACKGROUND-WORK-004
  - REQ-AGENTS-BACKGROUND-WORK-005
  - REQ-AGENTS-BACKGROUND-WORK-006
---

# Agent Background Work System Design

## Ownership and existing boundaries

The agents system owns provider capability, workload identity, and normalized
observations. Follow [the normalization decision](../../../decisions/2026-09-27-provider-neutral-background-work.md).
The initial adapter is [native Codex](codex-app-server.md). Task persistence
stores the inspection projection; platform runtime transports it; React only
renders normalized state. New symbols below are proposed, not existing APIs.

Reuse these existing boundaries rather than replacing them:

- `agentctl/types/streams.BackgroundWorkPayload` is adapter-issued attestation,
  not a public UI model. Existing `shell`, `subagent`, and `monitor` kinds remain.
- `internal/orchestrator/background_work_attestation.go` and existing background
  accounting own admission effects. The new projection never becomes a second
  prompt-admission authority. Preserve [session activity](../../platform/system-design/background-work-liveness.md)
  and [queue admission](../../tasks/system-design/queue-admission.md).
- `task_session_subagents` records a tool invocation keyed by session, execution,
  and call. Its terminal launch-tool status is not proof a spawned child ended.
  Preserve [its history contract](subagent-context-persistence-01.md).
- Existing normalized tool messages, `ParentToolCallID`, subagent result text,
  and `tool-subagent-message.tsx` supply transcript content. A workload references
  these messages, rather than creating a second transcript writer.
- [Conversation usage](../../costs/system-design/conversation-usage.md) owns costs.
- `agentctl/server/shell` owns user terminal shells. Codex background commands
  remain owned by the Codex process; no attachment through ShellManager or host PID.

| Requirement | Design sections |
| --- | --- |
| REQ-AGENTS-BACKGROUND-WORK-001 | Capabilities and rollout; Adapter matrix |
| REQ-AGENTS-BACKGROUND-WORK-002 | Identity and persistence; Lifecycle and reconciliation |
| REQ-AGENTS-BACKGROUND-WORK-003 | Control routing; Identity and persistence |
| REQ-AGENTS-BACKGROUND-WORK-004 | Content and accounting |
| REQ-AGENTS-BACKGROUND-WORK-005 | Shared client and responsive UI |
| REQ-AGENTS-BACKGROUND-WORK-006 | Adapter matrix; Validation boundaries |

## Capabilities and rollout

Add optional `BackgroundWorkProvider` beside the existing optional adapter
capabilities. It accepts normalized snapshot/control requests and returns typed
observations/errors. Adapters without it continue to work. Existing normalized
observations may feed a read-only projection without implementing controls.
Do not widen every adapter's mandatory interface or add provider switches in
services, websocket handlers, or React. Provider-specific translation belongs
inside `adapter/transport/<protocol>/`.

Capabilities distinguish observation and action:

- Observation: discovery (`snapshot` or `events_only`), output (`stream`,
  `snapshot`, `none`), transcript, parentage, reasoning summary, attributable usage.
- Actions: `stop`, `interrupt`, `write_input`, `close_input`. Each action has
  support plus current availability/reason (`unsupported`, `disconnected`,
  `not_running`, `ownership_unknown`, `operation_pending`). The server intersects
  adapter capability, item capability, session permission, and runtime state.
- Missing fields default to unsupported. Unknown kind/state becomes a read-only
  unknown row, never a command. Unknown action identifiers are rejected.
  Other jobs remain accessible in the shared list without pretending to be shells.

Introduce `features.agentBackgroundWork` /
`KANDEV_FEATURES_AGENT_BACKGROUND_WORK`, restart-required, default false in prod,
dev, and e2e; targeted fixtures explicitly enable it. Wire config, the existing
runtime flag registry, profiles, and frontend defaults through the current typed
registry pattern. Do not copy an outdated config-switch pattern from a skill.
`features.codexAppServer` still controls native Codex startup; both are needed
for new native controls. The shared feature is independent of that gate for ACP.
Do not change the Claude background prompt-handoff flag or its admission policy.

Gate the new producer/projection subscription, reads, output subscriptions,
control dispatch, and UI at the backend composition boundary. Disabled requests
fail before provider RPC, new projection writes, or output capture; existing
messages, attestation, usage, and process cleanup continue unchanged. Do not add
an MCP or agent-tool control endpoint in this package. Migrations may install
empty schema while disabled; enabling does not backfill historical liveness.

## Identity and persistence

Introduce a task-owned inspection model `BackgroundWork` with opaque `work_id`,
`task_id`, `session_id`, session incarnation, `kind`, title, optional origin-turn
and source-message references, optional `parent_work_id`, and current `run_id`.
Run state includes `running`, `waiting`, `completed`, `failed`, `interrupted`,
`ended`, `unknown`; nullable started/finished times, optional exit code, capabilities,
monotonic revision, and evidence provenance. Waiting requires an explicit
provider wait/approval indication; silence is not waiting. Never replace missing
times, exit codes, or usage with zero.

Add `task_session_background_work` and `task_session_background_runs` through
existing task repository migrations, cascading on session deletion. A backend-only
binding records protocol namespace, provider scope/thread/work IDs, source
execution, current connection generation, source call IDs, and provider run ID.
Do not serialize these bindings as control addresses. The binding maps one child
thread to one workload and each child turn to a run; several collaboration calls
can reference the same child without inflating counts. Shell item/process
bindings include the owning provider thread and execution scope. A Kandev work ID
is not a native ID and `ChildSessionID` remains provider correlation only.

Use unique constraints on session/incarnation plus adapter-supplied stable scope
and workload key; run uniqueness adds the provider run key. Adapters without
stable cross-reconnect identity use execution-scoped keys and do not guess a
resume join. A verified stable binding may reattach to a replacement execution
while preserving the work/run IDs. Transport loss/replacement fences the old
connection before new controls are admitted. Keep immutable origin-turn ownership;
unknown origins stay unknown. Nested parent links must belong to the same session
and cannot form cycles. Buffer early unbound observations at the adapter (at most
256 per connection); overflow marks reconciliation needed instead of guessing.

Historical invocation rows remain unchanged and link via source call references.
The workload projection stores lifecycle, not a replacement set of subagent
metrics. Record this distinction in the subagent design. No migration derives
running state from old tool messages. SQLite and PostgreSQL migrations must be
idempotent and preserve existing messages/subagent rows; test real repositories.

## Lifecycle and reconciliation

A shared reducer in task service accepts typed observations routed through
`streams.AgentEvent`, lifecycle event transport, and orchestrator event handling.
Adapter normalization also retains current attestation semantics for the existing
accounting tracker; deriving a UI row never fabricates attestation. Reuse that
same observation's identity for both paths, with one application per consumer.

Each adapter observation carries immutable execution/connection generation,
work/run identity, and source event cursor. Deduplicate exact replay. Replayed
final items can fill missing content but cannot reopen a terminal run. A successor
run requires explicit new provider-run evidence; delayed events update only their
original run. Stop acknowledgement means pending, not completed. Root completion
neither closes the stream nor clears child work. Full runtime termination can
mark owned runs interrupted only after termination evidence. No projection event
publishes root completion, queues a run, or changes coarse session state.

Codex snapshot discovery follows every `nextCursor` before atomically applying a
complete snapshot. Serialize polls per connection, with a 2s interval and a
bounded 5s whole-poll deadline. Continue only while attached/reconciliation is
needed; cancel on close/replacement. A failed page, timeout, unsupported RPC, or
stale generation cannot remove entries. Fence snapshots against observations
received after the poll began. Only a successful complete live-only snapshot can
establish an earlier process is no longer running; record an ended run with
unknown outcome/exit status when the provider supplies no outcome, not success.
Represent this as terminal `ended` with `outcome_known=false` and render
"Ended (outcome unavailable)"; do not feed a fabricated success to accounting.

On backend start or transport loss, previously live inspection rows become
unknown with controls disabled. Resume queries only the owning session's provider,
reconciles stable bindings, and replaces capabilities for the new connection.
Event-only providers remain unknown until fresh evidence. Browser disconnection
alone does not imply provider disconnection. Session reset/deletion fences its
incarnation before clearing client state; late events cannot recreate it.

Persist row updates before emitting `task_session.background_work.updated` through
the existing authenticated session websocket delivery. Events carry full normalized
work/run projections plus revisions, not provider payloads. List/read responses
are paginated; per-work revisions and request-start epochs prevent delayed HTTP
snapshots overwriting newer events or dropping rows introduced during a fetch.
Use the existing session activity-epoch approach as a pattern. Keep completed
history pageable; do not clear the list when active count becomes zero.

## Control routing

Proposed public task-owned API, through existing task/session authorization:

- `GET /api/v1/task-sessions/:sessionId/background-work` (cursor/limit).
- `GET /api/v1/task-sessions/:sessionId/background-work/:workId/output` (run, cursor).
- `POST /api/v1/task-sessions/:sessionId/background-work/:workId/actions` with
  `action_id`, `run_id`, observed `revision`, action, and bounded input if applicable.

The action handler calls a task application service, then the optional agent
runtime capability, lifecycle owner, agentctl client, instance-scoped agentctl
handler, and adapter. Add methods through existing executor forwarding; the
browser never contacts remote agentctl or Codex directly. The runtime interface
uses normalized IDs/contracts; the adapter resolves native addresses.

Authorize session/task access before lookup. Resolve only bindings in that
session/incarnation and validate run, capability, revision, and active connection
again at dispatch. Compare control eligibility under a short lock and reserve
an operation token; do not hold locks over RPC. Require a provider operation
that addresses the exact run/process; otherwise the adapter must not advertise
the control. Codex child interrupt sends both exact thread and turn IDs. An old
request must not discover and interrupt a newer turn as a fallback.

Store action receipts keyed by session/incarnation and `action_id`, with request
fingerprint and pending/acknowledged/rejected/uncertain result, in
`task_session_background_actions`. Replays of the same ID return the receipt;
changed payloads conflict. Reserve before dispatch and reject stale targets.
On crash or transport loss after reservation, retain uncertain rather than
redispatch. This is at-most-once attempt behavior, not a guarantee of provider
execution. Use a 10s operation deadline; timeout fences its late response from
changing successor state. Reconcile liveness separately. Never automatically
retry stdin, restart the process, or translate a native process ID to a host PID.

Typed public outcomes: unsupported, forbidden, not_found, stale_target,
unavailable, operation_pending, uncertain. Do not leak provider errors or another
session's existence. Input is UTF-8 text, at most 16 KiB per action, translated by
the adapter to native bytes; close-input is separate. No signal picker, control
sequence buttons, or PTY emulation is promised. Approval/question handling reuses
existing request/thread/turn correlation and session authorization; expose only
references to that existing flow, never a second answer mechanism.

## Content and accounting

Add normalized workload-output observations for command output where the adapter
can prove ownership and delivery. Source identity includes work, run, stream, and
provider item/cursor; deduplicate streamed versus authoritative final snapshots.
If the provider has no replay-stable output cursor, an adapter-assigned sequence
is valid only within its connection. On resume, rebase from an authoritative
item/output snapshot or expose a gap; never append replayed deltas by guessing
that a new connection sequence denotes new content.
Persist bounded chunks in `task_session_background_output` with assigned offsets:
1 MiB retained per run, 16 MiB per session, 64 KiB maximum normalized chunk.
Evict oldest chunks transactionally, expose first/next offsets and truncation,
and cap per-subscriber buffering at 256 KiB. A slow consumer receives a gap/refetch
signal rather than blocking protocol lifecycle or accumulating unbounded memory.
Close subscription queues on session deletion and socket loss.

Normalize output deltas in the adapter, forward through lifecycle/orchestrator,
and publish `task_session.background_work.output` after persistence. A subscriber
starts with bounded history plus a cursor, deduplicates overlap, and refetches on
a gap. If only final output exists, label it snapshot output. Log timestamps are
observation times unless the provider supplied an actual timestamp. No new log
stream is inferred from terminal-list metadata.

Child messages continue through the existing normalized message writer, with
work/run references added for selection. Reuse existing nested tool/message
renderers and result text. Retain raw private reasoning as unavailable; render
only exposed normalized summaries. Existing transcript retention is unaffected
by bounded command-log eviction. Local Clear advances a viewer cursor, with a
Reset view action to return to retained history; no destructive API is added.
ANSI renderer accepts formatting only, not executable HTML, terminal title or
clipboard escape side effects. Reuse `shell-output-disclosure.tsx` patterns;
do not instantiate an interactive xterm session for a log-only stream.

Usage is a read-only projection from the existing ledger keyed by verified child
thread/turn bindings. Extend the task usage query to filter those bindings when
needed. Do not sum cumulative snapshots or add child totals to an already
inclusive parent total. Preserve estimate/completeness and cost provenance;
unattributed or unavailable cost remains unavailable, never $0. A token stream
is not a committed billable observation. The usage writer remains unchanged
unless a focused attribution regression proves a necessary correction.

## Adapter matrix

Schema baseline: `pkg/codexappserver/schema/v0.154.0/` in PR #3916 at
`b3ead0d0376317215ddcadc48b316b3743782744`. Recheck exact branch/head before
implementation. [Official app-server docs](https://learn.chatgpt.com/docs/app-server#clean-background-terminals)
corroborate discovery/termination, but the pinned schema and reproducible fixtures
are the implementation authority. `analysis.md` is research context, not evidence.

| Adapter | Initial behavior | Evidence and fallback |
| --- | --- | --- |
| Codex app-server | Typed paginated background list; native targeted terminate; child run events/interrupt; normalized existing child content | Schema and fake RPC round trips required; separately record live observations. Experimental RPCs require initialize experimentalApi. Unknown/unsupported methods disable only their capability. |
| Codex command output | Handle owned `item/commandExecution/outputDelta` where available; reconcile final `aggregatedOutput` | Test stream/final overlap. Missing stream falls back to labelled snapshot output. Discovery alone cannot provide history. |
| Codex background stdin | Not advertised initially | `command/exec/write` targets standalone client-created commands, not proven thread-owned background commands. No SIGINT/SIGTERM/SIGKILL selection or restart fallback. |
| ACP | Existing normalized subagent and detached-shell observations feed shared inspection | Existing adapter-specific recognizers retain authority. No standard ACP list/stop/write assumption; default actions unsupported. Test observation-only and unavailable data. |
| Future Claude native | Same normalized contract required | Adapter implementation deferred. No fabricated capability or protocol-name switch now. |
| Conformance fixture | All normalized capabilities, selective absence, unknown/reordered events | Proves shared routes/UI without claiming real-provider support. |

Do not blindly inject `features.collab`, `features.multi_agent`, or
`features.background_terminals`. Inspect `experimentalFeature/list`, selected
version config schema, current effective configuration, and test actual behavior.
Protocol experimentalApi opt-in is distinct from model feature enablement.
Only explicitly needed, verified per-session overrides may be sent on start,
resume, and fork; preserve operator restrictions and inherited session policy.
If a capability remains unproven, keep it unavailable and record the limitation.

## Shared client and responsive UI

Main-conversation Send now / Queue for later follows the separate
[explicit steering design](../../platform/system-design/explicit-turn-steering.md)
in this implementation package. Workload visibility and detail tabs do not grant
that capability. Child-agent prompting remains out of scope.

Add normalized Go/TS DTOs and additive session events; proposed
`useBackgroundWork(sessionId)` selects a store slice and uses a domain API client.
Do not assume `useTaskBackgroundWorkState()` exists. Stores are session/incarnation
scoped, merge by revision, clear on deletion, and share state across Task chat,
Quick Chat where its composer is shared, and desktop/phone presentation. Render
capabilities, never `agentId === ...` or native RPC names. Integration must not
mount a new background feature in passthrough-only terminal mode.

Render a small content-width pill directly above the chat input, aligned with
the input's leading edge: `( 2 Background Jobs )`. It is not a full-width banner
or an additional toolbar. The pluralized count includes unique running, waiting,
and unknown current runs; completed runs are excluded. Keep status breakdowns
in the opened summary instead of expanding the pill. Its accessible name can
include unknown/waiting counts. A compact visual pill retains a 44px touch hit
area on phone/coarse pointers. The history action remains reachable when it hides.

Click/Enter/Space opens a desktop Popover listing jobs and agents with status,
elapsed time, and available quick controls. Row selection opens details directly;
View all opens the separate Background Work overview panel. Do not require hover or an intermediate
list selection to inspect a job. Close the popover after selection.

Desktop uses two first-class Dockview panel kinds, initially in `centerGroupId`
beside the existing chat/session tabs:

- `background-work`: one overview per owning session/incarnation, titled
  Background Work, containing Commands, Agents, Other jobs and History filters.
- `background-work-detail`: one dedicated detail tab per
  `(session_id, incarnation, work_id)`, titled with that workload's display name,
  such as Build Watcher or Review Auth. Selecting a row opens or activates this
  tab directly; it does not require opening the overview first. Distinct jobs
  and agents stay open simultaneously, with independent scroll, search, local
  clear, input draft, and inspected-run selection. A successor run does not create
  another tab or silently retarget a control for an explicitly inspected old run.

Use canonical panel-ID helpers with encoded opaque identity components, not the
workload title or an ambiguous string concatenation. Duplicate names do not
merge tabs. Repeated selection focuses the same workload's existing tab without
resetting its view state or moving it from a user's manually chosen location.
New detail tabs do not overwrite another workload's panel. The overview and
detail tabs can be closed independently; closing either only disposes its view
subscription and does not stop work, send input, or close another tab. Opening a
tab never creates a Kandev task, session, execution, or user shell. Returning to
Chat preserves its draft. Completed jobs remain inspectable in open tabs and
can be reopened from History. All work inside a detail tab opens/activates the
separate overview without closing the detail.

Follow `dockview-extra-panel-actions.ts` and `addPromptHistoryPanel` as patterns
for explicit center-targeted open/activate actions. Register both panel renderers in
`dockview-shared.tsx` and `dockview-panel-content.tsx`, and extend panel type/title,
serialization and environment reconciliation consistently. Store only opaque
session/incarnation/work identity and optional inspected-run selection in panel params; after restoration revalidate
ownership and show unknown/loading until refreshed. Remove inaccessible, deleted,
or disabled-feature panels without dispatching controls. Do not fall back to an
unrelated active session. Quick Chat details navigate to the owning task workbench
and open the same workload-specific detail tab (or overview for View all); preserve its draft and do not invent a
second desktop sheet for this entry point.

Stop/Interrupt appear only when supported; disconnected supported actions are
disabled with a localized reason. Only show stdin when write_input is advertised.
No Restart, Re-run, or Steer controls. Parent/child rows share transcript identity;
these panels are views of shared data, not additional message writers. Existing inline tool cards remain.

Phone: chip tap opens an inset summary Drawer using `useTouchDrawer`; View work
transitions to one full-height work surface (close the summary first). Tapping
a specific summary row opens that workload detail directly; View all opens the
list. List and
detail are successive views with Back, not stacked overlays or split panes.
Preserve per-workload view state when moving between phone details. Do not render
a desktop tab strip on phone or overwrite the saved desktop tab layout.
Use `useResponsiveBreakpoint`, `h-dvh`, safe-area padding, and a fixed header
with one `min-h-0 flex-1` scrolling content body. Keyboard-visible stdin remains
reachable. Reuse `mobile-picker-sheet.tsx` for summary anatomy and
`conversation-usage-display.tsx` for responsive disclosure; inspect live APIs.
28px ordinary desktop controls, at least 44px phone/coarse-pointer targets,
including narrow fine-pointer phones. Focus returns to the invoker on Close;
Back returns to the selected list row. Typing/draft and existing send/queue/steer
logic are unaffected by chip updates.

Localize all copy and accessibility names in six languages plus pseudo; generate
Traditional Chinese via the existing conversion command. Empty/loading/error,
retained history, unsupported stream/input, reconnect unknown, uncertain action,
and usage unavailable are explicit rendered states. See the [plan previews](../../../plans/agent-background-work/plan.md#ascii-ui-preview).

## Observability and validation boundaries

Use `background_work_observations_total` with closed outcome labels
(applied, duplicate, stale, unknown_owner, failed),
`background_work_actions_total` with action and result enums, and
`background_work_reconcile_total` with success/partial/unavailable/stale outcomes.
Do not label metrics with session/work IDs, commands, paths, output, stdin, or
raw errors. Structured diagnostics may include opaque correlation IDs but never
input/output or credentials. Test representative error and success outcomes.

Contract tests cover adapter capability absence, normalized content and ownership,
backend migration/replay, session-auth boundaries, generation races, bounded logs,
receipt uncertainty, and old-peer compatibility. Browser tests cover common UI
with Codex fixtures and ACP observations, not only fabricated Zustand state.
Run executor fake-server fixtures through local/Docker/SSH/Kind; live provider
checks are separately labelled and never inferred from fake success. Current
native-Codex and companion follow-up results remain historical evidence; this
package requires its own exact-head results.

## Implementation package

[Plan and work orders](../../../plans/agent-background-work/plan.md).
