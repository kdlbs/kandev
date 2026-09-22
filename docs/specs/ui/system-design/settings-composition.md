---
status: current
system: ui
requirements:
  - REQ-UI-SETTINGS-COMPOSITION-001
  - REQ-UI-SETTINGS-COMPOSITION-002
  - REQ-UI-SETTINGS-COMPOSITION-003
  - REQ-UI-SETTINGS-COMPOSITION-004
  - REQ-UI-SETTINGS-COMPOSITION-005
---

# Settings Composition System Design

## Purpose and boundaries

This design owns reusable settings composition, including the Task behavior arrangement.
Existing domain components retain API calls, drafts, permissions, validation, and immediate commands.
No settings route, backend contract, or plugin API changes.

The current implementation has shared groups and rows, a collapsed Runtime group, and lengthy inline descriptions.
The 2026-09-22 revision replaces that presentation with visible header tabs and optional technical help.
The original implementation remains the baseline for field state, permissions, and persistence.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| REQ-UI-SETTINGS-COMPOSITION-001 | Shared presentation and rollout |
| REQ-UI-SETTINGS-COMPOSITION-002 | Task behavior composition |
| REQ-UI-SETTINGS-COMPOSITION-003 | Concise help and state lifetime |
| REQ-UI-SETTINGS-COMPOSITION-004 | Mobile and accessibility |
| REQ-UI-SETTINGS-COMPOSITION-005 | Header tabs and discovery |

## Shared presentation and rollout

Reuse `SettingsPageHeader`, `SettingsSection`, `SettingsCardHeader`, `SettingsField`, and the settings sizing helpers.
Reuse `SettingsGroup` and `SettingsRow` from `components/settings/settings-group.tsx`.
Do not create a schema-driven settings renderer or a second settings store.

A group has one semantic heading, optional description/action, and one outer `SettingsCard` by default.
The `SettingsSection` adapter has an explicit frameless mode for plugin-owned
content, which remains outside the native settings frame.
Simple rows have no separate card frame or repeated title.
Use `SettingsSaveDirtyScope` for aggregate group markers while preserving each field marker.
Keep one discovery registration per existing target ID, attached to its actual row or subgroup.

Use three compositions:

| Composition | Shape | Use |
| --- | --- | --- |
| Preference group | Heading, bordered group, divided rows | Toggles and short selectors |
| Form group | Heading, bordered group, vertically related fields | Profiles, credentials, workspace forms |
| Resource group | Heading and actions, existing list or table | Repositories, agents, users, plugins |

Use a settings-local spacing contract: 24px between groups, 16px group padding, and 12px row vertical padding.
Use 8px label/control spacing in form fields and 4px label/helper spacing in compact rows.
These values use existing spacing tokens and root-font scaling.
Keep the current typography roles rather than introducing another font scale.
Ordinary desktop controls remain 28px. Touch controls retain at least 44px active targets.
Content rows grow with text and do not have a fixed height.
Use theme border and surface tokens without extra shadows or custom colors.

Use a single page heading followed by ordered group headings.
Allow `SettingsCardHeader` to use a heading level appropriate to its parent rather than hardcoding nested h3 headings.
Retain existing header tabs and action placement from the header-tabs contract.
Do not change global `@kandev/ui` Card styles or use broad descendant CSS to restyle unrelated components.

The [surface inventory](../../../plans/settings-composition/surface-inventory.md) assigns every first-party route family to a work order.
Within each family, inspect route imports and child forms, including dialogs reached from those pages.
Only shared typography, spacing, descriptions, and action composition change in dialogs.
Their lifecycle and primary submission controls remain intact.

## Task behavior composition

Keep `/settings/preferences/task-behavior`, its breadcrumb, and all legacy redirects.
Add the existing `SettingsPageHeader` with `SettingsTabsList` in its tabs slot, matching Data & Logs.
Use three tabs with stable IDs: `tasks`, `conversation`, and `runtime`.
The default is `tasks`. Do not add pages or sidebar entries.

| Tab | Existing controls in display order |
| --- | --- |
| Tasks | `CreationAutoFocusSettings`, `AgentGeneratedTaskTitleSettings`, `MCPTaskAgentProfileDefaultSettings`, `PreventAutoStartAgentSettings`, then `ArchiveConfirmationSettings` |
| Conversation | `UnreadDividerSettings`, `AnchoredPromptBarSettings`, `TodoListPanelSettings` |
| Runtime | `SessionCapacitySettingsContent`, `MessageQueueSettingsContent`, `SleepInhibitionSettings` |

Every section in the active tab is expanded. Remove the Runtime disclosure, its chevron, and its collapsed summary.
Remove the redundant introduction below each Task behavior group heading.
Keep short subgroup labels where they distinguish limits, merging, and power controls.
Keep profile options as two compact radio choices with one-sentence descriptions.
The profile precedence algorithm moves to the profile setting's info content.

The [work order](../../../plans/settings-composition/task-06-concise-help-and-tabs.md) contains the complete visible-copy inventory.
Each setting has a short description and an info control with its applicable detail.
Do not invent technical explanations merely to fill a popup.
Existing long descriptions are source material, not text to copy wholesale into every popup.
Preserve unique technical facts once, under the setting they explain.

## Concise help and state lifetime

Add a settings-local `SettingsInfo` component, using the sleep-prevention info control as the shipped exemplar.
Use existing `Tooltip` for noninteractive desktop details and `Drawer` for touch, selected through `useTouchDrawer`.
Provide a named button beside the label, such as “About agent-generated titles”.
Do not nest the info button inside a toggle label or radio label.
Hover and keyboard focus reveal desktop details. Escape dismisses them.
Touch opens a titled drawer with one scroll region, safe-area clearance, and focus return.
Keep the same translated content in both presentations. Opening info never mounts another setting owner.

