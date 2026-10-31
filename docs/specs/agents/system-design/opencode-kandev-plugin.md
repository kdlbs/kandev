---
status: draft
system: agents
created: 2026-10-28
requirements:
  - REQ-AGENTS-OPENCODE-PLUGIN-001
  - REQ-AGENTS-OPENCODE-PLUGIN-002
  - REQ-AGENTS-OPENCODE-PLUGIN-003
  - REQ-AGENTS-OPENCODE-PLUGIN-004
  - REQ-AGENTS-BACKGROUND-WORK-007
  - REQ-AGENTS-BACKGROUND-WORK-009
---

# Kandev integration plugin for OpenCode v2

## Purpose and boundaries

Agents owns the OpenCode integration contract. The
[integration decision](../../../decisions/2026-10-28-opencode-integration-plugin.md)
permits a declared plugin channel alongside ACP. The
[full-continuity design](background-work-opencode.md) still owns automatic turns,
live activity, and future upstream negotiation.

This package supplies one extensible server plugin and its first behavior,
`shell-outcomes` version 1. It reports actual terminal outcomes for jobs that ACP
already identified. It does not infer running activity from an open callback
connection, retained row, or native PID.

All new symbols and paths below are proposed unless explicitly called existing.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| REQ-AGENTS-OPENCODE-PLUGIN-001 | Artifact and launch configuration; Runtime lifecycle |
| REQ-AGENTS-OPENCODE-PLUGIN-002 | Plugin structure; Versioned protocol |
| REQ-AGENTS-OPENCODE-PLUGIN-003 | Ownership; Shell correlation; Durable acceptance; Failure boundaries |
| REQ-AGENTS-OPENCODE-PLUGIN-004 | Compatibility and rollout; Runtime lifecycle |
| REQ-AGENTS-BACKGROUND-WORK-007 | Shell correlation; Projection and visible results |
| REQ-AGENTS-BACKGROUND-WORK-009 | Projection and visible results |

## Verified provider surface

The source baseline is OpenCode 2.2.0, commit
`1097e2668b1e08ad63bdf0b1c4860b9bc01e420c`.

