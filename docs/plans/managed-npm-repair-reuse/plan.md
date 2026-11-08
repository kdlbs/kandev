---
created: 2026-11-08
status: done
requirements:
  - REQ-AGENTS-MANAGED-RUNTIME-RECOVERY-002
  - REQ-AGENTS-MANAGED-RUNTIME-RECOVERY-004
  - REQ-AGENTS-MANAGED-RUNTIME-RECOVERY-006
  - REQ-AGENTS-MANAGED-RUNTIME-RECOVERY-007
system_design:
  - ../../specs/agents/system-design/managed-npm-repair-reuse.md
  - ../../specs/agents/system-design/managed-npm-cache-recovery.md
  - ../../specs/agents/system-design/managed-npm-runtime-recovery.md
legacy_specs: []
---

# Implementation plan: Reuse verified managed npm repairs

## Overview

Recover a verified incomplete installation once, then reuse the validated
repair during later compatible cold launches. Preserve the selected version,
native conversation, running agents, and single replacement limit.

Implement the owned store first. Then connect task and utility owners.
Prove real npm and executor behavior before browser flows and public documentation.
Work orders are sequential. Implementation and permanent
test changes require a later explicit user request.

Agents owns this package because it owns managed runtime preparation and selection.
The [requirement](../../specs/agents/requirements/managed-npm-repair-reuse.md),
[design](../../specs/agents/system-design/managed-npm-repair-reuse.md), and
[ADR](../../decisions/2026-11-08-reusable-managed-npm-repairs.md) define the proposed extension.

## Evidence and settled scope

The affected Codex ACP 2.5.0 shared tree contained the installed package.
Its root manifest and executable link were absent.
Persisted startup evidence identified `root_manifest_missing`; npm exited 254.
Private recovery succeeded, but subsequent resumes repeated that recovery.
The original cause of the incomplete tree remains unknown.

The authorized one-time maintenance repair rebuilt only that exact shared entry.
npm reported it valid. One normal `--prefer-offline` launch completed ACP
initialization in about one second, without a prompt or conversation creation.
All 15 existing launches used private installations and remained independent.
This is incident evidence, not implementation evidence for reusable repair.

Confirmed scope: automatic recovery for verified cache errors, one replacement,
exact-version preservation, protection for other agents, and reuse across launches.
Normal npm preferences remain unchanged. Plain-npx migration is excluded.

The design uses an owned store instead of automatically replacing shared files.
The current contracts explicitly preserve shared trees, and process-list checks
cannot exclude every external consumer or new launch.

## Scope

### In scope

- Reusable repair generations after requirement-006 failures.
- Task start/resume and agent-wide/profile/model-option discovery.
- Managed ACP and managed native Codex app-server.
- Local PC, local Docker, and SSH, while executor storage persists.
- Context isolation, restart reuse, cancellation, publication, and old-peer fallback.
- Existing desktop/phone recovery outcomes and accurate public guidance.

### Out of scope

- Production implementation during this planning turn.
- Shared npm-tree mutation, manual cache edits, or a global launch mutex.
- New runtime selection, global binaries, normal freshness changes, or requirement 005.
- New UI copy, controls, settings, feature flags, SQL schema, or automatic task starts.
- Automatic generation cleanup, cache-wide scanning, or additional executor types.

## Technical approach

### Owned store

Add `RepairStore` and generation handles under `apps/backend/internal/agent/managedruntime/`.
Use the local `InstanceConfig.HomeDir`, bounded records, contained traversal,
atomic selection publication, and a cancellable OS lock per installation context.
Keep the existing temporary owner for legacy and unavailable-store paths.
Published generation handles must never enter recursive instance cleanup.

Build context identity from trusted installer inputs and executor-local npm context.
Preserve environment provenance from `runtime/environment` resolution.
Generated task/transport values must not fragment reuse or substitute user input.
Use a local private HMAC key so metadata and logs cannot reveal credentials.

### Task and utility integration

Extend authenticated instance/configure and utility request contracts with optional
installer provenance. Negotiate `managed_runtime_repair_reuse_v1` before relying on it.
Pass the local store to process and utility owners through normal construction.
Do not add backend-side shell repair or backend-derived remote paths.

