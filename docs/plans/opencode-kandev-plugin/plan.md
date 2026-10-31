---
created: 2026-10-28
status: in_progress
requirements:
  - REQ-AGENTS-OPENCODE-PLUGIN-001
  - REQ-AGENTS-OPENCODE-PLUGIN-002
  - REQ-AGENTS-OPENCODE-PLUGIN-003
  - REQ-AGENTS-OPENCODE-PLUGIN-004
  - REQ-AGENTS-BACKGROUND-WORK-007
  - REQ-AGENTS-BACKGROUND-WORK-009
system_design:
  - ../../specs/agents/system-design/opencode-kandev-plugin.md
  - ../../specs/agents/system-design/background-work-opencode.md
  - ../../specs/agents/system-design/opencode-v2-adoption.md
legacy_specs: []
---

# Implementation plan: Kandev integration plugin for OpenCode v2

## Overview

Supply one OpenCode v2 integration plugin automatically during Kandev launches.
Keep installation, ownership, delivery, and cleanup shared across explicit
behavior modules. Deliver native shell terminal outcomes as the first module.

The accepted scope is a plugin foundation and shell status/exit-code delivery.
ACP remains the conversation transport. Automatic assistant responses, live
background activity, streaming output, and process controls remain separate.

Implementation is in progress. Work orders 01 through 04 and 06 are complete.
Work order 05 has packaged agentctl proof but still needs bridge-enabled browser
and remote-executor runs. No commit, push, publication, or promotion is included.

## Inputs and ownership

- [Integration requirements](../../specs/agents/requirements/opencode-kandev-plugin.md).
- [Background-work requirements](../../specs/agents/requirements/background-work.md),
  especially 007.6 through 007.9 and the existing 009.4 through 009.6.
- [Integration design](../../specs/agents/system-design/opencode-kandev-plugin.md).
- [Integration decision](../../decisions/2026-10-28-opencode-integration-plugin.md).
- [Runtime selection](../../specs/agents/system-design/opencode-v2-adoption.md).
- [Full continuity package](../opencode-background-continuity/plan.md).

Agents owns this provider contract. Tasks, Platform, and the shared viewer
consume its results. The new asset is an OpenCode server plugin, not a plugin
for the Kandev marketplace or its runtime SDK.

### Confirmed choices

- Kandev supplies and loads the plugin at launch. Users do not install it.
- Only supported OpenCode v2 task sessions receive it. V1 remains untouched.
- One integration owns explicit behavior modules. Only shell outcomes ship here.
- Existing user configuration and plugins retain their meanings.
- Actual terminal evidence can settle jobs after the original foreground turn.
- The first version can still show unknown activity between prompt end and an
  actual terminal event. It adds no liveness guarantee or badge-count redesign.

### Verified baseline and proof boundary

Source inspection used OpenCode 2.2.0 at commit
`1097e2668b1e08ad63bdf0b1c4860b9bc01e420c`. V2 provides a `plugins` configuration
array, `Plugin.define({ id, setup })`, cleanup, and native event subscriptions.
ACP starts a private server. A default-service HTTP poller is not an owned-job
observer. Native shell creation does not reliably expose background intent.

[Protocol evidence](protocol-evidence.md) separates those facts from pending
packaged conformance. Task 01 must prove the exact plugin export, subscription
barrier, inline configuration merge, and cleanup before later tasks activate it.
An unsupported result preserves legacy ACP. Do not hide a failed proof with a
sleep, a database reader, or an unrelated server connection.

## Scope

### In scope

- A private workspace package and deterministic embedded ESM artifact.
- Automatic executor-local staging and additive launch configuration.
- Versioned bootstrap, behavior negotiation, claims, authenticated callbacks,
  bounded retries, and cleanup.
- A static behavior registry with a test-only second module.
- A `shell-outcomes` module for confirmed background shell identities.
- Existing ordered journal, workload projection, WebSocket, and viewer paths.
- Enabled, disabled, incompatible, early-result, later-turn, and replay cases.
- Packaged CLI, desktop, phone, and executor evidence.
- A restart-required release toggle, initially off in every shipped profile.

