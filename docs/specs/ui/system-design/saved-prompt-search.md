---
status: current
system: ui
requirements:
  - REQ-UI-SAVED-PROMPT-SEARCH-001
---

# Saved Prompt Search System Design

## Purpose and boundaries

This is an ephemeral presentation over the prompt list already owned by the
existing prompt store and `useCustomPrompts`. It does not own saved definitions,
permissions or reference expansion. [Saved prompt delivery](../../tasks/requirements/saved-prompt-delivery.md)
and [settings manual save](../requirements/settings-manual-save.md) remain
independent contracts.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-UI-SAVED-PROMPT-SEARCH-001` | List projection, interaction and responsive composition |

## List projection and interaction

`PromptsSettings` keeps a local query separate from `usePromptsState`'s form and
`getPromptDraftMeta`. Normalize only the query with trim and case folding for a
literal substring comparison against case-folded `CustomPrompt.name` and raw
`CustomPrompt.content`. Filter the saved array without mutation or sorting;
built-ins and custom entries follow the same rule. Never expand references or
send the query to a service. Empty normalized query uses the entire list.

`PromptListContent` must distinguish the original loaded collection from the
filtered result: retain its existing loading precedence and `settings:promptsEmpty`
for a truly empty collection, then show `common:noResults` for a nonempty
collection with no matches. The query control remains available in that state.
Use `common:searchPlaceholder` for both placeholder and accessible name; both
keys exist in all eight catalogs. A native search input provides ordinary text
editing and clear behavior.

Disable the query input when `editingId`, `showCreate`, `deleteTarget`, or
`isBusy` is active. Filtering uses the saved entries, never `formState`, so a row
matched before editing remains mounted while its draft is changed. Retain each edited or open delete target ID through saved-field refresh as long as it remains in the loaded list. Include each current saved row in its existing list position even when it no longer matches; do not cache or restore removed rows. After the respective action ends, normal filtering resumes. Existing
`SettingsPageTemplate` draft registration, action handlers, permission controls,
mutation requests, loading and errors remain authoritative. No query state is
passed to draft metadata or mutation calls. The existing session-local query can
remain after save, cancel, discard, or delete; the resulting saved list is then
projected normally. External list updates retain existing behavior; no new
lifecycle or save hardening is introduced.

## Responsive composition

The search input is inline above the existing list, after any create form, in
both desktop and phone settings. The existing settings content owns scrolling
and safe-area behavior; the change introduces no overlay, fixed control, or
nested scroll area. This frequent, shallow filter is directly usable without a
new drawer. The nearby `mobile-prompts-settings.spec.ts` demonstrates the
existing phone settings route and touch actions; the curated mobile language
uses direct navigation for primary content and contained controls. Preserve
existing row actions and phone delete drawer.

Use the native settings input primitive with a shrinkable, contained width and
ordinary desktop density. Phone width and coarse pointer each need a rendered
minimum 44px input height, independent of pointer precision on phones; preserve
the existing coarse-pointer anti-zoom styling. Desktop and phone share the same
query and matching logic. Focus, keyboard entry and native clear remain native.
The mobile browser test measures the actual input and its enclosing surface,
checks zero document horizontal overflow, and exercises touch plus filtering.

## Persistence and failure

Query state exists only while the settings component is mounted. Existing list
loading, failure and mutation behavior is unchanged. Search adds no backend
write, permission boundary, telemetry, or migration.
