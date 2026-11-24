---
status: active
system: integrations
created: 2026-11-17
owners:
  - kandev
---

# GitHub Repository Scope Error Requirements

## Overview

Workspace settings can limit GitHub operations to selected organizations or repositories. For branch-list and merge-method requests, Kandev must stop before it resolves credentials or calls GitHub when the requested repository falls outside this scope. The caller needs a clear response, but the response must not expose the requested repository identity or the configured allowlist.

## Requirements

### REQ-INTEGRATIONS-GITHUB-REPOSITORY-SCOPE-ERRORS-001: Safe local scope rejection

**Intent:** Report a local repository-scope rejection without exposing workspace policy details.

#### Acceptance criteria

- **AC-INTEGRATIONS-GITHUB-REPOSITORY-SCOPE-ERRORS-001.1:** For GitHub branch-list and merge-method requests, Kandev shall check the requested repository against workspace scope before credential resolution or a GitHub request. A rejected request shall resolve no credential and make no GitHub request.
- **AC-INTEGRATIONS-GITHUB-REPOSITORY-SCOPE-ERRORS-001.2:** A local scope rejection shall have a dedicated service error. It shall also retain the existing not-resolvable classification that stops repeated background attempts.
- **AC-INTEGRATIONS-GITHUB-REPOSITORY-SCOPE-ERRORS-001.3:** The branch-list and merge-method endpoints shall return HTTP 404 with code `github_repository_out_of_scope` for a local scope rejection. The response shall direct the user to workspace Settings. It shall not include the requested repository identity or configured allowlist. An upstream not-found result shall keep its separate response code.

## System design

See the [GitHub repository scope error design](../system-design/github-repository-scope-errors.md).

## Implementation plan

See the [delivery plan](../../../plans/github-repository-scope-errors/plan.md).