### Out of scope

- Native HTTP conversation transport, a forked CLI, or a separate npm release.
- OpenCode v1 hooks and unverified future v2 releases or majors.
- Arbitrary module loading, executable RPCs, prompts, tools, or host mutations.
- Output streaming, shell controls, automatic assistant text, usage attribution,
  active-background probes, and prompt steering.
- A new database table, client store, provider-specific viewer, or badge formula.
- Historical-job backfill and replay across lost native runtime events.
- Automatic promotion of the new feature or completion of the upstream plan.

## Technical approach

### Artifact and extension seam

Add `apps/packages/opencode-plugin` as a private pnpm package. Its source splits
the v2 entry point, core delivery, behavior registry, and shell module. Build a
dependency-free ESM directory. Keep the generated runtime asset and digest under
`apps/backend/internal/agentctl/server/opencodeplugin/assets` for Go embedding.
Go builds consume that checked artifact without a JavaScript installation.

Use the v2 promise API with type-only SDK dependencies. Pin the reviewed type
package. A typed static registry shares lifecycle and transport. A second module
exists only in tests to prove the extension seam.

### Launch and feature wiring

Reuse the resolved OpenCode family/source/version from runtime adoption. Carry
typed integration configuration through the existing lifecycle and instance
configuration chain. Add a narrow process-manager preparation step after final
environment resolution and before `buildFinalCommand`.

Stage the artifact atomically in the executor. Compose the child-only
`OPENCODE_CONFIG_CONTENT` layer with `hujson`. Append one absolute plugin path
and private bootstrap-path option. Preserve `OPENCODE_CONFIG`, original inline
fields, user plugin order/options, managed policies, and ACP MCP injection.
Respect explicit environment stripping and policy exclusions. Never write the
user's global or project configuration.

Introduce `features.opencodePlugin`, Go `FeaturesConfig.OpenCodePlugin`, and
`KANDEV_FEATURES_OPENCODE_PLUGIN`. Register it once in the typed registry with
restart-required metadata. Set prod/dev/e2e defaults false. Update the frontend
all-off feature contract and existing flag-label localization where applicable.
The shell module also requires `features.agentBackgroundWork`.

### Private host and owned claims

Agentctl owns a dedicated loopback listener and private launch lease. It does
not expose control credentials or change authentication exemptions. Bootstrap
files stay outside the workspace. All requests use a scoped credential.

The callback protocol has `hello`, a revisioned configuration watch, and bounded
event batches. Configuration supplies explicit claims for ACP-confirmed jobs.
It is not a general command channel. The plugin caches early native results
until a claim arrives. Ordinary foreground shells cannot create jobs.

### Terminal projection and persistence

Retain original shell bindings after foreground settlement. Capture the native
session/generation, source call, original delivery attribution, and workload/run
identity. Never attribute a late result to the current user turn.

Add native observation ingress at `process.Manager` and reuse
`persistDeliveryEvent` / `deliveryEventWriter`. Acknowledge only after a durable
commit. Persist stable event identity for retry after a lost acknowledgement.
Normalize into `streams.WorkloadRunObservation` and the existing
`background_work_updated` path. Shared persistence owns final revision and
terminal monotonicity. No root completion event is emitted.

### Compatibility matrix

| Provider and shape | Behavior | Required evidence | Fallback |
| --- | --- | --- | --- |
| Managed OpenCode 2.2.0, v2 plugin ready, owned claim | Native shell terminal result | Packaged CLI and actual artifact | Unknown on missing evidence |
| Native OpenCode 2.2.0 with the same proved contract | Same launch integration | Native-path version and plugin tests | Skip plugin on uncertainty |
| Other v2 version | Activate only after recorded conformance joins the manifest | Exact artifact/version result | Ordinary ACP |
| OpenCode v1, unknown version, future major, custom wrapper | No plugin injection | Negative launch tests | Ordinary ACP |
| New flag off | No runtime files, listener, credential, or claims | Disabled-path tests | Current legacy inspection |
| Shared background feature off | No shell behavior or workload observations | Module-disabled tests | Existing conversation behavior |
| Old retained agentctl | No hot installation or capability inference | Mixed-version adoption tests | Existing retained runtime |
| Claude, Codex, other ACP | No OpenCode launch side effects | Cross-provider regressions | Existing provider behavior |
| Future verified equivalent ACP capability | Select one source for the matching module | Capability and duplicate-source tests | Preserve unrelated modules |

