# Spec: `parked-board-mvp` — first vertical slice (Darwin + Linux)

**Status:** draft for review
**Parent:** `docs/specs/disambiguate-waiting/spec.md` (63 ACs) — this is its **first vertical
slice**, per the split proposal (v4). This spec is the CONTRACT for the slice; the parent remains
the source for AC text.
**Platform decision:** **both Unix platforms (macOS/Darwin + Linux) ship in V1.** Windows returns
`unknown` (never parked, renders exactly as today) until a follow-up slice.
**Date:** 2026-08-10

---

## Why

`disambiguate-waiting` (parked-on-background-work) failed to converge as one 63-AC unit. The
split proposal cuts it into thin vertical slices; this is the smallest — one real end-to-end path
that lights a single surface. It ships on **both Unix platforms** because:

1. **Both Unix probes already exist and are mature.** `apps/backend/internal/agentctl/server/
   process/probe_darwin.go` **and** `probe_linux.go` each implement the full descendant walk,
   `(pid, start-time)` identity, zombie exclusion, cycle guard, and a caller-budget-bounded walk,
   with real-process tests shared in `probe_unix_test.go` (built `linux || darwin`). **The
   OS-hard part — the single biggest churn source in the monolith — is done for both.** Shipping
   only one would mean writing *new* code to suppress the other (dispatch is by Go build tag, so a
   Linux build runs `probe_linux.go` and returns real values, not `unknown`).
2. **Linux keeps the CI gate; macOS keeps the showcase.** The backend test job runs on
   `ubuntu-latest`, so the Linux probe's real-process ACs are the enforced gate. macOS is the
   headline demo environment (Kandev **Desktop** is a Mac app; the reviewer runs on a Mac) and its
   ACs are host-gated on top. Shipping the pair gets both.
3. **Darwin is technically simpler than Linux here** (kept from v3, still true): `p_starttime` is
   wall-clock, so Darwin avoids the boot-tick re-derivation hazard the Linux probe handles
   (`probe.go`, `turnStartMarker` doc comment). Linux's hazard was already hit, fixed (round-6),
   and regression-guarded — so both are safe to freeze, not just Darwin.

## Ship story

> Behind an off-by-default runtime flag, on **macOS and Linux**, a Claude session that settles to
> `WAITING_FOR_INPUT` while a detached background shell it launched during that turn is still
> alive renders `task-state-background-running` on its **board card** instead of
> `task-state-waiting-for-input` — computed by one synchronous probe at settle, and cleared when
> the session next leaves `WAITING_FOR_INPUT`.

Real end to end: real Claude recogniser attestation → real settle hook → **real process-tree
probe** → real task/session DTO → real board card. Headline demo on macOS; CI enforcement on
Linux.

---

## The platform tradeoff, stated (not hidden)

With both Unix platforms in V1 the parent's **original** risk posture is restored, not weakened:

- **AC-80 is a pair.** The **Linux** instance runs on `ubuntu-latest` and is the **CI gate**. The
  **Darwin** instance is **host-gated**: it runs when the backend suite executes on a Mac (a dev
  machine, or an added macOS CI job) and MUST `t.Skip` with an explicit `runtime.GOOS != "darwin"`
  reason otherwise — via the `probe_notdarwin_test.go` skip-sibling, so a green ubuntu log never
  reads as Darwin coverage *and* the test name is never silently absent.
