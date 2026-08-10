# Spec: `parked-board-mvp` — first vertical slice (Darwin + Linux)

**Status:** frozen contract (Spec phase complete 2026-08-10)
**Slug:** `parked-board-mvp`
**Parent:** `docs/specs/disambiguate-waiting/spec.md` on branch `feature/waiting-attribution-hxr`
(85 ACs). This is its **first vertical slice**, per `docs/plans/parked-board-mvp/split-proposal.md`
(v4). **This document is the CONTRACT for the slice.** Every acceptance criterion below is
reproduced inline in its V1-narrowed form; a builder never needs to read the parent spec, and where
the two differ **this document wins**. Parent AC numbers are retained as identity for traceability
only.
**Platform decision:** **both Unix platforms (macOS/Darwin + Linux) ship in V1.** Windows and every
other GOOS return `unknown` (never parked, renders exactly as today).
**Date:** 2026-08-10

---

## 1. Why

`disambiguate-waiting` (parked-on-background-work) failed to converge as one 85-AC unit — 32 fix
commits over 14 review rounds. The split proposal cuts it into thin vertical slices; this is the
smallest — one real end-to-end path that lights a single surface. It ships on **both Unix
platforms** because:

1. **Both Unix probes already exist and are mature.**
   `apps/backend/internal/agentctl/server/process/probe_darwin.go` **and** `probe_linux.go` each
   implement the full descendant walk, `(pid, start-time)` identity, zombie exclusion, cycle guard,
   and a caller-budget-bounded walk. **The OS-hard part — the single biggest churn source in the
   monolith — is done for both.** Shipping only one would mean writing *new* code to suppress the
   other (dispatch is by Go build tag, so a Linux build runs `probe_linux.go` and returns real
   values, not `unknown`).
2. **Linux keeps the CI gate; macOS keeps the showcase.** The backend test job runs on
   `ubuntu-latest`, so the Linux probe's real-process ACs are the enforced gate. macOS is the
   headline demo environment (Kandev **Desktop** is a Mac app) and its ACs are host-gated on top.
3. **Darwin is technically simpler than Linux here:** `p_starttime` is wall-clock, so Darwin avoids
   the boot-tick re-derivation hazard the Linux probe handles. Linux's hazard was already hit,
   fixed, and regression-guarded — so both are safe to freeze.

## 2. Ship story

> Behind an off-by-default runtime flag, on **macOS and Linux**, a Claude session that settles to
> `WAITING_FOR_INPUT` while a detached background shell it launched during that turn is still
> alive renders `task-state-background-running` on its **board card** instead of
> `task-state-waiting-for-input` — computed by one synchronous probe at settle, and cleared when
> the session next leaves `WAITING_FOR_INPUT`.

Real end to end: real Claude recogniser attestation → real settle hook → **real process-tree
probe** → real task/session DTO → real board card. Headline demo on macOS; CI enforcement on Linux.

---

## 3. Verified inputs (sampled 2026-08-10, not assumed)

Every shape below was read from the tree, not inferred. Files named without a branch are on
`main`/the seed branch; files named `hxr:` are on `feature/waiting-attribution-hxr` (the harvest
source).

| Input | Sampled shape |
|---|---|
| Probe result domain | `probe.go` — exactly three string constants: `"live"`, `"settled"`, `"unknown"`. |
| Probe budget | `probe.go: parseProbeEnvBudget` — reads `KANDEV_PARKED_PROBE_BUDGET`; unset → 250ms; unparseable or `<= 0` → warn + 250ms. **Already implemented on the seed branch.** |
| Process identity | `probe.go: rootIdentity{pid, startTicks, startTime}`; `turnStartMarker{wallTime, bootTicks, hasBootTicks}` with `isZero()` keyed on `wallTime`. |
| Linux walk | `probe_linux.go: walkProcessTree` — BFS over `/proc`; returns `live` on the **first** non-zombie descendant with `startTicks >= marker.bootTicks`; `settled` if the walk completes with none; `unknown` on `root.pid <= 0`, `ctx.Err() != nil`, `!marker.hasBootTicks`, root re-read failure, root `startTicks` mismatch, or `/proc` read failure. `visited` map guards ppid cycles. |
| Darwin walk | `probe_darwin.go` — same contract in the wall-clock domain (`p_starttime`). |
| Other platforms | `probe_windows.go`, `probe_other.go` — stubs; `captureRootIdentity` returns `ok=false`, walk returns `unknown`. |
| Turn stamp | `hxr:` adds `process.Manager.RecordTurnStart(time.Time)` → `newTurnStartMarker`, plus `AgentPID(acpSessionID)`, `ProbeProcessTree(ctx, acpSessionID)`, `agentRootIdentity` captured right after `cmd.Start()`. Plumbed via `adapter.Config.RecordTurnStart` → `shared.Config.RecordTurnStart`. |
| Dispatch paths | `adapter_prompt.go` on main: `Prompt` (`:25`), `PromptSteer` (`:55`) and `fireWakeup` (`:425`) **all** call `sendPrompt` (`:72`). `sendPrompt` calls `beginPromptTurn(sessionID)` at `:141`, after its two early returns (`drop`, `conn == nil`). |
| Bypass hazard | `hxr:` stamps inside `a.syncNotifQueueThen(...)` fused with the deferred `turn_started` emit. `syncNotifQueueThen` (`adapter_updates.go:67`) **returns false and never runs the callback** when `lifetimeCtx` is done. |
| Recogniser | `normalize.go:305` on main **already** reads `agentID == claudeAgentID && payload.ShellExec() != nil && payload.ShellExec().Background` → `SetBackgroundWorkIdentity(BackgroundWorkKindShell, "", true, false)`. `hxr:` replaces this with the public registry (`background_launch_recognizer.go`). |
| Kind filter | `streams.BackgroundWorkKindShell` and `NormalizedPayload.IsDetachedBackgroundLaunch()` exist on main (`types/streams/background_work.go`); `backgroundWorkKind(payload)` exists at `orchestrator/event_handlers_streaming.go:753`. `stampSubagentBackgroundWork` (`normalize.go:641`) is the independent `Kind=subagent, Detached=true` producer that admits `mockAgentID`. |
| Settle seam | `orchestrator/event_handlers_streaming.go: updateTaskSessionStateWithHook` (~`:873`) — the one place `hxr:` hangs `onSessionParkedHook` / `unparkOnStateLeave`. |
| Formula | `hxr:parked_projection.go: computeParked` = `observedDetached && lastSample == "live" && sessionState == WAITING_FOR_INPUT`. |
| MCP settle exclusion | `internal/mcp/handlers/handlers.go:3296` and `parent_question.go:158` call `setSessionWaitingForInput`, which does **not** enter package `orchestrator`. |
| Session states | `models.go:1005-1020` — `CREATED, STARTING, RUNNING, IDLE, WAITING_FOR_INPUT, COMPLETED, FAILED, CANCELLED`. |
| Transport | `hxr:client_probe.go` (`agent.background.probe`, `{session_id}` → `{result}`), `hxr:manager_probe.go` (`BackgroundProbe` port; Kandev→ACP translation; `CheckSessionAccess`), `hxr:agent_probe.go` (agentctl WS handler → `procMgr.ProbeProcessTree`). |
| Board render | `kanban-card-content.tsx: renderTaskStatusIcon` — early return `null` at `:275`, bare `IconLoader2` at `:282`; then `getTaskStateIcon`. `lib/ui/state-icons.tsx: getTaskStateIconConfig` is the shared task resolver. |
| Board producers | Four independent producers of `KanbanTask`: `lib/kanban/map-task.ts: toKanbanTask`, and hand-built rebuilds in `lib/ws/handlers/kanban.ts` (`state.kanban.tasks` **and** `state.kanbanMulti.snapshots[…].tasks`) and `lib/ssr/mapper.ts: snapshotToState`. |
| i18n | `apps/web/src/locales/en/task.json` **already contains** `"backgroundWorkIsRunning": "Background work is running"` on main. No new copy is introduced by this slice. |
| Flag identity contract | `runtimeflags/registry.go` (key + env + kind + label + description + stability + risk + restart/mutability + typed `read`/`apply`), `common/config/config.go: FeaturesConfig` field with `mapstructure`+`json` tags, root `profiles.yaml` under `features:`, and `apps/web/lib/state/slices/features/types.ts` defaults. `features-contract.test.ts` requires **exact key equality across all four**. |

