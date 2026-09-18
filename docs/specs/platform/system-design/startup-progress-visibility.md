---
status: draft
system: platform
requirements:
  - REQ-PLATFORM-STARTUP-PROGRESS-001
  - REQ-PLATFORM-STARTUP-PROGRESS-002
  - REQ-PLATFORM-STARTUP-PROGRESS-003
  - REQ-PLATFORM-STARTUP-PROGRESS-004
  - REQ-PLATFORM-STARTUP-PROGRESS-005
---

# Startup progress visibility design

## Purpose and boundaries

Platform owns the startup snapshot, so this design extends the existing
`internal/startup` reporter rather than adding a second progress channel.

What the reporter reaches today, verified rather than assumed:

- It is created and put into the startup context at
  `backendapp/startup.go:184` (`startup.WithReporter`).
- `internal/persistence` uses it (`provider.go:36`, `:101`).
- `backendapp/storage.go` uses it (`:62`, `:215`).
- The SQLite task repository reaches it through `migrationContext()`, which is
  why the backfills can report without new arguments.
- `newBootstrapHandler` already serves `Snapshot()` on `/ready`.

What it does **not** reach, and what this design therefore has to plumb:

- **Router composition.** `routeParams` (`backendapp/helpers.go:703`) carries
  `persistenceHealth` but no reporter, and `readyHandler` (`:1013`) builds its
  503 and 200 bodies without a snapshot. A reporter field is added to
  `routeParams` and populated where the other params are.
- **The desktop client.** It reads no response body today: `request_ready`
  (`desktop/src-tauri/src/backend.rs:628`) calls `read_http_response_head`
  (`:651`), which stops at the first blank line, and tests the status line alone.
  It gains an 8192-byte-capped body read and a JSON decode inside its existing 250 ms socket
  read timeout. This is new work, not an existing parse being reused.

