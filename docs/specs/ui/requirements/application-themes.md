---
status: active
system: ui
created: 2026-10-22
updated: 2026-10-22
owners:
  - kandev
---

# Application Theme Support

## Overview

Users select installed themes to change the whole application, including text
editors and diff viewers. This is an extensible presentation system; Monokai is
an example contributed theme, not a built-in product feature.

UI owns reusable selection, preview, resolution, and rendering. The plugin
system owns [theme packages, imports, and marketplace lifecycle](../../plugins/requirements/theme-packages.md).
The user confirmed whole-app scope and both native packages and VS Code theme imports.

## Terminology

- **Choice:** System, Light, Dark, or an installed package's theme variant.
- **Theme reference:** Package identity plus variant key, independent of label.
- **Effective theme:** The validated current palette, with missing roles filled
  from its declared built-in light/dark base.
- **Application-owned surfaces:** Kandev chrome and embedded native UI. External
  websites, hosted IDEs, images, and arbitrary plugin-owned colors are excluded.

## Requirements

### REQ-UI-APP-THEMES-001: Extensible application appearance

**Intent:** Adding another theme does not require new theme-specific application code.

#### Acceptance criteria

- **AC-UI-APP-THEMES-001.1:** Appearance shall offer System, Light, Dark, and
  every eligible installed variant from the authoritative theme inventory.
  Labels shall identify the contributing package when names collide. Existing
  saved choices and default System semantics shall remain compatible.
- **AC-UI-APP-THEMES-001.2:** Selecting a contributed theme shall apply its
  effective palette to navigation, boards, task chrome, settings, menus,
  dialogs, toasts, rich-text surfaces, and theme-following integrated terminals.
  Host-styled plugin components shall inherit the same palette.
- **AC-UI-APP-THEMES-001.3:** Editable and read-only code, file/Review/Changes
  diffs, chat diffs and chat code shall use the selected theme through every
  currently selectable renderer. Changing themes shall preserve content,
  unsaved edits, undo history, selection, terminal sessions/buffers, comments,
  and diff-mode preferences. Renderer grammar differences may affect detailed
  tokenization; the documented portable role mapping shall work in all renderers.
- **AC-UI-APP-THEMES-001.4:** Unspecified roles shall use the selected theme's
  declared base. New themes shall not inherit leftover colors from a previous
  custom theme. Missing or invalid built-in preferences default to System;
  System follows OS changes, while explicit themes retain their declared scheme.
- **AC-UI-APP-THEMES-001.5:** Existing semantic addition/deletion, status,
  diagnostic and focus cues shall remain present. Theme preview shall identify
  low-contrast text/control pairs and unsupported mappings without changing
  author colors silently. Built-in fallback palettes shall retain readable
  contrast; labels/icons shall continue conveying meaning independently of color.

### REQ-UI-APP-THEMES-002: Selection lifecycle and recovery

**Intent:** Users can preview and keep themes without losing work or becoming
stuck on a broken or unavailable palette.

#### Acceptance criteria

- **AC-UI-APP-THEMES-002.1:** Appearance shall preview a selection immediately;
  explicit Save shall persist the choice on this browser/device. Reset,
  discard, or reload before Save restores the saved choice. Newer drafts shall
  survive an older Save acknowledgement, including storage-write failure recovery.
- **AC-UI-APP-THEMES-002.2:** Saved themes shall restore after reload and backend
  restart when still eligible. Validated cached shell colors may prevent a
  light flash while the authoritative inventory loads. Missing/disabled/removed,
  incompatible or corrupt variants shall use their declared built-in base (or
  System if unknown), show an unavailable state, and retain the saved reference
  for deliberate recovery rather than silently persisting a replacement.
- **AC-UI-APP-THEMES-002.3:** On an observed theme update/removal/disable or
  rejected/stale content response, the effective theme shall be reconciled to
  the latest eligible inventory. A failed catalog refresh shall retain the last
  acknowledged theme and show retryable status; it shall not imply removal.
  Invalid loaded data shall never reach CSS or a highlighter. Failed highlighting
  shall preserve readable code; late results shall not overwrite newer source/theme.
- **AC-UI-APP-THEMES-002.4:** Existing quick light/dark actions shall select
  their labelled built-in destination from any custom theme. Appearance shall
  expose Import and Browse themes entry points with management permissions
  explained. Installation alone shall never change the user's saved choice.
- **AC-UI-APP-THEMES-002.5:** Desktop and phone users shall select, preview,
  save, discard, and recover themes through existing settings navigation with
  keyboard/touch controls, at least 44px phone/coarse-pointer targets, contained
  pickers, safe-area-clear actions, and localized labels/status. Responsive
  fallbacks shall not overwrite the saved desktop choice.

## Out of scope

A built-in Monokai rollout, a visual theme editor, separate editor/diff overrides,
fonts/layout/icon themes, account preference synchronization, external IDEs,
OS-native chrome, recoloring authored content/images, and a new plugin host theme
API. Intentional fixed-dark terminals retain their existing behavior.

## Implementation plan

- [Extensible theme support](../../../plans/application-themes/plan.md)
