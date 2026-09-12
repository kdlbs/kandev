---
status: current
system: office
requirements:
  - REQ-OFFICE-RUN-HISTORY-RETENTION-003
  - REQ-OFFICE-RUN-HISTORY-RETENTION-004
---

# Office Run History Retention Operations System Design

## Purpose and boundaries

This design covers the operator half of run history retention: the settings
record, the per-table preview marker, the reporting value, the health check, and
the read/write System page surface. The sweep itself, the eligibility
predicates, the batching, and the engine-parity guarantees are designed in [run
history retention](run-history-retention.md).

The two documents share one component, `internal/office/retention`, and one
scheduler goroutine. They are split because they are two contracts with two
consumers: a scheduler deleting rows, and an operator reading a page. The split
also keeps each document inside the specification size limit without cutting
either contract.

Adjacent contracts read and constrained but not owned:

- `internal/health` — `Checker`, `Issue`, and the `/api/v1/system/health`
  response the System page's health card renders.
- `internal/system/settings.Store` — the key/value settings table, already used
  by storage maintenance under one JSON key with normalization on read.

## Component: the operator surface

### Settings

One `system_settings` key, `office_run_retention`, holding a JSON document,
read through a `Get`/`Save` pair with `Normalize` on both, matching
`internal/system/storage/settings.go`. Unparseable content returns the defaults
plus a sentinel error the caller turns into a health issue, never a boot failure
(AC-OFFICE-RUN-HISTORY-RETENTION-004.4).

```json
{
  "enabled": true,
  "sweep_interval_hours": 6,
  "batch_limit": 5000,
  "routine_runs": { "window_days": 30, "floor_per_owner": 50, "warn_rows": 25000 },
  "runs":         { "window_days": 30, "floor_per_owner": 50, "warn_rows": 25000 },
  "run_events":   { "warn_rows": 250000 }
}
```

`run_events` carries only a threshold: its lifetime is its run's, so it has no
window and no floor of its own. Ranges and rejection behavior are
AC-OFFICE-RUN-HISTORY-RETENTION-004.2 and .3; validation returns a field-named
error and writes nothing, as `validateRange` does for storage maintenance.
Writes are last-writer-wins through `Store.Save`; `CompareAndSwap` is not used
because these are operator-scale settings edited from one page, and an identical
repeated write is indistinguishable from no write
(AC-OFFICE-RUN-HISTORY-RETENTION-004.5).

**Each sweep re-reads the settings from the store at its start, and skips if that
read fails.** The buffered
wake channel that re-arms the interval is process-local, so on a deployment where
several backends share one PostgreSQL database — the deployment
AC-OFFICE-RUN-HISTORY-RETENTION-002.12 exists for — it only ever reaches the
process that served the `PUT`. A backend relying on a cached effective value
could win the sweep lock still holding the policy the operator has just replaced
and delete rows the new policy retains. Re-reading costs one indexed key lookup
per sweep interval, against the risk of deleting history under a window the
operator already widened. The wake channel keeps its job — re-arming the timer
promptly in the process that saw the change — and stops being the only path by
which a change reaches a sweep.

A failed or unparseable re-read **skips the sweep** rather than falling back to
the documented defaults. Everywhere else in this design an unreadable settings
document yields the defaults and a health issue
(AC-OFFICE-RUN-HISTORY-RETENTION-004.4), which is right for reporting and for
startup because neither deletes anything. It is wrong here: the default window is
30 days, an operator who widened theirs to 3,650 has by definition configured
something longer, and falling back would delete the decade of history they
configured the system to keep. Reading settings to *show* them fails open;
reading them to *delete by* fails closed.


### The preview marker

A second `system_settings` key, `office_run_retention_preview_completed`,
holding a JSON object keyed by swept table name whose values are the timestamp
at which that table's preview completed:
`{"office_routine_runs": "...", "runs": "..."}`.

