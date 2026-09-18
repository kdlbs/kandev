---
status: draft
system: platform
created: 2026-09-17
owners:
  - kandev
---

# Startup progress visibility requirements

## Overview

A Kandev upgrade can spend tens of minutes initializing. An operator sees one
repeating phase line and an unreachable board, and cannot tell which migration is
running, how much is left, or whether it is stuck.

Platform already owns startup phases, liveness, and readiness
([startup lifecycle](startup-lifecycle.md)). This capability adds the layer below a
phase: the named unit of work inside it, its measured progress, and delivery of
both to every startup surface.

It refines **AC-PLATFORM-STARTUP-LIFECYCLE-001.4**, which forbade every percentage.
It survives as AC-PLATFORM-STARTUP-PROGRESS-001.6 and 001.7, which it now cites.

No external prior-art source was reachable while this was written, so nothing here
is confirmed against another product.

## Terminology

- **Phase:** An existing coarse stage of startup, `opening_database` through
  `ready`, owned by [startup lifecycle](startup-lifecycle.md).
- **Step:** One named unit of work inside a phase, identified by a protocol-owned
  identifier; at most one is active at a time.
- **Measure:** How a step reports work done: `counted` (done and total known),
  `counting` (done only), or `opaque` (elapsed time only).
- **Activation:** One interval during which a given identifier is the active step;
  an identifier may be activated more than once per process.
- **Snapshot:** The startup state a caller reads in one atomic observation: startup
  identifier, phase, sequence number, active step, measure, counts, timing, stall.
- **Startup surface:** The launcher output, the desktop shell window, a browser
  reaching Kandev before it is ready, or an open application document.
- **Sweep step:** One step covering a set of same-shaped units, counting over that
  set rather than opening one step per unit.
- **Progress:** What AC-PLATFORM-STARTUP-PROGRESS-003.1, 003.4, and 003.7 require of
  a surface for the active step: for `counted`, done and total with the unit and a
  determinate bar; for `counting`, done with the unit; for `opaque`, elapsed time and
  the AC-PLATFORM-STARTUP-PROGRESS-001.5 statement; plus the estimate whenever the
  snapshot carries one. The rate is not among them; only the launcher reports it,
  under AC-PLATFORM-STARTUP-PROGRESS-003.5.

## Requirements

### REQ-PLATFORM-STARTUP-PROGRESS-001: Measured startup steps

**Intent:** Replace "applying migrations for 13 minutes" with the name of the work
in progress and an honest measurement of how much remains, so an operator can
decide whether to wait.

#### Acceptance criteria

- **AC-PLATFORM-STARTUP-PROGRESS-001.1:** While work belonging to a registered step
  is running, the snapshot shall report that step's identifier, label key, measure, unit, and elapsed milliseconds, and shall report the phase the process is live in, not the
  phase the registry declares for the step.
- **AC-PLATFORM-STARTUP-PROGRESS-001.2:** When no registered step is running, the
  snapshot shall report the phase with no step and shall not retain an earlier
  step's identifier.
- **AC-PLATFORM-STARTUP-PROGRESS-001.3:** A `counted` step shall carry a done count
  and a total, both non-negative integers, with done at most total.
- **AC-PLATFORM-STARTUP-PROGRESS-001.4:** A `counting` step shall carry a done count
  and omit the total.
- **AC-PLATFORM-STARTUP-PROGRESS-001.5:** An `opaque` step shall omit both counts,
  and every surface shall state that it cannot report progress rather than show a
  bar at any position.
- **AC-PLATFORM-STARTUP-PROGRESS-001.6:** A completion percentage shall be derived
  only from a `counted` measure, and no surface shall render a percentage,
  determinate bar, or completion fraction for a `counting` or `opaque` step.
- **AC-PLATFORM-STARTUP-PROGRESS-001.7:** A step intending to report `counted` shall
  report `opaque` until its total and initial done count are both known, and shall
  never report `counting`, an assumed total, or a placeholder total while measuring.
