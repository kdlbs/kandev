---
status: current
system: agents
created: 2026-11-08
updated: 2026-11-08
requirements:
  - REQ-AGENTS-MANAGED-RUNTIME-RECOVERY-002
  - REQ-AGENTS-MANAGED-RUNTIME-RECOVERY-004
  - REQ-AGENTS-MANAGED-RUNTIME-RECOVERY-006
  - REQ-AGENTS-MANAGED-RUNTIME-RECOVERY-007
---

# Reusable managed npm repair design

## Boundary and mapping

Agents owns installation selection and preparation on the execution host.
This design extends the existing [private recovery primitive](managed-npm-cache-recovery.md).
The [ADR](../../../decisions/2026-11-08-reusable-managed-npm-repairs.md) records the lifetime and publication choice.
Requirements 002, 004, and 006 remain the authority for eligibility and retry safety.

| Requirement | Design sections |
| --- | --- |
| 002 | Executor ownership; installation identity; compatibility |
| 004 | Recovery flow; cancellation and concurrency |
| 006 | Existing evidence boundary; task and utility integration |
| 007.1-.4 | Existing evidence boundary; lookup and recovery flow |
| 007.5-.7 | Store and installation identity; publication |
| 007.8-.10 | Cancellation and concurrency; file lifetime |
| 007.11-.14 | Failure behavior; compatibility |
| 007.15-.16 | Visible outcomes; verification |

## Existing evidence boundary

The completed requirement-006 package supplies `ManagedRuntimeCacheEvidence`,
`ManagedRuntimePreparationDirective`, `PreparePrivateRuntime`, and
`PreparedRuntime.RewriteNpxCommand`. Reuse those boundaries.

Eligibility still requires an exact registered command, a matching owned
pre-initialize failure, complete diagnostics, and independent missing-manifest inventory.
Neither a closed pipe nor a missing bin link grants repair authority alone.
The new store does not broaden npm error classification or requirement 005.

The observed defect is the repair's lifetime. `PreparedRuntime.Close` currently
removes the successful private tree after consumer cleanup.
`managedRuntimePreparationState` also retains utility roots only for their owner.
Consequently, a later cold launch can select the same incomplete shared entry.

## Components and integration points

Proposed additions are named explicitly here. They do not exist yet.

- Add `RepairStore`, `RepairContext`, and a generation handle under `internal/agent/managedruntime/`.
- Extend `PreparedRuntime` with an owned-store allocation path and an explicit publication lifetime.
- Keep ordinary `PreparePrivateRuntime` cleanup unchanged for legacy or ineligible operations.
- Construct trusted installation-environment provenance in `runtime/lifecycle/environment_resolution.go` and utility request construction.
- Carry that provenance through the authenticated instance and utility contracts.
- Integrate task lookup in `process.Manager` before final command preparation.
- Publish task repairs at successful `agent.initialize`, before conversation creation or restoration.
- Integrate ACP and native utility lookup/publication in their common preparation boundary.
- Pass `config.InstanceConfig.HomeDir` to the utility handler from `api.Server`.
- Keep lifecycle and hostutility as the existing recovery-policy owners.

The backend sends no repair directory or executable path. Agentctl derives
storage paths locally. Runtime observations read the installation actually selected.

## Store and installation identity

### Storage root

Use `<agentctl HomeDir>/managed-runtime-repairs/v1/` on the execution host.
`config.InstanceConfig.HomeDir` already identifies this Kandev installation.
Do not derive remote storage from a backend-host path.
Validate the root, owner, permissions, and containment before use.
Reject workspace containment and roots that resolve through the mounted provider state.
If a persistent safe root is unavailable, retain existing private recovery.

The root contains an owner marker, a private fingerprint key, per-context
lock files, generation directories, and one selection record per context.
Directories use `0700`. Key and record files use `0600` on Unix.
Windows applies the existing supported user-ownership and file-access rules.
No SQL table or user configuration key is added.

### Context construction

The context contains:

- Exact trusted package spec, selected bin, and managed protocol shape.
- Authenticated runtime-owner scope, execution-host user, and platform, including architecture and relevant libc identity.
- Resolved Node/npm executables and versions.
- The original effective npm cache and isolated project identity.
- Effective user/global npm configuration and their bounded file contents.
- Authorized installer environment and strip policy, after executor/profile precedence.

Capture installer inputs before Kandev injects task IDs, transport credentials,
temporary MCP endpoints, or prompt payloads. Include every user/profile/executor
environment input. Do not filter unknown variables by a guessed npm allowlist.
The provider process still receives its full authorized runtime environment.

Use explicit provenance, not variable-name heuristics, to separate generated
runtime values from installer inputs. If provenance is absent, do not reuse.
This is necessary for reuse across different tasks without merging user contexts.
Derive the owner scope from trusted task/profile authorization, never from task
IDs or a caller-provided environment variable. System utility work has a distinct scope.

