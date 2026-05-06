import { describe, it, expect } from "vitest";
import { resolveDesiredWorkflowId } from "./resolve-workflow";
import type { WorkflowsState } from "@/lib/state/slices";

type Workflow = WorkflowsState["items"][number];

function workflow(id: string, overrides: Partial<Workflow> = {}): Workflow {
  return { id, workspaceId: "ws-1", name: id, ...overrides };
}

describe("resolveDesiredWorkflowId", () => {
  it("keeps the currently active workflow when it is still visible", () => {
    const result = resolveDesiredWorkflowId({
      activeWorkflowId: "wf-2",
      settingsWorkflowId: "wf-1",
      workspaceWorkflows: [workflow("wf-1"), workflow("wf-2")],
    });
    expect(result).toBe("wf-2");
  });

  it("falls back to the persisted settings workflow when active is missing", () => {
    const result = resolveDesiredWorkflowId({
      activeWorkflowId: null,
      settingsWorkflowId: "wf-1",
      workspaceWorkflows: [workflow("wf-1"), workflow("wf-2")],
    });
    expect(result).toBe("wf-1");
  });

  // Regression: c64e835 forced fallback to the first visible workflow when
  // both active and settings ids were null, making "All Workflows" impossible
  // to select with multiple workflows present.
  it("returns null when the user has cleared the filter and multiple workflows exist", () => {
    const result = resolveDesiredWorkflowId({
      activeWorkflowId: null,
      settingsWorkflowId: null,
      workspaceWorkflows: [workflow("wf-1"), workflow("wf-2"), workflow("wf-3")],
    });
    expect(result).toBeNull();
  });

  it("auto-selects the only visible workflow when exactly one exists", () => {
    const result = resolveDesiredWorkflowId({
      activeWorkflowId: null,
      settingsWorkflowId: null,
      workspaceWorkflows: [workflow("wf-only")],
    });
    expect(result).toBe("wf-only");
  });

  it("ignores hidden workflows when resolving the fallback", () => {
    const result = resolveDesiredWorkflowId({
      activeWorkflowId: null,
      settingsWorkflowId: null,
      workspaceWorkflows: [workflow("wf-1", { hidden: true }), workflow("wf-2")],
    });
    expect(result).toBe("wf-2");
  });

  it("does not honor an active id that is no longer visible", () => {
    const result = resolveDesiredWorkflowId({
      activeWorkflowId: "wf-stale",
      settingsWorkflowId: "wf-1",
      workspaceWorkflows: [workflow("wf-1"), workflow("wf-2")],
    });
    expect(result).toBe("wf-1");
  });

  it("returns null when no workflows are visible", () => {
    const result = resolveDesiredWorkflowId({
      activeWorkflowId: null,
      settingsWorkflowId: null,
      workspaceWorkflows: [],
    });
    expect(result).toBeNull();
  });

  it("returns null when active id is stale and settings id is also null", () => {
    const result = resolveDesiredWorkflowId({
      activeWorkflowId: "wf-stale",
      settingsWorkflowId: null,
      workspaceWorkflows: [workflow("wf-1"), workflow("wf-2")],
    });
    expect(result).toBeNull();
  });
});