---

## 4. Frozen invariants (implemented on both Unix platforms — freeze, do not re-derive)

These two were the monolith's biggest churn source, invented piecemeal across rounds 9/10/14. Here
they are contract. A review finding that proposes changing either is a **spec defect**, not a fix.

**INV-1 — Process identity is `(pid, start-time)`, compared in ONE explicit clock domain per
platform, with zombie and before-turn-start descendants excluded.** Darwin compares in **wall
clock** (`p_starttime`, µs resolution); Linux compares in the **boot-tick domain**
(ticks-since-boot, `CLK_TCK = 100`). Both truncate the turn-start **down** to source resolution
before an **inclusive** (`>=`) comparison, so a same-tick birth counts as in-turn — **error always
falls toward `live`.** A bare-PID existence check is forbidden; a PID whose current occupant's
start time does not match the captured identity yields `unknown`.

**INV-2 — The turn-start marker is materialised once, at stamp time, via `newTurnStartMarker`;
probes never re-derive it.** Darwin does not strictly need this (wall clock is stamp/probe
indistinguishable), but **Linux does** — it freezes boot-ticks against the anchor read at that
instant. Building the marker at probe time is correct on Darwin and **silently wrong** on Linux: a
wall-clock adjustment between stamp and probe would shift the anchor and could push an in-turn
descendant before the re-derived turn start, flipping live work to falsely `settled`.

---

## 5. Scope decisions resolved in this spec

The prior draft carried four open questions and one "decision required". All are now **closed**.
No downstream step may reopen them without routing back to Spec.

### D-1 — Turn-stamp bypass: resolved as **(a) stamp on all dispatch paths** {#d1}

The stamp MUST fire on **every** non-dropped prompt dispatch — the operator path (`Prompt`), the
mid-turn steering path (`PromptSteer`), and the synthetic `ScheduleWakeup` path (`fireWakeup`).
All three already funnel through `sendPrompt`, so the stamp is placed **in `sendPrompt`,
unconditionally, immediately after `beginPromptTurn(sessionID)` and before `conn.Prompt`** — after
both early returns, so a dropped wakeup and an uninitialised adapter stamp nothing.

It MUST NOT be placed inside `syncNotifQueueThen`'s callback. The monolith put it there only
because it was fused with the `turn_started` emission, which this slice defers; that callback is
**skipped entirely when `lifetimeCtx` is done**, so a stamp placed there is silently absent at
shutdown. A stale (older) threshold biases the probe toward `live`, which renders a **spuriously
parked card** — a wrong showcase. Since V1 emits no stream event, the FIFO barrier buys nothing
and costs the bypass.

*Rejected alternative:* recording stale-stamp-on-wakeup as an accepted limitation. The whole
deliverable is a correct-looking board card; a false park is the one failure that discredits it.

### D-2 — No `parked_epoch` / `parked_revision` anywhere in V1 {#d2}

**The wire carries exactly one new field per DTO: a boolean.** V1 adds
`parked_on_background_work` to `TaskDTO`, `TaskSessionDTO` and `TaskSessionSummaryDTO`, and
**nothing else**. There is no epoch, no revision, no `resolveParkedTriple`, no lexicographic
discard rule, and no client-side revision cursor.

Rationale: the monolith's revision/epoch protocol exists to order *concurrent, asynchronous*
parked transitions produced by a sampler loop. V1 has no sampler. The bit changes only at a settle
transition or a state-leave, both of which already ride the session/task frames those transitions
publish, and the bit is **only meaningful beside `state == WAITING_FOR_INPUT`** — a stale `true`
arriving beside `RUNNING` renders nothing (§8.4). Shipping always-zero epoch/revision fields would
put an untested, unexercised ordering protocol on the wire and in the client; that is precisely
the kind of speculative machinery this slice exists to avoid. V3 adds both fields **additively**
(new JSON fields default to zero for old clients), so nothing is foreclosed.

**Consequences, stated so they are not re-derived:**

- `apps/web/lib/kanban/parked-projection.ts` and its test are **NOT harvested** (this overrides the
  HARVEST entry in `docs/plans/parked-board-mvp/reuse-map.md`, which predates this decision).
  `map-task.ts` gains a plain boolean read, not a triple.
- The DTO provider interfaces are the **one-value** form, not the harvested three-value form:
  `ParkedProjectionSnapshot(sessionID string) bool` and
  `TaskParkedProjectionSnapshot(taskID string) bool`. `EnrichParked`, `EnrichParkedSummary` and
  `EnrichTaskParked` assign one field each. A **nil provider leaves the field `false`**, which is
  the correct serialization when the projection is not wired.
- The frontend store field is `KanbanTask.parkedOnBackgroundWork?: boolean` — **optional**, so
  `kanban.ts`'s existing `preserveIfUndefined` discipline carries it across a rebuild that omits it
  (AC-58a). The wire field is `parked_on_background_work?: boolean` on both the task and session
  HTTP types; **absent is read as `false`** at the mapping boundary, never as "unknown".

### D-3 — Attestation clearing: on observed transition into `RUNNING` **or** `STARTING` {#d3}

`turn_started` (parent AC-41a/41b, the barrier + cancellation-admission machinery) is deferred to
V5. This slice clears the attestation on the backend's **own observed session-state transition** —
a write it already owns — into either `RUNNING` or `STARTING`.

`RUNNING` is the turn-start substitute. `STARTING` is included because two paths reach
`WAITING_FOR_INPUT` **without** passing through `RUNNING`: `GetTaskSessionStatus`'s stale-`STARTING`
heal and `ResetAgentContext`'s restore, both `STARTING → WAITING_FOR_INPUT`. Without the `STARTING`
half, an attestation from an earlier turn could survive into one of those heals and park a session
on work that belongs to a dead turn. With both halves, **every** path that reaches
`WAITING_FOR_INPUT` and probes has already cleared the attestation, which is exactly the premise
the parent's AC-40 note relies on and which V1 would otherwise leave to chance.

This is not a new invariant — it is the parent's own rule ("attestation never survives a turn the
backend saw start") enumerated over the states the backend actually observes. Clearing is strictly
conservative: it can only make `parked` false. The later `turn_started` slice **sharpens** the
boundary (steer / `ScheduleWakeup` self-resume / cancellation edges); it does not replace this rule.

### D-4 — No `macos-latest` CI job in this slice {#d4}

AC-80's Linux instance runs on `ubuntu-latest` and **is** the CI gate. The Darwin instance is
host-gated and skips with an explicit `runtime.GOOS` reason off-Darwin, via the
`probe_notdarwin_test.go` skip-sibling — so a green ubuntu log never reads as Darwin coverage *and*
the test name is never silently absent. Adding a macOS runner is CI-infrastructure scope this slice
does not need, since the gate already exists. **Named residual risk:** a Darwin-only regression is
invisible to default CI. Recommended before the flag is promoted to `prod: "true"`, not before this
merges.

