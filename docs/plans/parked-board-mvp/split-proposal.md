# Spec-split proposal: `disambiguate-waiting`

**Status:** revised draft (v4) — both Unix platforms in V1
**Date:** 2026-08-10

> **v4 change:** V1 now ships **both Unix platforms (Darwin + Linux)**, not Darwin-only. The
> Codex + Fable review of v3 found the premise of "Darwin-only" was incomplete: the branch
> **also** contains a complete, CI-tested **Linux** probe (`probe_linux.go`, 239 lines, with the
> boot-tick round-6 fix already applied and regression-guarded, its real-process tests running on
> `ubuntu-latest` via `probe_unix_test.go`). So "reuse the finished probe" applies symmetrically,
> and Darwin-*only* would have (a) required **new code to suppress the compiled-in, working Linux
> probe** — platform dispatch is by Go build tag, so a Linux build runs `probe_linux.go` and
> returns real values, *not* `unknown`; and (b) dropped the Linux CI gate, leaving **zero AC-80
> instances enforced on default CI** — strictly weaker than the parent contract, the real
> "green CI, unproven ability" risk. Shipping the Unix pair keeps the macOS showcase **and**
> restores the ubuntu CI gate **and** deletes the near-empty `parked-probe-linux` slice, at the
> cost of ~350 lines of already-tested code in V1's review scope. Windows stays `unknown` and is
> the only remaining platform add-on.
>
> **v3 change (superseded):** proposed Darwin-*only* for V1. Correct that Darwin is simpler
> (wall-clock `p_starttime`, no boot-tick hazard) and is the showcase env — both kept — but wrong
> that Linux was unfinished/deferrable. See v4.
>
> **v2 change (carried):** the split is **vertical** (thin end-to-end slices), not horizontal
> (seams → frontend → probe → projection → integration). The v1 layering left nothing
> user-visible until the final spec and pushed all integration risk to the end — both reviewers
> judged that a relocation of the monolith's failure, not a fix. The v1 diagnosis (isolate the
> hard pieces behind the port, freeze the concurrency invariants, add a convergence gate) is
> unchanged and carried forward.

## Why split

`disambiguate-waiting` entered Build as **one unit: 3,385 lines, 63 ACs, 8 task files**. It did
not converge: it had already been split once, still needed **5 spec-review rounds / 56
findings** to become buildable, then ran a **14-round build review-and-fix loop** (~32 fix
commits, +15.3k/−8.2k across 208 files) and stopped mid-round-14. **~25 of the 32 fix commits
are one theme reappearing:** parked-state races, sampler-goroutine shutdown ordering,
eviction/tombstone retention, lock scope, process identity (pid + start-time, boot-tick clock
domain) — concurrency edge cases discovered *during review*, never frozen in the contract.

The problem was never AC count; it was **invariant discovery under a concurrency-heavy domain,
fed through a review loop with no floor.** The fix is to ship a thin vertical slice that proves
the ability with almost no concurrency, then land the hard concurrency work as visible upgrades
to an already-live feature — each slice frozen at a reviewable size with its invariants stated
up front.

---

## THE FIRST DELIVERABLE — V1 `parked-board-mvp`

**The smallest end-to-end slice that showcases the new ability.** Deliver this first.

**Ship story.** *Behind an off-by-default runtime flag, on **macOS and Linux**, a Claude session
that settles to `WAITING_FOR_INPUT` while a detached background shell it launched is still alive
renders the background-running affordance instead of "waiting for input" on its board card —
computed once at settle, and cleared when the session resumes.* (The headline demo is on macOS —
Kandev **Desktop** is a Mac app and the reviewer runs on a Mac — but the ability ships on Linux
too, which is where the CI gate lives.)

This is the §J scenario itself (the 12-minute silent park observed 2026-08-08) made visible on
the board — the surface where the operator was actually misled (§G). It is genuinely
end-to-end: **real** recogniser attestation → **real** settle hook → **real** probe over a
**real** process tree → **real** task DTO → **real** board card. Nothing is faked; a fake probe
would prove UI wiring, not the ability. Both Unix probes already exist and are mature
(`probe_darwin.go`, `probe_linux.go`), so this slice *reuses the hardest, already-finished
component* on both platforms.

### What makes it thin (and why it dodges the churn)

