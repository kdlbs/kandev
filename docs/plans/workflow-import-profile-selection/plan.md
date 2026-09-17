---
created: 2026-09-17
status: complete
requirements:
  - REQ-TASKS-IMPORT-PROFILES-001
system_design:
  - ../../specs/tasks/system-design/workflow-import-profile-selection.md
legacy_specs: []
---

# Implementation plan: Workflow import profile selection

## Overview

Add a profile selection stage to manual workflow import.
Deliver one vertical work order covering the API, desktop/phone interaction, and persistence evidence.
Start with failing service and client tests, implement the selection flow, then run focused browser checks.

## Scope

### In scope

- Preview missing direct step profiles without persistence.
- Select existing replacements independently and preserve exact profile IDs.
- Preserve session targets and existing name deduplication.
- Validate the complete selection batch before writes.
- Provide desktop and phone selection, recovery, localization, and public documentation.

### Out of scope

- Workflow-default or review-action profile remapping.
- Unattended sync changes, inline profile creation, and runtime model selection changes.
- Changing legacy raw-YAML or MCP import behavior.
- New database transactions, migrations, or agent sessions.

## Technical approach

Add a read-only preview route beside `httpImportWorkflows`.
Extend that import handler with a JSON envelope for exact bindings while preserving its YAML path.
Add a context-aware profile catalog through backend service wiring.
The workflow service prepares and validates the selected batch before persistence.
It must preserve `sync_apply.go` behavior when extracting helpers from `service.go`.

Extract import state from `useWorkflowImportExport` into `use-workflow-import.ts` beside the current client.
Extend `ImportWorkflowsDialog` through focused components rather than growing the settings client.
Use the existing profile row style, with a Dialog on desktop and one navigable Drawer on phones.
The [system design](../../specs/tasks/system-design/workflow-import-profile-selection.md) defines contracts and recovery rules.

## ASCII UI preview

Entry point: Settings > Workspace > Workflows > Import.
Current behavior: the reported YAML produces a toast and closes no successful import.
Proposed behavior: the same input opens UI-01 with one unresolved step.

### UI-01: Desktop, unresolved profile

```text
+------------------------------------------------------------+
| Choose agent profiles                                    X |
| Some step profiles are not available on this installation.  |
|------------------------------------------------------------|
| Feature                                                    |
| Implement                                                  |
| Requested: Codex / gpt-5.6-luna / agent-full-access          |
| Agent profile: [Choose a profile                       v]  |
|                                                            |
| Other missing steps appear here, grouped by workflow.       |
|------------------------------------------------------------|
| [Back] [Cancel]                         [Import (disabled)] |
+------------------------------------------------------------+
```

### UI-02: Phone, unresolved profile

```text
+----------------------------------+
| < Back     Choose profiles      X |
|----------------------------------|
| Feature                          |
| Implement                        |
| Requested agent: Codex           |
| Model: gpt-5.6-luna               |
| Mode: agent-full-access          |
| [Choose a profile             >] |
|                                  |
| Other missing steps              |
| (one scrolling body)             |
|----------------------------------|
| [Import (disabled)]              |
|          safe-area space         |
+----------------------------------+
```

### UI-03: Phone, profile picker subview

```text
+----------------------------------+
| < Back       Implement           |
| [Search profiles               ] |
|----------------------------------|
| (logo) Implementation profile    |
| Codex / available model / mode   |
|                                  |
| (logo) Another profile           |
| Another agent / model / mode     |
| (one scrolling candidate list)   |
+----------------------------------+
```

UI-01 and UI-02 keep header/footer fixed. UI-03 replaces the body within the same phone surface.
Selecting a profile returns to UI-02. The Import button becomes enabled after every required selection.
An empty list shows “No available profiles”, a profile-settings link, and Retry.
A stale profile shows an inline error on its step. YAML and valid selections remain available.
Loading has a status region. Final submission disables repeated actions and dismissal.

Control order, grouping, navigation, scroll ownership, and action availability are required.
Spacing and example labels are illustrative. All UI text uses localization keys.
These views cover AC-TASKS-IMPORT-PROFILES-001.1, .3, .6, .8, and .9.

## Tests

| Criteria | Evidence |
| --- | --- |
| 001.1-.4, .7 | `import_profile_selection_test.go`: preview, exact matches, independent step replacements, and PR target preservation |
| 001.5-.7 | Same service suite: deleted/disabled/changed profiles, invalid mappings, skipped names, and zero writes on validation failure |
| 001.1, .5 | `import_profile_selection_test.go` in handlers: authorization, request limits, content types, and conflict response |
| 001.6, .9 | `use-workflow-import.test.ts`: generations, draft retention, retries, and duplicate submissions |
| 001.2, .4 | Existing service import/sync tests remain unchanged in behavior |

All identifiers have prefix `AC-TASKS-IMPORT-PROFILES-`.

## E2E tests

- Extend `e2e/tests/workflow/workflow-import-export.spec.ts` in `chromium`.
  Import the supplied six-step Feature structure with an unavailable descriptor, select a replacement, reload, and inspect Implement and PR.
  Cover multiple unresolved steps, exact-match bypass, cancellation, and recoverable errors.
- Add `e2e/tests/workflow/mobile-workflow-import-export.spec.ts` in `mobile-chrome`.
  Import by paste, use UI-03 through touch, return to UI-02, submit, reload, and inspect saved values.
  Cover long lists, internal scrolling, no horizontal overflow, safe footer placement, focus return, and 44 px targets.
- Retain existing upload, paste, invalid YAML, and duplicate-name scenarios.

## Work orders

- [x] [Task 01: Implement import profile selection](task-01-import-profile-selection.md)

## Verification results

Design validation on 2026-09-17:

- `python3 scripts/list-docs.py validate`: passed, 990 specifications and 285 decisions.
- `python3 scripts/lint-spec-files.test.py`: passed, 36 tests.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `git diff --check -- docs/specs docs/plans/workflow-import-profile-selection`: passed.
- `git status --short -- docs/specs docs/plans/workflow-import-profile-selection`: all four new artifacts are present and uncommitted.

Product verification completed on 2026-09-17:

- Backend focused workflow, controller, handler, and profile-catalog tests passed.
- Focused frontend Vitest tests, typecheck, ESLint, and i18n checks passed.
- Desktop Chromium workflow import E2E passed with 8 tests.
- Mobile Chromium workflow import E2E passed with 1 test.
- Public-doc validation, specification validation and lint, and `git diff --check` passed.

Review fixup verification also covers empty-catalog HTTP serialization,
rendered desktop and phone empty/loading states, deferred cancellation and
reopen, late file-read results, and in-flight edit/repeated-submit protection.

## Risks

- Import and sync share conversion helpers. Interactive checks must remain specific to the new API path.
- Profile edits can occur after preview. Final selection validation must use profile revisions and preserve exact selected IDs.
- A lost response can follow successful persistence. A retry must use existing workflow-name deduplication.
- Workflow-default and review-action profiles retain existing behavior and can remain unmatched outside this step picker.
