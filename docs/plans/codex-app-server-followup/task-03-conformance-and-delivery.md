---
id: "03-conformance-and-delivery"
title: "Protocol conformance and PR delivery"
status: complete
wave: 3
depends_on:
  - "01-approval-lifecycle"
  - "02-reconnect-and-usage"
plan: "plan.md"
requirements:
  - REQ-AGENTS-CODEX-NATIVE-002
  - REQ-AGENTS-CODEX-NATIVE-005
acceptance_criteria:
  - AC-AGENTS-CODEX-NATIVE-002.4
  - AC-AGENTS-CODEX-NATIVE-005.3
  - AC-AGENTS-CODEX-NATIVE-005.5
system_design:
  - ../../specs/agents/system-design/codex-app-server.md
---

# Task 03: Protocol conformance and PR delivery

## Summary

Make missing request coverage visible when the pinned protocol changes.
Extend diagnostic fixtures, reconcile the package, and push the verified follow-up to the existing PR.

## In scope

- Enumerate pinned server-request methods from generated protocol evidence. Check each against an explicit supported or deliberately rejected classification.
- Include structured approval and question payload fixtures, not only method-name checks.
- Retain schema digest/version evidence and the tested compatibility behavior for omitted `jsonrpc` fields.
- Extend `codexdbg` synthetic capture/inspect tests with unresolved/resolved requests, interleaved child events, and replayed usage.
- Keep raw capture private, bounded, and opt-in. Report unverified upstream behavior as unavailable evidence.
- Record actual results in this package and reconcile affected original work-order claims.
- Follow commit, push, and applicable PR-fixup skills; identify the existing PR from this branch and task context.
- Commit this package with the completed follow-up, then push to that PR. Preserve other session changes and merge-queue rules.

## Out of scope

Schema upgrades without evidence, copying the external generated client, new debugging commands without a demonstrated need, another PR, and merging.

## Acceptance

1. A newly introduced unclassified server request fails the coverage test. Unsupported requests receive an explicit protocol error.
2. Diagnostic fixtures preserve request and thread correlation without real model calls or raw transcript output in routine logs.
3. Required tests pass, results and remaining limitations are recorded, and the existing PR receives the completed follow-up commit.

## Verification

Run from the repository root:

```bash
(cd apps/backend && go test -race ./pkg/codexappserver ./internal/agent/codexdbg ./internal/agentctl/server/adapter/transport/codexappserver)
make -C apps/backend build-codexdbg
make -C apps/backend lint
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
git status --short -- docs/plans/codex-app-server-followup
```

Add `TestServerRequestCoverage` and diagnostic fixtures in existing debugger test files.
Run any additional impacted tests required by tasks 01 and 02. Record commands, outcomes, and unavailable checks before delivery.
After push, confirm the remote PR head includes the follow-up commit. Continue any previously authorized PR-fixup obligations.

## Files likely touched

- `apps/backend/pkg/codexappserver/schema_test.go` and pinned schema metadata or focused method fixture.
- `apps/backend/internal/agent/codexdbg/request_policy_test.go`, `recorder_test.go`, and inspection tests.
- `.agents/skills/codex-app-server-debug/SKILL.md`, only if diagnostic behavior changes.
- This package, affected original work orders, and owning designs where implementation details need clarification.

## Dependencies

Tasks 01 and 02.

## Risks

A skipped installed-binary check is not compatibility proof. A method inventory is not payload compatibility proof.
Keep the current supported version unless new evidence explicitly justifies changing it.

## Parallelism

`sequential`

## Inputs

- [Agent design](../../specs/agents/system-design/codex-app-server.md), inspection and validation sections.
- [Comparison report](../codex-app-server/research-agent-orchestrator.md), protocol-coverage findings and limits.
- Existing debugger skill, generated schema, and repository commit/push workflow.

## Results

The v0.154.0 schema inventory classifies every server-request method as supported or rejected in both the adapter and `codexdbg`. Tests cover the inventory, structured approval decisions, question answer-file validation, request resolution, interleaved child activity, response correlation, and omission of raw payloads from normal debugger output.

These local checks passed:

- `go test -race ./pkg/codexappserver ./internal/agent/codexdbg ./cmd/codexdbg ./internal/agentctl/server/adapter/transport/codexappserver ./internal/agentctl/server/instance ./internal/agentctl/server/process ./internal/task/usage ./internal/common/costs`
- `make -C apps/backend lint` and `make -C apps/backend build-codexdbg`
- `python3 scripts/list-docs.py validate`, `python3 scripts/lint-spec-files.test.py`, and `python3 scripts/lint-spec-files.py --all`
- `python3 scripts/lint-harness-files.test.py` and `python3 .github/scripts/lint-harness-files.py --all`
- `node scripts/validate-public-docs.mjs` and `node --test scripts/validate-public-docs.test.mjs`
- `pnpm exec vitest run components/task/chat/messages/use-permission-handlers.test.ts components/task/chat/messages/permission-action-row.test.tsx`
- Mobile permission-choice E2E and the fake-server Docker, SSH, and Kind Codex app-server matrix
- `git diff --check`

The native Codex 0.154.0 live suite passed initialization, prompt, resume, fork, shared-workspace behavior, and background completion. It did not observe a permission prompt, exact response usage, or child-to-collaboration-call correlation. The executor matrix used a fake app-server and does not prove upstream Codex compatibility in those executors.

The following PR attempt is historical and superseded by later heads. Commit `afbfafe5a898c2f77a82ac4af5c87c499da772c6` reached the existing [PR #3916](https://github.com/kdlbs/kandev/pull/3916). Current-head CI found a repeated `turn_fallback` string in backend static checks. The backend gate failed because of that lint failure; all backend test shards passed.

The fix uses `nativeUsageSourceTurnFallback`. Local race tests and backend lint passed. That `scripts/pr-await` attempt reached its 45-minute deadline with 11 E2E checks pending.

Direct `item/tool/requestUserInput` requests now route through the existing Kandev clarification controls. Desktop and mobile E2E cover the choice-only UI; the mobile clarification suite passed all 11 cases, and the focused desktop and mobile capture scenarios passed. The PR follow-up also passed focused Go unit/race tests, backend lint, public-doc validation, specification validation, and `git diff --check`. The desktop and phone screenshots are published on the PR media branch and linked from the PR description; they are not committed to the product branch.

Task 01 and this work order are complete for implementation and delivery. The direct-question changes and screenshots are in PR #3916. The later plan-status commit `b472c355d95f38357461c1aeaaf76de4309d3b3e` exposed a race in `TestLSPContinuityReconnectsToSameTaskHostStream`; the release handler sent upstream `exit` before the browser acknowledgement. It now sends the acknowledgement first. The test waits for a signaled lifecycle-fence release before asserting; the targeted test passed 50 race-detector repetitions, and the full WebSocket package passed under the race detector.

Commit `4795ed8244184e421d3a7d2a6b8379e1f15628f9` merged the current `main` into the feature branch and resolved the PR conflict. Focused backend, localization, E2E lint, and merge commit hooks passed. Exact-head `scripts/pr-await 3916` finished with 60 checks passed, 0 failed, and 0 pending. The documentation-coverage publisher temporarily exhausted GitHub code-search rate limits, then passed after cooldown. `scripts/pr-resolve list 3916` returned no unresolved threads; GitHub reports the PR mergeable/clean. The PR remains open. Live Codex 0.154.0 direct-question invocation and native Codex behavior inside Docker, SSH, and Kind remain unverified under original Task 07.
