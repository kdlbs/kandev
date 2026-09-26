---
id: "02-native-conversations"
title: "Gated native profiles and conversations"
status: done
wave: 2
depends_on:
  - "01-client-and-debugger"
plan: "plan.md"
requirements:
  - REQ-AGENTS-CODEX-NATIVE-001
  - REQ-AGENTS-CODEX-NATIVE-002
  - REQ-AGENTS-CODEX-NATIVE-006
acceptance_criteria:
  - AC-AGENTS-CODEX-NATIVE-001.1
  - AC-AGENTS-CODEX-NATIVE-001.2
  - AC-AGENTS-CODEX-NATIVE-001.3
  - AC-AGENTS-CODEX-NATIVE-001.4
  - AC-AGENTS-CODEX-NATIVE-002.1
  - AC-AGENTS-CODEX-NATIVE-002.2
  - AC-AGENTS-CODEX-NATIVE-002.3
  - AC-AGENTS-CODEX-NATIVE-002.4
  - AC-AGENTS-CODEX-NATIVE-006.1
  - AC-AGENTS-CODEX-NATIVE-006.3
system_design:
  - ../../specs/agents/system-design/codex-app-server.md
---

# Task 02: Gated native profiles and conversations

## Summary and scope

Deliver selectable native profiles and basic chat through the existing executor and process manager.
Wire all runtime-flag layers and enforce disabled behavior across direct and background entry points.
Implement native model probes, permissions, questions, MCP overlay, session resume, and utility inference.

## Exclusions

Child/background detail, new accounting observations, and forks land in dependent work orders.
Do not advertise their capabilities prematurely or migrate existing ACP profiles.

## Likely files and ownership

- `apps/backend/pkg/agent/protocol.go`; new `internal/agent/agents/codex_app_server.go` and tests.
- `internal/agent/agents/managed_npm_runtime*`, `internal/agent/registry/`, `internal/agent/settings/`.
- `internal/agentctl/server/adapter/factory.go`, new `transport/codexappserver/`.
- `internal/agentctl/server/utility/`, `internal/agent/runtime/` and `runtime/lifecycle/`.
- `profiles.yaml`, backend config/runtimeflags, web feature defaults.
- Existing web agent profile forms and native capability selectors.
- New fake app-server executable/scenarios, E2E fixture wiring, and UI-01 tests.

## Acceptance

1. Native profiles launch the selected native runtime, use correct executor credentials/configuration, and pass basic chat/resume/approval/question flows.
2. Disabled HTTP/WS/MCP, utility, dynamic routing, Office, and adoption paths cannot dispatch native work; historical records remain readable.
3. Desktop and phone can create a profile and converse, with localized unavailable/authentication states and unchanged ACP regression behavior.

## ASCII UI preview

UI-01 from [the plan](plan.md#ui-01-profile-creation):

```text
Agent type [Codex app server v]
Profile    [Native coding       ]
Model      [Provider models    v]
Permissions [Existing controls ]
                         [Save]
```

Phone keeps the existing single-column profile form with 44px controls.
Flag off removes new selection but preserves unavailable saved profiles.
Model loading and authentication failure must not silently select another agent.

## TDD and verification

Add `TestNativeRuntimeIdentity`, `TestNativeEntryPointsDisabled`, and native adapter lifecycle tests first.
Use fake-server E2E through the actual new transport.
Fresh worktrees first run `(cd apps && pnpm install --frozen-lockfile)`.
Run from the repository root:

```bash
(cd apps/backend && go test ./internal/agent/agents ./internal/agent/registry ./internal/agent/settings/... ./internal/agent/runtime/... ./internal/agentctl/server/adapter/... ./internal/agentctl/server/utility/... ./internal/runtimeflags ./internal/common/config ./internal/profiles)
make -C apps/backend lint
(cd apps && pnpm --filter @kandev/web test -- lib/state/slices/features/features-contract.test.ts components/settings/agent-profile-page.test.tsx)
(cd apps/web && pnpm run typecheck && pnpm run lint && pnpm run i18n:check)
(cd apps/web && pnpm e2e:run --project chromium tests/settings/codex-app-server-profile.spec.ts tests/chat/codex-app-server.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/settings/mobile-codex-app-server-profile.spec.ts tests/chat/mobile-codex-app-server.spec.ts)
git diff --check
```

## Risks

Native model probes must validate the selected managed Codex command and prepare its private npm prefix before spawn.
Native gateway settings must fail before launch when their translation is not supported.

## Results

Implementation and targeted validation passed. Native lifecycle, MCP overlay, and approval mapping are covered by the fake-server adapter tests. Desktop and mobile profile E2E tests pass with deterministic model responses, and the feature flag remains off by default. The profile UI test checks that the model catalog resolves before capture.

The Codex-specific chat E2E covers completed-turn forks. Fake-server adapter tests cover basic chat, resume, questions through Kandev's injected `ask_user_question_kandev` MCP tool, native `item/tool/requestUserInput` clarification requests, and approval flows. Native questions use Kandev's clarification controls; secret questions fail closed, and provider resolution closes pending clarifications. Desktop and phone E2E cover the choice-only clarification UI. The authenticated Codex 0.154.0 suite did not invoke a native question or approval request, so live compatibility remains unverified.

A follow-up found that the native model probe rejected the actual managed command and did not prepare its trusted npm prefix. [The model-probe work order](../codex-model-probe/task-01-managed-command.md) fixes both paths without relaxing the allowlist. Its default and selected-version builder tests, fake-process model probe, prefix-preparation failure test, four-package race suite, backend lint, and documentation checks pass.
