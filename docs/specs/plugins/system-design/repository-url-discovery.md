---
status: current
system: plugins
created: 2026-10-29
owners:
  - kandev
requirements:
  - REQ-PLUGINS-REPOSITORY-URL-DISCOVERY-001
---

# Repository URL discovery design

## Boundary

The plugin service discovers plugin-owned candidates. It does not select between built-in and plugin providers or write repository rows.
The task service combines candidates and applies its
[shared admission rule](../../tasks/system-design/project-repository-context.md#workspace-verified-remote-admission).
The existing [inspection design](repository-provider-task-creation.md#inspection-action) remains the descriptor and action authority.

This extends the server resolver to inputs without provider hints.
It reuses `repositories.inspect`, including `{"matched":false}`, and requires no new wire action or SDK method.

## Requirement mapping

| Requirement | Sections |
| --- | --- |
| `REQ-PLUGINS-REPOSITORY-URL-DISCOVERY-001` | Candidate snapshot, inspection, result contract, lifecycle, compatibility |

## Candidate snapshot

Add `Service.DiscoverRepositoryProviders` in `internal/plugins/repository_provider_discovery.go`.
Its input contains the verified workspace ID and URL, with optional immutable pinning hints.
It returns `RepositoryProviderDiscoveryResult` and an operation error.
The result contains all `Candidates` as existing inspection descriptors and an optional typed `Failure` from unsuccessful candidate inspections.
Cancellation, deadline, or lifecycle errors use the operation error and discard the result.
This separates an unrelated provider failure from an interrupted discovery pass.

Take a copied `Registry.List()` snapshot before dispatch.
Select records with `StatusActive`, manifest-owned `RepositoryProviders`, and a workspace-scoped `repositories.inspect` action.
Use the registry's existing exclusive ownership rules for each provider ID.
Sort candidates by plugin and provider ID for stable diagnostics, not selection precedence.
Do not hold the registry mutex across plugin calls.
Do not read plugin secrets, frontend registrations, or arbitrary plugin configuration to infer ownership.

Installation alone does not prove URL ownership.
Each plugin decides whether its connection is enabled and configured in the supplied workspace.
The host does not assume all VCS plugins expose the same connection action or configuration schema.

## Inspection and bounds

Factor the existing inspection implementation only enough to retain three internal outcomes:
validated match, ordinary no-match, and typed failure.
Keep explicit `InspectRepositoryProvider` behavior compatible, including its existing not-found projection for `{"matched":false}`.
Discovery must distinguish that envelope from an actual provider `404` or malformed descriptor.

Invoke each eligible manifest owner through the existing generation-fenced action dispatcher.
The action body remains `{"url":"..."}`. Immutable hints stay host-side for response validation.
The plugin must reject unrelated origins before authenticated network calls.
Use the existing descriptor validation for provider identity, scope, immutable ID, default branch, and HTTPS clone-origin agreement.

Allow at most four simultaneous inspections under one 15-second discovery deadline.
Each action also retains its declared request limit and existing 1 MiB response limit.
Collect outcomes independently so a failing unrelated provider does not cancel another provider's inspection.
Wait for all scheduled work or the discovery deadline before evaluating successful results.
Join canceled work before returning.
Caller cancellation always returns without accepted candidates.
The overall deadline returns an unavailable discovery error rather than an incomplete winner list.

The deadline covers dispatch admission, not only the RPC. Dispatch read leases are
context-aware, so a discovery pass waiting behind a lifecycle write lease (config
commit, restart, disable, or upgrade) still returns when its caller cancels or its
deadline expires. The write side keeps its existing exclusion and readers still
join before the pass returns; no unjoined goroutine is used to emulate
cancellation. A canceled pass with no eligible provider also returns the
cancellation error instead of an empty result.

## Result contract

- One or more valid matches: return all validated descriptors after the candidate snapshot recheck.
- No matches and only ordinary no-match outcomes: return an empty result without error.
- Failed candidate inspections: retain a stable typed `Failure` in the result. Prefer unavailable over invalid, then not-found, in deterministic order.
- An overall deadline, caller cancellation, or changed candidate snapshot: return an error without candidates.

With a completed discovery pass, valid matches remain usable despite an unrelated candidate's ordinary inspection error.
This matches the browser resolver's existing successful-match policy.
Multiple matches remain separate. The task service rejects ambiguity across the complete built-in and plugin candidate set.
No provider wins because its RPC completes first.

## Lifecycle

Capture each candidate's installation identity and existing `dispatchGeneration` fields.
Reuse `beginPluginDispatch` so disable, uninstall, restart, and upgrade keep their existing invocation leases.
Before returning candidates, compare the eligible provider/owner/installation snapshot with the current registry.
Refuse a changed snapshot and let the caller retry. Do not retry inside the same discovery request.
This is a discovery-boundary check, not a transaction with later task persistence.
Existing repository identity and credential-generation guards continue to govern later runtime operations.

## Task adapter

Extend `RepositorySelectionResolver` with `DiscoverRepositorySelections(ctx, workspaceID, input)`.
It returns `RepositorySelectionDiscoveryResult` and an operation error.
The result contains server-owned `Candidates` and the optional normalized candidate `Failure`.
Add the same method to `backendapp.pluginRepositorySelectionResolver` and its narrow inspector interface.
The adapter maps discovered descriptors exactly as its explicit resolver does today.
It sets `TrustedProviderDescriptor` only from validated plugin-service results.
Existing request selectors and branch choices remain caller-owned through the task trust helper.

The existing composition in `backendapp/services.go` wires this adapter into the shared task service.
Interface conformance makes missing discovery wiring a build error.
Test stubs gain an empty default discovery result so existing explicit-selection tests retain their meaning.

## Compatibility evidence

The inspected Bitbucket and Forgejo manifests both declare workspace `repositories.inspect` actions.
The [Bitbucket handler](https://github.com/kdlbs/kandev-plugin-bitbucket/blob/main/internal/plugin/workflow_repository_actions.go)
returns no-match for unconfigured or unrelated URLs and hydrates its authoritative repository identity.
The [Forgejo manifest](https://github.com/runoverlabs/kandev-plugin-forgejo/blob/main/manifest.yaml)
declares the same action. This is capability evidence, not a completed live Forgejo contract test.
No external plugin implementation change is part of this package.

Existing plugin clone descriptors require HTTPS.
This change does not promise support for every HTTP/SSH input accepted by an external plugin's own UI.
Supported input syntax remains provider-defined, and the returned descriptor must satisfy the host's existing rules.

## Observability

Reuse sanitized per-inspection logs and existing bounded failure categories.
Record exactly one discovery completion event for every exit, including invalid
requests, deadline, cancellation, and a changed eligible snapshot, with candidate
and match counts, duration, and failure category.
Successful and failed candidate inspections keep their own sanitized
per-inspection entries.
Do not log raw URLs, response bodies, plugin configuration, or credentials.
No new metric or runtime feature flag is required.

## Decision and delivery

- [Provider-owned bare URL discovery](../../../decisions/2026-10-29-provider-owned-repository-url-discovery.md)
- [Provider-neutral project-source package](../../../plans/office-project-self-managed-gitlab/plan.md)
