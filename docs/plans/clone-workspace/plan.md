---
created: 2026-10-07
status: implemented
requirements:
  - REQ-WORKSPACES-CLONE-001
  - REQ-WORKSPACES-CLONE-002
  - REQ-WORKSPACES-CLONE-003
system_design:
  - ../../specs/workspaces/system-design/clone-workspace.md
legacy_specs: []
---

# Implementation plan: Clone workspace setup

## Overview

Create an independent ordinary workspace using the source's repository/workflow
configuration and GitHub setup. The user chose full setup over GitHub-only
copying. Deliver graph storage first, then atomic authenticated creation, then
the desktop/phone flow, then browser proof and public instructions. Execute
sequentially in the primary session after the design-package handoff.

## Scope

In scope: general workspace settings, live repositories, inline/named scripts,
branch policies, repository sets, visible user-managed workflows and steps,
GitHub automation connection, scope, queries/default markers, and action presets.

Out of scope: runtime/history, Office/Improve Kandev managed state, general
secrets/bindings, other integration credentials, personal OAuth, watchers,
coordinators, automations, workflow-sync subscriptions, and physical checkout
copying. The form must state relevant exclusions before submission.

## Technical approach

- `task/service`: Add clone admission and request handling. Reuse caller
  identity/placement/default eligibility; keep successful creation publication
  after the coordinated commit. Preserve ordinary `CreateWorkspace` behavior.
- `task/repository/sqlite` and `workflow/repository`: Add explicit transaction
  participants for copying the graph. Reuse `workspace_bootstrap.go` insert
  conventions, typed step remappers in `workflow/models/models.go`, and existing
  serializer/SQL binding helpers. Validate unknown references rather than
  retaining source IDs. No new schema or migrations.
- `github` and `secrets`: Supply narrow clone transaction participants modeled
  on `github/copy.go` and `service_connections.go`. Preserve nil/empty values,
  registration+installation pair, independent PAT ciphertext, and excluded
  personal identity. Existing bootstrap/settings copy callers retain their
  contracts; do not globally replace their failure semantics.
- `backendapp`: Compose the stores on their shared writer; own one native
  transaction. Every clone participant uses its supplied handle, avoiding
  nested writer checkout. Add integration tests with full stores, rollback
  injection, routing, and persistence. See the design's connection compatibility
  matrix for supported sources and explicit unsupported-source behavior.
- `task/handlers`: Add only `POST /api/v1/workspaces/:id/clone`, name request,
  access-aware DTO, 201 result, and sanitized error mapping. Existing creation
  events update other clients; no new WS action or MCP surface.
- Web settings: Add a card Clone action above the existing overlay link, a
  shared submission hook, desktop Dialog and phone Drawer. Encode source IDs,
  merge current store state via `mapWorkspaceItem`, deduplicate events/response,
  and navigate to the new settings overview without setting `activeId`.

## ASCII UI preview

Structural requirements: source context precedes editable name, copy summary
and exclusions precede submission, card navigation and clone activation have
separate hit regions, and the draft is shared across viewport compositions.
Copy below is illustrative and must be localized. AC references:
`AC-WORKSPACES-CLONE-003.1` through `.5`.

```text
UI-01 desktop | Settings > Workspaces | eligible source
+------------------------------------------------------------------+
| Team tools  [Active]  | Repositories | Workflows | [copy] | >      |
+------------------------------------------------------------------+
              +---------------------------------------------+
              | Clone workspace                         [X] |
              | From: Team tools                            |
              | Name [Team tools (copy)                  ]  |
              | Copies settings, repositories, workflows,   |
              | and GitHub connection/query configuration.  |
              | No tasks or sessions. Personal sign-in,     |
              | other integrations, and secrets need setup. |
              |                                             |
              |                       [Cancel] [Clone]      |
              +---------------------------------------------+

UI-02 phone | Settings > Workspaces | clone drawer
+-----------------------------------+
| Team tools [Active]   [copy]   > |
| Repositories | Workflows          |
+-----------------------------------+
|          page behind drawer       |
| +-------------------------------+ |
| | ---                           | |
| | Clone workspace               | |
| | From: Team tools              | |
| | Name [Team tools (copy)     ]  | |
| | Copy summary and exclusions   | |
| |                               | |
| | [Cancel]          [Clone]      | |
| +-------------------------------+ |
+-----------------------------------+

UI-03 shared form | pending/error/invalid states
| Name [preserved draft                 ] |
| Error: Could not clone. Try again.      |  <- failure only
| [Cancel] [Cloning...]                   |  <- pending, no repeat submit
| [Cancel] [Clone (disabled)]             |  <- blank name
```

