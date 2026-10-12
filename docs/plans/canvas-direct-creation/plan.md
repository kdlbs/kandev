---
created: 2026-09-10
status: complete
requirements:
  - REQ-CANVASES-AGENT-WEB-APPS-009
system_design:
  - ../../specs/canvases/system-design/guided-canvas-task-launch.md
legacy_specs: []
---

# Implementation plan: Direct canvas creation

## Overview

Open the normal task dialog directly from the empty canvas sidebar row. Show
an editable goal followed by `@create-canvas`. Keep detailed instructions in
an editable built-in saved prompt and reuse backend expansion.

Canvases owns this outcome because it owns guided canvas task launch. Extend
the existing requirement/design pair; Tasks continues to own saved-prompt
delivery. Implement the prompt before the launcher starts referencing it.

## Scope

### In scope

- Direct sidebar setup and the existing workspace settings launcher.
- Localized coordinator-view goal, with an exact saved-prompt reference.
- Built-in definition, customization preservation, and structured launch tests.
- Existing scratch/local defaults, editable task choices, error recovery,
  desktop focus behavior, and phone task creation.
- Public usage documentation when implementation ships.

### Out of scope

- New canvas builders, MCP tools, permissions, promotion, or flag rollout.
- Generic task-composer autocomplete changes or terminal prompt expansion.
- A new mobile navigation entry or a new saved-prompt persistence model.
- Live-model evaluation or task creation in this planning package.

## Technical approach

Task 01 adds `apps/backend/config/prompts/create-canvas.md` and a
`builtin-create-canvas` entry in `getBuiltinPrompts`. Preserve existing
insert-on-conflict seeding. A user's same-name definition takes precedence.
Exercise the actual prompt service and orchestrator launch path; do not add
browser expansion or change shared failure and passthrough behavior.

Task 02 gives `CanvasTaskCreateLauncher` a sidebar presentation while keeping
one preset, open-state owner, and success handler. `EmptyCanvasRow` becomes a
button. Keep the settings shortcut and the current route when opening.
Mount the open dialog independently of empty-list rendering, and reset it on
workspace identity or feature availability changes. Restore focus on cancel.

Use the existing locale key for the goal and append `\n\n@create-canvas`
outside translation. The editor remains plain text. Users can replace the goal,
remove the reference, or customize its definition in Settings > Prompts.
The existing autocomplete's inline insertion remains unchanged.

The task description stores the user's text. Starting a structured session
adds hidden expansion to its canonical first message and agent prompt. A task
created without starting resolves at its later launch, using the then-current
definition. Custom workflows retain their existing task-prompt composition.

## ASCII UI preview

### UI-01: Desktop setup, empty workspace

Current: `Set up a canvas -> workspace settings -> Create canvas -> dialog`.
Proposed: `Set up a canvas -> dialog`.

```text
CANVASES                  [Settings] [v]
[Set up a canvas]
          |
          v
+-------------------------------------------------------------+
| Create a canvas                        Source: None           |
| +---------------------------------------------------------+ |
| | Create a new Kandev canvas with a coordinator view        | |
| | that lists the existing tasks.                           | |
| |                                                         | |
| | @create-canvas                                          | |
| +---------------------------------------------------------+ |
| Agent [v]   Model [v]   Executor [Local v]                    |
| Workflow [v]   Step [v]                                      |
| Advanced settings [v]                                       |
|                                  [Cancel] [Start task v]     |
+-------------------------------------------------------------+
```

The existing task dialog owns exact control geometry and optional fields.
Structure requirements: editable goal before task options, reference stays
short, one shared dialog, no settings navigation, existing submit choices.
Spacing and labels in this drawing are illustrative. Maps to criteria `.1`–`.4`,
`.6`, `.7`, `.9`, and `.11` under `AC-CANVASES-AGENT-WEB-APPS-009`.

### UI-02: Phone creation from workspace settings

```text
Workspace > Canvases
[Create canvas]
       |
       v
+-------------------------------+
| Create a canvas               |
|-------------------------------|
| Source: None                  |
| Create a new Kandev canvas     |
| with a coordinator view that  |
| lists the existing tasks.     |
|                               |
| @create-canvas                |
|                               |
| Agent / Model [v]             |
| Executor [Local v]            |
| Workflow / Step [v]           |
| Advanced settings [v]         |
|-------------------------------|
| [Cancel]       [Start task v] |
+-------------------------------+
```

Use the shipped full-screen `TaskCreateDialog`, with its form body scrolling
and footer reachable above the safe area. This focused form fits the editable
goal and task options. Use dynamic viewport containment, touch targets of at
least 44px, and no page horizontal overflow. Shared domain state and submit
logic remain identical to desktop. Maps to `.5`–`.7`, `.9`, and `.11`.

### UI-03: Failure and cancellation, both viewports

```text
[User's edited goal and @create-canvas remain in editor]
[Existing task-creation error]
[Cancel]                                  [Start task v]
```