Use the published generation before the first process start.
After a verified miss/failure, reuse the current replacement owner and budget.
Publish only after the exact replacement completes protocol initialization.
Inspect and observe the actual physical cache when a generation is selected.
Keep provider/session result publication separate from installation publication.

### Compatibility matrix

| Shape | Behavior | Proof / fallback |
| --- | --- | --- |
| Registered exact ACP command | Reuse a matching validated repair | Real local npm fixture and two cold launches |
| Managed native Codex app-server | Same store, native initialization boundary | Process and utility protocol tests |
| Selected OpenCode v1/v2 package | Preserve selected family and exact package | Existing catalogue fixture with both families |
| Agent-wide/profile/model options | Share compatible files, preserve result ownership | Profile context and publication regressions |
| Local PC | Persistent local agentctl store | New owner and backend/agentctl restart |
| Docker / SSH | Execution-host store for its storage lifetime | Containers-project scenarios |
| Old peer / absent provenance | Existing private recovery | Negotiation and legacy lifetime tests |
| Auth, policy, disk, arbitrary ENOENT, wrapper, passthrough | Existing eligibility and error behavior | Existing negative tests plus reuse controls |

### Companion packages

The completed [incomplete-cache package](../managed-npm-cache-recovery/plan.md)
provides evidence, preparation, and retry integration. Keep its historical
results intact. Update fixture assertions through this package when lifetime
expectations change; retain explicit legacy-peer coverage.

The pending [Codex dependency package](../managed-codex-dependency-recovery/plan.md)
still owns requirement 005. It must reuse the common preparation owner and
must not infer that requirement 007 publishes missing-dependency repairs.
This package adds no dependency validator or optional-package policy.

## Presentation and mobile contract

No rendered markup, copy, control, navigation, or composition change is planned.
No ASCII UI preview is required for this runtime and test-only UI scope.

Use existing startup/resume rows, retry progress, recovery cards, and profile loading.
First recovery can show attempt 2 of 2. A later first-attempt success must use
ordinary startup/resume status. The transcript retains scroll ownership.
Phone controls and failure details retain their current accessibility and containment.
Closest exemplars are the managed-npm cache-recovery session and settings specs.

If implementation requires a rendered UI change, update this package and the
mobile contract before that change. Add the relevant preview and locale scope then.

## Tests

Names below are planned additions. Product verification remains pending.

| Acceptance | Planned regression |
| --- | --- |
| 007.1/.4/.6 | `TestRepairContext` and `TestManagedRuntimeInstallContext`: user/npm/environment separation and stable generated inputs |
| 007.2/.5/.7 | `TestRepairStorePublication` and `TestManagedRuntimeRepairReuseNPM`: publish after initialize, stop owners, launch once without installation |
| 007.8-.10 | `TestRepairStoreConcurrency` and `TestManagedRuntimeRepairReuseLifetime`: process locks, cancelled builder, independent waiters, old-generation readers |
| 007.11/.12 | `TestRepairStoreInvalidation` and `TestManagedRuntimeRepairReusePublicationFailure`: invalid metadata and current-process continuity |
| 007.3/.4/.13 | `TestManagedRuntimeRepairReuse` and `TestManagedRuntimeRepairReuseProbe`: task/probe integration, matching results, no third start |
| 007.14 | `TestManagedRuntimeRepairReuseCompatibility`: old peers, absent provenance, retained running processes |
| 007.15/.16 | Session and settings desktop/phone scenarios: first recovery, later cold resume, restart, failure controls |

Use the existing loopback registry fixture in `prepared_runtime_test.go`.
Real-npm tests must reproduce missing-root failure before proving reuse.
Counting mocked starts or seeding an error card alone is insufficient.
No test requires public npm, provider credentials, or model inference.

## E2E tests

Add scenarios named `reusable verified cache repair` to existing helpers/specs.
Retain current startup and failure scenarios as controls.

- Session desktop/phone: recover from the incomplete shared tree, stop the process,
  then cold-resume. Assert one start, unchanged native ID/transcript, and no new installer.
- Restart the isolated backend and agentctl with storage intact. Prove reuse
  rather than adoption of a still-running process. Deliver one follow-up prompt.
- Settings desktop/phone: recover and repeat agent-wide discovery. Separately
  recover and repeat profile/model-option discovery within its authorized owner scope.
  Preserve saved/draft values and result ownership. Cross-owner reuse stays forbidden.
