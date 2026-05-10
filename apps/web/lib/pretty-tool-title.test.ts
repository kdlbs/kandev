import { describe, it, expect } from "vitest";
import { prettifyToolTitle } from "./pretty-tool-title";
import { EXTERNAL_MCP_TOOL_GROUPS } from "./settings/external-mcp-tools";

describe("prettifyToolTitle", () => {
  it("converts a kandev tool name to Title Case with brand prefix", () => {
    expect(prettifyToolTitle("create_task_kandev")).toBe("Kandev: Create Task");
    expect(prettifyToolTitle("list_workflow_steps_kandev")).toBe("Kandev: List Workflow Steps");
    expect(prettifyToolTitle("ask_user_question_kandev")).toBe("Kandev: Ask User Question");
    expect(prettifyToolTitle("move_task_kandev")).toBe("Kandev: Move Task");
  });

  it("uppercases known acronyms", () => {
    expect(prettifyToolTitle("update_mcp_config_kandev")).toBe("Kandev: Update MCP Config");
    expect(prettifyToolTitle("get_mcp_config_kandev")).toBe("Kandev: Get MCP Config");
  });

  it("trims surrounding whitespace before matching", () => {
    expect(prettifyToolTitle("  create_task_kandev  ")).toBe("Kandev: Create Task");
  });

  it("leaves non-kandev MCP tool names unchanged", () => {
    expect(prettifyToolTitle("mcp__github__list_issues")).toBe("mcp__github__list_issues");
    expect(prettifyToolTitle("Read")).toBe("Read");
    expect(prettifyToolTitle("Bash")).toBe("Bash");
  });

  it("leaves Claude-supplied human titles unchanged", () => {
    expect(prettifyToolTitle("Reading file foo.ts")).toBe("Reading file foo.ts");
    expect(prettifyToolTitle("Running `git status`")).toBe("Running `git status`");
  });

  it("does not match strings that merely contain _kandev mid-name", () => {
    expect(prettifyToolTitle("create_kandev_task")).toBe("create_kandev_task");
    expect(prettifyToolTitle("kandev_create_task")).toBe("kandev_create_task");
  });

  it("does not match uppercased or hyphenated variants", () => {
    expect(prettifyToolTitle("CREATE_TASK_KANDEV")).toBe("CREATE_TASK_KANDEV");
    expect(prettifyToolTitle("create-task-kandev")).toBe("create-task-kandev");
  });

  it("returns empty input unchanged", () => {
    expect(prettifyToolTitle("")).toBe("");
  });

  it("prettifies every tool name in the external MCP catalog", () => {
    const names = EXTERNAL_MCP_TOOL_GROUPS.flatMap((g) => g.tools.map((t) => t.name));
    for (const name of names) {
      const out = prettifyToolTitle(name);
      expect(out.startsWith("Kandev: ")).toBe(true);
      expect(out).not.toMatch(/_/);
    }
  });
});
