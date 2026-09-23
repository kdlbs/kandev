---
created: 2026-09-14
status: blocked
requirements:
  - REQ-TASKS-ATTACH-WORKSPACE-SOURCES-001
  - REQ-TASKS-ATTACH-WORKSPACE-SOURCES-002
  - REQ-TASKS-ATTACH-WORKSPACE-SOURCES-003
  - REQ-TASKS-ATTACH-WORKSPACE-SOURCES-004
system_design:
  - ../../specs/tasks/system-design/attach-workspace-sources.md
  - ../../specs/tasks/system-design/workspace-repository-placement.md
legacy_specs: []
---

# Implementation plan: Workspace repository placement

## Overview

Give Worktree tasks an optional parent-root startup and three explicit repository placements in the existing Add to workspace surface.
The user accepted these behaviors and previews on 2026-09-14, then explicitly requested implementation. The durable layout, nested materialization, preview, and available UI work are implemented; explicit root expansion remains blocked by the required session-continuity dependency.

Tasks own the environment and attachment contract. This package extends the existing [requirements](../../specs/tasks/requirements/attach-workspace-sources.md).
The [placement design](../../specs/tasks/system-design/workspace-repository-placement.md) and [ADR](../../decisions/2026-09-14-explicit-workspace-repository-placement.md) define technical authority.
The final column and path examples are illustrative. The three choices, visible tradeoffs, result tree, and accessible actions are required.

## Evidence and dependency

