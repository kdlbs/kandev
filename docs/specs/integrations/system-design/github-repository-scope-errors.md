---
status: current
system: integrations
requirements:
  - REQ-INTEGRATIONS-GITHUB-REPOSITORY-SCOPE-ERRORS-001
created: 2026-11-17
owners:
  - kandev
---

# GitHub Repository Scope Error System Design

## Boundary and state ownership

Workspace GitHub settings own the repository scope. The GitHub service reads that scope for each operation. GitHub credentials and GitHub itself provide separate access checks. Kandev does not persist new state for a rejected request.

## Request flow

1. A branch-list or merge-method request reaches the GitHub controller.
2. The service reads the workspace scope and checks the requested repository.
3. If the repository is outside scope, the service returns a typed local error before credential resolution or a GitHub request.
4. The controller maps the error to HTTP 404 and code `github_repository_out_of_scope`.
5. The response directs the user to update the workspace scope in Settings. It omits the repository identity and the allowlist.

## Error contract

`repoOutsideScopeErr` matches both `ErrRepoOutsideWorkspaceScope` and `ErrRepoNotResolvable`. The dedicated sentinel lets HTTP handlers identify the local policy rejection. The existing not-resolvable classification keeps background callers from retrying it.

The public response identifies a scope rejection. It does not identify the rejected repository or show the workspace allowlist. An upstream missing or inaccessible repository uses code `github_repository_not_found`.

## Regression coverage

The service tests prove that a scope rejection matches both sentinels. Endpoint tests prove that branch-list and merge-method requests return the same safe response and do not resolve credentials before rejection.

## Related documents

- [Requirements](../requirements/github-repository-scope-errors.md)
- [Delivery plan](../../../plans/github-repository-scope-errors/plan.md)
