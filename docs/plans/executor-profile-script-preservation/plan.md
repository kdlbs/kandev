---
created: 2026-10-07
status: draft
requirements:
  - REQ-EXECUTORS-PROFILE-EDITOR-001
system_design:
  - ../../specs/executors/system-design/profile-editor.md
legacy_specs: []
---

# Implementation Plan: Preserve executor scripts during partial saves

## Overview

Preserve omitted prepare and cleanup scripts in ordinary built-in executor
profile saves, including when another save commits after the first request reads
its profile. One sequential work order owns permanent RED/GREEN evidence, the
narrow service/store correction, real transport and launch-projection checks,
portability, documentation, and authorized delivery.

ROOT reviewed the four-artifact design package and explicitly released
implementation in the same primary session. The requirement/design pair is now
active/current; the sole work order remains in progress through delivery.
Autopilot and standing delivery did not release the original design gate. The existing
[editor-unification package](../executor-profile-editor-unification/plan.md) is
implemented and its route/UI results remain unchanged. This repair extends
only save persistence; it does not reopen its browser matrix or imply that its
old draft status authorizes additional work.

## Confirmed cause and evidence

Admission base and authoritative main at 21:42 UTC were
`bef6699b47d64768cec6aba278c0b00ebea97e6e`; this worktree began clean at that SHA.
`Service.UpdateExecutorProfile` loads a snapshot, overlays supplied fields,
then ordinarily calls `UpdateExecutorProfile`, whose full UPDATE writes both
scripts. A held name-only or opposite-script save can restore an older omitted
script after another save acknowledges its change. Future provisioning copies
that stored value through `Executor.applyProfile`.

ROOT's accepted read-only proof is
`/tmp/kandev-root-executor-script-discovery-20261007/candidate_test.go`, regular
mode `0400`, SHA256
`2fd1e74a27c823cdcd88ceb40642055903332bca097be8b76e1cfbe6c04d98c0`.
Never replay, copy into source, modify, or remove it. Original native handle
41143/PID 232073 actually joined exit 1, terminal chunk `4284f0`.
`qualification.json`, `receipt.json`, and `original.log` establish four causal
failures in stored and returned scripts and two passing controls, with two
independent real SQLite stores/services/connections and only the captured
profile read gated. Package time was 0.390s; total time including compilation
was 95.78s. The fresh group was gone and temporary source removed. The older
`triage.json` authoring status is superseded by qualification; its consumer audit
remains useful. This proves no HTTP, PostgreSQL, or runtime execution behavior.

## Scope

### In scope

- Script pointer intent through the ordinary built-in service/store path.
- Atomic omitted-column preservation and own-commit script/timestamp acknowledgement.
- Independent SQLite service regressions, real registered REST/WS and settings
  operations, PostgreSQL statement waits, scoped conformance, native Windows,
  and actual saved-profile launch projection.
- A brief public partial-save clarification and one concise backend convention
  note during implementation, if needed to keep the new required seam discoverable.

### Out of scope

- Other profile field races, stale full editor drafts, typed generic patches,
  credential retention, cancellation redesign, global revisions, or writer programs.
- Schema, wire/event shape, UI, localization, permissions, config/admission,
  environment precedence, runtime cleanup, or already-running resource changes.
- Plugin profile changes, removal of full legacy writes, or weakening exact CAS.
- Other worktrees, sibling remediation, speculative ADRs, or delegation.

## Technical approach

Extend the existing executor-owned profile-editor pair minimally. AC .5 remains
the saved-values basis; .8 through .12 state the omitted-script and compatibility
contract. Adjacent UI spacing, environment precedence, platform settings and
plugin owners retain their contracts. No independent UI pair or incident spec.

Add a two-pointer intent in a new focused model file and a required
`UpdateExecutorProfileWithScriptIntent` seam on `ExecutorRepository`.
The service keeps its existing pre-write validation and other-field preparation,
uses the new seam only for ordinary built-in saves, and publishes after success.
The store includes only supplied script columns in its actual UPDATE, captures
both scripts and its timestamp with RETURNING, exhausts/closes rows before
commit, and assigns captured values to the service's model after commit. Retain
legacy/exact methods unchanged; avoid a later reread or a silent full-write fallback.