The marker is **per table, not per database**. A single global flag is unsafe
against AC-OFFICE-RUN-HISTORY-RETENTION-002.7, which lets one table fail while
the sweep as a whole still completes: if `runs` errored during the preview sweep
and `office_routine_runs` succeeded, a global flag would be written anyway and
`runs` would delete for real on the next sweep having never shown an operator a
would-delete count — the exact failure this capability exists to prevent. With a
per-table marker, `runs` is simply previewed again next sweep
(AC-OFFICE-RUN-HISTORY-RETENTION-003.4).

A table's entry is written only when that table's preview evaluation completed
successfully, is never cleared by a settings change or a restart, and is not
written by a deleting sweep. A table whose preview finds nothing still gets its
entry, so its next sweep deletes normally.

If the key is present but unparseable, every swept table is treated as **not yet
previewed** and `office_retention_preview_unreadable` is raised
(AC-OFFICE-RUN-HISTORY-RETENTION-003.10). The asymmetry is deliberate: a
spurious re-preview deletes nothing and costs one sweep, whereas assuming a
preview had completed permits a first deletion no operator ever saw. The safe
direction is the one that cannot delete.

The preview's per-table counts are uncapped by the batch limit
(AC-OFFICE-RUN-HISTORY-RETENTION-003.9): capping them would report 5,000 to an
operator holding 200,000 eligible rows, which is exactly the number the warning
exists to convey.


### Reporting

An in-memory `LastSweep` value replaced wholesale at the end of each sweep that
actually ran: start and finish times, and per **reported** table the deleted
count, a backlog flag, and an error string. Deleted counts are counted from
**committed** transactions only: when a `runs` batch is abandoned and rolled back
(AC-OFFICE-RUN-HISTORY-RETENTION-002.7), the three satellite tables whose rows
that transaction addressed report **zero** deleted for that sweep, not the counts
their statements returned before the rollback. The failure is recorded against
`runs`, but the satellites must not show rows the rollback restored — the one
surface an operator has for "what actually happened" would otherwise be wrong
precisely in the failure case it exists for. Per **swept** table it also carries
whether that table was previewed in this sweep and, when it was, its
`WouldDelete` count — without that field the preview's headline numbers, which
AC-OFFICE-RUN-HISTORY-RETENTION-003.2 and
AC-OFFICE-RUN-HISTORY-RETENTION-003.9 require an operator to see, would have
nowhere to be read from. Because the preview marker is per table, `preview` is a
per-table flag rather than one flag for the sweep. Skips are held separately and
never overwrite this value. Nothing is persisted
(AC-OFFICE-RUN-HISTORY-RETENTION-004.6, and the "no persisted sweep history"
exclusion: a growing table recording the work of the job that stops tables
growing is the same defect in a new place). Before the first sweep the value is
absent, and the surface says so rather than rendering zeros
(AC-OFFICE-RUN-HISTORY-RETENTION-004.7).

**Retained counts do not come from `LastSweep`.** They are a property of the
table, not of a sweep, and three ACs need them when no sweep has run at all:
AC-OFFICE-RUN-HISTORY-RETENTION-003.7 (threshold warning while retention is
disabled), AC-OFFICE-RUN-HISTORY-RETENTION-004.8 (surface readable while
disabled), and AC-OFFICE-RUN-HISTORY-RETENTION-002.8 (disabled means no sweep,
yet counts are still reported). A separate `RetainedCounts` value therefore
holds one count per thresholded table — produced by the census described next —
plus, for `office_routine_runs`, the top routine by retained rows for
AC-OFFICE-RUN-HISTORY-RETENTION-003.5's attribution.

**The count evaluation is a status census, not a bare `COUNT(*)`.** For each
swept table it issues one
`SELECT status, COUNT(*) FROM <table> GROUP BY status`. The retained count is the
sum of those rows, so the census costs one scan rather than two, and the same
result set is what detects a status in neither the history nor the live-state set
and raises `office_retention_unknown_status:<table>`
(AC-OFFICE-RUN-HISTORY-RETENTION-001.10). That detector has to live here rather
than in the sweep: the sweep's predicate selects `status IN (<history statuses>)`
and therefore structurally cannot observe a status it does not select.
`run_events` is thresholded but not swept and has no status column, so it keeps a
plain `COUNT(*)`.