- **AC-PLATFORM-STARTUP-PROGRESS-001.8:** A step resuming after durably applied work
  shall report the whole corpus as its total and the corpus size minus the rows
  still requiring work as its initial done count, clamped to that range; the two
  counts need not observe one database snapshot.
- **AC-PLATFORM-STARTUP-PROGRESS-001.9:** For a `counted` or `counting` step that
  has advanced at least once, the snapshot shall carry a rate in units per second
  over a trailing 30000 millisecond window, falling back to the step's start below
  two samples, decaying toward zero once advancing stops, and treating a window
  shorter than one millisecond as one millisecond.
- **AC-PLATFORM-STARTUP-PROGRESS-001.10:** For a `counted` step with a rate above
  zero, the snapshot shall carry a remaining-time estimate of outstanding divided by
  rate, omitted for any other measure, a zero or unmeasured rate, or a result above
  86400000 milliseconds.
- **AC-PLATFORM-STARTUP-PROGRESS-001.11:** When a total or done count cannot be
  obtained, the step shall degrade one way (`counted` to `counting`, `counting` to
  `opaque`), record one warning, continue the underlying work, and never strengthen
  its measure again before its next activation.
- **AC-PLATFORM-STARTUP-PROGRESS-001.12:** A snapshot read concurrently with a step
  transition shall observe one consistent state and shall never pair one step's
  identifier with another step's counts, unit, or timing.
- **AC-PLATFORM-STARTUP-PROGRESS-001.13:** Each snapshot shall carry a sequence
  number beside its phase whether or not a step is active, starting at zero,
  increasing by exactly one per step transition, and never decreasing or wrapping
  within a process lifetime, plus a startup identifier drawn once per process that
  is neither a counter nor a clock reading; consumers shall compare sequence numbers
  only within one startup identifier, discarding a lower one, ordering equal ones by
  the snapshot's process elapsed milliseconds and preferring the later read on an
  exact tie, and shall replace all retained state whenever the startup identifier
  changes.
- **AC-PLATFORM-STARTUP-PROGRESS-001.14:** Beginning an already-active step shall
  leave its start time, counts, measure, and sequence number unchanged, and ending a
  step that is no longer active shall change no snapshot field.
- **AC-PLATFORM-STARTUP-PROGRESS-001.15:** Beginning a step while another is active
  shall end that step and open the new one as one transition with a single increment
  and no observable no-step interval; ending with none opened after it is likewise
  one transition. Steps shall not nest.
- **AC-PLATFORM-STARTUP-PROGRESS-001.16:** A done count that would exceed a `counted`
  total shall be reported as equal to the total, with one warning naming the step.
- **AC-PLATFORM-STARTUP-PROGRESS-001.17:** A step whose measured total is zero shall
  end without ever reporting `counted`, and no surface shall render a fraction,
  percentage, or bar with a zero denominator; it may still be open while it measures
  that total.
- **AC-PLATFORM-STARTUP-PROGRESS-001.18:** Each registered step shall declare its
  applicable database dialects and shall not be opened, reported, or rendered under
  any other dialect, with no empty or zero-valued step rendered in its place.
- **AC-PLATFORM-STARTUP-PROGRESS-001.19:** An advance of zero or less, a total equal
  to the current one, and any advance or total naming a non-active identifier shall
  each change nothing; a negative total shall be rejected; a different total shall be
  accepted only while still measuring; a total below done shall clamp done; an
  advance that would overflow shall saturate.
- **AC-PLATFORM-STARTUP-PROGRESS-001.20:** Each distinct warning condition shall be
  recorded at most once per activation.
- **AC-PLATFORM-STARTUP-PROGRESS-001.21:** A step added to an existing loop shall
  preserve that loop's ordering and tiebreak columns as the registry records them,
  and shall count a unit done only once its work is durably committed, never when it
  is merely selected.
- **AC-PLATFORM-STARTUP-PROGRESS-001.22:** A `counted` step ending below its total
  shall record one warning naming the step, its done count, and its total.

