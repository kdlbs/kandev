---
created: 2026-11-07
status: done
requirements:
  - REQ-AGENTS-MANAGED-RUNTIME-RECOVERY-002
  - REQ-AGENTS-MANAGED-RUNTIME-RECOVERY-004
  - REQ-AGENTS-MANAGED-RUNTIME-RECOVERY-006
system_design:
  - ../../specs/agents/system-design/managed-npm-cache-recovery.md
  - ../../specs/agents/system-design/managed-npm-runtime-recovery.md
legacy_specs: []
---

# Fix plan: Automatic recovery of incomplete npm caches

## Overview

Follow-on design: [Reusable managed npm repairs](../managed-npm-repair-reuse/plan.md)
adds cross-launch reuse. This completed package retains its original scope and results.

An upgrade must recover an incomplete selected runtime without manual npm
maintenance. Prepare the same exact package privately, then retry normal
discovery or session initialization once. Preserve the shared cache and saved
conversation. Agents owns this package because it owns runtime preparation
and recovery policy.

The [incomplete-cache requirement](../../specs/agents/requirements/managed-npm-cache-recovery.md)
owns requirement 006. The [shared runtime recovery contract](../../specs/agents/requirements/managed-npm-runtime-recovery.md)
continues to own the common retry and cache-safety criteria.

Implement the shared evidence/preparation boundary first. Then connect task
and discovery recovery. Finish with upgrade/restart, executor, and browser
proof. Work orders are sequential. The user authorized implementation and PR
delivery on 2026-11-07. One user-selected implementation worker executes them.
The primary session owns integration, artifact status, and delivery.

## Evidence and assumptions

Confirmed user intent: existing affected installations must recover when the
fixed Kandev build restarts. Future upgrades must not require a runtime update
click, terminal command, or fresh conversation for this failure.

On 2026-11-07, a source build selected new runtime defaults during startup.
Claude ACP `0.100.0`, Codex ACP `2.5.0`, and Codex app-server `0.166.0` each
had an installed package directory but lacked the execution root manifest
and `.bin` links. npm exited 254 with `ENOENT` before initialization.
Task resume then fell back to read-only workspace inspection.
The saved native conversation identity remained present, with no recovery block.

An isolated reproduction of each incomplete layout failed with the same error.
A fresh cache for each exact version completed protocol initialization.
No model prompts were sent. All investigation subprocesses and scratch caches
were removed. The cause of the original missing cache files remains unknown.

Current `managedRuntimeProbeFailureCode` classifies exact ETARGET and policy
errors. Task startup rejects unclassified ENOENT evidence. Both therefore miss
this condition. Explicit runtime maintenance repairs it, but requires user action.

The existing shared-tree protection is correct. Requirement 006 extends the
automatic private-preparation policy; it does not broaden generic ENOENT retry.
Existing automatic-start and session ownership policies remain authoritative.
No material product choice remains unresolved.

## Scope

### In scope

- Verified missing execution metadata for registered exact managed packages.
- Private installation, package/bin validation, owned lifetime, and one retry.
- Managed ACP and native Codex app-server startup and utility probes.
- Startup discovery, saved-session resume, profile discovery, and model options.
- Local PC, local Docker, and SSH task executors.
- Existing broken caches after restart, sibling safety, cancellation, truthful
  failure details, and desktop/phone proof without repair clicks.

### Out of scope

- Antigravity SIGILL, generic provider crashes, arbitrary modules, or all ENOENT errors.
- Shared-tree mutation, global cache cleanup, version rollback, or global binaries.
- Custom launch wrappers, passthrough, post-initialization replay, and unsupported executors.
- Missing bundled Codex platform dependencies under requirement 005.
- Persistent repaired-runtime promotion, cache sweeps, new settings, or schema migrations.

## Technical approach

### Shared executor-local boundary

