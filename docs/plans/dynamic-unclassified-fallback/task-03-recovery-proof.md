---
id: "03-recovery-proof"
title: "Prove recovery and document configuration"
status: done
wave: 3
depends_on:
  - 02-runtime
plan: "plan.md"
requirements:
  - REQ-AGENTS-DYNAMIC-AGENT-ROUTING-002
acceptance_criteria:
  - AC-AGENTS-DYNAMIC-AGENT-ROUTING-002.1
  - AC-AGENTS-DYNAMIC-AGENT-ROUTING-002.3
  - AC-AGENTS-DYNAMIC-AGENT-ROUTING-002.4
  - AC-AGENTS-DYNAMIC-AGENT-ROUTING-002.5
  - AC-AGENTS-DYNAMIC-AGENT-ROUTING-002.6
  - AC-AGENTS-DYNAMIC-AGENT-ROUTING-002.9
  - AC-AGENTS-DYNAMIC-AGENT-ROUTING-002.10
  - AC-AGENTS-DYNAMIC-AGENT-ROUTING-002.12
system_design:
  - ../../specs/agents/system-design/dynamic-unclassified-fallback.md
---
# Task 03: Prove recovery and document configuration

## Summary

Prove the extension through existing desktop and phone recovery flows. Publish configuration guidance only with the implemented behavior.

## In scope

- Add an isolated mock-agent scenario for matching unknown terminal failures, plus differing/error-with-effect variants.
- Read `apps/backend/cmd/mock-agent/AGENTS.md` before changing fixtures. Use existing mock scenario registration and no production-only test endpoint.
- Implement the E2E matrix in the plan. Exercise actual existing Retry actions between attempts, not direct engine calls.
- Confirm one logical session and one successor launch, preserved composer state, and route attribution after reload.
- Update the public agent and workflow reference sections with API examples, threshold bounds, step veto, manual retries, and conservative eligibility.
- Keep original default-stop guidance accurate. Add no claims of automatic retries below threshold or universal provider support.
- Record actual checks in every work order and synchronize the plan. Leave unrelated historical companion-package results intact.

## Out of scope

Automatic unknown-error retry loops, shared health changes, Office/utility enablement, and new visual settings controls.

## Acceptance

- Desktop and phone show manual recovery twice, then the successor after the third matching safe failure.
- Off, vetoed, different-diagnostic, and effectful cases remain manual. An unrelated settings save preserves the policy.
- Public docs describe the shipped contract, and all package references and targeted tests pass.

## Verification

Run from the repository root. New tests must fail for the intended missing behavior before production edits.

```bash
(cd apps/backend && go test ./cmd/mock-agent)
(cd apps/web && pnpm e2e:run tests/session/dynamic-unclassified-fallback.spec.ts --project=chromium)
(cd apps/web && pnpm e2e:run tests/session/mobile-dynamic-unclassified-fallback.spec.ts --project=mobile-chrome)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `apps/backend/cmd/mock-agent/scenarios.go`
- `apps/backend/cmd/mock-agent/scenarios_test.go`
- `apps/web/e2e/tests/session/dynamic-unclassified-fallback.spec.ts`
- `apps/web/e2e/tests/session/mobile-dynamic-unclassified-fallback.spec.ts`
- `apps/web/e2e/helpers/api-client.ts`
- `docs/public/agents-and-profiles.md`
- `docs/public/workflow-import-export.md`
- `docs/plans/dynamic-unclassified-fallback/plan.md`

## Dependencies

Task 02 must pass first.

## Risks

The managed E2E runner must rebuild production assets. Mock evidence must enter through the same trusted event path as real provider failures.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/agents/requirements/dynamic-agent-routing.md), `REQ-AGENTS-DYNAMIC-AGENT-ROUTING-002`.
- [System design](../../specs/agents/system-design/dynamic-unclassified-fallback.md), all sections.
- [Package evidence and test matrix](plan.md).
- [Decision](../../decisions/2026-09-28-repeated-unclassified-fallback.md).

## Results

Passed:

- `(cd apps/backend && go test ./cmd/mock-agent)`
- `(cd apps/backend && go test -tags fts5 ./internal/agent/runtime/... ./internal/orchestrator ./internal/backendapp ./internal/task/repository/sqlite)`
- `(cd apps/backend && go test -tags fts5 -race ./internal/agent/runtime/dynamic ./internal/orchestrator ./internal/backendapp ./internal/task/repository/sqlite -run 'Unclassified|Dynamic.*(Race|Conflict)|DynamicRoute.*Concurrent')`
- `(cd apps/backend && go test -race ./cmd/mock-agent)`
- `(cd apps/web && pnpm e2e:run tests/session/dynamic-unclassified-fallback.spec.ts --project=chromium)`
- `(cd apps/web && pnpm e2e:run tests/session/mobile-dynamic-unclassified-fallback.spec.ts --project=mobile-chrome)`
- `(cd apps/web && pnpm run typecheck)`
- `node --test scripts/validate-public-docs.test.mjs`: 62 tests passed.
- `node scripts/validate-public-docs.mjs`: 47 published pages validated.
- `python3 scripts/list-docs.py validate`: 322 decisions and 1221 specifications validated.
- `python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- `python3 scripts/lint-spec-files.py --all`: all specification files passed.
- `git ls-files -m -o --exclude-standard -- '*.go' | xargs -r gofmt -l`: no files required formatting.
- `git diff --check`: passed.

The desktop and mobile flows now prove two manual retries before the third matching safe failure advances to one successor. They also verify draft retention and persisted route attribution after reload. Desktop covers disabled policy, workflow veto, differing diagnostics, output, and tool-effect cases. A focused orchestrator regression proves manual recovery settles the failed turn and publishes the action-required state so the retry control returns to the UI. The mock-agent sequence is keyed by logical Kandev session identity and is covered across candidate ACP sessions.

