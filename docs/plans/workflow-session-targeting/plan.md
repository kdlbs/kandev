---
created: 2026-09-09
status: complete
requirements:
  - REQ-TASKS-WORKFLOW-PROFILE-SESSIONS-001
  - REQ-TASKS-WORKFLOW-PROFILE-SESSIONS-002
system_design:
  - ../../specs/tasks/system-design/workflow-profile-session-lifecycle.md
legacy_specs: []
---

# Implementation Plan: Workflow Session Targeting

## Overview

Allow Review to return to the task's initial agent or an earlier step's
conversation. Keep **When this step starts** and **When this step ends** as
independent controls. Keep lifecycle navigation visible above the recipient list.

The task system owns this vertical feature because step routing selects the
session that owns task execution. Deliver two complete slices in order:

1. Tasks 01-04 deliver initial targeting, lifecycle navigation at the top, and
   desktop/mobile proof of Sol → Luna → Sol. This is a working product milestone.
2. Tasks 05-08 add earlier-step targets, durable step bindings, author repair
   flows, and desktop/mobile proof while retaining the first slice's behavior.

Both slices are complete. The implementation includes the initial-session and
source-step targeting paths, their desktop and mobile controls, and the
regression evidence listed below.

## Scope

### In scope

- Explicit initial and earlier direct-profile step targets.
- Exact recipient identity, same-profile fresh sessions, and source end policy.
- Durable task bindings, restart, deletion, retry, and failure behavior.
- API, templates, duplication, portable version 2, and workflow sync.
- Desktop popover and phone drawer with lifecycle navigation above search.
- Translated copy, dirty tracking, read-only inspection, and public docs.

### Out of scope

- Changing default profile-only routing or reviving terminal conversations.
- Arbitrary task-session IDs in workflow definitions or later-step references.
- Indirect target chains, concurrent agents, Office participant ownership.
- Combining explicit targeting with conditional original-session configuration.
- Automatic deletion of parked sessions or changes to provider models.

## Technical approach

### Persist recipients and execution provenance

Task 01 adds nullable `session_target` with the `initial` variant to models,
REST and MCP, persistence, frontend types, and portable conversion. Unsupported
kinds fail validation. Keep `agent_profile_id` for existing profile-only routing.
Create/update/clear and omitted fields need independent cross-surface tests.
Task 05 extends the same union with `step`; no earlier-step choices appear before
their full backend support exists.

Update `internal/mcp/server/config_handlers.go` tool schemas/forwarding and
`internal/mcp/handlers/config_workflow_handlers.go` decoding/controller mapping.
Extend handler event parity and server forwarding tests with target payloads.
Change the one envelope version validator in `workflow/models/export.go`;
import and sync already share it. Target-bearing exports use version 2. Version
1 remains readable and emitted for workflows without explicit targets. The
initial-only reader rejects later `step` variants through closed-kind validation.

For the new column, keep workflow `repository/sqlite.go`, task stub schema and
migrations, defaults/bootstrap inserts, and `config/workflows/loader.go` aligned.
Test built-in null defaults as well as configured template values. The ID-only
`builtin_workflow_step_rows.go` helper is inspection context, not a required edit.

Initial identity comes from the existing original-session marker. Task 01 adds
a write-once `workflow_initial_session` task metadata snapshot, retaining its
logical profile after session deletion, and a bounded `workflow_session_route`
record for prepared/committed entry retries. Task 02 wires both into creation
and routing. Neither depends on the step-binding table. Preserve existing task
metadata with conditional transactional writes and dialect-specific tests.

Task 05 adds `task_workflow_session_bindings` for latest source-step selection
only. Task 06 records a binding even when that source reuses a session. Initial
provenance stays in the task snapshot. Add earlier-position validation, source
reference remapping, deletion cleanup, conformance, and stale-operation guards
in this slice. Audit task `workflow.go` deletion paths for table cleanup.

### Route explicit recipients

Extract typed target resolution from the growing
`event_handlers_workflow.go` into `workflow_session_target.go`. Use it in
preflight and preparation, including no-session starts, engine entry, manual,
queued, and automatic moves. Explicit targets bypass profile equality as a
reason to ignore `new`; an exact active target with `reuse` remains active.
Keep the existing lifecycle retirement, credentials, queue transfer, pending
move, stop-intent, and completion-signal machinery.

### Present workflow-aware choices

