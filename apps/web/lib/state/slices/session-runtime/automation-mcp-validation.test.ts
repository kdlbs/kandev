import { describe, expect, it } from "vitest";
import { isBuiltinMCPManifest } from "./builtin-mcp-validation";
describe("automation MCP execution evidence", () => {
  const manifest = {
    version: 2,
    catalog_version: 1,
    workspace_id: "workspace",
    task_id: "task",
    surface: "automation",
    workspace_revision: 0,
    task_revision: 0,
    automation_id: "automation",
    automation_revision: 1,
    packages: ["core", "configuration"],
    origins: { core: "required", configuration: "automation" },
  };
  it("accepts the versioned automation binding", () => {
    expect(isBuiltinMCPManifest(manifest)).toBe(true);
  });
  it("rejects a task masquerading as an automation or missing owner", () => {
    expect(isBuiltinMCPManifest({ ...manifest, surface: "kanban-task" })).toBe(false);
    expect(isBuiltinMCPManifest({ ...manifest, automation_id: "" })).toBe(false);
    expect(isBuiltinMCPManifest({ ...manifest, automation_revision: 0 })).toBe(false);
  });
});
