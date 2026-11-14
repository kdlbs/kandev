---
id: "04-human-questions"
title: "Preserve native human questions"
status: in_progress
wave: 3
depends_on:
  - 02-subscription-policy
plan: "plan.md"
requirements:
  - REQ-AGENTS-ANTIGRAVITY-ACP-012
acceptance_criteria:
  - AC-AGENTS-ANTIGRAVITY-ACP-012.1
  - AC-AGENTS-ANTIGRAVITY-ACP-012.2
  - AC-AGENTS-ANTIGRAVITY-ACP-012.3
system_design:
  - ../../specs/agents/system-design/antigravity-acp-agent.md
---

# Task 04: Preserve native human questions

## Summary

Prevent native Antigravity questions from being answered by tool auto-approval.
Reuse pending permission presentation and exact native option responses on desktop and phone.

## In scope

- Classify Antigravity `interaction_` requests as human input before coordinator, managed-MCP, or generic auto-approve branches.
- Carry classification through the normalized permission path without guessing from question prose or option count.
- Preserve question labels, option IDs, request identity, and existing user response/deduplication behavior.
- Keep manual and automated policy for ordinary tools unchanged.
- Make registered-agent acpdbg stop a recognized human-question capture without choosing an option. Actual human-answer qualification uses the product UI.
- Let unattended sessions and Office wait for input. Exercise stop/cancel/stale generation behavior.
- Reuse existing question/permission UI; localize any explanatory copy and retain phone sheet/touch behavior.

## Out of scope

A new form schema, generic multi-provider question inference, a new approval setting, and automatic answers.

## Acceptance

1. With auto-approve enabled, a native question remains pending and no option response is sent until the user selects one.
2. The exact selected option is sent once; stop/cancel/stale responses retain safe behavior on desktop and phone.
3. Ordinary tool, coordinator, managed-MCP, and other-provider approval paths retain their tested policies.

## ASCII UI preview

Excerpt of [UI-03](plan.md#ui-03-task-human-question-and-usage-details), covering 012.1-012.3.

```text
Desktop pending question              Phone existing question sheet
+-------------------------------+     +----------------------------+
| Antigravity needs your answer |     | Your answer is needed      |
| Which approach?               |     | Which approach?            |
| [Option A] [Option B]          |     | [        Option A        ] |
| Waiting for your choice       |     | [        Option B        ] |
+-------------------------------+     +----------------------------+
```

Use native labels, never translated control-flow option IDs. Phone actions are at least 44px and wrap.
This changes which requests remain pending; it does not require a new question renderer.

## Verification

Run from the repository root.

```bash
(cd apps/backend && go test -trimpath ./internal/agentctl/acpcompat ./internal/agentctl/server/acp ./internal/agentctl/server/process -run 'TestAntigravity|Test.*Permission|Test.*AutoApprov|Test.*Coordinator' -count=1)
(cd apps/backend && go test -trimpath ./internal/agent/acpdbg -run 'TestAntigravityDebuggerHumanQuestion' -count=1)
(cd apps/backend && go test -trimpath -race ./internal/agentctl/server/process -run 'TestAntigravity|Test.*Permission' -count=1)
(cd apps/web && pnpm e2e:run --project chromium tests/chat/antigravity-question.spec.ts tests/chat/agent-permission-unattended.spec.ts tests/chat/automatic-permission-presentation.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/chat/mobile-antigravity-question.spec.ts)
(cd apps/web && pnpm run i18n:check)
git diff --check
```

Named tests: `TestAntigravityHumanInputNeverAutoApproves`, `TestAntigravityPermissionOptionsPreserved`, and `TestAntigravityDebuggerHumanQuestion`.
Use a live negative-request observer armed before the question arrives. A timeout alone is not proof of no auto-answer.
Include another provider using a similar prefix and ordinary Antigravity tool approval as control cases.
Run changed-Go lint and targeted web lint/typecheck if UI copy/types change.

## Files likely touched

- `apps/backend/internal/agentctl/acpcompat/antigravity.go` and tests.
- `apps/backend/internal/agentctl/server/acp/client.go` and permission tests.
- `apps/backend/internal/agentctl/types/streams/` permission payload definitions if classification needs a field.
- `apps/backend/internal/agentctl/server/process/manager.go`, focused permission policy helpers, `antigravity_permission_test.go`.
- `apps/backend/internal/agent/acpdbg/` Antigravity question guard and a no-auto-answer regression.
- Existing task permission/question components only if explanatory copy or normalized classification requires it.
- `apps/web/e2e/tests/chat/antigravity-question.spec.ts`, `mobile-antigravity-question.spec.ts`, and synthetic wire fixtures.
- Seven locale catalogs if new visible copy is needed.

## Dependencies

Task 02 supplies shared Antigravity identity and compatibility selection.
Task 03 is not a technical prerequisite for deterministic question tests.

## Risks

- A guard only in generic auto-approve leaves coordinator or managed-MCP paths able to answer. Guard before every policy branch.
- Multiple `allow_once` options are normal for these questions. Choosing the first changes the user's answer.

## Parallelism

`sequential`

## Inputs

- Design section Native human questions.
- Existing process permission/coordinator tests and `agent-permission-unattended.spec.ts`.
- T3 Antigravity native question recognition and fabricated matching ACP frames.

## Results

Implemented.

- Classified Antigravity `interaction_` permission requests as human input before coordinator, managed-MCP, and generic auto-approve branches.
- Carried `human_input` through permission request types, pending snapshots, lifecycle payloads, and watcher data.
- Unattended orchestration and Office taskless permission resolution now leave human-input requests pending instead of selecting or cancelling an option.
- Registered-agent acpdbg stops a recognized native question capture without writing a response; raw `--exec` stays exempt.
- Added `TestAntigravityHumanInputNeverAutoApproves`, `TestAntigravityPermissionOptionsPreserved`, and `TestAntigravityDebuggerHumanQuestion` with other-provider and ordinary-tool controls.

Verification: targeted backend and race tests passed; changed-Go lint 0 issues. Live model-generated question observation remains Task 07 evidence.

Desktop and phone question E2E specs remain open; deterministic manager, ACP, and acpdbg coverage is complete.
