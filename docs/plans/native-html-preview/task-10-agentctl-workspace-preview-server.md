---
id: "10-agentctl-workspace-preview-server"
title: "Build the agentctl workspace preview server"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-NATIVE-HTML-PREVIEW-001
acceptance_criteria:
  - AC-UI-NATIVE-HTML-PREVIEW-001.4
  - AC-UI-NATIVE-HTML-PREVIEW-001.5
  - AC-UI-NATIVE-HTML-PREVIEW-001.11
system_design:
  - ../../specs/ui/system-design/native-html-preview.md
---
# Task 10: Build the agentctl workspace preview server

## Summary

Add the ephemeral static server and bounded current-buffer overlay to agentctl.
It must serve browser-ready HTML and relative workspace assets without exposing
paths outside the selected repository root.

## In scope

- Lazy per-root loopback listener creation, reuse, shutdown, and structured
  lifecycle logging.
- A concurrency-safe 32-entry overlay with 5 MiB per-document bounds,
  replacement versions, and least-recently-published eviction.
- Exact overlay serving, disk fallback, MIME types, no-store headers, normal
  404 responses, and method handling.
- Repository-aware canonical paths and traversal, encoded-traversal, and
  symlink-escape rejection.
- Agentctl publish route, request/response types, and unit tests.

## Out of scope

- Backend task-session routing.
- Frontend Browser-panel behavior.
- Browser-script isolation or content rewriting.

## Acceptance

- Publishing an unsaved entry document returns a stable live port for its
  workspace or repository root, a canonical path, and an increasing version.
  A request for that path returns the overlay.
- Relative and root-relative requests serve the correct workspace files with
  browser-appropriate content types.
- No request path or symlink can read outside the selected root.
- Bounds, eviction, concurrent publish/read, and shutdown behavior are tested.

## Verification

```bash
cd apps/backend
go test ./internal/agentctl/server/api
go test -race ./internal/agentctl/server/api
```

## Files likely touched

- `apps/backend/internal/agentctl/server/api/server.go`
- `apps/backend/internal/agentctl/server/api/workspace_preview.go`
- `apps/backend/internal/agentctl/server/api/workspace_preview_test.go`
- Agentctl instance lifecycle files that own API server shutdown.

## Dependencies

None.

## Risks

- URL and filesystem canonicalization can disagree after escaping or symlink
  resolution.
- A listener or overlay can outlive the agentctl instance if teardown ownership
  is incomplete.

## Parallelism

`sequential`

## Inputs

- Acceptance criteria `.4`, `.5`, and `.11`.
- Workspace preview manager, overlay bounds, trust, and lifecycle sections in
  the system design.

## Results

Implemented the agentctl-owned workspace preview server.

- Added one loopback static server per canonical workspace or repository root.
- Added bounded current-buffer overlays with replacement versions and oldest
  eviction across roots.
- Added MIME handling, no-store responses, GET and HEAD support, containment
  checks, traversal rejection, symlink escape protection, and teardown cleanup.
- Added focused tests for publishing, disk assets, replacement, bounds,
  eviction, traversal, symlinks, and shutdown.

Verification:

- `go test ./internal/agentctl/server/api` passed.
- `go test -race ./internal/agentctl/server/api` passed.
- `rtk make -C apps/backend test` passed with task-host internal config
  overrides cleared.
