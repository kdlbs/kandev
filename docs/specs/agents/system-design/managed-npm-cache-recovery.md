---
status: current
system: agents
created: 2026-11-07
updated: 2026-11-07
requirements:
  - REQ-AGENTS-MANAGED-RUNTIME-RECOVERY-002
  - REQ-AGENTS-MANAGED-RUNTIME-RECOVERY-004
  - REQ-AGENTS-MANAGED-RUNTIME-RECOVERY-006
---

# Incomplete managed npm cache recovery

## Boundary and requirement mapping

Agents owns runtime identity, preparation, discovery, and startup recovery.
Its [paired requirement](../requirements/managed-npm-cache-recovery.md) defines
the incomplete execution-tree outcome.
This design extends [managed npm recovery](managed-npm-runtime-recovery.md).
It covers the observed missing execution manifest and executable-link state.
The [decision](../../../decisions/2026-11-07-isolated-managed-npm-cache-recovery.md)
records the choice to preserve shared files and prepare privately.

| Requirement | Design boundary |
| --- | --- |
| `REQ-AGENTS-MANAGED-RUNTIME-RECOVERY-002` | Executor-local evidence; private preparation |
| `REQ-AGENTS-MANAGED-RUNTIME-RECOVERY-004` | Task recovery and its single replacement budget |
| `REQ-AGENTS-MANAGED-RUNTIME-RECOVERY-006` | Evidence, restart recovery, task/probe integration, compatibility, and proof |

This draft owns the common private-preparation primitive. The pending
[Codex dependency package](../../../plans/managed-codex-dependency-recovery/plan.md)
will reuse it and add its own dependency evidence and validation.
Requirement 005 remains pending. This correction does not implement it implicitly.

## Executor-local evidence

`process.Manager` and the utility executors capture bounded diagnostics before
redaction. Add a proposed `ManagedRuntimeCacheEvidence` structure under
`agentctl/types`. Keep it separate from generic npm-code classification.
Its fields identify the stable condition, trusted package spec, and failed
process generation or probe attempt. It contains no filesystem path or stderr.

Accept only a canonical command from a registered `ManagedNPMRuntimeAgent`.
Validate the exact version through `ValidateExactPackageSpec`. Preserve the
existing selected OpenCode package family. Native binaries, passthrough,
operator-defined commands, and launch wrappers remain ineligible.

Inspection uses the failed child's effective environment and strip policy.
Resolve npm's cache through direct argv with the same isolated project prefix.
Derive the execution key with `NpxExecutionCacheKey`; never use an error path
as the cache root. Read bounded manifests through contained filesystem access.
Reject symlinked roots, escaped targets, malformed metadata, and read errors.

For the observed `root_manifest_missing` condition, require all of these facts:

1. Initialization failed before conversation creation, restoration, or dispatch.
2. The matching owned process exited ordinarily. Complete diagnostics contain
   npm `ENOENT`, an open/read failure, and the exact derived root manifest path.
3. The selected execution tree exists. Its installed top-level package manifest
   has the trusted name and exact version.
4. A fresh contained read confirms the execution root's `package.json` is absent.

Compare any diagnostic path with the derived location before redaction.
The comparison only confirms the operation; it never supplies an install path.
A missing whole execution tree is a normal cold install, not this condition.
A missing workspace file, transitive module, permission failure, or a race
that restores the manifest does not qualify. A missing bin link alone, without
matching npm missing-file evidence, does not authorize a generic shell retry.

Inspect the expected package executable and npm link as additional inventory.
An npm bin symlink is valid only when its contained target matches the selected
package manifest. Do not reject ordinary contained npm links as hostile roots.
Do not execute an incomplete cached binary to establish eligibility.

The first implementation supports registered package `bin` declarations for
the selected built-in command. Reject ambiguous multi-bin declarations and
paths outside the package. Never install a package named only in stderr.
Preserve authentication, policy, disk-space, and intentional-stop precedence.
Do not add `ENOENT` to the generic transient-npm allowlist.

## Private preparation

