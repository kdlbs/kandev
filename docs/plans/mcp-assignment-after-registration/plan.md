---
created: 2026-10-22
status: in_progress
requirements:
  - REQ-AGENTS-MCP-ASSIGNMENT-001
  - REQ-AGENTS-MCP-ASSIGNMENT-002
  - REQ-AGENTS-MCP-ASSIGNMENT-003
  - REQ-AGENTS-MCP-ASSIGNMENT-004
system_design:
  - ../../specs/agents/system-design/workspace-mcp-assignment.md
legacy_specs: []
---

# Implementation plan: Assignment after MCP registration

## Overview

After registering a definition, show Saved with Done and optional Use in.
Use in opens an existing owner editor without changing a selection. The owner
retains save behavior and runtime timing. Build conditional selection safety,
then saved-only completion, then the three owner handoffs and recovery.

Implementation was explicitly authorized on 2026-10-23. Work orders 01 and 02 are
complete. Work order 03 is pending and requires the committed
management navigation prerequisite. Parent PR #3293 remains open at the verified
193de56ba2597b335a97b1e5ab01d09340f22647 tip.

## Baseline and coordination

- Task: `5a8cb5cd-22ee-42e5-bd6d-e2179e899c8c`.
- Workspace: `/home/zeval/.kandev/tasks/clarify-mcp-assignme_ymv86j32/pr3293-followup-source-20261022`.
- Own branch: `feature/clarify-mcp-assignme-mhh`.
- Fetched parent: `feature/plan-workspace-mcp-c-dcc` at
  `193de56ba2597b335a97b1e5ab01d09340f22647`, verified 2026-10-22.
- Separate stacked PR targets the parent branch while #3293 is open. Reconcile
  actual merge and retarget to main afterward without replaying parent changes.
  No merge is authorized. Do not mutate the parent or sibling workspace.

Read the current parent head and specifications again before implementation.
The parent's testing guide is historical context; its old ports and counts
are not current evidence. Never use ports 9998, 48593, or 48595 for this work.

Management sibling `000d477e-f792-4c9d-8503-50ed73bb9646` owns the missing
global-profile workspace picker and authorized owner navigation. Its full saved
plan was read; no implemented destination was available at planning time.
Work order 03 depends on its committed `task-02-management-navigation.md` result.
Reconcile the exact owner-link interface before that work order. Do not add
another management hub or copy uncommitted files. Work orders 01 and 02 do not
need that sibling. Inherited exclusions and built-in management remain excluded.

Exclusions sibling `15641d1a-d7ba-4216-aefd-5a4c36a88d73` proposes the session
`selection_revision`/`expected_revision` pair and optional
`excluded_definition_ids`. Its draft was read for coordination. Assignment uses
that one session precondition and leaves exclusions omitted/preserved. Existing
same-set application retries remain supported. Exclusions storage and controls
remain sibling-owned. Assignment work order 01 owns the shared conditional-write
primitive. Exclusions work order 01 depends on that committed primitive and
extends the same session transaction. Assignment has no exclusions prerequisite.
Record the actual operation/lock/type contract in work order 01's results.
This replaces the earlier whichever-lands-first option. The user authorized this implementation on 2026-10-23; sibling implementation
remains separately owned.

## Scope

### In scope

- Custom and marketplace registration completion with saved-only exit.
- Optional repository, workspace-contextual profile, and existing-session handoff.
- Same-definition recovery after assignment failure or lost acknowledgment.
- Conditional selection replacement, eligibility, and context race protection.
- Desktop/phone composition, localization, focused tests, and public guide updates.

### Out of scope

- Bulk or task-level assignment, owner creation, automatic selection or enabling.
- Used in projection, global-profile management implementation, or duplicate hubs.
- Session inherited exclusions, built-in management, probes, new runtime timing.

## Technical approach

The [design](../../specs/agents/system-design/workspace-mcp-assignment.md) defines
the full boundary. `mcp_selection_handlers.go`, `SelectionService`, and
`settings/store/mcp_selections.go` add a direct-membership precondition for
non-session owners and use the shared session revision precondition for sessions.
The web editor sends its accepted baseline/revision, not its draft or inherited
union, and addition-only writes omit exclusions. Conflict recovery uses explicit
reload/review and preserves creation.
Atomic writes return their own committed result. No migration is proposed.

`MCPSettings` and `MCPMarketplaceView` preserve the create/install result.
`useMCPWorkspaceSettings` guards workspace generations and reconciles rows by ID.
New focused completion and owner-choice components stay under
`components/settings/workspaces/`; data/controller hooks stay under
`hooks/domains/workspace/`. Their names below are proposed, not existing APIs.

