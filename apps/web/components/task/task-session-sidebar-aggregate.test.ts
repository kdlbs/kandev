import { describe, expect, it } from "vitest";
import {
  aggregateSidebarTasks,
  buildPendingFlags,
  readTaskPendingFlags,
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
const ACTIVE_TASK_UPDATED_AT = "2026-08-29T09:14:02Z";

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

  it("prefers live active steps when the workflow snapshot is stale", () => {
    const snapshots: WorkflowSnapshotMap = {
      "wf-1": makeSnapshot([makeStep("s1", 0, { title: "Snapshot Step" })], []),
    };
    const result = aggregateSidebarTasks(
      snapshots,
      "wf-1",
      [],
      [makeStep("s1", 2, { title: "Active Step" })],
    );
    expect(result.stepsByWorkflowId["wf-1"][0].title).toBe("Active Step");
    expect(result.allSteps.find((step) => step.id === "s1")).toMatchObject({
      title: "Active Step",
      position: 2,
    });
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

describe("aggregateSidebarTasks active precedence", () => {
  it("prefers the live active task over a stale workflow snapshot", () => {
    const snapshots: WorkflowSnapshotMap = {
      "wf-1": makeSnapshot(
        [makeStep("s1", 0)],
        [
          makeTask("t1", "s1", {
            title: "from snapshot",
            state: "IN_PROGRESS",
            primarySessionState: "STARTING",
            statusSummary: {
              revision: 2,
              updated_at: "2026-08-03T00:00:00Z",
            },
          }),
        ],
      ),
    };
    const result = aggregateSidebarTasks(
      snapshots,
      "wf-1",
      [
        makeTask("t1", "s1", {
          title: "from active",
          state: "REVIEW",
          primarySessionState: "WAITING_FOR_INPUT",
          statusSummary: {
            revision: 4,
            updated_at: "2026-08-03T00:00:02Z",
          },
        }),
        makeTask("t2", "s1"),
      ],
      [makeStep("s1", 0)],
    );
    expect(result.allTasks).toHaveLength(2);
    const t1 = result.allTasks.find((t) => t.id === "t1");
    expect(t1).toMatchObject({
      title: "from active",
      state: "REVIEW",
      primarySessionState: "WAITING_FOR_INPUT",
      statusSummary: { revision: 4 },
    });
    expect(result.allTasks.find((t) => t.id === "t2")).toBeDefined();
  });

  it("keeps a newer projected summary when the active task has no summary", () => {
    const projected = makeTask("t1", "s1", {
      statusSummary: {
        revision: 4,
        updated_at: "2026-08-03T00:00:02Z",
        git: { additions: 3, deletions: 0 },
      },
    });
    const result = aggregateSidebarTasks(
      { "wf-1": makeSnapshot([makeStep("s1", 0)], [projected]) },
      "wf-1",
      [makeTask("t1", "s1", { statusSummary: undefined })],
      [makeStep("s1", 0)],
    );

    expect(result.allTasks).toHaveLength(1);
    expect(result.allTasks[0].statusSummary).toMatchObject({
      revision: 4,
      git: { additions: 3, deletions: 0 },
    });
  });

  it("preserves autopilot from the projection when the active task is partial", () => {
    const projected = makeTask("t1", "s1", {
      autopilot: true,
      statusSummary: { revision: 1, updated_at: "2026-08-03T00:00:00Z" },
    });
    const result = aggregateSidebarTasks(
      { "wf-1": makeSnapshot([makeStep("s1", 0)], [projected]) },
      "wf-1",
      [
        makeTask("t1", "s1", {
          statusSummary: { revision: 2, updated_at: "2026-08-03T00:00:01Z" },
        }),
      ],
      [makeStep("s1", 0)],
    );

    expect(result.allTasks[0].autopilot).toBe(true);
  });
});

describe("aggregateSidebarTasks task lifecycle freshness", () => {
  it("uses newer task state when status-summary revisions are equal", () => {
    // @covers AC-TASKS-RUNTIME-STATE-PUBLICATION-ORDER-001.4
    const projected = makeTask("t1", "s1", {
      state: "REVIEW",
      updatedAt: "2026-08-29T09:14:01Z",
      statusSummary: {
        revision: 4,
        updated_at: "2026-08-29T09:14:01Z",
        queued_prompt_count: 5,
      },
    });
    const active = makeTask("t1", "s1", {
      state: "IN_PROGRESS",
      updatedAt: ACTIVE_TASK_UPDATED_AT,
      statusSummary: {
        revision: 4,
        updated_at: ACTIVE_TASK_UPDATED_AT,
        queued_prompt_count: 0,
      },
    });

    const result = aggregateSidebarTasks(
      { "wf-1": makeSnapshot([makeStep("s1", 0)], [projected]) },
      "wf-1",
      [active],
      [makeStep("s1", 0)],
    );

    expect(result.allTasks[0].state).toBe("IN_PROGRESS");
    expect(result.allTasks[0].statusSummary?.queued_prompt_count).toBe(5);
  });

  it("prefers the active task when task timestamps are equal", () => {
    const projected = makeTask("t1", "s1", {
      state: "REVIEW",
      updatedAt: ACTIVE_TASK_UPDATED_AT,
    });
    const active = makeTask("t1", "s1", {
      state: "IN_PROGRESS",
      updatedAt: ACTIVE_TASK_UPDATED_AT,
    });

    const result = aggregateSidebarTasks(
      { "wf-1": makeSnapshot([makeStep("s1", 0)], [projected]) },
      "wf-1",
      [active],
      [makeStep("s1", 0)],
    );

    expect(result.allTasks[0].state).toBe("IN_PROGRESS");
  });

  it("keeps task freshness independent from status-summary freshness", () => {
    const projected = makeTask("t1", "s1", {
      state: "REVIEW",
      updatedAt: "2026-08-29T09:14:01Z",
      statusSummary: { revision: 5, updated_at: "2026-08-29T09:14:03Z" },
    });
    const active = makeTask("t1", "s1", {
      state: "IN_PROGRESS",
      updatedAt: ACTIVE_TASK_UPDATED_AT,
      statusSummary: { revision: 4, updated_at: ACTIVE_TASK_UPDATED_AT },
    });

    const result = aggregateSidebarTasks(
      { "wf-1": makeSnapshot([makeStep("s1", 0)], [projected]) },
      "wf-1",
      [active],
      [makeStep("s1", 0)],
    );

    expect(result.allTasks[0]).toMatchObject({
      state: "IN_PROGRESS",
      statusSummary: { revision: 5 },
    });
  });

  it("keeps a newer workflow snapshot task over an older active task", () => {
    const projected = makeTask("t1", "s1", {
      state: "REVIEW",
      updatedAt: "2026-08-29T09:14:03Z",
    });
    const active = makeTask("t1", "s1", {
      state: "IN_PROGRESS",
      updatedAt: ACTIVE_TASK_UPDATED_AT,
    });

    const result = aggregateSidebarTasks(
      { "wf-1": makeSnapshot([makeStep("s1", 0)], [projected]) },
      "wf-1",
      [active],
      [makeStep("s1", 0)],
    );

    expect(result.allTasks[0]).toMatchObject({
      state: "REVIEW",
      updatedAt: "2026-08-29T09:14:03Z",
    });
  });
});

describe("aggregateSidebarTasks interruption projection", () => {
  it("preserves projected interruption when an equal-timestamp active task omits it", () => {
    const projected = makeTask("t1", "s1", {
      state: "REVIEW",
      interrupted: true,
      updatedAt: ACTIVE_TASK_UPDATED_AT,
    });
    const active = makeTask("t1", "s1", {
      state: "REVIEW",
      updatedAt: ACTIVE_TASK_UPDATED_AT,
    });

    const result = aggregateSidebarTasks(
      { "wf-1": makeSnapshot([makeStep("s1", 0)], [projected]) },
      "wf-1",
      [active],
      [makeStep("s1", 0)],
    );

    expect(result.allTasks[0].interrupted).toBe(true);
  });
});

describe("buildPendingFlags / readTaskPendingFlags", () => {
  it("aggregates permission from a secondary waiting session", () => {
    const flags = buildPendingFlags(
      { primary: [], secondary: [makePermissionRequest("secondary-permission")] },
      ["primary", "secondary"],
    );
    expect(
      readTaskPendingFlags(flags, [
        { id: "primary", state: "STARTING" },
        { id: "secondary", state: "WAITING_FOR_INPUT" },
      ]),
    ).toEqual({ clarification: false, permission: true });
  });

  it("excludes stale pending input from non-input-capable sessions", () => {
    const flags = buildPendingFlags({ starting: [makePermissionRequest("stale-permission")] }, [
      "starting",
    ]);
    expect(readTaskPendingFlags(flags, [{ id: "starting", state: "STARTING" }])).toEqual({
      clarification: false,
      permission: false,
    });
  });
});