## ASCII UI preview

The package reuses existing components and localized copy. These views specify
state changes, history access, and navigation. They add no new controls or copy.
Spacing is illustrative. Both map to BACKGROUND-WORK-007.6/.7 and -009.4/.5/.6.

### UI-01: Desktop job history

Entry: the existing background-work chip above the composer and its history.

```text
Before terminal evidence:             After terminal evidence:
[1 Background Job]                   [Background Work history]
  Tests       Unknown                  Tests       Completed   exit 0
  [Open]                               [Open]

Named central detail tab: Tests
  Completed   exit 0
  Output is not available for this job
```

The history entry remains reachable after the active count reaches zero. The
existing detail tab owns its content scroll. Unsupported process controls stay
absent. The composer draft and focus remain intact.

### UI-02: Phone job history

Entry: tap the existing chip/history entry, then select a job in its drawer.

```text
Background Work drawer
  Tests       Completed   [Open]

Full-height detail
  [Back] Tests
  Completed   exit 0
  Output is not available for this job
```

Reuse the shipped phone drawer and full-height detail surface. Keep one content
scroll owner, safe-area clearance, 44px touch targets, Back navigation, and focus
return. Failed and unknown outcomes use the same existing layouts.

## Tests

| Criteria | Planned executable evidence |
| --- | --- |
| PLUGIN-001.1/.2/.3/.4/.5, PLUGIN-004.3/.4 | `test/conformance.test.mjs`: packaged loading, actual v2 export, readiness, additive config, cleanup, and version mismatch |
| PLUGIN-002.1 through .5 | `test/registry.test.mjs`, `test/core.test.mjs`, Go `TestBehaviorRegistryContract` |
| PLUGIN-003.1/.2/.5/.8 | Go `TestOpenCodePluginLease`, `TestOpenCodePluginClaims`, `TestOpenCodePluginRedaction` |
| PLUGIN-003.3/.4/.6 | Go `TestOpenCodePluginDurableAck`, `TestOpenCodePluginLostAck`, `TestOpenCodePluginDeliveryGap` |
| PLUGIN-003.7, BACKGROUND-WORK-007.6/.7/.8 | `TestOpenCodePluginOutcomeKeepsOrigin`, `TestOpenCodePluginOutcomeDuringSuccessorPrompt`, `TestOpenCodePluginUnknownFallback` |
| PLUGIN-001.6/.7, PLUGIN-004.1/.2/.5/.6 | Launch, adoption, flag/profile contract, disabled-module, and authority-selection tests |
| BACKGROUND-WORK-007.6 through .9, -009.4 through .6 | Existing reducers plus bridge-enabled desktop/phone and executor scenarios |

Names above are proposed tests. Work orders name their files and exact commands.
Use TDD for logic. Do not create tests that only compare implementation constants.

## E2E tests

Extend `tests/task/opencode-background-work.spec.ts` and
`tests/task/mobile-opencode-background-work.spec.ts`. Preserve their legacy
unknown-path scenarios with the new flag off. Add bridge-enabled scenarios using
the real receiver and observation path. Do not substitute direct
`recordBackgroundWorkload` calls for bridge delivery in the new assertions.

Add an isolated packaged-provider smoke harness with a local deterministic model
endpoint. It must use the selected real OpenCode CLI, actual shipped plugin,
launch injection, callback receiver, journal, backend, and browser. No user
credentials, live database, project plugins, or external model calls are allowed.
Keep this proof distinct from deterministic mock-provider browser coverage.

