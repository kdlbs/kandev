# ADR-2026-08-10-plugin-change-request-mutations: Route Native Change-Request Mutations Through Providers

**Status:** accepted
**Date:** 2026-08-10
**Area:** frontend, protocol

## Context

Repository-provider plugins can list repositories and expose review data, but Kandev's
native **Create PR** workflow still delegates the complete operation to `agentctl`,
whose provider-specific creation code supports only built-in hosts. Registered review
providers likewise have no native unlink mutation or workspace association source, so
plugin links cannot drive the same task-row indicators and unlink controls as built-in
change requests.

## Decision

`RepositoryProviderRegistration` gains an optional provider-neutral change-request
creation callback. Kandev keeps the native create dialog and performs the ordinary Git
push first through the existing executor path. Only after that push succeeds does the
host call the active repository provider with the current workspace, task, persisted
repository, title, description, destination branch, and supported options. Plugin
callbacks must cross the authenticated action boundary; plugin servers rederive
repository and checkout authority from the verified task/repository context.
Normalized open reviews from registered providers also participate in the native VCS
action state, so an already-linked plugin change request suppresses the primary
**Create PR** action exactly like a built-in pull request.
For a task-scoped action, the browser may include an optional `repositoryId`.
Kandev accepts it only when that persisted repository is attached to the verified
task, then supplies both IDs in `VerifiedActionContext`; omitting it preserves the
existing task-only action behavior.

`ReviewProviderRegistration` gains optional unlink and workspace-association external-
store methods. Kandev renders unlink controls and task-row/card change-request glyphs
from those normalized callbacks and snapshots. Providers own association queries and
mutations; the host owns placement, accessibility, responsive behavior, pending/error
feedback, lifecycle cancellation, and semantic icons.
An explicit unlink must remain detached: provider implementations suppress automatic
source-branch relinking for that task/review pair until the user explicitly links it
again, and watch-owned associations relinquish ownership without deleting the task.

Built-in providers retain their current adapters. `agentctl` receives no Bitbucket or
plugin dispatch branch. Callback work and association subscriptions are revoked with
their owning plugin, matching the existing repository/review provider lifecycle.

## Consequences

Future code-host plugins can participate in native create, unlink, and task-indicator
workflows without provider-specific host branches. Creation now has a deliberate
two-stage failure model: a successful push followed by a failed remote create reopens
the native dialog in retry state and does not push again unnecessarily. Provider
implementations must truthfully declare option support and make create retries safe.

## Alternatives Considered

- Add Bitbucket creation to `agentctl`: rejected because it moves plugin API/auth logic
  into the host and repeats for every future provider.
- Register a plugin-owned **Create PR** button or modal: rejected because it duplicates
  Kandev's native Git eligibility, dialog, feedback, and mobile behavior.
- Infer plugin links by refreshing every task's full review detail: rejected because a
  task list would create an unbounded provider-request fan-out; one workspace
  association snapshot is the reusable bounded contract.
