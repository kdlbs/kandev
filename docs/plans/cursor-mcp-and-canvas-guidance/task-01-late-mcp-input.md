---
id: "01-late-mcp-input"
title: "Recognize MCP input on correlated updates"
status: done
wave: 1
depends_on: []
plan: plan.md
requirements:
  - REQ-AGENTS-AGENT-RICH-OUTPUT-002
acceptance_criteria:
  - AC-AGENTS-AGENT-RICH-OUTPUT-002.9
system_design:
  - ../../specs/agents/system-design/agent-rich-output.md
---

# Recognize MCP input on correlated updates

**Outcome.** New rich-output and walkthrough calls reach their existing transcript renderers. Historical package rows keep the current fallback policy.

1. In `apps/backend/internal/agentctl/server/adapter/transport/acp/dialect_cursor_test.go`, add `TestLateMCPInput`. Start Cursor and Grok calls as kind other with no input. Supply the complete envelope on an update. Assert bare arguments, canonical provider/tool, MCP provenance, output and complete status. The supplied reproducer currently returns other.
2. Cover promotion on a nonterminal update followed by output alone. Preserve earlier output when the promotion frame has none. Include a foreign provider with the same tool suffix, invalid non-object arguments, and an already canonical call.
3. In the same directory, change `adapter_tools.go` inside the locked result-update transaction after target resolution. Promote only a tracked generic placeholder for a supported dialect. Keep the promoted payload in activeToolCalls for later frames. Compute canonicalMCPUpdate after promotion. Leave native calls and Codex alias handling intact.

The existing consumer fixtures in `apps/web/components/task/chat/types.test.ts` and `messages/kandev-tool-message-rich-output.test.tsx` exercise the canonical package identity. During acceptance, open a new split-frame rich-output call and a walkthrough in task chat and confirm their existing panels open.

## Verification

Run from the repository root.

```bash
(cd apps/backend && go test -trimpath ./internal/agentctl/server/adapter/transport/acp -count=1)
```

## Results

The captured statusless update failed the provenance assertion before promotion. The full ACP package passed after the change. Both existing frontend consumer suites passed (50 tests). Chromium displayed the package-qualified rich output live and after reload. The walkthrough card navigated changed and unchanged files. The browser fixtures exercise the renderer separately from the local Cursor wire capture.

After a managed build, `(cd apps/web && pnpm e2e:run --no-build --project chromium tests/chat/builtin-rich-output.spec.ts tests/review/walkthrough.spec.ts -- --grep "renders package-qualified previews|floating card walks all steps" --retries=0)` passed both cases in an isolated test environment.
