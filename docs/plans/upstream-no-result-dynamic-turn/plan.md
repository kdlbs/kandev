---
created: 2026-10-12
status: in_progress
requirements:
  - REQ-AGENTS-DYNAMIC-AGENT-ROUTING-001
  - REQ-AGENTS-DYNAMIC-AGENT-ROUTING-002
system_design:
  - ../../specs/agents/system-design/dynamic-unclassified-fallback.md
  - ../../specs/agents/system-design/dynamic-agent-routing-02.md
legacy_specs: []
---
# Implementation Plan: Dynamic Routing Path Defects

## Overview

This package retains three independent routing corrections: counting authoritative empty normal turns under an enabled candidate policy, reporting non-dynamic route actions as validation errors, and activating a resumed route honestly after runtime readiness.
Candidate identity, attempt ownership, workflow fences, and current execution guards remain authoritative.

| Work order | Scope | Status |
| --- | --- | --- |
| [Task 01](task-01-empty-turn-unclassified-failure.md) | Empty normal turn failure counting | implemented; review remediation in progress |
| [Task 02](task-02-route-action-validation.md) | Typed route-action validation | implemented |
| [Task 04](task-04-restart-recovery-route-activation.md) | Resume-ready activation and explicit recovery | implemented; caller/ownership verification in progress |
| [Task 05](task-05-stranded-waiting-route-sweep.md) | Startup sweep of a stranded WAITING_FOR_INPUT route | implemented |

## Retained contract

Only a normal `end_turn` with trusted no-output/no-effect evidence receives the fixed, text-free empty-turn identity.
The policy must be explicitly enabled, its threshold valid, and the workflow step known without a veto.
Below the threshold recovery stays manual; at the threshold the existing fenced successor path owns the turn.
The counted turn's own completion must preserve its count. Current successful output clears a previous count, including text or plan content delivered only in the complete frame.

Every other admitted origin retains complete diagnostic identity, the existing bounds, and the rejection of diagnostics requiring redaction or truncation.
Post-start unknown failures retain manual recovery. Exit-code grouping and sanitized-prefix grouping were unaccepted proposals and are removed from this package, including their code, tests, work orders, and contract amendments.
This package does not authorize either policy change.

## Recovery behavior

Process-start and resume-ready observations activate the claimed generation for the current execution.
A failed durable activation is returned to the actual resume caller while preserving the ready runtime.
A later admission may reconcile that same generation once storage recovers, using the current live execution and runtime readiness; it must not activate a successor from a predecessor runtime or activate a cancelled session.
An explicit recovery may reclaim a stranded starting route without a live runtime. Passive resumes and other non-active route states retain the route guard.
Blocked explicit recovery returns an error; passive session open retains its waiting response.
The startup orphan-route sweep marks `action_required` a WAITING_FOR_INPUT session on a `starting` route with no executor row; a live row, a read error, and a stale generation keep it unchanged.

## Scope and ordering

Tasks 01, 02, and 04 are independent corrections, reviewed and delivered together.
Task 05 is a later independent correction delivered on its own.
The virtual-profile launch rejection from #4412 is already upstream and has no work order here.
No settings UI, automatic unknown-error retries, new catalogue rules, runtime flags, or Office behavior are added.
Existing account-hold admission from protected main remains in force.

## Verification ownership

Source-adjacent tests protect the real stream handler, resolver, WebSocket handler, SQLite-backed resume admission, and real resume caller.
The original contributor validation reports are historical; they are not current-head hosted CI evidence.
Review remediation uses focused contained red/green tests only. Broad Go/SQL, full frontend, E2E, and native/Windows commands run in hosted CI.
Final exact-head CI and review evidence is recorded externally in the owning task's recovery evidence; it remains pending until the final head passes those gates.

| Contract | Primary proof |
| --- | --- |
| Empty turns, threshold and exclusions | `TestEmptyTurnCompletionCountsUnclassifiedFailure`, `TestEmptyTurnCompletionPreconditions` |
| Complete-frame output survives and clears a prior count | `TestCompleteFrameTextIsNotAnEmptyTurn` |
| Unknown post-start failures stay manual | `TestPostStartUnknownFailuresKeepManualRecovery` |
| Non-dynamic route validation | `TestLoadDynamicRouteActionSessionRejectsNonDynamicProfile`, `TestWSRouteActionReportsNonDynamicProfileAsValidation` |
| Explicit stranded recovery is genuinely admitted | `TestExplicitRecoveryReclaimsStrandedStartingRoute` |
| Live activation ownership | `TestResumeAdmissionActivationRequiresCurrentLiveExecution` |
| Activation failure remains observable and recoverable | `TestActivateResumedDynamicRouteReportsFailedActivation`, `TestResumeAdmissionCompletesRecoverableStartingRoute` plus actual resume-path coverage |

## Current results

Original Backend and E2E workflows completed successfully at the original head.
The original forced frontend workflow failed during typecheck with a Node heap OOM; tests and build were skipped.
The single initial main integration inherits protected main's existing step-scoped frontend typecheck budget. No new global resource limit is introduced.
Focused exact-source diagnostic, admission, and completion-caller harnesses reproduced the contract and ownership failures, then passed with the remediation under the race detector. Their explicit fake boundaries are not SQL or full-stream proof. Permanent real-package caller tests and final hosted suite results remain pending.

Activation reconciliation also verifies the execution record's concrete profile against the current route candidate. Matching the session's execution ID alone is insufficient while a detached successor has been recorded but its predecessor still runs. The shared cancellation/settlement guard serializes observation and activation with successor launch, and a guarded session read revalidates the observed generation, candidate, execution, and nonterminal state.

Projection-write remediation requires an active persisted session projection before admission succeeds. `TestResumeAdmissionWaitsForPersistedActiveRouteProjection` exercises a real SQLite admission whose durable route activation succeeds while the projection write fails, then verifies same-generation repair without replacing the execution. The exact-source focused harness reproduced the false admission and passed after the readback guard. Sixteen earlier real-package cases, full Backend, and full Frontend passed at `66588636ad32`; those receipts remain historical for the projection-remediation head, whose final hosted gates are pending.
