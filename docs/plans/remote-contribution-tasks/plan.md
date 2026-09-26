---
spec: docs/specs/tasks/system-design/remote-contribution-tasks.md
created: 2026-08-04
status: completed
---

# Implementation Plan: Remote Contribution Tasks

## Overview

Teach the existing `create_task_kandev.repositories[].repository_url` field to recognize GitHub pull
request and GitLab merge request URLs without adding schema properties. Resolve provider-owned source
metadata before task creation, persist a typed contribution binding on the target task-repository
attachment, associate the existing remote change, and reconstruct the exact checkout and push target on
every launch. Keep `origin` and ordinary repository task behavior unchanged; fork write access is a
narrow, binding-authorized runtime capability.

The implementation follows
[ADR-2026-08-04-remote-contribution-bindings](../../decisions/2026-08-04-remote-contribution-bindings.md),
[provider-neutral remote repositories](../../decisions/2026-07-20-provider-neutral-remote-repositories.md),
and the existing [task Git credential policy](../../decisions/2026-07-27-task-git-credential-policy.md).

## Implementation status

- [x] Task 01 — provider-neutral binding validation and GitHub/GitLab contribution resolution.
- [x] Task 02 — MCP URL resolution, target attachment persistence, existing-change association, and
  compensation on association failure.
- [x] Task 03 — exact-SHA source materialization, target-origin preservation, restart-safe runtime
  projection, and collision-safe contribution remotes.
- [x] Task 04 — binding-authorized credential scopes, source-branch push routing, no-force enforcement,
  dry-run preflight, and existing-change reuse.
- [x] Task 05 — provider/runtime integration coverage, unchanged ordinary repository coverage, and public
  documentation updates.

## Invariants

- The public `create_task_kandev` input property names and count do not change.
- Provider APIs, not MCP callers, determine target/source identity, refs, SHA, state, and collaboration.
- One task-repository attachment represents the target repository; the contributor fork is a push
  destination, not another workspace source.
- All persisted URLs are canonical and credential-free. No provider title, body, token, lease, or
  credential-helper data enters the binding or trusted prompt.
- Contribution checkout and push paths fail closed on malformed or unknown binding versions, unsafe
  refs, stale head SHA, or source-scope mismatch.
- Ordinary repository tasks preserve current checkout, `origin`, credential, and create-PR behavior.

## Backend

### Provider-neutral binding and provider resolution

- Add a versioned `RemoteContribution` domain shape beside task repository models with strict encode,
  decode, validation, and metadata helpers. It carries provider/kind, canonical change URL, number/IID,
  open state, base/head branches, head SHA, source repository identity/URL, and collaboration eligibility.
- Extend GitHub `PR` plus PAT and `gh` conversions with the source repository owner/name/clone URL and
  `maintainer_can_modify`. Add a workspace-scoped resolver that parses a canonical `github.com` PR URL,
  fetches the PR, verifies returned target identity and open/live head state, and returns the shared
  binding without title/body content.
- Extend GitLab MR transport/domain models with source/target project IDs and collaboration state, and
  expose enough project lookup data to obtain the source project's canonical clone URL and path. Add a
  workspace-scoped resolver that reuses configured-host parsing, verifies the target project, fetches the
  source project, and returns the shared binding.
- Treat same-repository changes as collaboration-eligible. Reject fork changes when GitHub
  `maintainer_can_modify` or GitLab `allow_collaboration` is false. Validate provider refs with the shared
  Git branch/ref validator before returning a binding.

### MCP task creation and association

- Add a `RemoteContributionResolver` interface to the backend MCP create-task coordinator. Resolution
  receives the selected workspace and raw `repository_url`, and returns `matched=false` for ordinary
  repository URLs so the existing resolver remains authoritative.
- In `handleCreateTask`, resolve contribution URLs after workspace/workflow selection but before task
  persistence. Replace the contribution URL with the canonical target repository input, set base and
  checkout branches from provider data, and carry the typed binding into the target `TaskRepositoryInput`.
- Persist `remote_contribution` in the existing `task_repositories.metadata` JSON during task creation;
  no schema migration is required. Keep the existing GitHub `pr_number` compatibility metadata where
  current worktree/read paths still consume it until all runtime reads use the typed binding.
- After task persistence and before asynchronous launch, idempotently associate the GitHub PR or GitLab
  MR with the exact new task-repository attachment. On association failure, compensate the newly created
  task and return failure rather than launching an unlinked task.
- Wire provider adapters in `internal/backendapp` using the existing MCP dependency-setter pattern.
  Preserve current task/profile/executor inheritance and `start_agent` behavior.
- Change only the existing `repository_url` schema description. Add a catalog regression test that
  asserts the full property-name set is unchanged and that provider-specific source fields are absent.

### Runtime projection and worktree materialization

- Thread the validated binding from task-repository metadata through `repoInfo`, `RepoSpec`, lifecycle
  create/prepare requests, and agentctl repository configuration as one optional typed object rather than
  a growing list of primitives.