Issue [#3403](https://github.com/kdlbs/kandev/issues/3403) is confirmed by a source trace.
`WorktreePreparer.Prepare` uses the repository path for one repository, including its reuse return. `prepareMultiRepo` uses the parent path.
`branchMaterializer.finalizeMaterialize` promotes the persisted path and rescans without changing the running agent CWD.
`TestAddBranchToTask_ActiveTurnUsesLegacyMaterializer` deliberately preserves that behavior.
Merged [#1990](https://github.com/kdlbs/kandev/pull/1990) restored this live tool contract after idle rebind broke active calls.
This is a changed product contract for the explicit UI flow, not permission to route the legacy tool through restart.

Open [#3598](https://github.com/kdlbs/kandev/pull/3598) owns workspace-aware native restore and explicit saved-history continuation.
Last verified head: `62851c51fa2d347bbc0d162634c25c541c441d21`, open on 2026-09-15.
Task 04 depends on that contract landing or an explicitly selected compatible base. Refresh its exact head and re-read its design before implementation.
Do not cherry-pick, duplicate its recovery coordinator, or bypass explicit continuation to finish this package.
Tasks 01 through 03 can proceed before that dependency. Package delivery requires all five tasks.

Related issues #3227, #3309, #3073, and #3597 remain separate recovery or credential concerns.
This package does not relax cross-task authorization, infer repository ownership, or introduce generic workspace repair.

## Scope

### In scope

- Creation-time parent-root choice for single-repository Worktree tasks, off by default.
- Repository-only batch placement inside `./kandev/`, directly inside the current root, or below an expanded parent root.
- Durable placement across launch, resume, restart, additional sessions, and cleanup.
- Real nested worktrees, Git staging protection, and repository-aware tracking.
- Existing dialog/drawer integration, visible tradeoffs, localization, and desktop/mobile regression evidence.
- Explicit recovery for incompatible expansion, using the shared session-recovery contract.

### Out of scope

- New placement controls for Local, Docker, SSH, Sprites, or Kubernetes executors.
- Arbitrary destination paths, `.kandev/`, submodules, and relocation of existing repositories.
- Active-turn batch attachment, automatic native-session replacement, or sandbox permission bypass.
- Changing legacy add-branch request semantics or exposing placement selection in its MCP arguments.
- New global settings, executor defaults, or a remembered placement across unrelated tasks.

## Technical approach

1. Add typed task initial-layout, environment effective-layout, and per-slot relative placement fields.
   Preserve canonical physical inventory. Validate legacy paths without cardinality-based rerooting.
2. Carry the creation option through task DTOs and launch requests. Separate agent root from repository setup CWD.
3. Add read-only placement preview and an expected-revision check to the existing source mutation boundary.
   Materialize nested worktrees under owned roots. Rescan without calling promotion or rebind.
4. Reuse shared native-session recovery for explicit root expansion, with all-session idle and compatibility checks, after PR #3598 lands or an explicitly selected compatible base is available.
5. Extend source UI state, placement cards, consequences, and result trees. Finish public docs with the product behavior.

Exact file ownership and commands are in the work orders. New symbols and test files there are marked as proposed.
Implementation runs sequentially. Native subagents were authorized for read-only audits and bounded implementation work during this session; no persistent Kandev subtasks were created.

## ASCII UI preview

Read these views before editing UI. Compare the rendered implementation against them during the work order's existing E2E checks.
Record screenshots and any remaining differences in that work order's Results. Update both preview copies if the accepted design changes.
ASCII spacing is not a pixel requirement. Repository labels, branch labels, and paths are examples of the server-resolved preview.
Use plus/minus text with accessible benefit/cost labels, not color alone. All product copy must use translations.

### UI-01: Create task, Advanced, desktop

Entry: New task dialog, one repository, Worktree executor, Advanced open. AC-002.1-.5 and AC-004.3.
The new control follows existing dependency and priority controls. Existing controls are not removed.

```text
+----------------------------------------------------------------+
| Create task                                                [X] |
| ... existing task, repository, agent and executor controls ...  |
|                                                                |
| v Advanced                                                     |
| Depends on [None v]                  Priority [Normal v]       |
|                                                                |
| [ ] Start in a parent workspace folder                     (i) |
|                                                                |
| Help on hover or keyboard focus:                               |
| Start above the repository so you can add sibling repositories  |
| later without moving the agent's working directory.            |
| Some agents may discover repository instructions and skills     |
| differently with this layout.                                  |
|                                                                |
|                                        [Cancel] [Create task]  |
+----------------------------------------------------------------+
```

### UI-02: Create task, Advanced, phone

Same entry and criteria as UI-01. One-column Advanced controls. Tapping (i) opens an inset help drawer, not a hover tooltip.

```text
+------------------------------------+
| Create task                    [X] |
| ... existing fields ...            |
| v Advanced                         |
| Depends on [None v]                |
| Priority   [Normal v]              |
|                                    |
| [ ] Start in a parent              |
|     workspace folder           (i) |
|------------------------------------|
| [Cancel]            [Create task]  |
+------------------------------------+

        Help drawer after tapping (i)
+------------------------------------+
| Parent workspace folder        [X] |
| Start above the repository to add  |
| siblings later without moving the |
| agent's working directory.        |
|                                    |
| Instruction and skill discovery   |
| can differ between agents.        |
+------------------------------------+
```

Multiple initial repositories replace the switch with: “A parent workspace is already used for multiple repositories.”
Unsupported executors and repositoryless tasks omit this control. Switching away must not submit a hidden enabled value.
Reopening an unrelated creation dialog returns to the default, not a prior task's selection.

### UI-03: Add repositories, desktop, nested selection

Entry: Files > Workspace actions > Add to workspace. Idle Worktree task rooted at `storefront/`. AC-003.1-.5 and AC-004.1-.2.
Keep existing source menus, repository selectors, branch controls, and folder support.
The new placement section applies only to repository-only batches. No radio is selected initially; this view shows an example selection.

```text
+----------------------------------------------------------------+
| Add to workspace                                           [X] |
| Add repositories or folders to this task.                      |
|                                                                |
| [+ Repository v]  [+ Folder]                                   |
|                                                                |
| Repository           Branch                                    |
| [payments-api     v]  [main                 v]              [x] |
|                                                                |
| Where should the repositories go?                              |
| Current workspace: .../my-task/storefront/                      |
|                                                                |
| +------------------------------------------------------------+ |
| | (*) Inside ./kandev/                                        | |
| |     ./kandev/payments-api/                                  | |
| |                                                            | |
| |     + Keeps added repositories grouped together.            | |
| |     + Agent session and running processes stay unchanged.   | |
| |     - Storefront's instructions may apply to them too.      | |
| +------------------------------------------------------------+ |
|                                                                |
| +------------------------------------------------------------+ |
| | ( ) Directly inside the current folder                      | |
| |     ./payments-api/                                         | |
| |                                                            | |
| |     + Short paths beside your existing project folders.     | |
| |     + Agent session and running processes stay unchanged.   | |
| |     - Adds folders among storefront's own files.             | |
| |     - Storefront's instructions may apply to them too.      | |
| +------------------------------------------------------------+ |
|                                                                |
| +------------------------------------------------------------+ |
| | ( ) Expand the workspace root                               | |
| |     ../payments-api/                                        | |
| |                                                            | |
| |     + Repositories sit side by side, outside each other.     | |
| |     + Future repositories use the same parent workspace.    | |
| |     - Agent restarts. Terminals and dev servers stop.        | |
| |     - Native conversation recovery depends on the agent.    | |
| +------------------------------------------------------------+ |
|                                                                |
| Result                                                         |
| storefront/          <- agent stays here                       |
| `-- kandev/                                                    |
|     `-- payments-api/                                          |
|                                                                |
| Added repositories are excluded from the outer repository's     |
| normal Git staging.                                            |
|                                                                |
|                                  [Cancel] [Add repositories]   |
+----------------------------------------------------------------+
```

For direct placement, Result becomes `storefront/payments-api/`, with the agent still at `storefront/`.
The exclusion statement is shown only after preview confirms that protection is supported. Failed protection blocks submission with an explanation.

### UI-04: Add repositories, desktop, expansion selection

Same surface as UI-03. Replace the Result, consequence summary, and primary action. AC-003.3 and AC-004.1-.2/.6.

```text
| Result                                                         |
| my-task/             <- agent starts here                      |
| |-- storefront/                                               |
| `-- payments-api/                                              |
|                                                                |
| Your task, messages and plan remain available.                  |
| If native resume is unavailable, you must explicitly choose     |
| whether to continue in a new agent conversation using saved     |
| history. Private agent context may not carry over.              |
|                                                                |
|                             [Cancel] [Expand and add]          |
```

The preview retains any already nested repositories at their actual paths. Expansion does not rearrange existing files.
For an already parent-rooted task, omit placement cards, show the sibling destination, and keep “Add repositories” without a restart notice.

### UI-05: Add repositories, phone

Entry: phone Files workspace action. Full-height drawer with fixed header/footer and one scroll body. AC-004.4-.5.
The complete consequences remain readable through vertical scrolling. No information depends on hover or horizontal comparison.

```text
+------------------------------------+
| Add to workspace               [X] |
|------------------------------------|
| [+ Repository]  [+ Folder]          |
| payments-api / main            [x] |
|                                    |
| Where should it go?                |
|                                    |
| (*) Inside ./kandev/               |
|     ./kandev/payments-api/          |
|     + Grouped together             |
|     + No session restart           |
|     - Parent instructions may apply|
|                                    |
| ( ) Inside the current folder      |
|     ./payments-api/                |
|     + Short paths, no restart      |
|     - Mixed with project files     |
|     - Parent instructions may apply|
|                                    |
| ( ) Expand workspace root          |
|     ../payments-api/               |
|     + Separate sibling repos       |
|     - Restarts workspace processes |
|     - Session recovery varies      |
|                                    |
| Result and selected-option details |
| ...                                |
|------------------------------------|
| [Cancel]     [Add repositories]     |
+------------------------------------+
```

Reuse the shipped Add sources full-height drawer rather than compressing desktop columns.
Use `100dvh`, safe-area footer clearance, and at least 44px touch targets. Desktop controls retain normal 28px density.
The creation help drawer is inset because it contains a short explanation. Both surfaces restore focus to their actual opener.

### UI-06: Recovery, validation, and submission states

AC-003.5-.6 and AC-004.5-.6. These are inline states of the existing surface, not stacked dialogs.
Phone uses the same state order within its scroll body and fixed footer.

```text
Busy:
  Workspace changes are unavailable while an agent is working.
  Wait for all task sessions to finish their current turns.
  [Cancel]                         [Add repositories: disabled]

No placement selected:
  Choose where to add the repositories.
  [Cancel]                         [Add repositories: disabled]

Collision:
  ./payments-api already exists. No files were changed.
  Choose another placement or update the repository selection.
  [Cancel]                         [Add repositories: disabled]

Changed workspace:
  The workspace changed. Review the updated locations.
  [Cancel]                         [Review updated preview]

Unsupported native resume:
  This agent cannot resume its conversation in the expanded root.
  Continue with recorded messages and the task plan in a new
  agent conversation. Private agent context will not carry over.
  Existing repositories remain in place.
  [Cancel]              [Continue with history and add]

Submitting:
  Adding repositories...            [Add repositories: disabled]
  or: Expanding workspace...        [Expand and add: disabled]

Failure:
  Could not add repositories. <actionable reason>
  Selected repositories, branches and placement remain visible.
  [Cancel]                                      [Try again]
```

Do not claim rollback succeeded when compensation fails. Use the shared recovery-required surface in that case.
Cancel before submission makes no mutation. In-flight dismissal remains disabled under the existing submission behavior.

## Tests

Proposed test names deliberately use `WorkspacePlacement` or `InitialWorkspaceLayout` for focused commands.
Existing active-turn, setup-script, source rollback, and reuse tests remain in their owning suites.

| Criteria | Evidence |
| --- | --- |
| 002.1-.6 | Task creation DTO/store tests; `TestWorktreePreparer_InitialWorkspaceLayout`; `TestWorkspacePlacementReuse` |
| 003.1-.2/.5-.6/.10 | `TestWorkspacePlacementPreview`; `TestWorkspacePlacementNestedBatch`; real SQLite and Git fixtures |
| 003.4/.7 | `TestWorkspacePlacementGitExclusion`; `TestWorkspacePlacementTrackers`; `TestWorkspacePlacementCleanup` |
| 003.3/.8/.9 | `TestWorkspacePlacementExpansion`; existing active-turn legacy materializer regression |
| 004.1-.6 | Creation and placement component tests; desktop/mobile browser flows mapped to UI-01 through UI-06 |

## E2E tests

- New `e2e/tests/task/workspace-repository-placement.spec.ts`, project `chromium`: default and parent-root creation, all placements, actual destinations, Files/Changes, and retry/cancel.
- New `e2e/tests/task/mobile-workspace-repository-placement.spec.ts`, project `mobile-chrome`: same outcomes, help drawer, card selection, scrolling, safe-area actions, and disabled/error states.
- Retain `e2e/tests/task/add-workspace-sources.spec.ts` and `mobile-add-workspace-sources.spec.ts` for mixed/folder compatibility.
- Real harness smoke evidence is required for parent-root instruction discovery and native directory-move compatibility. Mock agents do not prove provider behavior.
  Use isolated tasks and installed representative agents. Record agent/version, initial CWD, before/after native ID, second-repository access, and discovery results.
  Treat unavailable harness credentials as an explicit unverified release risk, never a passing compatibility claim.

## Work orders

- [x] [Task 01: Persist layout and repository placement](task-01-durable-layout.md)
- [x] [Task 02: Offer parent-root task creation](task-02-create-layout-ui.md)
- [x] [Task 03: Materialize nested repositories safely](task-03-nested-materialization.md)
- [ ] [Task 04: Integrate explicit workspace expansion](task-04-expansion-recovery.md) — blocked by open PR #3598
- [ ] [Task 05: Deliver placement dialog and documentation](task-05-placement-dialog.md) — available placement states are implemented; expansion states remain blocked by Task 04

## Verification results

Design validation on 2026-09-14:

- `python3 scripts/list-docs.py validate`: passed, 270 decisions and 922 specifications.
- `python3 scripts/lint-spec-files.test.py`: passed, 36 tests.
- `python3 scripts/lint-spec-files.py --all`: passed.
- Local link and work-order ID validation: passed for all eight new package/design/ADR documents.
- `git diff --check -- docs`: passed. `git status --short` confirmed all six plan/work-order files are present.
- Catalog discovery confirmed the amended requirements, placement design, and ADR.

Implementation validation on 2026-09-15:

- Backend changed-package tests passed for lifecycle, backendapp, executor, task service, SQLite, and worktree packages.
- `go test -race ./internal/persistence/storeconformance -count=1`, `make lint`, `make sqlguard`, and `make -C apps/backend build` passed.
- Frontend typecheck, full web lint, i18n checks, Vite build, focused placement/dialog tests, and changed E2E-file lint passed.
- Desktop and phone E2E passed for parent-root task creation. Existing desktop and phone Add sources regression flows also passed.
- Public documentation validators, specification catalog validation, specification lint, E2E sleep ratchet, and `git diff --check` passed.
- The repository-wide Go run has two unchanged, environment-sensitive probe timing failures in `internal/agentctl/server/process/probe`; all changed packages passed. The repository-wide E2E sleep lint has 205 unrelated baseline errors, including missing rule definitions; the changed E2E files pass that lint directly.
- PR #3598 remains open at head `62851c51fa2d347bbc0d162634c25c541c441d21`. Task 04 and the expansion-dependent part of Task 05 remain blocked. No automatic native-session replacement was implemented.
- Live native-harness compatibility and cross-platform path evidence remain unverified.
Disposable Linux Git proof: a conditional include excluded an inner repository from outer `git status` and `git add .`.
A sibling worktree still reported an ordinary folder with the same name. The temporary repositories were removed automatically.
This proof does not substitute for the work orders' failure, platform, and lifecycle tests.

## Risks

- Native recovery changes in open PR #3598 can change integration details. Task 04 must refresh that dependency.
- Exact nested paths require audit of cleanup, reconstruction, setup scripts, and branch-slot identity. UI-only changes cannot fix the defect.
- Git exclusion precedence and shared repository configuration require real-Git regression coverage and conditional ownership.
- Current live agents can disagree with promoted database paths after legacy attachment. A database path alone cannot prove sandbox access.
- No generic permission widening is included. Providers can still restrict operations for other reasons.
- Explicit native-session replacement remains unauthorized. Expansion must wait for the durable recovery contract and an explicit continuation action.

## Current-workspace follow-up

The [new uncommitted planning package](../current-workspace-sources/plan.md) owns the Files + menu entry and Local/scratch/remote source extension. Historical test results and existing work-order statuses remain unchanged. This follow-up does not satisfy or remove any pending root-expansion recovery gate.