### REQ-PLATFORM-STARTUP-PROGRESS-002: Stable startup readiness contract

**Intent:** One unauthenticated endpoint that answers "starting, and here is what it
is doing", then "ready", without changing shape or disappearing.

#### Acceptance criteria

- **AC-PLATFORM-STARTUP-PROGRESS-002.1:** `GET /ready` shall be answerable from listener
  bind to shutdown and shall never return a not-found status in that window.
- **AC-PLATFORM-STARTUP-PROGRESS-002.2:** While startup is incomplete, `GET /ready`
  shall return an unsuccessful status carrying the status value `starting`, the
  service identity, the running version, and the snapshot.
- **AC-PLATFORM-STARTUP-PROGRESS-002.3:** After startup completes, `GET /ready` shall
  carry the service identity, the running version, and a `ready` snapshot with no
  step, returning a successful status unless an existing post-startup health
  condition makes it unsuccessful, which keeps its own status, status value, and
  explanatory fields beside that same snapshot; consumers shall tell "starting" from
  "since degraded" by the snapshot phase, not the status value.
- **AC-PLATFORM-STARTUP-PROGRESS-002.4:** The snapshot shall contain only
  protocol-owned identifiers, numbers, and booleans, and no free text, file path, SQL
  statement, error message, credential, or task, session, workspace, or user
  identifier.
- **AC-PLATFORM-STARTUP-PROGRESS-002.5:** `GET /health` shall keep its existing body,
  status, and desktop token header, and shall not carry the snapshot.
- **AC-PLATFORM-STARTUP-PROGRESS-002.6:** A consumer that cannot parse the snapshot,
  or receives a body without one, shall treat the status code as authoritative: a
  status reporting readiness means ready, and any other status means starting and
  shall render AC-PLATFORM-STARTUP-PROGRESS-003.13.

### REQ-PLATFORM-STARTUP-PROGRESS-003: Startup visible on every surface

**Intent:** An operator should reach the answer from wherever they are looking,
including a browser pointed at a migrating backend.

#### Acceptance criteria

- **AC-PLATFORM-STARTUP-PROGRESS-003.1:** While startup is incomplete, a browser
  navigating to an application route shall receive a rendered startup page carrying
  the phase, step name, and progress, not a machine-readable error body or an
  unreachable application shell.
- **AC-PLATFORM-STARTUP-PROGRESS-003.2:** The startup page shall poll every 1000
  milliseconds with at most one request in flight, retry after any failed or
  unparseable response without abandoning the poll while open, load the application
  once readiness is reported, and meanwhile render
  AC-PLATFORM-STARTUP-PROGRESS-003.13.
- **AC-PLATFORM-STARTUP-PROGRESS-003.3:** The startup page shall be served only for a
  `GET` on an application route whose `Accept` header resolves HTML, by the most
  specific matching media range, to a quality strictly greater than the one it
  resolves for `application/json`; any other method, any non-application route, an
  absent or malformed `Accept`, and any header not clearing that bar, including a
  bare `*/*`, shall keep the machine-readable body and status.
- **AC-PLATFORM-STARTUP-PROGRESS-003.4:** An open application document awaiting a
  backend restart shall show phase, step name, and progress while it starts, retry
  on the AC-PLATFORM-STARTUP-PROGRESS-003.2 interval without giving up, and render
  AC-PLATFORM-STARTUP-PROGRESS-003.13 while the backend is unreachable, including
  between process exit and listener bind, and resolve an unparseable or snapshot-free response by AC-PLATFORM-STARTUP-PROGRESS-002.6.
- **AC-PLATFORM-STARTUP-PROGRESS-003.5:** The launcher shall write one line per read that observes a phase, sequence-number, or stall-state change, and exactly one further line per 15000 milliseconds while none of those changes, each carrying phase, elapsed time, and the step name when one is active, plus done, total, rate, and estimate whenever the snapshot carries them. A transition not observed between two reads shall not be reported.
- **AC-PLATFORM-STARTUP-PROGRESS-003.6:** When the backend exits during startup, the
  launcher shall name the last observed step as well as the last observed phase.