- In worktree materialization, fetch the source branch from its credential-free source URL into a
  deterministic remote-tracking ref. Use a collision-resistant remote name derived from provider, host,
  and source path so shared repository/worktree Git config cannot alias two contributor forks.
- Require the fetched source ref to equal the persisted head SHA before creating the worktree. Create the
  local checkout branch using the existing checked-out-branch collision strategy, then set its upstream
  to the contribution remote's exact head branch. Preserve target `origin` for base fetches and diffs.
- Reconstruct the same remote and upstream on fresh launch, resume, reset, and remote/container
  materialization. Fail visibly if the source project disappears, the binding version is unsupported, or
  the remote name already resolves to inconsistent identity.
- Publish only structured contribution URL/provider/number/source-branch guidance to agent context.
  Never copy remote title, description, comments, or diff content into trusted context.

### Credential authorization and push routing

- Extend managed GitHub session scope construction to add the source owner/repository only for a fork
  binding. Update the broker scope authorizer to prove the requested source identity exactly matches a
  valid contribution binding on the session's task-repository attachment. Leave ordinary target scope
  authorization unchanged.
- Carry explicit `push_remote` and `push_branch` values into agentctl's repository tracker for
  contribution worktrees. Update `GitOperator.Push` to use those values while normal repositories still
  default to `origin` and their current local branch. Reject force pushes for remote-contribution tasks.
- Before agent process startup, run `git push --dry-run <contribution-remote>
  HEAD:refs/heads/<head-branch>` with the final task environment. This validates managed or
  executor-owned credentials without mutating the remote. Redact canonical URLs and provider errors in
  logs and user-visible failure details.
- Ensure create-PR/create-MR operations detect the persisted existing-change association and return or
  refresh it instead of pushing to `origin` and opening a duplicate.

## Tests

- **Provider resolution:** table-driven GitHub PAT/CLI and GitLab client/service tests cover canonical
  URL parsing, returned identity mismatch, same-repository changes, editable/non-editable forks,
  closed/merged changes, missing heads, invalid refs, and credential-free source URLs.
- **Binding persistence:** task service and SQLite tests round-trip version 1 in
  `task_repositories.metadata`, preserve unrelated metadata, reject invalid/unknown runtime bindings,
  and prove ordinary inputs omit the field.
- **MCP orchestration:** handler/server tests cover both providers, current workspace resolution,
  target repository creation, association-before-launch, compensation, `start_agent: false`, ordinary
  repository fallback, and unchanged schema properties.
- **Worktree/runtime:** temporary target/source Git repositories prove exact-SHA fetch, target `origin`,
  source upstream, local-branch collision behavior, stale SHA rejection, restart reconstruction, and
  no cross-task remote aliasing.
- **Credentials/push:** broker tests prove exact binding authorization and reject unrelated forks,
  malformed bindings, other tasks, and other sessions. Agentctl tests prove explicit source routing,
  dry-run preflight, redaction, no force push, and unchanged normal push behavior.
- **Integration:** a focused backend integration test creates a task from each provider URL with fake
  provider APIs and temporary Git remotes, prepares the session, commits, pushes, and verifies the source
  branch advances while the target branch and existing-change identity remain unchanged.

## Public documentation

- Update the reference page `docs/public/automation-and-mcp.md` to document the polymorphic
  `repository_url`, supported URL shapes, configured-provider prerequisite, unchanged input schema,
  collaboration requirement, and launch-time write preflight.
- Update the task coordination how-to in `docs/public/coordination.md` with one GitHub and one GitLab
  example and recovery guidance for disabled collaboration or missing credentials.
- Keep security details concise and link to the existing credential-policy explanation rather than
  documenting tokens or broker internals. These are a reference-page update and a how-to update; no new
  navigation entry is needed.

## Execution order

1. [Task 01: Provider contribution resolution](task-01-provider-contribution-resolution.md)
2. [Task 02: MCP creation and persistence](task-02-mcp-creation-and-persistence.md)
3. [Task 03: Runtime contribution materialization](task-03-runtime-contribution-materialization.md)
4. [Task 04: Credential and push routing](task-04-credential-and-push-routing.md)
5. [Task 05: Integration coverage and public docs](task-05-integration-coverage-and-docs.md)

The tasks are intentionally sequential. Each wave establishes a security- or runtime-sensitive contract
consumed by the next, and Tasks 02–04 touch shared task-launch types where parallel edits would be more
expensive than the saved time.

## Final verification

```bash
make -C apps/backend test
make -C apps/backend lint
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
```

No frontend implementation or browser E2E is required because this release changes only the MCP and
backend runtime surfaces. If a task-creation UI is added later, it requires a separate mobile-parity
design and E2E package.

## Verification results

- `make -C apps/backend test` — passed.
- `make -C apps/backend lint` — passed with zero backend, web, harness, and architecture issues.
- `node --test scripts/validate-public-docs.test.mjs` — 58 tests passed.
- `node scripts/validate-public-docs.mjs` — 41 published pages validated.
- The affected backend package suite — 5,603 tests passed across 17 packages.
- `git diff --check` — passed.
