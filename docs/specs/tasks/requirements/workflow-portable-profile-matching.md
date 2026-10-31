---
status: active
system: tasks
created: 2026-10-28
owners:
  - kandev
---

# Portable workflow profile matching

## Overview

Workflow authors need portable profile references to distinguish agent profiles
that use the same agent, model, and mode but different provider configuration.
The task system owns workflow definitions and their reconciliation. The agent
system owns profile configuration and eligibility.

## Requirements

### REQ-TASKS-PORTABLE-PROFILE-001: Resolve portable workflow profiles reliably

**Intent:** Preserve the author's intended profile across workflow export,
import, and repository synchronization without guessing on sync.

#### Acceptance criteria

- **AC-TASKS-PORTABLE-PROFILE-001.1:** A versioned workflow export shall carry each referenced workflow-default, direct step, and code-review profile's agent name, model, mode, and provider configuration options, including an explicit empty options set when none are configured. Import shall use those values to distinguish profiles.
- **AC-TASKS-PORTABLE-PROFILE-001.2:** Synchronization shall bind a descriptor only when exactly one eligible profile matches all values it specifies. When a descriptor has no match or multiple eligible matches, synchronization shall report the source, workflow, affected reference, and reason, and shall leave that entire existing workflow unchanged or refrain from creating the new workflow. Other valid workflows shall continue to synchronize.
- **AC-TASKS-PORTABLE-PROFILE-001.3:** An existing bound profile that still matches its descriptor shall remain bound when disabled or workspace-scoped. An option change that no longer matches shall not retain that binding as a match. When multiple eligible candidates remain, synchronization shall report ambiguity even if one is currently bound.
- **AC-TASKS-PORTABLE-PROFILE-001.4:** Existing version 1 and 2 files shall remain readable, with their absent options treated as unspecified. A uniquely matching legacy descriptor shall synchronize; an ambiguous or missing one shall follow the safe failure behavior above. A versioned option-aware export shall be rejected by older readers that do not support its version.
- **AC-TASKS-PORTABLE-PROFILE-001.5:** Browser import shall use option-aware matching and retain explicit replacement selection and revision checks for unmatched direct steps. Raw and MCP imports retain their existing handling of missing and duplicate matches, but must include specified options in their matching. Explicitly selected browser bindings shall survive submission unchanged.
- **AC-TASKS-PORTABLE-PROFILE-001.6:** Sync warnings shall be visible through the existing sync status surface and shall identify recovery by correcting the source descriptor or local profiles. Repository fetch failures or profile catalog read errors shall not be reported as successful profile matches.
- **AC-TASKS-PORTABLE-PROFILE-001.7:** Exported configuration options shall contain no credential fields or other profile settings. Documentation shall explain that option values are included in shared workflow files, legacy matching behavior, and recovery from unresolved references.

## Out of scope

- Creating or changing agent profiles, runtime model selection, and session routing.
- Changing the user's external workflow repository or its step assignments.
- Introducing an interactive picker for unattended synchronization.

## Related contracts

- [Import profile selection](workflow-import-profile-selection.md)
- [Profile disable behavior](../../agents/requirements/profile-disable.md)
- [System design](../system-design/workflow-portable-profile-matching.md)