- [Plugin loading](https://github.com/anomalyco/opencode/blob/1097e2668b1e08ad63bdf0b1c4860b9bc01e420c/services/www/src/docs/content/build/plugins/index.mdx)
  supports local plugin directories and a `plugins` array. V2 uses
  `Plugin.define({ id, setup })`, with an optional cleanup return value.
- [Promise plugin context](https://github.com/anomalyco/opencode/blob/1097e2668b1e08ad63bdf0b1c4860b9bc01e420c/packages/plugin/src/promise/plugin.ts)
  exposes `ctx.event`, `ctx.options`, and `ctx.location`.
- [Event subscription](https://github.com/anomalyco/opencode/blob/1097e2668b1e08ad63bdf0b1c4860b9bc01e420c/packages/client/src/shared-events.ts)
  has cancellation and transport-activity support. Its shared stream forwards
  `server.connected` on reconnect, including filtered subscriptions.
- [Native shell events](https://github.com/anomalyco/opencode/blob/1097e2668b1e08ad63bdf0b1c4860b9bc01e420c/packages/schema/src/shell.ts)
  supply `shell.created`, `shell.exited`, and `shell.deleted`. An exit includes
  shell ID, status, and an optional exit code.
- [Shell creation](https://github.com/anomalyco/opencode/blob/1097e2668b1e08ad63bdf0b1c4860b9bc01e420c/packages/core/src/tool/plugin/shell.ts)
  records the native `sessionID` in shell metadata. Initial background intent is
  not a guaranteed native field. Never classify a background launch from that
  event alone.
- [ACP startup](https://github.com/anomalyco/opencode/blob/1097e2668b1e08ad63bdf0b1c4860b9bc01e420c/packages/cli/src/commands/handlers/acp.ts)
  starts a private authenticated server. The plugin must not discover another
  service or extract its credentials.
- [Configuration loading](https://github.com/anomalyco/opencode/blob/1097e2668b1e08ad63bdf0b1c4860b9bc01e420c/packages/core/src/config.ts)
  accepts `OPENCODE_CONFIG_CONTENT` as a virtual configuration layer.

These are source observations, not packaged conformance results. Work order 01
must prove loading, readiness, configuration composition, cleanup, and native
events against the exact installed CLI. A mock producer cannot satisfy that gate.

## Components and responsibilities

| Component | Responsibility |
| --- | --- |
| `apps/packages/opencode-plugin/` | Private workspace package, v2 entry point, module registry, client transport, tests, and deterministic build |
| `internal/agentctl/server/opencodeplugin/` | Embedded artifact, launch configuration, lease, loopback host, typed module receivers, and limits |
| `internal/agentctl/server/process/` | Own host lifetime and the commit-before-publish delivery boundary |
| `internal/agentctl/server/adapter/transport/acp/` | Retain original shell bindings, issue claims, and normalize owned outcomes |
| Existing task background-work service and repository | Persist workload/run revisions and publish normalized updates |
| Existing web background-work hook and viewers | Show retained results on desktop and phone |

The plugin package is a Kandev implementation asset. It has no independent npm
publication or Kandev marketplace lifecycle. Do not add it to the Kandev plugin
registry or use the Kandev plugin SDK.

## Plugin structure

Use the v2 promise API to keep the runtime artifact independent of Effect
internals. Prefer type-only SDK imports and a plain compatible plugin export.
The loader must still see the v2 `id` and `setup` shape. The conformance test must
exercise the shipped export, not a special test entry point.

Separate these concerns:

- `src/index.ts`: the single `kandev` plugin entry point and cleanup.
- `src/core/`: bootstrap decoding, handshake, configuration watch, outbox,
  cancellation, and bounded diagnostics.
- `src/behaviors/registry.ts`: one static list of compiled behavior descriptors.
- `src/behaviors/shell-outcomes.ts`: native shell correlation and terminal facts.
- `src/protocol.ts`: the versioned envelope and closed payload unions.

A behavior descriptor declares ID, version, accepted configuration, start,
configuration update, and cleanup. Core supplies event access, an observation
sink, and its lifetime signal. A fake second behavior proves that the core
requires no shell-specific branches. It remains test-only.

Go receivers use the same IDs and fixture contracts. Adding a behavior requires
an explicit decoder and registry entry on both sides. Contract tests detect
drift. Do not add dynamic imports, remote scripts, arbitrary host actions, or a
second lifecycle manager for future capabilities.

## Artifact and launch configuration

Build a dependency-free ESM directory with `package.json`, `index.mjs`, and an
artifact manifest. Pin `@opencode/plugin` as a development-only type dependency
at the reviewed version. No SDK, Node, Bun, or npm installation is added at
activation time. The selected OpenCode runtime supplies its own execution engine.

Track the generated artifact under the Go embed directory. Its build check
compares regenerated bytes and SHA-256 with the manifest. Go builds and remote
helper cross-compilation consume the checked artifact without pnpm. Include it
in every agentctl build, including compact helper artifacts and desktop runtime
helpers. Distribution through existing agentctl installation avoids a separate
plugin downloader.

Before starting an eligible process, agentctl materializes the artifact under
its owned runtime directory using a content-hash path and atomic publication.
Reuse only a verified regular artifact with the expected bytes. Private launch
bootstrap files are separate from this immutable artifact directory.

Create a child-only `OPENCODE_CONFIG_CONTENT` value. Preserve any existing inline
configuration and append one absolute local plugin-directory entry to `plugins`.
The entry contains only the private bootstrap-file path in its options. Leave
`OPENCODE_CONFIG`, `OPENCODE_CONFIG_DIR`, HOME, XDG roots, cwd, and ACP MCP-server
arguments unchanged. Preserve existing plugin order, options, and configuration
substitution semantics. Reuse the existing `hujson` dependency for JSONC input.

The appended v2 entry has this shape. Paths are executor-local examples.

```json
{
  "package": "/private-runtime/opencode-plugin/artifact-hash",
  "options": {
    "bootstrapFile": "/private-runtime/launch-lease/bootstrap.json"
  }
}
```

Do not flatten global, project, managed, or file configuration into a new file.
Do not write `.opencode/plugins`, package manifests in user projects, or global
configuration. A malformed inline value or reserved-ID conflict disables this
integration with a bounded diagnostic. It must not erase or repair user data.
Do not silently replay a prompt after an integration startup error.

An exact duplicate of the Kandev-owned entry is idempotent. An unrelated entry
using the reserved `kandev` ID is a conflict, not authority to replace it.

## Ownership and private transport

Agentctl creates a random launch lease before process start. Capture instance
identity, actual process generation, selected runtime family/version, execution
owner, and the plugin artifact hash. Backend task ownership comes from existing
instance configuration, never from callback-supplied task IDs.

Use a dedicated HTTP listener bound to `127.0.0.1:0` in the executor. Its router
contains only `/v1/hello`, `/v1/config`, and `/v1/events`. It uses a random
per-launch bearer credential even when ordinary application authentication is
disabled. Do not exempt another route from control authentication or pass the
agentctl control credential to the plugin.

Store endpoint, credential, lease ID, protocol version, and artifact identity in
a mode-0600 bootstrap file outside the workspace. Use the platform's private
runtime-directory protection on Windows. Reject redirects and non-loopback
targets. Configuration options contain the path, not the credential.

The lease authorizes only event reporting and declared behavior configuration.
The plugin is trusted code inside the execution owner's sandbox. This design
does not claim isolation from arbitrary code already running as that same owner.

Maintain a separate native-session generation within the process lease. ACP
new/load/resume establishes its authoritative native ID. Reset, close, or a
replacement generation removes its claims. A callback cannot select another
session merely by supplying its ID.

## Versioned protocol

Protocol version 1 has three operations:

1. `hello`: negotiate the artifact, compatible runtime, and enabled behavior
   versions. The plugin returns readiness only after a real native subscription
   barrier. Use the pinned SDK's connection signal, not a fixed sleep.
2. `config`: a revisioned long poll returns a complete configuration for the
   negotiated modules. The initial shell module receives exact owned claims.
   Polls wait at most 15 seconds. This is integration configuration, not a
   process-status poll or a prompt to the model.
3. `events`: send bounded typed observations with lease, native generation,
   behavior ID/version, producer ID, sequence, and stable event ID.

The initial payload is `shell.terminal`: native session ID, shell ID, actual
native terminal status, optional exit code, and an optional native completion
time. Omit unavailable values. No command text, output, environment, PID,
reasoning, or conversation content belongs in this payload.

Configuration carries claims from Kandev to the plugin. It cannot invoke
arbitrary module methods. Unknown modules receive no authority. Unknown ingress
payload kinds or versions fail before projection.

Initial hard limits:

| Resource | Limit and response |
| --- | --- |
| Encoded event request | 64 KiB and 64 observations; reject larger requests |
| Pending sender outbox | 256 observations or 1 MiB; report a gap on exhaustion |
| Unclaimed terminal facts | 256 facts, 1 MiB total, 60-second lifetime |
| Owned live claim set | 256 shells per process; excess work retains legacy inspection |
| HTTP request timeout | 5 seconds, excluding the 15-second configuration long poll |
| Retry delay | Exponential delay from 250 ms through 5 seconds, with jitter |
| Plugin readiness budget | 5 seconds; timeout retains legacy ACP behavior |

Cancellation stops all watches and requests. Background delivery must not block
native event draining or OpenCode setup. Readiness completion and event intake
use independent bounded tasks. A module failure closes only its own capability.

## Shell correlation

Keep `confirmBackgroundLaunch` as the existing launch authority. It requires a
background input plus structured shell ID and running-result metadata. The
plugin never creates job rows from `shell.created` alone.

Retain an immutable binding outside `activeToolCalls`. Capture shell ID, native
session/generation, original tool-call ID, source prompt generation, original
delivery attribution, workload/run ID, and inspection capabilities. Preserve
this binding when `settleUnverifiedOpenCodeBackgroundWork` removes the active
launch tool at prompt end.

Publish each binding as a shell-module claim. The native observer records
`shell.created` metadata and joins later exits by shell ID. It retains a bounded
terminal fact until a matching claim arrives. This covers a fast completion
before ACP delivers its launch result. Foreground shells receive no claim and
cannot create background rows or block unrelated outcomes.

Only a claimed shell can enter the sender outbox. Each fact retains the native
session association captured at creation. Shell exit events without that
association remain unusable evidence. A deletion or a missing cache entry does
not imply successful completion.

The host rechecks the current claim on every receive and immediately before
commit. Removed, foreign, stale, or conflicting claims have no effects. A
bounded gap diagnostic explains an expired early result or a cache overflow.

## Durable acceptance and ordering

Use the existing `process.Manager.persistDeliveryEvent` and ordered
`deliveryEventWriter`. Add one proposed process-manager ingress method for
validated native plugin observations. It must capture immutable launch
attribution before the journal boundary. It must not read the current active
submission to attribute an older job's outcome.

The ACP binding normalizer produces an existing
`streams.EventTypeBackgroundWorkUpdated` with a
`streams.WorkloadRunObservation`. Preserve the original source call and origin
turn. Actions, streamed output, parentage, and attributable usage remain absent.

The ingress joins the same ordered persistence and wakeup path as adapter
updates. A successful HTTP acknowledgement follows journal commit. Do not
acknowledge an in-memory queue insertion. When durable storage is unavailable,
disable this module or return a retryable error. Preserve the existing durable
delivery error and recovery behavior.

Derive the observation's stable delivery event identity from the launch lease,
native generation, behavior version, native session, and shell terminal fact.
Persist idempotency with the journal record, rather than a memory-only receipt.
Reuse journal identity checks. A retry after a lost HTTP acknowledgement must
return the committed receipt without a second projection.

Claims remain available until durable acceptance. After commit, the host can
remove the active claim while keeping bounded receipt evidence. A retry can
still read its receipt after claim removal. Mismatched payloads under the same
event ID fail closed. Preserve terminal monotonicity in the shared reducer.

Do not emit a root `complete`, create a turn, resolve a submission, or increment
workflow-completion telemetry for a job observation. Test a terminal arriving
while another user turn is active and while no foreground turn exists.

## Projection and visible results

Map actual native facts as follows:

| Native evidence | Shared state |
| --- | --- |
| `exited`, exit 0 | `completed`, exit 0 |
| `exited`, nonzero exit | `failed`, actual exit |
| `timeout` | `failed`, actual exit only when supplied |
| `killed` | `interrupted`, actual exit only when supplied |
| `exited`, missing exit | `ended`, outcome unavailable |
| Missing exit event, deletion alone, stream gap, or expired fact | Retain `unknown` |

The existing task recorder owns persistence in
`task_session_background_work` and `task_session_background_runs`. No SQL table,
new client store, provider-specific UI branch, or conversation message is added.

Use the existing summary, detail tabs, history, and phone detail surface.
Terminal jobs leave the active count. Unknown jobs retain their current meaning
and count. This package does not change the badge formula.

Output remains unavailable because this module has no output capability. Reuse
existing localized status and exit-code presentation. Unsupported controls stay
absent. Background updates preserve drafts, focus, and current prompt-admission
behavior on both viewports.

## Runtime lifecycle

Integrate preparation after final runtime resolution and before
`process.Manager.buildFinalCommand`. The existing ACP `PrepareEnvironment` is a
no-op; it must not become a provider-config side effect for every ACP agent.
Use a narrow OpenCode launch-preparation seam in the process manager, with typed
resolved runtime evidence passed from lifecycle configuration.

Start the listener before the OpenCode subprocess. Supply the composed inline
configuration once. Observe handshake readiness before enabling shell claims.
A readiness timeout leaves ordinary ACP usable and the plugin capability off.
Do not delay unrelated providers or non-task utility sessions.

Start the readiness budget when the prepared location attaches and can activate
its plugin. Do not include package installation or CLI boot in that budget.
Require the subscription barrier before activating claims for the first prompt.

Stop/reset closes or revokes the applicable lease and native claims before
process resources are reused. Stale callbacks cannot recreate removed work.
Cleanup affects only Kandev-owned bootstrap and artifact files.

Plugin configuration watches do not count as user activity or prevent runtime
parking. Existing executor lifetime and parking rules retain authority. When
they stop the provider, the plugin loses its live channel. Missing terminal
evidence retains the existing unknown or outcome-unavailable history.

A retained agentctl retains its listener, plugin producer, claims, and journal
across backend disconnects. Existing authenticated adoption and replay remain
authoritative. Adopting it must not call ACP initialization or inject another
plugin. A retained older agentctl without the integration capability stays on
the legacy path. A replacement agentctl does not claim a surviving plugin lease
without existing recovery ownership proof.

This first version provides no replay of native events lost during plugin or
OpenCode downtime. A new process starts a new lease. Browser refresh can recover
only results already committed by agentctl or projected by the backend.

## Compatibility and rollout

Introduce `features.opencodePlugin` /
`KANDEV_FEATURES_OPENCODE_PLUGIN` with restart-required metadata. Its initial
prod, dev, and e2e profile defaults are false. An explicit installation override
enables automatic preparation on new eligible launches. Later promotion is a
separate change.

Follow the typed registry, configuration, frontend contract, and profile
completeness rules. Carry the effective Boolean and resolved runtime evidence
through lifecycle, agentctl startup/instance configuration, and process setup.
Do not read a parallel ambient feature environment inside the plugin.

The plugin host is enabled by this toggle. `shell-outcomes` additionally
requires `features.agentBackgroundWork`. With that shared feature off, no shell
subscription, claim, or workload observation starts. The module list is a
capability selection, not another feature registry.

Initial positive compatibility is the reviewed OpenCode 2.2.0 artifact.
Additional v2 versions require recorded conformance before joining the manifest.
Supported managed and native sources use the same evidence and handshake.
V1, unknown native versions, unsupported v2 versions, future majors, custom
unverified wrappers, and older agentctl peers preserve existing ACP behavior.

Choose one observation authority per module. A future proven equivalent ACP
capability disables the matching plugin module. The generic plugin remains
available for other approved behaviors. Do not use a release number alone to
claim upstream support.

## Failure boundaries and diagnostics

Plugin installation, handshake, or delivery errors do not supply success or
running evidence. Preserve ordinary ACP and current inspection-only capability.
Do not retry accepted prompts or manipulate native session storage to recover
plugin delivery. Historical unknown rows remain untouched.

Record bounded structured diagnostics for compatibility, preparation, readiness,
lease rejection, gap, queue exhaustion, and durable acceptance failure. Keep
lease, shell, session, and execution IDs in logs only. Reuse existing durable
delivery metrics. Any new counters have fixed behavior/result labels and no
identity, path, URL, credential, or command labels.

## Delivery and validation

[Implementation package](../../../plans/opencode-kandev-plugin/plan.md) owns
sequential work orders, precise tests, and desktop/phone previews. Package
conformance, deterministic receiver tests, real adapter integration, browser
checks, and remote-executor proof remain distinct evidence categories.

The related [full-continuity package](../../../plans/opencode-background-continuity/plan.md)
keeps its upstream tasks blocked. This package does not satisfy those tasks or
activate automatic-turn delivery, liveness, or steering.