The spec **already blesses the one-shot mode as permanent contract**: AC-74 —
`KANDEV_PARKED_PROBE_INTERVAL=0` means one synchronous sample at settle, no periodic sampling,
staleness-until-resume accepted, cleared by session-state change. **V1 ships as if the interval
were permanently 0.** That deletes from V1 the entire home of ~25 of the 32 fix commits:

- no sampling loop, no long-lived goroutines, no `WG.Add`/`Wait` ordering, no drain-on-shutdown;
- no live→settled auto-detection;
- no eviction/tombstone retention machinery;
- no revision/epoch protocol (the parked bit rides the same frames as session/task state and is
  only meaningful next to `state == WAITING_FOR_INPUT`; a stale `true` beside `RUNNING` renders
  nothing).

V1's concurrency surface is one coarse projection mutex. Its topology is small even though its
AC count is moderate: **one recogniser, one attestation, one synchronous probe, one in-memory
boolean, one DTO path, one surface.**

Three further narrowings, each with the acceptance named in-spec:

1. **Both Unix platforms: Darwin + Linux.** The descendant-walk + start-time predicate ships on
   both, reusing the existing `probe_darwin.go` and `probe_linux.go` (both already implement
   `(pid, start-time)` identity, zombie exclusion, cycle guard, and a caller-budget-bounded walk).
   **Windows** returns `unknown` → never parked → renders exactly as today (fail-closed,
   AC-26/27); it is the only platform add-on. This keeps the macOS showcase *and* the ubuntu CI
   gate — see *The platform tradeoff* below.
2. **No `turn_started` event.** AC-41a/41b — the barrier, cancellation admission, the round-3/4
   tombstone bounces — are deferred. V1 clears attestation on the backend's **own observed
   transition into `RUNNING`** (a state write it already owns). "Attestation never survives a
   turn the backend saw start" is a conservative invariant worth keeping forever; `turn_started`
   later *sharpens* the boundary (steer/wakeup/cancel edges), it does not replace the rule.
   Agentctl still stamps its recorded turn-start at `beginPromptTurn` (the probe predicate needs
   it) — V1 takes only the simple stamp-before-dispatch half of AC-41b, not the barrier.
3. **Claude-only inline recogniser.** V1 registers Claude through the seam's *shape* but defers
   the public registry + vendor-neutrality tests (AC-69/69a) to the seam slice.

