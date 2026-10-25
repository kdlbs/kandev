---
id: "01-add-server-repository-inspection"
title: "Add server repository inspection"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLUGINS-REPOSITORY-TASK-CREATION-001
acceptance_criteria:
  - AC-PLUGINS-REPOSITORY-TASK-CREATION-001.2
  - AC-PLUGINS-REPOSITORY-TASK-CREATION-001.4
  - AC-PLUGINS-REPOSITORY-TASK-CREATION-001.5
system_design:
  - ../../specs/plugins/system-design/repository-provider-task-creation.md
---

# Task 01: Add Server Repository Inspection

## Summary

Add a standardized backend action that asks the active manifest owner to turn
an untrusted repository URL into a validated, credential-free descriptor.
Extend the packaged fixture to implement the same contract.

## In scope

- Start with failing plugin-service tests for success, no match, missing
  ownership, wrong action scope, timeout, oversized or malformed output,
  provider mismatch, credential-bearing URLs, and clone-origin mismatch.
- Add the `repositories.inspect` action key, request and response models,
  action deadline, and response bounds beside `repositories.branches`.
- Resolve the plugin record only through active manifest provider ownership.
- Accept the preferred nested repository response and the compatible direct
  descriptor response.
- Return typed failures suitable for task transport mapping.
- Add deterministic fixture action declarations and handlers for repository
  inspection and stored-repository branch listing.
- Keep fixture frontend and backend provider identity, scope, immutable ID,
  owner, name, clone URL, and default branch identical.

## Out of scope

- Task-service invocation or write ordering.
- Task-create HTTP and WebSocket error mapping.
- External Bitbucket plugin changes.
- Browser-invoked repository inspection.

## Acceptance

- The active manifest owner receives one bounded workspace-scoped inspect
  request containing only the untrusted URL and verified action context.
- A valid direct or nested response becomes one normalized authoritative
  descriptor.
- Invalid ownership, action declarations, response content, or origin fails
  closed with no credential or raw response leakage.
- The fixture package passes manifest validation and returns descriptors that
  satisfy the production parser.

## Verification

Create the failing parser and invocation tests first. Confirm at least one
fails because no inspect action exists. Then run:

```bash
# From apps/backend:
rtk go test ./internal/plugins -run 'RepositoryProvider.*Inspect|InspectRepositoryProvider' -race
rtk go test ./cmd/plugin-fixture -run 'Repository|Action|FixturePackage' -race
```

## Files likely touched

- `apps/backend/internal/plugins/repository_provider_inspect.go`
- `apps/backend/internal/plugins/repository_provider_inspect_test.go`
- `apps/backend/cmd/plugin-fixture/plugin.go`
- `apps/backend/cmd/plugin-fixture/main_test.go`
- `apps/backend/cmd/plugin-fixture/fixture_package_test.go`
- `apps/backend/cmd/plugin-fixture/fixture-package/manifest.yaml`

## Dependencies

None.

## Risks

- Treating the request URL as an allowed credential origin would break the
  security boundary. The plugin must establish connection ownership first.
- Supporting two response envelopes can become open-ended. Limit compatibility
  to the documented direct and nested repository shapes.
- Error strings from provider APIs can contain sensitive URLs or account data.
  Categorize and sanitize at the plugin action boundary.

## Parallelism

`sequential`

## Inputs

- `REQ-PLUGINS-REPOSITORY-TASK-CREATION-001`.
- `docs/specs/plugins/system-design/repository-provider-task-creation.md`.
- `docs/decisions/2026-07-31-authenticated-plugin-actions.md`.
- Existing `repositories.branches` implementation and tests.

## Results

Implemented `repositories.inspect` with active manifest ownership, verified
workspace action context, direct and nested response compatibility, bounded
response parsing, descriptor and clone-origin validation, immutable hint
checks, timeout handling, redirect rejection, and safe typed errors. The
fixture package now declares and serves deterministic inspect and branch
actions.

Verification passed:

```text
go test ./internal/plugins -run 'RepositoryProvider.*Inspect|InspectRepositoryProvider' -race
go test ./cmd/plugin-fixture -run 'Repository|Action|FixturePackage' -race
```