### D-5 — No clear-on-execution-end publish {#d5}

Stale-until-resume is the parent's blessed `INTERVAL=0` behaviour (AC-74), stated below as
**AC-74 (V1)**. Adding a second clearing trigger widens the clearing-rule surface — the exact
surface V2 owns — for no showcase benefit. Left to V2.

### D-6 — Board is the single surface {#d6}

The board card (`kanban-card-content.tsx` + the shared `getTaskStateIconConfig` resolver + the four
producers). The sidebar task list, `/tasks`, the session switcher, tooltips, mobile, graph nodes
and the pseudo-locale audit are all V4.

### D-7 — Inline Claude recogniser; no public registry {#d7}

`normalize.go:305` on main **already is** the V1 recogniser. This slice therefore makes **no change
to `normalize.go`** and does **not** harvest `background_launch_recognizer.go` — that file is the
V5 public-registry seam (`RegisterBackgroundLaunchRecognizer`, panic-on-duplicate, AC-69). The
recogniser half of the predicate stays inline in agentctl; the `Kind == BackgroundWorkKindShell`
half is added in the backend's `handleToolCallEvent`.

---

## 6. Requirements

- A Claude session that settled to `WAITING_FOR_INPUT` after a turn in which a detached
  `Kind=shell` launch was attested, and whose synchronous probe reports `live`, SHALL be projected
  as **parked** and render distinctly on the **board card** — on macOS and Linux.
- Parked is a **projection**, not a lifecycle state. `TaskSessionState` gains no member; nothing is
  persisted; no migration is written.
- Liveness is derived by **one synchronous level-sample at settle**. There is no periodic sampling,
  no sampler goroutine, no eviction, no tombstone, no epoch, no revision.
- The projection is **conservative**: true only when Kandev observed the detached launch **and** the
  probe positively reports `live`. `settled` / `unknown` / no attestation ⇒ renders as today.
- **Notification behaviour SHALL be byte-for-byte unchanged.**
- **Off by default:** with the flag off, no probe is issued, the field is `false`, and behaviour is
  byte-identical to today.
- An agent with **no recogniser match** is never attested, never probed, never parked.

---

## 7. Design

### 7.1 The three-term formula

```
parked(session) :=  observedDetached(session)
                 && lastSample(session) == "live"
                 && sessionState(session) == WAITING_FOR_INPUT
```

