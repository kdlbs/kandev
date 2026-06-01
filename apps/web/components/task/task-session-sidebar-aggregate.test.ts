import { describe, expect, it } from "vitest";
import {
  aggregateSidebarTasks,
  type SidebarStepInfo,
  type WorkflowSnapshotMap,
} from "./task-session-sidebar-aggregate";
import type { KanbanState } from "@/lib/state/slices";

type KanbanTask = KanbanState["tasks"][number];

function makeStep(
  id: string,
  position: number,
  overrides?: Partial<SidebarStepInfo>,
): SidebarStepInfo {
  return { id, title: `Step ${id}`, color: "bg-neutral-400", position, ...overrides };
}

function makeTask(id: string, workflowStepId: string, overrides?: Partial<KanbanTask>): KanbanTask {
  return {
    id,
    title: `Task ${id}`,
    workflowStepId,
    position: 0,
    state: "TODO",
    ...overrides,
  } as KanbanTask;
}

function makeSnapshot(steps: SidebarStepInfo[], tasks: KanbanTask[]) {
  return { steps, tasks };
}

describe("aggregateSidebarTasks", () => {
  it("returns empty result for empty snapshots", () => {
    const result = aggregateSidebarTasks({});
    expect(result.allTasks).toEqual([]);
    expect(result.allSteps).toEqual([]);
    expect(result.stepsByWorkflowId).toEqual({});
  });

  it("collects tasks and steps from snapshots and tags each task with its workflow id", () => {
    const snapshots: WorkflowSnapshotMap = {
      "wf-1": makeSnapshot([makeStep("s1", 0), makeStep("s2", 1)], [makeTask("t1", "s1")]),
      "wf-2": makeSnapshot([makeStep("s3", 0)], [makeTask("t2", "s3")]),
    };
    const result = aggregateSidebarTasks(snapshots);
    expect(result.allTasks).toHaveLength(2);
    expect(result.allTasks.find((t) => t.id === "t1")?._workflowId).toBe("wf-1");
    expect(result.allTasks.find((t) => t.id === "t2")?._workflowId).toBe("wf-2");
    expect(result.allSteps.map((s) => s.id)).toEqual(["s1", "s3", "s2"]);
    expect(result.stepsByWorkflowId["wf-1"].map((s) => s.id)).toEqual(["s1", "s2"]);
  });

  it("sorts steps within a workflow by position", () => {
    const snapshots: WorkflowSnapshotMap = {
      "wf-1": makeSnapshot([makeStep("s2", 1), makeStep("s1", 0), makeStep("s3", 2)], []),
    };
    const result = aggregateSidebarTasks(snapshots);
    expect(result.stepsByWorkflowId["wf-1"].map((s) => s.id)).toEqual(["s1", "s2", "s3"]);
  });

  it("deduplicates steps across workflows by id", () => {
    const sharedStep = makeStep("shared", 0);
    const snapshots: WorkflowSnapshotMap = {
      "wf-1": makeSnapshot([sharedStep], []),
      "wf-2": makeSnapshot([sharedStep], []),
    };
    const result = aggregateSidebarTasks(snapshots);
    expect(result.allSteps.filter((s) => s.id === "shared")).toHaveLength(1);
  });

  it("returns global allSteps sorted by position", () => {
    const snapshots: WorkflowSnapshotMap = {
      "wf-1": makeSnapshot([makeStep("a", 5), makeStep("b", 1)], []),
      "wf-2": makeSnapshot([makeStep("c", 3)], []),
    };
    const result = aggregateSidebarTasks(snapshots);
    expect(result.allSteps.map((s) => s.position)).toEqual([1, 3, 5]);
  });
});
