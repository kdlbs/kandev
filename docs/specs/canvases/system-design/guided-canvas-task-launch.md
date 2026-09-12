---
status: draft
system: canvases
created: 2026-09-10
requirements:
  - REQ-CANVASES-AGENT-WEB-APPS-001
  - REQ-CANVASES-AGENT-WEB-APPS-009
---

# Guided canvas task launch design

## Ownership and requirement mapping

Canvases owns guided task creation and canvas-specific authoring instructions.
This is a capability split from [the canvas lifecycle design](agent-authored-web-apps.md),
not a separate frontend contract. Tasks owns saved-prompt delivery.

| Requirement | Design sections |
| --- | --- |
| `REQ-CANVASES-AGENT-WEB-APPS-009` | Guided canvas task launch, Saved prompt preparation |
| `REQ-CANVASES-AGENT-WEB-APPS-001` | Agent authoring guidance; lifecycle remains in the linked design |

## Guided canvas task launch

The desktop sidebar and workspace Canvases settings page use one shared canvas
task preset. The preset opens the standard `TaskCreateDialog`. It does not
create canvas metadata or add a canvas-only form.

The preset supplies:

- a localized task title and short canvas goal followed by `@create-canvas`
- repository-free source mode with an empty scratch path
- a preference for an eligible local executor profile
- the selected workspace

The normal dialog continues to own workflow, workflow step, agent profile, and
executor compatibility. The workflow and agent profile remain editable. The
executor preference uses capability-based selection and never stores a profile
identifier in the preset.

Successful task creation follows the normal task route. The user continues the
conversation there, and the task agent uses the authoring lifecycle. The same
full-screen task dialog serves the workspace settings action on a phone. The
desktop sidebar does not exist at that viewport.

All launch surfaces remain behind `features.canvases`. A disabled client does
not request canvas counts, add a settings tab, or register the task preset.

`EmptyCanvasRow` in `components/app-sidebar/sections/canvases-section.tsx`
becomes a semantic button using `CanvasTaskCreateLauncher`. Give the shared
launcher a small presentation option for the sidebar row versus the existing
settings button. Keep one owner for open state, preset values, and success
navigation. Preserve `sidebar-canvases-empty` and `settings-create-canvas`
selectors. The separate `OpenCanvasSettingsShortcut` remains a management link.
Opening or cancelling creation does not push a route or create canvas records.

Bind the dialog to the active workspace at opening. Close and reset it if that
workspace changes, disappears, or the feature becomes disabled. A stale draft
must not submit into a newly selected workspace. Keep the launcher outside any
row that can disappear when a canvas-list refresh completes while it is open.
Use the existing focus-return contract for cancellation and normal success
navigation, including `meta.willNavigate`, to prevent duplicate navigation.

The phone entry remains workspace Canvases settings. The existing full-screen
`TaskCreateDialog` is the closest shipped exemplar: one focused form suits a
long editable goal with task options. Reuse its business state and submission
handler. Keep the form body as the scroll owner, with reachable footer actions,
dynamic viewport containment, safe-area clearance, and at least 44-pixel touch
targets. Retain ordinary desktop control sizes. No new mobile navigation or
canvas-only form is needed. On submission failure, preserve the goal and normal
dialog error/retry behavior. Test the rendered phone surface and focus return.

### Saved prompt preparation

Add `config/prompts/create-canvas.md` to the backend's existing prompt embed.
Register `builtin-create-canvas`, named `create-canvas`, in
`internal/prompts/store/sqlite.go:getBuiltinPrompts`. The current
`seedBuiltinPrompts` insert-on-conflict behavior seeds fresh and existing
installations without overwriting same-name prompts or user edits. No database
schema or canvas service change is required. This is normal saved-prompt data;
its presence does not expose disabled canvas routes or grant tool capabilities.

Settings > Prompts exposes the definition through existing prompt management.
Use the current name-collision behavior: a pre-existing user `create-canvas`
definition wins. Do not introduce a reserved name, silently rename user data,
or recreate a removed record during dialog opening. Startup retains the normal
built-in reseeding behavior. Prompt content is editable agent instruction data,
stored in English by default; it is not duplicated across UI locale catalogs.

