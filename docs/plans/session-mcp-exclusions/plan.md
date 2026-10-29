---
created: 2026-10-22
status: completed
requirements:
  - REQ-AGENTS-SESSION-MCP-EXCLUSIONS-001
  - REQ-AGENTS-SESSION-MCP-EXCLUSIONS-002
  - REQ-AGENTS-SESSION-MCP-EXCLUSIONS-003
system_design:
  - ../../specs/agents/system-design/session-mcp-exclusions.md
legacy_specs: []
---

# Implementation Plan: Session MCP Exclusions

## Overview

Allow an existing task agent session to exclude inherited catalog definitions
and restore inheritance. Implement the saved contract and resolver first,
prove lifecycle acceptance second, then expose the desktop and phone controls.
Implementation was authorized by the user on 2026-10-23.

The [requirements](../../specs/agents/requirements/session-mcp-exclusions.md)
and [design](../../specs/agents/system-design/session-mcp-exclusions.md) define
the implementation contract.

## Baseline and coordination

- Task: `15641d1a-d7ba-4216-aefd-5a4c36a88d73`.
- Workspace: `/home/zeval/.kandev/tasks/allow-session-only-i_ae4fhzed/pr3293-followup-source-20261022`.
- Feature branch: `feature/allow-session-only-i-2gn`.
- Fetched parent and initial clean HEAD: `193de56ba2597b335a97b1e5ab01d09340f22647`.
- Integrated assignment WO01: `f9137074309bebe99e2c7576baefe936bc20358f`.
- User agreed to stack on assignment. Planned PR target:
  `feature/clarify-mcp-assignme-mhh`. The original parent branch stays unchanged.

The parent merged as `0b81ae48d4e2c358f80ac489e39388dfd8d7e6ab`. The local
stack now includes that squash through the rebased assignment branch. Keep the
PR target on assignment while it is unmerged; after assignment merges, reconcile
its actual merge and retarget to main without replaying dependency commits. No merge is authorized. Preserve other edits and
commits. Do not change the parent branch or sibling workspaces. No extra agents,
tasks, sessions, or autopilot are authorized.

Read sibling saved plans for interface alignment: management clarity
`000d477e-f792-4c9d-8503-50ed73bb9646`, registration assignment
`5a8cb5cd-22ee-42e5-bd6d-e2179e899c8c`, and built-in management
`6d1b4323-e821-4599-acea-07273a60dd78`. Registration assignment work order 01,
`docs/plans/mcp-assignment-after-registration/task-01-guard-selection-replacement.md`,
is the committed implementation prerequisite for local work order 01. It owns
conditional replacement, session revision admission, exact committed responses,
consistent GET snapshots, locks, and conflict/retry semantics. This package
extends that primitive for exclusions. Assignment work orders 02/03 and the
other siblings are not prerequisites. Never copy sibling uncommitted code.
Omitted exclusion fields preserve saved exclusions in additive writes.

## Scope

### In scope

- Session-owned persistence, atomic writes, validation, and reversible suppression.
- Origins from all repositories/profile/task/session, disabled and dormant cases.
- Desired/accepted comparison and existing safe idle/next-start application.
- Existing-session Customize actions, phone parity, localization, and focused tests.
- User documentation and activation of the narrow parent-contract exception.

### Out of scope

- Upstream exclusions, creation-time exclusions, and per-definition overrides.
- Built-in/tool management, native provider config editing, or forced revocation.
- Catalog usage navigation, profile management, and registration assignment.
- New reconnect protocols, automatic restarts, probes, and broad verification suites.

## Assumption check

Confirmed: only one session changes, origins stay visible, reversal is required,
active turns remain immutable, and built-in management belongs to a sibling.

Verified: the parent resolver unions positive IDs. Accepted snapshots and
provider-aware application already exist. The original session handler lacked client CAS; assignment WO01 supplies it.
The session body is reused during creation and needs explicit exclusion authority.

Design choices: exclusions last for the session identity until restored. Dormant
exclusions persist. Required Kandev capabilities remain outside the control.
These choices were in the completed package before the later implementation
request. No material design decision remains unresolved.

## Technical approach

### Saved state and effective composition

Extend `internal/agent/mcpconfig` and `internal/agent/settings/store` with the
session exclusion relation. Read and write it atomically with desired revision
and positive session IDs. Reuse the assignment primitive's session
`selection_revision` / `expected_revision`, mapped to existing desired revision,
for exclusion-aware writes. Do not introduce a second counter or session
membership precondition. Non-session membership CAS remains assignment-owned.
This explicitly supersedes the crossed `expected_excluded_definition_ids`
proposal and whichever-lands-first ordering. Session ID-set preconditions are
rejected; assignment work order 01 remains the single primitive owner.
Old additions-only writes preserve E under the same store lock.

Extend `mcp_selection_handlers.go` and the session response. Keep other scopes
additive. Extend the consistent `ReadMCPResolutionInputs` snapshot, subtract by
ID before secret resolution, and generate `session_excluded` decisions without
removing origins. Share the post-exclusion setup builder with comparison.

Session cleanup needs a narrow subscription to the existing `events.SessionRemoved`
in `registerMCPDeletionCleanup` and a settings-store session cleanup operation.
Task/workspace/definition cleanup must also handle exclusion-only sessions.

### Delivery acceptance

Retain the prepared-operation boundary and existing accepted-setup persistence.
Exercise startup, resume/load, reset, actual restart, rebind, and live adoption
with excluded definitions. Keep snapshot version 1 and extend comparison's
saved-identity guard with exclusion IDs. A failed operation retains the accepted
baseline without claiming runtime rollback.

| Delivery path | Exclusion behavior | Focused evidence | Fallback |
| --- | --- | --- | --- |
| ACP with eligible resume/load | Complete post-exclusion set at idle | Lifecycle fake client captures exact RPC and revision | Failed state retains baseline and retry |
| ACP without advertised support | Save E, no reconnect | Unsupported reconnect test | Next actual start |
| ACP with project MCP strategy | No same-process file-reload claim | Project reconnect test | Next actual start |
| Local/SSH passthrough | Prepare post-exclusion set on actual start | Existing passthrough operation tests | Save deferred state while running |
| Retained process adoption | Preserve current delivered setup | Acceptance/adoption guard regression | Pending choice remains until eligible application |
| Native provider or synthetic legacy configuration | No new catalog removal guarantee | Legacy alias and eligibility regression | Explain control's catalog-only scope |

Provider names do not establish reconnect support. Mock transport evidence does
not certify a live provider connection. No container suite is required for the
unchanged executor contract. A changed executor boundary needs a revised work order.

### Presentation

Extend the shared session controller, typed API, and origin rows. Exclusion
authority is an explicit existing-session capability, not `sessionPresentation`.
Keep connections, pending status, and configuration eligibility separate.
Use existing dialog/drawer primitives and shared Save ownership elsewhere.

## ASCII UI preview

UI-01: Desktop. Entry: composer plug > Customize. State: saved exclusion while busy.

```text
+--------------------------------------------------------------------+
| MCP servers                                                [Close] |
| [Connections] [Customize]                                          |
+--------------------------------------------------------------------+
| Changes waiting to apply                                          |
| Remove Atlas Docs from setup. Applies after this turn.             |
|                                                                  |
| This session only. Exclusions remain until you restore them.       |
| Selected for this task                                            |
| Atlas Docs                              [Restore inheritance]     |
| atlas-docs  Remote endpoint  SSE                                  |
| Selected by: Task Alpha                                           |
| Also selected by: Repository app; Repository api; Profile Review   |
| Saved: excluded from this session                                 |
| Last accepted setup still includes this definition.               |
|                                                                  |
| From agent profile                                               |
| Review Search                         [Exclude for this session]  |
| Saved: follows inheritance                                        |
|                                                                  |
| Additional servers                                               |
| [Search additional servers                                     ] |
| [ ] Other server                                                 |
+--------------------------------------------------------------------+
```

