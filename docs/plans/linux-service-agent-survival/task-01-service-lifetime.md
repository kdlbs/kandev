---
id: "01-service-lifetime"
title: "Preserve application-owned agent lifetime"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-SURVIVAL-001
acceptance_criteria:
  - AC-EXECUTORS-SURVIVAL-001.11
  - AC-EXECUTORS-SURVIVAL-001.12
  - AC-EXECUTORS-SURVIVAL-001.13
system_design:
  - ../../specs/executors/system-design/agent-survival-across-restart-01.md
---

# Task 01: Preserve application-owned agent lifetime

## Summary

Make generated Linux services honor application-owned agent lifetime while
preserving ordinary cleanup and backend ownership by the native launcher.
Deliver the renderer, bounded Linux backend guard, regressions and how-to together.

## In scope

Complete the Linux service backend guard in `startProcess` through private
`startManagedCommand` implementations. The Linux implementation selects only
label `backend` plus existing `KANDEV_SERVICE_MANAGER=systemd`, preserves process
attributes and arms `Pdeathsig=SIGTERM`. A dedicated goroutine pins the creating
OS thread across Start and the sole Wait. Return the start error or a single Wait
function/result to existing exit accounting. Release the thread on Start failure
or Wait completion. Other labels/platforms retain the ordinary Start/Wait path.
No caller-thread pin may outlive a start call, and no backend may depend on a
short-lived calling goroutine keeping its own OS thread alive.


Write portable renderer regressions first; run them red against `mixed`. Add a
Linux-only real-manager regression, then change the renderer to `process` and run
green. Preserve Type/ExecStart/User/PATH/environment/Restart/TimeoutStop and service
identity. Extend actual Linux process-attribute and manager stop controls; reuse
existing launcher Stop, pipe-monitor and rollback-gate evidence.

Update `docs/public/run-as-a-service.md` (how-to): explain that service stop/restart
stops the backend while eligible agents can remain under experimental opt-in
survival, that disabled shutdown and explicit task Stop still terminate agents,
and that old generated units need normal reinstall/update regeneration. Do not
promise host-reboot survival or complete provider continuation. Correct the service-main terminology: the native launcher is the main PID;
its backend child remains launcher-owned. Public docs ship with implementation,
not with speculative design alone.

## Out of scope

No additional flag/config/CLI surface, native scope ownership, adoption/provider
rewrite, dependency or cache changes, UI, privileged integration, or live Kandev
service mutation. Do not modify design part 3 or adjacent wrapper work.

## Acceptance

1. Both generated Linux scopes use process-only termination, and the native
   backend terminates after abrupt launcher death in either survival mode.
   Returning from the starting caller must leave a healthy backend alive. Keep unrelated unit
   output and launchd controls unchanged; regression fails on the original renderer.
2. Guarded real-manager retained/ordinary controls and actual disabled kernel,
   pipe, launcher stop and manager stop/detach controls pass at their stated layers.
3. Public how-to and owning internal pair agree on policy, opt-in limits and normal
   regeneration; manifest/results record actual commands and integration skip state.

## Launcher/backend regression

New `managed_start_linux_test.go` must exercise the existing production
`startProcess` path, not merely inspect a proposed attribute or call a new helper.
Name the regression `TestManagedBackendLifetimeFollowsLauncher`. A private
launcher actor starts a healthy backend actor from a short-lived goroutine that
locks its own OS thread and then returns without unlocking. That thread retires;
the backend must remain alive for a two-second observation, exceeding the
race helper's default one-second exit delay. Arm the backend actor's SIGTERM
handler before readiness and publish a signal-receipt marker immediately; the
healthy control requires both continued liveness and no marker. Do not infer
absence of SIGTERM from a non-zombie process at 200ms. SIGKILL the exact
verified launcher PID and require backend exit within three seconds. Current
production should fail because the healthy backend remains alive after its
launcher dies, not because compilation, readiness or cleanup failed. The pinned
short-lived caller and signal marker catch a naive thread-scoped Pdeathsig fix.

Cover the service-manager metadata present/absent boundary and a nonexistent
command returning its start error promptly. Preserve the existing Wait/exit code
behavior. Every actor self-expires within 20 seconds, uses guarded helper argv/env,
and is captured with birth identity; register cleanup before assertions and join
all original handles. This needs no actual backend, providers or credentials.


In `service_survival_linux_test.go`, require explicit test-only
`KANDEV_TEST_SYSTEMD_SURVIVAL=1`; otherwise skip. With opt-in, preflight Linux,
`systemd-run`, `systemctl --user`, writable user bus and available transient-unit
support; an unavailable manager is an explicit skip reason, not a passing case.
An opted-in local validation must actually execute and pass. Hosted CI remains
portable without a user manager. This test switch is not a product setting.