The browser submits the edited description with its literal reference. Existing
`AppendReferenceExpansionsWithContext` and orchestrator launch preparation
resolve the current saved record and attach trusted hidden context. See
[Saved Prompt Delivery](../../tasks/system-design/saved-prompt-delivery.md) and
[the server-owned expansion decision](../../../decisions/2026-09-01-server-owned-saved-prompt-expansion.md).
Reuse that boundary; do not expand in the launcher or trust browser definitions.
The stored task description stays short. The canonical first user message and
dispatched structured prompt contain the expansion exactly once.

Start task resolves at launch. Create without starting saves the reference and
resolves the current definition when a session later starts. Workflow prompt
composition remains authoritative: a custom workflow that omits the task prompt
does not gain a canvas-specific injection bypass. Test the normal workflow and
no-step launch paths. Passthrough remains literal. Unknown or renamed prompts
and lookup failures use existing non-fatal resolution behavior; no hidden
fallback is fabricated. Removing `@create-canvas` removes this expansion, while
the independently capability-gated general canvas context remains unchanged.

The default literal reference needs no autocomplete change. The existing task
creation picker can still insert a chosen prompt inline; changing that generic
picker behavior is outside this package. The concise preset remains editable
as plain text and expands on submission/launch when the reference is retained.

## Agent authoring guidance

### Discovery and creation prompts

The shared task prompt uses the
[agent discovery contract](../../agents/system-design/mcp-tool-discovery-guidance.md).
When the resolved session profile includes `CapabilityCanvas`, it also includes
this compact instruction:

> For a requested Kandev canvas, discover `create_canvas_kandev`, `read_canvas_authoring_skill_kandev`, and `publish_canvas_kandev`.
> Create the draft in Kandev before writing application files.
> Read the authoring skill once and edit only inside the returned source directory.
> Publish through MCP and report the returned release status.
> If publication is unsuccessful, report the failure and do not claim that the canvas is published.
> Files or a successful local build do not create a published Kandev canvas.

The optional section is absent when the resolved profile lacks canvas tools.
This covers canvas requests inside ordinary task conversations, independently
of the guided task preset.

`CanvasTaskCreateLauncher` uses `canvases:createCanvasTaskPrompt` for the
localized goal. Append the literal `\n\n@create-canvas` in the preset builder so
translations cannot change the lookup name. The assembled English preset is:

```text
Create a new Kandev canvas with a coordinator view that lists the existing tasks.

@create-canvas
```

The definition in `config/prompts/create-canvas.md` contains this guidance:

> Create a Kandev canvas for the goal described in the user's request.
> If the application goal is missing, ask what the canvas must show or do.
> Find the Kandev canvas MCP tools before writing application files.
> If they are not callable, use native tool search for `kandev canvas` or inspect the available MCP catalog.
> Call `create_canvas_kandev` to create the draft and obtain its source directory.
> Read `read_canvas_authoring_skill_kandev` once without a path.
> Build inside the returned directory and use authorized live Kandev data for domain views.
> Call `publish_canvas_kandev` and address any validation errors.
> Report the canvas identity and whether its release is active, awaits permission review, or was unsuccessful.
> If publication is unsuccessful, report the failure and do not claim that the canvas is published.
> If workspace access requires promotion, explain the user action that is still required.
> A local build alone does not publish a canvas inside Kandev.
> If the tools remain unavailable, report the limitation instead of claiming that workspace files are a Kandev canvas.

The existing selected question capability governs any clarification. The
preset cannot grant a question tool to an autopilot session that lacks one.
An application goal already supplied by the user needs no repeated interview.

English, Portuguese, and Simplified Chinese catalogs contain the short goal.
Traditional Chinese catalogs and the pseudo-locale use repository generators.
The saved prompt preserves exact tool identifiers, independent of UI locale.

The nearest mobile exemplar is the existing full-screen `TaskCreateDialog`
opened from workspace Canvases settings. This is a content change within that
surface. Its scroll owner, touch controls, and navigation remain unchanged.
Focused desktop and mobile tests cover the concise editable prompt and its
submitted value. Backend tests cover the full definition and actual launch
expansion. Tests must retain the reference when exercising expansion; replacing
the whole description with mock-agent commands does not prove that contract.


## Implementation plan

[Direct canvas creation](../../../plans/canvas-direct-creation/plan.md).