In Task 03, offer the initial target and existing profile catalog. In Task 07,
pass edited steps from `workflow-pipeline-editor-panels.tsx` into the selector
and extend `lib/workflows/session-target-options.ts` with earlier-step choices.
Keep a fixed lifecycle entry above search and a single scrolling list beneath.
Offer initial and distinct earlier-step choices before the profile catalog.
Preserve the existing shared settings save/discard coordinator.

Phone composition reuses `MobilePickerSheet` with a fixed-content slot above
the scrolling children for lifecycle/search; do not copy the shell. A shared
path move is optional, with all imports/tests preserved. Keep the existing
`useResponsiveBreakpoint` phone branch. Use `useTouchDrawer` only when a
pointer-driven disclosure actually needs it, not as a second responsive path.
Use dynamic viewport height, keyboard-aware
containment, safe-area padding, focus return, and 44 px touch controls. Desktop
keeps its normal control density. Shared state and handlers drive both surfaces.

## ASCII UI preview

These views record the preview reviewed in conversation. Control order, grouping,
fixed lifecycle navigation, and the phone drawer are required. Box dimensions
and spacing are illustrative; use existing UI primitives and localized labels.

UI-01 and UI-03 show the final second slice. For the first slice, omit the
Implement row entirely; all other structure and UI-02 remain the same. Task 03
contains that initial-only excerpt. Task 07 contains the added source choice
and UI-04 repair state. There are no disabled placeholders for future choices.

### UI-01: Review recipient selector

Entry: open the agent selector in the Review step. Proposed desktop popover:

```text
+--------------------------------------------------+
| Session lifecycle                              > |
| Reuse on start / Park on end                     |
+--------------------------------------------------+
| Search agents or workflow sessions...            |
+--------------------------------------------------+
| WORKFLOW SESSIONS                                |
| (*) Initial agent session                        |
|     Agent selected when the task starts          |
| ( ) Implement / 5.6 Luna                          |
|     Session used by the Implement step           |
|                                                  |
| AGENT PROFILES                                   |
| ( ) No profile override                          |
| ( ) Claude / Fable                               |
| ( ) Codex / 5.6 Sol                              |
| ( ) Codex / 5.6 Luna                              |
| ...                                              |
+--------------------------------------------------+
```

Before: lifecycle navigation is below all profile rows in the scrolling region.
After: lifecycle navigation and search stay above the single scrolling list.
Search filters choices, never lifecycle navigation. With no matches, the list
shows a localized empty result while lifecycle navigation remains available.
Read-only workflows permit inspection and navigation but disable mutations.

### UI-02: Lifecycle settings

Entry: choose Session lifecycle. Both groups belong to the selected Review step.

```text
+--------------------------------------------------+
| < Back                  Session lifecycle        |
+--------------------------------------------------+
| Target: Initial agent session                    |
|                                                  |
| When this step starts:                           |
| (*) Reuse an available session                    |
|     Continue the original conversation.          |
|     If unavailable, start a fresh one.            |
| ( ) Start a new session                           |
|     Initial agent profile, fresh conversation.   |
|                                                  |
| When this step ends:                             |
| ( ) Complete the session                         |
|     This conversation cannot be reused.          |
| (*) Park the session                             |
|     Stop the agent; keep its conversation.       |
+--------------------------------------------------+
```

Keep the target label visible and return Back to UI-01. For an earlier-step
target, change the description to that step's conversation. Explain that Plan
must park the initial conversation for Review to reuse it. The existing shared
Save changes action persists the draft; the selector adds no local save button.

### UI-03: Phone recipient drawer

Entry: tap the Review agent trigger. The inset bottom drawer uses UI-01's
choices and UI-02's lifecycle view with shared state and handlers.

```text
+----------------------------------+
| Review                           |
| [ Initial agent session       v ]|
|                                  |
|  +----------------------------+  |
|  | Agent session              |  |
|  | Session lifecycle        > |  |
|  | Reuse / Complete           |  |
|  +----------------------------+  |
|  | Search...                  |  |
|  +----------------------------+  |
|  | WORKFLOW SESSIONS          |  |
|  | (*) Initial agent session |  |
|  | ( ) Implement / 5.6 Luna   |  |
|  | AGENT PROFILES             |  |
|  | ...                       |  |
|  +----------------------------+  |
|  | Bottom safe-area spacing  |  |
|  +----------------------------+  |
+----------------------------------+
```

