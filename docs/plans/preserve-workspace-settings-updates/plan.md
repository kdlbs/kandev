---
created: 2026-10-07
status: in_progress
requirements:
  - REQ-WORKSPACES-SETTINGS-UPDATES-001
system_design:
  - ../../specs/workspaces/system-design/workspace-settings-updates.md
legacy_specs: []
---

# Implementation plan: Preserve workspace settings updates

## Overview

Make overlapping successful workspace settings saves retain disjoint edits.
One sequential work order independently authors regressions, adds the required
workspace field-persistence seam, preserves exact administration and admission,
and verifies registered update contracts with real databases.

The reviewed design package received later ROOT authoring and full implementation
releases in this same primary. Task `c4e67ceb-0984-4659-a12a-3ad7ff7c6112`,
session `41d6db63-82fc-4306-8211-7e7b789e63f7`, now implements the single work
order under the exclusive GLOBAL LOCAL-HEAVY release. Parent ROOT is
`14825981-b175-411d-999a-31ddc2aa5fc3`. No delegates, tasks, tabs, sessions, or
model switching; the continuous-loop cap remains two substantive persistent
tasks, including this child and upload69. HOSTED and MERGE remain unreleased.

## Scope

Implement [REQ-WORKSPACES-SETTINGS-UPDATES-001](../../specs/workspaces/requirements/workspace-settings-updates.md)
and the [paired design](../../specs/workspaces/system-design/workspace-settings-updates.md).
The design's compatibility inventory is the starting caller/mock/reset audit.
This package owns only workspace partial-settings persistence, returned
observations, tests, and narrow native-test wiring. Intentional full writers,
exact timestamps, namespace validation, and permission checks retain their
contracts.

Exclude schema/API shape, UI/store, runtime/runner/hierarchy changes,
authorization redesign, generic revisions, global event ordering, universal
writer serialization, visibility changes, and task numbering. No new rendered
behavior requires a browser test, ASCII preview, product build, or broad suite.

## Technical approach and order

1. Re-read the current package and repeat the caller audit against the released
   head. Independently author meaningful tests with the existing real-SQLite
   harness. Gate service reads after they return a real snapshot; use two
   services and distinct database/physical connections. Join every goroutine
   and cancel/unblock cleanup on failure. Do not use sleep-only concurrency.
2. Require `UpdateWorkspaceFields` on `WorkspaceRepository`; implement the
   typed nullable field patch and allowlisted bound UPDATE RETURNING with an
   optional exact timestamp predicate. Preserve direct full-write methods.
   Migrate `Service.UpdateWorkspace` to intent derived from request presence
   and admitted actual moves. Return/publish the mutation's observed row.
3. Update every interface mock and forwarding wrapper needed for compilation.
   Preserve ordinary REST/WS and exact plugin consumers. Run targeted TDD and
   scoped lint only after the separate resource release. Wire a narrow command
   into the existing hosted Windows native job; existing PostgreSQL jobs
   already select `./internal/task/repository/sqlite`.

| Boundary | Applicability | Planned evidence | Failure/fallback |
| --- | --- | --- | --- |
| Service + independent SQLite handles | Ordinary and exact workspace settings | Both stale-read overlap orders, controls, unit/auth, SQL-failure events | Existing errors; no whole-row fallback |
| Registered REST | PATCH optional values plus admitted unit moves | Real router, service, SQLite and bus; own DTO/event observation | Existing REST error mapping |
| Registered WebSocket | Existing optional scalar fields; no UnitID/CAS transport field | Actual dispatcher, service, SQLite and bus | Existing validation/internal-error shapes |
| Exact plugin administration | Existing ExpectedUpdatedAt service path | Existing consumer test plus real SQL matched/intervening conflict | Existing conflict/idempotent behavior |
| PostgreSQL repository | Same SQL seam and projection; actual supported dialect | Independent backend PIDs, real lock wait, NULL/bool/CAS/constraint rollback | Missing DSN is no execution proof |
| Native Windows database path | Shared Go/SQL code, no OS branch | New narrow cases actually RUN/PASS in native matrix | Build success/SKIP is insufficient |