Extract the kill-mode property from the real renderer; do not hardcode process
as the behavior under test or build a substitute renderer. Use independently
written test-binary helper actors selected by both private env and exact helper
argv; no recursive normal test entry or production API solely for test access.
Use uniquely random `kandev-issue4552-<nonce>-<case>.service` transient user units.
Verify exact unit identity, main PID, actor PID/start identity, distinct SID and
same service cgroup before stop. Register cleanup before every failure path.

Use the actual native production boundary: fixture launcher calls `startProcess`
with label `backend` and existing systemd service metadata; the backend actor
starts the agent actor. On normal fixture launcher SIGTERM, use the existing
supervisor termination path. On abrupt launcher death, signal only the exact
verified owned main PID after unit/MainPID identity checks. Verify all three
process births, the backend process group, and the agent's distinct SID in the
same service cgroup.

Exercise retained and ordinary agents under the rendered policy, explicit
`mixed` as a causal control, and abrupt launcher death with survival enabled and
disabled. The native backend must exit in both crash cases; only the enabled
retained agent may remain. Keep the existing 200ms retained/three-second ordinary
settlement windows and extend ownership/cleanup logs to all three actors. Real
production `buildSysProcAttr` is tested separately in its owning package. Pin the spawning parent thread while testing Linux Pdeathsig, whose binding is
thread-sensitive. Use readiness handshakes rather than startup sleeps. Await terminal main/unit state
and observe retained identity for at least 200ms; ordinary actor exit gets up to
three seconds (race test binaries have asynchronous exit delay). Never disable
race or mistake a zombie for a live process. Actor maximum lifetime: 20 seconds;
per-case deadline: 30 seconds; complete integration test: 120 seconds. Capture
unit/actor identity on failures. Use only verified fixture unit/PIDs for cleanup,
join owned children, and confirm actors gone/units inactive, including failure paths.

Never operate on real `kandev.service`, foreign units or broad process matches.
No daemon-reload, global reset-failed, prune or broad kill. If user-manager
constraints prevent safe fixture isolation, report the exact boundary instead
of expanding production scope.

## Correction verification

The earlier renderer/control commands below already have completed evidence.
For this backend-only correction, do not replay unchanged manager, pipe,
reversal or agentctl-launcher suites. Use the same Go 1.26/Node 24 PATH,
GOTOOLCHAIN=local, GOMAXPROCS=2, test GOMEMLIMIT=512MiB and empty PostgreSQL DSN.
Each run-quiet call receives its own directory. First run the new real-start
regression red, then after correction run the complete launcher package and the
forced real-manager fixture:

```bash
export PATH="/home/jcfs/.local/share/mise/installs/go/1.26.0/bin:/home/jcfs/.local/share/mise/installs/node/24.21.0/bin:$PATH"
export GOTOOLCHAIN=local GOMAXPROCS=2 GOMEMLIMIT=512MiB KANDEV_TEST_POSTGRES_DSN=
KANDEV_RUN_QUIET_DIR="$(mktemp -d /tmp/issue4552-backend-red-XXXXXX)" scripts/run-quiet go -- timeout -k 10s 300s sh -c 'cd apps/backend && go test -race -trimpath -tags=fts5 -p=1 -count=1 -timeout=120s ./internal/launcher -run "^TestManagedBackendLifetimeFollowsLauncher$"'
KANDEV_RUN_QUIET_DIR="$(mktemp -d /tmp/issue4552-backend-green-XXXXXX)" scripts/run-quiet go -- timeout -k 10s 300s sh -c 'cd apps/backend && go test -race -trimpath -tags=fts5 -p=1 -count=1 -v -timeout=120s ./internal/launcher'
KANDEV_RUN_QUIET_DIR="$(mktemp -d /tmp/issue4552-backend-systemd-XXXXXX)" KANDEV_TEST_SYSTEMD_SURVIVAL=1 scripts/run-quiet go -- timeout -k 10s 180s sh -c 'cd apps/backend && go test -race -trimpath -tags=fts5 -p=1 -count=1 -v -timeout=120s ./internal/launcher -run "^TestSystemdServiceAgentLifetime$"'
KANDEV_RUN_QUIET_DIR="$(mktemp -d /tmp/issue4552-backend-lint-XXXXXX)" GOMEMLIMIT=3GiB scripts/run-quiet lint -- timeout -k 10s 360s bash scripts/lint-go-changed apps/backend/internal/launcher/service.go apps/backend/internal/launcher/service_test.go apps/backend/internal/launcher/service_survival_linux_test.go apps/backend/internal/launcher/process.go apps/backend/internal/launcher/managed_start_linux.go apps/backend/internal/launcher/managed_start_other.go apps/backend/internal/launcher/managed_start_linux_test.go apps/backend/internal/agent/runtime/agentctl/launcher/sysprocattr_linux_test.go apps/backend/internal/agent/runtime/lifecycle/manager_stop_detach_test.go
```