**Turn-stamp bypass hazard (named here, resolved in the frozen spec's D-1).** V1 keeps the
turn-start *stamp* while deferring the `turn_started` event. On the branch the stamp fires inside
the ACP adapter's `syncNotifQueueThen` callback, fused with the (deferred) `turn_started`
emission, and the synthetic `ScheduleWakeup`/`fireWakeup` dispatch path can bypass a naive stamp. A
wakeup-dispatched turn that skips the stamp leaves a **stale (older) threshold**, biasing toward
`live` — cheap per the parent, but it can render a **spuriously parked card**, i.e. a wrong
showcase. **Resolved** (frozen spec §5, D-1): the stamp is placed in `sendPrompt`, unconditionally,
after `beginPromptTurn(sessionID)`, so all three dispatch callers (`Prompt`, `PromptSteer`,
`fireWakeup`) stamp on every non-dropped dispatch. The alternative — recording stale-stamp-on-
wakeup as an accepted V1 limitation — was considered and rejected, because a false park is the one
failure that discredits the whole showcase.

### V1 acceptance criteria

- **Formula & fail-closed core (full):** AC-21, AC-22, AC-24, AC-25, AC-26, AC-27, AC-36
  (boot omits → `false`, trivially), AC-37 (**both** GIVENs — the mock-agent `Kind != shell`
  guard is mandatory day one; dev/e2e run the mock agent), AC-40a, AC-40b, AC-50, AC-75.
- **Settle-path safety (subset):** AC-40 (synchronous probe whose **settlement delay is bounded
  by the probe budget** — the 250ms default; one probe per CAS transition), AC-49 (task-OR flips
  `true`; revision clauses deferred), AC-68 (subset: leaving `WAITING_FOR_INPUT` publishes
  `false`; "sampler stopped" clause is vacuous — there is no sampler).
- **Probe, Darwin + Linux (mechanism implemented in `probe_darwin.go` / `probe_linux.go`; tests
  do NOT yet close every AC — budget this):** AC-27a (zombie exclusion — needs a test with a
  zombie *descendant*, not just a reaped root), AC-27b (before-turn descendant ⇒ settled), AC-70
  and AC-70a (need the §L-shaped tree — bridge + CLI + in-turn stdio MCP — not `sh`/`sleep`),
  AC-71, AC-72, AC-80 (**both instances**: Linux on `ubuntu-latest` = the CI gate; Darwin
  host-gated `t.Skip` off-Darwin), AC-81. Also fix the **scrambled AC-label comments** in
  `probe_unix_test.go` (they predate the 2026-08-09 renumbering) — traceability is exactly what
  this split protects.
- **Transport (core clauses):** AC-45 (wire action + Kandev→ACP translation per layer),
  AC-46 (all ten conditions → `unknown`; table-driven, cheap — the backbone that makes the flag
  safe to enable).
- **Board surface only:** AC-58 (both early returns), AC-58a (both `kanban.ts` projections),
  AC-58b (**board half** — pending input must win; hiding an actionable question is the one
  regression this slice could introduce), AC-59 (board-resolver baseline), AC-84
  (`ssr/mapper.ts` first paint).
- **Guards that ship day one:** AC-76 (notification byte-for-byte fixture — V1 touches the
  settle path, so this guard earns its keep now, not in the last spec) and AC-35's behavioral
  clause (architecture test grows per slice).
- **Flag (amends the spec's "ships unflagged" section):** `parkedOnBackgroundWork` registered in
  `runtimeflags/registry.go`, `"false"` in every profile; gate at the **settle hook only**
  (flag off → no probe, no projection, fields false; the frontend needs no flag). Follow
  `/runtime-feature-flags`: registry ownership, profile defaults, disabled-path tests, env/SQLite
  override, restart semantics.

### V1 explicitly defers (with the accepted limitation named)

Periodic sampling & live→settled auto-clear (staleness-until-resume is AC-74's blessed
contract); eviction-publish/tombstones (execution-end-while-parked can leave a stale spinner
until resume — same acceptance class; V1 *may* add a trivial clear-on-execution-end publish, no
retention needed since there are no revisions); revision/epoch (AC-38/39/39a/77); multi-session
concurrency (AC-49a/78); every non-board surface; **Windows** probe; `turn_started` barrier;
the public recogniser registry.

### Invariants frozen before V1 build — already implemented on both Unix platforms

1. **Process identity = `(pid, start-time)`, compared in ONE explicit clock domain per platform,
   with zombie and before-turn-start descendants excluded.** Darwin uses **wall-clock**
   (`p_starttime`, µs resolution); Linux uses the **boot-tick domain** (ticks-since-boot). Both
   truncate the turn-start down to source resolution before an inclusive comparison, so the error
   falls toward `live`. This rule was the monolith's biggest churn source (invented piecemeal
   across rounds 9/10/14) but is **already implemented and documented** in `probe_darwin.go`,
   `probe_linux.go`, and `probe.go`. Freeze it; do not re-derive in review.
2. **The turn-start marker is materialised once, at stamp time, via `newTurnStartMarker`; probes
   never re-derive it.** Darwin doesn't strictly need this (wall-clock is stamp/probe
   indistinguishable), but **Linux does** — it freezes boot-ticks against the anchor read at that
   instant, and a "simplification" to build the marker at probe time is correct on Darwin and
   *silently wrong* on Linux. Freezing it in V1 (the branch already does this in
   `Manager.RecordTurnStart`) prevents the Linux path from becoming a V1 rework.

### The platform tradeoff (stated, not hidden)

With both Unix platforms in V1 the parent's original risk posture is **restored**, not weakened:
AC-80 is a pair — the **Linux** instance runs on `ubuntu-latest` and is the **CI gate**; the
**Darwin** instance is host-gated (`t.Skip` with a `runtime.GOOS` reason off-Darwin, via the
`probe_notdarwin_test.go` skip-sibling so the name is never silently absent). A Darwin-only
regression is the residual risk the parent explicitly accepts *because the Linux gate exists* —
and now it does. Everything platform-independent (formula, DTO, AC-45/46 transport, flag, board)
also runs on ubuntu CI. **Optional hardening:** a `macos-latest` job running
`go test ./internal/agentctl/server/process/...` closes even the Darwin-only residual — nice to
have, and worth adding before the flag graduates to `prod: "true"`, but no longer load-bearing
for V1 the way it was under Darwin-only.

