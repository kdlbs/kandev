---
status: draft
system: workspaces
created: 2026-08-25
owners:
  - kandev
---

# Worktree Base Refresh Requirements

## Overview

The workspace system owns the base ref for a task worktree. A host worktree can
use a local base without remote publication. Remote refresh improves freshness,
but it must not prevent intentional local development.

Remote materialization has a different contract. If Kandev has no usable local
base, it must obtain the selected branch before it creates the worktree.

## Terminology

- **Local base:** A valid local ref that Kandev can use to create the worktree.
- **Best-effort refresh:** A remote refresh attempt that can fall back to a
  local base.
- **Required materialization:** A remote fetch that is necessary because no
  usable local base exists.
- **Fetched remote base:** The remote-tracking ref from a successful fetch.

## Requirements

### REQ-WORKSPACES-WORKTREE-BASE-REFRESH-001: Local-First Worktree Base

**Intent:** Preserve local development while Kandev attempts to refresh remote
state before worktree creation.

**User story:** As a workspace user, I want Kandev to create worktrees from my
local branches without publishing them first.

#### Acceptance criteria

- **AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.1:** When pull-before-worktree is
  disabled, Kandev shall use an available local base without remote access.
- **AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.2:** When pull-before-worktree is
  enabled and a local base exists, Kandev shall attempt a remote refresh before
  it creates or recreates the worktree.
- **AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.3:** When refresh has an
  authentication, network, timeout, missing-branch, or non-cancellation Git
  error, Kandev shall use the local base and show a credential-safe warning.
- **AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.4:** When the refresh succeeds,
  Kandev shall preserve local-only commits and include compatible remote
  commits when one ref contains the other.
- **AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.5:** When local and remote refs
  diverge, Kandev shall preserve the selected local base and show a warning.
- **AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.6:** When no local base exists,
  Kandev shall stop preparation if required materialization cannot provide the
  selected branch.
- **AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.7:** When one repository in a
  multi-repository task has a usable local fallback, its refresh error shall not
  stop the task agent.
- **AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.8:** When required materialization
  fails for any repository, Kandev shall stop the task agent and identify that
  repository in a credential-safe error.
- **AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.9:** When an authenticated remote
  advertises zero refs, Kandev shall use the marked local baseline from
  `REQ-WORKSPACES-EMPTY-REMOTE-REPOSITORIES-001`.
- **AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.10:** When the caller cancels
  preparation, Kandev shall stop without creating a fallback worktree.

- **AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.11:** When a task repository is
  linked to a GitHub pull request, Kandev shall use the pull request's current
  base branch for materialization when live provider state is available, and
  shall retain the stored base when the provider lookup is unavailable.
  Cross-repository bases also obey criteria .15 through .19.
- **AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.12:** When polling observes that a
  linked pull request's non-empty base branch changed, Kandev shall update the
  matching task repository base without failing the pull-request sync if that
  secondary update is unavailable.
- **AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.13:** When required refresh for a
  pull-request task proves that the requested remote base no longer exists,
  Kandev shall use a different configured fallback only after that fallback
  refresh succeeds, and shall identify both branches in a warning. Without a
  usable fallback, preparation shall fail with a missing-remote-ref
  classification. Authentication, network, timeout, cancellation, divergent,
  uncertain, and other unproven failures remain fatal for pull-request tasks.
  This branch-only fallback does not apply to a repository-qualified PR base.

- **AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.14:** When a base refresh fails,
  backend diagnostics shall identify the repository, operation, branch, and
  failure category. Known causes shall have a fixed diagnostic explanation.
  Unknown causes shall remain explicitly unknown. Diagnostics shall exclude
  credentials, remote URLs, and raw command output. Existing refresh and
  fallback decisions shall remain unchanged.

## Proposed amendment: repository-qualified PR bases

Criteria .15 through .19 are draft. The workspace system owns base resolution
and materialization. The task system retains provider association ownership.
Delivery: [Fork PR base resolution](../../../plans/fork-pr-base-resolution/plan.md).

- **AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.15:** For a PR-linked checkout, base resolution shall preserve the target repository and branch. A same-named fork branch shall not replace that target.
- **AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.16:** Before required materialization succeeds, Kandev shall verify the target commit in the checkout. A provider-supplied base commit shall remain tied to its repository and branch.
- **AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.17:** When a cross-repository base cannot be resolved or fetched, required preparation shall fail without substituting a repository default. Errors shall preserve cancellation and omit credentials.
- **AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.18:** Automatic fallback and default-branch recovery shall preserve an explicit PR target. They shall not change repository defaults, push destinations, or another attachment's base.
- **AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.19:** Ordinary repository defaults and same-repository fallback behavior shall remain compatible. Every required repository must resolve independently; one valid repository shall not hide an unresolved fork base.

These criteria do not require a provider refresh for valid reused worktrees.
They do not change explicit manual comparison selection. Existing comparison
fetch failures keep their non-blocking status behavior after workspace creation.

## Compatibility

This requirement replaces the universal fail-closed refresh behavior from
Kandev v0.92.1. Remote-only executors and branches keep strict materialization.

## Out of scope

- Making setup-script errors fatal.
- Refreshing a valid worktree that Kandev reuses.
- Adding a background repository-refresh service.
- Resetting, rebasing, merging, or removing local refs.
- Changing agent Git transport after the agent starts.
- Pushing a local branch during worktree preparation.

## System design

- [Worktree Base Refresh System Design](../system-design/worktree-base-refresh.md)