| Path | Presence / guard | Behavior and evidence |
| --- | --- | --- |
| Built-in REST/WS | Existing nullable pointers; no expected version | Omitted scripts preserved; real registered handlers and independent writer |
| Settings-domain `executor_profile` | Existing request decode; no expected version | Same service path; registered settings operation with real DB |
| Config-mode MCP | `ExpectedUpdatedAt` from current row | Existing full exact CAS; deterministic matching/stale controls |
| Legacy repository writer | Full model | Both scripts intentionally replaced; legacy control |
| Plugin remote | Separate service branch | Nonempty local scripts rejected; no new seam use |
| K8s / remote Docker | Existing admin and runtime validation | Rejections before write; existing and focused controls |
| Sprites / env | Existing token merge and global-reference validation | Preserve current semantics; focused controls |

Use current native SQLite writer transaction admission and PostgreSQL row UPDATE
locking. There is no pre-UPDATE storage read, generic lock, retry, migration, or
production timestamp change. Required aggregate mocks gain explicit methods in
new focused test files; use embedded real repositories for integration fixtures.
Unsupported test-only compatibility methods return errors, never apparent success.

## Tests

All named tests below are planned permanent tests, not present or run at design
handoff. The work order specifies exact command groups and failure handling.

| Criteria | Planned evidence |
| --- | --- |
| .5, .8 | `TestExecutorProfileScriptsIndependentSQLite`: all four held snapshot interleavings, stored and returned pair |
| .9 | `TestExecutorProfileScriptsPresence`: omitted, JSON null compatibility, explicit empty, both present, same-script last commit |
| .10 | `TestExecutorProfileScriptsOwnCommit`: event/response pair and timestamp survive a later actual commit; `TestExecutorProfileScriptsApplyProfile`: real stored-row setup/cleanup projection |
| .11 | `TestExecutorProfileScriptsFailures`: precommit cancellation, rollback/query/serialization/result/commit errors and missing row, no success event |
| .12 | `TestExecutorProfileScriptsCompatibility`: exact, legacy, plugin, admin, K8s validation, Sprites/global-env controls |
| .8 through .12 | `TestExecutorProfileScriptsStorage`, `TestExecutorProfileScriptsStoreConformance`, PostgreSQL physical concurrency and compatibility |

## End-to-end boundary

`TestRegisteredExecutorProfileScriptsHTTP` and `TestRegisteredExecutorProfileScriptsWS`
in a new task-handler test file exercise the registered route/dispatcher, real
service and DB, decoded partial payload, concurrent script save, response, event,
and persisted row. `TestExecutorProfileScriptsSettingsDomain` exercises the
actual settings-domain binding through registered settings dispatch and a real
DB. Transport seams may supply identities or capture messages; they do not
replace persistence or service updates.

The canonical editor explicitly supplies both scripts. It gets no new promise
about stale drafts. No layout, copy, touch, navigation, or viewport behavior
changes, so this backend data-only repair requires no browser/E2E/build or
mobile preview. This is the mobile-parity audit outcome.

