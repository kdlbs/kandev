import { describe, expect, it } from "vitest";
import { automationMCPEvidence } from "./automation-mcp-evidence-model";
import type { Automation, AutomationRun } from "@/lib/types/automation";
import type { BuiltinMCPSessionView } from "@/lib/types/builtin-mcp";
const automation = {
  workspace_id: "workspace",
  task_mode: "automation_run",
  mcp_selection: { builtin_packages: ["core", "configuration"], definition_ids: [] },
} as unknown as Automation;
const run = { task_id: "task", session_id: "session" } as AutomationRun;
describe("automation MCP evidence", () => {
  it("does not infer process acceptance from today's saved choices", () => {
    expect(automationMCPEvidence(automation, run)).toEqual({
      desired: ["core", "configuration"],
      bound: undefined,
      observed: [],
      pending: false,
    });
  });
  it("ignores another task's evidence", () => {
    const view = {
      task_id: "foreign",
      session_id: "session",
      workspace_id: "workspace",
      binding_state: "accepted",
      bound: { session_id: "session", manifest: { task_id: "task", packages: ["diagnostics"] } },
    } as unknown as BuiltinMCPSessionView;
    expect(automationMCPEvidence(automation, run, view).bound).toBeUndefined();
  });
  it("shows a pending difference only against an accepted process", () => {
    const view = {
      task_id: "task",
      session_id: "session",
      workspace_id: "workspace",
      binding_state: "accepted",
      bound: {
        session_id: "session",
        manifest: { task_id: "task", packages: ["core"], automation_id: "automation" },
      },
    } as unknown as BuiltinMCPSessionView;
    expect(automationMCPEvidence(automation, run, view).pending).toBe(true);
    expect(
      automationMCPEvidence(automation, run, { ...view, binding_state: "unknown" }).pending,
    ).toBe(false);
  });
});