UI-02: Phone/coarse pointer. Same entry and state. The full-height phone drawer
and 80dvh tablet drawer use one column, with actions below each row's details.

```text
+----------------------------------+
| MCP servers              [Close] |
| [Connections] [Customize]        |
+----------------------------------+
| Changes waiting to apply         |
| Remove Atlas Docs from setup     |
| Applies after this turn          |
|                                  |
| This session only                |
| Until you restore inheritance    |
| Selected for this task           |
| Atlas Docs                       |
| Task Alpha                       |
| Also: app, api, Review profile   |
| Saved: excluded                  |
| Still in last accepted setup     |
| [Restore inheritance           ] |
|                                  |
| Additional servers               |
| [Search                        ] |
| [ ] Other server                 |
|       safe-area clearance        |
+----------------------------------+
```

Required structure: fixed header/view controls, one body scroller, unchanged row
order, all origins, explicit action labels, and pending actions before choices.
Phone actions measure at least 44 by 44px. Desktop ordinary actions measure
28px high. The current-setup line requires a matching known comparison. Text,
spacing, example names, and exact decoration are illustrative and localized.

UI-03: Material row and application states (inside the same body).

```text
Excluded + workspace disabled:
  Saved: excluded. Definition disabled for this workspace.
  [Restore inheritance]  Restoring will not enable the definition.
Dormant:
  Saved exclusions / Atlas Docs / No longer inherited
  [Restore inheritance]  Does not add a server.
Failed application:
  Saved change could not be confirmed. Last accepted setup retained.
  [Retry]  Or apply on the next agent start.
Conflict/rejection:
  Saved choices changed elsewhere / Could not save.
  Authoritative choices remain visible. Repeat the edit after refresh.
Unknown comparison:
  Saved: excluded. Exact changes cannot be compared yet.
Empty inheritance:
  No inherited servers. Additional servers remain available.
Initial load / refresh:
  Loading on first read only. Refresh keeps existing rows and focus.
```

UI-01/02/03 cover AC-003.1 through .5 and AC-002.4/.5. The required built-in
explanation is passive text, not a new built-in management row.

## Tests

All new test names below are planned tests, not existing coverage. Use TDD after
the implementation request. Existing neighboring tests are regression inputs.

| Acceptance criteria | Planned test and evidence |
| --- | --- |
| AC-001.1/.2 | `session_exclusions_test.go`: `TestSessionMCPExclusionsUnion` covers duplicate contributors, all repositories, session overlap, and sibling isolation |
| AC-001.3/.4/.5 | Store `mcp_session_exclusions_test.go`: `TestSessionMCPExclusionsPersistence` covers replay, reopen, dormant/disabled state, delete/recreate, session/workspace cleanup |
| AC-001.6 | Store and handler `TestSessionMCPExclusionsConflict`/`Authorization` cover stale revision, lost response recovery, mixed-client writers, exclusion-only revision advance, same-set Retry, rejected payload, and omitted-field compatibility |
| AC-001.7, AC-002.6 | Resolver `TestSessionMCPExclusionsDelivery` covers secret lookup suppression, no executable entry, reserved/synthetic targets, legacy alias guard, native-config boundary |
| AC-002.1/.2/.3/.4 | Lifecycle `mcp_session_exclusions_test.go`: `TestSessionMCPExclusionsLifecycle` covers active turn, idle RPC, restart, failure, overlapping saves, stale generation, and deferred paths |
| AC-002.5 | `setup_comparison_test.go` and `mcp-selection-changes.test.ts`: exclusion/restoration, known empty, disabled intent, unknown baseline, bounds, stale exclusion IDs |
| AC-003.1/.2/.5 | `mcp-session-exclusions.test.tsx`, editor/context tests: explicit action, retained origins, dormant/disabled rows, failed save, conflict reload, session switch, stable refresh |
| AC-003.3/.4 | Rendered desktop/phone tests: drawer/dialog geometry, keyboard/touch outcome, focus return, measured hit areas, long translations |

IDs in the table use the `AC-AGENTS-SESSION-MCP-EXCLUSIONS-` prefix.
Real PostgreSQL tests require a disposable `KANDEV_TEST_POSTGRES_DSN`; an unset
variable produces a skip, not PostgreSQL evidence.

## E2E tests

Add `tests/chat/mcp-session-exclusions.spec.ts` for Chromium and
`tests/chat/mobile-mcp-session-exclusions.spec.ts` for mobile-chrome. Reuse
`fixtures/test-base`, `helpers/mcp-pending-changes.ts`, and the real saved-selection
API. Seed separate disposable definitions/profiles, two repositories, and sibling
sessions. Restore any modified shared fixture state.

1. Exclude a multiply inherited definition during a busy mock turn. Keep origins
   visible, inspect pending removal, reload, and verify the sibling is unchanged.
   After idle, verify accepted setup and restoration. Covers AC-001.1/.2/.3 and
   AC-002.1/.2/.3/.5, AC-003.1/.2.
2. Exclude a disabled definition, re-enable upstream, and verify exclusion persists.
   Remove its contributors, restore the dormant exception, and verify no new
   positive selection. Covers AC-001.4/.5.
3. Exercise local save rejection, 409 conflict, failed application/retry, next-start
   deferral, and unknown comparison. Presentation injection is allowed for failure
   labels, but lifecycle tests must prove provider semantics. Covers AC-001.6,
   AC-002.4/.5 and AC-003.5.
4. Repeat primary exclusion/restoration by touch and keyboard. Check 320px,
   canonical phone, 767px fine pointer, 768px boundary, and 900px coarse pointer.
   Inspect one scroller, final-row reachability, focus return, and light/dark/pseudo
   wrapping. Covers AC-003.3/.4/.5.

Run changed desktop and mobile specs with existing MCP selection and pending-change
neighbors. Do not overlap E2E runs. Managed runners own isolated runtime ports.
Never use main :9998 or historical :48593/:48595 fixtures.

## Work orders

- [x] [Task 01: Persist and resolve session exclusions](task-01-persist-and-resolve.md)
- [x] [Task 02: Preserve application and recovery truth](task-02-application-recovery.md)
- [x] [Task 03: Expose responsive exclusion controls](task-03-responsive-controls.md)

Dependency order: committed assignment work order 01 -> local 01 -> 02 -> 03.
Reverify the integrated primitive and parent tip before implementation. This is
a source dependency, not permission to implement or to merge a sibling PR.
All three work orders passed their task-defined focused checks after integration
of assignment commit `f9137074309bebe99e2c7576baefe936bc20358f`.

## Documentation impact

Task 03 updates the MCP selection
how-to section in `docs/public/agents-and-profiles.md` with scope, persistence,
restore, disabled definitions, timing, failure, and native/built-in limits.
Inspect `sessions-and-review.md`, `feature-status.md`, root README, and screenshots
for contradictory claims. Do not fabricate new screenshots.

At implementation completion, reconcile the parent requirement clauses listed
in the new requirement, parent selection/pending designs, and the original ADR.
Keep additive ownership and historical parent plan/test receipts intact. Promote
the new requirement/design only when all three work orders pass.

## Verification results

Design validation on 2026-10-22:

- `python3 scripts/list-docs.py validate`: passed, including both new specifications.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `python3 scripts/lint-spec-files.test.py`: passed, 36 tests.
- `validateCoverage` from `.github/scripts/pr-docs.cjs`: actual docs-only changes
  are exempt. A preflight using the six documents and an anticipated
  `mcpconfig/selections.go` change returned covered for all three work orders,
  with no reference errors. This is a package-contract check, not a code diff.
- `git diff --check -- docs/specs docs/plans/session-mcp-exclusions`: passed.
  Each untracked file also passed `git diff --no-index --check /dev/null <file>`.
- Local Markdown target checks passed for all six new files.