Phone uses an inset bottom Drawer, standard 44px touch controls, safe-area
footer clearance, and one internal scroll owner for overflow at `dvh` bounds.
Desktop uses standard 28px fine-pointer controls. Cancel/Escape return focus;
pending submission keeps one form mounted. An uncertain network outcome uses
check-list-before-retry copy rather than the definite-failure wording above.
Nearest geometry exemplar: `components/task/mobile/mobile-picker-sheet.tsx`.

## Tests

| Acceptance criteria | Planned executable evidence |
| --- | --- |
| 001.1-.3, 001.5-.6, 002.4 | `workspace_clone_test.go`: `TestWorkspaceCloneGraph`, plus workflow remapping/invalid reference cases and bootstrap fallback |
| 001.4, 002.3, 002.5 | `github/workspace_clone_test.go`: `TestWorkspaceCloneGitHubSources` for every matrix row, including missing/inactive connection |
| 001.5, 001.7, 002.1-.3 | `backendapp/workspace_clone_test.go`: `TestWorkspaceCloneAtomicCreation`, authority, restart/readback, edit/delete isolation, full failure injection |
| 002.1-.2, 002.4-.5 | `handlers/workspace_clone_test.go`: `TestWorkspaceCloneHTTP`, inaccessible source, bad name, invalid references, managed source and sanitized failures |
| 003.1-.4 | `use-workspace-clone.test.ts`: eligibility, current-state merge/deduplication, synchronous double-submit guard, preserved draft/error, navigation and active-ID preservation |
| 003.2-.3 | `app/actions/workspaces.test.ts`: clone URL encoding, exact body, error propagation, no automatic creation retry |

All shortened IDs above use the `AC-WORKSPACES-CLONE-` prefix. Tests must fail
for the promised behavior before production changes; missing fixtures/selectors
alone are not behavioral RED. PostgreSQL cases use the existing
`KANDEV_TEST_POSTGRES_DSN` opt-in; report skips explicitly and run when the
configured test database is available. Native SQL uses dialect/binding helpers.

## E2E tests

- `e2e/tests/settings/workspace-clone.spec.ts`, project `chromium`: seed two
  source workflows with references, repository/set/policy/named script, source
  task and GitHub scope plus independent PR/issue defaults. Clone via the card,
  inspect copied settings, enter the target dashboard and switch kinds, reload,
  edit the clone, and prove source preservation and empty task history.
  Covers 001.1-.7 and 003.1-.4.
- `e2e/tests/settings/mobile-workspace-clone.spec.ts`, project `mobile-chrome`:
  complete the same primary flow from the phone card action and drawer;
  test cancel/focus return, blank name, delayed submission/double tap, failure
  retry, long summary/translation overflow, and draft retention on resize.
  Covers 003.1-.5 and copied-query outcome in 001.7.
- Full-store backend tests own token encryption, every rollback participant,
  organization reach and unsupported-source authorization. Browser assertions
  inspect user-facing outcome without exposing tokens or replacing the clone API.

## Work orders

- [x] [Task 01: Copy the workspace configuration graph](task-01-configuration-graph.md)
- [x] [Task 02: Create an authorized atomic clone](task-02-atomic-clone-api.md)
- [x] [Task 03: Add desktop and phone clone controls](task-03-responsive-clone-flow.md)
- [x] [Task 04: Prove browser behavior and document cloning](task-04-browser-proof-and-docs.md)

## Verification results

Design validation on 2026-10-07:

- `python3 scripts/list-docs.py validate`: passed (362 decisions, 1438 specifications).
- `python3 scripts/lint-spec-files.test.py`: passed (36 tests).
- `python3 scripts/lint-spec-files.py --all`: passed.
- PR-documentation `validateCoverage` preflight: actual document-only changes
  exempt; adding the planned runtime path to the input yields `covered`, with
  all four work orders accepted and zero errors. This tests packet readiness,
  not an existing runtime implementation.
- `git diff --check -- docs/specs docs/plans/clone-workspace`: passed;
  each untracked file also passes `git diff --no-index --check /dev/null <file>`.
  `git status --short` confirms the seven new documents and uncommitted package.

