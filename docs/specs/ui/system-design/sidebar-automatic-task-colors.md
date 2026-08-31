---
status: current
system: ui
requirements:
  - REQ-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-001
  - REQ-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-002
  - REQ-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-003
  - REQ-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-004
---

# Sidebar Automatic Task Colors System Design

## Purpose and boundaries

The UI system owns automatic task colors as a personal presentation contract. The feature reads task facts but never writes task records.

The backend owns the portable settings value. Existing task, workflow, executor, repository, and plugin APIs remain the fact sources.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| `REQ-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-001` | [Editor composition](#editor-composition) |
| `REQ-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-002` | [Rule model](#rule-model), [Color resolution](#color-resolution) |
| `REQ-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-003` | [Repository identity and discovery](#repository-identity-and-discovery) |
| `REQ-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-004` | [Settings persistence](#settings-persistence), [Responsive behavior](#responsive-behavior), [Failure behavior](#failure-behavior) |

## Existing contracts

- `TaskDTO` supplies workflow, step, state, priority, origin, primary executor, and ordered task-repository links.
- Workflow snapshots supply workflow names, step names, and stored step-color values.
- `useRepositories` supplies saved workspace repositories and their provider identities.
- `useRemoteRepositories` supplies built-in and plugin-provider repositories.
- Workspace repository discovery supplies local Git repositories that are not yet saved.
- `filter-dimension-registry.ts`, `use-filter-value-options.ts`, and `apply-view.ts` own the current sidebar filter contract.
- `task-session-sidebar-item.ts`, `session-task-switcher-sheet-item.ts`, and `task-session-sidebar-archived-item.ts` build task-row projections.
- `lib/task-colors.ts` remains the device-local store for manual task colors.
- `PATCH /api/v1/user/settings` remains the only durable write path for personal portable settings.

## Rule model

The user settings object adds `sidebar_task_color_automation`:

```json
{
  "enabled": true,
  "rules": [
    {
      "id": "rule-uuid",
      "enabled": true,
      "condition": {
        "dimension": "task_state",
        "value": "BLOCKED",
        "label": "Blocked"
      },
      "output": {
        "kind": "fixed",
        "color": "red"
      }
    }
  ]
}
```

The settings object is a complete replacement value. A missing object resolves to `{ "enabled": false, "rules": [] }`.

Each rule has one condition and one output. The supported dimensions are:

- `workflow_step`
- `repository`
- `workflow`
- `executor_profile`
- `task_state`
- `priority`
- `origin`

The `value` field stores a dimension-specific target or `null`. The `label` field stores display text for an unavailable target.

An incomplete rule has a `null` value and `enabled: false`. The backend preserves this rule. The resolver never matches it.

The output is either `{ "kind": "fixed", "color": "red" }` or `{ "kind": "workflow_step" }`. The second output is valid only for a workflow-step condition.

The fixed color keys are `gray`, `red`, `orange`, `yellow`, `green`, `cyan`, `blue`, `indigo`, `purple`, and `pink`.

The backend accepts at most 50 rules. It applies these bounds:

- 64 UTF-8 bytes for a rule ID
- 200 Unicode code points for a stored label
- 512 UTF-8 bytes for each entity ID, provider ID, host, scope, or provider repository ID
- 4096 UTF-8 bytes for a normalized local path

The backend rejects duplicate or empty rule IDs, unknown dimensions, oversized fields, malformed targets, and invalid outputs. An enabled rule must have a complete target.

## Dimension contracts and option sources

If values have the same meaning, the automatic-color editor reuses option builders from the sidebar filter editor. A shared `SidebarDimensionOption` model carries values, labels, colors, groups, and unavailable state.

The implementation extracts reusable workflow and step option builders from `use-filter-value-options.ts`. Filter operators and filter matching remain in `filter-dimension-registry.ts` and `apply-view.ts`.

Some rule dimensions differ from filters by product design:

| Rule dimension | Stored target | Option source | Difference from sidebar filters |
| --- | --- | --- | --- |
| `workflow_step` | workspace ID and step ID | Current workspace workflow snapshots | The workspace ID prevents cross-workspace collisions. |
| `repository` | Typed repository target | Repository rule catalog | A rule matches any attached repository. The filter keeps its primary repository slug behavior. |
| `workflow` | workspace ID and workflow ID | Current workspace workflow snapshots | The workspace ID prevents cross-workspace collisions. |
| `executor_profile` | Executor profile ID | Executor profiles from settings state | The filter continues to match `executorType`. The rule UI uses the label Executor profile. |
| `task_state` | Raw `TaskState` value | Static translated task-state list | The filter continues to use the broader review, in-progress, and backlog groups. |
| `priority` | `TaskPriority` value | Static translated priority list | This dimension has no filter equivalent. |
| `origin` | `TaskOrigin` value or `kanban` | Static translated origin list | A missing task origin normalizes to `kanban`. |

If another workspace is active, stored labels keep workspace-scoped targets understandable. A generated localized summary identifies a rule. The model does not persist a rule name.

## Color presentation contract

`TaskColor` remains the seven-color manual palette in `lib/task-colors.ts`. The manual menu does not add gray, cyan, or indigo.

`FixedAutomaticTaskColor` contains the ten fixed rule keys. The shared presentation registry maps gray to `bg-slate-500`. It maps every other key to its matching `bg-*-500` class.

`TaskMarkerPresentation` separates a stored color token from its rendered class or inline color. `TaskItem` consumes this safe presentation value.

The workflow-step output reads the current step color at resolution time. The shared parser supports these inputs:

- every value in the workflow step color catalog
- canonical color names returned by host or plugin projections
- explicitly listed legacy workflow classes, including `bg-neutral-400` and `bg-emerald-500`
- strict hexadecimal color literals in three, four, six, or eight digit form

The parser maps catalog values and legacy classes through static entries. It renders a valid hexadecimal value as an inline background color. It uses gray for every unsupported value.

The frontend never renders a rule value as a CSS class. New task-locale keys supply gray, cyan, and indigo labels for the rule editor in all shipped locales.

## Color resolution

`resolveAutomaticTaskColor` is a pure function. It receives normalized rules and these task facts:

- workspace ID
- workflow ID
- workflow-step ID and color
- task state
- task priority
- task origin
- primary executor profile ID
- attached repository identities

The function scans enabled rules in stored order. It returns the first matching output.

A fixed output returns its safe presentation value. A workflow-step output uses the shared workflow color parser.

An unsupported step color produces gray. A task with no primary executor profile does not match an executor-profile rule.

A missing task origin normalizes to `kanban`. State rules compare raw `TaskState` values instead of sidebar filter groups.

If any attached repository matches the target, the repository condition matches. Repository order does not change this result.

`TaskSwitcherItem` adds workspace ID, priority, normalized origin, primary executor profile ID, workflow-step color, and attached repository identities.

`buildSidebarItem` and `toSheetItem` copy priority, origin, and executor profile ID from the task projection. Their contexts join repository links to full repository records and map step IDs to step colors.

`buildArchivedSidebarItem` copies facts that the archived task projection contains. A fact that is absent does not match its rule dimension.

Desktop and mobile derive automatic colors from the same `TaskSwitcherItem` facts. Task, workflow, repository, and settings updates rebuild the affected presentation.

`TaskItem` still subscribes to the device-local manual color. It renders `automaticColor ?? manualColor`.

This composition preserves the manual value. If automation stops matching, the previous manual marker becomes visible again.

The task color context menu reads both sources. For an automatic effective color, it shows the generated rule summary.

## Repository identity and discovery

A repository condition uses one typed target:

| Kind | Stable fields | Match source |
| --- | --- | --- |
| `workspace` | `workspace_id`, `repository_id` | Exact saved repository ID in that workspace |
| `provider` | provider ID, host, scope, provider repository ID | Saved remote or plugin repository identity |
| `local` | normalized absolute path | Saved or discovered local repository path |

The picker selects the strongest available identity. Provider identity has priority over workspace ID. Local path is used for a local-only repository.

Provider matching includes the host and scope. This rule prevents collisions between providers or self-hosted instances.

A workspace target matches only tasks from its stored workspace. If the stable identity is equal, a provider or local target can match across workspaces.

The repository picker composes existing sources through one catalog hook. It does not call provider APIs directly.

The catalog contains:

- saved repositories from the active workspace
- discovered local repositories
- GitHub, GitLab, and Azure DevOps repositories
- repositories from registered plugin providers

The live catalog searches the active workspace. A stored target from another workspace stays in its rule with its stored label. The editor marks that target unavailable until its workspace becomes active.

The catalog removes duplicate options by stable identity. It keeps source groups and source-specific status.

Search filters saved and local options in memory. It also sends the debounced query to plugin providers that support remote search.

Refresh increments one catalog generation. The generation reloads saved, local, built-in remote, and plugin sources.

Late results from an older generation do not replace a newer result. One provider error does not hide successful sources.

A removed provider or unavailable local path leaves the stored rule intact. The editor uses the stored label and marks the target unavailable.

## Settings persistence

Backend user settings are the durable owner, as required by ADR 0041. The backend stores `sidebar_task_color_automation` inside the `users.settings` JSON value. No new database column or schema migration is required.

The Go user model, DTOs, controller mapping, service patch, stored JSON, HTTP response, boot payload, and settings event carry the complete object.

The frontend wire type and `UserSettingsState` carry normalized rules. `mapUserSettingsData` remains the common ingestion path.

The stored object contains the enabled state, rule order, targets, stored labels, and selected outputs. It does not store one derived color per task.

Automatic-color edits are global across saved views and workspaces. They are independent of a saved-view draft. The editor states this rule next to the control.

HTTP hydration, the boot payload, settings events, and mutation responses use the same backend value. The rules and fixed colors therefore follow the user to another browser.

Manual task colors remain in browser storage. They do not synchronize to another browser.

A serialized optimistic mutation helper owns writes. Each operation applies to the latest local rule list and sends one complete replacement.

The helper maps the authoritative response through the common settings mapper. User-settings revisions reject older HTTP or WebSocket snapshots.

## Editor composition

`SidebarSettingsDisclosure` is a shared collapsed-row primitive. Sort, Group by, Task row, and Automatic colors use this primitive.

Each disclosure has a label, summary, chevron, stable content ID, and `aria-expanded`. Disclosure state is transient and starts collapsed.

The Sort summary shows the selected field and direction. Its expanded content keeps the current `SortPicker`.

The Group by summary shows the selected group. Its expanded content keeps the current `GroupPicker`.

The Automatic colors summary shows Off or an enabled-rule count. Its expanded content contains:

- a global enable switch
- concise precedence and scope text
- ordered rule cards
- an Add rule action

A rule card shows the condition, target, output color, enabled state, reorder control, and remove action. The first matching rule is visually first.

The workflow-step rule offers Use step color. Other rule types use a fixed safe color.

Add rule creates a disabled task-state rule with no target and a fixed blue output. The user can change each field before enabling it.

If the user changes a rule dimension, the editor clears its target and disables the rule. An incomplete rule shows Complete this rule and matches no task.

At 50 rules, Add rule is disabled. The editor shows a localized limit message.

Deleted workflows, steps, executors, and repositories remain visible as unavailable targets. The user can edit or remove those rules.

## Responsive behavior

Desktop keeps the current anchored sidebar popover. Selectors use viewport-contained popovers with searchable command lists.

Phone and tablet keep the current inset drawer in `sidebar-filter-popover.tsx`. This existing surface is the nearest mobile exemplar. Its header stays fixed and its editor body remains the only vertical scroll owner.

The repository picker uses a focused pane inside the open drawer. It follows the focused navigation pattern from `mobile-picker-sheet.tsx`. A Back action returns to the rule card without opening another drawer.

Desktop and mobile share rule state, normalization, matching, catalog data, and actions. Only the picker composition changes.

Touch summary rows, reorder controls, and standalone actions have a 44 CSS pixel active dimension. No action depends on hover.

## Failure behavior

If a settings write fails and the failed operation is still current, the mutation helper restores the latest confirmed settings.

The sidebar sync toast shows a localized error. A failed global write does not change the saved-view draft.

If one repository source fails, the picker keeps successful groups. It shows the error beside the failed source and permits refresh.

If stored automation data is malformed, the frontend disables automation and drops invalid rules. It preserves valid disabled incomplete rules and does not change manual colors.

## Accessibility and security

Color is decorative task organization. Existing state icons, titles, labels, and group headings continue to carry meaning.

Rule summaries include text. A swatch is never the only description of a color or condition.

Reorder supports pointer, touch, and keyboard sensors with an accessible announcement. Focus returns to the changed rule after picker dismissal.

The backend bounds list size and text length. Repository targets contain credential-free identity only.

Plugin repository results remain untrusted data. The catalog treats their labels and URLs as text and validates their required identity fields.

## Tests

Backend tests cover defaults, exact bounds, incomplete-rule validation, JSON round trips, partial patches, settings revisions, and HTTP responses.

Frontend unit tests cover shared option sources, dimension differences, all rule dimensions, precedence, manual fallback, and step-color fallbacks.

Projection tests cover desktop, mobile, and archived task facts. They also cover repository metadata joins and missing facts.

Catalog tests cover search, refresh generations, source errors, duplicate identities, plugin reload, and unavailable stored targets.

Component tests cover disclosure summaries, global copy, rule editing, reorder, automatic-source copy, and desktop or mobile picker composition.

Desktop Playwright coverage proves database persistence across browser contexts, rule precedence, manual fallback, repository search, and live recoloring after a task change.

Mobile Playwright coverage proves the same user value through the drawer. It also proves touch targets, internal scrolling, and no horizontal overflow.

## Related decisions and designs

- [ADR 0041: Backend-Owned Portable User Settings](../../../decisions/0041-backend-owned-portable-user-settings.md)
- [Sidebar Task Row Presentation](sidebar-task-row-presentation.md)
- [Sidebar Task Focus](sidebar-task-focus.md)
