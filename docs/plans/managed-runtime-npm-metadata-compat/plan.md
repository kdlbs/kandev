---
spec: docs/specs/agents/requirements/runtime-updates.md
created: 2026-08-22
status: complete
---

# Implementation Plan: Managed Runtime npm Metadata Compatibility

## Overview

Normalize the version metadata returned by supported npm CLI versions before
building the managed-runtime catalogue. npm 12 wraps the result of a
multi-field `npm view` query in a one-element array, while earlier npm versions
return the object directly. The backend must accept both valid forms and keep
rejecting malformed or ambiguous metadata.

## Backend

### Version metadata resolver

- Update `apps/backend/internal/agent/settings/controller/agent_update.go`
  alongside `hostRuntimeUpdater.ResolveVersions` to accept the existing object
  response and npm 12's one-element array response.
- Normalize both forms to the existing `RuntimeVersionMetadata` contract. An
  empty or multi-entry array remains an error and cannot produce a catalogue.
- Preserve the existing exact npm argv, stable-version filtering, latest-tag
  validation, and preview error behavior.

## Tests

- **What:** A valid npm 12-shaped one-element array resolves to the same
  version catalogue and latest value as the object form.
  **File:** `apps/backend/internal/agent/settings/controller/agent_update_test.go`.
  **How:** Use the existing recording command executor and assert the returned
  metadata plus the exact direct npm argv.
- **What:** Existing preview and malformed-metadata behavior remains covered.
  **File:** `apps/backend/internal/agent/settings/controller/agent_update_test.go`
  and `apps/backend/internal/agent/settings/handlers/discovery_handlers_test.go`.
  **How:** Run the focused controller and handler packages after the regression
  test passes.

## Verification Results

- `cd apps/backend && go test -run 'TestHostRuntimeUpdaterResolves|TestHostRuntimeUpdaterRejectsAmbiguousNPMRuntimeMetadata' ./internal/agent/settings/controller`: 7 tests passed.
- `cd apps/backend && go test ./internal/agent/settings/controller ./internal/agent/settings/handlers`: 350 tests passed.
- `cd apps/backend && go test -race ./internal/agent/settings/controller ./internal/agent/settings/handlers`: 350 tests passed.
- `cd apps/backend && golangci-lint run ./... --new-from-rev="032ea05bc8997028b1690f5f351939c83a6f77c2" --timeout=5m`: no issues.
- `git diff --check`: passed. No temporary diagnostic state was created.

## Implementation Waves And Parallel Candidates

Wave 1, sequential:

- [x] [Task 01: Normalize npm version metadata](task-01-normalize-npm-version-metadata.md)

## Risks

- npm output is external-process data. The parser must not accept multiple
  metadata records or change the trusted package, registry, or command shape.
- This repair changes only catalogue resolution; it does not alter runtime
  selection, cache invalidation, or update activation.
