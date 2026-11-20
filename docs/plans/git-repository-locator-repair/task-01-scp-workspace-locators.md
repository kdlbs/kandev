---
id: "01-scp-workspace-locators"
title: "Admit conventional SCP workspace locators"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-WORKSPACES-REPOSITORY-LOCATORS-001
acceptance_criteria:
  - AC-WORKSPACES-REPOSITORY-LOCATORS-001.1
  - AC-WORKSPACES-REPOSITORY-LOCATORS-001.2
  - AC-WORKSPACES-REPOSITORY-LOCATORS-001.3
  - AC-WORKSPACES-REPOSITORY-LOCATORS-001.4
  - AC-WORKSPACES-REPOSITORY-LOCATORS-001.5
system_design:
  - ../../specs/workspaces/system-design/repository-locators.md
---

# Task 01: Admit Conventional SCP Workspace Locators

## Summary

Repair #5182 independently of registration editing.
Add the strict conventional SCP parser and call it before URL parsing in agentctl.
Prove materialization, removal, and multi-repository compensation across real handler/client boundaries.

## In scope

- Read the current main and #4934 heads again before implementation. Reconcile any changed validator.
- Add a neutral, pure `common/gitlocator` parser for the complete grammar in the design.
- Preserve existing network schemes, credential refusal, classified Git execution, and destination containment.
- Reject malformed `git@` forms finally, including empty path/host, traversal, extra separators, whitespace, controls, queries, and fragments.
- Keep removal-only local-origin recognition without admitting local clones.
- Add a real route fixture using temporary Git `insteadOf` settings. Keep the submitted and recorded origin in SCP form.
- Add lifecycle integration using the real HTTP client and agentctl routes, including created versus reused entries and failed rollback.
- Add the isolated Kubernetes smoke described in the plan, derived from existing Kind/SSH fixture patterns.

## Out of scope

- Registration editing, provider discovery, managed credential redesign, stored URL normalization, or task-model changes.
- Production fixture exceptions, E2E-profile file-URL support from #4934, and SCP IPv6/port extensions.
- Permanent test or production edits until the user explicitly requests implementation of this package.

## Acceptance

1. Accepted SCP examples reach real materialization and matching removal. Every hostile table case fails before Git/filesystem mutation without echoing its locator.
2. A two-repository launch prepares its secondary before startup. Failed later preparation removes new matching entries and preserves reused/unrelated checkouts.
3. Existing single-repository, credential, local-locator, containment, and quarantine controls pass. The remote smoke records its own actual result.

## TDD and verification

First add `TestWorkspaceSCPLocatorMaterializeAndRemove` and `TestRemoteWorkspaceSCPLocators` against current behavior.
Record their expected RED at the locator gate before correcting it.
Use a separate new test file because `workspace_materialize_test.go` is already large.

Run this block from the repository root after implementation:

```bash
(cd apps/backend && go test -trimpath ./internal/common/gitlocator ./internal/agentctl/server/api -run '^(TestParseConventionalSCP|TestWorkspaceRepositoryLocatorRejections|TestWorkspaceSCPLocatorMaterializeAndRemove|TestWorkspaceMaterializeRepository_.*|TestWorkspaceRemoveMaterializedRepository_.*)$' -count=1)
(cd apps/backend && go test -trimpath -race ./internal/agent/runtime/lifecycle -run 'Test(RemoteWorkspaceSCPLocators|MaterializeWorkspaceRepositories|RemoteWorkspaceProjection)' -count=1)
(cd apps/backend && go test -trimpath ./internal/gitcredentials -run '^TestResolveRepositoryIdentity' -count=1)
bash scripts/lint-go-changed apps/backend/internal/common/gitlocator/scp.go apps/backend/internal/common/gitlocator/scp_test.go apps/backend/internal/agentctl/server/api/workspace_materialize.go apps/backend/internal/agentctl/server/api/workspace_materialize_locator_test.go apps/backend/internal/agent/runtime/lifecycle/workspace_materialization_scp_test.go
```

The separate remote smoke requires Docker, Kind, and the existing pinned Kubernetes tools/images.
Run the dependency installation once per new worktree, then the guarded runner:

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && KANDEV_E2E_CONTAINERS=1 pnpm e2e:run --project containers tests/kubernetes/kubernetes-scp-multi-repository.spec.ts)
```

Keep one worker per shard. Do not overlap full suites or alter an installed runtime to obtain evidence.
If infrastructure is unavailable, record the exact blocker and leave the remote-smoke gate unexecuted.
Do not infer remote success from the local `insteadOf` fixture.

## Files likely touched

- `apps/backend/internal/common/gitlocator/scp.go` and `scp_test.go` (new)
- `apps/backend/internal/agentctl/server/api/workspace_materialize.go`
- `apps/backend/internal/agentctl/server/api/workspace_materialize_locator_test.go` (new)
- `apps/backend/internal/agent/runtime/lifecycle/workspace_materialization_scp_test.go` (new)
- `apps/web/e2e/tests/kubernetes/kubernetes-scp-multi-repository.spec.ts` (new)
- `apps/web/e2e/fixtures/ssh-image.ts`, `kubernetes-test-base.ts`, or a narrowly scoped new SSH Git fixture helper if required

## Dependencies

None. This work order can ship without the #5188 feature.
The existing checkout, rollback, credential, and subprocess boundaries are inputs, not replacement targets.

## Risks

- A broad SSH converter is not a security parser. Do not use successful conversion as admission.
- Local fixture rewrites can change the stored origin. Assert the exact request/origin and restore it only inside the disposable fixture when needed.
- The Kind smoke needs a `git` user and isolated SSH credentials. Existing SSH executor fixtures use another user.
- #4934 touches the same validator. It does not fix SCP and is not a dependency.

## Parallelism

`sequential`

## Inputs

- [Plan evidence and compatibility](plan.md)
- [Locator requirements](../../specs/workspaces/requirements/repository-locators.md)
- [Locator grammar, admission, and rollback design](../../specs/workspaces/system-design/repository-locators.md)
- Existing `workspace_materialize_test.go`, `workspace_materialize_credentials_test.go`, and `workspace_materialization_test.go`
- Root, backend, agentctl, and API `AGENTS.md`; `/tdd` backend guidance and `/e2e` fixture/resource rules

## Results

Implemented on 2026-11-08.

- `internal/common/gitlocator` now owns the strict conventional SCP parser.
  `validateRepositoryLocator` applies it before `url.Parse` and refuses every malformed `git@` form finally.
- `TestWorkspaceRepositoryLocatorRejections`, `TestWorkspaceRepositoryLocatorAcceptsNetworkAndSCP`,
  `TestWorkspaceSCPLocatorMaterializeAndRemove`, and `TestWorkspaceSCPLocatorRejectionDoesNotEchoOrMutate` pass.
- `TestRemoteWorkspaceSCPLocators` crosses the real agentctl HTTP server and client, materializes a secondary SCP
  checkout before startup, preserves a reused checkout, and rolls back only newly created checkouts after a later failure.
- The Kubernetes multi-repository smoke is not present. The E2E fixture cannot register an arbitrary SCP host as a
  provider identity without a new provider connection or a direct database seed. This is an unexecuted container gate,
  not a passing result.
