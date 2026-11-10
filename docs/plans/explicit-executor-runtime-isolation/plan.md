---
created: 2026-11-08
status: done
requirements:
  - REQ-EXECUTORS-RUNTIME-SELECTION-001
  - REQ-EXECUTORS-RUNTIME-SELECTION-002
  - REQ-EXECUTORS-RUNTIME-SELECTION-003
  - REQ-EXECUTORS-OWNERSHIP-004
  - REQ-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-001
  - REQ-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-003
system_design:
  - ../../specs/executors/system-design/runtime-selection.md
  - ../../specs/executors/system-design/per-user-ownership.md
  - ../../specs/tasks/system-design/additional-session-workspace-reuse.md
legacy_specs: []
---

# Plan: explicit executor runtime isolation

## Overview

Enforce the selected runtime before fresh work, then close retained-execution bypasses.
Expose actual execution identity through existing inventory and additive session API fields.
Finally prove the boundary in disposable container tests and update public guidance.
All four work orders are implemented. Container tests pass against a real daemon for worker
identity, namespace separation, daemon-failure refusal, and the disposable containerized-backend
topology with distinct backend/worker container IDs, retained-mismatch refusal, and exact Stop.

## Evidence and confidence

Investigation base and remote main were `9aba7b2edd020f6ba0f92c2bfe318429723ea2ed` on 2026-11-08.
The workspace started clean and already used that commit. No parent checkout was changed.
Source references below describe this baseline, not the older parent triage revision.