The header, lifecycle entry, and search remain outside the scrolling choices.
Constrain the drawer to the dynamic viewport and keep controls reachable with
the keyboard open. Use 44 px phone hit areas. Back/dismiss returns focus
predictably, and the document has no horizontal overflow.

UI-01 and UI-02 map to AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.8,
001.10, 002.2, 002.6, and 002.11 (same AC prefix). UI-03 also covers
001.13. The work package's desktop and mobile session-targeting Playwright
scenarios prove structure and prompt ownership; ASCII alone is not evidence.

### UI-04: Repair an invalid source target (second slice)

Entry: the author reorders or removes Implement, making Review's source invalid.
Desktop inline state:

```text
+--------------------------------------------------+
| Review                                           |
| Agent: [ Implement / 5.6 Luna                  v ]|
| ! Implement must remain an earlier source step.   |
| [Choose another target] [Use no profile override] |
| Undo the source edit to keep this target.         |
+--------------------------------------------------+
| Save changes [disabled until references are valid]|
+--------------------------------------------------+
```

Phone uses the same inline error with stacked actions. Choose another target
opens UI-03; a valid choice clears the error without changing lifecycle values.
Use no profile override clears the target explicitly. Discard restores the
saved source and dependent references. Server-side conflicts retain unsaved
drafts and identify dependent steps. Repair/save dependents first when the
shared coordinator cannot safely order all edits. Sync errors direct authors
to the source file. Covers AC-TASKS-WORKFLOW-PROFILE-SESSIONS-002.9 and 002.11.

## Tests

The following evidence was run after implementation:

| Acceptance | Evidence |
| --- | --- |
| 002.1, 002.8, 002.9, 002.12 | session target model/controller/service coverage: target shapes, partial updates, authorization, and invalid source edits |
| 002.8 | task/workflow repository and orchestrator coverage: round trips, immutable initial snapshot, prepared/committed routing, and retry behavior |
| 002.8, 002.9 | `task/repository/sqlite/workflow_session_bindings_test.go`: source binding persistence, deletion cleanup, replay, and stale-operation rejection |
| 002.8, 002.9 | `mcp/server/config_handlers_test.go` and `mcp/handlers/config_workflow_handlers_test.go`: target schema, forwarding, event parity, omission, and null |
| 002.8, 002.9 | `workflow/models/export_test.go` and workflow service tests: version 1/2, duplication, import/remap, and source/profile validation |
| 002.1, 002.3, 002.5–002.7 | orchestrator target tests and `workflow-session-targeting.spec.ts`: exact original, same-profile new, missing/terminal target, and current-target no-op |
| 002.4–002.7, 002.10 | source-step runtime E2E and orchestrator tests: exact source step, re-entry, profile changes, and unrelated same-profile sessions |
| 001.6, 001.11, 002.7–002.9 | orchestrator race tests plus desktop/mobile E2E: entry identity, retries, preflight, promotion, and lifecycle controls |
| 001.8–001.10, 001.12–001.13, 002.2, 002.9, 002.11–002.12 | selector, validation, duplication, and dirty-state tests: labels, filtering, pinned lifecycle, save/discard, read-only, and repair rules |

All AC numbers above use `AC-TASKS-WORKFLOW-PROFILE-SESSIONS-` as their prefix.
New logic uses TDD. Keep tests in focused new files where existing suites exceed
file-size limits. PostgreSQL tests require an isolated test DSN; a skip is not
PostgreSQL evidence. Update required-store/conformance coverage for the task
metadata/column first and binding table second, including previous-stable upgrade behavior.

Existing lifecycle criteria are regression obligations, not omitted scope:

| Acceptance | Owner and existing evidence |
| --- | --- |
| 001.1 | Task 02: `TestSwitchSessionForStep_ReusesNonterminalSession` in `event_handlers_workflow_profile_test.go` |
| 001.2 | Task 02: `TestSwitchSessionForStep_NewOnStartParkOnEndRoundTrip` in `event_handlers_workflow_profile_session_policy_test.go` |
| 001.3, 001.4 | Task 02: `TestSwitchSessionForStep_UsesDestinationStartAndSourceEndIndependently` in the same policy test file; assert both source outcomes |
| 001.7 | Task 01: `TestWorkflowStepProfileSessionPoliciesRoundTripAndDefaults` plus existing model/service policy tests |

Task 06 reruns the same routing regressions after extending target kinds. Only
001.1 and 001.5 were scoped for the new routing distinction; 001.2 was unchanged.