Coordination revision on 2026-10-22: assignment work order 01 owns the shared
conditional-write primitive. This package consumes its committed result. The
session revision remains existing desired revision; Retry retains same-set
application semantics. Catalog validation and specification lint passed again
after this reconciliation. All six files passed local-link and whitespace checks;
the documentation-coverage preflight remains covered for all three work orders.

At that design checkpoint, product implementation and runtime tests had not run.
The records below supersede that historical status.

## Risks

- A shared handler can accidentally clear E when an older client saves additions.
- Removing origins too early can let legacy fallback reintroduce an excluded server.
- Partial provider acceptance can make a rollback claim false despite retained baseline.
- Exclusion-only sessions can escape cleanup that searches positive associations.
- Sibling edits can overlap shared selection types, copy, and parent documents.


## Implementation verification: 2026-10-23

- Backend selection, setup, exclusion, cleanup, and lifecycle suites passed with race detection.
- Real PostgreSQL mixed writers admitted one revision and rejected its stale competitor; SQLite disk reopen preserved effective suppression and restored delivery eligibility.
- SQLguard and settings-store conformance passed. Changed-Go lint passed with zero issues.
- Frontend focused suites passed: 59 cases across eight files, plus the final hook rerun after a complexity-only refactor.
- Typecheck, changed-file ESLint with zero warnings, i18n completeness, and the new-copy ratchet passed.
- Managed desktop and neighbor browser suites: 13 passed. Managed phone and neighbor suites: 9 passed. Runs were sequential and used isolated fixtures.
- Public documentation validators passed (63 validator tests, 54 pages); specification catalog/lint and whitespace checks passed.

The initial broad resolver run hit a fixture portability failure in
`TestExecNativeMCPCommandRunnerBoundsInheritedPipeAfterCancellation`: its explicit
`/usr/bin:/bin` PATH lacked `sleep` on this Nix host. A separate one-line test fix
preserves the host PATH. The exact required broad command then passed under race
for mcpconfig, settings/store, and settings/handlers. Production runner behavior
is unchanged. Mock providers do not establish live-provider health, immediate
tool revocation, or native-config removal. No merge, parent-branch change,
additional session, or worker occurred.

Implementation refinement: exclusion actions display committed choices only and
require a repeated explicit action after Reload choices. Addition drafts retain
the prerequisite controller's behavior. Pending comparison uses committed IDs
rather than the retained draft, so rejected writes cannot hide saved changes.

Public docs type: how-to. Updated `docs/public/agents-and-profiles.md`; root README,
feature-status/session guides, and screenshot catalog needed no changed instructions.
The parent verification ledger remains historical and untouched.


## Local stack rebase: 2026-10-24

The coordinating session requested the exclusions step after assignment completed
its authorized rebase. The worktree was clean. Before rewriting, created
`backup/session-exclusions-pre-assignment-rebase-20261024` at
`442e9f01f77bd091f9d12d8867fa027bc74cf83e`.

Rebased only commits after OLD prerequisite
`f9137074309bebe99e2c7576baefe936bc20358f` onto verified assignment tip
`9c9abfb13b351ced2869185eb1e730f682e30637`, which contains parent squash
`0b81ae48d4e2c358f80ac489e39388dfd8d7e6ab`. The replacement prerequisite is
`23a403dc2224a668e7f35c60ff5892952608fd3b`; range-diff confirmed its patch is
identical to the old prerequisite. It was not used as the old range boundary.

Commit mapping:

- `8ef7c8d6980910f61803d87ab5e4d57bb376458e` ->
  `3c88cd7fa833e0a94c6ba28186ce90f1a613c557` (feature).
- `442e9f01f77bd091f9d12d8867fa027bc74cf83e` ->
  `b2670fffad2b8c0566d299ce41a167bd78fcaaac` (retained verification records).

Conflicts were limited to eight settings locale files: preserved both assignment
registration keys and exclusion keys. Range-diff shows no other feature patch
change. The PATH portability fix already exists in assignment commit `c49beff60`;
Git omitted that redundant hunk and retained both exclusion verification records.
Committed-positive-ID comparison and neighboring E2E helper corrections are
unchanged. No sibling branch, remote history, or runtime was modified.

Rebase validation:

- Refreshed workspace dependencies with `pnpm install --frozen-lockfile`.
- Backend conditional writes, exclusions, resolver, handlers, and application
  integration passed under race detection:
  `go test -trimpath -race ./internal/agent/mcpconfig ./internal/agent/settings/store ./internal/agent/settings/handlers ./internal/backendapp -run 'MCP|Selection|Effective' -count=1`.
- Lifecycle acceptance/defer/retry regressions passed:
  `go test -trimpath -race ./internal/agent/runtime/lifecycle -run 'MCP|Mcp' -count=1`.
- Actual disposable PostgreSQL 17.11 passed under race detection, not skipped:
  `go test -trimpath -race ./internal/agent/settings/store -run 'PostgresMCPSessionExclusions|PostgresMCPSelection|PostgresMCPResolution|PostgresConfirmedMCPDelete' -count=1 -v`.
  This includes concurrent positive/exclusion writers, legacy omission preserving
  exclusions, reopen, cleanup, owner-scoped CAS, import, and deletion.
- Ten frontend suites passed, 68 tests: the original eight focused exclusion
  suites plus registration completion and definition-form integration tests.
- `pnpm run typecheck`, `pnpm run i18n:check`, and `pnpm run i18n:ratchet` passed.
- Fresh managed desktop build and browser regressions passed, 13 cases:
  `pnpm e2e:run --host --project chromium tests/chat/mcp-session-exclusions.spec.ts tests/chat/mcp-pending-changes.spec.ts tests/task/task-mcp-selection.spec.ts`.
  Log: `/tmp/kandev-run.e2e.Lhc5dbMZ.log`.
- Phone browser regressions passed, 9 cases, sequentially against that fresh build:
  `pnpm e2e:run --host --no-build --project mobile-chrome tests/chat/mobile-mcp-session-exclusions.spec.ts tests/chat/mobile-mcp-pending-changes.spec.ts tests/task/mobile-task-mcp-selection.spec.ts`.
  Log: `/tmp/kandev-run.e2e.ioZDYK52.log`.
- Specification catalog (403 decisions, 1732 specifications), full spec lint,
  and whitespace checks passed.

Existing work-order results above remain historical evidence. Rebase validation
does not establish live-provider health. Planned PR base remains
`feature/clarify-mcp-assignme-mhh`. No push or merge is authorized by this step.


## Draft publication stack: 2026-10-24

The coordinator relayed the user's updated publication request: all four MCP
follow-ups publish draft checkpoints, without CI/review waiting or handling.
The current stack is main -> `feature/improve-mcp-manageme-mpe` ->
`feature/clarify-mcp-assignme-mhh` -> `feature/allow-session-only-i-2gn`.
The exclusions PR targets assignment, not main. Management remains a design
checkpoint; assignment WO03 still awaits management navigation. This draft does
not claim those sibling work orders are complete. Exclusions WO01/02/03 are
implemented and retain their completed results.