| Scenario | Desktop | Phone | Main criterion |
| --- | --- | --- | --- |
| Successful job settles after foreground end | Required | Required | 007.6/.7, 009.5 |
| Nonzero job outcome retains failure and exit code | Required | Required | 007.6, 009.6 |
| Outcome arrives during a later prompt | Required | Shared state coverage | 007.7, PLUGIN-003.7 |
| Fast job ends before claim delivery | Required | Shared state coverage | PLUGIN-003.5 |
| Duplicate callback and browser refresh | Required | Required | PLUGIN-003.3/.4 |
| Plugin off, incompatible, or disconnected | Required | Required | 007.8/.9, PLUGIN-004 |
| Draft, history, detail, unsupported controls | Required | Required | 009.4/.5 |
| Owning executor is Docker, SSH, or Kubernetes | Container project | Shared viewer | PLUGIN-001.3 |

Use event barriers and observable assertions. Keep existing shard and worker
limits. Run suites sequentially. Every test tears down only its own resources.

## Work orders

| Order | Work order | Dependency | Status |
| --- | --- | --- | --- |
| 01 | [Prove and package the v2 plugin](task-01-prove-plugin-artifact.md) | None | completed |
| 02 | [Build the private plugin host](task-02-private-plugin-host.md) | 01 | completed |
| 03 | [Deliver owned shell outcomes](task-03-shell-outcome-delivery.md) | 02 | completed |
| 04 | [Integrate automatic runtime launch](task-04-automatic-launch.md) | 03 | completed |
| 05 | [Prove browser and executor behavior](task-05-browser-executor-proof.md) | 04 | in progress |
| 06 | [Document activation and evidence](task-06-operator-documentation.md) | 05 | completed |

Update each work order and this table after its own checks pass. No work order
authorizes native subagents. Keep the package uncommitted for user review now.

## Risks and controls

- V2 plugin APIs can change. Limit activation to the conformance manifest and
  retain ordinary ACP on unsupported versions.
- Native subscription readiness and merge behavior need packaged proof. Task 01
  blocks activation until that proof exists.
- Claims can race results. Use bounded native facts and exact immutable claims.
- A callback retry can follow a lost acknowledgement. Journal-backed identity
  must prove duplicate acceptance before a claim is retired.
- A newer user prompt can own the current submission. Preserve the launch's
  original attribution and emit only a workload observation.
- Retained runtimes can outlive backend connections. Preserve existing adoption
  authority and do not hot-install into an old agentctl.
- A runtime gap can lose uncommitted native events. Leave unknown history rather
  than promise replay that OpenCode does not provide.

## Companion package reconciliation

The full-continuity package still owns automatic turns and live activity. Its
Tasks 02, 03, and 04 remain blocked. This package adds outcome-only coverage and
does not change historical completed statuses or recorded test counts.

Public docs change during Task 06, after behavior exists. The current owning
pages are `docs/public/agents-and-profiles.md`,
`docs/public/tasks-and-workflows.md`, and `docs/public/configuration.md`.
The planning turn changes internal documents only.

## Validation

On 2026-10-28, the design package passed catalog validation, all 36
specification-linter tests, full specification lint, local-link checks, and
whitespace checks. All 26 plugin acceptance criteria have work-order coverage.

On 2026-10-29, implementation checks passed:

- Plugin typecheck, unit tests, artifact build, and artifact digest check.
- Packaged `@opencode/cli@2.2.0` conformance with the real artifact.
- Go artifact, host, journal identity, process delivery, ACP claim, lifecycle
  eligibility, profile, startup-contract, and frontend feature-contract tests.
- A real agentctl end-to-end test using the packaged CLI, embedded artifact,
  private host, callback, journal, and workload projection.
- Review remediation for awaited outbox backoff, admission/revocation fencing,
  claim retirement after durable acceptance, behavior-owned configuration and
  payload authority, and full artifact/symlink verification.

Bridge-enabled browser proof and Docker, SSH, and Kubernetes executor proof
remain pending under work order 05.