- Simulate damaged selected metadata, cancelled preparation, and publication failure.
  Assert bounded attempts, accurate final details, and usable existing recovery controls.
- Docker/SSH: repeat compatible cold starts on the same execution-host storage.
  Assert host-specific roots, unchanged shared sentinels, and no second installer.

## Work orders

- [x] [Task 01: Add the owned repair store](task-01-owned-repair-store.md)
- [x] [Task 02: Reuse repairs in tasks and discovery](task-02-launch-and-probe-reuse.md)
- [x] [Task 03: Prove real npm and executor reuse](task-03-npm-and-executor-proof.md)
- [x] [Task 04: Prove user flows and document behavior](task-04-user-flows-and-docs.md)

## Verification results

Design checks passed on 2026-11-08:

- `python3 scripts/list-docs.py validate`: 438 decisions and 1940 specifications.
- `python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `validateCoverage` from `.github/scripts/pr-docs.cjs`: all four work orders covered, with zero reference errors.
- The coverage check included a projected `prepared_runtime.go` change to exercise the production documentation gate. No production file changed.
- `git diff --check` passed for tracked documents. Whitespace checks also passed for all eight new Markdown files.
- The new requirement/design and ADR appear in the generated catalog. No system README index was changed.

Implementation results on 2026-11-08:

- Task 01 and Task 02 are implemented and verified with focused Go tests,
  including the race detector for the owned store and process integration.
  Review-driven hardening binds repairs to authenticated application owners,
  hashes the effective environment and npm config, fences recovery against
  configuration changes, keeps one admission-time deadline, makes publication
  failure terminal, and rejects unsafe store identity files.
- Task 03 proves reusable repair with real local npm (loopback registry, missing
  shared root manifest, owner teardown, fresh store, no second installation or
  registry request). Docker and SSH executor scenarios kill only the owned
  agent process, retain execution-host storage, and resume without a new
  installer or registry request. Docker also proves a new container is a miss.
- Task 04 updates public guidance. The shared desktop and phone session reuse
  E2E scenario and the desktop/phone profile discovery reuse assertion passed,
  as did the existing recovery controls, the desktop/mobile probe controls, and
  the Docker/SSH private-preparation specs.
- E2E evidence on 2026-11-08:
  - Desktop and phone `reusable verified cache repair` session scenarios passed
    (2 each when combined with settings), including a backend restart with
    execution-host storage intact, a new execution ID, no new installer, no new
    registry request, no recovery card, and one follow-up prompt.
  - Desktop and phone profile discovery reuse passed after a refresh, with no
    new registry request.
  - Seven desktop startup-retry and profile-discovery controls passed.
  - Docker and SSH `reusable verified cache repair` executor cases passed
    (2 tests) after killing only the owned agent process, retaining storage,
    and resuming the same session without a new installer or registry request.
  - The Docker case also proves a second task in a new container is a storage
    miss.
- Same-container Docker and SSH reuse is proven by killing only the owned agent
  process, retaining the execution-host store, and asserting no new installer or
  registry request after resume. Docker also proves a new container is a miss.
- Review hardening is verified by owner-boundary, inherited-environment,
  custom/global npmrc, configuration-revision, admission-deadline, publication
  failure, unsafe identity file, utility authorization, capability negotiation,
  and initialize-before-discovery tests.
- On Windows, reuse is intentionally disabled until native ACL verification
  exists; private recovery remains available. Windows-native lock/replace
  verification remains in the Windows lane, and no cross-platform completion
  claim is made without it.

No implementation worker or delegation was authorized or used.

## Risks

- A key based on task IDs prevents reuse; a key that omits authorized inputs can cross installation contexts.
- Published roots must outlive every consumer, including detached agents and utility descendants.
- A publication callback can race with stop, configure, or a newer process generation.
- A lock must not become a manager-wide serialization point or reset a caller's deadline.
- A selected owned cache needs physical-root evidence and observations. Default-cache rediscovery can inspect the wrong installation.
- Persistent generations consume disk until explicit maintenance. This package intentionally adds no automatic deletion authority.
- Container recreation can remove executor storage; reuse cannot cross that boundary without a persistent store.
- The historical cause of the original incomplete npm tree remains unknown.
