import { describe, it, expect } from "vitest";
import { EXTERNAL_MCP_TOOL_GROUPS, countExternalMcpTools } from "./external-mcp-tools";

describe("external MCP tool catalog", () => {
  it("matches the backend ModeExternal tool count (46 tools)", () => {
    // Backend ModeExternal registers 47 tool registrations, but list_repositories_kandev
    // is registered twice (in registerConfigWorkflowTools and registerRepositoryTools),
    // leaving 46 unique tools.
    // See apps/backend/internal/mcp/server/server.go (case ModeExternal).
    expect(countExternalMcpTools()).toBe(46);
  });

  it("every tool name is unique and ends with the kandev suffix", () => {
    const names = EXTERNAL_MCP_TOOL_GROUPS.flatMap((g) => g.tools.map((t) => t.name));
    expect(new Set(names).size).toBe(names.length);
    for (const name of names) {
      expect(name.endsWith("_kandev")).toBe(true);
    }
  });

  it("includes create_task_kandev (the only task-spawning tool exposed externally)", () => {
    const names = EXTERNAL_MCP_TOOL_GROUPS.flatMap((g) => g.tools.map((t) => t.name));
    expect(names).toContain("create_task_kandev");
  });

  it("does not expose session-scoped tools (plan, ask_user_question)", () => {
    const names = EXTERNAL_MCP_TOOL_GROUPS.flatMap((g) => g.tools.map((t) => t.name));
    expect(names).not.toContain("ask_user_question_kandev");
    expect(names).not.toContain("create_task_plan_kandev");
  });

  it("every group has at least one tool and a non-empty title", () => {
    for (const group of EXTERNAL_MCP_TOOL_GROUPS) {
      expect(group.title.length).toBeGreaterThan(0);
      expect(group.tools.length).toBeGreaterThan(0);
    }
  });
});