Native stage **Test Windows executor profile scripts** must run the anchored new
functional tests and show actual named RUN/PASS. Preserve existing stages: this
base names its relevant stage **Test Windows profile enabled omission** (the
parent's label **Test Windows executor profile enabled** is not present here).
Do not rename/drop either existing or incoming relevant stages. Sibling74's
**Test Windows repository checkout defaults** belongs to pending PR4308, not
this base; retain it if it becomes incoming, without reading its worktree or
inventing its current presence.

## Public documentation audit

`docs/public/executors.md` is a reference/how-to guide whose existing profile
timing and per-runtime script table are accurate. During implementation add only
a short clarification near profile saving: ordinary partial API saves retain
omitted prepare/cleanup scripts; explicit values, including empty, replace them.
Do not imply that the full editor omits scripts or that saving changes a running
environment. Root README and screenshot catalog need no change. No public page,
visual, screenshot, or runtime-timing rewrite is warranted.

## Work orders

- [ ] [Task 01: Preserve ordinary profile script intent end to end](task-01-preserve-profile-scripts.md) (in_progress, sequential)

## Verification results

Implementation results are recorded in Task 01 and the live task plan. The
original DESIGN checkpoint below is historical; accepted ROOT diagnostic remains
read-only evidence. Cheap design checks completed on 2026-10-07:

- `python3 scripts/list-docs.py validate`: passed, 360 decisions and 1,434 specifications.
- `python3 scripts/lint-spec-files.test.py`: passed, 36 tests.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `git diff --check`: passed; status enumerated exactly four unstaged artifacts,
  two modified owner documents and two new plan files; index empty.
- PR-doc helper classified the actual docs-only diff as exempt. Its design
  reference preflight with the planned `service_resources.go` trigger passed
  `covered`, one work order, correct owner/design/AC references, `errors: []`.
  The planned trigger is not an actual production edit or implementation proof.
- Existing Node `/home/jcfs/.local/share/mise/installs/node/24.21.0/bin/node`
  ran the reference helper under Bash without login. Initial bare `node` was
  unavailable (exit 127); no install occurred. An initial wrapper expected
  `covered` on the docs-only exemption and exited 1; the planned-trigger check
  correctly exercises reference validation. These are cheap preflight diagnostics,
  not passing product tests or unreported heavy failures.

All commands returned terminal results; no native session ID or background
process was created. Zero live handles, workers, fixtures, credentials, or heavy
resources are owned by this task. Design turn ends at this handoff; implementation
still waits for the later explicit ROOT INTERRUPT.

## Delivery and resource gates

ROOT later explicitly released implementation after the completed design turn.
Child75 holds the exclusive global local-heavy grant; merge remains NONE.
Reachable failure tests use existing focused SQL/driver seams without production
marshal hooks or generic test frameworks. Active supported aggregate fixtures
retain faithful behavior; only unexercised unsupported doubles fail closed.
Initial lint is scoped to actual changed/required compatibility packages against
the admitted base; full `./...` changed-code lint is required on an actual Go PR
fixup, while hosted full current-head lint remains mandatory.

The live version-safe Kandev task plan records identity, system marker, phase,
question barriers, release/merge authority, and parent resource coordination.
Task `4bdb9b77-f970-4631-a4de-4ed4776f42a5`, primary session
`58b5b0b8-dba3-47cc-a898-2f6f91d23963`, profile/executor remain unchanged.
No agents, recursive tasks, new sessions/tabs, model switch, or operator approval.
At this checkpoint leave exactly four artifacts unstaged/uncommitted and end
the turn. ROOT's later explicit INTERRUPT alone releases implementation.

After release execute one heavy original at a time, actually join every returned
handle, and prove its fresh process group gone. Persist any timeout, resource,
transport, unknown or out-of-scope diagnostic and checkpoint ROOT before
recovery. No passing replay or discarded result. Private PG ownership/mounts
must be recorded before allocation; remove only proved-owned resources and
credentials. Foreign volume `2c48...` and paused work remain untouched.

Standing delivery authorizes normal active hooks, commit/push and a ready PR
after successful implementation checks; it does not authorize merge. Use current
canonical association/repository UUID `16026b06-bd79-47c0-aed1-dc7ca95f63d9`,
FIVE effective automation flags FALSE, and preserve the live author template/body
and bot appends. Freeze head except grounded corrections. No moving-main rebase
or synthetic merged test evidence. Return ALL local-heavy resources, joins,
fresh groups and owned fixture cleanup before ONE original 90m all-terminal
observer (GNU 91m, kill after 10s), fixed saved start/deadline; no duplicate,
reset or replacement without ROOT. Require six actual required contexts,
Backend/Frontend/E2E parent SUCCESS, named Windows/PG RUN/PASS without skips,
fresh complete/errors-empty report, and zero actionable visible or hidden threads.
Require authenticated CodeRabbit App347564 substantive FULL current-head coverage
of all changed files; inspect automatic coverage before at most one necessary
full request for a real gap. Processing/ACK is not evidence. Ground and disposition
findings; no optional review waiting or polish. Merge authority remains NONE
until ROOT's separate serial grant, then expected-head normal squash and
independent API/Git SHA/tree/blob/remote proof with joined owned cleanup. ROOT
alone verifies, archives, releases proof, and refills.

## Risks

- RETURNING rows must finish before commit; driver errors cannot produce success.
- Acknowledgements promise this save's script pair and timestamp, not coherent
  unrelated fields or a latest global snapshot.
- SQLite busy cancellation may settle only after existing busy-handler behavior;
  every test goroutine and native command still needs an actual join.
- PG skips and Windows compilation alone do not establish hosted portability.
- New required methods affect aggregate mocks; focused fail-closed adapters and
  actual integration fixtures keep compatibility work bounded.
- Overlapping sibling files require static, domain-specific compatibility checks
  near merge; they do not authorize unrelated edits or main movement.
