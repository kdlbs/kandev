---
id: "05-tool-authority"
title: "Enforced read-only tool execution"
status: pending
wave: 4
depends_on: ["02-objectives-routing","04-capability-inventory"]
plan: "plan.md"
requirements:
  - REQ-ORCHESTRATION-ASSISTANT-004
acceptance_criteria:
  - AC-ORCHESTRATION-ASSISTANT-004.1
  - AC-ORCHESTRATION-ASSISTANT-004.2
  - AC-ORCHESTRATION-ASSISTANT-004.3
  - AC-ORCHESTRATION-ASSISTANT-004.4
system_design:
  - ../../specs/orchestration/system-design/personal-assistant.md
---

# Task 05: Enforced read-only tool execution

## Inputs

Read the [requirements](../../specs/orchestration/requirements/personal-assistant.md) and [design](../../specs/orchestration/system-design/personal-assistant.md); legacy scenarios S02, S14, S15, and [plan](plan.md), Backend 4 (execution). Read applicable AGENTS.md and implementation skills before editing. The [baseline experiments](experiments.md) are continuation evidence, not completed implementation.

## Acceptance

1. Inspect runs enforce read-only effect policy on native Kandev, plugin, MCP and provider-native paths; unknown effects and unsupported restriction paths fail closed.
2. Every invocation rechecks live scope/tool availability, so stale discovery, revoked profiles and malicious tool output cannot enlarge authority.
3. Read-only experiments may write internal receipts only; mutating operations use durable IDs and unknown-outcome handling.

## Likely files

- apps/backend/internal/orchestration/runtime/authority.go, authority_test.go (new); handler.go, service.go
- apps/backend/internal/backendapp/adapters_assistant_capabilities.go
- apps/backend/internal/plugins/agent_tools.go and its scoped Host capability boundary
- apps/backend/internal/mcp/profile/profile.go; server registry and invocation boundary
- apps/backend/internal/agent/runtime/lifecycle launch capability plumbing
- apps/backend/internal/agentctl/server/adapter and MCP dispatch (only supported restriction paths)

## Implementation sequence

Write denial tests before adding the policy intersection evaluator. Carry mode/authority as backend-resolved runtime context, not caller-selected tool names. Prove how each provider-native tool path is restricted; refuse inspect launch for unsupported profiles. Plugin hints are untrusted advisory metadata unless backed by enforced operations/capabilities. Use synthetic tools with mutation counters.

## Verification

Run each parenthesized command from the repository root. Use the repository Go/Node/pnpm toolchains. Scoped Go tests are intentional: the available make test target runs the entire backend. New test filters must select the named new tests; a no-tests-to-run result does not satisfy acceptance.

```sh
(cd apps/backend && go test -tags fts5 -count=1 ./internal/orchestration/runtime ./internal/backendapp -run 'TestAssistant(Authority|ReadOnly|Revocation)')
(cd apps/backend && go test -tags fts5 -count=1 ./internal/plugins ./internal/mcp/server ./internal/agent/runtime/lifecycle -run 'TestAssistant|Test.*(PluginTool|Conversation|MCPProfile)')
```

## Dependencies and risks

Dependencies: `02-objectives-routing`, `04-capability-inventory`. Execute in the primary session unless the user explicitly authorizes subagents.

A shell or provider-native write tool can bypass an MCP-only restriction. Never label a prompt-only setup safe/read-only; record unsupported backends honestly. No broad allowlist changes.

## Output

One demonstrably constrained inspect path and explicit incompatibility for unsupported execution profiles.

## Detailed implementation checklist

1. Enumerate effect paths before coding: native task mutations, orchestration
   commands, plugins, external MCP, provider-native tools, shell/filesystem,
   executor APIs and queued/resumed invocations. For each identify the actual
   enforcement owner and the observable negative test. Unknown effect is denied.
2. Implement a run-mode authority evaluator that intersects current owner,
   binding/intent/context revisions, workspace/profile scope, explicit grants and
   operation effect. Apply at invocation and immediately before external dispatch.
   A cached directory entry or accepted intake is not continuing authorization.
3. Make native write handlers deny inspect even with an otherwise valid runtime
   token. Keep minimal conversation/audit receipts allowed; independently measure
   repository, task, workflow, integration and configuration mutation counters.
4. Select one provider/executor combination that can demonstrably disable or
   constrain all native write/shell tools. Verify its current official interface
   and implementation; document version/flags and runtime detection. If none can
   enforce the requirement, return unsupported before launch. Do not invent
   provider flags or ship an instruction-only read-only promise.
5. For plugins admit only proven operations backed by restricted host capabilities;
   manifest read-only hints alone are insufficient. Withhold unknown external MCP
   effects. Recheck user/resource authorization inside the authorized operation.
6. Propagate policy to resume, steer, queued dispatch and session reattachment.
   Revoke existing broker/runtime authorization on owner/profile/grant changes.
   Record stable prepared/dispatched/acknowledged/failed/unknown operation receipts
   and reconcile ambiguous delivery before retrying a possible write.
7. Document the supported inspection matrix, disabled reasons and precise internal
   receipt exception. Update a durable ADR if choosing a new enforcement boundary;
   do not claim cross-provider support from one tested adapter.

## Detailed evidence map

| Criterion | Planned evidence | Required edge cases |
| --- | --- | --- |
| AC-ORCHESTRATION-ASSISTANT-004.1 | `TestAssistantReadOnlyEffectMatrix` and provider adapter negative tests | Direct native write, shell, filesystem, unknown MCP, plugin hints, unsupported startup |
| AC-ORCHESTRATION-ASSISTANT-004.2 | `TestAssistantAuthorityRevocation` | Revoke between discover/prepare/dispatch, stale queue, changed profile, resumed session |
| AC-ORCHESTRATION-ASSISTANT-004.3 | `TestAssistantReadOnlyMutationCounters` | Internal receipt allowed, task/repository/config/integration baseline unchanged |
| AC-ORCHESTRATION-ASSISTANT-004.4 | `TestAssistantAuthorityUnknownDelivery` | Timeout after effect, restart before acknowledgement, stable receipt, no blind retry |

Add fault injection at the dispatch seam rather than sleeps. Unit tests use fake
providers with mutation counters; task 11 adds an isolated supported-provider
trial. Tests must cover the rejected calls, not merely inspect a prompt string.

## Scope boundaries and delivery

No broad allowlist, shell-policy or human-permission bypass is authorized. Provider
quality is separate from enforcement. This work order may truthfully deliver one
supported path and explicit unavailability elsewhere. Native permission answers
remain human-only in task 07, including during a maintenance grant.

## Parallelism

`sequential`

## Results

Pending. Record red/green test evidence, exact commands and counts, relevant artifacts, owned changes and cleanup here; synchronize the plan checkbox only after acceptance is met.
