---
status: current
system: ui
requirements:
  - REQ-UI-DESCRIPTIVE-SELECT-OPTIONS-001
---

# Descriptive Select Options System Design

## Purpose and boundaries

This design applies the shared two-row `SelectItem` presentation to the custom
TUI agent MCP strategy selector. It changes only option anatomy, responsive
geometry, and test coverage. The backend remains authoritative for strategy
keys, descriptions, and ordering, and the existing form remains authoritative
for the selected value and submission.

The UI system owns this contract because separating option names from
descriptions is reusable presentation behavior independent of MCP strategy
state. The CLI and agent systems continue to own strategy resolution and
runtime injection.

## Requirement mapping

| Requirement                             | Design sections                                                                                                   |
| --------------------------------------- | ----------------------------------------------------------------------------------------------------------------- |
| `REQ-UI-DESCRIPTIVE-SELECT-OPTIONS-001` | [Option anatomy](#option-anatomy), [Responsive behavior](#responsive-behavior), and [Verification](#verification) |

## Current-state evidence

`MCPStrategySelect` currently formats each backend option as one translated
string containing both `option.key` and `option.description`. Radix Select uses
the whole `SelectItem` text as the selected `SelectValue`, and the shared
trigger is non-wrapping. The resulting intrinsic width expands the add-agent
dialog grid and makes the other full-width form controls overflow with it.

The sidebar view `GroupPicker` and `TypedSortPicker` already use the shared
`SelectItem` `description` property. That primitive renders `ItemText` for the
name and an `aria-describedby`-linked second row for supporting text while
copying only `ItemText` into the closed trigger.

## Option anatomy

`apps/web/components/settings/mcp-strategy-select.tsx` passes
`option.description` through `SelectItem.description` and renders
`option.key` as the item child. The concise Off choice remains a single-row
option because it is a local sentinel rather than a backend-supplied strategy.

The selector trigger gains an explicit full-width, zero-minimum layout so it
shrinks inside the form grid. Selection and wire-value mapping continue through
`toStrategyValue` and `fromStrategyValue`; no strategy identifiers or API
payloads change.

## Responsive behavior

- Desktop and mobile use the existing add-agent Dialog and Radix Select. No
  new drawer or route is introduced because this is a short temporary choice
  and the current selector remains viewport-contained.
- The option list remains the only scroll owner when its content exceeds the
  available height. The dialog and document do not gain horizontal scrolling.
- Coarse-pointer trigger and option rows use a 44-pixel minimum hit area.
  Fine-pointer desktop keeps the surrounding settings density.
- Name, description, selected state, and value mapping are shared across
  viewports; there is no mobile-specific state or business logic.

The nearest shipped mobile exemplar is the sidebar view group selector, which
uses the same shared two-row option primitive. The custom TUI agent selector
reuses its name/description hierarchy and accessible association while keeping
the existing settings-dialog entry point.

## Failure and recovery

Strategy-list fetch failure keeps the existing safe fallback: only the Off
choice is available and custom TUI agent creation can continue without MCP
injection. Missing or empty strategy descriptions do not invent copy; the
shared item renders only the available name. No new asynchronous path is
introduced.

## Persistence and security

None. Strategy persistence, validation, and runtime MCP configuration remain
unchanged. Backend descriptions are displayed as React text and are not
interpreted as markup.

## Verification

A focused desktop browser regression opens the selector with a representative
long strategy description and proves that the open option has separate name
and description nodes with an accessible association. After selection, it
proves that the trigger contains the strategy name without the description.

Focused Chromium and mobile-Chrome tests open the real Add TUI Agent dialog,
inspect a backend-supplied strategy entry, select it, and assert the compact
trigger. Geometry checks cover dialog and option-list viewport containment,
absence of document horizontal overflow, and 44-pixel coarse-pointer hit areas.

## Related decisions

None. The fix reuses the existing shared Select API and establishes no new
public API, persistence boundary, or ownership rule.