> **On the deadline claim:** the walk checks `ctx` on entry and per iteration, but the
> `SysctlKinfoProcSlice("kern.proc.all")` / `/proc` enumeration syscall itself is **not
> ctx-interruptible**. So settlement delay is bounded by the caller's probe budget in the normal
> case (the syscall is a single fast read), but a pathologically stalled kernel read is not
> hard-preempted. State this honestly rather than claiming a strict deadline; it is acceptable
> for a level-sample and matches the parent's budget model.

---

## The remaining slices (each independently shippable, each adds visible value)

| # | Slug | Adds (ACs) | One-line ship story |
|---|------|-----------|---------------------|
| **V1** | `parked-board-mvp` | *(above)* | Flag on: parked Claude sessions show the affordance on the board card — **macOS + Linux**, one sample at settle. |
| **V2** | `parked-live-sampling` | AC-53, 54, 62, 73, 74, 81a, 81b + remaining AC-40/68 clauses | The affordance **clears itself** the moment background work finishes — periodic sampler, still single-session. |
| **V3** | `parked-projection-consistency` | AC-38, 39, 39a, 49a, 77, 78, 85 | Correct under restarts, reconnects, out-of-order frames, and multi-session tasks. |
| **V4** | `parked-everywhere` | AC-23, 34, 51, 51a, 52, 59 (full), 59a, 73a, 82, 83 | Sidebar task list, `/tasks`, session switcher, tooltips, mobile — all show the same precedence-correct state. |
| **V5** | `parked-turn-boundary-and-seam` | AC-41, 41a, 41b (full), 79, 79a, 69, 69a, AC-35 (full), contract amendment, flag graduation | Exact turn-boundary clearing + pluggable second-vendor recognisers; then `prod:"true"` and retire the flag. |

### Platform axis (independent of V2–V5)

Platform is orthogonal to the feature slices. **Both Unix platforms ship in V1** (their probes
and tests already exist — `probe_darwin.go`, `probe_linux.go`, shared `probe_unix_test.go`). Only
**Windows** remains an add-on, landable any time after V1:

| Slug | Adds (ACs) | One-line ship story |
|------|-----------|---------------------|
| `parked-probe-windows` | AC-80 (Windows instance, if pursued) + a `probe_windows.go` real walk | The ability works on Windows hosts; until then Windows returns `unknown` (renders as today). |

The code seam confirms this boundary: `probe_unix_test.go` is built for `linux || darwin` — the
real fault line is "Unix pair" vs. "Windows", exactly where V1 now cuts. The v3
`parked-probe-linux` slice is **deleted** — its work already exists and is in V1.

**Why this order converges where the monolith did not — the concurrency core is split across
two separate review boundaries:**

- **V2 owns sampler lifecycle only** (goroutine admission, `WG.Add`-before-`Wait`,
  reject-after-shutdown, drain; coarse per-session serialization). Freeze the **sampler shutdown
  state diagram** before build.
- **V3 owns distributed projection consistency only** (epoch + monotonic revision as a *total
  function over `(epoch, revision)` pairs*; the task-owned `members` cache and its **canonical
  lock order**; publish-failure recovery; tombstone retention derived from the durable cleanup
  horizon). Freeze the **lock order, the revision/epoch function, and the tombstone retention
  rule** before build.

  Keeping sampler-lifecycle and epoch/tombstone consistency in **different specs** is the crux:
  the monolith failed because both concurrency problems were debugged simultaneously. Each is
  now a live-feature upgrade validated against real V1 usage, not dark plumbing validated
  against nothing.

**Ordering latitude:** V4 (surfaces) is low-risk frontend over an already-proven projection and
is independently shippable — it may be pulled ahead of V3 if broadening visible coverage is
worth more than edge-case correctness in a given cycle. V5 may split into V5a (turn boundary) and
V5b (vendor seam + graduation) if its review drags; they are independent.

## AC accounting

After the atomization pass below, **every atomic AC clause / platform instance has exactly one
owner** (the plain "every AC once" claim doesn't survive the clause- and platform-splits, so it's
stated at clause granularity). Structural notes:

- **AC-41b is split by clause and called out** — its stamp-before-dispatch half is in V1; its
  barrier / cancellation-admission / process-time-attribution clauses are in V5. It is one of two
  ACs that cross a slice boundary; naming it prevents the wrong owner marking it green (the exact
  failure that demoted the old wave table).
