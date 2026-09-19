---
id: "02-workspace-agent-chat"
title: "Expose managed conversation chat to native plugin UI"
status: completed
wave: 2
depends_on:
  - "01-port-agent-conversation-host"
plan: "plan.md"
requirements:
  - REQ-PLUGINS-PLUGINS-001
acceptance_criteria:
  - AC-PLUGINS-PLUGINS-001.2
  - AC-PLUGINS-PLUGINS-001.7
  - AC-PLUGINS-PLUGINS-001.9
  - AC-PLUGINS-PLUGINS-001.11
system_design:
  - ../../specs/plugins/system-design/plugins-01.md
---

# Task 02: Expose Managed Conversation Chat to Native Plugin UI

## Summary

Expose a managed AgentConversation descriptor through the generic
`host.ui.WorkspaceAgentChat` surface. The Host owns descriptor resolution,
transcript reads, dispatch, subscription, and terminal lifecycle presentation.

## In scope

- Add the typed `WorkspaceAgentChat` SDK and host UI contract.
- Resolve a supplied workspace/session descriptor under the authenticated user,
  owning plugin, current installation generation, and exact managed task scope.
- Carry the resulting short-lived managed grant through source-backed v2 reads,
  turn dispatch, and live subscriptions, revalidating it before delivery.
- Reject foreign plugin, workspace, task, stale-generation, replaced, and
  deleted descriptors without exposing the hidden backing task in Kanban.
- Surface terminal removal through the public `deleted` status, keep read-only
  rendering available, and preserve responsive desktop and mobile presentation.
- Document the public host surface and author-facing descriptor usage.

## Out of scope

- A general browser metadata query or task-enumeration API.
- Coordinator-specific fields, roles, settings, or plugin code.
- Restoring the retired ordered-stream transport in place of source/revision v2
  reads and subscriptions.

## Acceptance

- A native UI plugin with the declared `agent_conversation` capability can render
  and reply through its own managed descriptor without broader prompt-history
  access.
- The Host denies a missing capability, foreign workspace/plugin, mismatched task,
  stale generation, and replaced or deleted descriptor before a transcript or
  turn is delivered.
- Lifecycle cleanup terminates the rendered surface and reports the public
  `deleted` status; desktop and mobile use the same managed contract.

## Verification

```bash
cd apps/backend && go test ./internal/plugins ./internal/gateway/websocket -run 'TestManagedConversationV2|TestConversationBinding' -count=1
cd apps && pnpm --filter @kandev/web test -- --run components/agent-conversation/workspace-agent-chat.test.tsx lib/plugins/conversation-host.test.tsx lib/plugins/conversation-source-scope.test.tsx
cd apps && pnpm --filter @kandev/web typecheck
node .github/scripts/pr-docs.test.cjs
node scripts/validate-public-docs.mjs
git diff --check
```

## Results

- Added the generic SDK contract, host-owned managed descriptor bridge, source/revision
  v2 authorization propagation, lifecycle status handling, fixture routes, and responsive
  coverage.
- Verified focused backend and web regressions, type checking, public documentation,
  and documentation-coverage evaluator tests on the delivery branch.