Owner destination builders come from the management sibling. Reuse
`RepositoryMCPSelectionSection`, `ProfileMCPSelectionCard`, session Customize,
and `linkToTask`. Consume the management sibling's typed owner routes. Assignment
owns the reserved focus-only `mcpServerId` hint described in the system design.
Settings can reveal its row; sessions wait until the user opens Customize. Do
not stage a checkbox, change search, PUT selections, start/ensure an agent, or
replay a consumed hint after reload. Existing Save changes or explicit session selection commits.

### Provider compatibility

| Path | Intended behavior | Evidence | Unsupported fallback |
| --- | --- | --- | --- |
| Registration and owner discovery, every transport | Storage and authorized reads only | Request spies and catalog/service tests | Readable discovery failure; Done remains available |
| Repository/profile selections | Future normal attachment uses saved additive set | Existing selection tests plus handoff E2E | Definition remains saved; failed selection is local |
| ACP with verified safe same-session reconnect | Existing idle apply pipeline only | Existing provider timing tests; mock E2E checks UI state | Existing deferred or failed state, not invented support |
| Active turn | Existing pending-idle state | Focused session handoff E2E | No mutation of delivered turn |
| Passthrough, project-file consumers, unsupported ACP reconnect | Saved selection waits for normal next start | Deferred-state fixture | No forced restart or package preparation by chooser |

Mock E2E proves UI and persistence only. No new claim about real provider
connectivity or npm materialization is part of this package.

## UI previews

These structured previews replace ASCII drawings in accordance with the task's
Kandev rich-output instruction. Control order and save ownership are required;
example names and spacing are illustrative. Use existing tokens and primitives.

### UI-01: Saved completion, both viewports

Entry: acknowledged custom or marketplace registration, Configured view.

| Order | Content/control | Behavior |
| --- | --- | --- |
| 1 | Saved to Acme / Atlas docs | Announces registration only |
| 2 | Choose where to use this server, or finish now | No assignment required |
| 3 | Done; Use in | Done completes; Use in opens the chooser |
| 4 | Existing catalog cards | Saved definition stays visible; each card offers Use in |

Phone uses a single column and wrapping actions. No setup dialog remains open.
Maps to AC-001.1 through .4 and AC-004.1 through .4 (full prefix
`AC-AGENTS-MCP-ASSIGNMENT`).

### UI-02: Owner choice and consequence review

| Region | Desktop bounded dialog | Phone inset Drawer |
| --- | --- | --- |
| Fixed header | Use Atlas docs in / Acme / Close | Same identity and contained Close |
| Scroll body, first view | Repository, Agent profile, Existing task agent session | Three full-width rows |
| Scroll body, next view | Search; authorized named results; task then session if needed | Replace content in the same Drawer; Back stays visible |
| Review | Named owner and consequence text | Wrap names and consequence above actions |
| Footer | Back; Open MCP settings | Safe-area footer, at least 44px targets |

Open navigates to the exact existing owner. Repository/profile reveal MCP and
can highlight Atlas docs once without changing selection or search. Session
navigation opens only the conversation; the hint waits until the user opens
Customize. Repository/profile show shared Save changes only after a user edits.
Session Customize explains autosave before its checkbox.
Maps to AC-002.1 through .6, AC-003.2/.4, and AC-004.1 through .4.

### UI-03: Partial success and unavailable targets

| State | Required presentation and recovery |
| --- | --- |
| Loading owners | Keep Saved and Done visible; status text; no stale choices |
| Empty/search empty | Explain no eligible targets or no matches; Back/Done |
| Owner unavailable | Readable reason; choose another owner; no hidden fallback |
| Selection conflict in owner editor | Choices changed; Reload choices; retain local draft until explicit reconciliation |
| Saved selection, runtime failure | Existing Customize failed notice and retry; no registration retry |

The catalog's Use in action remains the recovery entry after closing or reload.
Phone has one scroll owner and never places a recovery action behind an overlay.
Maps to AC-001.2/.4, AC-002.5, AC-003.1/.3/.4, and AC-004.2/.4.

## Tests

Names here are planned tests to create with TDD, not claimed existing evidence.

| Criteria | Test file and scenario |
| --- | --- |
| AC-003.1/.2/.3 | `mcp_selection_concurrency_test.go`: `TestMCPSelectionConditionalReplace`, `TestMCPSelectionCommittedResponse`, and `TestMCPSelectionRetryPreservesAcceptedState` |
| AC-002.3/.5, AC-003.4 | `mcp_assignment_eligibility_test.go`: unauthorized, wrong workspace, disabled profile, archived/deleted task/session; late request ownership in hook tests |
| AC-003.1/.3 | `postgres_mcp_selection_concurrency_test.go`: independent writers, known-empty set, replacement vs deletion/legacy import, session revision conflicts; after integration, mixed additions/exclusions clients preserve omitted fields |
| AC-001.1/.2/.4, AC-003.4 | `use-mcp-registration-completion.test.ts`: exact saved identity, rejected target lookup, uncertain ACK, context change, no second create |
| AC-002.1 through .4/.6, AC-003.2/.3/.4 | `mcp-assignment-navigation.test.ts`: typed links preserve query/hash; hint parser rejects malformed/duplicate IDs; wrong workspace, removed, disabled and filtered-out rows; delayed inventory and context changes; one-time consumption; no checkbox/search/draft writes or session overlay opening |