It is refreshed on the sweep interval by the same scheduler goroutine — on that
schedule whether or not retention is enabled, since a disabled install is
precisely the one whose tables grow unattended, and is also the only install
where an unrecognized status would otherwise never be noticed — and served from
memory in between.

**The first evaluation runs at `Start`, not at the first sweep.** The first sweep
is deliberately delayed five minutes (AC-OFFICE-RUN-HISTORY-RETENTION-002.10);
hanging the first count off that tick would leave every restart with five minutes
of absent counts, and a fresh install with none at all until it had swept once —
while AC-OFFICE-RUN-HISTORY-RETENTION-003.7 and -004.8 require the threshold
warning and the surface to work on exactly those installs.

It runs **on the scheduler goroutine, not on the caller of `Start`**. The census
is three unbounded scans of the largest tables in the database, and the install
that most needs them is the one where they take longest; blocking `Start` on them
would make bounding these tables a cause of slow boots. `Start` returns
immediately, the tables read *not yet computed* until the first census lands
seconds later, and that state is already required and already renders honestly.

`RetainedCounts` is therefore **tri-state per thresholded table**, not a number
that defaults to zero: *not yet computed*, *fresh as of T*, or *stale as of T*.
Zero is a real measurement and must not be how "nobody has counted yet" renders —
the same argument AC-OFFICE-RUN-HISTORY-RETENTION-004.7 makes for `LastSweep`,
and the reason AC-OFFICE-RUN-HISTORY-RETENTION-003.11 states it. The three states
are tracked **per table**, so one table's failing query neither discards nor
staleness-marks another's successful one; a failure with no prior success leaves
that table at *not yet computed* rather than fabricating a zero.

The evaluation is explicitly **not** computed per health poll or per page load:
`/api/v1/system/health` is polled by every open browser tab, and issuing three
unbounded `COUNT(*)`s against the largest tables in the database on each poll
would make this feature a cause of the load it exists to prevent
(AC-OFFICE-RUN-HISTORY-RETENTION-003.11).

Three surfaces, in descending durability:

1. **Structured logs.** One `info` line per sweep. One `warn` line per condition
   in REQ-OFFICE-RUN-HISTORY-RETENTION-003, carrying the table, the counts, and
   the threshold. Always on, in every profile.
2. **Health issues.** The package implements `health.Checker` with
   `Name() = "Office run retention"` and `Category() = "office"`, returning a
   `health.Issue` per active condition with
   `FixURL = "/settings/system/data-storage"`, which is the live route
   registered in `apps/web/src/settings-routes.tsx` — note the suffix, as
   `/settings/system/data` is not a registered path and `/settings/system/database`
   is only a redirect to it. `internal/health/checks_test.go` pins fix URLs to
   live routes ("expectedGitHubFixURL is the live route in ..."), and this one
   is pinned the same way. This is the production-visible surface
   required by AC-OFFICE-RUN-HISTORY-RETENTION-003.8 and is why the debug
   metrics endpoint is not it: `/debug/vars` is gated on the dev profile.
   Issue ids are stable and one per condition:
   `office_retention_preview_pending`, `office_retention_backlog:<table>`,
   `office_retention_threshold:<table>`, `office_retention_disabled:<table>`,
   `office_retention_settings_invalid`, `office_retention_failed:<table>`,
   `office_retention_preview_unreadable`, and
   `office_retention_unknown_status:<table>`.
   `Check` returns issues sorted by id, matching `storage.Runtime.Check`, so the
   health card's order is stable across polls.
   For `office_routine_runs`, the threshold issue's message names the routine
   holding the largest share of retained rows and that share
   (AC-OFFICE-RUN-HISTORY-RETENTION-003.5) — attribution computed from the same
   retained-count query, not a second detector.
3. **expvar.** Counters under `office_retention_*` following
   `office/scheduler/metrics_vars.go`. Development convenience only; nothing in
   REQ-OFFICE-RUN-HISTORY-RETENTION-003 depends on it.


