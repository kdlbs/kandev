---
status: draft
system: office
requirements:
  - REQ-OFFICE-ROUTINE-CATCHUP-003
---

# Office Routine Catch-Up System Design, part 2: renaming the policy

Part 1 ([routine-catch-up-01.md](routine-catch-up-01.md)) covers the tick, the
claim, the gap summary and its delivery to the agent — REQ-001 and REQ-002.
This part covers REQ-003 alone: renaming `enqueue_missed_with_cap` to
`summarize_missed` across every surface that names it, and the test plan for
both parts.

The split is by deliverable, not by convenience. Part 1 changes what the system
*does* on resume; this part changes what every surface *says* about it, which is
a wider but shallower blast radius — a Go enum, six creation paths, a SQLite
table rebuild, two frontend files, five locale catalogs and two design
documents.

## Renaming the policy

`models.CatchUpPolicySummarizeMissed = "summarize_missed"` replaces
`CatchUpPolicyEnqueueMissedWithCap` as the canonical value.
`CatchUpPolicyEnqueueMissedWithCap` is retained as a deprecated alias constant,
not deleted, so the normalizer has a name to compare against.

One normalization funnel, `models.NormaliseCatchUpPolicy(string) RoutineCatchUpPolicy`:

| Input | Result |
|---|---|
| `summarize_missed` | `summarize_missed` |
| `enqueue_missed_with_cap` | `summarize_missed` (deprecated alias) |
| `skip_missed` | `skip_missed` |
| empty, or anything else | `summarize_missed` |

`RoutineCatchUpPolicy.Valid()` accepts the alias so an API client sending the
old value gets 200, not 400; the persisted and returned value is the new one,
satisfying AC-003.2 and AC-003.4 together.

**Where the funnel is applied.** AC-003.1 binds every creation path, not just
the HTTP one, so the enumeration has to be exhaustive:

| Entry | Why it is on the list |
|---|---|
| `routines/handler.go` create | Sets its own default when the request omits the field |
| `routines/handler.go` update | Assigns the request value straight onto the routine |
| `Repository.CreateRoutine` | **Injects the literal old value** when the field is empty |
| `Repository.CreateRoutineTx` | Same injection, separate copy of the code |
| `CreateDefaultCoordinatorRoutine` | Hard-codes the old constant on the pre-installed routine |
| Read paths (`GetRoutine`, `ListRoutines`) | Normalize the alias and unknown values on the way out |

The two repository entries are the ones a line-by-line reading of the HTTP layer
misses, and they are not hypothetical: config-sync's routine reconciler builds a
`models.Routine` with **no `CatchUpPolicy` set at all** and calls
`CreateRoutineTx` directly, bypassing the handler and its defaulting. Left
unchanged, every config-sync-created routine would keep writing the retired
string forever, after the one-time migration had cleaned up every pre-existing
row — a permanent AC-003.1 violation on a live path.

**Read-side mechanism.** `GetRoutine` and `ListRoutines` are `SELECT *` through
`sqlx` `StructScan`/`SelectContext`; there is no post-scan hook to put the
normalizer in, so "the row scanner" is not an existing function to edit.
Implement `sql.Scanner` on `RoutineCatchUpPolicy` and normalize there. That
covers every present and future read path in one place, where per-call
post-processing would have to be added to each new query and would be forgotten
by exactly the same reasoning that produced this defect. `driver.Valuer` is not
needed: writes already go through the funnel above.

Note the earlier claim that "Office routines are not part of the config-sync
YAML contract" is about the *field* — the YAML carries no catch-up policy — and
must not be read as "config-sync does not create routines". It does.

**Migrating persisted rows and the stored default.** Two separate problems, and
one `UPDATE` only solves the first.

Row *values* migrate with a single `r.migrate.Apply` step:
`UPDATE office_routines SET catch_up_policy = 'summarize_missed' WHERE catch_up_policy = 'enqueue_missed_with_cap'`.
With the `sql.Scanner` normalization above this is belt-and-braces rather than
load-bearing — a row that escapes it still reads correctly.

