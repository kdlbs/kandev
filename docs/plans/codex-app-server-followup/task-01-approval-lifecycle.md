---
id: "01-approval-lifecycle"
title: "Approval concurrency and offered decisions"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-CODEX-NATIVE-002
  - REQ-AGENTS-CODEX-NATIVE-003
acceptance_criteria:
  - AC-AGENTS-CODEX-NATIVE-002.2
  - AC-AGENTS-CODEX-NATIVE-002.3
  - AC-AGENTS-CODEX-NATIVE-003.2
system_design:
  - ../../specs/agents/system-design/codex-app-server.md
---

# Task 01: Approval concurrency and offered decisions

## Summary

Keep native event processing active while a user considers an approval.
Preserve the offered provider choices and resolve each pending request at most once.

## In scope

- Recheck the current shared client and adapter; use TDD for gaps that remain.
- Separate waiting server-request handlers from ordered notification dispatch with bounded admission and deterministic shutdown.
- Keep response correlation independent of approval waits. Preserve string and numeric IDs without collisions.
- Retain original structured decisions in the backend and map normalized option IDs to those choices.
- Reject stale or unoffered selections. Apply Kandev permission policy before exposing choices.
- Handle provider resolution, user cancellation, and disconnect without double replies or lingering permission requests.
- Validate question IDs and answer shapes against the original request using the existing clarification contract.
- Test overload explicitly; never silently drop lifecycle events or spawn unlimited handlers.

## Out of scope

New permission UI, protocol payload parsing in the browser, new process hosts, and automatic approval of unknown methods.

## Acceptance

1. An unresolved approval does not block child activity, another RPC response, cancellation, or provider resolution.
2. Only offered and authorized choices can be sent. Structured choices round-trip unchanged, and resolved requests reject late answers.
3. Handler admission, cancellation, and shutdown pass race tests without leaking waiters; existing approval/question controls retain their contract.

## Verification

Run from the repository root:

```bash
(cd apps/backend && go test -race ./pkg/codexappserver ./internal/agentctl/server/adapter/transport/codexappserver)
(cd apps/backend && go test ./internal/agentctl/server/instance)
git diff --check
```

Add `TestApprovalWaitDoesNotBlockNotifications`, `TestOfferedDecisionRoundTrip`, and `TestResolvedRequestRejectsLateAnswer`, or document equivalent tests.
Include explicit overload, disconnect, and simultaneous answer/resolution cases.
If shared permission or clarification contracts change, run their affected tests and the existing browser scenarios identified in the manifest.

## Files likely touched

- `apps/backend/pkg/codexappserver/client.go` and `client_test.go`.
- `apps/backend/internal/agentctl/server/adapter/transport/codexappserver/adapter.go` and approval helpers/tests.
- Existing normalized permission and clarification contracts, only where needed.

## Dependencies

The active implementation session must finish its current work before starting this follow-up.

## Risks

Resolving a request and canceling its handler can race with a user answer. One state transition must own reply eligibility.

## Parallelism

`sequential`

## Inputs

- [Agent design](../../specs/agents/system-design/codex-app-server.md), client and normalization sections.
- [Comparison report](../codex-app-server/research-agent-orchestrator.md), recommendations 1 and 2.
- Existing client, adapter, and debugger request-policy tests.

## Results

Approval handling now runs independently from ordered notification dispatch. The client bounds concurrent requests, preserves RPC ID types, and resolves each request once during user reply, provider resolution, cancellation, or disconnect.

`go test -race` passed for `pkg/codexappserver` and the native adapter transport. Client tests cover notification progress, bounded admission, exact request resolution, and answer-versus-resolution races. Adapter tests cover offered choices and rejection of unoffered choices.

Provider decisions retain their original structured payload. The adapter rejects stale or unoffered selections. Race tests cover request resolution, cancellation, and shutdown. The debugger validates answer-file question IDs, response shape, and offered options.

Direct `item/tool/requestUserInput` requests now route through the existing clarification action and desktop/phone controls. The bridge preserves provider question IDs and choices, maps selected option IDs back to offered labels, supports explicitly allowed free-text-only questions, validates exact answer coverage, and returns empty answer arrays on rejection. Secret questions fail closed because clarification answers are persisted in conversation messages. Provider resolution cancels the pending clarification through the existing session timeout path.

Adapter tests cover option mapping, secret rejection, question cancellation, and rejected responses. API bridge tests cover Kandev session/task identity, option and text answers, response validation, and cancellation notification. Clarification and MCP handler tests cover the text-only validator and preserve the ordinary two-option requirement. The frontend overlay test confirms custom text is hidden when the provider disallows it while omitted metadata retains the existing default. Mobile clarification E2E and final race/lint checks are recorded in the PR delivery results.

Status is complete for implementation. Live Codex 0.154.0 question requests remain unverified; the native executor matrix remains an outstanding Task 07 item.