## E2E tests

- `e2e/tests/workflow/workflow-session-targeting.spec.ts`, project `chromium`:
  author Plan/Implement/Review, select the task's initial profile at creation,
  prove Review prompt ownership and session IDs for reuse and fresh modes.
  Task 04 owns the initial-only scenario; it cannot depend on later step support.
- `e2e/tests/workflow/mobile-workflow-session-targeting.spec.ts`, project
  `mobile-chrome`: author the same choice through the drawer, configure both
  lifecycle groups before scrolling a large profile list, save/reload, run to
  Review, and inspect the receiving conversation. Cover filtering with no
  results, touch sizes, focus return, viewport containment, and horizontal overflow.
 `mobile-workflow-step-session-targeting.spec.ts` for equal-profile source
 distinction, skip/re-entry behavior, invalid reference repair, and remapping.
 It reruns the first slice's two specs to prove the extension preserves them.
- The combined desktop spec covers initial reuse/fresh routing, source-step
  routing, and invalid-source repair. The mobile spec covers the responsive
  drawer, fixed lifecycle controls, save behavior, and viewport containment.
  The desktop suite passed all four scenarios; the mobile suite passed its
  lifecycle scenario.
- Extend the existing workflow agent-switch scenarios only where their shared
  helpers or selectors change; keep legacy profile-only assertions.

## Work orders

- [x] [Task 01: Persist initial recipient contracts](task-01-persist-recipient-contracts.md)
- [x] [Task 02: Route initial workflow recipients](task-02-route-explicit-recipients.md)
- [x] [Task 03: Expose the initial session choice](task-03-expose-session-choices.md)
- [x] [Task 04: Prove initial session targeting](task-04-prove-session-targeting.md)
- [x] [Task 05: Persist source-step targets](task-05-persist-source-step-targets.md)
- [x] [Task 06: Route source-step recipients](task-06-route-source-step-recipients.md)
- [x] [Task 07: Expose source-step choices](task-07-expose-source-step-choices.md)
- [x] [Task 08: Prove source-step targeting](task-08-prove-source-step-targeting.md)

Execution order: 01 → 02 → 03 → 04 (initial milestone), then 05 → 06 → 07 → 08
(source-step milestone). No delegated implementation is authorized.
The exact commands belong to each work order. Install workspace dependencies
once before package commands if the worktree has no `apps/node_modules`.

## Verification results

Design and implementation validation on 2026-09-09:

- `rtk python3 scripts/lint-spec-files.test.py`: passed, 30 tests.
- `rtk python3 scripts/lint-spec-files.py --all`: passed.
- `rtk git diff --check`: passed.

Backend workflow packages: 804 tests passed. MCP schema and parity tests: 16
passed. Task repository race tests passed; store conformance passed 290 tests.
Orchestrator target and lifecycle race tests passed 180 tests; workflow engine
and step-entry tests passed 230 tests. Backend build and E2E plugin packaging
passed. Frontend focused tests passed 36 tests; typecheck and full web ESLint
passed with no warnings. All translation gates passed. Desktop workflow
targeting E2E passed 4 tests; mobile workflow targeting E2E passed 1 test.
Public documentation validation, full spec lint, and whitespace checks passed.

The park-by-default follow-up aligned the backend and frontend normalizers,
both SQL schema owners, runtime nil handling, public docs, and the desktop and
phone previews. Final checks passed 69 frontend regressions, 164 focused
orchestrator regressions, 11 race regressions, SQL guard, backend lint,
frontend typecheck and targeted lint, i18n ratchet, spec lint, and public-doc
validation. A full orchestrator attempt reached 1,385 passing tests before the
package's fixed 10-minute timeout; the task-defined focused suite completed.

The PostgreSQL-specific commands were not run because this environment did not
provide an isolated KANDEV_TEST_POSTGRES_DSN. The backend boot test command was
run but reported no matching tests in the current checkout.

## Risks

- Existing same-profile early returns can bypass explicit recipient intent.
- Initial provenance must survive primary changes without acquiring new markers.
- A step binding written before a failed promotion could misroute later work.
- Cached or repeated step-entry events can create duplicate fresh sessions.
- Step references require coordinated remapping and validation on all write paths.
- Version 2 exports require upgraded readers; version 1 routing is preserved.
- `reuse` cannot preserve a conversation that its source step completed.
- Conditional session options remain exclusive; their wording must not imply routing.
