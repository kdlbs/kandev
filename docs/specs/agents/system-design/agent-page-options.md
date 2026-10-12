---
status: current
system: agents
requirements:
  - REQ-AGENTS-PAGE-OPTIONS-001
---

# Agent page options system design

## Purpose and boundaries

This design moves the existing profile-navigation preference into a domain-specific options surface.
It uses the established state hook and shared overlay primitives.
The [navigation requirement](../requirements/hide-disabled-profiles-nav.md) retains ownership of filtering behavior.
No backend or shared overlay changes are required.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| `REQ-AGENTS-PAGE-OPTIONS-001` | Components, interaction flow, responsive composition, copy and accessibility, state and permissions |

## Components

- `InstalledAgentsHeader` in `apps/web/app/settings/agents/page.tsx` adds Options before the existing Terminal control.
- A new route-local `AgentOptionsDialog` in `agent-options-dialog.tsx` owns the Options trigger and its controlled open state.
  It renders a Dialog or Drawer through `@kandev/ui`, selected by `useResponsiveBreakpoint().isMobile`.
- `HideDisabledAgentProfilesSetting` remains the only switch implementation and mounts inside the active surface.
  Its labelled switch retains `id="hide-disabled-agent-profiles-in-nav"`.
  It uses the shorter approved copy and removes the standalone card treatment inside the overlay.
- `InstalledAgentsSection` removes the inline preference row and retains the existing list and shell composition.

The new component owns no profile data and performs no direct fetches.
It shares the trigger, preference, and footer content across responsive presentations.
Only the active overlay branch mounts.

## Interaction flow

1. Options opens the surface with the current preference value.
2. The switch calls the existing `setHideDisabled` function.
3. The existing hook publishes the new value to navigation consumers and other tabs.
4. Done, desktop Close, Escape, or the standard drawer dismissal closes the surface.
5. Focus returns to the Options trigger.

The open state belongs above the responsive branch. A breakpoint change preserves it and the preference.
Use primitive triggers where practical and a shared trigger reference for focus return across branch changes.
Dismissal performs no write and does not undo an earlier toggle.

## Responsive composition

Desktop uses a compact centered Dialog. Match the shared settings controls and 28px desktop action size.
Inspect `DialogContent` breakpoint classes before overriding its width. Override the matching responsive variant.

Phones below 768px use the inset bottom Drawer pattern from `components/kanban/mobile-menu-sheet.tsx`.
Reuse its rounded inset surface, header, internal body scroll, and safe-area treatment through the shared Drawer primitive.
This short, occasional preference fits a temporary drawer rather than a dedicated route.
Tablet and wider views retain the dialog with coarse-pointer touch targets.

The existing installed-agent toolbar can wrap on phones while preserving control order.
Options retains its visible label and sliders icon.
The drawer contains a fixed title, one switch row, immediate-application text, and a Done footer.
Use a dynamic viewport maximum height with one `min-h-0 overflow-y-auto` body.
Override the Drawer primitive's direction-specific `80vh` maximum with the equivalent `dvh` rule.
Clear the bottom safe area. Keep Options, Done, and the switch's active target at least 44px on phones and coarse pointers.
Use a touch-sized labelled row if the Switch primitive alone has a smaller active area.
Avoid double activation when the switch itself receives the tap.

## Copy and accessibility

Translate Options, Agent options, Changes apply immediately, Done, and the revised switch label and helper text.
Use `t()` inside components and semantic Dialog/Drawer titles and descriptions.
Reuse an existing translation only if its meaning matches. Add missing keys across all supported catalogs.
Generate the Traditional Chinese catalogs with `pnpm run i18n:zh-hant` and refresh the pseudo-locale.

Associate the switch label and helper text with its control.
Use the overlay description for the immediate-application text.
Retain focus trapping, keyboard activation, and primitive dismissal behavior.
Supply translated accessible names for icon controls, including desktop Close.

## State and permissions

`useHideDisabledAgentProfilesInNav` retains the existing localStorage key `kandev:agents:hideDisabledInNav:v1` and default `false`.
`useLocalStorageBoolean` retains its storage event and custom-event synchronization.
The preference remains browser-local and does not synchronize across devices.
No migration, draft buffer, new store slice, or authorization write is necessary.
Options is independent of the `canManage` guard used for custom-agent creation.

Read errors retain the hook's default. Write errors retain its existing thrown-error behavior.
The options surface does not claim a successful save on a failed write.
This relocation adds no new success notification or persistence mechanism.

## Verification boundaries

Component tests retain switch semantics and persistence-hook coverage.
Desktop E2E proves toolbar order, opening and dismissal, focus return, saved-value restoration, and actual navigation filtering.
Phone E2E proves drawer composition, persisted toggles, containment, touch targets, and no document overflow.
A narrow fine-pointer case covers 767px and 768px to prove that phone composition depends on width.
Compare rendered screenshots with the [plan previews](../../../plans/agent-page-options/plan.md#ascii-ui-preview).