Failure uses the existing error surface and permits retry. Cancel returns to
the entry surface without navigation or task/canvas creation. While submitting,
the existing pending state prevents duplicate submission. The feature-off or
missing-workspace state has no launcher. Maps to `.11` and existing feature-off
criterion `AC-CANVASES-AGENT-WEB-APPS-001.7`.

## Tests

| Criteria | Targeted evidence |
| --- | --- |
| `.6`, `.8`, `.10` | `store/sqlite_test.go`: new `TestCreateCanvasBuiltin` cases for seed, restart, edits, collision, and required tool instructions. |
| `.9`, `.10` | `orchestrator/create_canvas_prompt_launch_test.go`: new `TestCreateCanvasPromptLaunch` cases using the real prompt service, workflow/no-step start, prepared start, changed saved content, absent reference, and exactly one expansion. |
| `.1`–`.4`, `.11` | `canvases-section.test.tsx`, `canvas-task-create-launcher.test.tsx`: direct opening, unchanged route, defaults, feature-off, missing/changing workspace, list refresh, cancel/focus, submission failure, and single success navigation. |
| `.6`, `.7` | `canvas-task-prompt.test.ts`: every localized goal plus exact assembled reference, no long instructions. |
| `.5`, `.7`, `.9`, `.11` | Desktop and phone Playwright scenarios below. |

All abbreviated criteria belong to `AC-CANVASES-AGENT-WEB-APPS-009`.
Use TDD for changed logic. Preserve existing saved-prompt tests for missing
references, untrusted browser content, idempotence, and passthrough exclusion.

## E2E tests

- In `tests/canvas/plugin-canvas.spec.ts` (`chromium`), add a
  `canvas setup opens task dialog directly` case. Expand the empty section,
  click setup, assert unchanged URL, defaults, cancellation/focus, and a second
  launch. Keep a settings-launch case proving the same preset.
- Update its `canvas creation prompt` case: edit only the goal while retaining
  `@create-canvas`, submit, verify short stored description and normal route.
  Cover create-without-start and later launch in focused backend integration
  tests; do not manufacture browser expansion metadata.
- Update `tests/canvas/mobile-plugin-canvas.spec.ts` (`mobile-chrome`),
  `creates a scratch canvas task`: assert the short preset, editable agent and
  workflow, retained reference on submission, cancellation, and retry after a
  controlled creation error. Assert containment, internal scroll, footer
  reachability, and actual primary touch-target size; inspect a screenshot.
- Keep deterministic lifecycle fixture coverage. Commands may be appended to
  the goal for mock-agent behavior, but must not replace the saved reference
  in expansion coverage. Backend launch capture proves the built-in definition
  reaches the agent; browser visibility alone cannot prove this.

Managed E2E runners rebuild production assets and isolate fixture data. Run
desktop and mobile sequentially with retries disabled.

## Work orders

- [x] [Task 01: Seed the canvas saved prompt](task-01-canvas-saved-prompt.md)
- [x] [Task 02: Open the canvas task dialog directly](task-02-direct-canvas-dialog.md)

Dependency order: 01, then 02. Both are complete and sequential.

## Companion plans

The proposed [task-create chip package](../task-create-prompt-chips/plan.md)
extends reference presentation and autocomplete in the shared create-task editor.
It does not change this package's historical results or completed status.

This package supersedes the sidebar setup redirect in the completed Task 02 of
[the UX follow-up](../plugin-backed-canvases-ux-follow-up/plan.md), and the
long visible preset in completed Task 02 of
[the discovery prompt package](../mcp-discovery-canvas-prompts/plan.md).
Their recorded test results remain historical. Their unrelated work and the
UX package's outstanding external ACP evaluation remain unchanged. Run updated
shared tests through this package; do not reuse historical counts as evidence.

## Verification results

Planning validation on 2026-09-10:

- `rtk python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- `rtk python3 scripts/lint-spec-files.py --all`: passed after splitting guided
  launch into its own Canvases design to meet the 32 KiB design-file limit.
- `rtk git diff --check`: passed.
- Work-order status inspection: both files present, complete, and linked.

Implementation and rendered verification completed on 2026-09-10. Exact
results are recorded in Task 02. The implementation preserves the existing
saved-prompt launch authority and adds the direct desktop entry point without
changing the phone settings path.

## Risks

- A same-name custom prompt intentionally overrides the shipped instructions.
- Deleted/renamed references retain existing non-fatal fallback behavior;
  unsupported passthrough sessions do not receive hidden expansion.
- A custom workflow can omit the task description. This package does not
  override workflow prompt composition or promise expansion in that case.
- The default coordinator lists only authorized data. Broader workspace data
  can require user promotion or permission review.
- The launcher prefers the setup trigger for focus restoration and falls back to
  the always-mounted section header if a list refresh removes that trigger.
- Tests proving prompt delivery cannot guarantee external model compliance.

## Documentation impact

The docs-maintainer check found `docs/public/canvases.md` and
`docs/public/developer-tools.md`. Task 02 updates their creation how-to and
saved-prompt reference when behavior ships. This design turn changes internal
documents only. No new ADR is needed: existing saved-prompt authority and
canvas scope decisions remain in force.