Agentctl completes the snapshot with its inherited environment and npm inputs.
The normal isolated prefix continues to exclude repository `.npmrc` effects.
Normalize environment keys according to the execution platform.
Recheck configuration and context before installation, selection, and publication.
Unreadable, oversized, changed, or ambiguous inputs prevent reusable publication.

Derive the directory key with HMAC-SHA256 over the canonical context.
Keep the random HMAC key private to this store and stable across restart.
Create the key exclusively. A concurrent creator reads the winning key.
An unreadable or malformed key disables reuse instead of silently replacing identity.
Never persist raw environment values, npm credentials, registry URLs, or ordinary
credential hashes in generation metadata or logs.

### Generation records

A generation has an opaque ID, schema version, owner marker, package identity,
context digest, project/cache subdirectories, and bounded validation metadata.
A selection record names one generation by ID, never by an arbitrary path.
Readers reject unknown versions, traversal, symlinked roots, mismatched ownership,
wrong package/bin identity, absent manifests, and escaped executable targets.
Reuse existing `InspectNpxExecutionTree` and platform bin validation.
An invalidation must match the observed generation under the context lock.
A delayed invalidation cannot clear a newer successful publication.

Only the requested context is read. No startup-wide cache inventory is added.
All record reads have a fixed 64 KiB bound.

## Lookup and recovery flow

### Normal cold launch

1. Resolve the trusted logical command and installation context.
2. Look up and validate the matching published generation locally.
3. On a valid hit, use its project/cache through `RewriteNpxCommand`.
4. On a miss, keep the original normal command and npm cache.
5. Start the process and use the existing initialize/create/load/prompt sequence.

A lookup does not install packages, contact the registry, consume a replacement,
change a saved profile, or touch a running retained execution.
Normal `--prefer-offline` behavior remains unchanged.
An invalid record loses selection eligibility. Its generation files remain intact.

### First verified incomplete-cache failure

1. Existing recovery consumes its single replacement allowance.
2. Stop and reap the failed process through its current owner.
3. Apply the existing cancellable backoff and remaining recovery deadline.
4. Acquire the lock for this installation context and recheck its selection.
5. Reuse a generation that another caller already published, or prepare one new generation.
6. Validate package identity, manifest, bin link, and executable containment.
7. Start the replacement with that generation's project/cache.
8. After successful protocol initialization, publish the generation and release the lock.
9. Continue the original conversation operation once.

The npm preparation command remains the existing exact-package `npm exec` operation.
Do not hand-write npm manifests, copy the incomplete tree, or run a model prompt.
Initialization success is the publication boundary. Restoration success is not required.
A saved conversation error must not invalidate an otherwise valid installation.

## Publication and file lifetime

Allocate a generation at its final stable directory before installation.
Keep it unselected until publication. Do not rename an installation after its
child starts, because the child can retain absolute paths.

After file validation and successful initialization, write a temporary selection
record in the same directory. Flush it and replace the record atomically.
Use platform-safe replace behavior and flush the directory where supported.
Validate owner, context, and process/probe generation immediately before publication.
Readers see either the previous complete record or the new complete record.

Published generation handles release process ownership without removing files.
Session teardown, utility response completion, backend detach, and adoption do
not remove published generations. Kandev never rebuilds these generations in place.
npm can perform its ordinary metadata writes within a selected cache.
An invalid generation receives no new consumers, but existing consumers retain their files.

Unpublished generations retain the current cleanup rule: remove only the exact
owned root after all installer and provider descendants stop.
After an unproven crash boundary, retain files rather than guessing that consumers stopped.
Automatic garbage collection and orphan sweeping are excluded from this package.
Explicit maintenance requires stopped consumers and exact owned-generation validation.

## Cancellation and concurrency

Use a cancellable OS file lock per context across local agentctl processes.
Reuse the platform lock patterns under `common/passthroughconfig` without
coupling runtime repair to the MCP lock namespace.
The proposed repair lock belongs to `agent/managedruntime`.

The initiating task/probe owns preparation through its existing operation context.
Its cancellation stops and reaps owned children before unpublished cleanup.
Waiting callers have independent contexts and deadlines.
They recheck the selected generation after acquiring the lock.
If the builder cancels or fails, the next eligible waiter can become the builder.
It uses its remaining original budget, never a new 90-second allowance.

Hold the context lock through candidate initialization and publication or failure cleanup.
Do not hold manager state mutexes over npm, lock waits, or protocol I/O.
Existing start-operation serialization must leave stop and cancellation usable.
Different contexts and already-running agents continue independently.
Abrupt owner exit releases the OS lock. An unpublished directory grants no reuse authority.

