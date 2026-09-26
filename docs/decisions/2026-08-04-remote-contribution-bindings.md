# ADR-2026-08-04-remote-contribution-bindings: Bind Remote Contributions to Target Repositories

**Status:** accepted
**Date:** 2026-08-04
**Area:** backend, protocol, security, GitHub, GitLab

## Context

`create_task_kandev` can resolve a repository URL, but a contributor pull request or merge request has
two repository identities: the target repository that owns the change and the source repository and
branch that must receive commits. GitHub also exposes pull-request refs that are readable but are not a
stable writable branch. GitLab represents the same relationship with target and source projects.

Adding caller-supplied provider, change number, head branch, source repository, and push-remote fields
would enlarge every MCP catalog and let an untrusted caller construct inconsistent or over-broad Git
credential requests. Replacing `origin` with the fork would instead break base-branch comparison,
existing repository identity, CI/PR association, and normal task behavior.

## Decision

Remote change URLs are a semantic extension of the existing
`create_task_kandev.repositories[].repository_url` value. The public input schema gains no properties.

The backend parses and resolves recognized URLs through the workspace's provider service. It persists a
versioned, non-secret `remote_contribution` binding in the target `task_repositories.metadata`. The
binding contains only provider-authored change identity, base/head refs and SHA, source repository
identity and canonical credential-free remote URL, and collaboration eligibility. The target repository
remains the attachment's normal `repository_id`.

Runtime materialization keeps `origin` pointed at the target repository and adds a dedicated
contribution remote for the source repository. The checkout is pinned to the resolved head SHA and its
upstream/push target is the source branch. Ordinary attachments continue to push to `origin`.

Managed GitHub credentials may add a source-repository lease scope only when it exactly matches a valid
contribution binding attached to the authorized task and session. Executor-owned credentials are not
broadened; they must pass a non-mutating source-branch push preflight before the agent starts. No token,
lease, credentialed URL, or credential-helper detail is persisted.

GitHub PR and GitLab MR associations are created before launch. Provider title, body, comments, and diff
content remain untrusted and are not copied into the initial prompt or trusted task context.

## Consequences

- MCP clients get the workflow with a minimal description-only catalog change.
- The provider, not the caller, is authoritative for source repository and branch identity.
- Target-repository comparisons, review integration, and normal `origin` semantics remain intact.
- Fork pushes require a narrow second credential scope and explicit runtime push routing.
- Runtime and broker code must understand one versioned contribution binding and fail closed on unknown
  or inconsistent values.
- A task may persist while launch fails if executor-owned credentials cannot write the source branch;
  the task remains retryable after credentials or collaboration permissions are corrected.

## Alternatives considered

### Add `change_url` and provider-specific fields to the MCP tool

Rejected. A single new URL field would be modest, but supporting caller-supplied head/source details
would grow the schema and create conflicting sources of truth. Even a lone `change_url` is unnecessary
because `repository_url` already selects the repository materialized for the task.

### Attach the contributor fork as a second task repository

Rejected. The task would materialize two copies of substantially the same repository, confuse primary
repository and PR association, and expose the fork as an independent workspace source rather than a
narrow push destination.

### Replace `origin` with the contributor fork

Rejected. Base-branch fetches, comparisons, provider identity, and existing automation assume `origin`
is the task's target repository.

### Checkout GitHub's `refs/pull/<number>/head` and push it

Rejected. The ref is suitable for reading the submitted head but is not the contributor's writable
branch identity, and GitLab has no identical provider-neutral contract.

### Tell the agent to run provider CLI commands manually

Rejected. It postpones URL validation and permission failures until after work starts, exposes more
provider-specific behavior to prompts, and cannot support deterministic resume or credential scoping.
