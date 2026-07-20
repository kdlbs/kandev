import { describe, expect, it } from "vitest";
import {
  aggregateSidebarTasks,
  buildPendingFlags,
  readPendingFlags,
  type SidebarStepInfo,
  type WorkflowSnapshotMap,
} from "./task-session-sidebar-aggregate";
import type { KanbanState } from "@/lib/state/slices";
import { sessionId as toSessionId, taskId as toTaskId, type Message } from "@/lib/types/http";

function makePermissionRequest(id: string, status?: string): Message {
  return {
    id,
    session_id: toSessionId("s1"),
    task_id: toTaskId("t1"),
    author_type: "agent",
    content: "",
    type: "permission_request",
    metadata: status ? { status } : undefined,
    created_at: "",
  };
}

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

  it("uses tasks already present in the snapshot", () => {
    const snapshots: WorkflowSnapshotMap = {
      "wf-1": makeSnapshot([makeStep("s1", 0)], [makeTask("t1", "s1", { title: "from snapshot" })]),
    };
    const result = aggregateSidebarTasks(snapshots);
    expect(result.allTasks).toEqual([
      expect.objectContaining({ id: "t1", title: "from snapshot" }),
    ]);
  });

  it("uses snapshot steps for each workflow", () => {
    const snapshots: WorkflowSnapshotMap = {
      "wf-1": makeSnapshot([makeStep("s1", 0, { title: "Snapshot Step" })], []),
    };
    const result = aggregateSidebarTasks(snapshots);
    expect(result.stepsByWorkflowId["wf-1"][0].title).toBe("Snapshot Step");
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

describe("buildPendingFlags / readPendingFlags", () => {
  it("flags a session with a pending permission request", () => {
    const flags = buildPendingFlags({ "sess-1": [makePermissionRequest("p1")] }, ["sess-1"]);
    expect(readPendingFlags(flags, "sess-1")).toEqual({ clarification: false, permission: true });
  });

  it("does not flag a session whose permission request is already resolved", () => {
    const flags = buildPendingFlags({ "sess-1": [makePermissionRequest("p1", "approved")] }, [
      "sess-1",
    ]);
    expect(readPendingFlags(flags, "sess-1")).toEqual({ clarification: false, permission: false });
  });

  it("only computes flags for the requested session ids", () => {
    const flags = buildPendingFlags(
      { "sess-1": [makePermissionRequest("p1")], "sess-2": [makePermissionRequest("p2")] },
      ["sess-1"],
    );
    expect(readPendingFlags(flags, "sess-1").permission).toBe(true);
    expect(readPendingFlags(flags, "sess-2").permission).toBe(false);
  });

  it("returns all-false for a null/unknown session", () => {
    expect(readPendingFlags({}, null)).toEqual({ clarification: false, permission: false });
    expect(readPendingFlags({}, "missing")).toEqual({ clarification: false, permission: false });
  });

  it("falls back to the snapshot pending action when messages are not loaded", () => {
    expect(
      readPendingFlags({}, "sess-1", {
        primarySessionState: "WAITING_FOR_INPUT",
        primarySessionPendingAction: "clarification",
      }),
    ).toEqual({ clarification: true, permission: false });
  });

  it("falls back to pending permission from the snapshot", () => {
    expect(
      readPendingFlags({}, "sess-1", {
        primarySessionState: "WAITING_FOR_INPUT",
        primarySessionPendingAction: "permission",
      }),
    ).toEqual({ clarification: false, permission: true });
  });

  it("prefers loaded messages over a stale snapshot pending action", () => {
    const flags = buildPendingFlags({ "sess-1": [] }, ["sess-1"]);
    expect(
      readPendingFlags(flags, "sess-1", {
        primarySessionState: "WAITING_FOR_INPUT",
        primarySessionPendingAction: "clarification",
      }),
    ).toEqual({ clarification: false, permission: false });
  });

  it("does not use the snapshot pending action after the session moves on", () => {
    expect(
      readPendingFlags({}, "sess-1", {
        primarySessionState: "RUNNING",
        primarySessionPendingAction: "clarification",
      }),
    ).toEqual({ clarification: false, permission: false });
  });
});