All three terms are required. `observedDetached` is set by the ordered tool-call consumer and
cleared per [D-3](#d3). `lastSample` is written only by the settle-hook probe. `sessionState` is
re-read from the repository immediately before the write is applied — **not** trusted from hook
entry — because the probe's round trip (up to the budget) can outlast the state it observed.

Task-level: `parked(task) := OR over the task's sessions`. Boolean OR, so member iteration order is
not observable. A task with no sessions, or whose sessions have no recorded projection, is `false`.

### 7.2 Concurrency model {#concurrency}

V1 has **one coarse projection mutex** (`parkedStatesMu`) guarding the per-session rows, and **one
per-task mutex** guarding the task-level member set. No long-lived goroutine exists.

**Lock order (frozen):** `parkedStatesMu` is acquired before any per-task mutex, and **never** held
across a call into the session repository, the workflow engine, the event bus, or the probe port.

**Probe revalidation.** The synchronous probe runs with **no lock held**. On completion, the write
is applied under `parkedStatesMu` in the **same critical section** that revalidates it — never as a
separate unlocked check beforehand. The revalidated tuple is exactly two values:

1. `observedDetached` — unchanged since the probe was issued;
2. `stateGeneration` — a per-session, process-local counter, **not serialized on any carrier**,
   incremented on every observed transition this slice handles: into `RUNNING`, into `STARTING`,
   and out of `WAITING_FOR_INPUT`. A single transition may satisfy more than one of those (e.g.
   `WAITING_FOR_INPUT → RUNNING`) and may therefore increment it more than once — that is
   harmless and intended. **Only inequality is ever observed, never the magnitude or the delta.**

If either differs, the sample is **discarded**: nothing is written, no transition, no publish. This
is the monolith's guard narrowed from four counters to two — `turnMarker` and `generation` existed
only to disambiguate the sampler's eviction/revival races, which V1 does not have.

**Where the transitions are observed.** All of it — the settle probe, the un-park, and the
attestation clearing of [D-3](#d3) — hangs off the **one** seam
`orchestrator.Service.updateTaskSessionStateWithHook` (`event_handlers_streaming.go`). No second
observation point is added, and no path outside package `orchestrator` participates (which is what
makes **AC-40b** a decision rather than an accident).

**Two callers, same session.** At most **one** synchronous probe is issued per CAS-confirmed
transition into `WAITING_FOR_INPUT`. The state CAS in `updateTaskSessionStateWithHook` is the
serialization point — exactly one caller wins the transition and therefore exactly one calls the
hook. No additional lock is added for this, and the hook must not re-read "was already waiting"
outside the CAS.

**Two sessions, same task.** Both may transition concurrently. The per-task critical section covers
`members` write → recompute OR → compare → publish-decision as one unit. Because the OR is
idempotent and order-free, the final value is `true` if either member is `true` regardless of
arrival order.

### 7.3 Settle-path budget

The synchronous probe runs under `context.WithTimeout(ctx, budget)` where `budget` is
`KANDEV_PARKED_PROBE_BUDGET` (default 250ms). **The settle transition is never delayed beyond the
budget.** A probe that does not complete in time resolves to `unknown`.

### 7.4 Board rendering precedence (frozen total order)

`getTaskStateIconConfig` evaluates in this order; the first match wins:

1. pending permission
2. pending clarification
3. `foregroundActivity === "generating"`
4. `foregroundActivity === "background"`
5. **`parkedOnBackgroundWork`** ← new branch
6. `WAITING_FOR_INPUT`
7. interrupted
8. everything else, unchanged

Parked is evaluated **after** both `foregroundActivity` checks: `foregroundActivity` is a
most-active-wins aggregate and `parkedOnBackgroundWork` is an OR, so both can legitimately be true
at once on a multi-session task, and an actively generating session must outrank a merely parked
one. Parked is evaluated **before** `WAITING_FOR_INPUT` — that is the whole feature — and **after**
both pending-input branches, so an actionable question is never hidden behind the background
affordance.

`renderTaskStatusIcon` has two early returns that must both learn the new term: the `return null`
short-circuit (`:275`) and the bare-`IconLoader2` launch-spinner short-circuit (`:282`).

The parked affordance renders through `BackgroundWorkTaskIcon` (`IconCircleDashed`,
`data-testid="task-state-background-running"`, tooltip + `aria-label` from the existing
`task:backgroundWorkIsRunning` key). The pre-existing `foregroundActivity === "background"` icon
stays a bare `IconLoader` with no `data-testid` — byte-identical to today.

### 7.5 The runtime flag

| Layer | Value |
|---|---|
| Registry key | `features.parkedOnBackgroundWork` |
| Env var | `KANDEV_FEATURES_PARKED_ON_BACKGROUND_WORK` |
| Go config field | `FeaturesConfig.ParkedOnBackgroundWork` (`mapstructure:"parked_on_background_work" json:"parkedOnBackgroundWork"`) |
| `profiles.yaml` | `prod: "false"`, `dev: "false"`, `e2e: "false"` |
| Frontend defaults key | `parkedOnBackgroundWork: false` in `apps/web/lib/state/slices/features/types.ts` |
| Kind / stability / risk | `KindFeature` / `StabilityExperimental` / `RiskMedium` |
| Restart / mutability | `RestartRequired: true`, `Mutable: true` |

**Gate placement: the settle hook, and only the settle hook.** When the flag is off,
`onSessionParkedHook` returns before capturing any snapshot and before issuing any probe.
`lastSample` is never written, so the formula's second term is never satisfiable and every DTO
serializes `false`.

**How the flag is read.** The gate is a **per-call read** of `cfg.Features.ParkedOnBackgroundWork`
at settle-hook entry, through the config handle `Service` already holds. **Nothing is gated at
construction time:** the probe port is always injected and the agentctl turn-start stamp
(`RecordTurnStart`) is always plumbed and always fires. Both are unobservable when the flag is off —
the stamp stores one `time.Time` in memory on a path that already runs, and the port is never
called. Keeping construction ungated is what allows the gate to live in exactly one place.

**Two consequences of that, stated so they are not re-derived:**

- The **attestation is still recorded** when the flag is off (a cheap in-memory bool on an existing
  code path). It can never reach a rendered surface, because the probe never runs and `lastSample`
  is therefore never `"live"`.
- **`RestartRequired: true` is correct even though the read is per-call.** A mid-run toggle from on
  to off stops new probes, but it cannot retroactively clear a session already holding
  `parked = true` in process memory — that row clears only when the session next leaves
  `WAITING_FOR_INPUT`. A restart drops the whole projection (**AC-36**), which is what makes the
  off-state total and byte-identical to a build without this feature. Do not "simplify" this to
  `false`.

**The frontend needs no `useFeature()` gate** — it renders whatever the DTO carries, and with the
flag off the DTO always carries `false`. The frontend defaults key is nevertheless **required**,
because `features-contract.test.ts` enforces exact key equality across `profiles.yaml`, the Go
registry, and the frontend defaults. Omitting it fails that test.

This section amends the parent spec's "ships unflagged" position for the sliced delivery.

### 7.6 Publication carriers

The projection is **runtime-only and never persisted**, so it introduces **no new event type** and
no new WebSocket action beyond `agent.background.probe` (§8.4). It rides carriers that already
exist:

| Change | Carrier |
|---|---|
| Session-level `false ⇄ true` | the existing `session.activity_changed` event, re-published so the DTO enrichment re-derives the new value |
| Task-level OR `false ⇄ true` | the existing `task.updated` event |
| First paint / boot | the Go boot payload (`WorkflowSnapshot`), via `ssr/mapper.ts` (**AC-84**) |
| REST reads | the existing task and task-session endpoints, via the DTO enrichment |

Re-deriving an **unchanged** value publishes nothing. Because parked is not a task-row mutation,
the `apps/backend/AGENTS.md` "every task-row mutation publishes via the event bus" rule is
satisfied by the existing `task.updated` publication — no new `publishTaskEvent` call site is added.

### 7.7 Internationalization

This slice introduces **no new user-facing copy**. `apps/web/src/locales/en/task.json` already
carries `"backgroundWorkIsRunning": "Background work is running"` on `main`, and
`BackgroundWorkTaskIcon` consumes it through `t()`. No hardcoded literal is added on any changed
line, so `pnpm run i18n:ratchet` has nothing to flag. Do not add a second key for the same string.

---

## 8. Acceptance criteria

EARS-style. Parent AC numbers retained as identity; text is the V1-narrowed form and is
**authoritative**. `V1-*` criteria are new to this slice.

### 8.1 Formula and fail-closed core

- **AC-21** — **GIVEN** a session that settled to `WAITING_FOR_INPUT` after a turn in which the
  Claude recogniser attested a `Detached=true`, `Kind=shell` background launch, **and** the probe
  reports `live`, **WHEN** the session DTO is serialized, **THEN** `parked_on_background_work` is
  `true`.
- **AC-22** — **GIVEN** the same session, **WHEN** the task DTO is serialized, **THEN** the task's
  `parked_on_background_work` is `true` **and** its `foreground_activity` is unchanged from today's
  value.
- **AC-24** — **GIVEN** a session that settled with **no** attested detached launch, **WHEN** its
  DTO is serialized, **THEN** `parked_on_background_work` is `false` regardless of what the probe
  reports.
- **AC-25** — **GIVEN** a session with an attested detached launch and a probe result of `settled`,
  **WHEN** its DTO is serialized, **THEN** `parked_on_background_work` is `false`.
- **AC-26** — **GIVEN** a session with an attested detached launch and a probe result of `unknown`,
  **WHEN** its DTO is serialized, **THEN** `parked_on_background_work` is `false`.
- **AC-27** — **GIVEN** a platform that cannot enumerate the agent's descendants with their start
  times (any GOOS other than `darwin` or `linux`), **WHEN** the probe is taken for a session with an
  attested detached launch, **THEN** the probe's returned value is `unknown` — never `live`, never
  `settled` — and the session DTO reports `parked_on_background_work: false`.
- **AC-36** — **GIVEN** a parked session, **WHEN** the backend restarts and the boot payload is
  built, **THEN** `parked_on_background_work` is `false`. (The projection is process memory; a
  restart loses it, which is the correct conservative direction.)
- **AC-37** — **GIVEN** an ACP agent whose payloads the inline Claude recogniser does not match,
  **WHEN** it settles a session after backgrounding work, **THEN** `parked_on_background_work` is
  `false`, **zero probes are issued for that session**, and the board card renders exactly as it
  does today; **AND GIVEN** specifically the `mock-agent` used by the `dev` and `e2e` profiles,
  driven through `/detached-background` so that `stampSubagentBackgroundWork` stamps
  `Kind=subagent, Detached=true` and `IsDetachedBackgroundLaunch()` returns **true**, **THEN** the
  same three outcomes still hold — because the backend additionally requires
  `backgroundWorkKind(payload) == streams.BackgroundWorkKindShell`. *(The second GIVEN is a
  regression guard, not a formality: `stampSubagentBackgroundWork` is an independent producer of
  `Detached=true` that admits `mockAgentID`, so an unfiltered backend predicate makes this AC false
  against the shipped tree for the agent every dev and e2e profile runs.)*
- **AC-50** — **GIVEN** a task with no sessions, or whose sessions have recorded no projection,
  **WHEN** the task DTO is serialized, **THEN** `parked_on_background_work` is `false`.
  *(Narrowed: the parent's `parked_revision == 0` clause is void under [D-2](#d2).)*
- **AC-75** — **GIVEN** a parked session, **WHEN** the operator submits a prompt that is **queued
  but not admitted** (the session is still `WAITING_FOR_INPUT`), **THEN**
  `parked_on_background_work` is still `true` and the background affordance is still rendered; and
  **WHEN** that prompt is subsequently admitted and the session enters `RUNNING`, **THEN** it
  becomes `false`. The projection tracks the machine, not the operator.

### 8.2 Settle-path safety

- **AC-40** — **GIVEN** a session whose probe cannot complete a sample within
  `KANDEV_PARKED_PROBE_BUDGET`, **WHEN** **any** CAS-confirmed transition into `WAITING_FOR_INPUT`
  computes the projection — not only a turn end — **THEN** the result is treated as `unknown`, the
  session is not parked, and **that transition is not delayed beyond the budget**; **and** at most
  **one** synchronous probe is issued per CAS-confirmed transition even when two settle paths race
  the same session, which the state CAS guarantees rather than an added lock (§7.2).
- **AC-40a** — **GIVEN** a turn settling on a session with **no** attested detached launch, **WHEN**
  the projection is computed, **THEN** no probe is taken at all and turn settlement incurs no probe
  latency.
- **AC-40b** — **GIVEN** a session settled to `WAITING_FOR_INPUT` by the **MCP clarification path**
  (`Handlers.setSessionWaitingForInput`, `internal/mcp/handlers`), which does not enter package
  `orchestrator`, **WHEN** that settle occurs, **THEN** **zero** probes are issued for that session,
  `parked_on_background_work` is `false`, and the board card renders
  `task-state-waiting-for-input`. This is a named exclusion, asserted so it is a decision rather
  than an omission.
- **AC-49** — **GIVEN** a task with two sessions S1 and S2, neither parked, **WHEN** S2 transitions
  to parked, **THEN** the task's `parked_on_background_work` becomes `true`; and **WHEN** S2 later
  un-parks while S1 is parked, **THEN** the task's value stays `true`. *(Narrowed: the parent's
  `parked_revision` independence and discard clauses are void under [D-2](#d2).)*
- **AC-68** — **GIVEN** a parked session whose probe last reported `live`, **and it is the only
  parked session on its task**, **WHEN** the session enters `RUNNING` **with no further sample being
  taken**, **THEN** `parked_on_background_work` is `false`, the change is published on the existing
  `session.activity_changed` carrier, and the board card stops rendering
  `task-state-background-running`. This is the session-state term of the formula clearing it,
  because `lastSample` is still `live` and is never re-read. *(Narrowed: the parent's "a higher
  `revision`" clause is void under [D-2](#d2); the "AC-53 stopped the loop" premise is vacuous —
  no loop exists.)*
- **V1-01** — **GIVEN** a session with an attested detached launch, **WHEN** the backend observes
  that session transition into `RUNNING`, **THEN** the attestation is cleared, so a subsequent
  settle of that session with no new attested launch computes `parked_on_background_work: false`
  and issues **zero** probes ([D-3](#d3)).
- **V1-02** — **GIVEN** the same session, **WHEN** the backend observes it transition into
  `STARTING` instead, **THEN** the attestation is likewise cleared, so a subsequent
  `STARTING → WAITING_FOR_INPUT` heal (stale-`STARTING` heal or `ResetAgentContext` restore) issues
  **zero** probes and reports `false` ([D-3](#d3)).
- **V1-03** — **GIVEN** a probe that is still in flight, **WHEN** the session's attestation is
  cleared or the session leaves `WAITING_FOR_INPUT` before the probe returns, **THEN** the completed
  sample is **discarded**: `lastSample` is not written, `parked` does not change, and nothing is
  published (§7.2 revalidation).
- **V1-04** — **GIVEN** two settle paths racing the same session, **WHEN** both attempt the
  transition into `WAITING_FOR_INPUT`, **THEN** exactly **one** wins the state CAS and exactly
  **one** synchronous probe is issued.

### 8.3 Probe — Darwin + Linux

Mechanism is implemented in `probe_darwin.go` / `probe_linux.go`. **Tests do not yet close every
one of these; that debt is in scope for this slice** (§9).

- **AC-27a** — **GIVEN** an agent whose only in-turn descendant is a **zombie**, **WHEN** the probe
  is taken, **THEN** it reports `settled`. *(Needs a test that spawns a real zombie **descendant**,
  not merely a reaped root.)*
- **AC-27b** — **GIVEN** an agent with exactly one live descendant whose process start time is
  **before** the current turn's recorded start, **WHEN** the probe is taken, **THEN** it reports
  `settled`.
- **AC-70** — **GIVEN** a Claude ACP session that is idle with **no background workload running**,
  whose agent process group contains the bridge process, one or more CLI processes, and one or more
  stdio MCP server processes (the §L-shaped tree), **and every one of those descendants has a start
  time strictly BEFORE the current turn's recorded start**, **WHEN** the probe is taken, **THEN** it
  reports `settled`. This is the regression guard against sampling process-group membership, which
  would report `live` here. *(Needs the bridge + CLI + MCP-shaped tree, not `sh`/`sleep`.)*
- **AC-70a** — **GIVEN** the same §L-shaped idle session **but with one stdio MCP server whose start
  time is at or after the current turn's recorded start** (the lazily-connected case), **WHEN** the
  probe is taken, **THEN** it reports `live`. Together with AC-70 this pins that the predicate is
  the **start time**, not the process's identity or command name. This false "still busy" is an
  accepted failure mode, now observed rather than only described.
- **AC-71** — **GIVEN** a Claude ACP session in which the agent has backgrounded a shell during the
  current turn, and that shell has been placed in **its own process group**
  (`pgid != the agent's pgid`), **WHEN** the probe is taken, **THEN** it reports `live`. Regression
  guard against sampling process-group membership, which would report `settled` here.
- **AC-72** — **GIVEN** a session whose agent has a descendant that started **before** the current
  turn and is still alive, and no descendant started during the current turn, **WHEN** the probe is
  taken, **THEN** it reports `settled`; and **GIVEN** the same session after a descendant is started
  **during** the current turn, **WHEN** the probe is taken again, **THEN** it reports `live`.
- **AC-80** — **GIVEN** a descendant whose process start time falls in the **same source-resolution
  tick** as the recorded turn start but strictly after it in nanoseconds, **WHEN** the probe is
  taken, **THEN** it reports `live`, because the turn start is truncated **down** to the source's
  resolution before the **inclusive** comparison (INV-1). This criterion exists **twice, once per
  platform**: the **Linux** instance runs on the `ubuntu-latest` backend job and **is the CI gate**;
  the **Darwin** instance is host-gated on `runtime.GOOS == "darwin"` and MUST `t.Skip` with an
  explicit platform reason when it does not run, via the `probe_notdarwin_test.go` skip-sibling, so
  a green CI log never reads as Darwin coverage and the test name is never silently absent. An
  implementation satisfying only the Linux instance has **not** satisfied AC-80. **GIVEN** an
  implementation that reads Darwin start times from `ps -eo lstart`, **THEN** this criterion fails —
  that is the intended guard against that source.
- **AC-81** — **GIVEN** `KANDEV_PARKED_PROBE_BUDGET` set to `0`, and separately to a negative
  duration, and separately to an unparseable string, **WHEN** configuration is loaded, **THEN** in
  every case the value is rejected, a warning is logged, and the effective budget is the **250ms**
  default — so no synchronous probe is ever issued without a deadline.
- **V1-05** — **GIVEN** a session for which `RecordTurnStart` has **never** been called (no prompt
  dispatched yet, so the marker `isZero()`), **WHEN** the probe is taken, **THEN** it reports
  `unknown` — never `settled`.
- **V1-06** — **GIVEN** an agent process whose `(pid, start-time)` identity was **not** captured at
  start, or whose PID's current occupant's start time does **not** match the captured identity
  (PID reuse), **WHEN** the probe is taken, **THEN** it reports `unknown` — a bare-PID fallback is
  forbidden (INV-1).
- **V1-07** — **GIVEN** the stamp is written on the operator path, **WHEN** the same session is
  dispatched via `PromptSteer` and separately via the synthetic `fireWakeup` path, **THEN** the
  recorded turn-start marker advances on **each** of those dispatches too, and in no case is the
  stamp skipped for a dispatch that reaches `conn.Prompt` ([D-1](#d1)). Asserted at `sendPrompt`'s
  three callers.

### 8.4 Transport

- **AC-45** — **GIVEN** the backend needs a liveness sample for a session, **WHEN** it takes one,
  **THEN** the request travels as the WebSocket action `agent.background.probe` on the existing
  agent stream carrying `session_id`, the request carries **no timestamp**, and the response
  `result` is one of exactly `live`, `settled`, `unknown`; **and** the `session_id` on the wire is
  the **ACP** session id, translated from the **Kandev** task-session id the **port** was called
  with, by `lifecycle.Manager` — asserted by driving the port with a Kandev id and reading the ACP
  id off the emitted frame, which fails for an implementation that passes the Kandev id straight
  through; **and** `Client.ProbeBackgroundWorkloads` is called with the **already-translated ACP**
  id, so the `Client` performs no lookup of its own.
  **AND GIVEN** a well-formed response and a **nil** error, **THEN** the port returns **that exact
  literal unchanged** — asserted separately for `live`, for `settled` and for `unknown`, so an
  implementation that emits a correct frame and then maps every successful response to `unknown`
  fails.
  **AND GIVEN** one `Probe` invocation, **THEN** **at most one** `agent.background.probe` frame is
  put on the wire for it: **no retry**, and **no coalescing** with a concurrent invocation for the
  same session — concurrent invocations are independent calls.
- **AC-46** — **GIVEN** each of these **ten** conditions in turn:
  1. the agent stream is disconnected (`ErrAgentStreamNotConnected`);
  2. the probe budget elapses;
  3. agentctl replies `ErrorCodeUnknownAction`;
  4. the response body is unparseable;
  5. the response carries a `result` outside the three literals;
  6. the port returns a **non-nil error alongside a `live` value**;
  7. the port implementation **panics**;
  8. the Kandev task-session id **cannot be translated** to an ACP session id (no execution, no live
     agentctl attachment, or an unknown session);
  9. the Kandev task-session id is the **empty string**;
  10. the caller's context is **already done on entry** (cancelled or past its deadline before the
      call);

  **WHEN** the backend resolves the probe, **THEN** in **every** case the result is `unknown` and
  the session reports `parked_on_background_work: false`; and in the **last three** cases **no
  `agent.background.probe` frame is put on the wire at all**. Table-driven — this is the backbone
  that makes the flag safe to enable.

### 8.5 Board surface

- **AC-58** — **GIVEN** a parked task at `state=REVIEW` with no pending input, no
  `foreground_activity` and not interrupted, **WHEN** its card renders on the **board**
  (`components/kanban-card-content.tsx`), **THEN** `data-testid="task-state-background-running"` is
  present. This must hold against **both** early returns: `renderTaskStatusIcon` returns `null` at
  `:275` for exactly this input — so the board renders **no icon at all** today, not a spinner —
  and returns a bare `IconLoader2` at `:282` when a launch spinner is showing. An implementation
  that changes only one of the two **fails**.
- **AC-58a** — **GIVEN** a parked task rendered on the board, **WHEN** a subsequent `kanban.update`
  arrives that does **not** carry `parked_on_background_work`, **THEN** the card still renders
  `data-testid="task-state-background-running"` — i.e. the value survives the rebuild in **both**
  `apps/web/lib/ws/handlers/kanban.ts` projections (`state.kanban.tasks` and
  `state.kanbanMulti.snapshots[…].tasks`), exactly as `foregroundActivity` already does. Without
  this, AC-58 passes in a unit test and fails on a live board.
- **AC-58b** — **GIVEN** a parked task that **also** has a pending clarification, and separately one
  that has a pending permission, **WHEN** the card renders on the **board** through the shared
  `getTaskStateIconConfig` resolver, **THEN** the pending-input affordance wins and
  `task-state-background-running` is **absent** (§7.4). *(Narrowed: the parent's `/tasks` half is
  V4 under [D-6](#d6).)*
- **AC-59** — **GIVEN** a task that is **not** parked, **WHEN** it renders through
  `getTaskStateIcon`, **THEN** the icon is **identical to today's** for the matrix already
  enumerated in `apps/web/lib/ui/state-icons.test.tsx` (task-icon describe blocks), extended with
  the new option pinned `false`. In particular the pre-existing `foregroundActivity === "background"`
  branch stays a bare `IconLoader` with **no** `data-testid`. *(Narrowed: the parent's six call
  sites and the second `task-item.test.tsx` baseline are V4 under [D-6](#d6).)*
- **AC-84** — **GIVEN** a Go boot snapshot (`WorkflowSnapshot`) in which a task carries
  `parked_on_background_work: true`, **WHEN** `snapshotToState` (`apps/web/lib/ssr/mapper.ts`) maps
  it into `KanbanState.tasks` and the **board card** renders **before any `kanban.update` has
  arrived**, **THEN** `data-testid="task-state-background-running"` is present. An implementation
  that projects the field in `toKanbanTask` and the two `kanban.ts` projections but **not** in
  `ssr/mapper.ts` **fails this criterion and passes AC-58a**, whose GIVEN is a *subsequent*
  `kanban.update`. `snapshotToState` hand-builds `KanbanTask` field by field and does not route
  through `toKanbanTask`, so it is a fourth, independent producer — and it is the one that runs on
  first paint.
- **AC-74 (V1)** — **GIVEN** a session parked on its synchronous settle sample, **WHEN** the
  background workload subsequently exits and the session remains `WAITING_FOR_INPUT`, **THEN** no
  further probe is taken and `parked_on_background_work` remains `true` indefinitely; and **WHEN**
  the session then leaves `WAITING_FOR_INPUT`, **THEN** it becomes `false`. **The stale affordance
  is accepted and specified, not a defect** ([D-5](#d5)).

### 8.6 Guards shipped day one

- **AC-76** — **GIVEN** any session that this spec parks, un-parks, or leaves unparked, **WHEN** its
  turn completes, **THEN** `session.turn_finished` is delivered within the same turn-completion
  handling; `Service.handleSemanticOccurrence` is reached with the **same `taskID`, `sessionID`,
  `occurrenceID` (the turn id) and `eventType`** as a fixture captured from the pre-change code for
  the same inputs; the resulting `notificationPayload` — `{TaskID, TaskSessionID, OccurrenceID,
  EventType, Title, Body, Payload}` — is **byte-identical** to that fixture; **exactly one**
  `InsertDelivery` occurs for that occurrence id; and **no notification is withheld, deferred,
  delayed, reordered or dropped by anything in this spec**. Asserted across the parked, un-parked,
  `unknown`-probe and no-recogniser cases. *(The notification path carries no timestamp field at
  all — `handleSemanticOccurrence` builds `models.Delivery{UserID, ProviderID, EventType,
  TaskSessionID, OccurrenceID}` and dispatches the seven-field payload above — so the byte-identical
  assertion is **total**.)*
- **AC-35** — **GIVEN** `features.claudeBackgroundPromptHandoff` is off in every profile, **WHEN** a
  session is parked and the operator submits a prompt, **THEN** the prompt follows exactly the
  admission path it follows today for that session state; **and** an architecture test asserts that
  **none of the symbols this slice introduces** — the parked projection and its accessors, the probe
  port and its production implementation, and the settle hook — references the
  `claudeBackgroundPromptHandoff` flag key, the `Features.ClaudeBackgroundPromptHandoff` config
  field, or the `claudeBackgroundPromptHandoffEnabled` /
  `claudeBackgroundPromptHandoffEnabledForSession` accessors; **and** a parked session's projection
  is computed identically with that flag forced on and forced off, for the same inputs. **The
  assertion is at SYMBOL granularity, not package granularity** — package `orchestrator`
  legitimately references that flag at `service.go:61` and `turn_activity.go:1194/:1214/:1328` for
  the unrelated `ForegroundActivity` gate, so a package-scoped assertion is false by construction
  and would prove nothing. *(Narrowed: the parent's "sampling loop" and "recogniser registry"
  symbols do not exist in V1 ([D-5](#d5), [D-7](#d7)); the remaining three are the full set this
  slice introduces.)*

### 8.7 The runtime flag

- **V1-08** — **GIVEN** a fresh checkout with no environment overrides, **WHEN** each of the `prod`,
  `dev` and `e2e` profiles is applied, **THEN** `features.parkedOnBackgroundWork` resolves to
  `false`.
- **V1-09** — **GIVEN** the flag is **off**, **WHEN** a session that would otherwise park settles to
  `WAITING_FOR_INPUT` with an attested detached launch, **THEN** **zero** probes are issued, the
  session and task DTOs serialize `parked_on_background_work: false`, and the board card renders
  byte-identically to a build without this feature.
- **V1-10** — **GIVEN** the flag is **on**, **WHEN** the same session settles, **THEN** exactly one
  probe is issued and the projection is computed. Together with V1-09 this is the disabled/enabled
  pair the flag checklist requires.
- **V1-11** — **GIVEN** an explicit `KANDEV_FEATURES_PARKED_ON_BACKGROUND_WORK` environment
  variable, **WHEN** a SQLite override for the same key is also present, **THEN** the environment
  variable wins and the admin UI reports the key as locked; and **GIVEN** no environment variable,
  **THEN** the SQLite override wins over the profile default. (Precedence: env > SQLite override >
  profile default.)
- **V1-12** — **GIVEN** the four flag layers, **WHEN** `features-contract.test.ts` and the backend
  registry/profile completeness tests run, **THEN** `parkedOnBackgroundWork` is present with exact
  key equality in `profiles.yaml`, `FeaturesConfig`, `runtimeflags/registry.go` and
  `apps/web/lib/state/slices/features/types.ts`, and `RestartRequired` is `true`.

---

## 9. Test debt closed by this slice

These are pre-existing gaps in harvested code, explicitly in scope:

1. **AC-27a** — a zombie **descendant** test. The existing coverage reaps a root, which does not
   exercise the descendant-zombie exclusion in `walkProcessTree`.
2. **AC-70 / AC-70a** — real §L-shaped trees (bridge + CLI + stdio-MCP process shapes), not
   `sh`/`sleep` stand-ins.
3. **`probe_unix_test.go` AC-label comments are scrambled** — they predate the 2026-08-09
   renumbering and point at the wrong criteria. Fix them as part of this slice; a wrong label is
   worse than none because it reads as coverage.

---

## 10. Determinism, ordering, concurrency, nil/empty, defaults

Stated explicitly so no builder is forced to invent them.

### 10.1 Ordering and tiebreak

| Flow | Rule |
|---|---|
| Probe descendant walk | The probe is an **existential** test ("does any qualifying descendant exist"), so BFS traversal order is **not observable** — the answer is identical for any order. No tiebreak column is needed or defined. Enumeration order of `/proc` (Linux) and `kinfo_proc` (Darwin) is explicitly **unspecified and irrelevant**. |
| Task-level OR | Boolean OR over members: **order-free and idempotent**. Member iteration order is not observable. |
| Two carriers for one entity | V1 defines **no** cross-frame ordering rule, because it ships no revision ([D-2](#d2)). Each carrier's last-received value wins for that carrier, exactly as `foreground_activity` behaves today. The bit is only rendered beside `WAITING_FOR_INPUT` (§7.4 step 6 is what it displaces), so a stale `true` arriving beside `RUNNING` **renders nothing** and is self-correcting. |
| Icon precedence | The **frozen total order** in §7.4. Ties are impossible: the ladder is a first-match `if` chain over mutually evaluated conditions, and the order is asserted by AC-58b and AC-59. |
| Hook ordering at the settle seam | Within `updateTaskSessionStateWithHook`: state write → existing `publishTaskSessionStateChanged` → **then** the parked hook (probe-and-project on entry to `WAITING_FOR_INPUT`, un-park on exit from it) → then `republishTaskActivityOnSettle`. The parked hook never precedes the state write, so the state it re-reads is always at least as new as the transition that triggered it. |

### 10.2 Idempotency and retry

- **Probe:** exactly **one** frame per port invocation. **No retry** on any failure — every failure
  maps to `unknown` (AC-46). **No coalescing** of concurrent invocations for the same session.
- **Attestation:** `markObservedDetached` is a set-to-true; repeated calls within one turn are a
  no-op. Clearing is a set-to-false; repeated clears are a no-op.
- **Settle:** one probe per **CAS-confirmed** transition. A repeated `WAITING_FOR_INPUT` write that
  does not change the state does not win the CAS and issues no probe.
- **Publication:** re-deriving an **unchanged** parked value publishes nothing. Only a `false→true`
  or `true→false` transition publishes.
- **Flag registration:** registry registration is process-start and single-valued.

### 10.3 Concurrency

Fully specified in §7.2. Summary: one coarse projection mutex + one per-task mutex; frozen lock
order; probe runs lock-free and its write is revalidated against `(observedDetached,
stateGeneration)` in the same critical section as the write; the state CAS is the single-probe
serialization point; the task OR is order-free.

### 10.4 Nil, empty and error behaviour

| Condition | Result |
|---|---|
| Turn-start marker never recorded (`isZero()`) | probe → `unknown` (**V1-05**) |
| Root `(pid, start-time)` identity uncaptured, or PID-occupant mismatch | probe → `unknown` (**V1-06**) |
| `root.pid <= 0` | probe → `unknown` |
| Context cancelled / budget elapsed at any point in the walk | probe → `unknown` |
| Linux boot anchor unreadable at stamp time (`!hasBootTicks`) | probe → `unknown`, never `settled` |
| `/proc` unreadable | probe → `unknown` |
| Individual `/proc/<pid>` entry unreadable (raced exit, permission) | **skipped**, walk continues — a partial snapshot does not fail the probe |
| ppid cycle in the snapshot | terminated by the `visited` guard; walk completes normally |
| Any of AC-46's ten transport conditions | `unknown`, and no frame for the last three |
| Probe implementation panics | recovered → `unknown`; the settle path is never taken down |
| `unknown` or `settled` sample | `parked = false` |
| Unsupported GOOS (Windows and all others) | `unknown` → renders exactly as today |
| Nil projection provider on a DTO | field left at `false` |
| Task with zero sessions | `false` |
| Session with no projection row | `false` |
| Flag off | `false`, zero probes |
| Empty Kandev session id | `unknown`, no frame |

**No condition anywhere in this slice yields `live` on error.** Every unknown resolves to
not-parked, which is today's rendering.

### 10.5 Defaults and boundary values

| Knob | Default | Boundary behaviour |
|---|---|---|
| `KANDEV_PARKED_PROBE_BUDGET` | `250ms` | `0`, negative, or unparseable → warn + default (**AC-81**). Never unset: no probe is ever issued without a deadline. |
| `KANDEV_PARKED_PROBE_INTERVAL` | **not read at all in V1** | V1 has no sampler; the variable is **not consulted** and setting it to any value has **no effect**. It is not an error and produces no warning. Deferred to V2 with the parent's AC-81a semantics. |
| Same-tick descendant birth | counts as **in-turn** | Turn start truncated **down** to source resolution; comparison is **inclusive** (`>=`). Error falls toward `live` (INV-1, **AC-80**). |
| Zombie descendant | **excluded** | Never counts as live (**AC-27a**). |
| Descendant born strictly before turn start | **excluded** | (**AC-27b**, **AC-72**). |
| `features.parkedOnBackgroundWork` | `false` in `prod`, `dev`, `e2e` | Precedence env > SQLite override > profile (**V1-11**). `RestartRequired: true`. |
| Linux tick resolution | `CLK_TCK = 100` → 10ms | Hardcoded per the fixed Linux kernel ABI, as ps(1)/top(1)/procps do. |
| Darwin start-time resolution | µs (`p_starttime`) | `ps -eo lstart` is a **forbidden** source (**AC-80**). |

---

## 11. Out of scope

Each item is a **named exclusion**, i.e. a contract — not silence.

- **Windows and all non-Unix probes** → `unknown`, renders as today. A real `probe_windows.go` walk
  and the Windows AC-80 instance are a follow-up platform slice.
- **Periodic sampling and live→settled auto-clear** (V2). *Accepted limitation:* a parked card can
  stay parked after the background work exits, until the session next leaves `WAITING_FOR_INPUT`
  (**AC-74 (V1)**, [D-5](#d5)).
- **`parked_epoch`, `parked_revision`, the lexicographic discard rule, `resolveParkedTriple`,
  multi-session consistency, restart/reconnect ordering, tombstones, eviction** (V3): parent AC-38,
  AC-39, AC-39a, AC-49a, AC-77, AC-78, AC-85. See [D-2](#d2).
- **All non-board surfaces** (V4): sidebar task list (AC-23, AC-34, AC-59a), `/tasks` row (AC-73a),
  session switcher and tooltips (AC-51, AC-51a, AC-52), mobile, graph nodes, pseudo-locale audit
  (AC-82), the §M precedence matrix as a frozen table, `session-parked-merge.ts`,
  `task-parked-merge.ts`, `tasks-parked*`. See [D-6](#d6).
- **`turn_started` event + barrier, full turn attribution, the public recogniser registry, and
  flag graduation** (V5): parent AC-41, AC-41a, AC-41b (full), AC-69, AC-69a, AC-79, AC-79a, and the
  §J full end-to-end AC-73. See [D-3](#d3), [D-7](#d7).
- **A `macos-latest` CI job.** [D-4](#d4).
- **Notification deferral.** Owned by the sibling spec `docs/specs/parked-notification-deferral/`.
  This slice withholds nothing (**AC-76**).
- **Persistence.** Nothing about this projection is written to SQLite or Postgres. No schema
  change, no migration, no repository method.
- **Permissions changes.** The probe port reuses the existing `CheckSessionAccess` guard, per the
  documented bare-`*BySessionID`-lookup rule in `apps/backend/AGENTS.md`. A denial maps to `unknown`
  like every other failure (AC-46), never to an auth error surfaced to a probe caller. No new
  permission, scope, or role is introduced.

---

## 12. User-visible surfaces, docs, and mobile

The complete set of surfaces this slice changes for a user. Anything not listed here is unchanged.

1. **The board card** (`components/kanban-card-content.tsx`). The only rendering surface
   ([D-6](#d6)). A parked task shows the violet `IconCircleDashed` spinner with
   `data-testid="task-state-background-running"` and the existing
   `task:backgroundWorkIsRunning` tooltip / `aria-label`, in place of the WAITING_FOR_INPUT question
   mark (or, at `state=REVIEW` with nothing else showing, in place of **no icon at all**).
2. **Settings > System > Feature Toggles.** The new `features.parkedOnBackgroundWork` entry appears
   with its label, description and risk copy. Off in every shipped profile.

**Public docs — required in this slice.** `docs/public/configuration.md` enumerates every runtime
flag in **two** tables (the snake_case config-key table around `:131-134` and the camelCase
Feature-Toggles table around `:303-307`). **Both** gain a `features.parkedOnBackgroundWork` /
`KANDEV_FEATURES_PARKED_ON_BACKGROUND_WORK` row, default off. Use `/docs-maintainer`.

**`KANDEV_PARKED_PROBE_BUDGET` is deliberately NOT documented publicly in this slice** — it is an
internal tuning knob for a feature that is off in every shipped profile, and documenting a knob for
an experiment nobody can reach invites support questions about a surface that does not exist yet.
It gets a public row when the flag is promoted to `prod: "true"`. Named exclusion, not an omission.

**Mobile parity.** `renderTaskStatusIcon` is the shared board-card renderer, so any mobile board
view that already uses it inherits the affordance with no extra work and **no capability is
desktop-only**. This slice adds **no** new mobile-specific interaction, layout, or component, and
therefore introduces no parity gap. The mobile *session* surfaces named in the parent spec
(`mobile-sessions-section.tsx`, the phone session switcher) are a different resolver and are V4
under [D-6](#d6) — their absence here is a scope boundary, not a regression: they render exactly as
they do today.

---

## 13. Test plan

- **Platform-independent (ubuntu CI):** formula ACs (21, 22, 24, 25, 26, 27, 36, 37, 50, 75),
  settle-path ACs (40, 40a, 40b, 49, 68, V1-01…V1-04), DTO serialization, AC-45/46 transport,
  AC-76 notification fixture, AC-35 architecture + behavioural clauses, flag ACs V1-08…V1-12.
- **Linux real-process (ubuntu CI — the enforced gate):** AC-27a, AC-27b, AC-70, AC-70a, AC-71,
  AC-72, AC-80 (Linux instance), AC-81, V1-05, V1-06 via `probe_unix_test.go` /
  `probe_linux_test.go`. **Fix the scrambled AC-label comments here.**
- **Darwin real-process (host-gated — macOS dev machine):** the same predicate set + AC-80 (Darwin
  instance), each `t.Skip`ping with a `runtime.GOOS` reason off-Darwin via `probe_notdarwin_test.go`.
- **Frontend unit/component:** AC-58, AC-58a, AC-58b, AC-59, AC-84.
- **E2E (Playwright, board project):** the parked board card affordance, harvested from
  `hxr:apps/web/e2e/tests/kanban/parked-session-affordance.spec.ts` and **narrowed to the board
  case only** — the `/tasks` and sidebar cases in that file are V4. The harvested spec injects the
  projection via the `__KANDEV_E2E_STORE__` bridge rather than driving a real detached process;
  keep that approach, and **drop its `parkedRevision` sentinel**, which exists only to defeat the
  `resolveParkedTriple` discard rule that V1 does not ship ([D-2](#d2)).
- **Manual showcase (macOS and/or Linux):** enable the flag, run a Claude session that backgrounds
  a detached shell (`&` / `nohup`), let it settle, confirm the board card shows
  `task-state-background-running`. *Note the one-shot limitation:* the card appears after the settle
  sample and **remains until the session resumes** — including, potentially, after the shell exits.

---

## 14. Suggested delivery order (advisory, not contract)

1. Flag entry across all four layers + disabled-path tests (safe no-op merge).
2. Turn stamp on all dispatch paths ([D-1](#d1)) + `RecordTurnStart` / `AgentPID` /
   `ProbeProcessTree` on `process.Manager`; harvest the probe tests (they depend on `AgentPID`).
3. Transport: `client_probe.go`, `manager_probe.go`, `agent_probe.go` + AC-45/46 tables.
4. Attestation on the ordered consumer (`Kind == shell` filter) + clear-on-`RUNNING`/`STARTING`
   ([D-3](#d3)).
5. Settle-hook synchronous probe + the three-term formula + `dto/parked.go` and the boolean-only
   `dto.go` field additions ([D-2](#d2)).
6. Board wiring: `kanban-card-content.tsx` both returns, `state-icons.tsx` board bits, all four
   `KanbanTask` producers, `map-task.ts`, `ssr/mapper.ts`, `backend.ts` / `http.ts` types.
7. AC-76 guard + AC-35 architecture test; close the probe test debt (§9); the two
   `docs/public/configuration.md` rows (§12).

## 15. Convergence rule

A review finding that requires **inventing a new invariant** — a new identity, retention,
ordering, or lock rule not already stated in this document — is a **SPEC DEFECT**. Route it to a
revision of this spec, never to another fix commit. That rule is what the monolith lacked, and it
is why 25 of its 32 fix commits landed in machinery this slice does not ship.
