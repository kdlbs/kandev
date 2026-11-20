---
created: 2026-11-17
status: complete
requirements:
  - REQ-INTEGRATIONS-GITHUB-REPOSITORY-SCOPE-ERRORS-001
system_design:
  - ../../specs/integrations/system-design/github-repository-scope-errors.md
legacy_specs: []
---

# Implementation Plan: GitHub Repository Scope Errors

## Overview

Return a safe, actionable error when workspace GitHub scope rejects a branch-list or merge-method request. Keep the rejection before credential resolution and provider calls. Preserve the existing retry classification for background callers.

## Design summary

Workspace settings own the scope. For branch-list and merge-method requests, the GitHub service checks the requested repository before it resolves credentials or calls GitHub. A typed service error preserves the existing not-resolvable classification. Both endpoints return the same HTTP 404 response with code `github_repository_out_of_scope`.

The response directs the user to workspace Settings. It omits the requested repository identity and the configured allowlist. An upstream repository lookup failure keeps its separate response code.

## Work order

- [Task 01: Reject out-of-scope GitHub repositories](task-01-reject-out-of-scope-repositories.md)

## Verification

`go test -trimpath ./internal/github` passed. The specification catalog validated 455 decisions and 2,032 specifications. The full specification lint passed.

The PR documentation evaluator returned `covered` for the changed source paths and this delivery package. `git diff --check` passed, and the tracked Go error file has no conflict markers.
