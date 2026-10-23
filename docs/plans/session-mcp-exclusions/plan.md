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

After parent merge, reconcile the actual merge and retarget to main without
replaying parent commits. No merge is authorized. Preserve other edits and
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
