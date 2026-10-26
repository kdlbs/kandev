---
id: "03-native-connection"
title: "Native launch and secure SSH connection"
status: done
wave: 3
depends_on:
  - "02-builtin-inventory"
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-NOMAD-001
  - REQ-EXECUTORS-NOMAD-002
  - REQ-EXECUTORS-NOMAD-004
acceptance_criteria:
  - AC-EXECUTORS-NOMAD-001.1
  - AC-EXECUTORS-NOMAD-002.1
  - AC-EXECUTORS-NOMAD-002.2
  - AC-EXECUTORS-NOMAD-004.1
  - AC-EXECUTORS-NOMAD-004.2
  - AC-EXECUTORS-NOMAD-004.3
system_design:
  - ../../specs/executors/system-design/nomad-native-executor.md
---

# Task 03: Native launch and secure SSH connection

## Summary

Launch one immutable native job and connect over existing pinned SSH forwarding. Reuse artifact and protocol primitives without invoking SSHExecutor process-launch or process-kill ownership.

## In scope

- Validate dedicated NodeID/platform/SSH mapping and job policy: native raw_exec, one group, no restart/reschedule/replacement, explicit CPU/memory, reserved control/instance ports.
- Reuse AgentctlResolver and existing SSH transfer helpers for selected runtime bytes/digest/atomic rename; private bootstrap nonce/config goes through stdin into exact owned roots, never job env/argv/logs.
- Every bootstrap atomically acquires the non-expiring worker-local start fence outside allocation directories, records immutable owner/launch marker, and cannot launch twice or steal a stale lock. Nomad alone launches bootstrap then exec agentctl. Reuse StartPortForward, remoteControlHandshake/createRemoteAgentInstance and reverse API tunnel; extract only connection-only helper seams and retain SSH regressions.
- Persist authenticated ServerIdentity and drain capability before ACP start. Never call SSHExecutor.CreateInstance/startAgentctlAndHandshake or retry by launching another controller.

## Out of scope

- Rendered UI, Docker/VM execution, plugin implementation, provider SDK/protobuf changes, model-backed workloads, and production rollout.

## Acceptance

- Fake API/SSH tests prove exact job/port/resource policy, enforced-index idempotency, selected artifact integrity and secret-free specification/bootstrap.
- Authenticated SSH with pinned host key forwards the recorded control/instance ports and reverse API without exposing a public runtime; unsupported auth/capability/identity fails closed.
- One Nomad-owned controller is launched; connection failure cannot start a second unmanaged process or bypass cleanup inventory.

## Verification

The focused local contract suite exercises the real Nomad HTTP client and the
SSH/SFTP/agentctl protocol boundaries with deterministic fakes. It does not
start Nomad, Kandev, SSH, or a worker service; the live allocation and
disconnect proof is reserved for the later isolated-worker work order. Run
from the backend directory with the existing SDK dependencies and a task-owned
Go cache:

```bash
GOFLAGS=-p=1 GOCACHE=/private/tmp/kandev-go-build-cache go test -count=1 -v \
  ./internal/agent/nomad \
  ./internal/agent/runtime/lifecycle \
  ./internal/agent/runtime/agentctl \
  ./internal/agentctl/server/api \
  ./internal/backendapp \
  -run '^(TestClient(Register|LooksUp)|TestBindSingle|TestBuildNativeJob|TestNomadRegisterRequest|TestBuildBootstrapScript|TestEncodePrivateEnvironment|TestNomadProfile|TestNomadEnvironment|TestNomadJobID|TestUnmarshalEnvironment|TestEnvironmentState|TestNomadExecutor|TestConnectNomad|TestNomadWorkerPaths|TestNomadLaunchDigest|TestJobMatchesState|TestNomadCreate|TestApplyAuthoritativeNomad|TestBuildLaunchMetadataNomad|TestWireNomadInventory|TestRuntimeSealAndDrain|TestControl|TestNew.*Nomad|TestRegister.*Backend)'

GOFLAGS=-p=1 GOCACHE=/private/tmp/kandev-go-build-cache go test -race -count=1 \
  ./internal/agent/nomad ./internal/agent/runtime/lifecycle \
  -run '^(TestClient(Register|LooksUp)|TestBindSingle|TestBuildNativeJob|TestNomadRegisterRequest|TestBuildBootstrapScript|TestEncodePrivateEnvironment|TestNomadProfile|TestNomadEnvironment|TestNomadJobID|TestUnmarshalEnvironment|TestEnvironmentState|TestNomadExecutor|TestConnectNomad|TestNomadWorkerPaths|TestNomadLaunchDigest|TestJobMatchesState|TestNomadCreate|TestApplyAuthoritativeNomad|TestBuildLaunchMetadataNomad|TestWireNomadInventory)'
```

Results for this implementation: the regular focused command passed in all
five packages, including the full `CreateInstance` success path and durable
intent retention on registration failure. The targeted race command passed in
both Nomad and lifecycle packages. No required test matched zero tests. These
results establish adapter-contract behavior only; they do not establish a
real allocation, process cleanup, retry after backend restart, or client
disconnect behavior.

## Files likely touched

- `apps/backend/internal/agent/nomad/job.go, bootstrap.go and tests (new)`
- `apps/backend/internal/agent/runtime/lifecycle/executor_nomad_connection.go and tests (new)`
- `apps/backend/internal/agent/runtime/lifecycle/executor_ssh_operations.go and executor_ssh_runtime_api.go only for bounded connection helper extraction`
- `apps/backend/internal/agent/runtime/lifecycle/executor_nomad.go and metadata/state tests (new)`

## Dependencies

[Task 02](task-02-builtin-inventory.md).

## Risks

The SSH launch helpers combine connection and independent process ownership; do not reuse them wholesale. Do not reinterpret a controller-local path as remote. Native macOS reservations are not hard limits.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/executors/requirements/nomad-native-executor.md), using frontmatter IDs.
- [System design](../../specs/executors/system-design/nomad-native-executor.md), matching implementation boundary and current source.
- [Plan](plan.md), including strict real-worker runner/prerequisites.
- [ADR](../../decisions/2026-10-24-native-nomad-cleanup-evidence.md).
- Read applicable scoped `AGENTS.md` before editing code.

## Results

Implemented the Nomad `CreateInstance` bridge through profile validation,
durable `intent`/`registered`/`allocated`/`ready` checkpoints, create-only
Nomad job registration, running allocation/task verification, agentctl
handshake, and private bootstrap staging. The complete fake-protocol test
asserts the call sequence, checkpoint phases, nonce exclusion from Nomad job
and command arguments, private bootstrap file mode, and retained connection.
The failure test verifies an uncertain registration remains durable intent
and is not represented as allocated. `StopInstance` and recovery remain
fail-closed for the next work orders; no live services or allocations were
started for this work order.