### HTTP and frontend

`GET /api/v1/system/retention` returns effective settings, `LastSweep`, the
separately-held skip record, and `RetainedCounts`.

`PUT /api/v1/system/retention` **replaces the whole document**; it is not a
merge patch. A field the caller omits takes its documented default rather than
its stored value, so the same request body always yields the same stored
document and a repeated identical write is genuinely a no-op — which is what
AC-OFFICE-RUN-HISTORY-RETENTION-004.5's "identical repeated write changes
nothing and returns the same normalized document" requires. A merge would break
that: under a merge, whether a request is a no-op depends on what was stored
before it. An unrecognized field, a numeric field whose value is not a whole
number, a field present with a JSON `null`, and a field whose value is of the
wrong type, are each rejected with a field-named 400 and nothing is written
(AC-OFFICE-RUN-HISTORY-RETENTION-004.9); rejecting rather than ignoring an
unknown field means a client that misspells `window_days` is told so instead of
silently getting the default. `null` needs saying because this endpoint is a full
replace: since an omitted field deliberately means "take the default", the
tempting reading of `null` is the same one, and that reading turns
`{"runs": {"window_days": null}}` into a silent 3,650-to-30-day reduction that
destroys a decade of history on the next sweep. Decode into pointer fields and
reject an explicit null, rather than into value fields where `null` and omission
are indistinguishable. Successful writes return the normalized document.

Both routes are admin-scoped like the other System routes and are readable while
retention is disabled (AC-OFFICE-RUN-HISTORY-RETENTION-004.8).

One card on **Settings > System > Data & Logs**, the page served at
`/settings/system/data-storage` and rendered by
`apps/web/components/settings/system/data-logs-settings.tsx`, beside
`database-stats-card.tsx`: the enable toggle, the numeric fields, and the
last-sweep readout. It follows the storage-maintenance cards' shape. All new
copy goes through `t()` and must ship in `pt-pt`, `zh-cn`, `zh-hk`, and `zh-tw`;
`pnpm run i18n:check` and the new-code ratchet gate the build. Health issue
titles and messages stay English, matching every other backend-produced
`health.Issue`.


## Ordering, concurrency, and failure

| Question | Answer | AC |
|---|---|---|
| Concurrent settings writes | Last-writer-wins; an identical repeat changes nothing | 004.5 |
| Settings unreadable | Defaults used, health issue raised, sweep proceeds | 004.4 |
| Settings out of range | Rejected with the field named, stored settings unchanged | 004.3 |
| Retention disabled | No sweep, no deletion; counts and threshold warnings still reported | 002.8, 003.7, 004.8 |
| Preview scope | Per swept table, not per database; a table that failed its preview is previewed again | 003.4 |
| Preview marker unreadable | Treated as not previewed; re-preview deletes nothing | 003.10 |
| Retained counts | Table property from a `GROUP BY status` census; first evaluation at `Start`, then on the sweep interval; served from memory; computed even while disabled | 003.11 |
| Counts before the first evaluation, or first evaluation fails | Reported *not yet computed* per table, never as zero; state tracked per thresholded table | 003.11, 004.7 |
| Unknown status detected | By the census, not the sweep predicate; one issue per table listing every unrecognized status in ascending order | 001.10 |
| Preview on a table with more eligible rows than the batch limit | Reports the full eligible count; never reported as backlog | 003.6, 003.9 |
| Settings changed on another backend | Each sweep re-reads settings at its start, so a process that did not serve the write still uses the new policy | 004.5 |
| Settings unreadable at sweep start | Sweep skipped and recorded as skipped; never run under the shorter default window | 004.5, 004.4 |
| Batch abandoned and rolled back | Satellite tables report zero deleted, not their pre-rollback statement counts | 002.7, 004.6 |
| Settings write shape | Full replace; omitted field takes its default; unknown field 400 | 004.9 |

## Testing

Unit tests in `internal/office/retention`, plus the frontend checks below.