Add an info slot to `SettingsRow`. Keep short help connected to the control through the existing generated description ID.
Use plain text or structured short paragraphs in info, including the existing technical identifiers where needed.
Do not place links, inputs, or other interactive controls inside Tooltip content.
Runtime field components use the same info component beside their labels.
Replace the existing profile-specific inline MCP explanation and sleep-specific trigger with this shared pattern.

Keep active load errors, invalid input, permission restrictions, and managed-value notices inline.
Represent instance scope once at the Runtime tab introduction: “Applies to all workspaces.”
Keep “Manual starts can exceed this limit” beside the automatic-session field.
Keep “0 means unlimited” beside the queue field.
Show effective values and source badges only where they explain a difference or an override.
Move merge compatibility algorithms, title fallback rules, OS commands, and session-copy precedence into info.

Retain the existing stateful domain components and one contributor per resource.
The page can retain queue/session hooks above tab presentation, avoiding duplicate fetches.
Pass their aggregate dirty state to the Runtime group as today.
Remove Runtime-specific reveal keys, attention callbacks, and summary formatting only when they lose all callers.
Do not remove general target-disclosure support still used elsewhere.

## Header tabs and discovery

Reuse `SettingsTabs`, `SettingsTabsPanel`, and `useSettingsTab` with their existing URL and mounted-panel contracts.
Panels remain mounted after first visit. Never conditionally unmount a visited settings owner on tab selection.
The route-level save provider owns all dirty contributors across panels, including Reset and partial failures.
Do not add tab-specific Save buttons or bypass cross-route guards.

Map existing `GENERAL_SETTINGS_TARGETS` to tabs:

- `agentTaskProfile`, `agentGeneratedTitles`, `archiveConfirmation`, `creationAutoFocus`, `preventAutoStartOnOpen`: `tasks`.
- `unreadMessages`, `transcriptNavigation`: `conversation`.
- `messageQueue`, `sessionCapacity`: `runtime`.

Preserve target IDs and aliases. Reuse the existing tab hook for initial fragments, query selection, history, and explicit discovery events.
A recognized fragment takes precedence over an unrelated tab query.
For a previously visited hidden panel, complete selection before calling target focus/highlight.
Use one post-activation reveal request through the existing registry if the existing tab composition does not already guarantee this ordering.
Do not create another registry or retry timer.

Use `UnsavedChangesBadge` in tab labels for dirty panels, with unchanged base tab names for accessible matching.
Report dirty/error state from existing domain owners without another save contributor or persisted UI state.
A new invalid draft or save failure in an inactive tab selects that tab once so its field is visible.
Resolve simultaneous failures by tab order: Tasks, Conversation, Runtime. Do not switch repeatedly on rerender.
A load error in an inactive tab shows an accessible error marker and opens when its tab is selected, without stealing active editing focus.

## Mobile and accessibility

The shipped Settings index, page shell, and Data & Logs header tabs define navigation and geometry.
Desktop tabs align beside the page title. Phone tabs form a row below the title.
Short English labels fit the existing tab strip. Translated overflow stays within that strip, not the document.
Runtime remains a labelled navigation choice, not an unlabeled expansion affordance.

All sections stay visible in the active tab. Info is a temporary explanation, so phone details use a drawer.
Keep settings in one page scroll region. The drawer owns scrolling only while it is open.
Keep existing safe-area-aware Save controls and content clearance.
Touch tab and info targets measure at least 44px. Desktop controls retain their shared 28px size.
Selectors stack below descriptions on phones. Switch labels wrap without clipping the control or its active target.
Inactive panels cannot receive focus. Info triggers have visible focus and localized accessible names.

## Localization and compatibility

Render translations through `t()` or `Trans`, including tab labels, badges, info labels, and short descriptions.
Update English, Portuguese, Simplified Chinese, and Japanese. Generate Traditional Chinese through `pnpm run i18n:zh-hant`.
Use plural keys with counts. Descriptions may span multiple lines and never use ellipsis.
Preserve existing translation keys when their meaning remains unchanged.
Preserve technical values, existing test IDs where practical, and search aliases for old labels.

The [typography package](../../../plans/settings-typography/plan.md) remains a related implementation record.
Its existing unfinished statuses are not evidence of completion or a reason to rebuild shipped primitives.
Record this package as the owner of grouping and spacing. Reconcile overlapping verification evidence without replacing historical results.
The requirement and design pair preserves the local composition rationale, so this change does not require a separate ADR.

## Verification and documentation

Use component tests for cross-tab drafts, error routing, accessible help, and target revelation.
Use desktop and mobile E2E for tabs, expanded runtime, info interaction, saving, and repeated search targets.
Extend the existing settings typography route matrix for each migrated family.
Keep public documentation unchanged during design. During implementation, update affected Task behavior descriptions and section names.
Add the shared composition rule to `apps/web/AGENTS.md` when its implementation ships.

## Related contracts

- [Settings typography](../requirements/settings-typography.md)
- [Settings discovery](../requirements/settings-discovery.md)
- [Settings header tabs](settings-header-tabs.md)
- [Control sizing](control-sizing.md)
- [Route save coordinator](../../../decisions/0046-settings-route-save-coordinator.md)
- [Implementation plan](../../../plans/settings-composition/plan.md)