Adjacent contracts used but not owned here: the bootstrap listener and handler
switch ([startup lifecycle](startup-lifecycle.md)), the required-store catalog in
`internal/persistence/requiredstores`, the backend localization catalog in
`internal/i18n`, and the web application's localization catalogs.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-PLATFORM-STARTUP-PROGRESS-001` | [Step model](#step-model), [Reporter API](#reporter-api), [Measuring each step](#measuring-each-step) |
| `REQ-PLATFORM-STARTUP-PROGRESS-002` | [Readiness payload](#readiness-payload) |
| `REQ-PLATFORM-STARTUP-PROGRESS-003` | [Surfaces](#surfaces), [Naming and localization](#naming-and-localization) |
| `REQ-PLATFORM-STARTUP-PROGRESS-004` | [Stall detection](#stall-detection) |
| `REQ-PLATFORM-STARTUP-PROGRESS-005` | [Step registry](#step-registry) |

## Components and responsibilities

- **`internal/startup`** owns `Phase`, `Step`, the registry, the reporter, and the
  snapshot. Its only dependency is the logger, so any startup-time package can
  import it without a cycle.
- **`internal/persistence`** opens the backup step and reports bytes written.
- **`internal/backendapp/storage.go`** opens and advances the repository store
  sweep; all 19 of its admissions run before the phase change.
- **`internal/backendapp`** (`services.go`, `orchestrator.go`,
  `storage_maintenance.go`, `main.go`) advances the service store sweep.
- **`internal/task/repository/sqlite`** opens the named backfill steps.
- **`internal/agent/runtime/lifecycle`** opens the session recovery step.
- **`internal/backendapp/httpserver.go`** serves the readiness payload and the
  server-rendered startup page from the bootstrap handler.
- **`internal/launcher`** reads the snapshot in its readiness wait.
- **`apps/web`** renders startup state inside the application document.
- **`apps/desktop`** forwards the snapshot into its existing startup window.

## Data and contracts

### Step model

`seq` sits at the snapshot top level, not inside `step`, because
AC-PLATFORM-STARTUP-PROGRESS-001.13 requires it to be present whether or not a
step is active and to increment on the transition to no step. `boot` sits beside
it and scopes it: `seq` is only comparable between two reads carrying the same
`boot`.

```json
{
  "phase": "recovering_sessions",
  "boot": 8231904417,
  "seq": 7,
  "elapsed_ms": 812431,
  "phase_elapsed_ms": 42112,
  "step": {
    "id": "sessions.recovery",
    "label_key": "startup.step.session_recovery",
    "measure": "counted",
    "unit": "sessions",
    "elapsed_ms": 41904,
    "done": 1180,
    "total": 2945,
    "rate_per_second": 28.2,
    "eta_ms": 62600,
    "since_advance_ms": 812,
    "stalled": false
  }
}
```

`total` is present only for `counted`; `done`, `rate_per_second`, and
`since_advance_ms` are absent for `opaque`; `eta_ms` is present only for `counted`
with a positive rate. The existing snapshot fields keep their names and meanings,
so a consumer that ignores `boot`, `seq`, and `step` behaves exactly as it does
today.

`boot` is a positive integer below 2^53 drawn once at reporter construction from
the same source the process uses for other non-guessable values, never a counter, a
clock reading, or a process identifier, all three of which repeat across restarts on
a restarted host. It is a number, which AC-PLATFORM-STARTUP-PROGRESS-002.4 admits; the 2^53 bound is what makes that safe, since every integer below it decodes exactly in a consumer reading JSON numbers as doubles. It is drawn in `startup.New` from `crypto/rand`, masked to 53 bits and redrawn on a zero. There is no fallback value, no last-resort composite, and no draw-failure path to specify: `crypto/rand.Read` never returns an error on the Go version this module requires, which is what lets `New` keep its error-free signature (`startup/progress.go:59`). It is never logged as an identity, never persisted, and carries
no information about the installation.

### Reporter API

`Reporter` gains `BeginStep(id)`, `SeedDone(id, done)`, `SetTotal(id, total)`,
`Advance(id, delta)`, `Degrade(id)`, and `EndStep(id)`, each guarded by the
reporter's existing mutex so a snapshot taken during a transition is internally
consistent. Package-level
`startup.BeginStep(ctx, id)` helpers follow the existing `SetPhase` pattern and
are no-ops when the context carries no reporter, which keeps every call site safe
in tests and embedded use.

Boundary behaviour, implementing AC-PLATFORM-STARTUP-PROGRESS-001.14, .15, .16 and
.19 in one place rather than at each call site:

| Call | Not the active id | Active id |
| --- | --- | --- |
| `BeginStep` | ends the active step and opens this one, `seq += 1` | no-op |
| `EndStep` | no-op | closes the step, `seq += 1` |
| `Advance(d)` | no-op | `d <= 0` no-op; else add, saturating; clamp to total |
| `SeedDone(n)` | no-op | `n < 0` rejected; equal no-op; else assign; rejected once the total is set or the step has advanced |
| `SetTotal(t)` | no-op | `t < 0` rejected; equal no-op; a different `t` accepted only while the step is still measuring, which any accepted total ends, `t == 0` included; `t == 0` blocks promotion and so seals the activation `opaque`; otherwise promotes a `counted` step, clamping a seeded `done` to `t` |
| `Degrade` | no-op | `counted`→`counting`→`opaque`, never upward; on a step already reporting `opaque` the measure is unchanged and promotion is refused for the rest of the activation |

A step whose registry entry declares `counted` opens reporting `opaque` and
promotes to `counted` on `SetTotal`, which is the single promotion point. Both
inputs AC-PLATFORM-STARTUP-PROGRESS-001.7 names are therefore known at promotion:
the total is the argument, and the initial done count is whatever `SeedDone` has
already assigned, or zero for a step that starts from nothing. Until then there is
no total to render a fraction against, so the snapshot admits it cannot report
progress. A step declaring `counting` reports `counting` from its first snapshot
with `done` at zero; a step declaring `opaque` never promotes.

Ordering is fixed rather than free: `SeedDone` is valid only before `SetTotal`, so
promotion is never observed with a done count that a later seed then moves. A seed
above the total it is later given is clamped down by `SetTotal` under the rule
AC-PLATFORM-STARTUP-PROGRESS-001.19 already states for a total below done, which is
why the seed itself needs no knowledge of the total.

`SeedDone` exists so AC-PLATFORM-STARTUP-PROGRESS-001.8's resume seed is not an
`Advance`. It assigns `done` outright rather than adding to it, appends no rate
sample, and does not touch the last-advance timestamp, so a resumed backfill seeded at 78,300 of 669,423 neither invents throughput nor resets the stall clock AC-PLATFORM-STARTUP-PROGRESS-004.8 requires a seed to leave alone. It is rejected once the total is set or the step has advanced, so no path rewinds an advancing count.

A measured total of zero never promotes. Accepting it ends measuring, and AC-PLATFORM-STARTUP-PROGRESS-001.19 admits a different total only while measuring, so no later `SetTotal` revives it: the step ends `opaque`. That is why AC-PLATFORM-STARTUP-PROGRESS-001.17 holds at a concurrent `/ready` read and not only at `EndStep` - no reader observes a zero denominator, because no such state exists.

`Degrade` exists so a failed counter query need not be a second `BeginStep`, which would move the step's start time and sequence number. Promotion and `Degrade` are the only ways a measure changes after `BeginStep`, and they cannot fight: promotion happens at most once per activation and only out of the opening `opaque`, `Degrade` only downward, and a degraded step cannot promote again before its next activation (AC-PLATFORM-STARTUP-PROGRESS-001.11). `Degrade` on a step already reporting `opaque` moves no measure but latches the activation, so a step that opened `counted` and lost its counter query before `SetTotal` never promotes; without the latch that criterion would be unfalsifiable for the step that degrades earliest.

Warnings are deduplicated inside the reporter by (activation, condition), so
AC-PLATFORM-STARTUP-PROGRESS-001.20 holds even when a loop calls a rejected
boundary every batch.

Rate is computed inside the reporter from a bounded ring of advance samples, so
every surface shows the same number and no consumer re-derives it. A sample records
the actual increase in reported `done`, not the requested delta, so an advance that
saturates or clamps at the total contributes only the work that was really counted;
recording the request would overstate throughput at exactly the moment a sweep
finishes and project an estimate out of work that never landed. An advance whose
actual increase is zero appends no sample at all, so it neither dilutes the rate nor
refreshes the window, for the same reason
AC-PLATFORM-STARTUP-PROGRESS-004.8 leaves the stall clock running through it. The window runs from the read time back to the oldest sample no older than 30 s, so elapsed always covers the silence since the last advance and the rate begins decaying the moment advancing stops rather than holding its last value for a further window; with fewer than two samples the step's start is the older point. A window that computes shorter than one
millisecond is treated as one millisecond, so two advances landing inside the same
clock tick produce a large finite rate rather than a division by zero: `+Inf` and
`NaN` have no JSON number encoding and would break every consumer, which is the
boundary AC-PLATFORM-STARTUP-PROGRESS-001.9 closes.

`eta_ms` is `(total - done) / rate * 1000`, since `rate` is per second, emitted raw: rounding for display is the
surface's job. An estimate above 86400000 ms is omitted entirely rather than
emitted or clamped, per AC-PLATFORM-STARTUP-PROGRESS-001.10 — a rate small enough
to project more than a day is measurement noise, and "no estimate" is the honest
report of it. Surfaces already render a `counted` step with no estimate, because
a zero rate produces the same shape.

### Step registry

```go
type StepSpec struct {
    ID          StepID
    Phase       Phase
    Measure     Measure
    Unit        Unit
    LabelKey    string
    Dialects    []Dialect
    Corpus      string   // record set the reported count covers, "" when Measure is opaque
    Outstanding string   // predicate selecting work still outstanding, "" when done is not seeded
    Order       []string // the loop query's existing ordering columns, outermost
                         // first, empty when the loop walks no ordered row set
}
```

`LabelKey` is a localization key, not a rendered string, and it is the only name a
step has (AC-PLATFORM-STARTUP-PROGRESS-005.4). `Unit` is an enum (`rows`,
`messages`, `turns`, `sessions`, `bytes`, `stores`), not a free string, so the
localized noun and its plural are chosen by the rendering surface rather than
shipped in the payload.

`Corpus`, `Outstanding`, and `Order` are the declaration
AC-PLATFORM-STARTUP-PROGRESS-005.1 requires, and they live in the struct, not only in the table below, because that criterion makes the registry the artifact the completeness tests read.
The table below is the rendering of those fields, not a second source: test 6
asserts the two agree. `Corpus` and `Outstanding` are the declared identity of
what is counted, checked by review and by the count query that reads them, not a
query fragment the reporter executes.

The three are not all owed by every measured step, which is why
AC-PLATFORM-STARTUP-PROGRESS-005.1 conditions each one separately and closes with
a step declaring nothing it does not have. A step that reports any count declares
`Corpus`. Only a step seeding `done` from durable state declares `Outstanding`, and
only a step advancing over a query-ordered row set declares `Order`: after the
2026-09-18 retirement recorded below, no registered step declares either. The store
sweeps and session recovery walk a catalog slice and a Go map, which have an
iteration order but no ordering columns, and naming one would invent a contract the
loop does not have; all three count forward from zero on every boot and have no
resume predicate to name. Both fields stay in the struct and stay conditioned
separately, because AC-PLATFORM-STARTUP-PROGRESS-001.8 and 001.21 still bind the
next step that resumes or rides an ordered loop, and `SeedDone` remains the
reporter's only seeding entry point. An `opaque` step declares none of the three.

| Identifier | Phase | Measure | Unit | Dialects | Corpus | Outstanding | Order |
| --- | --- | --- | --- | --- | --- | --- | --- |
| `database.backup` | `backing_up_database` | `counting` | bytes | SQLite | staging file size in bytes | n/a | n/a |
| `stores.repositories` | `applying_migrations` | `counted` | stores | both | catalog entries whose descriptor names this sweep | n/a | n/a |
| `stores.services` | `initializing_services` | `counted` | stores | both | catalog entries whose descriptor names this sweep | n/a | n/a |
| `task.prompt_seq.backfill` | `applying_migrations` | `opaque` | rows | both | n/a | n/a | n/a |
| `task.message_timestamps.backfill` | `applying_migrations` | `opaque` | rows | both | n/a | n/a | n/a |
| `task.subagent_context.backfill` | `applying_migrations` | `opaque` | rows | both | n/a | n/a | n/a |
| `sessions.recovery` | `recovering_sessions` | `counted` | sessions | both | the `executors_running` record set read before the sweep | n/a | n/a |

Three identifiers were retired on 2026-09-18, after this design first froze, and
AC-PLATFORM-STARTUP-PROGRESS-005.6 bars their reuse: `task.journal.turns.backfill`,
`task.journal.messages.backfill`, and `plugins.session_events.mirror`. Upstream
`2eee90d38` ("test: close conversation storage follow-up gates", #3742) deleted
`backfillConversationJournal`, both of its count helpers, and
`syncAllCommittedSessionEvents` in full, dropped `conversation_turn_versions`,
`conversation_message_versions`, and `conversation_session_streams`, and removed the
mirror's `provider.go` call site without a replacement. The successor
`conversation_session_revisions` table is trigger-maintained and treats revision zero
as correct for a session that has not changed, so no batch backfill is left to
measure. The 2026-09-17 incident's two named steps are therefore unreachable on this
code. What replaced them, `cleanupLegacyConversationJournal` (`base_schema.go:65`),
is one transaction of DDL with no row loop and no unit to count, so it registers no
step; that exclusion is deliberate, and a later change that gives it a row loop is
exactly what the registry checks below exist to catch. The three identifiers move
into the append-only retired list beside the registry, which `init()` already panics
on at process start and which completeness test 5 exercises; that list, not this
paragraph, is what enforces AC-PLATFORM-STARTUP-PROGRESS-005.6.

Completeness tests give the registry teeth:

1. Every `requiredstores.Catalog()` entry is claimed by exactly one sweep step
   (AC-PLATFORM-STARTUP-PROGRESS-005.2). Every catalog entry already has an admission site: the 36 `recordRequiredStore` call sites cover all 41 because `recordPluginStores` (`services.go:1420`) is one site looping over six ids. What no descriptor carries is a sweep assignment, so this test fails until all 41 have one.
2. The registered identifier set equals the list in
   AC-PLATFORM-STARTUP-PROGRESS-005.3 exactly, in both directions.
3. Every `LabelKey` resolves in every supported backend locale and every supported
   web locale.
4. Every step's declared `Phase` matches the phase its call site runs in
   (AC-PLATFORM-STARTUP-PROGRESS-005.7).
5. Retired identifiers live in an append-only list and cannot be re-registered.
6. Declarations match the measure, and nothing is declared that the step does not
   have. A `counting` or `counted` entry declares a non-empty `Corpus`; an `opaque`
   entry leaves `Corpus`, `Outstanding`, and `Order` all empty. A non-empty
   `Outstanding` requires `Measure == counted`, because a resume seed is only
   meaningful against a total. `Outstanding` and `Order` are otherwise independently
   optional, for the reasons above. Every field the table renders must match its cell, which is what keeps the two renderings from drifting. Whether a step that could resume or could declare an order *ought*
   to is a review question, not a test one: no unit test can know that a loop has an
   ordered query it failed to declare, which is why
   AC-PLATFORM-STARTUP-PROGRESS-005.1 puts the obligation on the registry entry.

## Control flow

### Measuring each step

- **Database backup** (`backing_up_database`): `counting` in bytes. A single
  `VACUUM INTO` (`persistence/snapshot.go:63`) cannot report rows, and its output
  is defragmented so the source file size is not a valid total. A ticker stats the
  staging file and calls `Advance` with the delta. An `ENOENT` is not a measurement failure at any
  point in the step: `VACUUM INTO` creates the file only once it begins writing, so
  the ticker reports no bytes and waits. Any other stat error calls `Degrade` once, not per
  tick. Postgres skips backup entirely, which is why the step
  declares SQLite only.
- **Store admission sweeps** (`applying_migrations`, then
  `initializing_services`): the two sweeps split on the existing
  `startup.SetPhase(ctx, startup.InitializingServices)` at `storage.go:215`.
  Runtime order decides the owner, not file and not definition line: everything
  admitted before that call executes belongs to `stores.repositories`, everything
  after it to `stores.services`. All 19 `storage.go` admissions are
  `stores.repositories`, the five inside `provideSupportRepos` (defined at `:240`,
  admitting at `:249`, `:261`, `:271`, `:281`, `:291`) included, because
  `provideRepositories` calls it at `:107` - same function, before `:215`.
  `stores.services` is `services.go`, `orchestrator.go`, `storage_maintenance.go`,
  and `main.go`. A sweep
  cannot infer its own share at runtime, so `requiredstores.Descriptor` gains a
  sweep field naming which of the two owns that entry. That single declaration is both the source of each sweep's
  total, read once at `BeginStep`, and what the
  AC-PLATFORM-STARTUP-PROGRESS-005.2 check asserts is total and non-overlapping.
  `recordRequiredStore` advances the sweep its descriptor names, by one.
- **The two journal backfills and the plugin session-event mirror**: retired with
  their work by `2eee90d38`, as recorded above. They were this design's only
  `SeedDone` and `Order` callers; both reporter paths remain, exercised by unit
  tests rather than by a registered step, and `SetTotal` stays the single promotion
  point for the `counted` steps that survive.
- **Prompt sequence, message timestamp, and subagent context backfills**
  (`applying_migrations`): `opaque`. `backfillPromptSeq` is one `UPDATE` plus one
  `INSERT … SELECT`, the timestamp migration is a single `UPDATE`
  (`base_migrations.go:208`), and `migrateSubagentContextBackfill` is a single
  `INSERT … SELECT`. None has a loop to advance from, and inventing one would
  change the work being measured.
- **Session recovery** (`recovering_sessions`): `counted` in sessions. The total is
  the record set actually handed to the fan-out, after `recoverableRecords`
  (`manager_lifecycle.go:96`) drops confirmed-passthrough sessions - not the raw
  inventory read, because a passthrough record is never a re-tracking candidate and
  counting it would put the total permanently out of reach. The sweep advances by
  one per instance the consumer loop (`manager_lifecycle.go:122`) finishes
  reconstructing, which is the only per-record point at which
  AC-PLATFORM-STARTUP-PROGRESS-001.21's rule is met. It cannot advance inside the
  fan-out: `RecoverInstances` (`executor_registry.go:138`) returns only the
  instances a backend revived, so no backend can report that it examined and
  declined a record, and six backends share that signature. Nor is `RecoverAll`'s
  return (`:114`) such a point: it yields partial results beside an error, and
  reconstruction, deadline expiry and refusal all happen after it. A record no
  backend revived, or one the loop declines, is therefore never counted, so this
  step too can end below its total, which AC-PLATFORM-STARTUP-PROGRESS-001.22 makes
  audible. It declares no order because it advances in no query's order (`RecoverAll`
  iterates a Go map, `executor_registry.go:137`). A failed inventory read leaves the
  set unknown rather than empty (`manager_lifecycle.go:70`) and skips recovery
  outright, so the step calls `Degrade` once and ends `opaque`: reporting an unknown
  inventory as a measured zero is the dishonest count
  AC-PLATFORM-STARTUP-PROGRESS-001.11 exists to prevent. A read that succeeds and
  finds nothing is the other case: the total is honestly zero, and
  AC-PLATFORM-STARTUP-PROGRESS-001.17 ends the step without ever reporting
  `counted`. Nothing is counted while a record is merely selected.

Degradation is uniform: any counter source that fails calls `Degrade` once and the
work continues. No measurement is on a failure path.

### Readiness payload

The bootstrap handler already returns the snapshot on `/ready` and gains `seq` and
`step`. `readyHandler` gains the same `startup` object on every branch, which
requires the `routeParams` plumbing named above. Its existing
`persistenceHealth` branch keeps its 503, its `starting` status value, its
`reason` and its `store_ids`, and carries a `ready` snapshot beside them: the
snapshot phase is what distinguishes "still starting" from "started and since
degraded" (AC-PLATFORM-STARTUP-PROGRESS-002.3). `/health` is untouched.

### Naming and localization

`Phase.Label()` in `startup/progress.go:25` returns hardcoded English and is used
by the launcher's child-exit message (`launcher/health.go:370`). It stays as it
is: it is diagnostic output, which `docs/i18n.md` keeps in English.

Step names are a different thing and must not reuse it. The snapshot carries the step's `LabelKey` as `label_key`, because the web application and the desktop shell read the wire, not the Go registry, and AC-PLATFORM-STARTUP-PROGRESS-005.4 forbids building a name from the identifier. The backend-rendered page resolves it through `i18n.T`/`i18n.Tf` with
the locale from `i18n.FromRequest` (cookie, then accepted languages, then default). That chain is the one AC-PLATFORM-STARTUP-PROGRESS-003.9 names, but `parseAcceptLanguage` (`i18n/i18n.go:124`) does not yet fall through as the criterion requires: it coerces an unparseable `q` to 1.0, promoting a malformed entry to first place, and it accepts `q=0`. Both are corrected there, not worked around here, so every caller gets the same rule. The web application resolves the same key through i18next. Counts
beside a unit noun use `Tf` with a `count` argument so `_one`/`_other` selection
happens in the catalog, never by assembling an ending in Go or TypeScript.

Rendering an estimate is the surface's job: 60000 ms or less renders in whole
seconds rounded up with a floor of one second, anything longer in whole minutes
rounded up.

### Stall detection

The reporter records the time of the last advance that increased `done`. A resume
seed (`SeedDone`, which is not an advance at all), a zero-or-negative advance, an advance for a non-active identifier, and an
advance clamped to a total already reached all leave that timestamp alone, so none
of them masks a stall. `stalled` is computed at read time as
`since_advance_ms >= 120000` for `counted` and `counting` steps, and is always
false for `opaque` steps.

### Surfaces

- **Server-rendered startup page.** The bootstrap handler answers a request that
  both classifies as an application route and, per
  AC-PLATFORM-STARTUP-PROGRESS-003.3, gives `text/html` a strictly greater `Accept`
  quality value than `application/json` with a self-contained HTML page instead of
  the JSON body. An equal or lower quality keeps the JSON, so
  `application/json;q=1, text/html;q=0.1` stays a JSON client. The status stays 503, and the
  response carries no-store cache headers so no intermediary can serve it after
  startup. The page is rendered by Go from `internal/i18n`, embeds no bundle, and
  polls `/ready` every second with one request in flight, abandoning a request after 5000 ms so a hung connection cannot hold that slot and skipping any tick that falls while one is still outstanding, and loads the application when the snapshot reports ready rather than on a successful status: AC-PLATFORM-STARTUP-PROGRESS-002.3 has a post-startup degradation answer unsuccessfully with a `ready` snapshot.
  The route test is `webapp.IsSPARoute` minus the paths the bootstrap handler answers
  itself, `/ready` and `/health`:
  `classifyNonSPA` (`webapp/routes.go:66`) enumerates only `/api`, `/ws`, `/health`
  and static paths, so `/ready` itself classifies as an application route, and an
  HTML-preferring browser navigating to it would otherwise be answered with the page
  instead of the snapshot AC-PLATFORM-STARTUP-PROGRESS-002.2 requires. The exclusion
  belongs to the bootstrap handler; global route classification is unchanged.
  This is the only surface that can work before the SPA is serveable, because the
  boot payload needs repositories that do not exist yet.
- **Application document.** The existing restart flow polls system info and learns
  nothing while the backend is down. It gains a `/ready` poll on the same
  one-second cadence, bounded exactly as the page's poll is - a request abandoned
  after 5000 ms, a tick skipped while one is outstanding - and with no give-up while
  the dialog is open: while the backend is unreachable, including between process exit and listener bind, the restart
  dialog keeps the last snapshot it read and labels it as last known rather than
  blanking or showing an error. Before any snapshot has been read at all there is
  no phase and no elapsed time to keep, so the dialog shows the waiting state of
  AC-PLATFORM-STARTUP-PROGRESS-003.13 instead: a statement that Kandev is starting
  and not yet reachable, with no step name, no timer, and no bar. The same component renders when a loaded document
  finds the backend starting.
- **Launcher.** `probeReadyStatus` (`launcher/health.go:379`) already decodes the
  snapshot. The printed line gains the step name, and gains each of `done`,
  `total`, the rate, and the estimate whenever the snapshot carries that field,
  rather than only for a `counted` step, since `database.backup` carries a done
  count and a rate with no total. The existing 15-second
  floor is kept, with an immediate line on a phase change, on a `seq` change, and
  on a stall-state transition. The
  8192-byte `io.LimitReader` at `:427` stays sufficient: the snapshot adds
  bounded enum and integer fields.
- **Desktop.** `wait_for_ready` (`src-tauri/src/backend.rs:550`) polls `/ready` already but reads no body; with the body read named above it decodes the snapshot and calls the existing `set_status` bridge
  (`:215`, `window.__KANDEV_DESKTOP_SET_STATUS`), so the existing startup window
  shows the step without a new window or a new IPC surface. Its per-request bound is
  the 250 ms connect and read timeout `request_ready` already sets (`:634`, `:637`),
  so a hung connection cannot stall the refresh; the wait itself never gives up. The desktop shell is
  a separate bundle with no translation pipeline, so its strings stay in the
  default locale; it formats the estimate with the same rounding rule. A response it
  cannot parse, or one carrying no snapshot, is resolved by
  AC-PLATFORM-STARTUP-PROGRESS-002.6 and rendered by
  AC-PLATFORM-STARTUP-PROGRESS-003.13: it keeps the last snapshot it read and labels
  it as last known, or shows the waiting state if it has never read one. Neither case is a startup failure.

## Failure and recovery

Progress reporting is strictly best-effort and never on a failure path. A counter
query error, a stat error on the backup staging file, an unregistered identifier,
or a malformed snapshot each degrade the report and leave the work running.

A stalled step changes no behavior: nothing cancels, retries, or times out because
of it. The existing listener-health budget and its overrides are the only
mechanisms that can end a slow startup.

## Persistence

Nothing here persists. The snapshot lives in process memory for one process
lifetime, and `seq` is not comparable across processes; `boot` is what makes that
detectable, so a document holding `seq: 42` from a previous process replaces its
retained state on the restarted process's `seq: 0` instead of discarding it as
stale (AC-PLATFORM-STARTUP-PROGRESS-001.13). No registered step resumes from durable
state once the three retired identifiers are gone, so every current step counts
forward from zero on each boot; AC-PLATFORM-STARTUP-PROGRESS-001.8 governs the next
step that does.

## Security

The snapshot is unauthenticated, so it carries only registry enums and integers.
No path, SQL, error text, credential, or task, session, workspace, or user
identifier enters it. The row counts it does expose describe corpus size only, and
the default bind is loopback.

The server-rendered startup page contains no operator-supplied or database-derived
text; every string comes from the backend catalog and every number is an integer
from the snapshot.

## Observability

Existing phase-transition logs are kept. Step transitions log at info with the
step identifier, its measure, and its final counts. Degradations, unregistered
identifiers, rejected boundary values, and a done count exceeding its total each
log one warning per activation naming the step. No new metrics export is added.

## Related decisions

- [Database upgrade safety](../../../decisions/0008-db-upgrade-safety.md)