- The first sweep on a seeded database deletes nothing and reports a
  would-delete count; the second deletes (003.1, 003.4).
- A preview on a database with more eligible rows than the batch limit reports
  the full eligible count (003.9).
- A table that errors during the sweep in which it was being previewed is
  previewed again on the next sweep instead of deleting, while a sibling table
  that succeeded proceeds to delete (003.4).
- A preview marker that is present but unparseable causes a re-preview, not a
  deletion, and raises `office_retention_preview_unreadable` (003.10).
- Retained counts and the threshold warning are produced on a fresh install
  before any sweep, and while retention is disabled (003.7, 003.11, 004.8), and
  a health poll does not issue a table count.
- Counts are available immediately after `Start`, without advancing any clock to
  the first sweep's delay (003.11). A test that waits out the delay would pass
  against an implementation that hangs the first count off the sweep tick, which
  is the defect this asserts against.
- Before the first evaluation, and when the first evaluation fails with no
  earlier success, the surface reports the table as *not yet computed* and not as
  zero (003.11, 004.7).
- With one thresholded table's count query failing and the other two succeeding,
  the two keep fresh counts and only the failing one is marked, and the threshold
  warnings derived from the successful counts still fire (003.11).
- A swept table holding a status in neither status set raises
  `office_retention_unknown_status:<table>` from the census while retention is
  **disabled** and no sweep has ever run (001.10, 003.11).
- A preview on a table with more eligible rows than the batch limit reports the
  full eligible count and raises no backlog warning (003.6, 003.9).
- A settings change written through one store handle is used by a sweep driven
  from a second handle that never saw the change notification (004.5).
- A sweep whose settings read fails is skipped and recorded as skipped, and
  deletes nothing under the default window (004.5).
- A `runs` batch abandoned after its retry reports zero deleted for the three
  satellite tables rather than their pre-rollback counts (002.7, 004.6).
- A recorded skip leaves the previous `LastSweep` readable and unchanged (002.2,
  004.7).
- A settings write omitting a field stores that field's default; a write with an
  unknown field or a fractional number is rejected 400 naming the field and
  stores nothing; the same write applied twice is a no-op (004.9).

Commands:

```
cd apps/backend
go test ./internal/office/... -race -count=1
KANDEV_TEST_POSTGRES_DSN=<dsn> go test -race ./internal/office/... -count=1
cd apps/web && pnpm run typecheck && pnpm run i18n:check
```

## Rejected alternatives

- **One global "preview completed" flag.** Simpler, and unsafe: because
  AC-OFFICE-RUN-HISTORY-RETENTION-002.7 lets one table fail while the sweep
  completes, a global flag is written even when a table never got its preview,
  and that table then deletes for real having shown the operator nothing. The
  per-table marker costs one JSON object.
- **Compute retained counts inside the health check.** Direct, and it puts three
  unbounded `COUNT(*)`s on a route every open browser tab polls. Refreshing on
  the sweep interval and serving from memory gives the same number without
  making the bound-the-tables feature a source of load on those tables.
- **A merge-patch settings write.** Under a merge, whether a request is a no-op
  depends on what was stored before it, which
  AC-OFFICE-RUN-HISTORY-RETENTION-004.5 forbids.
- **Deletion off by default.** Safe, and it means the gap stays open on every
  install that never visits the settings page. The per-table preview gives the
  same protection without that outcome.
- **Report warnings only through `/debug/vars`.** That endpoint is gated on the
  dev profile, so on a production build the warnings would not exist
  (AC-OFFICE-RUN-HISTORY-RETENTION-003.8).

## Prior art, applied

`internal/system/storage` supplies the settings storage and normalization
pattern, the hours-based interval with min and max bounds, and the
`health.Checker` route to a production-visible warning; `storage.Runtime.Check`
also sorts its issues by id, which this check matches so the health card's order
is stable across polls.

Neither GitLab Duo nor the Claude apps gateway previews before a policy's first
deletion or warns ahead of the window. Those are this capability's additions,
and they are the whole reason this document is separate from the sweep's.
