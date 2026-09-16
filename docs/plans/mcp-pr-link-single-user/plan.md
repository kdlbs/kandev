---
created: 2026-09-15
status: implemented
requirements:
  - REQ-INTEGRATIONS-TASK-CHANGE-LINK-MCP-001
system_design:
  - ../../specs/integrations/system-design/task-change-link-mcp.md
legacy_specs: []
---

# Implementation Plan: Single-user MCP PR linking

## Overview

Restore GitHub PR association from in-session MCP when authentication is
disabled. One sequential work order covers the host identity resolution,
coordinator, and regression tests. The existing active requirement already
requires authorized mutations to succeed; no new product requirement is needed.
Integrations owns the repair because its association operation consumes the
identity; generic authentication behavior stays unchanged.

## Evidence and root cause

At 2026-09-15T01:35:43Z, task d899fc6e-f75b-440a-942c-379da3a2f22a
called `link_task_pr_kandev` for GitHub PR 3676 with a valid attached canonical
repository ID. Message 39bf555c-c484-449d-9085-87c53c392f06 records
`authenticated user identity is required for GitHub PR links`.
The running backend reported auth mode `disabled`; HTTP returned `default-user`.
`scope.Resolver.Scope` intentionally leaves disabled-mode MCP context unscoped,
but `taskChangeLinkCoordinator.link` rejects that context before provider I/O.

## Scope

### In scope

- Trusted single-user identity resolution for GitHub link and replacement.
- Preserve real identities, denied callers, repository reach, and rollback.
- Regression evidence through the real MCP handler and coordinator.

### Out of scope

- Global MCP identity changes, auth configuration, credential routing, schemas,
  GitLab behavior, rendered UI, and live deployment or retrying the user's PR.

## Technical approach

Follow the [identity design](../../specs/integrations/system-design/task-change-link-mcp.md#github-caller-identity).
Inject a narrow trusted identity resolver into `taskChangeLinkCoordinator`
from `helpers.go`. Host composition consults `auth.Service.Mode()` and reuses
`httpmw.SyntheticIdentity()` only for explicitly disabled auth. A nil auth
service or missing resolver fails closed for identity-free GitHub requests.
Preserve existing identities without replacement. Reject a present empty identity.
Use the resolved user ID with `AssociateExistingPRByURLForWorkspace`; keep the
original request context for task authorization and existing provider routing.

## Tests

- `.1`: `TestTaskChangeCoordinatorGitHubIdentityModes` covers disabled,
  enabled, setup, unavailable, existing real, and malformed identity cases.
- `.1`, `.2`: `TestTaskChangeLinkMCPSingleUser` dispatches the actual MCP action
  with a server-derived principal through real handlers and coordinator,
  using disposable SQLite and a fake provider. Assert the canonical returned
  association and the provider's workspace/user identity. Reject forged caller,
  foreign workspace, and unattached repository without provider calls.
- `.3`, `.4`: `TestTaskChangeCoordinatorGitHubReplacementSingleUser` covers
  successful replacement, same-link no-op, and failed incoming link preserving
  the old association. Existing rollback tests continue to pass.
- Run the existing scope tests to ensure disabled-mode generic MCP stays
  unscoped and enforced-mode owner failures remain denied.

## End-to-end evidence

The MCP-to-provider-boundary test covers the affected agent operation without
an artificial browser test or live GitHub mutation. It must use the production
host identity resolver, not inject a synthetic identity directly into the
test context. Provider network and persistence behavior are unchanged.

## Work orders

- [x] [Task 01: Restore single-user GitHub linking](task-01-restore-single-user-linking.md)

## Verification results

Implementation completed on 2026-09-15. The original MCP regression failed
before the correction and passed afterward. Final targeted Go checks passed:

- `(cd apps/backend && go test ./internal/backendapp -run 'Test(TaskChange|GitHubChangeURL)' -count=1)` (8.174s).
- `(cd apps/backend && go test ./internal/mcp/handlers -run 'Test(TaskChange|ValidateTaskChange)' -count=1)` (0.924s).
- `(cd apps/backend && go test ./internal/mcp/scope -count=1)` (0.055s).

See the [work-order results](task-01-restore-single-user-linking.md#results)
for Red/Green evidence and fixture correction. Production code matches the
existing system design's operation-local identity resolution. Document checks:

- `python3 scripts/list-docs.py validate`: passed, 269 decisions and 923 specs.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `python3 scripts/lint-spec-files.test.py`: passed, 36 tests.
- `git diff --check`: passed.
- `git status --short -- docs/plans/mcp-pr-link-single-user`: confirmed the
  new package is present and untracked; no commit or publication performed.

## Risks

An unconditional default-user fallback could bypass multi-user ownership.
The fallback must require explicitly disabled auth and preserve all existing
request reach checks. Existing GitHub credential failures remain errors.

## Documentation impact

Internal design and delivery documents updated. Implementation restores the
documented tool contract without new operator steps or arguments. Changes are
local and uncommitted; no live instance or upstream PR was modified.