Review remediation passed:

- `TestHandleAgentFailedManualDynamicRecoveryRunsTerminalCleanup` exercises the actual failure handler with disabled and below-threshold policies. It verifies automation finalization, activity retirement, managed-input settlement, turn cleanup, and no automatic successor.
- `TestUnclassifiedWorkflowContextFencedAtRouteClaim`, `TestUnclassifiedWorkflowStepRevisionFencedAtRouteClaim`, and `TestUnclassifiedWorkflowContextFencedBeforeDetachedLaunch` use deterministic barriers to move the task, edit the step revision, or enable the veto after evidence capture. Each leaves the route in manual recovery with no streak and no successor launch.
- The focused mixed-failure regression verifies that an ineligible classified transport failure resets the unknown-error streak.
- The focused and race-enabled commands listed in [Task 02](task-02-runtime.md) passed after the remediation.
- Session-open recovery now rejects dynamic sessions whose durable route is not active, and generic resume serializes with manual route actions. This keeps action-required and scheduled routes under their explicit recovery owner. `TestResumeTaskSession_DynamicRouteActionOwnsRecovery` and the dynamic-route cases in `TestSessionOpenRecoveryEligibility` passed in the full tagged orchestrator suite.
- Final browser reruns passed: `(cd apps/web && pnpm e2e:run --host --no-build --project chromium e2e/tests/session/dynamic-unclassified-fallback.spec.ts)` and `(cd apps/web && pnpm e2e:run --host --no-build --project mobile-chrome e2e/tests/session/mobile-dynamic-unclassified-fallback.spec.ts)`. One preceding desktop rerun timed out waiting for the successor message; the unchanged immediate rerun passed in 36 seconds.
- PR fixup confirmed that manual retry dispatches the accepted prompt into the resumed candidate session. The route action's own prompt can pass its lock, while unrelated prompts remain fenced. Automatic successor launches now carry their own recovery-attempt identity after a manual retry; the mock-agent logical-session counter survives process restart and retry prompts without the original system envelope.
- Latest PR-fixup verification passed: full `go test ./internal/orchestrator -count=1` and `go test ./cmd/mock-agent -count=1`; the focused race-enabled orchestrator identity/workflow-barrier tests and mock-agent cross-process counter test; `make -C apps/backend build`; and `make -C apps/backend e2e-plugin-package`.
- Earlier PR-fixup E2E runs passed in 43.7 seconds on desktop and 16.2 seconds on mobile.
- A later PR-fixup run reproduced successor startup failure: the lifecycle manager supported initial-prompt dispatch callbacks, but the production lifecycle adapter did not forward them. Added the forwarding method and pinned the capability in the adapter's compile-time interface assertion.
- Final checks after the adapter fix and prompt-setup refactor passed: `(cd apps/backend && go test ./internal/backendapp -count=1)`, `(cd apps/backend && go test ./internal/orchestrator -run '^(TestDynamicRelaunchCreatedSessionCarriesRecoveryAttemptIdentity|TestResumeAttempt_ModelSwitchFallbackCancellationBeforeInitialPromptAcceptance)$' -count=1)`, `(cd apps/backend && go test ./internal/orchestrator/executor -count=1)`, and `(make -C apps/backend build && make -C apps/backend e2e-plugin-package)`.
- The final host E2E reruns passed against that backend build: `(cd apps/web && pnpm e2e:run --host --no-build --project chromium tests/session/dynamic-unclassified-fallback.spec.ts -- --grep 'uses repeated safe failures')` (1 test, 38.0 seconds) and `(cd apps/web && pnpm e2e:run --host --no-build --project mobile-chrome tests/session/mobile-dynamic-unclassified-fallback.spec.ts)` (1 test, 15.6 seconds). The production Vite assets were already current and unchanged.
- After merging base `cb9a530`, the base's final prompt-ownership fence was updated to preserve the route action's own prompt exception while continuing to reject unrelated prompts. `TestRouteActionPromptDispatchOwnershipAllowsOwnRouteLock` verifies both cases. The merged callback API was consolidated, and startup now consumes the base's three callback values while retaining failure settlement.
- Post-merge verification passed: `(cd apps/backend && go test ./internal/orchestrator ./internal/orchestrator/executor ./internal/backendapp -count=1)` and `(cd apps/backend && go test ./internal/orchestrator -run '^(TestRouteActionPromptDispatchOwnershipAllowsOwnRouteLock|TestRouteActionClaimBlocksPromptAdmission|TestDynamicRelaunchCreatedSessionCarriesRecoveryAttemptIdentity|TestResumeAttempt_ModelSwitchFallbackCancellationBeforeInitialPromptAcceptance)$' -count=1)`. `python3 scripts/lint-spec-files.py --all` and `python3 scripts/list-docs.py validate` passed. Managed desktop and mobile fallback E2Es passed after rebuilding the backend, pseudo-locale web assets, and fixture: `pnpm e2e:run --project chromium tests/session/dynamic-unclassified-fallback.spec.ts -- --grep 'uses repeated safe failures'` (1 test, 58 seconds) and `pnpm e2e:run --project mobile-chrome tests/session/mobile-dynamic-unclassified-fallback.spec.ts` (1 test, 20 seconds).
- No requirements or design update is needed: the merge remediation restores the existing internal route-action prompt ownership contract and does not change public behavior.
- The normal changed-package Go hook found two existing-workspace executor tests still using the prior helper signature. Updated the test calls to pass the new optional callback arguments; `(cd apps/backend && go test ./internal/orchestrator/executor -count=1)` passed.