The column's *stored default* needs more, and AC-003.3 requires it on upgraded
installs too. Editing the `DEFAULT 'enqueue_missed_with_cap'` text in
`createRoutineTables` fixes nothing on an existing database: the statement is
`CREATE TABLE IF NOT EXISTS` and is a no-op there, and SQLite has no
`ALTER COLUMN ... SET DEFAULT`. Changing it requires a table rebuild, following
the pattern this repository already documents (`apps/backend/AGENTS.md`,
"Table-rebuild migrations") and already uses in this very file —
`runTaskPriorityRecreate` in `base_migrations.go` is the worked precedent:
acquire a dedicated connection, `PRAGMA foreign_keys=OFF` with a deferred
re-enable, create `office_routines_new` with the corrected default, `INSERT ...
SELECT` every column, `DROP TABLE office_routines`, rename.

The `foreign_keys=OFF` is not incidental. `office_routine_runs` declares
`FOREIGN KEY (routine_id) REFERENCES office_routines(id) ON DELETE CASCADE`, so
dropping the old table with enforcement on would cascade every routine run out
of existence. Mirror every column in the replacement `CREATE TABLE` and the copy
list, and add a replay regression test proving values and timestamps survive —
the run rows in particular.

Leaving the default clause stale because no insert exercises it was rejected: an
operator reading an upgraded install's live schema would see a default naming a
value the system no longer treats as canonical, the same
disagreement-between-surfaces defect this capability exists to close.

**Frontend.** Replace *every* occurrence of the `enqueue_missed_with_cap`
literal in `create-routine-dialog.tsx` and `routine-detail-view.tsx` — do not
work from a fixed line list, which is what produced the omission this design
previously shipped. There are four distinct kinds of site, and only two are
`SelectItem` values:

| Kind | Effect if missed |
|---|---|
| `SelectItem value=` (both files) | The control writes the retired value |
| Create-dialog initial state default | New routines are created with the retired value |
| Detail-view fallback default | An unset policy renders as the retired value |
| `catchUpPolicy === "enqueue_missed_with_cap"` guards (both files) | **The `catch_up_max` input stops rendering entirely** |

The last row is why AC-003.8 exists. Those two comparisons decide whether the
cap input is shown at all; once every routine reads as `summarize_missed` they
are permanently false, so a builder who updates only the option values and
defaults ships a UI where the cap field has silently vanished — while still
passing AC-003.4 and AC-003.6, neither of which observes the control.

The i18n key `office:enqueueMissedWithCap` is replaced (not re-worded in place —
the key name would then lie too) by `office:summarizeMissed`, and
`office:beyondThisCountMissedTicksAre` is replaced by a key stating that the
bound is on the ticks counted and reported. Both across `en`, `pt-pt`, `zh-cn`,
`zh-hk`, `zh-tw`; use `pnpm run i18n:zh-hant` for the Traditional pair and
`pnpm run i18n:pseudo` to regenerate the pseudo catalog. No em dash in any
locale value.

## Documentation consistency

Three distinct corrections, and AC-003.7 requires all three. Work by **grepping
both files for `enqueue_missed_with_cap`, for "missed", and for `missed_ticks`,
and correcting every hit** — the lists below name the sites that exist today and
are illustrative, not a work list to tick off. This design shipped that mistake
once already, in the frontend notes above, and repeating it here would leave
the fourth surface disagreeing after a capability whose entire purpose is to
stop surfaces disagreeing.

*Statements that imply multiple fires:* `scheduler-01.md`'s "Catch-up policy"
section ("fire missed ticks up to the cap"), `scheduler-02.md`'s failure-mode
table row, its restart scenario, and the retention paragraph's "catch-up cap
(default 25) drops missed routine ticks" sentence. Each is corrected to state
one summarized wake and to reference
[the requirement](../requirements/routine-catch-up.md) rather than restating its
terms.

*Places that merely name the value:* `scheduler-01.md`'s routine-field list
(`catch_up_policy: enqueue_missed_with_cap (default, cap 25)`), its worked
routine-config example block, and `scheduler-02.md`'s schema column comment.
None claims multiple fires, so the first correction leaves them untouched — and
they would then name a policy value the system no longer accepts as canonical,
which is the exact class of defect this capability exists to remove. All become
`summarize_missed`, with the alias mentioned only where it is labelled
deprecated.