Verified remote assignment head:
`6faaa61b817dc9dddc60d2c12026cc1273748a13` (draft PR #4760), on management
`f7bbe6076477eebace8740a42adb206e7229e3e4` (draft PR #4759).
Backup `backup/exclusions-pre-publication-stack-20261024` preserves
`70bc5b29ee07cec8dae3db5e792f2826e864be3a`. Replayed only exclusions-owned
commits after OLD assignment cut point `9c9abfb13b351ced2869185eb1e730f682e30637`
onto the new assignment head. No conflicts occurred; range-diff shows all three
patches unchanged:

- `3c88cd7fa833e0a94c6ba28186ce90f1a613c557` ->
  `8deeb1d125fa8b5ad91be7568b1317c42162af77`.
- `b2670fffad2b8c0566d299ce41a167bd78fcaaac` ->
  `8822bfaf2f0c4320ce26b4d6fddb29ef1fe81236`.
- `70bc5b29ee07cec8dae3db5e792f2826e864be3a` ->
  `20b498ed916c99ecfa35af14aab82d8170ef0c4d`.

Committed-positive-ID comparisons, exclusion-preserving conditional writes, and
neighbor E2E helper corrections are unchanged. No sibling branch was mutated.
The manual test instance at port 48671 was shut down at the user's request;
the main instance at port 9998 was untouched.

Post-stack validation:

- `go test -trimpath -race ./internal/agent/mcpconfig ./internal/agent/settings/store ./internal/agent/settings/handlers ./internal/backendapp -run 'MCP|Selection|Effective' -count=1`: passed.
- Actual disposable PostgreSQL 17.11 selection/exclusion concurrency,
  resolution, import/deletion, and cleanup checks under race detection: passed.
- Ten focused frontend suites: 68 tests passed. Typecheck and i18n checks passed.
- Fresh managed desktop build and regressions, including desktop/phone screenshot
  capture: 14 passed (`/tmp/kandev-run.e2e.0hXF3dVv.log`). Capture assets were
  inspected and compressed. The temporary capture spec was removed.
- Phone regressions passed: 9 cases, sequentially against the same build
  (`/tmp/kandev-run.e2e.M7qzBerK.log`).
- Documentation catalog (403 decisions, 1738 specifications), full spec lint,
  and whitespace checks passed.

Unchanged-source lifecycle, SQLguard, store-conformance, lint, and localization
ratchet receipts remain recorded in the earlier work-order results; those broad
checks were not repeated solely for this conflict-free rebase. Mock fixtures do
not establish live-provider health. The final remote SHA and draft URL are
recorded in the saved task plan and coordinator handoff after publication.
No CI watcher, reviewer request, auto-merge, or merge is part of this checkpoint.


## Assignment completion integration: 2026-10-25

The coordinator authorized routine integration after assignment work order 03.
Verified the published assignment branch at
`fe08307e9f4701715e9fc906b1d65916c18971e1`, based on management
`45e90466378f982e13cc762c52432f14ab43c32c`. Assignment reports all its work
orders complete. The earlier pending-work descriptions above are historical.
The PR remains draft #4761 targeting `feature/clarify-mcp-assignme-mhh`.

The clean original tip `69be161548702b4be530536f925777da628302e0` is preserved
by `backup/exclusions-pre-assignment-wo03-20261025`. Replayed only the four
exclusion-owned commits after OLD assignment
`6faaa61b817dc9dddc60d2c12026cc1273748a13` onto the verified replacement:

- `8deeb1d125fa8b5ad91be7568b1317c42162af77` ->
  `ecf6743c2a664ce436fd667bbab74b2e5d33912d`.
- `8822bfaf2f0c4320ce26b4d6fddb29ef1fe81236` ->
  `80f0ba5f04d08c8e8427ce3a9915f9f28a8e9bab`.
- `20b498ed916c99ecfa35af14aab82d8170ef0c4d` ->
  `64411a07c0de2f19dbcda1e6c315f6be8461e1d9`.
- `69be161548702b4be530536f925777da628302e0` ->
  `d7c38c3df8c1fa24917ac47d683fd04e6ea9cd18`.

Resolved shared response, hook, selector, and locale conflicts by preserving
both contracts: profile legacy-import status, assignment route hints and explicit
Customize activation, session origins, exclusions, and saved-versus-applied
comparison. Assignment now owns the identical committed-positive-ID correction
and both neighboring E2E helper corrections; these remain present once and no
longer appear in the exclusions diff. Range-diff shows the other three replayed
commits unchanged. No parent or sibling commits were replayed.

Combined UI code exceeded the existing function length/complexity limits.
Extracted the row annotation into the existing exclusions module, combined the
identical exclusion capability guards, and destructured the accepted selection
snapshot once. Existing behavior tests passed before and after this refactor;
no behavior or new product scope was introduced.

Affected checks:

- MCP/selection/effective Go suites passed under race detection in mcpconfig,
  settings/store, settings/handlers, and backendapp. This includes the new
  assignment eligibility contract and exclusions on the integrated source.
- Actual disposable PostgreSQL 17.11 concurrent selection/exclusion writers,
  owner conditions, resolution snapshots, import/deletion, and cleanup passed
  under race detection. Receipt: `/tmp/exclusions-wo03-postgres.log`.
- Eight focused frontend suites passed (66 tests); after the refactor, the
  session hook, rendered exclusions, and context-switch suites passed (18 tests).
- Typecheck, i18n completeness, changed frontend lint with zero warnings, and
  changed handler Go lint with zero issues passed.
- Specification catalog (403 decisions, 1738 specifications), full spec lint,
  and whitespace checks passed.

The first managed browser attempt completed the fresh build but inherited the
previous preview's Tailscale bind address. Fixture startup failed before browser
assertions. The owned process group was stopped; the wrapper's zero exit without
a summary is not a passing test receipt. Reused that fresh build with the existing
isolated browser library path and explicit loopback bind. Initial log:
`/tmp/kandev-run.e2e.kYD7CBJ1.log`. Main port 9998 was not changed.

Browser integration passed on the fresh build, sequentially, with one worker
and retries disabled:

- Desktop: 22 passed (`/tmp/kandev-run.e2e.QCr4CCVz.log`), covering exclusions,
  pending changes, task selection, assignment handoff, and the capture scenario.
- Phone: 17 passed (`/tmp/kandev-run.e2e.XhxN3AA8.log`), covering exclusions,
  pending changes, task selection, and assignment handoff.

Commands used the managed host runner with `--no-build` only after the completed
fresh build above. The final source was unchanged between those runs. Existing
lifecycle and SQLguard/store-conformance receipts remain applicable to unchanged
source; no full suite, CI waiter, or review handling was added.

Fresh desktop and phone screenshot assets were then captured with
`CAPTURE_PR_ASSETS=1` in a dedicated one-case managed run, passed without retries
(`/tmp/kandev-run.e2e.rlWdhyZC.log`). Both manifest entries were checked against
this run, compressed, and visually inspected. The disposable capture spec was
removed. The images show the final integrated source; later changes are this
verification record only. The full hook receipt and final remote SHA are recorded
in the saved task plan and coordinator handoff after publication.


## Inline profile dependency integration: 2026-10-25

Verified assignment remote `f3be6a5186f43dcce4487371c7a493be6c9ec280`, containing
management `7aca966a6ec262eb1ce839fa883f66e23441d477`. This dependency update
makes profile choices inline and adapts the assignment focus hint. The session
still requires an explicit Customize action. No new exclusion scope was added.

Preserved the clean published exclusions tip
`9d5a5250c9b8a0eca92842f61fce0a32071f0124` as
`backup/exclusions-pre-inline-profile-20261025`. Replayed only own commits after
OLD assignment `fe08307e9f4701715e9fc906b1d65916c18971e1` onto the new tip.
No conflicts occurred; range-diff marks all five patches identical:

- `ecf6743c2a664ce436fd667bbab74b2e5d33912d` ->
  `8f67dd5a3f833fef7c6e3c78bac22d0226a62f6e`.
- `80f0ba5f04d08c8e8427ce3a9915f9f28a8e9bab` ->
  `f463e7ae99f7c995d0d80a51ab0a07c8c3cf16dc`.
- `64411a07c0de2f19dbcda1e6c315f6be8461e1d9` ->
  `8a107656e3e1e01534b1b51467075aae32850e83`.
- `d7c38c3df8c1fa24917ac47d683fd04e6ea9cd18` ->
  `2db5889451b5b5afd781b531277ba77d8f30138e`.
- `9d5a5250c9b8a0eca92842f61fce0a32071f0124` ->
  `2925ed62fd7e7eabd68d0bebe986cde1ac34cdfe`.

The backend, session hook/selector, and explorer source are byte-identical to
the previous verified publication. Existing race/PostgreSQL/lifecycle receipts
remain applicable; they were not rerun for this frontend-only dependency update.
The shared picker change removes an obsolete reveal effect from its wrapper;
the session uses the unchanged content component. Current exclusions screenshots
still show the unchanged surface. Profile UI evidence belongs to the prerequisite
PRs; no unrelated profile screenshot replaces the exclusions evidence.

Focused integration validation:

- Six frontend suites passed, 29 tests: shared picker, exclusion component,
  session hook, assignment focus, settings draft, and profile configuration.
- Full web typecheck passed. Specification catalog and full spec lint passed.
- Fresh managed desktop build and four relevant browser journeys passed with
  one worker and retries disabled (`/tmp/kandev-run.e2e.Md4DCfB7.log`): exclusion
  restore, global profile assignment, workspace-owned profile assignment, and
  explicit session Customize handoff.

- The same four journeys passed on phone, sequentially against that fresh build,
  with one worker and retries disabled (`/tmp/kandev-run.e2e.F0XVo080.log`).

Only this integration record was added after checks. Normal hook receipt and
final local/remote/PR SHA are recorded in the saved task plan and coordinator
handoff. PR #4761 remains draft on assignment. No CI/review handling, merge,
sibling edits, new agents, or runtime changes are part of this update.

## Management squash integration: 2026-10-26

Verified assignment remote `7604e81b0446062a5600f07857062380d0a1c587` contains
actual management squash `e5c38184cf9c191075dc88c1ffad28c636858bf5` on main.
Management PR #4759 is merged; assignment PR #4760 now targets main. Exclusions
PR #4761 continues to target the assignment branch and remains draft.

Preserved the clean published exclusions tip
`77a3e83a6445c8a53066fdf8a7c3d0caac4ccbee` as
`backup/exclusions-pre-management-squash-20261026`. Replayed only own six commits
after OLD assignment `f3be6a5186f43dcce4487371c7a493be6c9ec280` onto the new
assignment head. The resulting range contains only these exclusion-owned commits;
no management or assignment commit was replayed:

- `8f67dd5a3f833fef7c6e3c78bac22d0226a62f6e` ->
  `8b537c513b2480269b61d3ea8edeb0ca96ce5fe6`.
- `f463e7ae99f7c995d0d80a51ab0a07c8c3cf16dc` ->
  `586a1a145f99581970881c66487e453ea604fa40`.
- `8a107656e3e1e01534b1b51467075aae32850e83` ->
  `a3e92b0de43fdb86a3c446246a9cc7b808a7df14`.
- `2db5889451b5b5afd781b531277ba77d8f30138e` ->
  `4e212bcc3e11e4ebabe0fe01e42c38641a947d10`.
- `2925ed62fd7e7eabd68d0bebe986cde1ac34cdfe` ->
  `b54f3d59cac47b2917e4f599e48305f93d0ba6d5`.
- `77a3e83a6445c8a53066fdf8a7c3d0caac4ccbee` ->
  `4903e6115223b64ffad5b87c82e93d7a0a19a051`.

Conflicts were limited to appended task-locale keys in eight catalogs. Preserved
both the new main question-image keys and the session-exclusion reason. Range-diff
shows only the changed neighboring locale context in the feature patch; the other
five replayed patches are identical. The shared settings draft hook and its
suspended-render test are byte-identical to assignment, including the layout-effect
ref publication and expected_definition_ids assertion. Inline profile hint readiness
and explicit session Customize behavior remain intact.

The MCP resolver/store/handler code and session exclusion UI are byte-identical
to the prior publication. Existing PostgreSQL concurrency, lifecycle, SQLguard,
store-conformance and session screenshots remain applicable; the main/base update
does not change the screenshot's user-visible exclusion surface. No new product
implementation was needed.

Affected checks completed on the integrated source:

- `go test -trimpath -race ./internal/agent/mcpconfig ./internal/agent/settings/store ./internal/agent/settings/handlers ./internal/backendapp -run 'MCP|Selection|Effective' -count=1`: passed, including application wiring on the merged main baseline.
- Eleven focused frontend suites passed, 91 tests. This includes selection
  editor/context, settings draft, assignment focus, session exclusion rendering,
  pending comparison, API, and task-route admission tests.
- Full web typecheck and i18n completeness passed. All eight merged task catalogs
  retained the upstream question-image keys and exclusion reason.
- Catalog validation (403 decisions, 1744 specifications), full spec lint, and
  whitespace checks passed.
- Fresh managed desktop build and 21 focused Chromium E2E cases passed with
  zero retries; log `/tmp/kandev-run.e2e.92U49Np8.log`.
- The same build passed 17 mobile Chrome E2E cases with zero retries; log
  `/tmp/kandev-run.e2e.FEoGoO2J.log`. Both runs covered exclusion/restore,
  committed-versus-pending changes, task origins, assignment handoff, and error
  recovery. Desktop additionally covered narrow fine-pointer and coarse-pointer
  tablet paths. Runs were sequential, with one worker each.

Only this integration record changed after validation. Normal commit-hook receipt
and the final local/remote/PR SHA are recorded in the saved task plan and coordinator
handoff. Publish with the captured exact exclusions remote-OID lease. PR #4761
remains draft on assignment; the next replay boundary is
`7604e81b0446062a5600f07857062380d0a1c587`. No CI/review handling, merge, sibling
edits, new agents, or main runtime changes are part of this update.


## Completed assignment review integration: 2026-10-27

Verified assignment remote `74069f8b52326eb09180846b6fd3f42503017413` and
PR #4760 OPEN/READY on main. It is not merged. Exclusions PR #4761 remains
DRAFT on `feature/clarify-mcp-assignme-mhh`.

Preserved clean own tip `fd8bb59aaa4589fc99926b66367fa48ef3b4464a` as
`backup/exclusions-before-assignment-review-20261027`. Replayed only seven own
commits after actual integrated assignment
`7604e81b0446062a5600f07857062380d0a1c587` onto verified assignment `74069f8b`.
Intervening assignment tips were never used as the old boundary.

- `8b537c513b2480269b61d3ea8edeb0ca96ce5fe6` -> `ef1366051864cea0454920f9884a6e3efbc22f82`.
- `586a1a145f99581970881c66487e453ea604fa40` -> `d923141e9422c0d3387d609ab2f0d98f680a0532`.
- `a3e92b0de43fdb86a3c446246a9cc7b808a7df14` -> `279e2adfef0fc61fcef03b3fdbda30013aaa2392`.
- `4e212bcc3e11e4ebabe0fe01e42c38641a947d10` -> `7eb92799d319439770e5782cf6859edba9fda4f0`.
- `b54f3d59cac47b2917e4f599e48305f93d0ba6d5` -> `b03277b8f2d48088e6ac72be0cb70454ed8b9e0a`.
- `4903e6115223b64ffad5b87c82e93d7a0a19a051` -> `74efb956583f9a6e84310d12854796269959015e`.
- `fd8bb59aaa4589fc99926b66367fa48ef3b4464a` -> `609d1e8a879aa6c7efb16990b9b3ba0279aaabc5`.

Conflicts were limited to the shared selection service and public recovery prose.
Kept assignment's public `ReplaceConditional` addition-eligibility wrapper and
internal task-session `Replace` path through private `replaceConditional`.
Moved exclusion-context preparation into that shared private operation after
condition/owner validation and before the atomic repository write. Internal launch
writes still preserve omitted exclusions without acquiring HTTP runtime eligibility.
Public writes retain eligibility and exclusion validation. Kept nil positive-ID
presence semantics, exact committed snapshots, and postcommit notification.

Inherited `a3a9f8dd8fff675dc18466a982b823c2ba16ac20`: membership comparison sorts a
clone of saved IDs, preserving the snapshot's database order. Shared editor
layout-effect ref publication also remains intact. Session exclusion controls and
session hook are byte-identical to the previous publication. Existing session
screenshots remain applicable. Public how-to recovery prose preserves upstream
paragraph structure and explains committed-only exclusion actions and explicit
retry after reload. Six non-feature replayed patches are identical.

Validation on the integrated source:

- Eleven focused frontend suites: 92 tests passed, including shared suspended-render
  context/draft guards, session exclusion rendering, pending comparison, and APIs.
- Disposable PostgreSQL 17.11 UTF-8 cluster under race: numeric ICU membership,
  mixed assignment/exclusion writers, conditional repository/session writers,
  import/deletion, resolution snapshot, and confirmed deletion all passed. Log:
  `/tmp/exclusions-assignment-review-postgres-utf8.log`.
  The initial disposable cluster used SQL_ASCII and rejected ICU collation creation;
  reran with explicit UTF-8, without source changes or test skips.
- Specification/catalog checks passed (409 decisions, 1769 specifications).
  Public-doc validation passed for 54 pages and all 63 validator tests.
- Changed Go lint passed with zero issues.
- `go test -trimpath -race ./internal/agent/mcpconfig ./internal/agent/settings/store ./internal/agent/settings/handlers ./internal/backendapp -run 'MCP|Selection|Effective' -count=1`: passed, including internal launch eligibility, public assignment admission, exclusions round-trip and application wiring.
- Full web typecheck and i18n passed: all six real locales complete, pseudo in
  sync, and 4170 guarded source files checked.
- Fresh managed build: 21 Chromium cases passed without retries; log
  `/tmp/kandev-run.e2e.Fhw8i2Ih.log`. Same build: 17 mobile Chrome cases passed
  without retries; log `/tmp/kandev-run.e2e.WAM1Et7M.log`. Runs were sequential,
  one worker each. Coverage includes exclusions/restore, committed pending state,
  assignment navigation, ownership/recovery, origins and coarse-pointer controls.

Only this verification record changed after validation. Normal commit-hook receipt
and final local/remote/PR SHA are recorded in the saved task plan and coordinator
handoff. Publish using the captured remote-OID lease
`fd8bb59aaa4589fc99926b66367fa48ef3b4464a`. The next replay boundary is
`74069f8b52326eb09180846b6fd3f42503017413`. No CI monitoring, review handling,
merge, sibling edits, new agents, or main-runtime changes were performed.


## Assignment descendant merge integration: 2026-10-28

Independently verified assignment remote
`6f15a8dca3ffc4d512dcd75fd04545ae2134b25c`, PR #4760 OPEN/READY on main,
not merged. Exclusions began clean at
`359158d447267dc640975899881625a5530d878f`. The actual shared ancestor remains
`74069f8b52326eb09180846b6fd3f42503017413`.

Saved `backup/exclusions-before-assignment-merge-20261028` at the previous
exclusions head. Assignment advanced by a normal merge, so integrated the verified
head with a merge and retained every existing exclusions commit ID. No upstream
commits were replayed and no published history was rewritten. The next integrated
assignment boundary is `6f15a8dca3ffc4d512dcd75fd04545ae2134b25c`.

Automatic integration had no conflicts. Overlap was limited to the settings schema,
eight task-locale catalogs, and public agent documentation. The settings schema
retains the exclusion table/index alongside upstream Copilot profile persistence.
Inherited CAS sort-clone comparison, internal/public replacement distinction,
exclusion omission/revision semantics, and shared editor guards are unchanged.
Session exclusion controls are unchanged, so existing screenshots remain applicable.
New upstream public sections coexist with the exclusion recovery instructions.

Affected verification on the integrated tree:

- Eleven focused frontend suites passed, 87 tests. Upstream changed task-route
  tests; the same selected suite paths were run. Exclusion/shared-editor coverage
  remains present.
- Real PostgreSQL 17.11 UTF-8 tests passed under race: numeric ICU membership,
  concurrent assignment/exclusion writers, conditional repository/session writes,
  resolution snapshots, import/deletion and confirmed deletion. Log:
  `/tmp/exclusions-20261028-postgres.log`.
- Documentation catalog (413 decisions, 1791 specifications), full spec lint,
  54 published pages and all 63 public-doc validator tests passed.
- Go race checks passed in mcpconfig, settings store, settings handlers,
  backendapp and lifecycle with `-run 'MCP|Selection|Effective' -count=1`.
  This includes exclusion idle acceptance/failure and internal/public selection
  eligibility. Log: `/tmp/exclusions-20261028-go.log`.
- Full web typecheck and translation validation passed; pseudo stays synchronized
  and all six real locales are complete (4187 guarded source files checked).
- Fresh managed build: 21 desktop cases passed without retries, log
  `/tmp/kandev-run.e2e.lGkewyxG.log`. Same build: 17 phone cases passed without
  retries, log `/tmp/kandev-run.e2e.VBvilgDm.log`. Runs were sequential with one
  worker; included exclusions/restore, pending truth, assignment navigation,
  origins, narrow fine-pointer and coarse-pointer controls.

Only this own verification record was added to the automatic merge tree. Final
merge SHA/parents, normal hook receipt and remote/PR identity are recorded in the
saved task plan and coordinator handoff. PR #4761 remains draft on assignment.
No CI monitoring, review handling, PR merge, sibling edits, new agents, or main
runtime changes were performed.


## Ready review and main rebase: 2026-10-28

The user explicitly requested ready status, CI/comment remediation and conflict
resolution by rebase. This supersedes the earlier draft/no-review checkpoint.
PR #4761 was marked ready. Assignment #4760 merged as
`4222c3654f9bb1efd1a7d27b5151c4b4c4f1ffd2`; GitHub retargeted this PR to main.
Current main at rebase was `d17d84a8084b15ddf1df10ff465a51d27c780e64`.

Backup `backup/exclusions-before-main-fixup-20261028` preserves
`8a71af4c2bbc169a3b326e5d484e93fec7e754d5`. Replayed only eight own non-merge
commits after the integrated assignment boundary
`6f15a8dca3ffc4d512dcd75fd04545ae2134b25c` onto main. The previous merge's
verified remerge delta contained only this plan's 52-line verification record;
restored that record byte-for-byte after rebase. No upstream assignment history
was replayed. Rebase completed without file conflicts. MCP writer/resolver and
session exclusion source remain byte-identical to the previous head.

Initial exact-head snapshot had no actionable review comments or failed checks;
required checks were missing while the PR conflicted. Fresh CI and review
verification is pending the rebased publication. No PR merge is authorized.


Local verification for the ready-review rebase:

- All five affected Go race packages passed: MCP resolution, settings store,
  settings handlers, backend application wiring and runtime lifecycle.
- All 64 focused frontend cases passed; full typecheck and translation checks
  passed (4198 guarded files). Catalog/spec and public-doc validation passed.
- Fresh managed build: 21 desktop cases passed without retries, recorded in
  `/tmp/kandev-run.e2e.lyCI56oY.log`.
- The phone run passed 16 cases and failed the pending-state case with a missing
  `mcp_state` in the intercepted response and shutdown HTTP 503 evidence. The
  initial failure is retained in `/tmp/kandev-run.e2e.hvuUoHcn.log`. No product
  cause was established. That single case then passed in a fresh traced run
  (`/tmp/kandev-run.e2e.h38yuPfm.log`) and two further fresh-worker repetitions
  (`/tmp/kandev-run.e2e.vTDln1M9.log`), all with retries disabled. No source or
  timeout change was made to obtain those results.
- Main subsequently advanced to `aa229b60dd9281043f7638767777222f5723f710`.
  Its only added change is database-maintenance JSON output (#4903), with no
  overlap in MCP source, browser source, fixtures or dependencies. Backed up
  the first rebased tip at `backup/exclusions-before-main-aa229-20261028`,
  preserved this uncommitted record as a patch, and replayed the same eight
  owned commits without conflicts. The final tree differs from the tested tree
  only in those upstream maintenance files. Focused backend/frontend checks
  passed again on the final tree (three Go race packages and all 64 frontend
  cases); GitHub CI and reviewer findings remain pending.

### Ready-review application retry correction

Greptile thread `PRRT_kwDOQ2-eWs6xLXDu` identified a valid draft-preservation bug.
Application Retry used the addition-edit callback, replacing a retained rejected
draft with saved IDs even when the accepted revision was invalid. Two regression
cases failed before the fix and passed after it. Retry now writes saved IDs
without editing the draft and is unavailable until Reload choices succeeds after
a rejected save. The shared editor rejects stale captured callbacks without a
write; exclusion omission continues to preserve committed exclusions. Desktop
and phone pending-state journeys now cover this combined recovery path. Final
validation passed: 66 frontend cases, typecheck, changed-file lint/format,
catalog/spec checks and both desktop/phone recovery journeys with retries
disabled. Browser logs: `/tmp/kandev-run.e2e.N7s3C453.log` and
`/tmp/kandev-run.e2e.G4SYbNo5.log`. GitHub thread disposition remains pending
publication of the fix.

CodeRabbit's grouped review comment in review `5685434777` identified that both
viewport captures shared a manifest identity. The capture helper now supplies
separate desktop/phone keys and frames the actual Reload choices action before
capture. Its existing seven manifest/capture tests pass, including preservation
of separate keys from one spec. This correction changes test artifacts only;
production retry behavior and its existing verification remain unchanged.

Both desktop and phone capture/recovery journeys passed with retries disabled
after the identity correction; their manifest test identities are distinct.
Logs: `/tmp/kandev-run.e2e.EGVVcE5L.log`, `/tmp/kandev-run.e2e.jxqzP7M2.log`.

### Ready-review rebase after task-creation recovery merged

Main advanced from `aa229b60dd9281043f7638767777222f5723f710` to
`91a20b1373c567ad7b138d6d509fc18d80d3a3a6` during CI. Backed up the
published head `698ce064d727906f1f39339b94abbe96dc7bf8c7` at
`backup/exclusions-before-main-91a-20261028` and replayed the eleven owned
commits. The only conflicts were the eight task translation catalogs: retained
all upstream task-creation recovery copy and the unchanged session-exclusion
key. Duplicate-key validation and exact parsed comparisons confirmed that each
catalog equals upstream plus that key. Range-diff shows only the changed insertion
context; the other ten patches are identical. MCP source and review fixes remain
byte-identical to the previous published head.

Validation on the reconciled tree passed: 73 focused frontend/capture tests,
three MCP Go race packages, full frontend typecheck, translation checks (4208
guarded files), changed-file ESLint, catalog formatting, documentation catalog
and full specification lint. A fresh managed build passed four desktop cases
(pending/retry, normal exclusions, narrow fine-pointer and coarse-pointer tablet)
and two phone cases, all with retries disabled. Logs:
`/tmp/kandev-run.e2e.V0FZANvJ.log` and `/tmp/kandev-run.e2e.s1u9suhI.log`.

The prior head's Greptile thread was replied to and resolved; its CodeRabbit
review was clear after the capture-identity correction. Claude's partial backend
review had no blockers; its session revision question was answered. The prior
Cloudflare docs-preview failure was provider error 10500; attempt 2 passed
without source changes. These are historical review/CI receipts, not validation
of the rebased head. New-head CI and automated review remain pending publication.

### CI assertion correction for failed MCP application

E2E shard 9 reported the same stale assertion on desktop and tablet: it required
"stays in use" for failed application. The exclusion design deliberately keeps
only the last accepted setup for comparison after failure; it cannot claim that
setup remains active. All eight CI error contexts showed the correct rendered
message. The exact desktop case reproduced locally with retries disabled
(`/tmp/kandev-run.e2e.kdA0Fx11.log`).

The shared status helper now checks the comparison-only wording and rejects
"stays in use" for failed application, while preserving the active-setup checks
for pending-idle and deferred-restart states. No production behavior or public
documentation changed. Both desktop/tablet cases passed
(`/tmp/kandev-run.e2e.hrUojzhD.log`), and all three phone widths passed
(`/tmp/kandev-run.e2e.bUbQwn2u.log`), with retries disabled. Changed-file ESLint
and Prettier passed. New-head CI remains pending publication.

### CI retry audit and rebase onto the plugin SDK merge

Backed up local `bc562858dbd6cd19fc39506903cdce9cc146587a` at
`backup/exclusions-before-main-3a740-20261028`, then replayed the thirteen
owned commits after `91a20b1373c567ad7b138d6d509fc18d80d3a3a6` onto
main `3a740c6911a340e73ade7edad6f4b234035f2fc9`. The rebase was clean;
all thirteen patches are identical by range-diff. This consumes main's
scroll-settlement correction for the mobile Threads fixture and preserves its
causal MCP response/reload helper. Dependency installation completed normally.

The complete available browser blobs for published `c92eae774` contained five
failed MCP cases (desktop, tablet and three phone widths), all from the stale
failed-application assertion corrected above. All twelve phone error contexts
matched the same cause. Three other cases passed only after retry: mobile
Threads (fixed upstream), Quick Chat recovery, and LSP hover.

Quick Chat's failure artifact showed that the synthetic failed session resumed
and became resolved. A local fixture invariant reproduced the defect: the
supposed bootstrap failure still had a live mock runtime (expected false,
received true; `/tmp/kandev-run.e2e.45XDN34U.log`). The fixture now stops that
runtime and confirms it stopped before seeding the failure. It disables
open-time automatic restart for this explicit-recovery case and restores the
original setting in `finally`. All four setup-recovery cases pass without
retries (`/tmp/kandev-run.e2e.gaYqMudA.log`). No product recovery behavior changed.

The LSP hover failure did not reproduce in isolation or after its five preceding
cases on two CPU cores: six cases passed without retries
(`/tmp/kandev-run.e2e.4NmXFtXq.log`). Its screenshot and protocol assertion were
inspected; no speculative LSP change was made. Fresh CI remains required.
The PostgreSQL protected-sweep failure also did not reproduce in five race runs
each on PostgreSQL 16 and 17; one failed-job rerun is in progress. Neither local
passes nor pending reruns are treated as a clean CI result.

The rebased production tree is byte-identical to validated synthetic merge
`57465eaf3384c81a25457c9b96ea97f8aa580b6b`: only test/documentation corrections
differ. That tree passed five affected backend race packages and desktop/phone
MCP recovery. On the actual rebased branch all 73 focused frontend/capture cases
pass again. Public docs and screenshots remain accurate because these changes
only correct fixtures and consume upstream work.

Full frontend typecheck, changed fixture ESLint/Prettier, documentation catalog
validation and specification lint also passed on the rebased branch. The existing
public feature/recovery documentation and screenshots need no change for this
fixture-only correction.

CodeRabbit review `5688243458` requested removal of the retained-recovery
button's legacy 30-second visibility timeout. The assertion now uses the default
timeout after the existing persisted FAILED-state poll and visible error
assertion. The stopped-runtime and temporary auto-start fixture guards remain.
The exact recovery case passed with retries disabled and the default timeout
(`/tmp/kandev-run.e2e.1cBGvllL.log`); changed-file ESLint passed.


### Damaged-workspace fixture isolation (2026-10-28)

The prior published head completed CI with 89 passed and 11 skipped checks.
The complete 118-report browser audit still found one retry: storage discovery
ran after deletion fixtures that left an empty workspace marker. The storage
page correctly reported unavailable data. The three-case local sequence
reproduced the same undefined-total failure with retries disabled
(`/tmp/kandev-run.e2e.lEjGjLo1.log`).

The damaged-task helper now retains the original marker and exposes cleanup.
All callers restore it and remove the fixture-owned file in `finally`, after
the existing retained-file, branch, deletion and restart assertions. Cleanup
does not recreate a marker already removed by normal cleanup. No storage or
deletion product behavior, assertion timeout, or retry policy changed.

The same three-case sequence passes (`/tmp/kandev-run.e2e.iMnuHdJ2.log`).
All three phone callers pass (`/tmp/kandev-run.e2e.4xvd13h8.log`), as does the
desktop Tasks page caller (`/tmp/kandev-run.e2e.PzxRu3Gg.log`), with retries
disabled. Full typecheck and changed-file ESLint/Prettier pass. Public docs and
UI captures remain accurate. Fresh CI and automated review are required after
publication; the preceding green checks do not waive the observed retry.

Main advanced without overlapping PR paths. The exact synthetic merge of the
preceding production tree with main `f9bfdcd8907d57a067bce3484d77dd75a1cc32b7`
passed 79 frontend cases, full typecheck, five MCP backend race packages, the
streams race package, and desktop/phone MCP pending-retry journeys. Synthetic
commit: `ac14dfab98ebbb626bfd3fb943b5cf37043bcefd`. The current correction changes
only test cleanup and this record; final base/head evidence remains external.


### Rename fixture Git-status ordering (2026-10-28)

The next published head completed 90 passing and 11 skipped checks. All 118
browser reports were audited: the storage-discovery failure was gone, but the
blur-rename case passed only after retry. Its disk assertions passed while the
screenshot showed Changes active instead of Files. Three ordinary isolated runs
passed (`/tmp/kandev-run.e2e.zpr0WjpK.log`), so a temporary WebSocket probe held
only the real renamed-file status notification. Releasing it after the existing
Files selection reproduced the exact missing-tree failure
(`/tmp/kandev-run.e2e.6eaabzJA.log`).

Waiting for the renamed file in Changes before selecting Files passed the same
controlled ordering (`/tmp/kandev-run.e2e.rt8ZhLMz.log`). The temporary probe was
removed. Both committing rename cases now wait for that visible Git update;
all disk/tree assertions and existing timeouts remain. All four rename cases
pass without retries (`/tmp/kandev-run.e2e.P9Nb7jrA.log`), as do changed-file
ESLint and Prettier. An initial local reproduction could not launch full
Chromium because host libraries were absent; a process-local library path
resolved that environment issue before the recorded test runs.

No product behavior or public documentation changed. Current-main integration
and the next exact-head CI/review cycle remain required before delivery.


### Translation conflicts after main advanced (2026-10-28)

Main advanced again while CI ran. Rebased the 17 owned commits after
`2863069ed7251f5da73ad6be1411be6a79d2914c` onto
`a33ff4139ada169271a8177fa181c45b0c0176e7`. Backup
`backup/exclusions-before-main-a33ff-20261028` preserves the published
`fe1e2b0ecc9ae496434adf064eab70ab0b6b4d3f`. Only the eight task catalogs
conflicted. Duplicate-key-aware three-way comparison found no conflicting
values; each resolved catalog is the exact semantic union of both inputs.
The exclusion reason is now beside the existing MCP reason keys, avoiding
repeated end-of-file conflicts with unrelated catalog additions. Values are
unchanged. No source or test logic changed in this integration.

Validation passed: 73 focused frontend cases; full typecheck and i18n checks;
five affected backend race packages covering MCP, selection, effective state,
runtime storage and prompt suggestions; two desktop and two phone MCP journeys
with retries disabled (`/tmp/kandev-run.e2e.5HHH6XQU.log` and
`/tmp/kandev-run.e2e.csEcdP5m.log`); harness/spec tests and lint; conflict-file
normal hooks and Prettier. Public feature docs and screenshots remain accurate.
Fresh published-head CI/review remains required; prior-head progress is
historical and is tracked externally.


### Current-head browser audit and Settings recovery, 2026-10-29

The full 118-report browser audit for `d7b7dc4ab` found four retry signals:
workspace creation lost its new card, a phone profile drag overshot its target,
Send Now did not render its selected message in time, and the phone oversized
message response selector matched both the prompt and response. No assertion
or timeout was relaxed.

A deferred Settings bootstrap regression reproduced the workspace loss and
rename rollback before the fix (`/tmp/exclusions-workspace-red.log`). Bootstrap
now refreshes its snapshot if the workspace list changed while loading, then
applies the result only while mounted. This preserves the existing active-
workspace revision behavior. Ten focused bootstrap/state tests and typecheck
passed. This restores existing workspace behavior; public instructions, MCP
contracts and screenshots do not change.

The profile trace showed an edge-adjacent drag auto-scrolling its target away.
Three isolated runs did not reproduce it. A temporary 30-animation-frame
finger hold reproduced the same overshoot without retries
(`/tmp/kandev-run.e2e.xQtG1AV8.log`); centering the source before sampling drag
coordinates passed that controlled case (`/tmp/kandev-run.e2e.V3b6Mtap.log`).
The probe was removed. The permanent fixture still verifies real touch input,
44px handles, saved/reopened order and scrolling outside the handle. The
oversized-message assertion now matches the exact assistant response instead
of also matching the scripted prompt. No product drag behavior changed.

Send Now passed three fresh isolated runs with retries disabled
(`/tmp/kandev-run.e2e.TYeysNBR.log`). Its cause is not established. The exact five-test predecessor sequence also
passed (`/tmp/kandev-run.e2e.1CeQ4bCi.log`). With affinity restricted to four
CPUs, that sequence and Settings workspace creation passed without retries
(`/tmp/kandev-run.e2e.6WaKHfPI.log`, six cases). No speculative queue change
was made.

On a fresh managed build, both changed phone cases passed without retries
(`/tmp/kandev-run.e2e.pMSp2t3M.log`). Focused unit tests, full typecheck, i18n,
changed-file ESLint/Prettier and docs/spec validation passed. The preceding
CI run completed successfully, but its full artifact audit and retry summary
retain the four historical retry signals; fresh-head CI remains required.


### Runtime-update request observation, 2026-10-29

The `83c1ff0b3` browser artifact audit exposed a mobile runtime-update fixture
asserting its POST count synchronously after a tap. A temporary 250ms request-
admission delay reproduced the exact expected-one/actual-zero failure with
retries disabled (`/tmp/kandev-run.e2e.OxXsifV8.log`). Waiting for the existing
fixture count passed that same probe (`/tmp/kandev-run.e2e.Ja3ceBNl.log`).
The probe was removed; the permanent change is one `expect.poll` assertion.
The count remains exactly one and the target-version assertion is unchanged.
All six cases in the phone runtime-update spec then passed without retries
(`/tmp/kandev-run.e2e.mKYftEsW.log`), as did changed-file ESLint and Prettier.
No production, timeout, public documentation or screenshot change is needed.

The same audit contained a folder-picker-open timeout in the Local scratch
source flow. Three fresh isolated runs passed (`/tmp/kandev-run.e2e.VBMRWuQ4.log`),
then all four source-flow cases passed in predecessor order with four-CPU
affinity (`/tmp/kandev-run.e2e.OVYtLlFh.log`), all with retries disabled. No
root cause was established and no speculative picker change was made.


### Threads question scroll-owner fixture, 2026-10-29

The complete `83c1ff0b3` CI run passed 90 checks with 11 skips, but its 118
browser reports and retry summary retained three first-attempt failures. The
runtime-update observation and unreproduced folder-picker case are recorded
above. The third failure tried to swipe an offscreen inner question after its
footer had become the declared scroll owner. A temporary footer-at-bottom
probe reproduced the same negative hit-area height without retries
(`/tmp/kandev-run.e2e.uHd3aLh1.log`). Targeting the declared footer owner passed
the identical probe (`/tmp/kandev-run.e2e.sDK56DA6.log`). The probe was removed.

The helper now derives gesture coordinates from the active scroll owner. It
retains native wheel/touch input, settling, clipping and target-hit assertions.
Four normal phone/coarse-pointer cases passed without retries
(`/tmp/kandev-run.e2e.3S6WCAi7.log`); the desktop Columns inner-owner case also
passed (`/tmp/kandev-run.e2e.0yFT9cvu.log`). The desktop Grid footer wheel
case passed (`/tmp/kandev-run.e2e.QoSuw02p.log`). Changed-file ESLint and Prettier
passed. This is a test helper correction; production behavior, public docs
and screenshots are unchanged. Fresh published-head CI is required.
