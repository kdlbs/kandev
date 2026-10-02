---
status: current
system: workspaces
requirements:
  - REQ-WORKSPACES-REMOTE-RESOLUTION-001
created: 2026-10-02
owners:
  - kandev
---

# Remote repository resolution system design

## Purpose and boundaries

Workspace repository resolution chooses which registration supplies new remote
work. It must distinguish persisted provider identity from local availability.
The executor continues to own clone materialization and task preparation.

`FindOrCreateRepository` resolves provider identity separately from local
availability. A `source_type=local` registration can retain its saved identity
after its directory is deleted. The executor's
`ensureRepoLocalPathForSessionAndState` deliberately does not clone local rows,
so remote selection validates local availability before adopting such a row.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| `REQ-WORKSPACES-REMOTE-RESOLUTION-001` | Selection, registration, launch, and verification |

## Selection

Apply availability-aware selection only to a remote lookup carrying `RemoteURL`
with no explicit `LocalPath`. Both `resolveRepoInputRemote` and
`resolveTrustedRemoteRepository` already enter `FindOrCreateRepository` with
that shape. Do not change `resolveRepoInputID`, local-path registration, or
ordinary provider metadata lookup semantics.

Keep the existing provider-identity lookup as the common fast path. Before
adopting a `source_type=local` result for a remote request, inspect the saved
directory. A confirmed not-exist result makes it ineligible. An existing path
must pass the existing canonical Git validation and resolve to its saved
identity. Permission, I/O, cancellation, invalid Git, and canonical-path
mismatch errors propagate; they do not authorize fallback. Do not create or
change filesystem content during selection.

`findRepositoryForRemoteSelection` owns this admission. Its
`findAvailableRemoteRepository` fallback enumerates alternatives only when the
first local match is confirmed absent.

If the first match is a deleted local directory, enumerate workspace repository
rows through `repoEntities.ListRepositories`, not the presentation-level
deduplicated list. Filter candidates using the same normalized
`ProviderRepositoryIdentity` semantics as `GetRepositoryByProviderIdentity`:
scope plus immutable repository ID when scoped, otherwise the existing
workspace/provider/host/owner/name lookup. Preserve the scoped-import refusal
to adopt an unscoped legacy row. Exclude Kandev task-worktree registrations
using the existing replacement guard.

Order matching candidates by `CreatedAt`, then `ID`, as the current lookup
does. Skip every confirmed-deleted local directory and adopt the first eligible
candidate. Provider-managed rows remain eligible when pathless or when their
managed clone was deleted; the executor can materialize them. A usable local
candidate retains its registration and launch behavior. Keep the whole
lookup/create sequence under `repoResolveMu` so concurrent callers in the
backend instance converge.

## Registration and persistence

If no eligible match exists, use the existing provider creation branch in
`FindOrCreateRepository`. It creates a separate provider row with the validated
request identity, canonical remote URL, and requested default branch. The
original local row and its dependent rows remain untouched. Do not change its
source type or clear its local path. Do not inherit local setup scripts, secret
bindings, or branch configuration into the new provider registration.

Apply existing field backfill only to the selected eligible registration.
Preserve the `created` result: only a row inserted by this call is owned by its
rollback. A later request must discover the same managed row even while an
older deleted local row remains the earliest database match.

No schema, API, profile, feature flag, or repository-wide identity migration is
needed. Path-based presentation deduplication can continue to show distinct
local and managed checkout registrations.

## Launch and failure behavior

`resolveTaskRepoInfoForSession` calls `ensureTaskCheckoutPath`, which selects
the existing provider clone path. `ensureRepoClonedForSessionAndState` persists
the managed path through `UpdateRepositoryLocalPath`; existing task/session
credential requests, clone admission, checkout options, and default-branch
backfill remain authoritative. Explicit local launches retain the executor's
no-reclone rule.

Use the existing error propagation for validation, remote authentication, clone,
and cancellation errors. A successful selection does not claim launch success
until preparation succeeds. Do not retry against the skipped local checkout.
The original local directory must remain absent after managed preparation.
Repository deletion after selection remains an ordinary preparation failure;
this correction introduces no background repair or mutation of local rows.

## Verification and observability

Service tests use a real SQLite store, create and register a Git checkout,
delete it, then resolve the same provider URL. Assert separate managed identity,
preservation of the original row and links, mixed candidate selection, repeated
and concurrent convergence, usable-local reuse, and identity isolation.

A backend integration test crosses real service resolution, persisted task
attachment, executor repository preparation, and worktree creation. Use a
disposable local Git origin through a test transport adapter and observe the
agent-start boundary with an existing fake. It must reproduce failure before
the selector change and prepare a valid checkout afterward, preserving the
requested branch and leaving the original directory absent. No internet or
developer runtime is required.

Retain existing executor regressions for provider-clone recovery and local
ownership. Existing clone and failure diagnostics suffice; no new metrics or
credential-bearing logs are needed. Desktop and phone submit the same remote
locator, with no rendered UI changes.

## Related decisions

- [Explicit local repository trust](../../../decisions/2026-07-20-explicit-local-repository-trust.md)
- [Provider-origin identity](../../../decisions/2026-07-20-repository-provider-origin-identity.md)
- [Provider-neutral remote repositories](../../../decisions/2026-07-20-provider-neutral-remote-repositories.md)

The correction preserves these ownership boundaries. A separate repair ADR is
unnecessary because the selection rule and its rationale are contained here.
