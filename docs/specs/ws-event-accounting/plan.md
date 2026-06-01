# WS event accounting — Phase 2 plan

## Release strategy

This PR (#1130) does NOT merge until **`KANDEV_E2E_WS_ASSERT=1` is the
default in CI and the full e2e suite is green with it on.** That is the
single acceptance criterion — not a percentage coverage target, not a
metrics-dashboard handoff, not "most flakes resolved." Strict assertion
on, suite green, merge.

Implication for Workstream 2 (production observability): **deferred to
a post-merge follow-up project.** Ack channel + Prometheus metrics +
`ws_clients_ack_stale_total` are valuable but they are production
telemetry, not test confidence. They do not move the merge needle and
should not block it. The workstream is documented below for the
follow-up implementer; do NOT pull it into the merge scope.

Implementation order for the merge path is therefore:

1. **Workstream 0** — bridge audit (`wrapBridgeHandler` +
   `BRIDGE_SKIPPED_ACTIONS`). Ships first because it gives us the debug
   tool to triage handler-side drops (the bug class the user has
   already observed: "new task → env prep → agent messages don't
   appear without refresh").
2. **Workstream 1** — per-session sequencing. Closes the cross-session
   misrouting gap that per-connection seq can't see.
3. **Workstream 3** — flip `KANDEV_E2E_WS_ASSERT=1`, run the full e2e
   suite, triage every drop, fix the underlying bugs, audit-revert the
   band-aids on main. This is where "all tests improved" actually
   happens.

Workstream 2 stays in this document as the production-observability
follow-up but is OUT OF SCOPE for the merge.

---

## Known regression case: agent messages don't appear without refresh

User-observed bug, mapped end-to-end by trace investigation
(Workstream 0 commit). Use it as a smoke test for whether Phase 2
actually delivers the reliability guarantee.

**Symptom:** Create a new task. Wait for env preparation. Agent
produces messages. UI shows the message list as empty. Hard refresh
the page → all messages appear.

**Root cause class (named here so future bugs of the same shape are
recognizable):** "Bridge wrote, UI doesn't read." The bridge handler
for `session.message.added` correctly writes to TQ at
`qk.session.messages(sid)`. The Zustand handler in
`lib/ws/handlers/messages.ts` also runs and writes to
`messages.bySession[sessionId]`. The message-list components
(`components/task/chat/message-list-native.tsx`, via
`use-lazy-load-messages` and `use-session-messages`) only read from
Zustand — they don't read TQ. Whichever Zustand path fails to populate
(race with `fetchAndStoreMessages`, sessionId set after events arrive,
slice not yet reactive) results in empty UI.

**What Workstream 0's bridge audit detects:** The bridge handler ran
and `cacheChanged === true`, so by Workstream 0's metric the event was
"applied." This is correctly true at the TQ level, but the UI doesn't
benefit. **Bridge audit catches one half of "applied"; the other half
requires the UI to actually subscribe.**

**Implication for the merge criterion:** "Strict assert green in CI"
is not sufficient on its own for the messages domain. Add to the
Workstream 3 acceptance bar: **the message-list component must read
from `useQuery(queryOptions.session.messages(sid))`** before this
branch merges. That's a one-component migration on top of what already
exists in `lib/query/query-options/session.ts`. Once the UI reads from
TQ, the bridge audit becomes a tight invariant for this domain too.

**Other domains likely affected by the same pattern:** Audit every
`lib/query/bridge/<domain>.ts` against `hooks/domains/<domain>/` and
the components those hooks feed. Anywhere the bridge writes to a TQ
key but the UI still reads from Zustand is a latent version of this
bug. Track in `audit.md` (gitignored).

**Audit findings (from a 2026-05-28 pass):**

- **Migrated cleanly:** workspace, settings, github, gitlab, automations,
  jira, linear, integrations, session-runtime, session-runtime-streams,
  features, comments.
- **Session messages:** FIXED in this branch. The chat message list now
  reads via `useQuery(sessionMessagesQueryOptions(sid))` and the old
  `fetchAndStoreMessages` race source is deleted. 8 other
  `messages.bySession` consumers remain (badges, counters, dialog
  pre-flight checks) — left intact; they're served by the Zustand
  transitional mirror, not the failing-to-update display path.
- **Kanban — partial.** ~30 files read `state.kanban.tasks/workflowId/
  steps`. Bridge writes to `qk.kanban.multi()` but UI doesn't read it.
  Must migrate before merge (Workstream 3 acceptance bar).
- **Office tasks list — partial, harder than it looked.** The
  `usePaginatedTasks` hook does keyset pagination via REST; a naive
  switch to `useQuery(officeQueryOptions.tasks(wsId, filters))` loses
  pagination because that factory fetches a single page. Proper fix
  needs `useInfiniteQuery` AND bridge invalidation logic against the
  paginated cache key. An earlier attempt swapped Zustand reads for
  local `useReducer` state and inadvertently dropped the WS-update path
  entirely (the Zustand handler still writes, but neither the bridge
  nor the local state surfaces those writes to the UI). Reverted. To
  fix properly: (a) add a paginated `useInfiniteQuery` factory in
  `query-options/office.ts`, (b) make the bridge handler invalidate
  every page of that key on `task.updated` / `task.created`, (c) migrate
  the consumer hook. Roll this into the kanban migration phase since
  the patterns are similar.
- **Office `sidebar-agents-list.tsx`** — turned out to be already
  migrated. The audit's claim of "still reads Zustand" was inaccurate;
  the Zustand reads in that file are for `workspaces.activeId` and
  unrelated session-state counters, not for office agents.

---

## Branch context

This branch is rebased on top of the **TanStack Query migration**
(formerly PR #1052, the squashed commit `b95b168b refactor(web): migrate
server state from Zustand to TanStack Query (waves 0–5)`). The intent
is to ship the migration and the WS reliability work as one cohesive
unit — the migration without accounting introduces a parallel write
path that can silently desync from the source of truth; the accounting
without the migration leaves the FE on a Zustand model we're trying to
retire. Together they deliver "every server-state event the backend
emits is observably received, parsed, and applied to the FE cache."

PR #1052 is being superseded; do NOT plan around it merging
independently. Anything below that previously read "after #1052 merges"
is now "already on this branch."

**Phase 1** (already on this branch, see commit log) ships per-connection
sequence stamping, a backend ring buffer, an FE receive-side ring buffer,
and a fixture hook that computes drops after every test. With
`KANDEV_E2E_WS_ASSERT=1` an event the backend sent but the FE never
processed becomes a test failure naming the dropped event.

Phase 2 closes the gaps Phase 1 left open, instruments the new
TQ-bridge layer so handler-side drops are also catchable, and adds the
production observability needed to act on drops outside of CI. Three
workstreams, listed in priority order — they're independent enough to
parallelize once the per-session work lands, since the ack channel and
the audit/migration both depend on per-session as their unit of truth.

---

## Workstream 1 — Per-session sequencing

### Why

Phase 1's `seq` is per-WS-connection. That catches "the FE never saw
event N" but NOT "an event for session A was routed to session B's
handler with the right seq." Cross-session routing bugs would still
pass strict-mode accounting today.

The Office routing, the kanban All-Workflows view, and the sidebar's
per-session badge counters all subscribe to multiple sessions
concurrently — those are the surfaces where a misrouted event would
silently corrupt UI state instead of throwing.

### What changes

**Backend** (`apps/backend/internal/gateway/websocket/`):

1. Envelope gains `session_seq int64` and keeps the existing `seq`
   (rename it `connection_seq` for clarity in the JSON tag, but keep
   `Seq` field name in the Go struct to avoid churn). `session_seq` is
   stamped from a per-session atomic counter held inside `Hub.sessionSeqs
   map[string]*atomic.Int64`. Counter created lazily on first send.
   The Go struct is in `apps/backend/pkg/websocket/message.go`; the JSON
   tag rename is the only wire-format change. Grep `Seq` in
   `pkg/websocket/` and `internal/gateway/websocket/` to confirm no
   external consumer reads the field by name (CLI and agentctl use ACP,
   not this envelope, but verify).
2. `BroadcastToSession` / `BroadcastToTask` / `BroadcastToRun` paths
   need to know the session ID at stamp time. Today they already do —
   that's the routing key. Thread it through `stampAndMarshal` so the
   per-client clone gets both per-connection AND per-session seq.
3. `wsSentLog` records both seqs. The endpoint takes a new optional
   `session_id` query param; when set, returns only events for that
   session sorted by `session_seq`.
4. **Counter lifecycle**. `Hub.sessionSeqs` will leak if entries are
   never removed — long-lived hubs accumulate one `*atomic.Int64` per
   session ever seen. Tie counter removal to the existing session
   unsubscribe path: when the last client unsubscribes from a session,
   delete the counter entry. Add a Hub-level test that 1000 subscribe/
   unsubscribe cycles leave `len(sessionSeqs) == 0`. Same lifecycle
   discipline applies to `Client.lastAckedSessionSeq` in Workstream 2.

**FE** (`apps/web/lib/ws/`):

1. `WsAccount` becomes session-aware: replace the single buffer with
   `Map<sessionId, RingBuffer>` PLUS the existing connection-wide buffer.
   A non-session event (no `session_id` on the envelope — handshake,
   global notifications) goes into the connection bucket only. A
   session-scoped event goes into BOTH the connection bucket AND its
   session bucket. The connection bucket stays as the safety net that
   catches drops of non-routed events; per-session is additive and
   catches misrouting that the connection bucket cannot see.
2. `detectGaps` runs per session AND per connection. The window snapshot
   becomes `{connectionId, bySession: Record<sessionId, {processedSeqs,
   gaps, maxSeq, minSeq}>}` — backward-shape-compatible by keeping the
   top-level fields populated from the connection bucket.

**Helper** (`apps/web/e2e/helpers/ws-account.ts`):

1. `computeWsDrops` takes an optional `sessionId` filter. With no
   filter, diff every session.
2. `formatDroppedEvents` groups output by session.

**Tests**:

- New backend test: `Hub.BroadcastToSession` to two sessions produces
  independent `session_seq` streams; a single client subscribed to both
  sees interleaved per-connection seqs but a monotonic per-session seq
  stream for each.
- New FE test: when a session is added then removed, its ring buffer
  is dropped (no zombie buffers accumulating across long sessions).
- New e2e test: `tests/system/ws-event-accounting.spec.ts` — opens two
  task sessions, drives events on both, asserts no gaps in either's
  per-session stream.

### What it catches

- Cross-session routing bugs (events for A delivered to B).
- Session subscription gaps after reconnect (a session that was
  subscribed pre-reconnect loses events between reconnect and re-subscribe).
- Sidebar/kanban state corruption from misrouted events.

### Out of scope

- Reordering detection (events arriving out of order with the right
  seqs). Possible Phase 3.
- Late-arrival detection (event arrives after the test asserts). Same.

---

## Workstream 2 — Ack channel + production observability

### Why

Phase 1 detects drops in tests. Production drops are invisible — a
real user's sidebar quietly stops updating and we never know. Phase 2
adds a live feedback loop so:

- The backend knows what the FE has acknowledged. If acks lag by N
  events, that's a backpressure signal.
- A Prometheus counter `ws_events_dropped_total{type, session_id_hashed}`
  exposes the drop rate as a real metric. Alerting becomes possible.

### What changes

**Wire format**: add a new envelope type `ws.ack` (FE → backend):

```json
{
  "type": "request",
  "action": "ws.ack",
  "payload": {
    "connection_seq": 42,
    "session_acks": { "sess-1": 7, "sess-2": 12 }
  }
}
```

The `action` field uses the existing request/response dispatch — confirm
by greping `internal/gateway/websocket/` for the request action router
and register `ws.ack` as a new handler in the same place.

FE sends this opportunistically:
- After processing N events (default 50) since last ack, or
- After T ms with no activity (default 5s), or
- On `visibilitychange` to hidden — **best-effort only**. A normal WS
  `send` during `pagehide` is frequently dropped by the browser. Either
  accept that hidden-tab acks may be lost (audit must not attribute the
  resulting lag as a drop — see backpressure bound below), or switch the
  flush path to `navigator.sendBeacon` against a small HTTP endpoint
  (different transport, extra complexity, decide before implementing).

**Backend**:

1. New handler in `gateway/websocket/hub.go`: `handleAck(client, payload)`
   updates `Client.lastAckedConnectionSeq` and `Client.lastAckedSessionSeq[sessionId]`.
2. Background goroutine (one per Hub, not per Client) runs every
   `KANDEV_WS_DROP_AUDIT_INTERVAL` (default 30s). For each client, diff
   `sentLog.Max() - lastAckedConnectionSeq`. If > threshold (default
   100), increment `ws_events_dropped_total` with the (estimated)
   missing events. Same per session.
3. **Bound the attribution window**. If a client never acks (paused tab,
   buggy build, network black hole), `sentLog.Max() - lastAckedConnectionSeq`
   grows unbounded but the ring buffer has already evicted the old
   entries — so the goroutine keeps incrementing drop counters for
   events that were actually delivered. Cap the per-iteration drop
   attribution to the ring-buffer window size, and treat any client with
   `lag > ring_buffer_size` as "ack-stale": stop attributing drops to it
   and emit a separate `ws_clients_ack_stale_total` counter so we can
   distinguish "real drop" from "client gave up acking".
4. Hub exposes `Hub.DropCounters()` for the existing observability
   surface (`/debug/vars` in dev). Same pattern as the office routing
   counters.

**FE**:

1. New module `apps/web/lib/ws/ws-account-acker.ts` owns the FE → BE
   ack scheduling. Hooks `WsAccount.onRecord` (new lightweight callback)
   to count post-last-ack events.
2. `client.ts` wires the acker on connect, tears it down on disconnect.

**Metrics**:

1. `ws_events_dropped_total{type="<event_type>", session_bucket="..."}` —
   counter. Type comes from the dropped envelope's action.
   `session_bucket` is `fnv32(session_id) % 64` (NOT the raw or hashed
   session ID) — bounded cardinality is non-negotiable on Prometheus;
   raw or full-hash session IDs would create one time series per task
   ever opened. 64 buckets is enough to see distribution skew without
   exploding the metric.
2. `ws_events_acked_total{session_bucket="..."}` — counter, same
   bucketing.
3. `ws_ack_lag_seconds` — histogram of (now - sent_at) for the highest-
   acked event in each ack message. No session label.
4. `ws_clients_ack_stale_total` — counter, no labels. Increments when a
   client crosses the ring-buffer-size lag threshold (see backend
   change 3 above).

**Tests**:

- Backend: an ack with `connection_seq` ahead of `sentLog.Max()` is
  rejected with a log warning (catches buggy FE that fakes acks).
- Backend: drop counter increments when the ack lag exceeds threshold.
- FE: acker batches events; sends an ack at N=50 OR after T=5s
  whichever first; flushes on disconnect.
- E2E: kill the WS server mid-test, restart it, verify acks resume from
  the new connection without falsely attributing the disconnect window
  as drops.

### Out of scope

- Replay-from-last-ack (backend re-sending missed events on reconnect).
  That's a much bigger reliability project — for now we surface the
  drop and let the existing reconnect path re-fetch state.
- Per-event ack (ack each individually). Wastes bandwidth for typical
  agent streams that produce 100+ events/sec.

---

## Workstream 3 — Migration and audit

### Why

Phase 1's strict assertion is off by default to keep the suite green
during rollout. Now we have to actually flip it everywhere AND audit
the band-aid fixes that were papering over the same drops the
accounting will now catch.

### What changes

1. **Flip the assertion on**. CI `e2e-tests.yml` Build job already sets
   `NEXT_PUBLIC_KANDEV_E2E_MOCK=true`. The e2e shard job needs
   `KANDEV_E2E_WS_ASSERT=1` added to its env block.

2. **Triage the failures**. Start with one shard, observe the
   `[ws-account]` warnings in CI logs, group failures by event type
   and session-state preconditions. Catalog every drop in
   `docs/specs/ws-event-accounting/audit.md`. **Add `audit.md` to
   `.gitignore` first** — only `docs/specs/*/plan.md` is currently
   ignored; `audit.md` is a working scratchpad and should not be
   committed. Add the pattern `docs/specs/*/audit.md` alongside the
   existing `plan.md` line.

3. **Fix or document each drop**:
   - If a drop is reproducible → backend or FE bug. Fix it.
   - If a drop only happens in a specific WS-disconnect race → ensure
     the reconnect path actually re-fetches the dropped state, and add
     a regression test.
   - If a drop is a known limitation (e.g., events emitted between
     `clearWsAccount` and the page navigation completing) → document
     it in the helper and skip via a per-spec opt-out.

   **2026-05-29 allowlist expansion:** the first strict-mode rehearsal
   surfaced a ~30-action `no-bridge-entry` background — none of them
   real drops. They split into three categories, all now allowlisted in
   `BRIDGE_SKIPPED_ACTIONS` / `BRIDGE_SKIPPED_PREFIXES`:
     - **Zustand-only notifications.** Existing entries
       (`session.state_changed`, `message.queue.status_changed`) plus
       `session.waiting_for_input` (toast trigger only), `input.requested`
       and `permission.requested` (UI event-bus consumers without a TQ
       cache key), and the `session.agentctl_*` family (handshake status
       drives the agent status badge via Zustand — the old `agentctl_`
       prefix did not match these because they're under `session.`, so a
       dedicated `session.agentctl_` prefix was added).
     - **Control-plane subscription / focus acks.** `session.subscribe`,
       `session.unsubscribe`, `session.focus`, `session.unfocus`,
       `task.subscribe`, `task.unsubscribe`, `run.subscribe`,
       `run.unsubscribe`, `user.subscribe`, `user.unsubscribe`. The
       backend echoes the request action on every response envelope and
       stamps it with a per-connection seq, so the receipt enters
       `WsAccount.receivedEvents` — but responses dispatch via
       `pendingRequests` resolve/reject (`type === "response"`), never
       `ws.on()`, so no bridge handler can fire. Any session-scoped ack
       therefore reads as `no-bridge-entry` until allowlisted. Same shape
       applies to every other request/response action with a session_id
       in payload, which the allowlist now also covers: session lifecycle
       (`session.launch / .ensure / .recover / .stop / .delete /
       .reset_context / .set_primary / .set_plan_mode / .set_mode`),
       task-session polling (`task.session.status / .list / task.session`),
       agent operations (`agent.prompt / .cancel / .stop / .logs / .status
       / .stdin / .resize`, `permission.respond`), message-queue mutations
       (`message.queue.add / .cancel / .get / .update / .append / .remove`),
       task-plan requests (`task.plan.get / .create / .update / .delete /
       .revert / .revisions.list / .revision.get` — the corresponding
       `task.plan.created / .updated / .deleted / .revision.created /
       .reverted` notifications stay bridged), session git queries
       (`session.git.snapshots / .commits`, `session.cumulative_diff`,
       `session.commit_diff`), session file-review queries
       (`session.file_review.get / .update / .reset`), shell control
       (`session.shell.status`, `shell.subscribe`, `shell.input`), the
       `vscode.*` and `user_shell.*` operation families (the latter via a
       `user_shell.` prefix).
     - Long-term, the receipt-level audit could exclude responses
       structurally by carrying `envelope.type` into
       `WsAccountReceivedEvent` and skipping anything with
       `type === "response"`. That's a bigger surface change and out of
       scope here; the static allowlist is the smaller fix and the entries
       are documented inline in `lib/query/bridge/index.ts` so removing
       one when a future bridge wave claims the action is a one-line
       change.

4. **Audit-revert the band-aid fixes from PR #1126**. These commits are
   merged to main, not on this branch — find them with
   `git log main --oneline --grep="waitForChatIdle\|useResyncOnTabActivate\|plan-tab-indicator"`
   and `git log main --oneline -- apps/web/e2e/`. For each, decide
   whether Phase 2 made the band-aid unnecessary:
   - `waitForChatIdle` reload-and-retry helper — see if the underlying
     idle WS event is now actually reliable. If yes, drop the reload
     branch.
   - `FileEditorPanel` tab-activation re-sync (the `useResyncOnTabActivate`
     hook) — same question for the gitStatus signature change WS event.
   - `plan-tab-indicator` 5s → 15s timeout bump — was this masking a
     plan-update WS drop? If yes, fix the WS path and revert the
     timeout.
   - The 6 `idleInput → waitForChatIdle` seed-helper swaps — same
     question per spec.

5. **Update CLAUDE.md / AGENTS.md** with the accounting expectation.
   New WS message types should always be tested under strict mode; new
   sessions should be subscribed via the existing fixture so the
   accounting tracks them automatically.

### Docker e2e flake sweep (2026-05-29, post-rebase onto main)

Rebased onto latest main (25 commits, clean). Ran the full chromium suite
in the CI runtime image (`kandev-ci:runtime-local`, backend built in
`build-latest` for matching glibc), sharded 5× with `workers=1` per shard,
strict mode on — a faithful CI mirror, fully isolated from the host dev
instance. Result: **all shards green** (984 passed), zero WS-accounting
drops, with a tail of retry-flakes.

Classified every flaky test by re-running each in a **fresh, isolated
container** (no cross-shard contention):

- **`diff-update.spec.ts:174` — intrinsic bug, FIXED.** The diff panel
  rendered uncommitted-file content from the Zustand `gitStatus` mirror,
  which only refreshes on a `session.git.event` from agentctl's workspace
  git poll (3s fast / 30s slow). On the cold-start race where focus→fast
  is lost, a post-turn file change left the diff stale up to 30s. The
  editor panel already mitigated this (`useResyncOnTabActivate`); the diff
  panel had no equivalent. Added `client.refreshSessionData()` +
  `useResyncGitStatusOnTabActivate` (commit `d6254489`). Pre-existing gap,
  NOT a migration regression (confirmed unchanged vs main). 24/24 in fresh
  containers.
- **`create-task-branch-selector.spec.ts:540` — rare race, FIXED.** Click
  swallowed during popover hydration before the refresh handler wires →
  `waitForRequest` hangs. Retry-click via `toPass` (commit `d843fa53`).
- **All other flakes — non-deterministic host-contention artifacts, NOT
  defects.** clarification, monitor, config-management (×N),
  permission-approval, markdown-preview, office, port-forward, git-changes,
  diff-expansion, tool-completion. Proof: (1) each passes clean AND fast in
  a fresh isolated container (e.g. clarification 63/63; the 3 tests that
  hard-failed one shard pass in 1.7s/4.9s/2.5s alone); (2) two identical
  full-suite runs gave **0 vs 3 hard failures** — same code, same config,
  different outcome = resource starvation, not a code path. They surface
  only when 5 heavy shards (each = Go backend + Next standalone + Chromium
  + mock agent) share one host's CPU/IO/disk. CI's 10 **isolated** runners
  don't oversubscribe this way, so they don't reproduce there.

**To get a clean local full-suite pass**, run fewer concurrent shards
(2–3, not 5) so the host isn't oversubscribed — the contention tail
disappears. There is no further code/test fix for the contention cluster:
the waits are condition-correct; the tests just time out under CPU
starvation, which only blind timeout inflation (discouraged) or reduced
local parallelism addresses.

### Status (2026-05-29)

**Done.** Strict mode is enforced on the main CI e2e shards. Both a full
local 1022-test chromium run AND the first CI run under
`KANDEV_E2E_WS_ASSERT=1` produced **zero WS-accounting drops** — the
receipt layer (`WsAccount`, per-connection + per-session) and the apply
layer (bridge audit) are both clean in CI, not just locally.

**First CI strict-mode run (commit 8e0a4b1b → re-run on 3b850c14):**
- Claude review verdict flipped to `ready` (blockers cleared by the
  `flattenTasksPaginated` / `tasksPaginated`-invalidation unit tests).
- "Backend Tests" red was a golangci-lint `unused` on the dead
  `stampAndMarshal` wrapper left by the Workstream 1 refactor — removed
  (commit 3b850c14).
- 3 E2E shards (1/5/6) each had 1 test fail after `retries: 2`:
  `permission-approval:94`, `create-task-github-url:747`,
  `create-task:201`. All three are **pre-existing contention flakes** —
  each passes 3/3 in isolation under strict mode, none touch our migrated
  paths (the permission icon reads the unchanged `lib/ws/handlers/`
  Zustand mirror; worktree/git is untouched), and none was a WS-accounting
  drop. CI sharded load + a 2-retry ceiling occasionally surfaces them;
  a re-run typically clears them. Not regressions, not in scope for this
  PR (pre-existing suite flakiness).
- **Proof it's general contention, not our diff:** re-running the
  shard-1 chat cluster (markdown-preview + monitor + permission-approval
  + quick-chat) together in a single local worker reproduced failures —
  but a *different* subset each run, and `markdown-preview:129` (a
  dockview-reload-restore test our diff does NOT touch) was among them.
  A test in untouched code flaking under clustering means the flakiness
  is worker-contention from running many heavy agent-streaming specs
  back-to-back — exactly what 10-way sharding exists to bound — not
  anything this branch introduced. Each spec passes 3/3 run alone.
  **Recommendation:** re-run the failed shards to get a green check;
  address suite contention-flakiness separately (out of scope here).

Triage of the 24 hard failures from the local run:

- **2 genuine regressions from the TQ migration — fixed.** Plan-mode
  workflow advancement (`create-task` plan specs) stayed on the old step:
  `applyIfNewer` in `bridge/kanban.ts` discarded a `task.updated` whose
  `updated_at` tied the cached second, dropping the step/state change.
  Fixed by folding `workflowStepId` + `state` (server-authoritative)
  through the stale-timestamp path alongside the primary-session fields.
- **1 was our own e2e spec** (`ws-event-accounting.spec.ts`) — rewrote to
  drive a live message so per-session buckets populate deterministically.
- **2 pre-existing env failures** — code-server can't launch in the
  sandbox (vscode-open-*); branch touches zero vscode code.
- **19 pre-existing parallel-contention flakes** — pass in isolation;
  CI's 10-way sharding + `retries: 2` absorbs them.

Strict mode was never the cause of a failure (the WS check runs only
after a test body passes).

**Follow-up (not blocking merge):** the `containers` e2e project
(docker/ssh executors) does NOT yet set `KANDEV_E2E_WS_ASSERT=1` — it
wasn't validated locally (needs a Docker daemon). Enable it there after a
strict containers run triages any container-specific drops.

### Done criteria

- `KANDEV_E2E_WS_ASSERT=1` is the default on the CI chromium/mobile e2e
  shards (`.github/workflows/e2e-tests.yml`). ✅
- Every domain registrar in `lib/query/bridge/` wraps its handlers via
  `wrapBridgeHandler`; every `WS` event with `session_id` that the
  fixture observes either lands in the bridge audit ring with
  `cacheChanged === true` or is on `BRIDGE_SKIPPED_ACTIONS`.
- The audit catalog has every observed drop categorized as
  `fixed` / `documented` / `accepted-with-rationale`, with each entry
  noting whether the drop was at the receipt layer (`WsAccount`) or
  the apply layer (bridge audit).
- The band-aid fixes are either reverted (if Phase 2 made them
  unnecessary) or kept with an explicit comment naming the underlying
  WS reliability issue they're working around.

---

## TanStack Query bridge: own it, instrument it

The TQ migration lives on this branch (`b95b168b`, parent of Phase 1).
Every server-state domain has a `lib/query/bridge/<domain>.ts`
registrar that mirrors `lib/ws/handlers/<domain>.ts` 1:1 but writes
into the TQ cache instead of Zustand. During the transition window,
**both pathways fire on the same WS event** until each per-domain
bridge removes its Zustand counterpart.

Because the bridges are on our branch, we instrument them directly
rather than treating them as an external dependency.

### What still works unchanged

`WsAccount.record()` is called at the `handleParsedMessage` seam in
`apps/web/lib/ws/client.ts`, BEFORE any handler dispatch. It records
what the client *parsed*, not what any specific consumer wrote. So the
core "did the envelope arrive?" check is unaffected by the migration —
both the per-connection ring buffer (Phase 1) and the per-session ring
buffer (Workstream 1) capture the receipt regardless of whether the
event then routes to Zustand, the TQ bridge, or both.

### What changes

1. **Workstream 1's per-session accounting must coexist with dual-write
   handlers.** No code change to `WsAccount` — the seam still fires
   once per parsed envelope. But the new e2e spec
   (`tests/system/ws-event-accounting.spec.ts`) needs to assert state
   via *both* Zustand selectors AND `queryClient.getQueryData(qk....)`
   for any domain that has shipped a bridge. The accounting check
   itself is migration-agnostic; UI assertions around it are not.

2. **Workstream 2's ack channel should reuse `subscribeWebSocketClient`.**
   `lib/ws/connection.ts` now exposes `subscribeWebSocketClient(listener)`
   for consumers (notably `<QueryBridge />`) that mount before the WS
   client exists. The new `ws-account-acker.ts` module should use this
   same primitive to attach/detach across reconnects rather than wiring
   into `WebSocketConnector` directly — same pattern, less surface area.

3. **Ack timing is at parse, not at handler completion.** A naive read
   of "ack what the FE processed" might suggest acking after every
   handler (Zustand + bridge + future) has run. Don't. Ack at parse time
   to keep the contract simple and match `WsAccount.record()`'s site.
   Handler-side drops (bridge wrote to wrong cache key, Zustand mutation
   threw silently) are a different failure mode — see point 5.

4. **Allowlist bridge-skipped events.** PR #1052 explicitly leaves
   `session.state_changed`, `agentctl_*` events, and
   `message.queue.status_changed` in Zustand only. Workstream 3's audit
   tooling must NOT flag these as "bridge didn't process X" — they're a
   known split. Add a `BRIDGE_SKIPPED_ACTIONS` constant to
   `apps/web/lib/query/bridge/index.ts` (or read the comment in
   `bridge/session.ts`) and surface it from the audit helper so triage
   sees only real coverage gaps.

5. **Per-handler "did this event affect state?" counter — promote to
   mandatory.** A gap `WsAccount` cannot catch on its own: the envelope
   arrived, was parsed, recorded — but the bridge wrote into the wrong
   query key, or the Zustand mutation no-op'd because the slice was
   unmounted. Because we own the bridges, this is cheap to do well.

   Add a `wrapBridgeHandler(action, handler)` helper in
   `lib/query/bridge/index.ts` that:
   - Calls the underlying handler.
   - Captures `queryClient.getQueryCache().getAll().length` and a hash
     of the affected query keys before/after.
   - Records `{action, sessionId?, cacheChanged, queryKeysTouched}` to
     `window.__kandev_bridge_audit__` (a bounded ring buffer, only
     populated when `NEXT_PUBLIC_KANDEV_E2E_MOCK === "true"`).
   - Every per-domain registrar (`registerSessionBridge`,
     `registerKanbanBridge`, …) wraps its handlers via this helper at
     registration time.

   The fixture's `computeWsDrops` then has a second pass: for each
   recorded WS event with `session_id`, assert that the bridge audit
   ring has a matching entry with `cacheChanged === true` or the action
   is on `BRIDGE_SKIPPED_ACTIONS` (see next point). A receipt without a
   cache mutation is a bridge bug — exactly the class of "the event
   reached the FE but didn't affect state" failure the user called out.

6. **`BRIDGE_SKIPPED_ACTIONS` allowlist.** The bridge intentionally
   leaves some actions Zustand-only: `session.state_changed`,
   `agentctl_*`, `message.queue.status_changed`. Hardcode this list in
   `lib/query/bridge/index.ts` and export it. The audit helper imports
   it and excludes those actions from the cache-mutation assertion.
   When a future bridge wave moves one of these into the bridge,
   removing the entry is a one-line change.

7. **No "wait or overlap" decision needed for Workstream 3.** Previous
   drafts of this plan debated whether to start Workstream 3 before or
   after the TQ migration merges. That's resolved: the migration is on
   this branch. Strict assertion (`KANDEV_E2E_WS_ASSERT=1`) flips on
   as part of Workstream 3, with all bridges already in place.

8. **Documentation update (Workstream 3 step 5) expands.** When adding
   a new WS message type, the rule is:
   - Register a handler in BOTH `lib/ws/handlers/<domain>.ts` AND
     `lib/query/bridge/<domain>.ts`, wrapped via `wrapBridgeHandler`.
   - OR, if the action is explicitly Zustand-only, add it to
     `BRIDGE_SKIPPED_ACTIONS` with an inline comment explaining why.
   Add this to `apps/web/CLAUDE.md` and root `CLAUDE.md` alongside the
   accounting expectation. Lint rule (custom eslint or runtime warn in
   dev) that flags handlers in `lib/ws/handlers/` without a matching
   bridge entry or allowlist membership is a nice-to-have.

### The reliability guarantee this branch delivers

With Phase 1 + Phase 2 + the bridge instrumentation above shipping
together, the FE can make a sharp claim: every server-state event the
backend emits has three observable checkpoints — received (WS frame
parsed), recorded (in `WsAccount` and on the wire seq), and applied
(bridge handler ran and the cache changed, OR the action is on the
documented Zustand-only allowlist). A failure at any checkpoint is a
loud, attributable test failure, not a silent UI desync.

This is the user-facing pitch for the branch: not "we migrated to
TanStack Query" but "we migrated to TanStack Query AND made the WS
event pipeline auditable end-to-end."

---

## Order of operations

The three workstreams are intentionally sequenced so audit can verify
both new mechanisms:

0. **Bridge instrumentation first.** `wrapBridgeHandler` +
   `BRIDGE_SKIPPED_ACTIONS` from the TanStack section is a prerequisite
   for both Workstream 1's spec and Workstream 3's audit. Ship it as
   the first commit on top of the TQ-migration parent; it's small,
   localized to `lib/query/bridge/`, and unblocks everything else.
1. **Workstream 1 next.** Per-session sequencing is the unit of truth
   the ack channel and the audit both need. Ship it, verify with a
   focused e2e spec that exercises both `WsAccount` gap detection and
   bridge audit cache-mutation assertion, leave global assertion off.
2. **Workstream 2 in parallel with the start of Workstream 3.** The
   ack channel doesn't need the audit complete; the audit can begin
   with per-session accounting + bridge audit alone. They overlap.
3. **Workstream 3 closes the loop.** Strict assertion on, drops fixed
   or documented, band-aids audited.

Rough effort estimate (single agent, focused, no context-switching):

- Workstream 0 (bridge instrumentation): 1 day
- Workstream 1: 2-3 days
- Workstream 2: 3-4 days (ack channel + metrics + alert wiring)
- Workstream 3: 1-2 weeks (depends entirely on how many drops surface) —
  **open-ended.** Triage what one shard surfaces before committing to
  the full migration. If a single shard produces 50+ distinct drop
  signatures, stop and re-scope with the user rather than grinding
  through individually.

---

## What this plan does NOT include

- **Replay / catch-up after disconnect.** Backend buffering of events
  the FE missed during a disconnect window, replayed on reconnect.
  That's a separate reliability project worth its own ADR — needs
  decisions on memory bounds, event-type filtering, replay ordering
  guarantees. Phase 2 surfaces the drops; the FE's existing reconnect-
  refetch path handles recovery, suboptimally.

- **Event ordering verification.** Right now we only check presence
  (every seq from min to max). Adding order verification would catch
  out-of-order delivery but requires per-event causality knowledge that
  varies by event type.

- **Multi-tab consistency.** Two tabs of the same user subscribed to
  the same session. Today each tab's connection has its own seq stream;
  drops in tab A don't affect tab B. Probably OK for now but worth
  documenting if a user reports cross-tab divergence.

---

## References

- Phase 1 code lives on `feature/ws-event-accounting`:
  - Backend ring buffer: `apps/backend/internal/gateway/websocket/ws_sent_log.go`
  - FE ring buffer: `apps/web/lib/ws/ws-account.ts`
  - Test glue: `apps/web/e2e/helpers/ws-account.ts`
  - Fixture hook: `apps/web/e2e/fixtures/test-base.ts`
  - E2E endpoint: `apps/backend/cmd/kandev/e2e_reset.go` (`handleE2EWsSent`)
- TanStack Query bridge layer (parent commit, on this branch):
  - Bridge registrar entry point: `apps/web/lib/query/bridge/index.ts`
    (this is where `wrapBridgeHandler` and `BRIDGE_SKIPPED_ACTIONS` land)
  - Per-domain bridges: `apps/web/lib/query/bridge/<domain>.ts`
    (features, comments, workspace, settings, automations, integrations,
    github, gitlab, jira, linear, kanban, office, session,
    session-runtime, session-runtime-streams)
  - WS client subscriber primitive: `apps/web/lib/ws/connection.ts`
    (`subscribeWebSocketClient`)
  - Bridge-skipped event rationale: header comment in
    `apps/web/lib/query/bridge/session.ts`
- Design discussion: PR #1126 thread, "WS event accounting" reply chain.
- Conventions for new tests: `apps/backend/AGENTS.md` (`testing/synctest`),
  `apps/web/CLAUDE.md` (eslint complexity limits).
