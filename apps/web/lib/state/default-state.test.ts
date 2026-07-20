import { describe, expect, it } from "vitest";
import { mergeInitialState, type DefaultState } from "./default-state";

describe("mergeInitialState", () => {
  it("does not retain legacy kanban server-state mirrors", () => {
    const merged = mergeInitialState({
      kanban: { workflowId: "wf-1", steps: [], tasks: [] },
      kanbanMulti: {
        snapshots: {
          "wf-1": { workflowId: "wf-1", workflowName: "Development", steps: [], tasks: [] },
        },
      },
      workflows: { activeId: "wf-1" },
    } as Partial<DefaultState>);

    expect("kanban" in merged).toBe(false);
    expect("kanbanMulti" in merged).toBe(false);
    expect(merged.workflows.activeId).toBe("wf-1");
  });
});
