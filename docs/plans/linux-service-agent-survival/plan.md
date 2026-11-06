---
created: 2026-10-19
status: done
requirements:
  - REQ-EXECUTORS-SURVIVAL-001
system_design:
  - ../../specs/executors/system-design/agent-survival-across-restart-01.md
legacy_specs: []
---

# Implementation Plan: Linux service agent survival

## Overview

Correct the generated Linux service lifetime policy for [issue 4552](https://github.com/kdlbs/kandev/issues/4552).
One sequential work order corrects the renderer, binds the Linux service backend
to its launcher, and updates the service how-to. The executors system owns this repair because the failed
contract is agent process lifetime, although the renderer belongs to the launcher.
The owning requirement and design remain draft; this repair does not certify the
broader draft recovery contract.

## Scope

In scope: AC-EXECUTORS-SURVIVAL-001.11-.13, both Linux service scopes, the native
launcher/backend lifetime boundary, application cleanup controls and service docs.

Out of scope: new scopes/cgroups, executor/adoption protocols, persistence,
provider resume, flag identities/defaults/rollout, new CLI options, launchd or
Windows behavior changes, and changes to running deployments. Adjacent manager
wrapper work and design part 3 remain outside this package.

## Root cause and evidence

`renderSystemdUnit` currently emits `KillMode=mixed`. Linux `Setsid` creates a
session but leaves agentctl in the backend service cgroup. After the main process
exits, systemd can SIGKILL the retained process despite Kandev choosing detach.
The canonical issue reports this for the generated Ubuntu user service.

Supplied controlled Linux systemd 255 evidence used real rendered kill-mode input
and test-binary actors with separate SID in the verified service cgroup:
retained/mixed died; retained/process lived; ordinary/process with kernel
parent-death SIGTERM exited within a bounded three-second observation. Retained
observations were 200ms. These controls establish the OS/service boundary, not an
actual agentctl adoption, provider turn or backend end-to-end guarantee.
The new regression implementation must be independently authored from production
behavior.

## Launcher/backend ownership

The native unit starts a launcher, not `__backend` directly. `runManagedApp`
launches the backend through `restartableBackend.start` and `startProcess`;
`configureManagedProcess` currently sets only `Setpgid`. The parent watchdog runs
inside the launcher. It cannot stop the backend after that launcher is SIGKILLed.
The original two-actor systemd test did not model this extra native child.
The amended fixture exercises launcher, backend and agent ownership.

The correction introduces private `startManagedCommand` implementations
for Linux and other platforms. Linux selects only the existing `backend` label
with `KANDEV_SERVICE_MANAGER=systemd`; other contexts retain ordinary Start/Wait.
A dedicated goroutine locks its OS thread across backend Start and the sole Wait,
arming SIGTERM parent death without coupling backend lifetime to a short-lived
calling goroutine. Start errors release the thread; successful Wait results feed
existing process exit accounting once. Do not change service ExecStart, backend
startup/stop budgets, persistence or agentctl survival attributes.

## Technical approach

Change only the lifetime property in `apps/backend/internal/launcher/service.go`
to `KillMode=process`. Preserve every other emitted setting. The fixed service
policy allows startup-resolved survival values to control application retention.
Keep Linux disabled `Pdeathsig`, pipe, launcher cleanup and manager stop semantics.
No install-time flag branch is introduced. The backend guard is independent of
the survival flag and uses only existing service-manager metadata. See the owning design's
[service boundary and alternatives](../../specs/executors/system-design/agent-survival-across-restart-01.md#native-linux-service-lifetime).

Both user and system output receive portable renderer regressions. The guarded
real-manager test exercises only unique transient user units and controlled actors;
actual system-scope service execution is not required or claimed. macOS is a
portable unchanged-output control. Remote executors retain their own lifetimes.

## Tests

- `service_test.go`: new `TestRenderSystemdUnitPreservesApplicationAgentLifetime`
  covers both Linux scopes, exact process policy and unchanged unit properties;
  existing launchd/environment/identity tests remain compatibility controls.
- New `service_survival_linux_test.go`:
  `TestSystemdServiceAgentLifetime` exercises the generated policy with retained,
  ordinary parent-bound and causal explicit-mixed controls (AC 001.11/.12).
- New `managed_start_linux_test.go`: `TestManagedBackendLifetimeFollowsLauncher`
  calls the real `startProcess` from a short-lived pinned caller, proves the
  healthy backend remains live for two seconds without a SIGTERM-receipt marker
  after that caller returns, then SIGKILLs the
  fixture launcher and requires backend exit. Cover service and non-service
  contexts and start failure without a test-only production seam (AC 001.13).
- Extend `TestSystemdServiceAgentLifetime` with launcher -> backend -> agent
  actors and explicit abrupt-launcher-death cases with survival on/off. Existing
  stop and mixed controls remain; all fixture identities must be cleaned.
- `sysprocattr_linux_test.go`: new `TestBuildSysProcAttrParentDeath` uses actual
  production attributes in controlled parent/child processes for enabled/disabled
  behavior. Existing liveness-pipe, real launcher Stop and agentctl pipe-monitor
  tests exercise independent guards (AC 001.12).
- `manager_stop_detach_test.go`: extend existing terminating controls to explicit
  `StopAgent` and `StopReasonLaunchRollback`, keeping enabled detach and disabled
  shutdown cases. Assert terminating runtime calls/events, not only branch text.
- Existing backendapp adoption/disabled-reclaim tests guard reversal behavior.

## End-to-end evidence

The real user-systemd integration is the end-to-end service-boundary evidence;
there is no browser/UI change and no artificial Playwright test. A skipped
integration run is not a pass. Full actual-agentctl/provider continuation remains
outside this repair's proof scope.

## Work orders

- [x] [Task 01: Preserve application-owned agent lifetime](task-01-service-lifetime.md)

## Verification results

The real-start regression first reproduced the backend surviving abrupt launcher
death. After the correction, the full launcher package passed (25.996s), including
the two-second healthy caller-return control, metadata/label controls and start
failure. Forced real user-systemd integration executed all five cases without
skips (9.839s): normal retained/ordinary, mixed control and abrupt launcher death
with retained/ordinary agents. All 15 actors were absent and all five units
inactive/unloaded after cleanup. Earlier manager, pipe, reversal and agentctl
controls remain valid. This is service-boundary evidence, not provider continuation.

Specification checks passed (388 decisions, 1656 specifications, 36 tool tests).
Public-doc tests passed 63/63 and validated 48 pages. Actual delivery-reference
preflight was covered with no errors, and diff whitespace checks passed. The
service how-to now explains agent lifetime and normal unit regeneration.

## Risks

`process` removes systemd's final descendant sweep. The healthy backend must
not be treated as an intentional survivor; the backend guard closes that boundary. Application guards remain
responsible for normal disabled shutdown and stop; a hung or defective child may
outlive the unit. The flag stays off by default. Existing units retain their old
policy until normal install/update regeneration. A user-manager integration cannot
prove privileged system service execution or complete backend/provider behavior.