Implementation completed on 2026-10-08 (Europe/Lisbon). All four work orders
are complete. Targeted backend tests, 24 frontend tests, scoped lint/typecheck,
localization, SQL guard, race storeconformance and public-document validators
passed. Final production-build browser runs passed: desktop 2/2 (15.9s),
phone 2/2 (16.9s). PostgreSQL clone coverage is present but skipped locally
because `KANDEV_TEST_POSTGRES_DSN` is unset. Exact browser commands, resource
limits and document-validation evidence are recorded in Task 04.

## Risks

- Existing GitHub copy helpers independently commit writes; calling them from
  inside another transaction can deadlock the single SQLite writer and break
  rollback. New participants must share the actual supplied transaction.
- Step remappers intentionally retain unknown IDs. Validation must reject source
  task/session or excluded-step references rather than creating cross-workspace links.
- Field-by-field copying can omit live behavior. Test nondefault values for
  every supported scalar and reference, including completion/signal/cancel gates.
- App registrations remain shared, while PATs are copied. Personal OAuth and
  other provider connections remain excluded and must be explained in the UI.
- An ambiguous response can mean a clone committed. Refresh/check the list;
  do not claim exactly-once creation or silently retry the POST.

## Delivery checkpoint

Implementation was authorized by the later explicit request. Requirements are
`active`, the design is `current`, and this package is implemented. The
implementation checkpoint left the changes uncommitted; the subsequent user
continuation proceeds with a normal hooked commit. No delegation was used.

### Workspace actions placement refinement

The user requested an action attached to the workspace identity and another
entry inside the workspace page. Use one shared ellipsis menu beside the name
and Active badge in list cards and every workspace settings header. The labelled
Clone workspace item closes the menu before opening the existing form; cancel
returns focus to the persistent ellipsis trigger. The list's whole-card
navigation remains the main tap destination. Reuse the shared Radix inset phone
menu and clone Drawer, with 44px targets, bounded scroll and safe areas. The
agent-profile row actions are the nearest shipped secondary-action exemplar.
The hook and form move to shared workspace hook/component owners, without
changing clone persistence or authorization. Clone reads saved configuration.

```text
Desktop list: | Name [Active] [···] | Resources          | > |
Desktop page: | Folder | [Workspace v] [Active] [···]       |
Phone page:   | Folder | [Workspace v] [Active] [···] |
                         -> inset actions menu
                            [copy] Clone workspace
                         -> existing clone dialog/sheet
```

Verification adds wide-card action proximity, keyboard activation, menu-to-form
focus return, phone menu target sizes, header containment and real cloning from
the viewed workspace on a settings tab in both projects. Existing clone flow
and retry coverage remains in scope.

Implemented and verified: 33 targeted unit tests, scoped lint, TypeScript and
complete localization gates passed. Fresh managed desktop 3/3 and phone 3/3
browser tests passed, including wide-card proximity and page-header cloning.
Task 04 records exact results.

### PR review admission coverage

Task 02 adds 13 rejection cases and a positive control for all four workspace
defaults. The targeted service clone tests passed with the race detector; an
admission-bypass overlay failed all 13 cases, proving the regression boundary.
Production contracts and rendered UI are unchanged. Task 02 records the exact
command, results and non-code review dispositions.


The PostgreSQL CI clone fixture failed on its pre-storage timestamp version.
A local PostgreSQL 16 reproduction failed identically; reloading the source
through the repository before cloning fixed the fixture. All eight coordinator
clone tests then passed three race-enabled repetitions with PostgreSQL enabled.
The production stale-source guard remains the owning concurrency contract.

Frontend CI also exposed three coordinator-route fixture failures after the
shared settings header acquired clone actions. Local reproduction confirmed
the missing store API in the route fixture's state-provider mock. Isolating
the clone interaction at that routing test boundary passed 82 tests across
nine affected suites. Task 03 records the exact command and fixture repair.


PR E2E remediation adjusts two pre-existing browser tests: completed-tools
continuity now verifies the completed result in both viewers rather than
requiring a transient retry countdown to survive navigation; mobile PR feedback
waits for the finite drawer animation before tapping Close. Production code,
assertion timeouts and exact continuation/side-effect checks remain unchanged.
Task 04 records the original CI failure, desktop RED/GREEN and shard replay.


The same shard replay exposed a profile-label hydration race. Desktop/phone
profile-discovery tests now await the fixture's resolved model label before
capturing the baseline; they retain selection-preservation and persisted-ID
checks. Task 04 owns these test-only PR remediation changes and their evidence.


Task 04's original E2E failures pass in a full shard replay with retries
disabled. The replay exposed two additional timing assertions, repaired with
existing animation/model-label readiness checks; all 40 focused desktop/phone
repetitions pass. Final remote CI is still pending.
