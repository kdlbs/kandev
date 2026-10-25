---
created: 2026-09-24
status: implemented
requirements:
  - REQ-UI-MOBILE-MENU-008
system_design:
  - ../../specs/ui/system-design/unified-mobile-navigation.md
legacy_specs: []
---

# Implementation Plan: Mobile plugin deduplication

## Overview

The phone menu independently renders workspace and task toolbar registrations.
Provider Usage creates separate component instances for the same status in both
slots, so component identity cannot identify the duplicate. Choose the task
slot per plugin owner when its task control renders content. The user explicitly authorized
uninterrupted implementation and PR delivery in this session.

## Scope and technical approach

Add an owner exclusion to the host PluginSlot and MainTopBarPluginActions.
MobilePluginNavSection selects exclusions from chat-top-bar owners with mounted
content in its supplied task actions. PluginSlotPresence observes owner wrappers
to preserve the workspace fallback while task controls render no content. Flatten the two context groups into
one wrapping row; retain sidebar workspace actions and plugin destinations.
No SDK signature, plugin implementation, persistence, or desktop behavior changes
are required.

## ASCII UI preview

UI-01: Task hamburger menu. Existing inset drawer, fixed Menu heading, one
safe-area-aware vertical scroller, and 44px touch controls remain the exemplar.

```text
Before                    After
Plugins                   Plugins
  Workspace                 [CPU] [Companion] [Usage 42%]
  [CPU] [Usage 42%]          [Plugin destinations]
  Task
  [Companion] [Usage 42%]
```

Task controls keep task/session context. Listings use workspace controls.
No contributions means no section. Desktop retains its separate toolbar
locations. Wrapping and hierarchy are requirements; spacing is illustrative.

## Tests and E2E

Existing plugin slot and mobile section unit suites cover identity, distinct
owners, multiple task actions, workspace fallback, unload, and saved layouts.
The packaged mobile plugin E2E verifies actions, context, 320/393/767px
containment, and desktop breakpoint behavior. Capture a real installed Provider
Usage plugin with deterministic provider data and API-seeded task records on
base and fixed builds; no DOM replacement.

## Work orders

- [x] [Task 01: Select one mobile toolbar per plugin](task-01-select-mobile-toolbar.md)

## Verification results

- RED: four focused unit failures reproduced owner duplication and subgroup headings.
- GREEN: 64 tests in five focused Vitest suites passed, including task slot
  context and error-boundary coverage. The task slot suite was included in the
  recorded command in addition to the four listed initially.
- Managed production build: all three selected mobile scenarios passed (two
  permanent regressions plus the disposable installed-plugin capture).
- Final mobile E2E: two scenarios passed after adding archived-task fallback
  and 768/1440px parity checks. Phone assertions cover 320/393/767px, control
  activation, current task/session props, one scroller, touch targets, and no
  horizontal overflow. This final run reused the same fresh production build.
- Web typecheck, changed-file ESLint, Prettier, and i18n ratchet passed.
- Public docs: 62 validator tests and 47 pages passed. Catalog validation:
  304 decisions and 1,136 specifications; specification and harness lint passed.
- Five inspected screenshots: before/after dark and light plus expanded usage.
  Installed the unmodified Provider Usage 0.9.3 release in isolated backends;
  seeded three Atlas Studio tasks, a session, and messages through the API.
  The provider's quota HTTP response is deterministic fixture data (42% session,
  28% weekly); no DOM replacement or live account data. Before assets used base
  d60274528129dc17d413357c104c9c427c4e7af9; after assets used the fixed build.
- Capture backend PIDs exited, ports closed, and temporary databases/repositories
  were removed by the fixture. The disposable capture spec was removed; only
  evidence remains in ignored PR assets and an external artifact directory.
- The phone Plugins drawer is absent from desktop; desktop parity is verified
  by the 768/1440px route checks rather than an unrelated screenshot.
- `git diff --check` passed.

## Risks

A plugin with both toolbar slots must put its task-relevant actions in its task
slot. Sidebar workspace actions are independent and remain reachable. Owner
identity is stable even when component factories return distinct functions.

## CI documentation follow-up

The coverage evaluator identified the new requirement missing from the design's
frontmatter. Added `REQ-UI-MOBILE-MENU-008` to the design dependency list. The
exact repository coverage evaluator failed before the metadata correction and
passes afterward (`status: covered`), along with catalog/spec lint and diff
checks. Product code and screenshot content are unchanged.

The authoring-guide review identified stale Workspace/Task subgroup guidance.
Updated both slot tables, the canonical guide, PLUGIN-API reference, and SDK/host
type comments to describe the same owner-based selection. Public docs validation
(62 tests, 47 pages), coverage evaluation, spec/catalog lint, and diff checks pass.

## Null-rendering task fallback

Review found registered task controls can return null before a session exists.
The new regression fails under registration-only selection. Observe mounted
content in layout-neutral owner wrappers, retaining workspace controls until
task content appears and restoring them if it disappears. An observer scoped
to the task slot disconnects on unmount and only reports changed owner sets.
Targeted unit tests, fresh production mobile E2E, and refreshed after screenshots
verify the correction: 64 focused unit tests and all three selected mobile
scenarios passed against a fresh production build.

## Context presentation clarification

The final review identified an older acceptance criterion still requiring
workspace/task distinction. AC-UI-MOBILE-MENU-007.2 now delegates plugin
presentation to 008.1/008.2, and 008.1 explicitly supersedes the former subgroup
distinction. This documentation correction preserves the verified behavior and
screenshots. Catalog validation, specification lint, the exact PR documentation
coverage evaluator, and diff checks passed.
