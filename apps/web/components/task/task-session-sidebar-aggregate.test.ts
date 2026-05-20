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
    workflowId: "wf-1",
    state: "TODO",
    repositoryIds: [],
    ...overrides,
  } as KanbanTask;
}

function makeSnapshot(steps: SidebarStepInfo[], tasks: KanbanTask[]) {
  return { steps, tasks };
}

describe("aggregateSidebarTasks", () => {
  it("returns empty result for empty snapshots and no active workflow", () => {
    const result = aggregateSidebarTasks({}, null, [], []);
    expect(result.allTasks).toEqual([]);
    expect(result.allSteps).toEqual([]);
    expect(result.stepsByWorkflowId).toEqual({});
  });

  it("collects tasks and steps from snapshots and tags each task with its workflow id", () => {
    const snapshots: WorkflowSnapshotMap = {
      "wf-1": makeSnapshot([makeStep("s1", 0), makeStep("s2", 1)], [makeTask("t1", "s1")]),
      "wf-2": makeSnapshot([makeStep("s3", 0)], [makeTask("t2", "s3")]),
    };
    const result = aggregateSidebarTasks(snapshots, null, [], []);
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
    const result = aggregateSidebarTasks(snapshots, null, [], []);
    expect(result.stepsByWorkflowId["wf-1"].map((s) => s.id)).toEqual(["s1", "s2", "s3"]);
  });

  it("does not apply active-kanban fallback when activeWorkflowId is null", () => {
    const result = aggregateSidebarTasks({}, null, [makeTask("t1", "s1")], [makeStep("s1", 0)]);
    expect(result.allTasks).toEqual([]);
    expect(result.allSteps).toEqual([]);
  });

  it("injects tasks from active kanban when the workflow has no snapshot", () => {
    const result = aggregateSidebarTasks({}, "wf-1", [makeTask("t1", "s1")], [makeStep("s1", 0)]);
    expect(result.allTasks).toHaveLength(1);
    expect(result.allTasks[0].id).toBe("t1");
    expect(result.allTasks[0]._workflowId).toBe("wf-1");
    expect(result.stepsByWorkflowId["wf-1"]).toHaveLength(1);
    expect(result.allSteps).toHaveLength(1);
  });

  it("does not duplicate tasks already present in the snapshot", () => {
    const snapshots: WorkflowSnapshotMap = {
      "wf-1": makeSnapshot([makeStep("s1", 0)], [makeTask("t1", "s1", { title: "from snapshot" })]),
    };
    const result = aggregateSidebarTasks(
      snapshots,
      "wf-1",
      [makeTask("t1", "s1", { title: "from active" }), makeTask("t2", "s1")],
      [makeStep("s1", 0)],
    );
    expect(result.allTasks).toHaveLength(2);
    const t1 = result.allTasks.find((t) => t.id === "t1");
    expect(t1?.title).toBe("from snapshot");
    expect(result.allTasks.find((t) => t.id === "t2")).toBeDefined();
  });

  it("does not overwrite snapshot steps when the workflow already has a snapshot", () => {
    const snapshots: WorkflowSnapshotMap = {
      "wf-1": makeSnapshot([makeStep("s1", 0, { title: "Snapshot Step" })], []),
    };
    const result = aggregateSidebarTasks(
      snapshots,
      "wf-1",
      [],
      [makeStep("s1", 0, { title: "Active Step" })],
    );
    expect(result.stepsByWorkflowId["wf-1"][0].title).toBe("Snapshot Step");
  });

  it("deduplicates steps across workflows by id", () => {
    const sharedStep = makeStep("shared", 0);
    const snapshots: WorkflowSnapshotMap = {
      "wf-1": makeSnapshot([sharedStep], []),
      "wf-2": makeSnapshot([sharedStep], []),
    };
    const result = aggregateSidebarTasks(snapshots, null, [], []);
    expect(result.allSteps.filter((s) => s.id === "shared")).toHaveLength(1);
  });

  it("returns global allSteps sorted by position", () => {
    const snapshots: WorkflowSnapshotMap = {
      "wf-1": makeSnapshot([makeStep("a", 5), makeStep("b", 1)], []),
      "wf-2": makeSnapshot([makeStep("c", 3)], []),
    };
    const result = aggregateSidebarTasks(snapshots, null, [], []);
    expect(result.allSteps.map((s) => s.position)).toEqual([1, 3, 5]);
  });

  it("drops active-kanban tasks whose workflowStepId is not in the active steps", () => {
    // Repro for workspace-switch-sidebar-isolation: SSR hydration on the
    // session page refreshes kanban.steps to workspace B's steps, but
    // hydrator's mergeKanbanTasks accumulates kanban.tasks across workflow
    // switches so workspace A's tasks (referencing A's stepIds) linger.
    // Before the fix, the fallback re-tagged them with workspace B's
    // workflow id and they leaked into the sidebar.
    const result = aggregateSidebarTasks(
      {},
      "wf-B",
      [makeTask("task-A", "step-from-A"), makeTask("task-B", "step-B")],
      [makeStep("step-B", 0)],
    );
    expect(result.allTasks.map((t) => t.id)).toEqual(["task-B"]);
  });
});
