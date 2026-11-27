---
id: "02-canvas-guidance"
title: "Deliver missing canvas instructions on follow-up"
status: done
wave: 2
depends_on: [01-late-mcp-input]
plan: plan.md
requirements:
  - REQ-AGENTS-MCP-DISCOVERY-002
acceptance_criteria:
  - AC-AGENTS-MCP-DISCOVERY-002.6
system_design:
  - ../../specs/agents/system-design/mcp-tool-discovery-guidance.md
---

# Deliver missing canvas instructions on follow-up

**Outcome.** A structured kanban session that started without canvas guidance receives it with its next eligible user prompt. The delivery decision uses the profile of the execution that receives that prompt.

1. At successful session attachment, use the local core MCP server's effective profile to arm one reminder in agentctl.
2. At ordinary prompt dispatch, include the canonical canvas section. Clear the reminder only when the provider succeeds and the attachment still owns it.
3. Exercise a loaded conversation through the session-load and prompt endpoints. Check enabled and disabled bindings and inspect the actual provider prompt. Check rejection and stale completion separately.

Implementation uses `apps/backend/internal/agentctl/server/api/canvas_guidance.go` and `internal/sysprompt/sysprompt.go` under the backend. Keeping the receipt with the MCP attachment avoids copying live capability state into session metadata. A new attachment may repeat the instructions once.

## Verification

Run from the repository root.

```bash
(cd apps/backend && go test -trimpath ./internal/{agentctl/server/api,sysprompt,task/handlers,orchestrator,backendapp} -run "CanvasGuidance|ResumeWorkspaceHelperMCP|LaunchPromptIntegrity" -count=1)
```

## Results

The enabled integration case received plain user text before the fix. The corrected provider request contains the authoring instructions. The disabled case receives plain user text.

The Go verification command above passed in all five packages. The command-shape regression failed before the fix. Five runs of `(cd apps/backend && go test -trimpath ./internal/agentctl/server/api -run "CanvasGuidance|PromptOrSteerRouting" -count=5)` passed in one invocation.

Repository checks passed: `python3 scripts/list-docs.py validate`, `python3 scripts/lint-spec-files.py --all`, and `git diff --check`.