[Issue #5189](https://github.com/kdlbs/kandev/issues/5189) remained open and unassigned at the fresh checks.
Its canonical body had no comments or attachments. The reporter describes stock v0.99.1, repository-free and retained repository tasks, and authenticated processes inside the backend container.
Those deployment observations are reporter evidence. They were not independently reproduced here.

### Confirmed source path

1. `Dockerfile:130` sets `KANDEV_DOCKER_ENABLED=false` in the shipped service image.
2. `internal/backendapp/agents.go:88` registers Docker only when enabled. Standalone is always registered.
   The lifecycle constructor uses `ExecutorFallbackWarn` at line 129.
3. `manager_executor.go:getExecutorBackend` maps `local_docker` to Docker correctly.
   On missing registration, allow/warn policies return standalone. Unknown and plugin-remote selections already fail.
4. `launchInternal` resolves profiles and builds the environment before the late backend lookup in `launchBuildExecutorRequest`.
   Orchestrator `buildLaunchAgentRequest` also resolves profile credentials before that lookup.
5. `buildExecutionFromInstance` and `initializeCreatedExecution` use the resolved backend's `Name()` as actual runtime.
   `buildRunningFromExecution` persists that runtime and the returned container ID.
   Requested executor metadata can therefore remain Docker while actual execution is standalone.
6. Cached workspace paths run `ensureCachedWorkspaceExecutionAdmitted`, which checks durable workspace admission but contains no actual-runtime comparison.
   `launchInternal` can return a workspace execution before backend selection. Promotion can then prepare an agent on it.
7. `DockerExecutor.CreateInstance` returns daemon/client and container errors directly.
   No host fallback was found after registered Docker creation fails. This needs regression coverage, not a second fallback mechanism.

Paths in this trace are relative to `apps/backend/`, except `Dockerfile`.

### Disposable diagnosis

A temporary Go overlay added `TestPlanning5189BackendSelectionProbe` without editing repository test files.
It called the current `getExecutorBackend` with only a mock standalone backend registered.
Observed results:

| Probe | Observed result |
| --- | --- |
| Explicit `local_docker`, allow | standalone, no error |
| Explicit `local_docker`, warn | standalone, no error |
| Explicit `local_docker`, deny | unavailable error |
| Empty, local, local_pc, worktree, mock_remote | standalone controls pass |
| Unknown, plugin_remote | refused controls pass |
| Cached execution labelled local_docker with actual standalone | returned by GetOrEnsureExecution |

The cache fixture deliberately had no durable reader. It proves that branch's missing comparison.
The production durable-reader conclusion comes from the read-only source trace, not an integration reproduction.
No agentctl, authenticated agent, container, credential store, journal, or user deployment was started or modified.

Executed command, from `apps/backend`:

```bash
go test -trimpath -tags fts5 -overlay /tmp/executor-runtime-planning-b0gj625s/overlay.json ./internal/agent/runtime/lifecycle -run '^TestPlanning5189BackendSelectionProbe$' -count=1 -v
```

Result: PASS, one top-level probe. The temporary overlay was removed before handoff.
Its output confirms current incorrect behavior. It is not the future expected-behavior regression.

<details>
<summary>Disposable probe source, retained as diagnosis evidence only</summary>

The overlay supplied this file as `internal/agent/runtime/lifecycle/planning_5189_probe_test.go`.
The source was compiled against the unchanged package and its existing test doubles.
It is not a permanent repository test.

```go
package lifecycle
import (
 "context"
 "testing"
 "github.com/kandev/kandev/internal/agent/executor"
)
func TestPlanning5189BackendSelectionProbe(t *testing.T) {
 log := newTestRegistryLogger()
 registry := NewExecutorRegistry(log)
 registry.Register(&MockExecutor{name:executor.NameStandalone})
 for _, p := range []ExecutorFallbackPolicy{ExecutorFallbackAllow,ExecutorFallbackWarn,ExecutorFallbackDeny} {
  m:=&Manager{executorRegistry:registry,executorFallbackPolicy:p,logger:log}
  rt,err:=m.getExecutorBackend("local_docker")
  if p==ExecutorFallbackDeny {if err==nil {t.Fatal("deny unexpectedly succeeded")};t.Logf("local_docker policy=%s error=%v",p,err);continue}
  if err!=nil || rt.Name()!=executor.NameStandalone {t.Fatalf("unexpected result: %v %v",rt,err)}
  t.Logf("CONFIRMED local_docker policy=%s actual=%s",p,rt.Name())
 }
 m:=&Manager{executorRegistry:registry,executorFallbackPolicy:ExecutorFallbackWarn,logger:log,executionStore:NewExecutionStore()}
 for _, selection:=range []string{"","local","local_pc","worktree","mock_remote"} {
  rt,err:=m.getExecutorBackend(selection);if err!=nil || rt.Name()!=executor.NameStandalone {t.Fatalf("control %q failed: %v",selection,err)}
 }
 for _, selection:=range []string{"unknown-runtime","plugin_remote"} {if _,err:=m.getExecutorBackend(selection);err==nil {t.Fatalf("control %q accepted",selection)}}
 old:=&AgentExecution{ID:"planning-execution",TaskID:"planning-task",SessionID:"planning-session",ExecutorType:"local_docker",RuntimeName:executor.NameStandalone}
 if err:=m.executionStore.Add(old);err!=nil {t.Fatal(err)}
 got,err:=m.GetOrEnsureExecution(context.Background(),old.SessionID)
 if err!=nil || got!=old {t.Fatalf("cache result %v %v",got,err)}
 t.Logf("CONFIRMED cache returns selected=%s actual=%s (no durable-reader fixture)",got.ExecutorType,got.RuntimeName)
}
```

</details>

Two existing injected-client tests also passed:

```bash
(cd apps/backend && go test -trimpath -tags fts5 ./internal/agent/runtime/lifecycle -run '^TestDockerExecutor_EnsureClient_(RetriesOnFailure|RecoversAfterTransientFailure)$' -count=1 -v)
```

These tests establish lazy-client retry behavior only. They do not exercise a Docker daemon, worker startup, or UI.

**Confidence for #5189: high** in the missing-registration root cause and the required correction.
The cache/promotion gap is confirmed in source and the bounded fixture.
The reporter's exact process membership, daemon state, and retained history remain unverified.
No lower-confidence companion issue is included or assigned.

## Assumption check and ownership

The user explicitly settled the runtime boundary, preservation rules, and planning-only checkpoint.
Catalog and adjacent-system discovery found no active built-in runtime-selection criterion to reuse for this gap.
The new [runtime requirements](../../specs/executors/requirements/runtime-selection.md) define only selection, retained compatibility, and observed identity.
The [runtime design](../../specs/executors/system-design/runtime-selection.md) owns their technical boundary.

Reuse active criteria without rewriting them:

- [Ownership](../../specs/executors/requirements/per-user-ownership.md), `AC-EXECUTORS-OWNERSHIP-004.1` through `.4`.
- [Workspace reuse](../../specs/tasks/requirements/additional-session-workspace-reuse.md), `AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-001.3`, `.4`, and `003.1` through `.4`.

Executors owns the failed physical execution contract. Agents' runtime-identity display concerns executable/package identity, not executor isolation.
The existing task and auth contracts retain environment and actor authority.
The [ADR](../../decisions/2026-11-08-required-executor-runtime.md) records why explicit fallback and automatic replacement are rejected.
No material unresolved user choice blocks this package.

## Scope

### In scope

- Required selection after current default/profile/workflow resolution.
- Disabled/unregistered Docker, registered but unavailable daemon, container creation failure, and healthy Docker.
- Repository-free and repository-backed launches, workspace-only ensure, promotion, cache/coalescing, resume, restart recovery, and new prompt admission.
- Zero credential/script/worker effects on registration denial, with existing actor/ownership guards preserved.
- Additive actual-runtime projection with independently verifiable Docker identity.
- Existing failure framing, desktop/phone regression checks, and public executor/deployment guidance.

### Out of scope

- Production or permanent-test changes during this planning turn.
- Automatically moving or terminating retained workers, altering workspace modes, or restoring isolation by relabelling metadata.
- Installing or mounting a deployment Docker socket, changing operator configuration, forwarding host credentials, or adding fallback permission.
- Provider-specific lifecycle redesign, new UI controls, schema migrations, feature flags, runtime upgrades, or a metrics subsystem.
- Deferred #5186, #5191, #5169, and #5203. Sibling prompt/handoff and Claude credential/model work remain separately owned.

## Technical approach

### Required admission

Implement the narrow lifecycle operation proposed in the design through the existing orchestrator capability seam.
The effective nonempty executor type requires its mapped backend regardless of the internal allow/warn policy.
An actually empty selection retains standalone. Keep mapping aliases and provider-specific permission semantics unchanged.
Preserve the constructor shape to avoid an unrelated test/consumer migration.

Run registration admission after authorization and before remote secret resolution or managed Git credentials.
Repeat at lifecycle creation before profile preparation and workspace mutation.
Validate backend/instance runtime and Docker identity before publishing success.
Registered provider failures propagate through existing cleanup and failure handling, with no host retry.

### Existing execution admission

Use actual runtime evidence for session and environment caches, coalesced results, promotion, startup, and new prompts.
Inspect retained inventory before resume cleanup and credential leases.
Unavailable or mismatched runtime refusal must not trigger generic stale-runtime teardown or fallback recreation.
Keep inventory and authenticated observation available for exact Stop.
Preserve existing generation, actor, journal, source, and workspace ownership checks.

Registration admission runs before a policy-authorized transition claim.
Reuse compatibility applies to the selected environment's candidate, not a proven predecessor that existing transition policy will retire.
Keep #5175's exact predecessor retirement. A same-binding Docker/standalone mismatch never manufactures permission to detach.

For each request, all evidence needed for that selected execution must agree.
A healthy sibling cannot justify a mismatched selected execution. A mismatched sibling cannot invalidate an independent healthy execution.
Cover both mixed cases with distinct IDs and file sentinels.

### Observed identity

Read `execution_runtime` from the same `executors_running` row that supplies execution and container IDs.
Add nullable read projection fields to full sessions and summaries, including event and boot-payload conversion paths.
Preserve existing event field names and optional container-ID semantics.
Update the handwritten web session type, without introducing rendered content or a new SQL column.

### Compatibility matrix

| Selection or state | Transport / identity | Intended behavior | Evidence / unsupported result |
| --- | --- | --- | --- |
| Empty effective selection | Default standalone | Existing default | Unit controls; missing default errors |
| Local, local_pc, Worktree | Standalone, existing actor | Existing behavior | Alias/ownership controls |
| Mock remote | Test-only standalone mapping | Preserve test contract | Explicit test control |
| Local Docker, unregistered | No worker identity | Refuse before secrets/scripts | Counting fakes; no fallback |
| Docker registered, daemon/create error | Docker only | Propagate safe error | Injected failure; zero host creates |
| Healthy Local Docker | Actual Docker + daemon container ID | Separate worker | Real containers project |
| Retained standalone, selected Docker | Conflicting actual runtime | Block new effects, retain Stop | Cache/promotion/resume/restart tests |
| Retained Docker, missing/conflicting ID | Unproven container identity | Block new work | No inferred identity or replacement |
| SSH, Sprites, Remote Docker, Kubernetes | Existing provider-specific transport/identity | Missing required backend refuses; healthy provider semantics unchanged | Registration matrix and existing reuse/ownership controls; no claim of fresh live provider validation |
| Unknown or plugin unavailable | No admitted backend/provider | Preserve explicit denial | Unit controls; no host materialization |

The internal fallback-policy change affects missing recognized built-ins intentionally.
No supported public option was found that promises execution elsewhere after a concrete selection.
Do not extend this package into provider-specific configuration or shared-workspace support.

## Tests and traceability

All named new tests are proposed, not files added during planning.

| Criteria | Proposed evidence | Work order |
| --- | --- | --- |
| RUNTIME-SELECTION-001.1, .2, .4, .5 | `manager_runtime_selection_test.go`: `TestExecutorRuntimeSelectionPolicies`; `executor_runtime_selection_test.go`: `TestExplicitRuntimeAdmissionBeforeEffects` | 01 |
| RUNTIME-SELECTION-001.1, .3 | `TestExplicitDockerCreationFailureDoesNotUseStandalone`, `TestDockerRuntimeIdentityRequired` in lifecycle selection tests | 01 |
| OWNERSHIP-004.1, .3 | Authorized/unauthorized and durable-actor counting fakes in the admission tests | 01, 02 |
| RUNTIME-SELECTION-002.1, .2, .4 | `manager_runtime_reuse_test.go`: `TestExecutorRuntimeReuseAdmission`; `executor_runtime_resume_test.go`: `TestExecutorRuntimeResumeAdmission` | 02 |
| RUNTIME-SELECTION-002.3; OWNERSHIP-004.2, .4 | `TestExecutorRuntimeMismatchPreservesStop`, `TestExecutorRuntimeRecoveryObservation` | 02 |
| ADDITIONAL-SESSION-WORKSPACE-REUSE-001.3, .4; 003.1-.4 | Same work-order tests plus existing inherited, transition, repository inventory, and rotation regressions | 02 |
| RUNTIME-SELECTION-003.1-.4 | `TestSessionExecutionRuntimeProjection`, `TestSessionDTOExecutionRuntime`, existing persistence/runtime event contracts | 03 |
| RUNTIME-SELECTION-001.1-.3; 002.1-.4; 003.2, .3 | Real Docker launch/denial/reuse/identity matrix | 04 |

Abbreviated criteria in this table use the `AC-EXECUTORS-` prefix, except workspace reuse, which uses `AC-TASKS-`.
Each work order carries full IDs in frontmatter.

## E2E tests and disposable smoke

- `tests/session/executor-runtime-failure.spec.ts`, project `chromium`: disabled Docker refuses through normal user launch, preserves selection, shows existing failure framing, and local control still works.
- `tests/session/mobile-executor-runtime-failure.spec.ts`, project `mobile-chrome`: same failure/retry/selection outcome through existing phone controls.
  No hover-only diagnostics or new phone layout is planned.
- `tests/docker/runtime-boundary.spec.ts`, project `containers`: real Docker success, daemon/create failures, no-repository/repository paths, retained mismatch, resume, and separate-container proof.
  Include the disposable containerized backend described in work order 04, not only the existing host backend fixture.

The healthy container test correlates selected profile, `execution_runtime`, WS `runtime`, execution ID, and the full daemon container ID.
It independently inspects backend and worker container IDs, labels, process membership, and namespace identities.
The backend ID and worker ID must differ. A filesystem marker or hostname assertion is insufficient.

Only an isolated disposable test daemon/host is permitted. Do not point the recipe at a developer's running deployment.
Work order 04 owns fixture topology, mirrored source paths, bounded synthetic canary, and exact teardown.
It must fail a missing required capability explicitly rather than count skipped container cases as proof.

Proposed commands after implementation:

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm e2e:run --host --shards 1 --project chromium tests/session/executor-runtime-failure.spec.ts)
(cd apps/web && pnpm e2e:run --host --shards 1 --project mobile-chrome tests/session/mobile-executor-runtime-failure.spec.ts)
(cd apps/web && KANDEV_E2E_CONTAINERS=1 pnpm e2e:run --host --shards 1 --project containers tests/docker/runtime-boundary.spec.ts tests/docker/docker-launch.spec.ts)
```

The managed runner builds and tears down its resources. Run these commands sequentially with one worker per shard.
The containerized service is a fixture launched by the targeted spec even though the outer runner uses `--host`.
No browser, live Docker, containerized-backend, or end-to-end test ran during planning.

## Public documentation

Update public docs with implementation in work order 04:

- `docs/public/executors.md`, reference: distinguish required selection from the standalone default, including failure and retry behavior.
- `docs/public/docker.md`, deployment how-to: retain the image's disabled-Docker default and the mirrored-path limitation.
  Explain refusal, actual runtime, and separate runner verification. Do not present socket mounting as a fix.
- Use these pages for the additive API diagnostic field because no new page is needed.

Public pages now describe required selection, refusal, retained mismatch, and observed runtime.
Desktop, phone, host-Docker, and containerized-backend specs pass.

## Work orders

- [x] [01: Reject unavailable required runtimes before effects](task-01-runtime-admission.md).
- [x] [02: Enforce the boundary on retained execution](task-02-retained-runtime-admission.md).
- [x] [03: Project observed runtime identity](task-03-runtime-identity-projection.md).
- [x] [04: Prove container boundaries and document behavior](task-04-container-proof-and-docs.md).

Order: 01, then 02, then 03, then 04. Execute sequentially in the primary session after an explicit implementation request.

## Dependencies and overlap decisions

At the planning check, 146 open PR titles/bodies and 100 recently updated merged PRs were inspected through `gh`.
Issue timeline and explicit open-PR search contained no #5189 fix cross-reference.
Semantic searches covered Docker, executor, runtime, environment, fallback, and the affected symbols.

- [#5175](https://github.com/kdlbs/kandev/pull/5175), open at final refresh `0ae5237fb6be3e42c015119ee8de277c9ff141b6`: changes inherited environment transitions.
  It advanced from the initial `3907a47a47f2f0ca9fe70ad52b74b1c02127178e` snapshot during this investigation.
  The refreshed source and contract diff adds durable child claims, canonical convergence, session-scoped runtime retirement, and persisted `task_environment_id` evidence.
  Its new criteria `AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-003.5` through `.8` are branch-owned until merge.
  This package validates registration before its claim and distinguishes a proven transition predecessor from a same-binding fallback worker.
  Preserve its exact-identity retirement, resume/Kubernetes refusals, group preservation, and ownership/unknown-evidence denials.
  Do not copy its new criteria into this package or supersede its transition changes.
- [#5162](https://github.com/kdlbs/kandev/pull/5162), open at `4a9f1a968ba558eeccf1f084bae7c0b64a196569`: persists the selected concrete profile during promotion.
  Its small `promoteWorkspaceExecution` change is distinct. Retain its profile update after successful admission.
- [#5176](https://github.com/kdlbs/kandev/pull/5176), open at `1249b5d8d565b8e4836f06b95b3f9cc55458a1d2`: cold-resume actor admission.
  Do not duplicate its actor-restoration work or treat runtime validation as a substitute.
- [#5184](https://github.com/kdlbs/kandev/pull/5184), open at `29989fc62d8f55d18ab254a80433960b9d4ced1d`: shared profile secret grants and prepared launch actor admission.
  This package adds no grants and preserves actor precedence. Its files can overlap the planned admission insertion.
- [#4713](https://github.com/kdlbs/kandev/pull/4713), open at `51e17cb54302c0e943cbb9e293cebb39ae2e2a26`: retires failed environment owners.
  Runtime mismatch is not permission to retire an environment. Preserve that package's separate ownership predicates.
- Merged [#5200](https://github.com/kdlbs/kandev/pull/5200) and [#5201](https://github.com/kdlbs/kandev/pull/5201) are already in this base.
  Preserve standalone journal-owner adoption and SSH launch-proof handling. Neither corrects missing-runtime fallback.

No stack or merge dependency is required for the confirmed registration defect.
Before implementation, refresh main and these heads, read changed contracts, and reconcile overlapping tests.
Do not cherry-pick adjacent PRs or broaden this task without authorization.

## Assignment

`gh api user` returned `carlosflorencio`.
After the unit probe and a second unassigned/open check, `gh issue edit 5189 --add-assignee carlosflorencio` succeeded.
The live readback confirmed that assignee and OPEN state at `2026-11-08T20:57:13Z`.
No issue comments, PR comments, closure, or PR creation occurred.

## Verification results

Planning diagnosis passed as recorded above.
The document checks run at this handoff are:

```bash
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check -- docs/specs docs/decisions docs/plans/explicit-executor-runtime-isolation
git status --short -- docs/plans/explicit-executor-runtime-isolation
```

Final validation passed: 440 decisions, 1948 specifications, and 36 linter tests.
All specification files and scoped diff checks passed. Untracked package files also passed whitespace and local-link inspection.

The repository coverage preflight used `validateCoverage` from `.github/scripts/pr-docs.cjs`.
Inputs were the five package files and six linked requirement/design documents, read from this working tree.
Actual changed paths returned `exempt` because this turn changes documentation only.
A second evaluation added the planned `manager_executor.go` path to exercise work-order validation.
It returned `covered`: four work orders, seven accepted requirement references, and zero errors.
That second input is a prospective scope check, not a claim that production code changed.
Repeat it locally with:

```bash
node <<'JS'
const fs = require('node:fs');
const v = require('./.github/scripts/pr-docs.cjs');
const dir = 'docs/plans/explicit-executor-runtime-isolation';
const packagePaths = fs.readdirSync(dir).filter(n => n.endsWith('.md')).map(n => `${dir}/${n}`);
const contracts = [
  'docs/specs/executors/requirements/runtime-selection.md',
  'docs/specs/executors/system-design/runtime-selection.md',
  'docs/specs/executors/requirements/per-user-ownership.md',
  'docs/specs/executors/system-design/per-user-ownership.md',
  'docs/specs/tasks/requirements/additional-session-workspace-reuse.md',
  'docs/specs/tasks/system-design/additional-session-workspace-reuse.md',
];
const fileContents = Object.fromEntries([...packagePaths, ...contracts].map(p => [p, fs.readFileSync(p, 'utf8')]));
const changedFiles = packagePaths.map(filename => ({filename, status: 'added'}));
changedFiles.push({filename: 'apps/backend/internal/agent/runtime/lifecycle/manager_executor.go', status: 'modified'});
const result = v.validateCoverage({changedFiles, fileContents});
console.log(JSON.stringify(result, null, 2));
if (!result.ok) process.exit(1);
JS
```

Implementation, affected unit/race/SQLite/DTO tests, web typecheck, lint, the desktop/phone
refusal specs, and the host and containerized real-Docker boundary specs passed. See work order 04.

### Implementation verification (2026-11-09)

```bash
go build ./...                                        # apps/backend
go test -trimpath -tags fts5 ./internal/agent/runtime/lifecycle -count=1
go test -trimpath -tags fts5 ./internal/orchestrator/executor -count=1
go test -trimpath -tags fts5 ./internal/orchestrator -count=1
go test -trimpath -tags fts5 ./internal/backendapp ./internal/task/dto ./internal/task/models -count=1
go test -trimpath -tags fts5 ./internal/task/repository/sqlite -run 'TestSessionExecutionRuntimeProjection' -count=1
go test -trimpath -tags fts5 ./internal/orchestrator/executor -run 'TestLaunchPreparedSessionRejectsRetainedRuntimeBeforeEffects|TestExplicitExecutorProfileParentFailureIsUnresolved' -count=1
go test -trimpath -tags fts5 ./internal/agent/runtime/lifecycle -run 'TestColdWorkspaceEnsureRefusesRetainedMismatch|TestColdLaunchRefusesRetainedMismatch|TestExecutorRuntimeInventoryReadFailureFailsClosed|TestExecutorRuntimeUnknownRemoteIdentityFailsClosed' -count=1
go test -trimpath -race -tags fts5 ./internal/agent/runtime/lifecycle ./internal/orchestrator/executor ./internal/backendapp -run '<runtime-selection filters>' -count=1
cd apps/web && pnpm run typecheck
(cd apps/web && pnpm e2e:run --host --no-build --shards 1 --project chromium tests/session/executor-runtime-failure.spec.ts)
(cd apps/web && pnpm e2e:run --host --no-build --shards 1 --project mobile-chrome tests/session/mobile-executor-runtime-failure.spec.ts)
(cd apps/web && KANDEV_E2E_CONTAINERS=1 pnpm e2e:run --host --no-build --shards 1 --project containers tests/docker/runtime-boundary.spec.ts)
(cd apps/web && KANDEV_E2E_CONTAINERS=1 pnpm e2e:run --host --no-build --shards 1 --project containers tests/docker/containerized-runtime-boundary.spec.ts)
GOLANGCI_LINT_CACHE=/tmp/opencode/golangci-cache golangci-lint run <changed packages> --new-from-rev=9aba7b2e --timeout=12m
```

All Go suites passed, the focused race run passed, the web typecheck passed, the browser and
container specs passed, and lint reported 0 issues for the changed packages. The containerized
spec starts a disposable daemon and backend container, asserts distinct backend/worker full
container IDs and namespaces, and covers retained-mismatch refusal plus exact Stop. The full SQLite
package exceeds the default 10-minute test timeout on this host; the session, summary, and
projection subsets passed.

## Risks

- Required missing built-in runtimes stop falling back under allow/warn. This deliberate compatibility change requires public release guidance.
- Historical standalone workers selected as Docker will block new work. Preserve their truthful inventory and Stop, rather than silently replacing them.
- Existing resume cleanup occurs early. A late-only guard could destroy retained work before reporting mismatch.
- Cache waiters, environment-keyed access, and direct startup/prompt calls can bypass a creation-only correction.
- Provider success cannot prove isolation without a verified identity. New validation must preserve provider-specific cleanup ownership on partial failure.
- The stock service image lacks a supported complete Local Docker deployment by default. The positive smoke needs a disposable mirrored-path topology.
- `execution_runtime` is observed inventory, not a live heartbeat. A stopped row must not become a running claim.
- Actorless cold resume and prepared-profile changes have adjacent owners. Keep their semantics without claiming this package fixes those issues.
- A blanket comparison against every predecessor would block #5175's legitimate transitions. Compare reuse candidates against the selected binding and preserve exact transition authority.