- **AC-80 exists once per platform instance** (the parent states this explicitly): the **Darwin**
  and **Linux** instances are both owned by **V1** (Linux = the ubuntu-CI gate; Darwin =
  host-gated); a Windows instance only if that platform is pursued via `parked-probe-windows`. An
  implementation satisfying only one instance has not satisfied AC-80.
- **AC-27b is owned by V1** (its Darwin+Linux probe set) — reconcile the companion spec, whose
  probe list currently omits it.
- **Composite ACs to atomize first (preserve IDs with suffixes):** AC-45, AC-46, AC-73, AC-76,
  AC-80, AC-83, AC-85 each bundle several independently-deliverable contracts. A short mechanical
  AC-atomization pass on the spec should precede re-planning, or slice boundaries will re-encode
  the composite ambiguity.

## Convergence gate (refined by both reviews — keep, don't auto-downgrade)

The v1 "3 rounds then downgrade non-correctness findings" rule was **too blunt** — review
fatigue is not evidence a finding is cosmetic; here "retention tuning" and "test rigor" findings
repeatedly exposed real stale-state and shutdown defects. Replace it with a **classify-by-cause**
gate:

1. **A finding that requires inventing a new invariant** (a lock order, an identity rule, a
   retention rule that the spec never stated) is a **spec defect, not a code fix.** Route it to a
   `/spec --revise` of the *owning slice* and re-freeze — **never** to another fix commit.
   Applied earlier, this rule alone converts the 14-round fix loop into two or three spec
   bounces.
2. **A correctness finding against a stated invariant** → fix and re-review, no round cap.
3. **A genuinely non-correctness finding** (cosmetic, doc, test-naming) after the correctness set
   is clean → downgrade to a tracked follow-up and ship `PARTIAL READY`, **with explicit human
   acceptance** of each deferred item. `PARTIAL READY` is already a first-class ksdd terminal
   state.

## What changed in v4 (both Unix in V1 — after Codex + Fable review of v3)

- **V1 platform: Darwin-only → Darwin + Linux.** Both Unix probes are already finished and the
  Linux one is CI-tested; Darwin-only would have needed suppression code and dropped the CI gate.
  Windows is the only remaining platform add-on; `parked-probe-linux` deleted.
- **CI-gate risk fixed.** With Linux in V1, AC-80's Linux instance restores the default ubuntu CI
  gate; the `macos-latest` job drops from load-bearing to optional hardening.
- **Deadline claim corrected** — the enumeration syscall is not ctx-interruptible; settlement is
  budget-bounded in the normal case, not hard-preempted.
- **Second frozen invariant added** — marker materialised once at stamp time (Linux needs it;
  Darwin doesn't, but freezing it stops the Linux path becoming a V1 rework).
- **Turn-stamp bypass hazard named** — synthetic `fireWakeup` dispatch can skip the stamp →
  spuriously-parked card; require the stamp on all paths or accept it explicitly.
- **Probe ACs re-graded** from "largely satisfied" to "mechanism implemented; AC-27a/70/70a tests
  still owed"; flagged the scrambled AC-label comments in `probe_unix_test.go` as a V1 fix.
- Companion spec (`parked-board-mvp-darwin-spec.md`) needs the same edits + a header fix (it still
  says "per v2") — pending.

## What changed in v3 (superseded by v4)

- Proposed **Darwin-only** for V1, treating Linux as an unfinished add-on. Correct on Darwin's
  simplicity and showcase value (both kept in v4); wrong that Linux was deferrable — it is
  finished and CI-tested, so v4 ships the Unix pair.

## What changed in v2 (for reviewers who saw the first draft)

- Structure: **horizontal S1–S5 → vertical V1–V5.** No standalone "seams" or "no-op frontend"
  spec; no final stub→real "flip" — every slice wires its own backend result to a visible
  consumer.
- **Real off-by-default `runtimeflags` entry** replaces the inert `KANDEV_PARKED_*` parsing
  (inert public config creates compatibility obligations before behavior exists).
- **AC-76 moved to the first backend slice** (V1), not last.
- **`turn_started` is no longer treated as a harmless seam** — it ships in V5 with the behavior
  that needs it.
- **Concurrency core split in two** (V2 sampler-lifecycle, V3 distributed-consistency).
- **Convergence gate reclassified** from time-based to cause-based.
- **AC misassignments fixed** (v1 had backend ACs — AC-24/25/26/38 — under "frontend", and
  render-assertion ACs — AC-83/84 — under "no-op seams" where they were unfalsifiable).