*Places that document the routine wakeup payload's shape:* `scheduler-01.md`'s
run-reason table row (`{routine_id, variables, missed_ticks?}`) and
`scheduler-02.md`'s `RoutinePayload` struct listing, whose `MissedTicks` field
carries a "when catch-up cap collapsed N fires" comment. Neither names the
policy value nor claims multiple fires, so neither correction above reaches
them, yet both describe a contract this design extends with `missed_since` and
`missed_truncated`. Each gains the two new fields, and the `MissedTicks` comment
is corrected to say the count is of ticks *counted and reported*, never fired.

`docs/specs/office/README.md` gains both new files in its specification map.

## Testing

Backend, in `internal/office/routines/`:

- `catch_up_test.go` already covers the counting loop and the default cap; it is
  extended for `FirstMissedAt` exactness under truncation, `Truncated`, the
  `ElapsedTicks == 1` no-summary case, the failure path's strictly-after re-arm,
  and both `catch_up_max` clamps — `0 -> 25` and `5000 -> 1000` (AC-001.4). The
  existing tests referencing `CatchUpPolicyEnqueueMissedWithCap` move to the new
  constant, with one test retained on the alias to prove normalization.
- The `catch_up_max == 1` boundary (AC-002.4, AC-002.11): a gap spanning many
  ticks records **no** gap summary, and in particular does not record
  `truncated = true` with a zero count. This is the case where the guard added
  to AC-002.4 is the only thing standing between two ACs, so assert the absence
  directly rather than inferring it from a count.
- A tick test asserting exactly one routine run and one wakeup request for a
  gap spanning more than `catch_up_max` ticks — the AC-001.1 regression that
  would have caught the original misnaming.
- A `GetDueTriggers` ordering test over rows with equal `next_run_at`.
- Reconciliation (AC-001.9): an enabled cron trigger with a null `next_run_at`
  and a stale `updated_at` is armed and does not dispatch; a second reconciler
  affects zero rows and does not move `next_run_at`; and a trigger whose
  `updated_at` is inside `catchUpReclaimAfter` is left untouched, which is the
  live-claim case the CAS alone does not cover.
- Re-arm failure (AC-001.2): `UpdateTriggerNextRun` returns an error, no routine
  run is created for that claim, and a later tick's reconciliation arms the
  trigger without dispatching.
- A malformed-expression case asserting the re-arm lands exactly 24 hours after
  the processing instant and that a second tick a day later dispatches once
  more, not repeatedly (AC-001.11).
- Dispatch-failure cases (AC-001.10): heavy-path task creation fails and the run
  reads `failed`; lightweight `CreateWakeupRequest` fails and the run reads
  `failed`; an idempotency-conflict sentinel does **not** mark the run failed.
  And AC-001.12: `CreateRoutineRun` itself fails, no run row exists, and the
  trigger stays armed forward.
- A write-once case for AC-002.10: a run carrying a gap summary is transitioned
  through `coalesced` and `failed`, and the three columns are unchanged after
  each.

