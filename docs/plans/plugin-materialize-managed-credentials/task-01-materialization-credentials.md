---
id: "01-materialization-credentials"
title: "Repair and verify materialization credentials"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-PLUGIN-002
acceptance_criteria:
  - AC-EXECUTORS-PLUGIN-002.5
  - AC-EXECUTORS-PLUGIN-002.6
  - AC-EXECUTORS-PLUGIN-002.7
system_design:
  - ../../specs/executors/system-design/remote-executor-plugins.md
---

# Task 01: Repair and verify materialization credentials

## Summary

Run repository materialization Git with the current instance authorization while
preserving installed credential tooling. Prove the repair through real Git and
credential-helper execution, then document private repository prerequisites.

## In scope

- Follow the paired design's repository materialization credentials section.
- Write the managed-credential regression first and record its expected failure.
- Thread one detached environment snapshot through both Git runners and all nested fetch/checkout paths.
- Cover helper-path preservation, instance isolation, revoked/no-lease behavior,
  malformed indexed config, unrelated config, denial sanitization, and noninteractive execution.
- Update `docs/public/plugins-authoring.md` under remote executor providers:
  workspace `task_git_credentials_mode=managed` permits host-issued credentials;
  fresh workspaces default to `executor`, which requires executor-owned access.
  Do not claim automatic host credentials or change defaults.

## Out of scope

Provider/bootstrap protocol changes, new credentials or flags, rendered UI,
production cloud provider deployment, and unrelated Git refactors.

## Acceptance

1. New `TestWorkspaceMaterializeRepository_ManagedCredentials` fails on the old
   code and passes after the fix with real authenticated clone/reuse-fetch and a
   broker/helper reached using instance-only credentials.
2. Every materialization Git path uses a detached current snapshot; helper tooling
   survives without reviving stale startup authorization, with existing public,
   cancellation, sparse checkout, and sanitization behavior preserved.
3. Public guidance explains both credential modes. Every command below passes,
   and PR creation, review/CI remediation, and merge follow repository skills
   under the user's existing delivery authorization after the design checkpoint.

## Verification

Run from the repository root. Install web dependencies once before Playwright.

```bash
(cd apps/backend && go test ./internal/agentctl/server/api -run '^TestWorkspaceMaterializeRepository_ManagedCredentials$' -count=1)
(cd apps/backend && go test ./internal/agentctl/server/api -count=1)
(cd apps/backend && go test ./internal/agentctl/server/process -run 'TestManager.*(GitEnvironment|ManagedGit|Configure)|TestWorkspaceGitEnvironment' -count=1)
(cd apps/backend && go test ./cmd/plugin-fixture -run '^TestPluginExecutorFixture' -count=1)
pnpm --dir apps install --frozen-lockfile
pnpm --dir apps/web e2e:run --host --shards 1 --project chromium tests/plugins/remote-executor-plugin.spec.ts
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
node scripts/validate-public-docs.mjs
git diff --check
```

Run `bash scripts/lint-go-changed` with the explicit changed production/test Go
paths before committing. Record that concrete command in Results after paths
are known. If new snapshot/helper tests have names outside the focused process
regex, extend the command to include them before declaring verification complete.
Do not run overlapping E2E suites or bypass the runner's resource guard.

## Files likely touched

- `apps/backend/internal/agentctl/server/api/workspace_materialize.go`
- `apps/backend/internal/agentctl/server/api/workspace_materialize_credentials_test.go` (new)
- `apps/backend/internal/agentctl/server/process/manager.go`
- `apps/backend/internal/agentctl/server/process/workspace_git_env_test.go`
- `docs/public/plugins-authoring.md`
- Paired executor requirement/design and this delivery package as needed for results.

## Dependencies

None. Implement sequentially in the primary conversation after the repository's
explicit implementation handoff. Read scoped backend, agentctl, and API guidance,
then `/tdd`; use `/docs-maintainer`, `/commit`, `/pr`, and `/pr-fixup` at delivery.

## Risks

See the plan's credential isolation and indexed config risks. Tests must be
hermetic and must never print live credentials or depend on a developer's Git
configuration. Real cloud validation is separate evidence and must not be inferred
from fixture success.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/executors/requirements/remote-executor-plugins.md), REQ-EXECUTORS-PLUGIN-002.
- [Design](../../specs/executors/system-design/remote-executor-plugins.md#repository-materialization-credentials).
- Existing API materialization tests and process `workspace_git_env_test.go`.
- [Issue evidence](https://github.com/kdlbs/kandev/issues/4677).

## Results

Completed 2026-10-22 in the primary session, without delegation.

- RED: `go test ./internal/agentctl/server/api -run '^TestWorkspaceMaterializeRepository_ManagedCredentials$' -count=1` returned the expected HTTP 422 instead of 201 before production changes.
- GREEN: the same command passed after introducing the request-scoped runner and instance snapshot.
- `go test ./internal/agentctl/server/api -count=1`: passed (72.561s).
- Final added API tests: `go test ./internal/agentctl/server/api -run '^TestWorkspaceMaterializeRepository_(ManagedCredentials|PublicWithoutManagedCredentials|MalformedCredentialsFailBeforeGit)$' -count=1`: passed (0.444s).
- `go test ./internal/agentctl/server/process -run 'TestManager.*(GitEnvironment|ManagedGit|Configure)|TestWorkspaceGitEnvironment' -count=1`: passed. Includes all new materialization environment tests.
- `go test ./cmd/plugin-fixture -run '^TestPluginExecutorFixture' -count=1`: passed.
- `pnpm --dir apps install --frozen-lockfile`: passed.
- `pnpm --dir apps/web e2e:run --host --shards 1 --project chromium tests/plugins/remote-executor-plugin.spec.ts`: passed, 2 tests (13.7s), with rebuilt runtime, web assets, and packaged provider fixture.
- `bash scripts/lint-go-changed apps/backend/internal/agentctl/server/api/workspace_materialize.go apps/backend/internal/agentctl/server/api/workspace_materialize_credentials_test.go apps/backend/internal/agentctl/server/process/manager.go apps/backend/internal/agentctl/server/process/materialize_git_env.go apps/backend/internal/agentctl/server/process/materialize_git_env_test.go`: passed, 0 issues. The first lint identified a test-helper defer/exit ordering issue, corrected before this successful run.
- `python3 scripts/list-docs.py validate`: passed.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `node scripts/validate-public-docs.mjs`: passed (54 public pages).
- `git diff --check`: passed.

Commands used the installed toolchain through `mise exec --` where needed.
Public docs updated: `docs/public/plugins-authoring.md` (reference guidance).

The API regression uses real Git, a child credential helper, a local private Git
HTTP service, and a deterministic broker. It verifies clone, fetch, reuse,
checkout-runner config, instance isolation, denied-access cleanup, no persisted
secrets, public/no-lease operation, and sanitized malformed-environment failure.
Ordinary checkout reuse does not fetch; authenticated fetch is exercised explicitly
through the same operation-scoped runner. No live cloud provider is claimed.
