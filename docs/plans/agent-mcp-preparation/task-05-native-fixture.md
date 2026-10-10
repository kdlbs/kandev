---
id: "05-native-fixture"
title: "Prove native task preparation"
status: complete
wave: 3
depends_on: ["02-credential-continuity", "03-runtime-preparation", "04-preparation-ui"]
plan: "plan.md"
requirements:
  - REQ-AGENTS-MCP-PREP-002
acceptance_criteria:
  - AC-AGENTS-MCP-PREP-002.1
  - AC-AGENTS-MCP-PREP-002.2
  - AC-AGENTS-MCP-PREP-002.4
system_design:
  - ../../specs/agents/system-design/agent-mcp-preparation.md
---

# Task 05: Prove native task preparation

## Summary

Prove native task preparation according to the linked requirements and design.

## In scope

Own isolated native fixture harness/test support and compatibility evidence only. Exercise actual Kandev preparation on fresh isolated workspace then native ACP and terminal call to synthetic fixture_ping. No manual enable/login outside preparation, no blanket tool permission and no real provider business operations. Use memory Cursor account store and temporary HOME, never mutate personal keychain/auth files. Reuse /private/tmp/kandev-cursor-native-smoke investigation as reference; durable harness must not contain secrets. Record exact version, commands and marker evidence. Root performs final code review, not this work order.

## Out of scope

Other work-order ownership, commits, personal credential changes and unrelated refactors.

## Acceptance

- Satisfy every referenced acceptance criterion within this boundary.
- Add failing behavior tests first, implement, then pass targeted checks.
- Preserve existing dirty work and report exact validation evidence.

## Verification

```bash
(cd apps/backend && GOCACHE=/tmp/kandev-cursor-review-go-cache go test ./internal/agent/mcpconfig ./internal/agent/runtime/lifecycle -run 'Cursor|MCP' -count=1)
```

## Files likely touched

- `apps/backend/internal/agent/runtime/lifecycle/`
- `docs/plans/agent-mcp-preparation/compatibility-evidence.md`

## Dependencies

02-credential-continuity, 03-runtime-preparation, 04-preparation-ui

## Risks

See plan compatibility and concurrency risks; escalate contract changes to coordinator.

## Parallelism

sequential

## Inputs

- [Requirements](../../specs/agents/requirements/agent-mcp-preparation.md).
- [System design](../../specs/agents/system-design/agent-mcp-preparation.md).
- Existing scoped AGENTS.md, TDD skill and neighboring test patterns.

## Results

Completed on 2026-09-28. The opt-in native tests passed against Cursor Agent
`2026.09.26-dd393fe` on Darwin arm64 (73.772s total):

- Fresh authenticated preparation loads and invokes the synthetic fixture tool
  through both native terminal mode and ACP, retaining explicit ACP permission.
- Missing authentication is classified from the actual native diagnostic.
- After synthetic task-local login, a replacement ACP child loads the exact saved
  conversation ID and regains fixture tools without creating a new conversation.
- The temporary HOME uses an in-memory Cursor account credential store; the
  harness checks that the account token was not written to its temporary files.

See [compatibility evidence](compatibility-evidence.md) for the exact command,
fixture boundaries, and platform/provider limits. This proves native transport
compatibility, not real SaaS consent or provider authorization. Production recovery
is covered separately by lifecycle tests in Task 03.
