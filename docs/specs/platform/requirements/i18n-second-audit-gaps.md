---
status: draft
system: platform
created: 2026-08-13
owners:
  - Kandev
---
# Additional I18n Audit Gaps Requirements

## Overview

The second localization audit found English display copy that escapes JSX literal linting because it lives in hooks, plain-object configuration, helper return values, toast payloads, or developer demo pages. These leaks make locale switching and the pseudo-locale incomplete even though the guarded JSX migration is complete.

## Requirements

### REQ-PLATFORM-I18N-SECOND-AUDIT-GAPS-001: Additional I18n Audit Gaps

**Intent:** The second localization audit found English display copy that escapes JSX literal linting because it lives in hooks, plain-object configuration, helper return values, toast payloads, or developer demo pages. These leaks make locale switching and the pseudo-locale incomplete even though the guarded JSX migration is complete.

#### Acceptance criteria

- **AC-PLATFORM-I18N-SECOND-AUDIT-GAPS-001.1:** Review findings, task plans, file operations, task/session actions, task movement, task creation, sidebar synchronization, walkthrough requests, GitHub/GitLab feedback, and related production operations resolve user-facing success and failure copy from catalogs when the operation runs.
- **AC-PLATFORM-I18N-SECOND-AUDIT-GAPS-001.2:** Azure DevOps configuration retains WIQL, IDs, provider/status values, and agent prompt templates as stable data while mode labels, pull-request statuses, default query labels, action labels, and hints are localized at render time.
- **AC-PLATFORM-I18N-SECOND-AUDIT-GAPS-001.3:** Configurable keyboard shortcuts retain stable IDs and bindings while Settings resolves catalog keys for display names at render time.
- **AC-PLATFORM-I18N-SECOND-AUDIT-GAPS-001.4:** The review shortcut, Mermaid error toast, orphan swimlane title, and developer demo/QA page chrome are localized. Agent-message fixture payloads remain intentionally untranslated.
- **AC-PLATFORM-I18N-SECOND-AUDIT-GAPS-001.5:** Newly migrated files are added to `i18nGuardFiles` where they are not already covered. Translation lookup never occurs at module scope, and plural copy uses `count` with plural catalog entries.
- **AC-PLATFORM-I18N-SECOND-AUDIT-GAPS-001.6:** **GIVEN** a review, plan, file, task, session, movement, feedback, or synchronization operation succeeds or fails, **WHEN** the UI reports the result, **THEN** owned fallback copy uses the active locale while server/domain error details remain unmodified.
- **AC-PLATFORM-I18N-SECOND-AUDIT-GAPS-001.7:** **GIVEN** Azure DevOps or keyboard shortcut configuration is rendered, **WHEN** the locale changes, **THEN** display labels update without changing stored IDs, values, bindings, WIQL, or prompts.
- **AC-PLATFORM-I18N-SECOND-AUDIT-GAPS-001.8:** **GIVEN** an orphaned workflow task is rendered, **WHEN** the synthetic swimlane appears, **THEN** its title is localized while `__kandev_orphan__` remains the stable sentinel and is never offered as a move destination.

## Migrated source detail

## Why

The second localization audit found English display copy that escapes JSX literal linting because it
lives in hooks, plain-object configuration, helper return values, toast payloads, or developer demo
pages. These leaks make locale switching and the pseudo-locale incomplete even though the guarded
JSX migration is complete.

## What

- Review findings, task plans, file operations, task/session actions, task movement, task creation,
  sidebar synchronization, walkthrough requests, GitHub/GitLab feedback, and related production
  operations resolve user-facing success and failure copy from catalogs when the operation runs.
- Azure DevOps configuration retains WIQL, IDs, provider/status values, and agent prompt templates as
  stable data while mode labels, pull-request statuses, default query labels, action labels, and hints
  are localized at render time.
- Configurable keyboard shortcuts retain stable IDs and bindings while Settings resolves catalog keys
  for display names at render time.
- The review shortcut, Mermaid error toast, orphan swimlane title, and developer demo/QA page chrome
  are localized. Agent-message fixture payloads remain intentionally untranslated.
- Newly migrated files are added to `i18nGuardFiles` where they are not already covered. Translation
  lookup never occurs at module scope, and plural copy uses `count` with plural catalog entries.

## Scenarios

- **GIVEN** a review, plan, file, task, session, movement, feedback, or synchronization operation
  succeeds or fails, **WHEN** the UI reports the result, **THEN** owned fallback copy uses the active
  locale while server/domain error details remain unmodified.
- **GIVEN** Azure DevOps or keyboard shortcut configuration is rendered, **WHEN** the locale changes,
  **THEN** display labels update without changing stored IDs, values, bindings, WIQL, or prompts.
- **GIVEN** an orphaned workflow task is rendered, **WHEN** the synthetic swimlane appears, **THEN**
  its title is localized while `__kandev_orphan__` remains the stable sentinel and is never offered as
  a move destination.
- **GIVEN** either developer demo page is rendered, **WHEN** controls, headings, counts, empty/error
  states, or accessibility names appear, **THEN** page chrome is localized while intentional agent
  fixture content remains verbatim.
- **GIVEN** the affected source trees are swept, **WHEN** remaining candidates are reviewed, **THEN**
  every untranslated candidate is documented as domain data, diagnostics, product/code text, or an
  agent-facing prompt rather than silently ignored.

## Out Of Scope

- Watcher labels, the unknown-task fallback, and orphan catalog cleanup owned by task
  `0e0b8b61-35f9-49fa-9bcd-2cab7484510c`.
- Core Voice Mode copy removed from main by `#2576`; voice input is now plugin-owned.
- Translating console diagnostics, raw server errors, identifiers, product names, code examples,
  WIQL, provider/status values, branch/repository data, or agent-facing prompt templates.
- Layout, navigation, touch behavior, viewport branching, or other mobile composition changes.
- Translating fixture payload content whose purpose is to demonstrate agent messages.
