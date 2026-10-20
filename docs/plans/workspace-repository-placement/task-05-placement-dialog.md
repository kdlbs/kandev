---
id: "05-placement-dialog"
title: "Deliver placement dialog and documentation"
status: blocked
wave: 5
depends_on: ["04-expansion-recovery"]
plan: "plan.md"
requirements:
  - REQ-TASKS-ATTACH-WORKSPACE-SOURCES-003
  - REQ-TASKS-ATTACH-WORKSPACE-SOURCES-004
acceptance_criteria:
  - AC-TASKS-ATTACH-WORKSPACE-SOURCES-003.1
  - AC-TASKS-ATTACH-WORKSPACE-SOURCES-003.2
  - AC-TASKS-ATTACH-WORKSPACE-SOURCES-003.3
  - AC-TASKS-ATTACH-WORKSPACE-SOURCES-003.4
  - AC-TASKS-ATTACH-WORKSPACE-SOURCES-003.5
  - AC-TASKS-ATTACH-WORKSPACE-SOURCES-003.6
  - AC-TASKS-ATTACH-WORKSPACE-SOURCES-003.7
  - AC-TASKS-ATTACH-WORKSPACE-SOURCES-003.8
  - AC-TASKS-ATTACH-WORKSPACE-SOURCES-003.9
  - AC-TASKS-ATTACH-WORKSPACE-SOURCES-003.10
  - AC-TASKS-ATTACH-WORKSPACE-SOURCES-004.1
  - AC-TASKS-ATTACH-WORKSPACE-SOURCES-004.2
  - AC-TASKS-ATTACH-WORKSPACE-SOURCES-004.4
  - AC-TASKS-ATTACH-WORKSPACE-SOURCES-004.5
  - AC-TASKS-ATTACH-WORKSPACE-SOURCES-004.6
system_design:
  - ../../specs/tasks/system-design/workspace-repository-placement.md
---

# Task 05: Deliver placement dialog and documentation

## Summary

Expose all three placements in the existing Add to workspace dialog and phone drawer.
Show benefits, costs, authoritative paths, and operation-specific actions before submission.

## In scope

- Shared preview service/state, placement cards, result tree, stale-preview handling, and translated consequences.
- Source selection, no-selection initial state, already-parent-rooted fast path, mixed/folder compatibility, and error preservation.
- Desktop and phone E2E, rendered preview comparison, and public user documentation.

## Out of scope

New dialogs, arbitrary paths, another recovery coordinator, or new placement controls on non-Worktree executors.

## Acceptance

1. UI-03 through UI-06 match the rendered information order and actions on desktop and phone. Every option exposes its benefits and costs.
2. Each submitted selection produces the shown paths and the stated session/process behavior. Stale preview and failed submission preserve form state.
3. Files, Changes, editor targets, and chat file links identify nested/sibling repositories correctly. Mixed/folder and remote flows retain their behavior.

## ASCII UI preview

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



Full preview and shared notes: [plan](plan.md#ascii-ui-preview). The plan owns the combined previews.

## Tests and TDD

Extend source-dialog component tests and add proposed `workspace-placement.test.ts` for view-model and result-tree logic.
Extend the two Task 02 E2E files with all placements, collision, stale preview, no-selection, busy, retry, cancellation, and explicit recovery scenarios.
Assert actual Files/Changes results and preserved session/process identities, not just successful HTTP responses.
Verify phone containment, one scroll owner, keyboard/focus return, readable long paths, and touch target sizes.
Inspect screenshots for UI-03/UI-04/UI-05 and the recovery/error variants. Record differences in Results.

## Verification

```bash
(cd apps/web && pnpm exec vitest run components/task/add-workspace-sources/)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm e2e:run --project chromium tests/task/workspace-repository-placement.spec.ts tests/task/add-workspace-sources.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/task/mobile-workspace-repository-placement.spec.ts tests/task/mobile-add-workspace-sources.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `apps/web/components/task/add-workspace-sources/add-workspace-sources-dialog.tsx`
- `workspace-change-consequences.tsx`, `use-submit-workspace-sources.ts`, source-row state and availability in that directory
- Proposed focused `workspace-placement.ts`, `workspace-placement-options.tsx`, and preview hook in that directory
- `apps/web/lib/types/http-workspace-sources.ts`, source API client and accepted session/workspace projection handlers
- `apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw,pseudo}/task.json`
- The four E2E files in the verification commands
- `docs/public/automation-and-mcp.md` (reference: legacy versus explicit batch semantics)
- `docs/public/executors.md` (explanation: layout, inheritance, process effects)
- `docs/public/coordination.md` and `docs/public/feature-status.md` where existing attachment claims need reconciliation

## Dependencies

Tasks 02 through 04. Reuse the finished preview/materialization/recovery contracts.

## Inputs

Design: Presentation; Preview contract; Failure behavior.
Nearest mobile exemplar: the existing full-height Add sources drawer, with fixed header/footer and internal scrolling.
Retain shared row state and submission hooks. Replace unconditional host-restart copy with actual selected-operation consequences.

## Risks

The old consequences component promises automatic history restoration. Remove that promise for explicit expansion and show the shared recovery action instead.
Nested repositories are Git-ignored in their parent but still require independent Files/Changes visibility.


## Parallelism

`sequential`

## Results

Implemented the available placement dialog surface: no-selection state, nested placement preview, destination/result details, stale-preview and collision handling, localized consequences, preserved source form state, and desktop/phone integration. Focused component tests passed, changed E2E-file lint passed, and the desktop and phone Add sources regressions passed. Public docs were updated and validated.

Blocked for completion by Task 04 and open PR #3598. The expansion choice remains disabled with an explicit recovery explanation, so expansion submission, native continuation, and their recovery E2E states are not implemented. No screenshot comparison or live native-provider compatibility claim is recorded.
