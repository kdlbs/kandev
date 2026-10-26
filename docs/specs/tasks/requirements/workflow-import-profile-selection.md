---
status: draft
system: tasks
created: 2026-09-17
owners:
  - kandev
---

# Workflow import profile selection

## Overview

Users can import workflows whose step profiles do not exist on the destination installation.
They choose an existing replacement profile for each affected step before import.
The task system owns this behavior because it owns workflow definitions and their session targets.
The agent system continues to own profile configuration and eligibility.
Name selection is the shared prerequisite for browser and noninteractive one-time imports.

## Terminology

- **Missing profile:** An explicit step profile descriptor without an eligible exact local match.
- **Replacement:** An existing local profile that the user selects for one missing profile.
- **Eligible profile:** An enabled global profile available to the importing user under existing profile access rules.

## Requirements

### REQ-TASKS-IMPORT-PROFILES-001: Resolve missing step profiles before import

**Intent:** Make imported steps usable without requiring manual YAML edits.

#### Acceptance criteria

- **AC-TASKS-IMPORT-PROFILES-001.1:** When a manual browser import has missing profiles, Kandev shall show every affected workflow and step before creation.
  Each entry shall show the requested agent, model, and mode and a searchable profile picker.
- **AC-TASKS-IMPORT-PROFILES-001.2:** When a step has an eligible exact match, Kandev shall retain that match without requiring user selection.
  When all explicit step profiles match, import shall proceed without an additional selection screen.
- **AC-TASKS-IMPORT-PROFILES-001.3:** Each missing profile shall require an explicit selection before import becomes available.
  Users can select different replacements for steps with identical requested profiles.
  The picker shall offer eligible profiles across agent families and identify each profile by name, agent, model, and mode.
- **AC-TASKS-IMPORT-PROFILES-001.4:** Kandev shall preserve initial-session targets, earlier-step targets, prompts, events, and session lifecycle settings after profile selection.
  A step without an explicit profile descriptor shall not require a replacement.
  Selecting a replacement for Implement shall preserve PR's reference to Implement.
- **AC-TASKS-IMPORT-PROFILES-001.5:** When any replacement is unresolved or invalid, Kandev shall create no workflows from that selection submission.
  Disabled, removed, inaccessible, or workspace-scoped profiles shall not become replacement bindings.
  A profile change after selection shall require another selection or review before import.
- **AC-TASKS-IMPORT-PROFILES-001.6:** When no eligible profiles exist, Kandev shall explain the problem and provide access to profile settings and a retry action.
  Cancellation shall create nothing. Recoverable errors shall preserve the YAML and valid selections.
  A YAML or destination change shall invalidate the previous selection review.
- **AC-TASKS-IMPORT-PROFILES-001.7:** Workflows skipped under existing name deduplication shall not require profile selections.
  For each supported document, Kandev shall process workflow occurrences in document order using exact, case-sensitive names without trimming.
  Every occurrence whose name already exists in the destination workspace, including hidden workflows, shall be skipped.
  Otherwise, the first occurrence shall be retained and every later occurrence of that name shall be skipped.
  Preview shall report only retained occurrences' explicit step profiles, using their original document indexes and step positions.
  A missing profile on a skipped occurrence shall not block a retained occurrence's import.
  Kandev shall identify repeated step names by their workflow and position.
- **AC-TASKS-IMPORT-PROFILES-001.8:** Desktop and phone users shall complete the same selection and import flow.
  The phone surface shall provide internal scrolling, visible navigation, safe-area spacing, and touch targets of at least 44 px.
  Neither surface shall cause document horizontal overflow. Keyboard users shall select profiles and return to the initiating control after dismissal.
- **AC-TASKS-IMPORT-PROFILES-001.9:** Loading and retry states shall have localized status text.
  Duplicate submissions shall remain unavailable while an import request is pending.
  After success and reload, each imported step shall retain its selected profile and session targets.
- **AC-TASKS-IMPORT-PROFILES-001.10:** Browser preview, browser submission, raw-YAML API import, and MCP import shall use the same workflow-name selection rule for the same document and destination state.
  On successful submission, `created` shall contain each retained occurrence's name once, and `skipped` shall contain every skipped occurrence's name, each list in document order.
  Skipped occurrences shall create no workflows or steps and shall not modify existing or earlier retained definitions.
  Distinct exact names shall remain independently importable, including names differing only in case or surrounding whitespace.
- **AC-TASKS-IMPORT-PROFILES-001.11:** All one-time import paths shall retain structural validation of the entire supported document before name selection can lead to creation.
  A malformed skipped occurrence shall still reject the document without creating workflows or steps.
  Browser submissions shall retain structural binding validation, including for supplied bindings on skipped occurrences; missing selections or unavailable profiles on skipped occurrences shall not require resolution.

## Compatibility and exclusions

This change covers explicit step profiles in manual browser imports of supported workflow exports.
The shared name-selection rule also covers the raw-YAML API and MCP imports.
Their profile matching and failure contracts otherwise retain their behavior.
The browser uses an explicit selection submission contract.
Workflow-default profiles, review-action profiles, conditional session rules, and unattended workflow sync retain their existing behavior.
Inline profile creation, profile configuration changes, runtime model fallback, and automatic replacement selection are outside this capability.
The capability does not create agent sessions or change existing workflows.
Name selection is per document. Concurrent-request uniqueness, global name constraints,
schema changes, and choosing a later occurrence when the first retained occurrence fails are outside this capability.

## Related contracts

- [Workflow session recipients](workflow-profile-session-lifecycle.md)
- [Profile disable behavior](../../agents/requirements/profile-disable.md)
- [System design](../system-design/workflow-import-profile-selection.md)