The `PreparedRuntime` owner lives under `agent/managedruntime`.
It receives a trusted package descriptor, the effective environment, and a
process runner owned by the local agentctl instance. The descriptor includes
the exact selected version and expected command from built-in metadata.

The owner creates a private `0700` root under the execution host's temporary
directory. Resolve the trusted temporary parent to its canonical path before
allocation. Retain no-follow inspection of the private root and its descendants.
It records an opaque owner marker and creates separate project
and npm-cache directories. Paths remain outside the task checkout, mounted
provider home, shared npm cache, and other preparation roots.

Prepare through npm argv equivalent to the existing `CacheUpdateCommand`:
`npm --prefix <private-project> --cache <private-cache> exec --yes --prefer-online --package=<exact-spec> -- node -e ''`.
This materializes the package without a model prompt. Keep configured registry,
credentials, proxies, certificate trust, and policy. The private cache override
is internal to this operation; preserve its original value in logical context
identity and never write it to the profile or user npm configuration.

Validate the new root manifest, installed package name/version, selected bin
link, and executable target before replacement. The target must remain inside
the prepared tree. Use platform-aware bin validation, including Windows shims.
Do not hand-write npm's root manifest or copy incomplete `node_modules`.
Do not run a package-provided shell string as a validation command.

Keep optional-dependency overrides out of this generic repair. The future
Codex dependency validator owns its scoped `--include=optional` extension.
Normal dependency resolution can change unselected transitive versions.
Runtime observations must report the resulting installation honestly.

Rewrite only internal prefix/cache arguments at agentctl's final command
boundary. Preserve the logical trusted command, cwd, protocol arguments,
profile CLI flags, selected version, and native conversation ID.
Both ACP and Codex app-server use this owner. Do not add a provider-name branch
for the cache metadata condition.

Installers and validation commands use existing owned process runners.
Cancellation must stop and reap their descendants before private-root cleanup.
Task prepared files remain owned by the instance until all consuming processes
stop; backend request completion and detach/adoption do not release them.
Utility preparation remains owned until the final probe tree stops.
Cleanup removes only the verified owner root. A marker does not authorize a
startup-wide orphan sweep. Failed cleanup retains ownership for a later retry.

No shared preparation cache, persistent promotion, new storage table, or global
launch lock is introduced. Independent operations have independent roots.
Host probe retries retain the instance's existing admission ordering.
For profile single-flight work, the existing flight owns preparation. One HTTP
waiter's cancellation does not cancel other waiters. Context supersession,
flight timeout, and manager shutdown cancel the shared operation.

## Wire and compatibility

Add a proposed `managed_runtime_private_preparation_v1` agentctl capability.
Carry verified cache evidence in initialize failure details and in a typed
utility `ProbeResponse` field. Use a stable
`managed_runtime_cache_incomplete` probe code. Raw evidence stays local.

Extend the authenticated `/api/v1/start` request and `ProbeRequest` with an
optional typed preparation directive. Bind it to the failed generation or
probe attempt and exact trusted package. The request carries no cache path,
executable string, registry URL, or environment replacement chosen by stderr.
The utility owner retains its bounded attempt evidence through retry cleanup.
Clear evidence after success, invalidation, or instance teardown.

Revalidate the directive against the configured command and captured failure.
Negotiate capability through `Client.RequireCapability` before relying on the
new field. Existing empty start requests remain compatible. An older peer
does not receive an ignored repair directive or a destructive fallback.
These contracts stay internal; no browser or MCP package-install API is added.

## Task recovery

Extend `prepareManagedRuntimeStartupRetry` with a verified-cache branch before
the generic unclassified-code rejection. That branch still requires matching
generation, ordinary exit, complete collection, and the typed initialize phase.
Unknown or permanent diagnostics prevent repair.

Reuse `retryManagedRuntimeStartupWithProgress`, `beginStartupRecoveryWithCancel`,
and `stopManagedRuntimeBeforeRetry`. Consume the existing replacement budget
before preparation. Confirm old-process cleanup, keep the existing backoff,
and pass the directive through `Client.Start`. Preparation, validation, and
replacement share the existing 90-second ceiling and earlier caller deadline.
An installer does not count as a second provider-process start.

