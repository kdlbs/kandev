---
id: "01-starter-files"
title: "Safe OKF starter files"
status: complete
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PROJECTS-CONTEXT-KNOWLEDGE-001
acceptance_criteria:
  - AC-PROJECTS-CONTEXT-KNOWLEDGE-001.1
  - AC-PROJECTS-CONTEXT-KNOWLEDGE-001.2
  - AC-PROJECTS-CONTEXT-KNOWLEDGE-001.3
  - AC-PROJECTS-CONTEXT-KNOWLEDGE-001.4
  - AC-PROJECTS-CONTEXT-KNOWLEDGE-001.5
system_design:
  - ../../specs/projects/system-design/context-knowledge.md
---

# Task 01: Safe OKF Starter Files

## Summary

Create an OKF index and structured notes for new context. Repeated provisioning
creates only missing defaults and preserves every existing file.

## In scope

- Use TDD for fresh, partial, custom, empty, concurrent, and unsafe roots.
- Replace the notes-only early return in `ContextStore.Provision`.
- Create the two design templates through existing safe handles and exclusive
  file creation, with current file and directory permissions.
- Retain existing write conflict, byte preservation, and path-containment tests.
- Update existing starter-content and directory-list assertions for the new index.

## Out of scope

- Startup or resume migration, metadata validators, database changes, and UI.
- Changes to context authorization, storage roots, file limits, or deletion.

## Acceptance

- A fresh project has the exact starter structure described in the design.
- Partial provisioning and concurrent calls preserve existing bytes, including
  empty/custom files, while completing missing defaults.
- Existing storage rejection and hash-conflict behavior continue to pass without
  format-dependent save errors.

## Verification

Run from the repository root:

```bash
(cd apps/backend && go test -race ./internal/projects)
git diff --check
```

## Files likely touched

- `apps/backend/internal/projects/context_store.go`
- `apps/backend/internal/projects/context_store_test.go`
- `apps/backend/internal/projects/service_test.go`, only if starter assumptions
  require updates.

## Dependencies

None.

## Risks

- Treating an empty file as absent or overwriting a concurrent create winner.
- Accidentally bypassing no-follow filesystem operations.
- Fixed directory-entry counts in existing tests omit the new index.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/projects/requirements/context-knowledge.md)
- [Starter files and compatibility design](../../specs/projects/system-design/context-knowledge.md#starter-files)
- [Format decision](../../decisions/2026-10-05-project-context-knowledge-format.md)
- `apps/backend/AGENTS.md` and `apps/backend/internal/projects/AGENTS.md`
- Existing `ContextStore` safe-handle and hash-conflict tests.

## Results

Implemented the index and structured notes starters through the existing
no-follow directory handle and exclusive file creation. Provisioning creates
each missing file independently, preserves existing regular files byte for byte
(including empty files), validates concurrent create winners, and rejects
unsafe starter entries. Updated service and directory-list expectations.

Verification on 2026-10-05:

- `cd apps/backend && go test ./internal/projects -run 'TestContextStoreProvision' -count=1`: passed after the implementation.
- `cd apps/backend && go test ./internal/projects -run 'TestProjectContextServiceUsesWorkspaceAuthorizationAndHashConflict' -count=1`: passed.
- `cd apps/backend && go test -race ./internal/projects`: passed.
- `git diff --check`: passed before the final service assertion update; rerun in final package checks.
