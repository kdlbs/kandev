---
created: 2026-10-09
status: done
requirements:
  - REQ-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-001
  - REQ-TASKS-MCP-WORKSPACE-MODE-004
  - REQ-EXECUTORS-KUBERNETES-TASK-POD-001
system_design:
  - ../../specs/tasks/system-design/additional-session-workspace-reuse.md
  - ../../specs/tasks/system-design/mcp-workspace-mode.md
  - ../../specs/executors/system-design/kubernetes-task-pod.md
legacy_specs: []
---

# Implementation Plan: Kubernetes inherited workspace admission

## Overview

Reject unsupported cross-task Kubernetes attachment before exposing a session.
Keep deferred subtask creation and its workspace-mode default intact. Preserve
task-owned pod isolation and provide the supported `new_workspace` alternative.

The confirmed failure crosses task workspace inheritance and executor admission:
`prepareSessionForStartWithWorkflowRoute` persists a child session,
`propagateInheritedEnvironment` binds it to the parent's ready environment, and
`KubernetesExecutor.claimTaskKubernetesEnvironment` rejects the differing task
owner only during asynchronous workspace creation. The child becomes FAILED
although no agent process was requested. See [bounded evidence](evidence.md).

## Scope

In scope: common launch preflight, existing-session defense, typed unsupported
reuse projection, paired regression coverage, and public recovery guidance.
Browser ensure response ownership is included after Kind verification exposed
same-task hydration discarding the typed error before recovery presentation.

Out of scope: changing creation defaults, cross-task pod pooling, relaxing UID or
ownership checks, migrating existing tasks, quick-terminal controls, issue #4370,
and the separately observed archive-during-bootstrap cleanup race.

## Technical approach

Resolve inherited canonical ownership and effective executor through the existing
orchestrator inheritance/profile resolvers before session creation/publication.
Use one focused predicate for Kubernetes runtime plus a different known owner.
Apply it to parent-session and group fallback bindings, including `shared_group`.
Keep missing-owner, creating-environment, and executor-mismatch errors distinct.

Add the missing typed unsupported-reuse sentinel in
`internal/task/models/workspace_binding.go`, implementing the already documented
`workspace_reuse_unsupported` contract. Map it to recoverable `CONFLICT` in the
session launch/ensure handlers and MCP spawn path, with guidance that requires an
explicit new task workspace. No browser component, schema migration, or default
executor change is required. Preserve lifecycle rejection for stale or bypassed
bindings; never reuse another task's execution as an attachment handle.

| Runtime and identity | Intended result | Evidence |
| --- | --- | --- |
| Kubernetes, different task owns parent/group environment | Unsupported reuse before session publication | Prepare/ensure/start service and handler tests |
| Kubernetes, same task owns ready environment | Independent session in existing pod | Existing task-pod lifecycle tests |
| Kubernetes, explicit new workspace | Own environment and pod | Kind paired control |
| Existing supported non-Kubernetes inheritance | Preserve current binding | Existing inheritance tests |
| Missing owner, executor mismatch, creating environment | Preserve corresponding typed rejection | Negative service matrix |

## Tests

- `AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-001.4`: add
  `TestKubernetesInheritedWorkspaceAdmission` in a new orchestrator test file;
  drive prepare (workspace on/off), ensure, and start. Assert no new session,
  event publication, runtime allocation, or parent state mutation. Include parent
  primary-session and materialized-group fallback sources.
- `.001.2` and `.001.3`: same-task attachment retains independent execution and
  unchanged parent files/inventory; run existing lifecycle task-pod tests.
- `AC-TASKS-MCP-WORKSPACE-MODE-004.2` and `.004.3`: deferred MCP creation with
  omitted mode still creates `inherit_parent`; explicit `new_workspace` remains
  distinct. Do not make queued creation allocate compute.
- `AC-EXECUTORS-KUBERNETES-TASK-POD-001.6`: verify bypassed legacy bindings
  continue to fail without adopting or deleting parent resources.
- Handler tests verify `CONFLICT`, `workspace_reuse_unsupported`, recoverability,
  safe bounded guidance, and unchanged error categories for other failures.

## E2E tests

Add `apps/web/e2e/tests/kubernetes/kubernetes-inherited-workspace.spec.ts` using
the existing Kind fixture and real API/WS paths in the `containers` project.
Create queued omitted-mode and explicit-new-workspace children of a ready parent
with the same profile. Prepare the inherited child and assert the typed rejection
and no session/pod; prepare the independent child and wait for its own ready
environment. Assert parent pod UID, environment inventory, and a dirty-file
sentinel remain unchanged. No renderer or mobile interaction changes are planned.

## Work orders

- [x] [Task 01: Admit inherited Kubernetes workspaces before session publication](task-01-admission.md)

## Verification results

The user authorized implementation on 2026-10-09. Backend admission and
transport regressions reproduced the late rejection and incorrect error
categories, then passed after correction. The Kind regression reproduced
`INTERNAL_ERROR` from browser `session.ensure`; its green run was interrupted
when the execution pod was replaced.

Validation resumed on 2026-10-13. Specification catalog, specification lint,
public-doc validation, E2E lint, i18n and E2E sleep ratchets, and public-doc
validator tests pass. All seven targeted backend packages pass with `-race`.
The session-hook package and existing recovery/mobile-feedback consumers pass:
93 files, 915 tests. The final rebuilt Kind check passes with no retries: one
test verifies real MCP deferred creation, the browser conflict and recovery
details, an independent child pod, and preservation of the parent pod,
environment record, and dirty-file marker. The disposable cluster was removed.
Go is installed at `/usr/local/go/bin`, outside the default shell PATH.

The final command and result record is in the completed work order.
Authentication was restored and ready PR [#4485](https://github.com/kdlbs/kandev/pull/4485)
was published with desktop and mobile recovery screenshots. Review remediation
also validates the shared-group environment selected inside the binding
transaction, closing the interval after orchestrator preflight. Later review
remediation extends that check to all executor mismatches, rechecks late parent
bindings, and revokes browser response ownership when admission is withdrawn.

## Risks

- A preflight placed only in workspace preparation misses ensure/start or existing
  sessions. All entry points need the common owner predicate and lifecycle guard.
- Group members have legitimate foreign workspace owners for other executors.
  Reject only unsupported Kubernetes attachment; do not ban shared groups globally.
- Automatically changing defaults or relaxing resource ownership would change
  accepted materialization/trust contracts and requires separate product design.