Wakeup, in `internal/office/wakeup/` and its repository — the coalesce merge
(AC-002.10). **Every case below runs against BOTH merge entry points**,
`MarkWakeupRequestCoalesced` and `PromoteRunAndCoalesceWakeupIfQueued`, as a
table test over the two. That is not belt-and-braces: until the refactor in
[part 1](routine-catch-up-01.md#delivering-the-gap-to-the-agent) lands they are
two separate SQL statements, and a suite that exercises only the helper passes
green with the promoted-run path fully broken — the exact failure the design
text previously invited. What the table does **not** do is catch a third merge
site: a table over two named entry points cannot observe a statement that is not
in it, so do not rely on it for that. The guard for that is a separate
source-level pinning assertion: that `json_patch` appears exactly once in
`wakeup_requests.go` **outside comments**, once the unification has landed.

The qualifier is the whole assertion, not pedantry. `grep -c json_patch` on that
file returns **three** lines today, and only two of them are SQL: the third is the
helper's own doc comment, which names the function in prose and survives the
refactor untouched. An unqualified "exactly once" would therefore FAIL against a
correct implementation, and the builder would be left inventing the real
predicate. Assert on SQL occurrences — parse the file, or match the statement
rather than the bare identifier.

The in-tree precedent for a test that reads Go source and asserts a structural
count is `internal/common/subproc/raw_git_audit_test.go`'s
`TestProductionGitCommandsUseTheAdmissionSeam`, which walks the tree with
`go/parser` and reports violations by file and line. It is **not** the auth
middleware allowlist: `apps/backend/CLAUDE.md` describes that one as a "pinning
test", but `auth/httpmw/middleware_test.go`'s `TestEnabledModeAllowlistMatrix`
issues real HTTP requests and asserts status codes per path — a behavioral
matrix, and the wrong shape to model this on.

Two directions per entry point, and the second is the one `omitempty` alone
would pass for the wrong reason: a gap-carrying routine wake coalescing into a
gapless in-flight run leaves that run gapless, and a gapless wake coalescing
into a gap-carrying run leaves the original gap intact rather than
half-overwriting it. Add a third: a non-routine wakeup source whose payload
contains none of the three keys round-trips byte-identically through the merge,
which is what makes the `json_remove` safe to put in a shared helper.

Repository: round-trip of the three columns including the NULL-versus-zero
distinction, in `routines_config_fields_test.go`'s neighbourhood. Plus a
default-injection test covering `CreateRoutine` and `CreateRoutineTx` with an
empty `CatchUpPolicy` (AC-003.1), and a replay regression test for the table
rebuild proving every column and timestamp survives and that
`office_routine_runs` rows are not cascaded away (AC-003.3).

`catch_up_max` write normalization (AC-001.13) is tested on **every** path that
can currently store an out-of-range value, which is what makes the test
meaningful — and that is both repository statements, not only the update.
`CreateRoutine` and `CreateRoutineTx` clamp `<= 0 → 25` today and have no upper
bound at all, so a create carrying `5000` persists `5000`; the create paths are
therefore in scope by the same rule that puts the update path there. So:
`CreateRoutine`, `CreateRoutineTx` and `UpdateRoutine` each with `0` and with
`5000`, each read back at the clamped value rather than the submitted one, plus
the handler's create and update paths end to end — **asserting on the create and
update RESPONSE BODY itself, not only on a subsequent `GET`**. Those two
responses are the only place part 1's in-place-reassignment decision
([Normalizing catch_up_max](routine-catch-up-01.md#normalizing-catch_up_max)) is
observable: both handlers serialize the same `*Routine` they passed to the
service, so a test that POSTs `5000` and then re-reads the routine with `GET`
passes identically whether the funnel mutated the struct or only the SQL bind,
and would not catch the one divergence AC-001.13 exists to prevent. Cover
`office/service.Service.UpdateRoutine` as well — it is the second service layer
reaching the repository without the routines handler, and it is what
`config_import.go` calls, so it is the regression that would prove the clamp was
put in the handler instead of the repository. Plus a migration
replay proving a pre-existing row at `0` and one at `5000` are corrected in
place, and that a row already in range is left untouched — the read-back is the
assertion AC-001.13 actually names, so assert on the returned routine, not on
the tick's behaviour, which AC-001.4 already covers.

Prompt: `BuildPromptContextForTest` cases asserting the wake context renders for
a run with a gap under **each** of the three routine reasons — including
`RunReasonRoutineDispatchEvent`, which is the promoted-run case a Cron-only gate
would silently drop (AC-002.5) — and that a non-routine reason with the same
`context_snapshot` renders nothing.

Frontend: a store/API test that `summarize_missed` round-trips, a test that the
`catch_up_max` control renders under the summarizing policy and not under
`skip_missed` (AC-003.8), and `pnpm run i18n:check` for the five locales.

E2E is proposed rather than assumed; the surfaces are listed in the task plan.
