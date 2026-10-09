---
created: 2026-09-24
status: done
requirements:
  - REQ-UI-CONTROL-SIZING-001
system_design:
  - ../../specs/ui/system-design/control-sizing.md
legacy_specs: []
---

# Implementation Plan: Plugin settings layout

## Overview

Flatten the host plugin settings page and compact its actions while preserving
the existing plugin lifecycle and permission behavior. This is a presentation
refactor under the existing [control sizing contract](../../specs/ui/requirements/control-sizing.md).
The user authorized implementation, screenshots, and PR delivery in one session.

## Scope and technical approach

Use the unframed SettingsPageTemplate variant, a compact installed-list toolbar,
an inline SettingsRow for automatic updates, and divided PluginRow entries.
Keep the current hooks, translated copy, direct row navigation, errors, and
confirmation surfaces. No backend, persistence, plugin SDK, or release changes.

## ASCII UI preview

UI-01: Settings > Plugins > Installed, populated state. Current phone layout
stacks three full-width buttons and nests the list and preference in cards.

```text
Phone                                 Desktop
[Installed] [Browse]                  [Installed] [Browse]
Installed plugins [Install plugin]    Installed plugins [Install plugin]   Sync  Check for updates
Sync  Check for updates               Automatic updates description                   [switch]
Automatic updates       [switch]     Last-check status
Description                          ---------------------------------------------------------
Last-check status                    Plugin name / status / version / actions
--------------------------------     Description and categories
Plugin name / status / version       Auto-update                                      [switch]
Actions                              ---------------------------------------------------------
Description and categories           Next plugin
Auto-update             [switch]
--------------------------------
Next plugin
```

Structural requirements: retain labels and touch targets while removing nested
frames. The existing settings page owns scrolling and safe areas. Use shared
SettingsRow and control-sizing primitives; retain the existing phone uninstall
drawer. Spacing in the preview is illustrative. Empty/loading/error and pending
actions retain their current content and behavior. Long labels may wrap the
toolbar without causing horizontal scrolling.

## Tests

Existing page and PluginRow unit coverage protects actions and permissions.
Mobile Playwright checks aligned secondary actions, compact Install placement,
44px controls, responsive boundaries, and reachable row actions. Desktop
Playwright checks 28px toolbar controls and the installation lifecycle.
These cover AC-UI-CONTROL-SIZING-001.1, .3, .4, .5, .6, .7, and .8.

## Work orders

- [x] [Task 01: Flatten plugin settings](task-01-flatten-settings.md)

## Verification results

Completed on 2026-09-24. The mobile regression first failed with secondary
actions separated vertically by 52px. After implementation: 2 mobile and 17
desktop Playwright scenarios passed, as did 54 unit tests, typecheck, focused
ESLint, localization, public-doc and spec validators, and diff checks. See the
[work-order results](task-01-flatten-settings.md#results). Four fresh PR screenshots
cover Installed and Browse on phone and desktop with synthetic records; the
disposable capture test passed and was removed afterward.
The public plugin guide uses the new desktop Installed screenshot and no longer
repeats an illustration of the previous layout.

## Risks

Row overlay links must not intercept inline actions. Removing padding must not
clip long plugin IDs or touch targets. All plugin controls retain their existing
permission checks.