## Tests and acceptance mapping

Permanent test names and required evidence are listed below. Executed outcomes
are recorded separately in the work-order results. Their
files and exact command selectors are fixed by the
[work order](task-01-persist-settings-fields.md#verification-after-release).

| Criteria | Required independent evidence |
| --- | --- |
| .1 | `TestWorkspaceFieldUpdatesConcurrentSQLite`: name versus enabled/timeout in both held-read directions; defaults versus settings with distinguishable values; physical handle identity |
| .2, .3 | `TestWorkspaceFieldUpdatesPresenceAndDefaults`, `TestWorkspaceFieldUpdatesStorage`: false, empty text, each default clear/trim, omission/null/empty request, defaults, untouched SQL NULL, same-field last write, mixed fields |
| .4, .6 | `TestWorkspaceFieldUpdatesAdmissionAndFailures`: manage/reach denial, actual allowed unit move, missing/cross-org destination, missing placer, empty/same destination controls, omitted unit after admitted move, invalid timeout, cancelled-before-write and injected SQL failure; no success events |
| .5, .6, .8 | `TestWorkspaceFieldUpdatesExactFenceSQLite`: matched fence and stale fence after real intervening save; fields/timestamp unchanged on conflict; deliberate complete-write success/conflict; existing exact plugin test |
| .6 | `TestWorkspaceFieldUpdatesStorage`: SQLite trigger-aborted multi-field statement rolls back values and timestamp; missing ordinary/fenced rows retain error mapping |
| .1, .2, .4, .7, .8 | `TestRegisteredWorkspaceFieldUpdatesHTTP` / `WS`: both overlap directions, ordinary JSON presence and own response/event correspondence, final persisted union, error/event controls, HTTP-only placement support preserved |
| .1-.6 | `TestPostgresWorkspaceFieldUpdatesPhysicalConcurrency` / `Compatibility`: actual row contention both directions, returned row, normalization/defaults, exact success/stale conflict at SQL boundary, constraint rollback |

## End-to-end contract evidence

The relevant end-to-end boundary is registered REST and WS through the real
workspace service and persistence, including event observation. Handler helper
calls, fake writers, and hand-built DTOs cannot substitute. The existing save
builder is a read-only input audit, not executed browser evidence. Desktop and
phone share the same unchanged save behavior; there is no causal rendered
change requiring Playwright or a frontend build.

Linux service/repository/handler cases and hosted PostgreSQL/native cases must
appear as actual new-test RUN/PASS on the exact delivery head. Preserve their
complete logs. Skip-only database cases, package exit alone, build-only native
jobs, old successful logs, and replayed candidate proof do not satisfy this.

## Work orders

- [ ] [Task 01: Persist workspace settings field intent](task-01-persist-settings-fields.md) — in_progress (full implementation and local checks), sequential, no dependency.

## Phase and resource gates

Design permits only read-only inspection and cheap catalog, specification
36-test, lint, reference-coverage, and whitespace checks. No production or
permanent tests, install, Go command, product check, database fixture, commit,
push, or PR in the original design phase. Later ROOT released full implementation,
exclusive LOCAL-HEAVY, and normal commit/push/ready-PR publication after checks.
HOSTED and MERGE remain separately gated.

Later scoped authoring requires ROOT's implementation release and can precede
heavy verification if ROOT releases it separately. Heavy work requires the
single GLOBAL LOCAL HEAVY slot; all commands are sequential. Go uses trimpath,
fts5, race, p1, GOMAXPROCS=2, GOMEMLIMIT=512MiB, CLI timeout 5m / GNU 6m with
kill-after 10s. Initial lint is scoped, concurrency 2 and allow-serial-runners;
do not add full backend lint. Only an actual subsequent backend fixup gets one
full CHANGED `./...` lint against the exact API base, GOMAXPROCS=2,
GOMEMLIMIT=1GiB, with the same time bounds.

If absent dependencies prevent normal active hooks, install pinned pnpm
9.15.9 once only after explicit heavy release; preserve normal hooks and never
bypass them. No install is scheduled merely to validate these Markdown files.

For every future process preserve argv, cwd, UTC start/end, logs, PID/groups,
native session/chunk IDs, CLI/GNU/kill cutoffs, actual terminal join, and fresh
empty groups. Resource, timeout, transport, unknown, or out-of-scope failure
checkpoints ROOT; there is no automatic retry.

HOSTED needs a separate ROOT release: one original 90m all-terminal wait with
GNU 91m/kill10, no duplicate/reset/successor/rerun without ROOT. Require all six
required contexts and actual Backend, Frontend, E2E parent SUCCESS on exact
head, and applicable PostgreSQL/native new-test RUN/PASS. CodeRabbit App347564
must provide substantive FULL review of all exact-head files, source covered
and kind reviewed; accept sufficient automatic evidence first, make one review
request only for a proved skip gap. Ground and disposition every real finding;
zero unresolved/hidden/change-requested/human gates. Freeze head except actual
corrections; no moving-main rebase, synthetic tests, passing replay, weakened
gates, or optional polish.

MERGE needs a separate normal expected-head squash grant and the single SERIAL
MERGE slot. Independently verify actual merge, tree, all owned blobs and remote;
join owned cleanup only. Preserve managed worktree, dependencies/caches,
foreign refs/FETCH_HEAD, paused oversized task, and old unproved volume. ROOT
owns archive and sole proof release. Child-to-ROOT interrupt is forbidden; an
optional queued callback never gates work or retries if the queue is full.
The primary and task plan remain directly readable checkpoints. For a critical
unknown use the available parent-question tool and end the turn immediately.

## Documentation assessment

The workspaces owner is reused; the partial-settings capability is the minimal
missing pair. Adjacent task/workflow field updates, unit reach, executor idle
parking, and UI save presentation are referenced rather than copied. No new
system README, repair/incident duplicate, or ADR is needed. Public default and
placement guidance in `docs/public/tasks-and-workflows.md` and
`docs/public/team-access.md` remains accurate for this bounded fix. No public
page or screenshot change is planned. Mobile outcome is backend data-only.

## Verification results

Design checks passed on 2026-10-07: catalog validation (359 decisions, 1425
specifications), all 36 specification-linter tests, and full specification lint.
The existing PR-documentation validator returned prospective reference status
`covered` with no errors for this single work order. Both new specs appear in
the workspace catalog. Relative links, all four files' whitespace, and
`git diff --check` passed; the index is empty and exactly four artifact files
are untracked. The reference preflight used the existing installed Node binary
via its absolute path because Node was absent from the shell PATH; no install
or product check was needed.

At the original design handoff, no product, permanent test, PostgreSQL, native,
browser, or hosted check had run. Those results validated the design package
only; the work order was pending and both owner documents were draft. Later
ROOT releases and implementation outcomes are recorded in the work-order results.

## Risks

- Losing default clear intent during normalization, or assigning a stale unit,
  would recreate an omission bug; typed presence and admitted-move tests bound it.
- Timestamp representation must preserve the existing exact SQL predicate on
  SQLite and PostgreSQL; a service-only conflict test is insufficient.
- A stale loaded object can survive in response/event plumbing even after the
  SQL repair; registered-flow tests must observe the returned row.
- Deliberate full-row writers retain overwrite semantics. This package does
  not promise universal workspace serialization or new admission-race defenses.

## Current implementation checkpoint

The later ROOT releases produced and locally validated the reviewed seam.
See [work-order results](task-01-persist-settings-fields.md#results) for the
causal RED, ten targeted top-level passes (seven new, three existing), exact
plugin consumer, real local PostgreSQL contention and compatibility, required
persistence checks, owned fixture absence and failed initial lint plus its one
authorized successful resource recovery. Normal publication and later hosted
checks retain the standing phase gates; ONE work order remains in_progress.