- **Residual risk:** a *Darwin-only* regression is invisible to default CI. The parent accepts
  this **because the Linux gate exists** — and now it does. This is the parent contract, inherited
  intact (v3's Darwin-only would have dropped the Linux half and left zero AC-80 instances gated).
- **Optional hardening:** a `macos-latest` job running `go test ./internal/agentctl/server/
  process/...` closes even the Darwin-only residual. Nice to have; worth adding before the flag
  graduates to `prod: "true"`, but no longer load-bearing for V1.
- Everything platform-independent (formula, DTO, transport error-mapping, flag, board rendering)
  runs on ubuntu CI as normal.

---

## What is reused vs. new

**Reused as-is (already on the branch — this slice depends on, does not rebuild):**
- Both Unix probes: `probe_darwin.go` and `probe_linux.go` (`captureRootIdentity`,
  `newTurnStartMarker`, `walkProcessTree`) and the shared `(pid, start-time)` identity model in
  `probe.go`; the shared real-process tests in `probe_unix_test.go`.
- `KANDEV_PARKED_PROBE_BUDGET` parsing + 250ms default (`parseProbeEnvBudget`, AC-81).
- The `agent.background.probe` wire action shape.

**New or narrowed for this slice:**
- The **runtime flag** gating the settle-path probe (see Flag).
- The **one-shot projection subset**: synchronous probe at the settle hook only; the three-term
  formula; clear on next `WAITING_FOR_INPUT` exit. **No sampler goroutine, no revision/epoch, no
  tombstone.**
- The **Claude-only inline recogniser** + `Kind == shell` filter (public registry deferred).
- **Board-card wiring only** (one surface).
- **Test debt to pay in this slice** (not new code, but not yet closed): a zombie-*descendant*
  test for AC-27a, §L-shaped trees for AC-70/70a, and fixing the **scrambled AC-label comments**
  in `probe_unix_test.go` (they predate the 2026-08-09 renumbering).

---

## Requirements (narrowed from parent)

- A Claude session that settled to `WAITING_FOR_INPUT` after a turn in which a detached
  `Kind=shell` launch was attested, and whose synchronous probe reports `live`, SHALL be projected
  as **parked** and render distinctly on the **board card** — on macOS and Linux.
- Parked is a **projection**, not a lifecycle state. `TaskSessionState` gains no member.
- Liveness is derived by **one synchronous level-sample at settle** (the parent's blessed
  `INTERVAL=0` mode, AC-74). No periodic sampling in this slice.
- The projection is **conservative**: true only when Kandev observed the detached launch **and**
  the probe positively reports `live`. `settled`/`unknown`/no-attestation ⇒ renders as today.
- **Notification behaviour SHALL be byte-for-byte unchanged** (AC-76).
- Off by default: with the flag off, no probe is issued, the fields are `false`, and behaviour is
  byte-identical to today.
- An agent with **no registered recogniser** is never attested, never probed, never parked
  (AC-37).

## The one-shot model (why the concurrency core is absent)

Parent **AC-74** makes `KANDEV_PARKED_PROBE_INTERVAL=0` a permanent, contracted mode: one
synchronous sample at settle, no periodic sampling, staleness-until-resume accepted, cleared by
session-state change. **This slice ships as if the interval were permanently 0.** That removes the
sampling loop, sampler-goroutine lifecycle (`WG.Add`/`Wait`, reject-after-shutdown, drain),
live→settled auto-detection, and eviction/tombstone retention — **the home of ~25 of the
monolith's 32 fix commits.** The parked bit rides existing session/task frames and is only
meaningful beside `state == WAITING_FOR_INPUT`; a stale `true` beside `RUNNING` renders nothing,
so no revision/epoch protocol is needed yet. Concurrency surface: one coarse projection mutex, no
long-lived goroutine.

## Frozen invariants (already implemented on both Unix platforms — freeze, don't re-derive)

1. **Process identity = `(pid, start-time)`, compared in ONE explicit clock domain per platform,
   with zombie and before-turn-start descendants excluded.** Darwin uses **wall-clock**
   (`p_starttime`, µs resolution); Linux uses the **boot-tick domain** (ticks-since-boot). Both
   truncate the turn-start **down** to source resolution before an **inclusive** comparison, so a
   same-tick birth counts as in-turn (error falls toward `live`). Implemented in `probe_darwin.go`,
   `probe_linux.go`, documented in `probe.go`. This was the monolith's biggest churn source
   (invented piecemeal across rounds 9/10/14) — here it is contract.
2. **The turn-start marker is materialised once, at stamp time, via `newTurnStartMarker`; probes
   never re-derive it.** Darwin doesn't strictly need this (wall-clock is stamp/probe
   indistinguishable), but **Linux does** — it freezes boot-ticks against the anchor read at that
   instant. A "simplification" to build the marker at probe time is correct on Darwin and
   *silently wrong* on Linux. The branch already does this in `Manager.RecordTurnStart`; freezing
   it prevents the Linux path from becoming a rework.

## Turn-boundary handling (narrowed — no `turn_started` event)

