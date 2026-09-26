import { describe, it, expect } from "vitest";
import {
  resolveBoardWorkflowId,
  resolveBoardWorkflowSteps,
  resolveDesiredWorkflowId,
} from "./resolve-workflow";
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

  it("preserves All Workflows when exactly one workflow is visible", () => {
    const result = resolveDesiredWorkflowId({
      activeWorkflowId: null,
      settingsWorkflowId: null,
      workspaceWorkflows: [workflow("wf-only")],
    });
    expect(result).toBeNull();
  });

  it("ignores hidden workflows when preserving an empty filter", () => {
    const result = resolveDesiredWorkflowId({
      activeWorkflowId: null,
      settingsWorkflowId: null,
      workspaceWorkflows: [workflow("wf-1", { hidden: true }), workflow("wf-2")],
    });
    expect(result).toBeNull();
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

// The board dropdown offers every workflow in the store — hidden ones included
// (e.g. Improve Kandev) — so an explicitly persisted selection must survive
// reloads instead of silently reverting to All Workflows.
describe("resolveDesiredWorkflowId — hidden workflow selection", () => {
  const HIDDEN_WORKFLOW_ID = "wf-hidden";
  const VISIBLE_WORKFLOW_ID = "wf-visible";

  it("honors an explicitly persisted hidden workflow filter", () => {
    const result = resolveDesiredWorkflowId({
      activeWorkflowId: null,
      settingsWorkflowId: HIDDEN_WORKFLOW_ID,
      workspaceWorkflows: [
        workflow(HIDDEN_WORKFLOW_ID, { hidden: true }),
        workflow(VISIBLE_WORKFLOW_ID),
      ],
    });
    expect(result).toBe(HIDDEN_WORKFLOW_ID);
  });

  it("honors a hidden workflow id from the URL over the persisted filter", () => {
    const result = resolveDesiredWorkflowId({
      activeWorkflowId: HIDDEN_WORKFLOW_ID,
      settingsWorkflowId: VISIBLE_WORKFLOW_ID,
      workspaceWorkflows: [
        workflow(HIDDEN_WORKFLOW_ID, { hidden: true }),
        workflow(VISIBLE_WORKFLOW_ID),
      ],
    });
    expect(result).toBe(HIDDEN_WORKFLOW_ID);
  });

  it("does not auto-select a hidden workflow the user never chose", () => {
    const result = resolveDesiredWorkflowId({
      activeWorkflowId: null,
      settingsWorkflowId: null,
      workspaceWorkflows: [
        workflow(HIDDEN_WORKFLOW_ID, { hidden: true }),
        workflow(VISIBLE_WORKFLOW_ID),
      ],
    });
    expect(result).toBeNull();
  });
});

describe("resolveBoardWorkflowId", () => {
  it("uses the selected mobile workflow before its snapshot hydrates", () => {
    expect(
      resolveBoardWorkflowId({
        isMobile: true,
        selectedWorkflowId: "wf-new",
        focusedWorkflowId: "wf-new",
        hydratedWorkflowId: "wf-old",
      }),
    ).toBe("wf-new");
  });

  it("uses the focused workflow for the mobile All-workflows view", () => {
    expect(
      resolveBoardWorkflowId({
        isMobile: true,
        selectedWorkflowId: null,
        focusedWorkflowId: "wf-focused",
        hydratedWorkflowId: null,
      }),
    ).toBe("wf-focused");
  });

  it("keeps desktop actions tied to the hydrated workflow", () => {
    expect(
      resolveBoardWorkflowId({
        isMobile: false,
        selectedWorkflowId: "wf-selected",
        focusedWorkflowId: "wf-focused",
        hydratedWorkflowId: "wf-hydrated",
      }),
    ).toBe("wf-hydrated");
  });
});

describe("resolveBoardWorkflowSteps", () => {
  const oldSteps = [{ id: "old-step", position: 0 }];

  it("does not reuse steps from a previously hydrated workflow", () => {
    expect(
      resolveBoardWorkflowSteps({
        effectiveWorkflowId: "wf-new",
        hydratedWorkflowId: "wf-old",
        snapshots: {},
        activeSteps: oldSteps,
      }),
    ).toEqual([]);
  });

  it("uses active steps when they belong to the effective workflow", () => {
    expect(
      resolveBoardWorkflowSteps({
        effectiveWorkflowId: "wf-old",
        hydratedWorkflowId: "wf-old",
        snapshots: {},
        activeSteps: oldSteps,
      }),
    ).toBe(oldSteps);
  });

  it("sorts steps from the effective workflow snapshot", () => {
    expect(
      resolveBoardWorkflowSteps({
        effectiveWorkflowId: "wf-new",
        hydratedWorkflowId: "wf-old",
        snapshots: {
          "wf-new": {
            steps: [
              { id: "later", position: 2 },
              { id: "first", position: 0 },
            ],
          },
        },
        activeSteps: oldSteps,
      }).map((step) => step.id),
    ).toEqual(["first", "later"]);
  });
});