Use the existing cheap documentation/coverage/diff commands after the final docs
edit. Lint arguments must match the final PR's changed Go paths. No new dependency
install is required; the existing frozen install remains present.

## Earlier renderer/control verification

Run from repository root, sequentially. Use task-local PATH selection for existing
Go 1.26.0 and Node 24.21.0, with no global mise mutation. No pnpm command is needed
for this backend change; before any later hook requiring pnpm, install frozen
workspace dependencies once from `apps/` if this fresh worktree lacks them.

```bash
export PATH="/home/jcfs/.local/share/mise/installs/go/1.26.0/bin:/home/jcfs/.local/share/mise/installs/node/24.21.0/bin:$PATH"
export GOTOOLCHAIN=local GOMAXPROCS=2 GOMEMLIMIT=512MiB KANDEV_TEST_POSTGRES_DSN=
KANDEV_RUN_QUIET_DIR="$(mktemp -d /tmp/issue4552-render-XXXXXX)" scripts/run-quiet go -- timeout -k 10s 300s sh -c 'cd apps/backend && go test -race -trimpath -tags=fts5 -p=1 -count=1 -timeout=120s ./internal/launcher ./internal/agent/runtime/agentctl/launcher'
KANDEV_RUN_QUIET_DIR="$(mktemp -d /tmp/issue4552-manager-XXXXXX)" scripts/run-quiet go -- timeout -k 10s 300s sh -c 'cd apps/backend && go test -race -trimpath -tags=fts5 -p=1 -count=1 -timeout=180s ./internal/agent/runtime/lifecycle -run "^TestStopAgentWithReason|^TestStopAgentExplicit"'
KANDEV_RUN_QUIET_DIR="$(mktemp -d /tmp/issue4552-pipe-XXXXXX)" scripts/run-quiet go -- timeout -k 10s 180s sh -c 'cd apps/backend && go test -race -trimpath -tags=fts5 -p=1 -count=1 -timeout=60s ./cmd/agentctl -run "^TestMonitorParentLiveness"'
KANDEV_RUN_QUIET_DIR="$(mktemp -d /tmp/issue4552-reversal-XXXXXX)" scripts/run-quiet go -- timeout -k 10s 240s sh -c 'cd apps/backend && go test -race -trimpath -tags=fts5 -p=1 -count=1 -timeout=120s ./internal/backendapp -run "^TestResolveSurvivingAgentctl"'
KANDEV_RUN_QUIET_DIR="$(mktemp -d /tmp/issue4552-systemd-XXXXXX)" KANDEV_TEST_SYSTEMD_SURVIVAL=1 scripts/run-quiet go -- timeout -k 10s 180s sh -c 'cd apps/backend && go test -race -trimpath -tags=fts5 -p=1 -count=1 -v -timeout=120s ./internal/launcher -run "^TestSystemdServiceAgentLifetime$"'
KANDEV_RUN_QUIET_DIR="$(mktemp -d /tmp/issue4552-lint-XXXXXX)" GOMEMLIMIT=3GiB scripts/run-quiet lint -- timeout -k 10s 360s bash scripts/lint-go-changed apps/backend/internal/launcher/service.go apps/backend/internal/launcher/service_test.go apps/backend/internal/launcher/service_survival_linux_test.go apps/backend/internal/agent/runtime/agentctl/launcher/sysprocattr_linux_test.go apps/backend/internal/agent/runtime/lifecycle/manager_stop_detach_test.go
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check
git status --short -- docs/plans/linux-service-agent-survival
```

The first regression run may target only
`TestRenderSystemdUnitPreservesApplicationAgentLifetime` for red evidence.
The commands above are final green evidence. Keep the lint argument list equal
to actual changed Go paths; it lints complete owning packages. Use its separate
3GiB budget when capacity permits, not the test budget. CI owns full backend lint.
Each run-quiet directory is unique and task-owned. Record original command handles,
exit codes and joins externally. Do not infer success from duplicate runs.
No full backend, frontend or E2E suite is prescribed.

Run delivery-reference preflight from repository root after implementation:

```bash
/home/jcfs/.local/share/mise/installs/node/24.21.0/bin/node <<'JS'
const fs = require('fs');
const cp = require('child_process');
const validator = require('./.github/scripts/pr-docs.cjs');
const tracked = cp.execFileSync('git', ['diff', '--name-status', '--no-renames', 'HEAD'], {encoding:'utf8'});
const added = cp.execFileSync('git', ['ls-files', '--others', '--exclude-standard'], {encoding:'utf8'});
const changedFiles = tracked.trim().split('\n').filter(Boolean).map(line => {
  const [code, filename] = line.split('\t');
  return {filename, status: {A:'added', M:'modified', D:'removed'}[code]};
});
changedFiles.push(...added.trim().split('\n').filter(Boolean).map(filename => ({filename,status:'added'})));
const paths = changedFiles.map(file => file.filename);
const fileContents = Object.fromEntries(paths.filter(p => p.startsWith('docs/') && fs.existsSync(p)).map(p => [p, fs.readFileSync(p, 'utf8')]));
const result = validator.validateCoverage({changedFiles,fileContents});
console.log(JSON.stringify({ok:result.ok,status:result.status,errors:result.errors}));
if (!result.ok) process.exitCode = 1;
JS
```

Catalog lint alone does not validate delivery references. The design checkpoint
may validate anticipated `service.go` as a declared future input, distinguishing
that result from actual implementation coverage.

## Files likely touched

- `apps/backend/internal/launcher/process.go`
- `apps/backend/internal/launcher/managed_start_linux.go` (new)
- `apps/backend/internal/launcher/managed_start_other.go` (new)
- `apps/backend/internal/launcher/managed_start_linux_test.go` (new)

- `apps/backend/internal/launcher/service.go`
- `apps/backend/internal/launcher/service_test.go`
- `apps/backend/internal/launcher/service_survival_linux_test.go` (new)
- `apps/backend/internal/agent/runtime/agentctl/launcher/sysprocattr_linux_test.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_stop_detach_test.go`
- `docs/public/run-as-a-service.md`
- Owning requirement/design pair and these plan/work-order status/results.

## Dependencies

None. Existing application lifetime controls are prerequisites, not new work.
Read unchanged session-state requirements and design part 2's shutdown/platform
gating sections. Preserve registry metadata and prod/dev/e2e defaults.

## Risks

A unit can be stopped while retained agents still consume resources. Disabled
kernel/pipe guards are distinct from graceful manager/provider cleanup and cannot
prove it end to end. Integration readiness, race exit delay and fixture identity
must be handled explicitly. Any required broader cleanup redesign exceeds this
work order.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/executors/requirements/agent-survival-across-restart.md), AC 001.11-.13.
- [Lifetime design](../../specs/executors/system-design/agent-survival-across-restart-01.md#native-linux-service-lifetime).
- Existing `service_test.go`, `survival_kill_path_test.go`,
  `launcher_pipe_unix_test.go`, `cmd/agentctl/main_test.go`,
  `manager_stop_detach_test.go`, and `agentctl_survival_gate_test.go`.

## Results

The launcher/backend guard is implemented. The real-start regression first failed
because the backend survived launcher SIGKILL. After correction, the full launcher
package passed (25.996s), including immediate SIGTERM-marker checks and two-second
healthy caller-return observation, absent-metadata/other-label controls and start
failure. The forced real-systemd test executed all five three-actor cases without
skips (8.76s; package 9.839s), covering normal stop, mixed control and abrupt launcher
death with survival enabled/disabled. All 15 actors were absent and all five units
inactive/unloaded after cleanup. Flag metadata, profiles, adoption and provider
protocols remain unchanged.

- Renderer regression first failed on both generated scopes because the original
  output was `mixed`, not `process`.
- The final launcher/agentctl-launcher command passed both full packages
  (18.257s and 3.842s), including the production Linux process-attribute parent-death
  test and existing launcher stop/liveness controls.
- Manager, parent-pipe and reversal commands passed (1.331s, 1.126s and 1.141s).
  Explicit Stop and launch rollback terminate; enabled backend shutdown detaches;
  disabled shutdown and authenticated reclaim controls remain passing.
- Forced-opt-in real-systemd command executed all three cases with no skips:
  retained/process, ordinary/process and mixed causal control passed in 3.81s
  (package 4.876s). Distinct SID/shared cgroup and process birth identity were
  verified. All six actors were absent and all three owned units inactive/unloaded
  after cleanup. This proves the service boundary, not provider continuation.
- Canonical `lint-go-changed` with all nine final changed Go paths passed with zero issues.
- Catalog validation passed (388 decisions, 1656 specs); all 36 specification-tool
  tests and all-spec lint passed. Public-doc validator tests passed 63/63 and
  validated 48 published pages. Delivery-reference preflight returned
  `ok:true,status:covered,errors:[]` using actual added/modified paths.
- `git diff --check` passed; both plan files are present. No broad backend,
  frontend or browser suite was run.

Exact verification commands and resource settings are in the Verification section.
Public docs updated: `docs/public/run-as-a-service.md`, primarily a how-to guide.
