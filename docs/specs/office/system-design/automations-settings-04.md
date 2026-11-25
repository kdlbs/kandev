---
status: current
system: office
requirements:
  - REQ-OFFICE-AUTOMATIONS-SETTINGS-002
  - REQ-OFFICE-AUTOMATION-CONTINUITY-001
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

`AutomationEditor` renders sections whose choices come before the fields they
change. Each section is a heading over one card, built by `EditorSection`, and
its dirty state covers every field it renders.

1. `NameField`, which carries the **Enabled** switch beside the name label.
2. `WhenSection`. Triggers come first because the per-PR option, the
   placeholders, and the empty-schedule wording depend on them.
3. `DestinationSection` (**Where runs go**): `TargetModeSection`, then
   `WorkflowField` for task destinations or the conversation selector for
   managed conversations. The workflow precedes the per-PR step picker that
   lists its steps.
4. `RepeatRunsSection` (**Repeat runs**): `ContinuationPolicySection` (not for
   managed conversations), then Max concurrent runs. The shared-task lock and
   the choice that causes it share one card.
5. `InstructionsSection` (**Instructions**): the task title (not for managed
   conversations), `PromptSection`, `FollowUpPromptField`, and
   `PlaceholderList`. Both inputs to the prompts, the trigger and the
   continuation policy, are above it.
6. `AgentSection` (**Agent**): agent and executor profiles, repository access,
   and `AgentToolsDisclosure`. It renders nothing for managed conversations.
7. `FailureSection` (**If a run fails**): `RetryPolicySection`. It renders
   nothing for managed conversations.
8. `RunsSection` and the delete controls.

`AgentToolsDisclosure` is a `Collapsible`, closed by default. Its trigger shows
`summarizeAgentTools`: the labels of enabled built-in packages in catalog
order, the names of selected workspace servers, and a count of selected plugin
tools, joined with `Intl.ListFormat` for the active language, or a failure
line when the MCP choices cannot load. It opens each time the MCP choices fail
to load, an MCP save conflict needs a reload, or an approved plugin tool needs
review, so no warning hides behind the summary; after that it follows the
person's toggles. `isMCPSelectionDirty` and `isPluginToolsDirty` compare the
canonical selections for both the disclosure and the Agent card's changed
state. It owns the `useAutomationMCPOptions` request
and passes the result to `AutomationMCPAccess`, so the summary and the
switches read one response.

## Segmented choice

`SegmentedChoice` wraps the Radix `RadioGroup` primitive. Its items have the
`radio` role inside a `radiogroup` named by the visible heading, and the arrow
keys move the selection, and the group's `aria-describedby` points at the line that
describes the selected option. Each other item's description is a
controlled `Tooltip`: it never opens for the selected item, stays closed when
the selection changes, and is disabled on coarse pointers, where a tap selects
and the description line explains the result.
The selected item has a primary-tinted fill and an inset primary outline, so
it stands out from the muted group in both themes. The styles key on
`aria-checked`, because the wrapping tooltip trigger replaces the item's
`data-state` with its own open state.
Pressing the selected item again keeps it selected. Below the `sm`
breakpoint the group stacks vertically and its arrow-key orientation becomes
vertical. Items are at least 44 pixels high
below `md` and on coarse pointers. Item IDs keep the earlier radio IDs, such
as `automation-retry-finite`.

## Copy and detail

- `PlaceholderList` is a closed `Collapsible` rendered once after both
  prompts. `PromptSection` no longer prints its own list. The prompt editor's
  `{{` completion is unchanged.
- `ScheduleSelector` receives `hasOtherTrigger` from `TriggersSection`. With a
  condition trigger, an empty schedule shows `noScheduleWithTrigger`.
- The frequency select always offers **No schedule**. Choosing it with a
  saved or drafted schedule calls `onClear`, and `TriggersSection` deletes the
  scheduled trigger draft.
- `getTriggerSummary` maps saved PR event IDs through the shared `PR_EVENTS`
  label keys. An unknown ID is shown as saved.
- The Configuration package's authority note renders in the package detail
  view. The third-party permission note follows the workspace server list.
- Option labels are short, and the descriptions carry the detail. Renamed
  labels replace the old ones in every catalog and in public documentation.

## Verification

Component tests cover the segmented choice semantics, each section's options
and descriptions, the placeholder disclosure, the schedule wording and
clearing, the tools summary, and the trigger summary. Desktop and phone E2E
scenarios cover section order, the workflow above the step picker, the
selected-option fill in both themes, stacked options with touch-sized targets,
the shared-task lock, retry details, the tools disclosure, and the per-PR
follow-up position.