Add cache inventory and `PreparedRuntime` ownership under
`apps/backend/internal/agent/managedruntime/`. Use existing exact key derivation,
bounded runtime manifest inspection, safe npm project resolution, and owned
process runners. Require local missing-file confirmation and matching raw npm
evidence before producing a typed safe condition. No diagnostic path supplies
repair authority.

Add optional typed evidence/directives to agentctl initialize, start, and probe
contracts. Negotiate `managed_runtime_private_preparation_v1` through existing
capability handling. Revalidate the package, failed generation or attempt, and
environment at the execution boundary. Reuse one prepared-runtime owner across
process, ACP utility, and native Codex utility paths.

### Recovery owners

Extend `managed_runtime_startup.go` through the existing one-replacement owner.
Retain STARTING while cleanup, private preparation, and replacement run.
Preserve prompt admission and create/load/resume ordering.

Factor a bounded probe-retry helper from `hostutility/manager.go` and use it
from agent-wide, profile baseline, and profile model-option flights.
Keep admission ordering, context-generation cancellation, single-flight scope,
and final publication ownership. No profile or runtime selection is saved.

### Compatibility matrix

| Provider/shape | Intended behavior | Evidence / fallback |
| --- | --- | --- |
| Claude ACP and Codex ACP, exact managed package | Verify incomplete tree, privately prepare, retry once | Local npm and protocol regressions |
| Native Codex app-server through managed npm | Same repair before native initialize | Utility and task native-protocol tests |
| Other registered managed package | Same inventory contract, no provider-name heuristic | Synthetic second-provider fixture |
| Profile baseline/options | Preserve full context and publish only matching results | Generation and draft-preservation tests |
| Local Docker / SSH | Prepare on the executor with its npm context | Containers-project regressions |
| Native binary, wrapper, passthrough, old peer | Existing error, no repair directive | Negative compatibility tests |
| Auth/policy/disk/permissions or post-init failure | Existing precedence, no replacement | Phase and cleanup tests |

### Companion packages

The pending [Codex dependency package](../managed-codex-dependency-recovery/plan.md)
already proposes a private owner and task/probe directives. This package now
owns their generic implementation. Its Task 01 runs before that package's
Task 01. The dependency package then adds only its specialized inventory,
optional-package policy, and validator to the common owner.

Its task/probe work orders extend the common retry helper after this package
lands; they must not create another installation or retry mechanism.
Requirement 005 stays pending and has no acceptance claim in this package.
Historical completed ETARGET packages retain their recorded results.

## Presentation and mobile contract

No rendered layout, control, or copy change is planned. Use existing boot retry
progress, task recovery cards, and profile discovery loading/results.
The closest exemplars are `managed-runtime-startup-retry.spec.ts`, its mobile
counterpart, and profile capability discovery specs. No new ASCII preview is
required for this backend recovery change.

Desktop and phone must recover through normal startup discovery and session
open, with no repair action. The task transcript keeps scroll ownership.
On exhaustion, existing retry controls and collapsed safe details remain
accessible and contained. A UI change discovered during implementation requires
an updated preview and locale scope before editing that surface.

## Tests

All names below describe planned additions. None is completed product evidence.

| Acceptance criteria | Required regression |
| --- | --- |
| 006.1, 006.9, 002.1/.4 | `TestManagedRuntimeCacheEvidence`: matching root ENOENT, exact manifest identity, missing root, wrong path/spec, symlinks, read errors, and permanent-error precedence |
| 006.4-.6, 002.3 | `TestPreparedRuntimeNPM`: real local npm installation, exact package/bin validation, private cache, unchanged shared sentinels |
| 006.5-.7 | `TestPreparedRuntimeLifetime`: live healthy/recovered/exhausted siblings, cancellation, descendant cleanup, detach/adoption |
| 006.2-.4/.7-.8, 004.4/.7 | `TestManagedRuntimeCacheResume`: failed cached generation followed by same native conversation and one accepted prompt |
| 006.2/.7-.10 | `TestManagedRuntimeCacheProbe`: ACP/native bootstrap, profile baseline/options, shared budget, obsolete context, isolated publication, old peers |
| 006.3/.8 | Backend restart and visible session-open/settings flows without repair/update/retry clicks |

