---
id: "01-destination-model-and-github-resolution"
title: "Destination model and GitHub resolution"
status: complete
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/improve-kandev/spec.md"
---

# Task 01: Destination Model and GitHub Resolution

## Acceptance

- A strict versioned `contribution_destination` model round-trips through task-repository metadata without
  accepting secrets, malformed URLs, unsupported providers, target aliases, or unknown versions.
- The workspace GitHub service reports direct target write or returns one exact writable fork whose parent
  is the requested canonical repository; it creates and bounded-polls a missing human-owned fork.
- PAT and named CLI automation identities are supported. A GitHub App without direct target write fails
  with a typed configuration error and never borrows a personal or ambient host credential.

## Verification

```bash
cd apps/backend
rtk go test ./internal/task/models ./internal/github -run 'Test.*(ContributionDestination|ContributionFork|ForkDestination)'
```

## Files likely touched

- `apps/backend/internal/task/models/contribution_destination.go`
- `apps/backend/internal/task/models/contribution_destination_test.go`
- `apps/backend/internal/github/models.go`
- `apps/backend/internal/github/client.go`
- `apps/backend/internal/github/pat_client.go`
- `apps/backend/internal/github/gh_client.go`
- `apps/backend/internal/github/service.go`
- focused GitHub PAT, CLI, service, and fake-client tests

## Dependencies

None. Reuse credential-free repository validation and remote-name hashing from
`internal/task/models/remote_contribution.go` without weakening that existing binding.

## Parallelism

Sequential foundation. Tasks 02-03 consume the exact persisted and provider-authoritative shape.

## Inputs

- Spec: Improve Kandev creation-time publication route and fork failure modes.
- ADR: server-authored task binding, same automation identity, canonical target ownership.
- Existing patterns: `RemoteContributionRepository`, workspace auth resolution, PAT/CLI client adapters,
  and provider error sanitization.

## Risks

- A repository named `<login>/kandev` is not sufficient proof; compare parent provider identity and path.
- GitHub fork creation is asynchronous. Poll with a bounded context and retry only provider-defined
  not-ready states, not authorization or conflict failures.
- Do not widen the main GitHub client interface if a narrow capability interface keeps mocks and providers
  simpler.

## TDD sequence

1. Add failing model validation/round-trip and resolver behavior tests.
2. Implement the smallest domain helpers and provider capability required to pass.
3. Refactor shared credential-free repository validation without changing remote-contribution behavior.
4. Run both focused suites again with retries/caches disabled where applicable.

## Output contract

Report the binding schema, provider authority checks, actor limitations, files changed, red/green commands,
remaining risks, divergence, and task/plan status updates.

## Completion

Completed 2026-08-12. Added the versioned credential-free destination model and metadata helpers, stable
destination remote naming, GitHub repository/fork API capabilities for PAT, CLI, mock, and no-op clients,
and the workspace resolver. Direct target write is preferred; otherwise only a human automation identity
may create or adopt an exact `<login>/kandev` fork whose parent identity matches `kdlbs/kandev`. App
installations without direct target write fail closed with typed, sanitized errors.

Verification: `rtk go test ./internal/task/models ./internal/github -count=1` passed, including round-trip,
malformed-binding, fork-creation, parent-identity, polling, conflict, and actor-capability cases.
