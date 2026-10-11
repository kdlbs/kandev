---
created: 2026-10-07
status: in_progress
requirements:
  - REQ-EXECUTORS-K8S-FAILURE-RECOVERY-001
  - REQ-EXECUTORS-K8S-VALIDATION-001
system_design:
  - ../../specs/executors/system-design/kubernetes-failure-recovery.md
  - ../../specs/executors/system-design/kubernetes-validation-isolation.md
legacy_specs: []
---

# Kubernetes session resilience

## Overview

Repair cleanup after an agent-container restart and isolate supported heavy
validation from the agent/control container. User-authorized scope is changes
1, 3 and 4 from the session-break investigation. Change 2 (cleanup-error
propagation and session diagnostics) is being implemented in another task.

## Recovery already performed

Original task `a10536a6-9137-4566-af67-f945f831c274`, session
`6268090d-8d41-4f1e-bacd-267e69a3bd90`, resumed through normal `session.recover`
after one narrowly scoped encrypted control-token repair. Pod/PVC were retained;
new execution `7596bb1d-8501-45aa-bcc3-473c200e7a2f` uses native conversation
`01a111d5-5257-7be1-8cef-17926f5bd301`. Four source edits survived. The task
received a sequential-validation guard and continues its own PR work. No new
session, production profile mutation, or prevention code was deployed.
Operational evidence is in the sibling koi repository's
`log/2026-10-07-kandev-session-oom-recovery.md`; the recovery journal is encrypted
and private outside the repository. Do not reuse that repair as a runtime API.

## Scope

In scope: shared stop authentication/persistence, retryable attachment lifetime,
one task-wide validation admission slot, separately bounded descendants, guarded
repository entry points, focused regressions and disposable Kind acceptance.

Out of scope: change 2, filter feature/PR edits, fresh conversation creation,
universal provider shell interception, new DB/public API/UI contracts, automatic
image publication/profile rollout, security isolation between trusted sessions,
and general Docker/Kind/SSH validation parity inside the runner.

## Technical approach

Follow the two linked designs and
[validation boundary ADR](../../decisions/2026-10-07-kubernetes-validation-workload-boundary.md).
Existing task-owned Pod/PVC identity, ownership locks, bootstrap handshake,
encrypted recovery secrets and Docker workspace grant remain authoritative.
Do not persist a new credential before confirming the exact retained workload.
Never release execution ownership merely because a socket is unreachable.

The runner uses the existing privileged companion, with hard child limits and
conservative aggregate budget reservation. Example policy is 2 GiB child + 1 GiB
daemon reserve under the shipped 3 GiB companion. The affected live profile has
different limits and is not changed by this package. Soft Go/browser tuning is
not evidence of hard memory isolation. Node pressure and arbitrary direct shell
commands can still restart the agent; restart-safe cleanup remains necessary.

## Work orders and dependency order

| Order | Outcome | Dependencies |
| --- | --- | --- |
| [01](task-01-restart-safe-cleanup.md) | Authenticated restart-safe stop and lifecycle regressions | None |
| [02](task-02-isolated-validation-runner.md) | Bounded runner, companion accounting gate and admission tests | 01 |
| [03](task-03-guard-validation-entry-points.md) | Make/package entry-point routing and operator/agent guidance | 02 |
| [04](task-04-restart-and-oom-acceptance.md) | Real restart, sibling, OOM and continuation acceptance | 01, 02, 03 |

All work is sequential in the primary session. Waves express dependency order;
no subagents or persistent subtasks are authorized. Unit regressions are written
before the corresponding production changes. Work order 04 adds integration
proof; it is not a separate generic review pass.

## Requirement coverage

Recovery criteria .9-.12 map to 01; .13 maps to 01 and 04. Validation criteria
.1-.5 map to 02 and 04; .2/.3/.6 map to 03 as well. Existing recovery criteria
.1-.8 and task-Pod/Docker ownership contracts remain controls rather than newly
claimed delivery. No rendered interface changes are planned.

## Existing package reconciliation

`kubernetes-recoverable-failure-cleanup/plan.md` remains historically blocked on
its real Kind acceptance; its work owns .1-.5, not this restart-token defect.
`kubernetes-resume-profile-env/plan.md` is done and owns .6-.8. Preserve their
statuses and recorded results; do not promote the shared draft recovery spec
solely because the new work orders pass. Re-run the retained recovery scenario
in 04 and record new evidence here, linking companion evidence only when it
actually resolves its earlier gate. Coordinate with change 2 at integration:
this package does not edit `executor_resume.go` or session error components.

## Verification strategy

Each work order lists runnable commands rooted independently at repository root.
Future output paths/test names are explicitly identified. Execute heavy checks
sequentially, with GOMAXPROCS=2 and GOFLAGS=-p=1. Do not run full lint beside E2E.
Never inject OOM or restart the recovered production Pod for tests. A skipped
opt-in Kind test, zero selected tests, or pre-test fixture failure is not evidence.
The final Kind run must report assertions executed and sanitized resource IDs.