The session remains STARTING until normal initialization settles. Late events
from the failed generation cannot fail its replacement. After replacement,
perform the original conversation create/load/resume once. A later failure
cannot start another repair. Keep accepted-prompt admission in its current owner.

Apply this initialize-phase path to managed native Codex app-server as well as
ACP. The existing phase name is not permission to exclude the native transport.
Add a native-protocol lifecycle regression rather than proving utility probes alone.

## Startup discovery and profile recovery

Factor the eligible probe replacement policy from `probeManagedRuntime` into
one bounded helper. Agent-wide bootstrap, `probeProfileCapabilitiesFlight`,
and `resolveProfileModelConfigFlight` use that helper with their original
requests and publication guards. Both utility protocol executors return typed
verified evidence. Codex app-server must classify and log sanitized final
causes through the same safe boundary, rather than losing structured stderr.

ETARGET and incomplete-cache repair share one probe replacement budget.
Keep admission limits, exact runtime selection, profile flags, environment,
secret resolution, generation keys, and model/mode options. A changed profile
context cancels preparation and prevents stale replacement/publication.
Profile results never populate the agent-wide catalogue or save a draft.

Startup bootstrap already runs discovery for eligible installed agents.
`GetTaskSessionStatus` already marks a FAILED session resumable when its
resumable execution row and saved token remain valid. Reuse that route after
restart. Do not migrate old failures or clear error rows directly.
Opening an eligible session invokes its normal automatic resume; the new
repair runs inside that operation before the read-only fallback can occur.
Archived, stopped, completed, deferred, and prevent-auto-start cases keep their
current policy. Restart does not start every historical task.

## Visible outcomes and diagnostics

Reuse existing boot retry progress, session recovery cards, and discovery busy
states. Successful repair produces no terminal failure card or read-only notice.
Failure uses existing generic startup copy, safe details, and existing actions.
No new layout, control, or translation key is planned.

Use the existing task transcript as the scroll owner on desktop and phone.
Profile selectors retain their current responsive composition and refresh
targets. Browser tests assert results after restart without a repair click.
Tests also assert failure details, accessible retry, and phone containment.

Structured logs record scope, stable reason, attempt, process generation,
preparation outcome, and elapsed time. Exclude raw diagnostics, cache paths,
registry URLs, and secrets. Preparation failure retains its final safe cause
and actual provider-start count. No new metric label or database schema is needed.

## Verification matrix

| Shape | Required result | Proof |
| --- | --- | --- |
| Managed Claude/Codex ACP, installed manifest but missing execution manifest | Private prepare and same-session retry | Real subprocess inventory and local npm fixture |
| Managed native Codex app-server | Same cache recovery before native initialize | Utility and lifecycle native-protocol tests |
| Other registered managed ACP package | Same generic condition and bin validation | Synthetic second-package fixture |
| Existing FAILED session after backend restart | Normal automatic resume, same token, no repair click | Restart/browser tests and session status regression |
| Agent-wide bootstrap, profile baseline, model options | Matching recovered results, no saved selection changes | Hostutility tests and desktop/phone settings flows |
| Healthy, recovering, exhausted siblings | Healthy process can read files and serve another request | Mixed real-process test with shared sentinels |
| Local Docker and SSH | Preparation occurs on execution host | Existing containers-project fixtures |
| Missing whole tree, unrelated ENOENT, malformed or unreadable inventory | No new repair classification | Negative inventory/classifier tests |
| Native binary, wrapper, passthrough, old peer, unsupported executor | Existing visible failure, no private preparation | Compatibility tests |
| Auth/policy/permissions/disk/stop or post-init error | Existing precedence, no replay | Negative phase and cancellation tests |

Tests use local package archives and a local registry fixture, with no paid
provider requests or public registry dependency. The npm-backed regression
must first reproduce root-manifest `ENOENT`, then prove private materialization
and protocol initialization. It cannot substitute ETARGET or only count retries.

The [delivery package](../../../plans/managed-npm-cache-recovery/plan.md)
owns work order status and exact validation commands.