`turn_started` (AC-41a/41b, the barrier + cancellation-admission machinery) is **deferred**. This
slice clears attestation on the backend's **own observed transition into `RUNNING`** — a state
write it already owns. "Attestation never survives a turn the backend saw start" is a conservative
invariant worth keeping permanently; the later `turn_started` slice **sharpens** the boundary
(steer / `ScheduleWakeup` self-resume / cancellation edges), it does not replace this rule.
Agentctl still records its turn-start stamp at `beginPromptTurn` (the probe predicate needs it) —
this slice takes only the stamp-before-dispatch half of AC-41b.

**Turn-stamp bypass hazard (must be resolved in V1, not discovered).** On the branch the stamp
fires inside the ACP adapter's `syncNotifQueueThen` callback, fused with the (deferred)
`turn_started` emission, and the synthetic `ScheduleWakeup`/`fireWakeup` dispatch path can bypass
a naive stamp. A wakeup-dispatched turn that skips the stamp leaves a **stale (older) threshold**,
biasing toward `live` — cheap per the parent, but it can render a **spuriously parked card**, i.e.
a wrong showcase. **Decision required:** V1 either (a) requires the stamp on **all** dispatch paths
including `fireWakeup`, or (b) records stale-stamp-on-wakeup as an explicit accepted V1 limitation.

## Flag

- Register `parkedOnBackgroundWork` in `apps/backend/internal/runtimeflags/registry.go`;
  `"false"` in every profile in `profiles.yaml`.
- Gate at the **settle hook only**: flag off ⇒ no probe, no projection, fields `false`. The
  frontend needs no flag (it renders whatever the DTO carries).
- Follow `/runtime-feature-flags`: registry ownership, profile defaults, **disabled-path tests**,
  env + SQLite override, restart semantics.
- This amends the parent's "ships unflagged" section for the sliced delivery.

---

## Acceptance criteria (this slice)

AC text is the parent's. Probe ACs cover **both Unix platforms**.

**Formula & fail-closed core (new projection work):** AC-21, AC-22, AC-24, AC-25, AC-26, AC-27,
AC-36, AC-37 (both GIVENs — incl. the `mock-agent` `Kind != shell` guard for dev/e2e), AC-40a,
AC-40b, AC-50, AC-75.

**Settle-path safety (new, subset):** AC-40 (synchronous probe whose **settlement delay is bounded
by the probe budget** — 250ms default; one probe per CAS transition), AC-49 (task-OR flips `true`;
revision clauses deferred), AC-68 (subset: leaving `WAITING_FOR_INPUT` publishes `false`; the
"sampler stopped" clause is vacuous — no sampler exists).

**Probe — Darwin + Linux (mechanism implemented in `probe_darwin.go` / `probe_linux.go`; tests do
NOT yet close every AC — budget this):** AC-27a (zombie *descendant* ⇒ settled — needs a test that
spawns one, not just a reaped root), AC-27b (before-turn descendant ⇒ settled), AC-70 (§L idle,
all descendants pre-turn ⇒ settled — needs the bridge+CLI+MCP-shaped tree, not `sh`/`sleep`),
AC-70a (one in-turn stdio MCP server ⇒ live), AC-71 (in-turn backgrounded shell in its own pgid ⇒
live), AC-72 (start-time predicate pair), AC-80 (**both instances**: Linux on `ubuntu-latest` =
the CI gate; Darwin host-gated `t.Skip` off-Darwin), AC-81 (budget default).

**Transport (core clauses, new wiring):** AC-45 (wire action + Kandev→ACP id translation in
`lifecycle.Manager`, literal passthrough in `Client`), AC-46 (all ten failure conditions ⇒
`unknown`; table-driven — the backbone that makes the flag safe to enable).

**Board surface only (new frontend):** AC-58 (both `kanban-card-content.tsx` early returns),
AC-58a (survives rebuild in both `kanban.ts` projections), AC-58b (**board half** — pending input
wins), AC-59 (board-resolver baseline unchanged), AC-84 (`ssr/mapper.ts` first paint).

**Guards (ship day one):** AC-76 (notification byte-for-byte fixture — this slice touches the
settle path), AC-35 behavioural clause.

**Flag (new, amends parent):** the `parkedOnBackgroundWork` registry entry + disabled-path tests +
override/restart semantics.

---