- **AC-PLATFORM-STARTUP-PROGRESS-003.7:** The desktop shell shall display phase, step
  name, and progress while awaiting readiness, refresh on the
  AC-PLATFORM-STARTUP-PROGRESS-003.2 interval, replace them with the application once
  ready, resolve an unparseable or snapshot-free response by
  AC-PLATFORM-STARTUP-PROGRESS-002.6, and treat neither as a startup failure.
- **AC-PLATFORM-STARTUP-PROGRESS-003.8:** Every startup string rendered by the web
  application or the backend shall be translated in each supported locale for that
  surface, and a count rendered beside a noun shall use plural-aware translation
  rather than an assembled ending.
- **AC-PLATFORM-STARTUP-PROGRESS-003.9:** Every backend-rendered startup string shall
  resolve for the locale derived from the request, preferring the locale cookie, then
  the accepted languages, then the default, falling through on any missing, empty,
  malformed, or unsupported value.
- **AC-PLATFORM-STARTUP-PROGRESS-003.10:** An estimate of 60000 milliseconds or less
  shall render in whole seconds rounded up with a floor of one second while work
  remains, a longer one in whole minutes rounded up, never sub-second.
- **AC-PLATFORM-STARTUP-PROGRESS-003.11:** A startup surface shall remain usable
  without animation under a reduced-motion preference and shall expose the current
  step and progress to assistive technology as a live region.
- **AC-PLATFORM-STARTUP-PROGRESS-003.12:** The startup page shall carry an
  unsuccessful status and response headers instructing caches and intermediaries not
  to store or reuse it; behaviour those headers cannot control, such as session
  history, is out of scope.
- **AC-PLATFORM-STARTUP-PROGRESS-003.13:** A snapshot carrying no step shall render
  phase and elapsed time alone; an unreadable or absent response shall render the
  last snapshot read, labelled as such; and before any snapshot has been read the
  surface shall state only that Kandev is starting and not yet reachable. No case
  shall render a placeholder step name, an empty progress element, a bar, or an
  unread phase or elapsed time.

### REQ-PLATFORM-STARTUP-PROGRESS-004: Stuck distinguished from slow

**Intent:** A backfill that is slow and a process that is wedged look identical
today. They must not.

#### Acceptance criteria

- **AC-PLATFORM-STARTUP-PROGRESS-004.1:** The snapshot shall carry milliseconds
  elapsed since the active step's done count last advanced, measured from the step's
  start before its first advance.
- **AC-PLATFORM-STARTUP-PROGRESS-004.2:** When a `counted` or `counting` step has not
  advanced for at least the stall threshold, the snapshot shall report it as stalled.
- **AC-PLATFORM-STARTUP-PROGRESS-004.3:** The stall threshold shall be 120000
  milliseconds and not operator-configurable.
- **AC-PLATFORM-STARTUP-PROGRESS-004.4:** When the count advances, the step shall stop
  being reported as stalled and the elapsed-since-advance value shall reset.
- **AC-PLATFORM-STARTUP-PROGRESS-004.5:** An `opaque` step shall never be reported as
  stalled, including while it measures a total it intends to report as `counted`.
- **AC-PLATFORM-STARTUP-PROGRESS-004.6:** Every surface shall distinguish a stalled
  step from a running one, state how long it has gone without progress, and reflect a stall-state change within one refresh interval.
- **AC-PLATFORM-STARTUP-PROGRESS-004.7:** A stalled step shall not cancel, restart,
  time out, or otherwise alter the work.
- **AC-PLATFORM-STARTUP-PROGRESS-004.8:** Only an advance that increases the reported
  done count shall reset the elapsed-since-advance value; a resume seed, an advance of
  zero or less, one naming a non-active identifier, and one clamped to a total already
  reached shall each leave it running.