Task 01 includes a real npm regression against a local package registry fixture.
It must reproduce npm's root-manifest failure and then initialize the repaired
protocol process. Stubbing ETARGET or only counting process starts is insufficient.
No CI test requires public npm access, provider credentials, or model inference.

## E2E tests

Add focused desktop and phone cache-recovery specs in `session/` and `settings/`.
Extend the existing managed-runtime fixtures with missing-manifest inventory and
a private npm preparation fixture. Use the selected package catalogue rather
than freezing shipped default pins in tests.

- Seed an existing resumable session and incomplete shared cache. Restart the
  backend, open the task, and verify automatic recovery without a repair click.
  Assert the same Kandev/native IDs, prior transcript, usable composer, no
  read-only notice, and one new prompt delivery (006.2-.4/.8).
- Restart with incomplete host caches. Verify model discovery before task
  creation. Refresh concrete profile baseline/options and preserve saved and
  draft values. Native Codex app-server receives its own protocol case (006.2/.8).
- Fail preparation and replacement separately. Assert one accurate final
  failure, safe details, working existing retry, and no third start (006.7-.9).
- Cancel or supersede during preparation. Assert no stale replacement/result
  and continued healthy-sibling operation (006.5/.7).
- Repeat executor-local preparation on Docker and SSH. Assert executor cache
  isolation and backend/shared-tree sentinels (002.1/.3, 006.2/.5).

## Work orders

- [x] [Task 01: Verify and privately prepare incomplete caches](task-01-private-cache-preparation.md)
- [x] [Task 02: Recover tasks and discovery automatically](task-02-automatic-runtime-recovery.md)
- [x] [Task 03: Prove upgrade recovery and document behavior](task-03-restart-and-executor-proof.md)

## Verification results

Implemented and verified on 2026-11-07. All three work orders are complete.

- Exact task-defined backend selections passed, including ACP/native Codex
  startup, profile/model options, generation matching, owner lifetime,
  cancellation, and old-peer behavior. Real npm preparation and race checks pass.
- Fresh production artifacts passed 2 desktop and 2 phone recovery/discovery
  cases. An existing FAILED row resumes after restart with the same native token,
  transcript, usable composer, and one accepted follow-up without repair actions.
- Docker/SSH passed 4 cases, proving private preparation on both execution hosts
  alongside the existing temporary-error behavior and shared-cache preservation.
- Existing startup/profile browser regressions: 7 passed. Negative conditions
  compose backend coverage with existing failure-control and draft/model UI cases.
- Native Codex's real pre-initialize probe process produces bound evidence.
  OpenCode v1 and v2 use registered selected-package recognition.
- Go/web lint, formatting, docs validation, specification lint, documentation
  coverage, and whitespace checks pass. Task 03 records command reconciliation,
  environment setup, and an unrelated restricted-filesystem test limitation.
- Public recovery guidance is updated in `docs/public/agents-and-profiles.md`.
  Requirement 006 is active and its design is current. Requirement 005 and its
  separate dependency-recovery package remain pending.

The primary conversation owns commit, PR publication, the requested 15-minute
wait, and CI/review remediation. No production UI change requires screenshots.

## Risks

- The original deletion cause is unknown. Recover from independently verified
  state without claiming to prevent an unproven npm race.
- A cold private install can exceed a short probe deadline. Honor the earlier
  deadline and expose the actual final cause.
- Retained agentctl peers need capability negotiation. Do not stop a healthy
  retained peer merely to obtain a repair capability.
- Private files must outlive provider and installer descendants, including
  lazy reads after backend detach. Request-scoped cleanup can break recovery.
- Logical profile identity must retain authorized environment values while
  physical execution uses the private npm cache internally.
- Windows bin shims and contained Unix symlinks need platform-specific proof.
- The pending dependency package must extend the common owner rather than
  introduce duplicate preparation or a second replacement budget.