Before design handoff run:

```bash
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
python3 scripts/list-docs.py decisions --format paths
git diff --check -- docs/specs docs/decisions docs/plans/kubernetes-session-resilience
git status --short -- docs/specs docs/decisions docs/plans/kubernetes-session-resilience
```

Use the repository's `.github/scripts/pr-docs.cjs` exported `validateCoverage`
for an offline preflight over these work orders and their referenced documents;
require `covered`, not documentation-only `exempt`. Actual implementation CI
must still evaluate its real changed paths/head revision.

## Risks and unresolved delivery constraints

A consumed bootstrap nonce plus failure of every durable token write cannot
survive a simultaneous backend crash; keep the failure explicit. Shared
credential/instance locks require one order and no DB transaction over network.
A failed stop must retain useful recovery inputs even when its old forward died.
Accounting must be measured on the selected runtime. The Docker slot is a
cooperative admission contract, not security against raw daemon users. One large
check can OOM its own child; report failure and adjust verified limits rather
than silently running it in agent memory. Fixture-local images require explicit
nested-daemon loading. Publishing images and enabling production profiles remain
separate work after acceptance.

## Results

Design package validated on 2026-10-07; implementation authorized by the subsequent “go for it” instruction. Work orders 01, 02 and 03 are implemented with their targeted regression gates passed. Order 04 is blocked on live Kind acceptance: local image load exceeded its600-second bound before either test body. No production profile rollout is included.
`list-docs validate` passed (358 decisions, 1414 specifications); spec-linter
regressions passed (36 tests); all specification lint and diff whitespace passed.
ADR discoverability passed. Offline preflight using the actual PR documentation
coverage validator returned `covered` for all four work orders and their linked
requirements/designs. A planned production path exercised the coverage branch;
no production source edit or real PR coverage result is implied. Recovery operational
success is independent of unimplemented prevention changes. Leave all Kandev
design files unstaged and uncommitted at the required design-package handoff.


Implementation progress: cleanup now retries typed control 401 through serialized durable credential recovery before retiring exact session attachments. Opt-in validation has one bounded Docker slot and a companion accounting gate, guarded Make/E2E entry points, and operator guidance. Change 2 remains excluded. The immutable worker image built with enforced 4 GiB/2 CPU BuildKit limits; its source/tool/browser smoke passed using a task-owned network because this host has no Docker default bridge. Exact disposable Kind evidence remains outstanding.

Final local verification state: all prevention source and regression scenarios
are implemented, but the repaired-fixture run failed before either live fault
assertion path (1 fixture failure, 1 did not run, no retries). A working disposable
Docker/CI runner remains required for order04 and the retained recovery controls.
Do not mark the new validation specs current or the shared recovery spec active
without that evidence. Source/race/entry-point/image-smoke checks are independent
of this integration blocker. No image publication or profile rollout occurred.

CI handoff is prepared: only the manifest shard selecting the resilience spec
builds/verifies the bounded full worker and receives its exact image ID, with
build/blob artifacts retained. The4 preparation process tests and17 E2E workflow
contract cases passed. No workflow dispatch, commit, push or PR creation had occurred at that checkpoint.
The repository spec-driven-development skill requires asking after repeated
focused check failures; local Kind retries are stopped. The user subsequently selected the CI draft PR option, explicitly authorizing
commit, push and draft PR publication despite the local live gate blocker.
The draft PR will provide a clean runner for acceptance. Image publication,
profile rollout and merge remain outside this authorization.

A draft-only focused acceptance job is now prepared to avoid repeated
build/manifest/shard runner allocations for the live gate. It performs the same
bounded image verification and runs all five new/retained Kubernetes scenarios
with zero retries, rejecting skips or missing executed results. Ordinary ready
PR checks retain their existing paths. The focused job remains unexecuted at
this checkpoint; no specification or work-order live status is promoted.


After focused run37635602757 failed two new scenarios (three retained passes,
retries0/skips0), the user authorized targeted corrections and fresh CI acceptance
at the required three-attempt checkpoint. Order04 is in progress again. Its
results record the abnormal restart fault, bounded own-cgroup mapping, actual
small daemon/independent host accounting proof, and historical migration fixture
correction. These do not accept the full Kind/OOM/native gate; new specifications
remain draft until that gate and affected CI checks pass.


Fresh acceptance run37683760716 was cancelled during a26-minute stalled APT/
Chromium setup, before image verification or Kind allocation. Its logs establish
an unreachable Azure Ubuntu mirror. The focused-job-only correction uses the
official HTTPS archive and bounded transport/step deadlines; acceptance assertions
remain unchanged. No new native/containment failure or pass is inferred from this
pre-test interruption. Order04 and the complete live gate remain in progress.
