---
status: current
system: office
requirements:
  - REQ-OFFICE-AUTOMATIONS-SETTINGS-002
created: 2026-11-25
owners:
  - kandev
---

# Automation editor layout

## Purpose and boundaries

The [automation settings contract](../requirements/automations-settings.md)
owns how the editor arranges its sections. This design places each choice
above the fields it affects and shows option detail on demand. It changes no
API field, stored value, or validation rule. Existing designs keep the
behavior of each control: [continuity](automation-continuity.md),
[target modes](automation-target-modes.md),
[retries](automation-retries.md), [MCP selection](automation-mcp-selection.md),
[plugin tools](automation-plugin-tools.md), and
[PR follow-up](automations-pr-follow-up.md).

## Requirement mapping

| Requirement | Acceptance criteria | Design sections |
| --- | --- | --- |
| `REQ-OFFICE-AUTOMATIONS-SETTINGS-002` | .1, .2, .5, .6 | [Section order](#section-order) |
| `REQ-OFFICE-AUTOMATIONS-SETTINGS-002` | .3 | [Segmented choice](#segmented-choice) |
| `REQ-OFFICE-AUTOMATIONS-SETTINGS-002` | .4, .7 | [Copy and detail](#copy-and-detail) |

## Section order

`AutomationEditor` renders:

1. `NameField`, which carries the **Enabled** switch beside the name label.
2. `WhenSection`.
3. `ThenSection`: `TargetModeSection`, then `ContinuationPolicySection` (not
   for managed conversations), the task title, `PromptSection`,
   `FollowUpPromptField`, `PlaceholderList`, and the configuration fields.
4. `AgentToolsSection`: one card holding `AutomationMCPAccess` and, for
   automation-history runs, `AutomationPluginTools`. It renders nothing for
   managed conversations.
5. `LimitsSection`: Max concurrent runs and, except for managed
   conversations, `RetryPolicySection`.
6. `RunsSection` and the delete controls.

The Then card's dirty state now includes the continuation policy and per-PR
fields, because they render inside it. The name card's dirty state includes
`enabled`.

## Segmented choice

`SegmentedChoice` wraps the shipped `ToggleGroup` primitive with
`type="single"`. Its items have the `radio` role inside a `group` named by the
visible heading, and the group's `aria-describedby` points at the line that
describes the selected option. Each other item's description is a
controlled `Tooltip`: it never opens for the selected item, stays closed when
the selection changes, and is disabled on coarse pointers, where a tap selects
and the description line explains the result.
A single toggle group emits an empty value when the selected item is pressed
again; the wrapper ignores it, so a choice is never cleared. Below the `sm`
breakpoint the group stacks vertically. Items are at least 44 pixels high
below `md` and on coarse pointers. Item IDs keep the earlier radio IDs, such
as `automation-retry-finite`.

## Copy and detail

- `PlaceholderList` is a closed `Collapsible` rendered once after both
  prompts. `PromptSection` no longer prints its own list. The prompt editor's
  `{{` completion is unchanged.
- `ScheduleSelector` receives `hasOtherTrigger` from `TriggersSection`. With a
  condition trigger, an empty schedule shows `noScheduleWithTrigger`.
- `getTriggerSummary` maps saved PR event IDs through the shared `PR_EVENTS`
  label keys. An unknown ID is shown as saved.
- The Configuration package's authority note renders in the package detail
  view. The third-party permission note follows the workspace server list.
- Option labels are short, and the descriptions carry the detail. Renamed
  labels replace the old ones in every catalog and in public documentation.

## Verification

Component tests cover the segmented choice semantics, each section's options
and descriptions, the placeholder disclosure, the schedule wording, and the
trigger summary. Desktop and phone E2E scenarios cover section order, stacked
options with touch-sized targets, the shared-task lock, retry details, and the
per-PR follow-up position.
