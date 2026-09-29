---
created: 2026-09-28
status: done
requirements:
  - REQ-AGENTS-DYNAMIC-AGENT-ROUTING-002
system_design:
  - ../../specs/agents/system-design/dynamic-unclassified-fallback.md
legacy_specs: []
---
# Implementation Plan: Repeated Unclassified Fallback

## Overview

[Issue #4015](https://github.com/kdlbs/kandev/issues/4015) requests an opt-in extension to intentional manual recovery.
The user accepted the feature direction on 2026-09-28. The issue is assigned to `carlosflorencio`.
This package records the implemented extension and its verification.

The agent system owns this work because it owns candidate selection and persisted route state.
[Requirements](../../specs/agents/requirements/dynamic-agent-routing.md) and the [design](../../specs/agents/system-design/dynamic-unclassified-fallback.md) define the contract.
The [ADR](../../decisions/2026-09-28-repeated-unclassified-fallback.md) narrows the previous unconditional stop rule.

## Evidence and cause

`routingpolicy.Document` has only transient and hard policies. `PolicyFor` rejects unclassified errors.
`Evaluate` also stops when `FallbackAllowed` is false.
`dynamic.Conductor.nextAfterLaunchFailure` rejects unrecognized wrappers and applies eligibility shortcuts before policy evaluation.
`dynamic.Engine.preparePolicyFailure` currently passes `FallbackAllowed` as `EffectSafe`.
There is no matching-failure streak in `dynamic.PolicyState`.
The browser's `normalizeDynamicPolicy` reconstructs only the existing two policy sections.

The existing focused tests pass:

```bash
(cd apps/backend && go test ./internal/agent/runtime/routingpolicy ./internal/agent/runtime/dynamic -run 'TestEvaluate|TestConductorDoesNotFallbackForUnclassifiedLaunchFailure' -count=1)
```

`TestEvaluateOrdersSafetyResetRetryAndExhaustion/unclassified_stops` establishes the current default.
The conductor test confirms an ordinary `workspace failed` error returns to the caller.
It does not prove every future evidence path, so the new tests must cover real admission and multiple candidates.
The first new behavioral test is `TestUnclassifiedFallbackThreshold` in `dynamic/unclassified_fallback_test.go`.
With three distinct safe matching attempts, it must fail before implementation because no successor is selected.

## Scope

### In scope

- Optional per-candidate policy, workflow veto, and round-trip preservation.
- Proven-safe unknown terminal provider errors and proven agent startup failures in task dynamic sessions.
- Durable matching streaks, manual retry continuity, restart retention, and atomic advancement.
- API, repository, concurrency, desktop/mobile recovery, and compatibility evidence.

### Out of scope

- Automatic retries below threshold, new visual settings controls, and shared health penalties.
- Office/utility behavior changes, classifier expansion, and broad fallback for arbitrary launch errors.
- Runtime flag registration or graduation. This extension uses saved policy and existing routing availability gates.

## Technical approach

First preserve configuration across every write/read boundary while the runtime retains its current behavior.
Then implement typed evidence admission and the durable streak in the existing transition owner.
Finally exercise actual recovery surfaces and document the available API configuration.
All work orders run sequentially. No subagents are authorized.

| Origin or surface | Required identity | Intended behavior | Evidence | Unsupported shape |
| --- | --- | --- | --- | --- |
| Terminal provider result through supported adapters | Execution, prompt generation, complete diagnostic, current effect evidence | Count only eligible task failures | Orchestrator tests and mock-agent E2E | Manual recovery |
| Agent process/session startup | Trusted startup phase and preallocated attempt ID | Count proven pre-dispatch failures | Conductor and lifecycle producer tests | Generic launch wrappers remain manual |
| Classified transient/hard | Existing classifier contract | Unchanged | Existing policy and recovery suites | Existing behavior |
| Office, utility, concrete profiles | Explicit caller scope | No extension | Negative integration cases | Unknown scope refuses admission |

No provider brand is implicitly supported. An adapter without complete terminal evidence stays on manual recovery.
The protocol evidence contract, not a provider name, determines support.

## Companion packages

`docs/plans/provider-error-policies/plan.md` is marked done and owns the original two-class policy delivery.
`docs/plans/dynamic-agent-routing/plan.md` contains historical unfinished entries.
Neither package proves this extension. Their recorded results and statuses remain historical.
The new package owns the added acceptance criteria, tests, and delivery results.
The modified owning designs link this extension rather than revising old test counts.

## Tests

All names below identify planned tests unless explicitly recorded as existing evidence.

| AC suffix | Test file and test |
| --- | --- |
| .1-.2 | `routingpolicy/policy_test.go`: `TestUnclassifiedPolicyValidation`; settings controller `dynamic_profile_test.go`: `TestUnclassifiedPolicyRoundTrip` |
| .3-.4 | `dynamic/unclassified_fallback_test.go`: `TestUnclassifiedFallbackThreshold` |
| .5-.6 | `orchestrator/dynamic_unclassified_fallback_test.go`: `TestUnclassifiedFallbackEvidenceAdmission`; conductor `TestUnclassifiedStartupAdmission` |
| .7-.8 | `dynamic/unclassified_fallback_test.go`: `TestUnclassifiedStreakResetMatrix`, `TestUnclassifiedStreakRestart` |
| .9 | workflow repository/service/handlers `unclassified_fallback_test.go`: `TestUnclassifiedStepVetoRoundTrip`; `stepevents_test.go`: `TestUnclassifiedStepVetoPayload` |
| .10 | browser `agent-profile-normalize.test.ts`: policy preservation cases; `workflow-api.test.ts`: omitted/false/true update cases |
| .11 | task SQLite `dynamic_route_concurrency_test.go`: `TestUnclassifiedFailureClaimOnce`; backendapp `dynamic_routing_test.go`: manual retry continuity |
| .12 | `orchestrator/dynamic_unclassified_fallback_test.go`: `TestUnclassifiedFallbackScope`; runtime resolver negative cases |

All Go paths in this table are relative to `apps/backend/internal/agent/runtime/` unless their owning package is named.
Work orders give complete scope paths and exact commands.
Include a mixed sequence: safe A failure, effectful A failure, safe A failure. It must not reach the threshold from the combined count.
Also cover A/error-X, A/error-Y, A/error-X and stale A evidence after B becomes active.

## E2E tests

Add `apps/web/e2e/tests/session/dynamic-unclassified-fallback.spec.ts` for Chromium.
Add `apps/web/e2e/tests/session/mobile-dynamic-unclassified-fallback.spec.ts` for `mobile-chrome`.
Use a disposable mock-agent scenario with exact eligible diagnostics and no output/effects.
Seed a two-candidate profile through the existing API. Exercise existing manual retry controls between attempts.
Assert two manual stops, one switch on the third matching failure, one session, and successful successor output.
Also cover policy off, step veto, different diagnostics, and output/tool evidence preventing fallback.
Assert the saved API policy survives an unrelated profile edit and reload.
These scenarios cover .1, .3-.6, .9-.10. Unit and integration tests own concurrency and restart matrices.

No rendered layout changes are planned, so no ASCII layout preview is required.
The mobile-parity exception for state/data preservation applies to settings normalization.
Existing phone recovery controls still require the targeted mobile end-to-end proof.

## Work orders

- [x] [Task 01: Preserve policy and workflow configuration](task-01-configuration.md)
- [x] [Task 02: Route repeated safe unknown failures](task-02-runtime.md)
- [x] [Task 03: Prove recovery and document configuration](task-03-recovery-proof.md)

## Verification results

Investigation: focused policy/conductor command passed before this package.
Documentation checks passed:

- `python3 scripts/list-docs.py validate`: 322 decisions and 1221 specifications validated.
- `python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- `python3 scripts/lint-spec-files.py --all`: all specification files passed.
- `.github/scripts/pr-docs.cjs` `validateCoverage`: all three work orders passed linked-reference preflight with a simulated runtime change.
- `git diff --check -- docs/specs docs/decisions docs/plans/dynamic-unclassified-fallback`: passed.

The first specification lint found the edited provider design over its size limit.
The classification summary was shortened without removing its contract, and the rerun passed.

Implementation checks passed:

- `(cd apps/backend && go test ./cmd/mock-agent)`.
- `(cd apps/backend && go test -tags fts5 ./internal/agent/runtime/... ./internal/orchestrator ./internal/backendapp ./internal/task/repository/sqlite)`.
- `(cd apps/backend && go test -tags fts5 -race ./internal/agent/runtime/dynamic ./internal/orchestrator ./internal/backendapp ./internal/task/repository/sqlite -run 'Unclassified|Dynamic.*(Race|Conflict)|DynamicRoute.*Concurrent')`.
- `(cd apps/backend && go test -race ./cmd/mock-agent)`.
- Both work-order Playwright commands passed in the managed runner. It rebuilt the backend and pseudo-locale Vite assets before each run.
- `(cd apps/web && pnpm run typecheck)`.
- Public docs: 62 validator tests passed and 47 published pages validated.
- Specification validation: 322 decisions and 1221 specifications validated; 36 linter tests passed; all specification files passed lint.
- `gofmt` inspection and `git diff --check` passed.

The implementation package and its work orders are complete. Review remediation and final verification are recorded below.

## Review remediation results

The three reported routing defects are fixed and covered by orchestrator-level regressions. Manual `action_required` recovery now reaches the existing terminal cleanup and managed-input settlement path while preserving its durable streak. A current classified failure resets the streak before ineligible fallback gates. Workflow evidence now carries workflow ID and step revision; the SQLite route-claim and detached-launch fences revalidate that snapshot with task/session ownership under the workflow-step and task locks.

Final PR fixup exposed and fixed one production wiring gap in recovery ownership: `lifecycleAdapter` did not forward the lifecycle manager's initial-prompt dispatch callbacks. Automatic successor startup now registers its acceptance/failure callbacks through the adapter, and a compile-time interface assertion pins that capability.

Passed after remediation:

- `(cd apps/backend && go test ./internal/agent/runtime/dynamic ./internal/task/repository/sqlite ./internal/orchestrator -run '^(TestRecordRouteDecisionClaimsInitialGenerationAndWritesAttemptAtomically|TestUnclassifiedWorkflowContextFencedAtRouteClaim|TestUnclassifiedWorkflowStepRevisionFencedAtRouteClaim|TestUnclassifiedWorkflowContextFencedBeforeDetachedLaunch|TestDynamicUnclassifiedStreakResetsAfterClassifiedNonFallbackFailure|TestHandleAgentFailedManualDynamicRecoveryRunsTerminalCleanup|TestPersistPendingDynamicRecoveryLeavesTurnForTerminalFailureCleanup)$' -count=1)`.
- `(cd apps/backend && go test -race ./internal/orchestrator -run '^TestUnclassifiedWorkflow(StepRevisionFencedAtRouteClaim|ContextFencedAtRouteClaim|ContextFencedBeforeDetachedLaunch)$' -count=1)`.
- `(cd apps/backend && go test -tags fts5 -race ./internal/agent/runtime/dynamic ./internal/orchestrator ./internal/backendapp ./internal/task/repository/sqlite -run 'Unclassified|Dynamic.*(Race|Conflict)|DynamicRoute.*Concurrent' -count=1)`.
- `(cd apps/backend && go test -tags fts5 ./internal/agent/runtime/... ./internal/orchestrator ./internal/backendapp ./internal/task/repository/sqlite)`.
- `(cd apps/backend && make build)`.
- `git diff --check`; changed Go files pass `gofmt` inspection.

The first full tagged test pass hit a lifecycle cache-prune temporary-directory cleanup race. The isolated test passed, and the full tagged suite passed on rerun. The final build and race-filtered suite also passed after the last runtime changes. The step-revision barrier regression and its race-enabled run passed afterward.

After the adapter forwarding fix and prompt-setup refactor, backendapp tests, the focused orchestrator cancellation/ownership regressions, and the full executor package passed. The changed-package hook caught two existing-workspace executor tests that needed nil values for the new optional callbacks; their call sites were updated. `make -C apps/backend build && make -C apps/backend e2e-plugin-package` passed. The final host E2E reruns against that rebuilt backend passed: desktop with the `chromium` project and the `uses repeated safe failures` filter (38.0 seconds), and mobile with the `mobile-chrome` project (15.6 seconds). Both used the existing current production Vite assets.

Session-open recovery also rejects a dynamic session while its route is pending or action-required, and generic session resume serializes against manual route actions. The tagged orchestrator suite passed with `TestResumeTaskSession_DynamicRouteActionOwnsRecovery` and the route-state eligibility cases. Final desktop and mobile browser reruns passed. One additional desktop run timed out waiting for the successor message; the unchanged immediate rerun passed in 36 seconds.

## Risks

- Existing safety and semantic flags are coupled. Changing them globally could widen unrelated retry paths.
- Manual retry selection can replace policy JSON and silently lose the streak.
- Duplicate events and same-generation status transitions can overcount without durable fencing.
- Lossy diagnostics cannot prove identical errors. Conservative refusal will limit coverage for some adapters.
- Older clients can erase unknown configuration during whole-profile writes. Current client preservation is mandatory.
- Workflow context can change between failure receipt and launch. Revalidate under the existing settlement owner.
