---
status: current
system: ui
requirements:
  - REQ-UI-NEW-SESSION-ADVANCED-001
  - REQ-UI-NEW-SESSION-ADVANCED-002
---

# New session advanced settings design

## Boundary and grounding

This is launch-form presentation, not a new MCP state owner. Implementation
baseline: main at `6a1a2ae10003c6bccc754ab144c6035ac38158d1`. The user explicitly
selected New Task's Advanced settings as the exemplar. At this baseline,
`NewSessionDialog` places `MCPSessionSelector` before `TaskFormInputs`; on
desktop its outlined disclosure reveals the full selection body inline.

Use `TaskCreateAdvancedSettings` and `TaskCreateMCPSettingRow` as the visual and
interaction references. Do not mount the task-specific component with its task
copy or task-dialog portal hook inside the session dialog.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| REQ-UI-NEW-SESSION-ADVANCED-001 | Composition; State ownership |
| REQ-UI-NEW-SESSION-ADVANCED-002 | Responsive picker; State ownership; Verification |

## Composition

In `apps/web/components/task/new-session-dialog.tsx`, place a small
`NewSessionAdvancedSettings` component after the composer and its recovery
feedback, before `DialogFooter`. Keep Context visible above the composer.
Use the New Task ghost disclosure style: subdued label, adjacent rotating
chevron, no full-width outlined dropdown chrome. Default closed per form mount.

Inside, render one compact label/help/count row. Use `settings:mcpServers`,
`settings:mcpSelectedCount` and `task:advancedSettings`; help text must describe
session additions, never task-wide selection. Keep help within the information
control or picker rather than adding a persistent paragraph to the form.

Use a creation-only `NewSessionMCPPicker` for the count-triggered overlay.
`MCPSessionSelector` also serves Task Edit at the implementation baseline, so
retain its shell. Reuse `MCPSessionSelectionBody` and its shared selection content
contracts without changing the existing-session explorer. Do not generalize
unrelated forms or add a settings registry.

## Responsive picker

Use `useTouchDrawer` for coarse-pointer layouts and the responsive phone boundary
for narrow fine-pointer layouts, with shared control sizing. Fine-pointer desktop uses a bounded
Popover aligned to its trigger; the portal must belong to the New Session
Dialog's content/focus boundary. Do not reuse the task-create-specific portal
context without an explicit shared boundary.

The launch dialog owns the popover container and passes it through the form.
Its content must allow overflow so the anchored picker is not clipped at the
dialog edge. The picker itself bounds and scrolls its list.
Ordinary picker Enter events cannot reach the dialog's default launch action.
Enter in the search field also prevents implicit form submission. IME candidate
confirmation (`isComposing` or key code 229) retains its native behavior. The
trigger's accessible name includes its translated label and selected count.

Touch uses one Drawer/sheet with title, close action, safe-area padding and a
single internally scrolling list. Bottom padding is at least 8px so the list
stays inside the shared Drawer surface inset, including on devices with no
safe-area inset. Preserve the underlying form without opening
an additional nested selection surface. Escape/close dismisses only the picker
and restores focus. Collapse closes any open overlay before hiding its trigger.
Use existing token spacing and reduced-motion behavior. Long labels wrap or
truncate with readable accessible names. Match New Task density rather than
introducing a new visual language.

## State ownership

`useNewSessionMCPSelection` remains mounted in the form, above the disclosure.
`mcpServerIds`, profile/context state, prompt ref and launch submission remain
owned there. Unmounting the collapsed presentation cannot clear selections or
prompt content. Local search/open state may reset on a fresh form opening.

Retain disabled/loading/unavailable behavior, inherited annotations and
selection eligibility from the current controller/content. Launch continues
through `useSessionLaunchSubmit` without API or payload changes. Existing
session revisions, exclusions and application evidence are outside this shell.

## Verification

Exercise the composed form, not only static markup: count after selection and
collapse, preserved drafts, Escape focus, nested overlay containment, explicit
launch payload and fresh-open reset. Cover phone and coarse tablet input,
long localized labels, empty/loading/unavailable content and handoff. Existing
New Task MCP selection and existing-session Customize remain regression controls.

## Delivery relationships

No prerequisite on assignment #4760, exclusions #4761 or built-in packages
#4762 is needed for this presentation change. Implement on current main after
checking for newer changes to the owned files; coordinate any actual shared
file conflict at integration time. No separate ADR is needed for reusing the
existing interaction pattern.

## Implementation plan

- [Plan and verification](../../../plans/new-session-advanced-settings/plan.md).