### REQ-PLATFORM-STARTUP-PROGRESS-005: No new silent startup work

**Intent:** The 2026-09-17 incident was two steps that shipped with no reporting.
A third must not.

#### Acceptance criteria

- **AC-PLATFORM-STARTUP-PROGRESS-005.1:** The startup step registry shall be the only
  source of valid step identifiers and shall declare each one's phase, measure, unit,
  label key, and applicable dialects; a step reporting a count shall declare the corpus
  that count covers; a step seeding its done count from durable state shall declare the
  predicate selecting the work still outstanding; and a step advancing over a
  query-ordered row set shall declare that query's ordering and tiebreak columns. A
  step shall declare nothing it does not have.
- **AC-PLATFORM-STARTUP-PROGRESS-005.2:** Every required persisted store in the
  catalog shall be counted by exactly one store-admission sweep step and never
  reported as a step of its own, and an automated check shall fail on an entry
  counted by none or by more than one.
- **AC-PLATFORM-STARTUP-PROGRESS-005.3:** Exactly these identifiers shall be
  registered: `database.backup`, `stores.repositories`, `stores.services`,
  `task.prompt_seq.backfill`, `task.message_timestamps.backfill`,
  `task.subagent_context.backfill`, and `sessions.recovery`; an automated check
  shall fail on a registered identifier absent from this list or a listed
  identifier that is unregistered.
- **AC-PLATFORM-STARTUP-PROGRESS-005.4:** An automated check shall fail when a
  registered step's label key is missing from any supported backend or web locale,
  and no surface shall build a step name from the identifier.
- **AC-PLATFORM-STARTUP-PROGRESS-005.5:** Beginning a step with an unregistered
  identifier shall record one warning naming it, open no step, leave the sequence
  number unchanged, and not fail startup.
- **AC-PLATFORM-STARTUP-PROGRESS-005.6:** A removed step identifier shall never be
  reused: `task.journal.turns.backfill`, `task.journal.messages.backfill`, and
  `plugins.session_events.mirror`, retired with their work by `2eee90d38`.
- **AC-PLATFORM-STARTUP-PROGRESS-005.7:** Opening a registered step outside its
  declared phase shall record one warning while the snapshot reports the live phase,
  and an automated check shall assert that each step's declared phase matches its call
  site.

## Out of scope

- **Making startup faster.** Backfill performance belongs to its own change. The one
  cost added is AC-PLATFORM-STARTUP-PROGRESS-001.8's total measurement; no latency
  budget is set for it.
- **Cancelling, skipping, retrying, or reordering startup work.** Every surface here
  is read-only.
- **Progress for work after readiness.** Background maintenance, retention sweeps,
  and post-ready jobs keep their reporting.
- **A dedicated version endpoint.** An undocumented version path's 404 is answered
  by `GET /ready`, which carries the version.
- **Localizing the desktop shell.** Its window is a separate bundle with no
  translation pipeline, so its strings stay in the default locale;
  AC-PLATFORM-STARTUP-PROGRESS-003.8 covers the web and backend surfaces.
- **Per-store startup steps.** An earlier draft gave each of the 41 catalog entries
  its own step: 41 names per locale, for work whose duration is not what the
  operator waits on. AC-PLATFORM-STARTUP-PROGRESS-005.2's two sweeps report the
  same coverage; a store later found slow can be promoted.
- **Sharpening "bounded periodic status" in AC-PLATFORM-STARTUP-LIFECYCLE-001.4.**
  AC-PLATFORM-STARTUP-PROGRESS-003.5 bounds the launcher; tightening that clause
  belongs to its own requirement.
- **Persisting progress across process lifetimes.** The snapshot describes the
  running process; sequence numbers are scoped to its startup identifier.
- **An operator-configurable stall threshold, a progress log file, a metrics export,
  or a step per DDL statement.** The snapshot and existing structured logs are the
  only outputs; a step is per sweep or named backfill.

## System design

See [startup progress visibility](../system-design/startup-progress-visibility.md).