For tasks, the pending publication ticket belongs to the exact process generation.
It outlives the HTTP start response, but not the original recovery deadline.
Carry the remaining recovery deadline in the authenticated preparation request.
Agentctl caps it at 90 seconds and rejects an expired or malformed value.
Use the instance-owned cancellation context after the start request returns.
Stop, configure, initialization failure, and deadline expiry retire the ticket.
If initialization never arrives, reap the unpublished candidate and release its lock.
Utility publication remains inside the existing owned probe operation.

Existing startup and process-generation fences remain authoritative.
Late events from a failed process cannot publish, invalidate, or stop a successor.
Profile supersession cancels its existing flight. One HTTP waiter cannot cancel other
waiters in that already-shared profile flight.

## Task and utility boundaries

Add an optional typed `ManagedRuntimeInstallContext` to authenticated create/configure
and utility requests. It contains installer environment provenance, not physical paths.
Advertise the proposed `managed_runtime_repair_reuse_v1` capability.
Require matching protocol/schema support before sending or relying on this field.
Retain `managed_runtime_private_preparation_v1` as the independent legacy fallback.

Task `process.Manager` owns the selected generation and publication attempt.
The authenticated initialize handler reports successful initialization to that exact owner.
The callback must carry immutable instance and process-generation evidence.
It must not reread a newer mutable process identity or call back into a held manager lock.

`utility.managedRuntimePreparationState` uses the same store for ACP and native Codex probes.
Agent-wide discovery, profile baseline, and model-option operations retain separate result
publication guards even when their compatible installation files are shared.
Never populate the agent-wide catalogue from a profile result.

Evidence must inspect the cache actually used by the failed process.
For a selected repair generation, derive that root from its owned handle.
Do not rediscover the default user cache or trust a diagnostic path.
Runtime observations also use the actual selected project/cache.

## Failure behavior and compatibility

| Condition | Behavior |
| --- | --- |
| No published match | Original normal launch; requirement-006 recovery remains available |
| Store unavailable or unsafe | No reuse; existing private preparation if eligible |
| Changed context or selected version | Cache miss for that context; no runtime-selection write |
| Invalid generation | Remove only its selection eligibility; retain files for existing consumers |
| Installation or initialization fails | Do not publish; existing final error; no third agent start |
| Publication fails after initialization | Keep the current process and its private lifetime; log failure to publish |
| Cancellation or teardown | Stop owned unpublished work; preserve every published generation |
| Old peer or missing provenance | Current private recovery; no untrusted or silently ignored reuse field |

Initial execution hosts remain local PC, local Docker, and SSH.
Reuse lasts only while that host's Kandev store persists.
A newly created container is a new storage lifetime.
Sprites, Kubernetes, remote Docker, custom commands, wrappers, and passthrough remain
outside this extension unless the existing contract already validates their path.
No healthy retained peer is restarted to obtain the new capability.

## Visible outcomes and diagnostics

Reuse the existing transcript, boot row, recovery card, and profile loading states.
First recovery can show attempt 2 of 2. A later successful first attempt shows
ordinary startup or resume status, without preparation progress or a repair click.
There is no new copy, layout, control, translation key, or responsive composition.
Desktop and phone use their current entry points and scroll owner.

Add structured result events with closed outcomes: `miss`, `hit`, `invalid`,
`prepared`, `published`, and `publish_failed`.
Record actual start count and elapsed time through existing recovery diagnostics.
Exclude filesystem paths, registry URLs, fingerprints, raw stderr, and secrets.
Do not add identifier-valued metric labels or a new metrics subsystem.

## Verification

Use the existing loopback registry and local npm fixtures. No test requires
public npm, provider credentials, or model inference.

- Reproduce real root-manifest failure, recover, stop all owners, then cold-launch
  the same context once without another installation or registry request.
- Repeat after a fresh store owner and agentctl/backend restart.
- Cover ACP, native Codex app-server, both selected OpenCode families, and a generic registered fixture.
- Vary user, package version, npm configuration, credentials, and arbitrary installer environment values.
- Change only Kandev-generated session/transport inputs and prove compatible installation reuse.
- Cancel the builder with another waiter present; let the waiter recover within its own budget.
- Return the start response before initialize; preserve the ticket until initialize or its original deadline.
- Abandon initialization and prove that no permanent lock or unpublished process remains.
- Exercise two OS processes publishing one context, plus independent contexts.
- Reject partial records, malformed metadata, symlinks, escaped bins, and changed inputs.
- Deliver a delayed invalidation after successor publication and preserve the successor selection.
- Keep a real healthy consumer reading an old generation while a successor publishes.
- Assert no publication for failed initialization, and current-process continuity on publication failure.
- Assert first-recovery and later-first-attempt outcomes through desktop and phone session/discovery flows.
- Prove Docker and SSH use execution-host storage, not the backend store.

The [plan](../../../plans/managed-npm-repair-reuse/plan.md) owns exact commands
and implementation status. Product tests remain pending until implementation.