## Out of scope (deferred, with the accepted limitation named)

- **Windows probe** → `unknown` (renders as today). A `probe_windows.go` real walk and the Windows
  AC-80 instance are a follow-up platform slice. *(Linux is now in scope — the v3 deferral is
  reversed.)*
- **Periodic sampling & live→settled auto-clear** (parent V2). *Accepted limitation:* a parked
  card can stay parked after the background work exits, until the session next leaves
  `WAITING_FOR_INPUT` — exactly AC-74's blessed staleness. May add a trivial clear-on-execution-end
  publish (no retention needed — no revisions).
- **Revision/epoch, multi-session consistency, restart/reconnect ordering, tombstones**
  (parent V3): AC-38, AC-39, AC-39a, AC-49a, AC-77, AC-78, AC-85.
- **All non-board surfaces** (parent V4): sidebar (AC-23), `/tasks` (AC-73a), session switcher /
  tooltips (AC-51/51a/52), mobile, pseudo-locale (AC-82), graph nodes; §M precedence matrix as a
  frozen table.
- **`turn_started` barrier + full turn attribution + public recogniser registry** (parent V5):
  AC-41, AC-41a, AC-41b (full), AC-69, AC-69a, AC-79, AC-79a; the §J full E2E (AC-73).

---

## Test plan

- **Platform-independent** (ubuntu CI): formula ACs, DTO serialization, AC-45/46 transport,
  AC-76 notification fixture, AC-35, flag disabled-path + override + restart.
- **Linux real-process (ubuntu CI — the enforced gate):** AC-27a, AC-27b, AC-70, AC-70a, AC-71,
  AC-72, AC-80 (Linux instance) via `probe_unix_test.go` / `probe_linux_test.go`. **Fix the
  scrambled AC-label comments** here as part of the slice.
- **Darwin real-process (host-gated — macOS dev machine or optional `macos-latest` job):** the same
  predicate set + AC-80 (Darwin instance). Each `t.Skip`s with a `runtime.GOOS` reason off-Darwin
  via `probe_notdarwin_test.go`.
- **Manual showcase (macOS and/or Linux):** enable the flag, run a Claude session that backgrounds
  a detached shell (`&`/`nohup`), let it settle, confirm the board card shows
  `task-state-background-running`. *Note the one-shot limitation:* the card appears after the settle
  sample and **remains until the session resumes** — including, potentially, after the shell exits
  (V1 does not re-sample; that is V2).
- **Convergence gate (from the proposal):** a review finding that requires *inventing a new
  invariant* (a new identity/retention/lock rule) is a **spec defect → route to `/spec --revise` of
  this slice**, never another fix commit.

## Suggested delivery order (advisory)

1. Flag entry + disabled-path tests (safe no-op merge).
2. Recogniser (Claude, `Kind==shell`) + attestation on the ordered consumer; clear-on-`RUNNING`;
   resolve the turn-stamp bypass decision.
3. Settle-hook synchronous probe via the port → `probe_darwin.go` / `probe_linux.go`; AC-45/46
   transport.
4. Three-term formula + DTO fields (session + task-OR); AC-21–27b, 36, 40, 40a, 40b, 49, 50, 75.
5. Board wiring: `kanban-card-content.tsx` both returns, both `kanban.ts` projections,
   `ssr/mapper.ts`; AC-58/58a/58b/59/84.
6. AC-76 guard + AC-35 clause; close probe test debt (Linux on CI; Darwin on macOS); fix
   `probe_unix_test.go` AC labels.

## Open questions

1. **Turn-stamp bypass:** require the stamp on all dispatch paths (incl. `fireWakeup`) in V1, or
   accept stale-stamp-on-wakeup as a named limitation? (Affects showcase correctness.)
2. **Add a `macos-latest` CI job now**, or accept dev-machine-only Darwin coverage for the slice?
   (Optional now that Linux gates AC-80; recommended before flag graduation.)
3. **Include the trivial clear-on-execution-end publish in this slice**, or leave stale-until-
   resume strictly to V2? (Cheap here; slightly widens scope.)
4. **Board vs. `/tasks` as the single surface.** Board chosen (the §G/§J misread was the board);
   `/tasks` (AC-73a) is a near-free sibling if a second surface is wanted in V1.