Existing selection editor tests must remain covered when changing its API.
Work order 01 includes service, repository, handlers, backend owner validation,
typed client, and hook checks. Do not claim PostgreSQL coverage when its DSN is
absent. Use a disposable database and record that limitation explicitly.

## E2E tests

Create `e2e/tests/settings/mcp-assignment.spec.ts` (`chromium`) and
`mobile-mcp-assignment.spec.ts` (`mobile-chrome`), with helpers under
`e2e/helpers/mcp-assignment.ts`.

| Flow | Criteria | Required proof |
| --- | --- | --- |
| Custom and marketplace saved-only | AC-001.1/.2/.3/.4 | Exactly one definition, reload persistence, zero selection/start/probe requests, edit does not repeat completion |
| Repository and profile handoff | AC-002.1/.2/.3, AC-003.2 | Correct owner, explicit workspace for global profile, no PUT on navigation, Save/discard, other choices preserved |
| Existing session handoff | AC-002.4/.6, AC-003.2/.3 | Exact sibling session, no ensure/start or automatic Customize opening; highlight only after user opens it; explicit checkbox autosave; pending/deferred/failed distinct; transcript retained |
| Focus-only hint | AC-002.6, AC-003.4, AC-004.3 | Existing selected state and search unchanged; invalid/unavailable/filtered rows never broaden inventory; async load and workspace race safe; consumed key removed with other query/hash intact; rerender, reopen and reload do not replay |
| Partial success and races | AC-001.2/.4, AC-002.5, AC-003.1/.3/.4 | Failed/uncertain selection and 409 recovery retain one definition; disable/delete/access change fails safely; stale workspace responses ignored |
| Phone and keyboard | AC-004.1 through .4 | All owner types and saved-only reachable; final result reachable; 44px targets, one scroller, focus return, no horizontal overflow, locale wrapping |

Phone checks include 320px, configured Pixel 5, 767px fine pointer, 768px
boundary, and 900px touch tablet. Measure controls and container bounds. Test
desktop-to-phone-to-desktop while search/choice is set; state must survive.
Use one failing request fixture rather than disabling real services. Never run
overlapping full suites. Commands below are executed sequentially.

## Work orders

- [x] [01: Guard selection replacement](task-01-guard-selection-replacement.md)
- [x] [02: Preserve saved registration completion](task-02-registration-completion.md)
- [ ] [03: Connect optional owner handoffs](task-03-owner-handoff.md)

Order: 01, 02, then management sibling navigation prerequisite, then 03.
No delegation is authorized. Work order 02 is an intermediate saved-only result;
the overall feature is incomplete until all three owners pass work order 03.

## Verification results

Design checks completed on 2026-10-22:

- `python3 scripts/list-docs.py validate`: passed (397 decisions, 1693 specs).
- `python3 scripts/lint-spec-files.test.py`: passed (36 tests).
- `python3 scripts/lint-spec-files.py --all`: passed.
- `git diff --check -- docs/specs docs/plans/mcp-assignment-after-registration`:
  passed. A separate whitespace scan covered all six untracked draft files.
- Repository PR-documentation `validateCoverage` preflight: passed as docs-only
  exempt. A separate package traceability check covers REQ/AC/design references.

Historical design checkpoint: runtime, product unit, and E2E checks were not run. No
production or permanent test files changed. Planned implementation commands
and evidence belong in the corresponding work orders. Work orders 01 and 02 now record implementation evidence; none of the parent guide's historical counts were reused.

Implementation checkpoint 2026-10-23: conditional writes and saved-only
registration are implemented and tested. Six desktop/phone E2E cases passed.
Management plan version `40b577c3-0d95-43b5-9a57-ae8af8ee149b` is unchanged
and still records uncommitted design only. Its committed task 02 navigation
prerequisite is unavailable. Work order 03 remains pending; no optional owner
chooser, owner-route hint, or full-feature completion is claimed. Do not copy
uncommitted sibling code or implement its ownership here.

## Risks

- Management navigation is not implemented in the baseline. Work order 03 must
  consume its reviewed committed result or return an integration blocker.
- Selection writes currently lack conditional browser protection. A client-only
  reread or mutex cannot prove concurrent database correctness.
- Task owners and settings storage have separate service ownership. Retain
  authorization and deletion cleanup; do not claim a new cross-service transaction.
- Existing settings contributor ACK reconciliation must preserve newer local
  drafts. A null/stale hook result cannot count as a successful save.
- Sibling work may touch shared MCP files/locales. Reconcile commits by intent;
  do not overwrite sibling changes or repurpose their requirement IDs.
