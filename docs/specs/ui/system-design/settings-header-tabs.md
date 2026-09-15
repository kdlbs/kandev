---
status: current
system: ui
requirements:
  - REQ-UI-SETTINGS-HEADER-TABS-001
  - REQ-UI-SETTINGS-HEADER-TABS-002
---

# Settings Header Tabs System Design

## Purpose and boundaries

The existing `SettingsPageHeader` in `components/settings/settings-typography.tsx` remains the header owner.
Add an optional `tabs` slot alongside its existing `actions` slot.
Add a reusable `SettingsTabs` composition in `components/settings/settings-tabs.tsx`.
These proposed exports reuse `@kandev/ui/tabs` rather than a new tab implementation.
Domain components retain data access, drafts, and action handlers.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| REQ-UI-SETTINGS-HEADER-TABS-001 | Header and interaction |
| REQ-UI-SETTINGS-HEADER-TABS-002 | URL selection and state lifetime |

## Header and interaction

`SettingsTabs` supplies controlled root, list, and panel composition around the shared Radix primitives.
Its input consists of stable tab IDs, translated labels, selected ID, and a change callback.
The page places the list in `SettingsPageHeader.tabs` and panels below the header divider, within one Tabs root.
The component accepts no domain-specific tab names, network clients, or settings fields.

Use the shared default segmented treatment: muted track, distinct selected surface, and visible keyboard focus.
The title/description occupies the left column. Tabs occupy the right column and align with the title.
Actions, when supplied, follow the tabs within the right-side group.
Below 768px, the title/description, tabs, and optional actions form successive rows.
Allow the right-side group to wrap below the title when translated content cannot fit.
Constrain the strip width and scroll its contents horizontally. Keep the selected trigger visible.
Use 28px desktop triggers and at least 44px phone/coarse-pointer targets, including the containing track.
Do not modify global tab styles for unrelated consumers.

Use manual Radix activation: arrows/Home/End move focus, and Enter/Space activates a tab.
All tab panels have stable IDs and accessible associations. Inactive panels are hidden and inert.
The outer settings page remains the vertical scroll owner. The header is not newly sticky.

## URL selection and state lifetime

Add a shared `useSettingsTab` hook under `hooks/domains/settings/`.
It reads `tab` through `useSearchParams` from `lib/routing/client-router.ts`.
The page supplies allowed IDs, a default ID, and a target-to-tab mapping.
Unknown values fall back without changing settings. No local-storage preference is added.

Tab clicks replace the current URL query entry with `router.replace(..., { scroll: false })`.
This preserves refresh and copied links without creating one browser-history entry per tab click.
Back/forward restores the tab encoded in each page entry. Existing dirty navigation guards still apply to traversal.
Preserve unrelated query parameters. Remove a stale settings-target hash when the user explicitly chooses another tab.
A recognized target takes precedence over a conflicting tab query so the requested control can become visible.

The hook can use `runWithNavigationBlockerBypassed` only for a validated same-origin, same-path tab replacement.
The consumer must preserve its mounted draft owners across that replacement.
Do not bypass guards for cross-page discovery, redirects, or browser traversal.
The existing `SettingsSaveProvider` is keyed by pathname, so query changes preserve its identity.
Keep stateful panels mounted after first activation and use explicit hidden/inert state when inactive.
`forceMount` alone is insufficient because it can remove the primitive's automatic hiding.
Keep their save contributors registered. Pure viewers such as Logs mount only while active.
Do not create two responsive copies of domain components.

For search, resolve the target before mounting/revealing content.
Listen for `SETTINGS_TARGET_REQUEST_EVENT` as well as initial fragments and location updates.
Mount the destination panel, then let `SettingsTarget` registration fulfill the pending focus request.
Each target ID has one registration. Hidden panels must not consume focus requests before activation.

## Failure and compatibility

Missing or failed domain data remains inside its panel. Navigation remains usable.
Save errors retain drafts and existing coordinator feedback even when the failing panel is inactive.
No new backend API, settings preference, or global navigation bypass is required.

## Mobile contract

The shipped Settings index and full-height Settings surface provide the entry point and scroll model.
The header uses inline navigation because tabs select primary content, not temporary choices.
Phone targets follow shared control sizing and safe-area behavior from the existing settings shell.
Desktop and phone share selected tab, drafts, and handlers.
Tests cover 767px/768px, the configured phone device, coarse-pointer tablets, long labels, and keyboard focus.

## Related decisions

- [Settings header tabs and maintenance grouping](../../../decisions/2026-09-15-settings-header-tabs.md)
- [Settings route save coordinator](../../../decisions/0046-settings-route-save-coordinator.md)
