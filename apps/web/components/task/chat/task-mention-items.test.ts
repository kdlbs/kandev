import { describe, it, expect } from "vitest";
import { buildTaskMentionItems } from "./task-mention-items";
import type { WorkflowSnapshotData } from "@/lib/state/slices/kanban/types";

function snapshot(overrides: Partial<WorkflowSnapshotData> = {}): WorkflowSnapshotData {
  return {
    workflowId: "wf-1",
    workflowName: "Main flow",
    steps: [],
    tasks: [],
    ...overrides,
  };
}

describe("buildTaskMentionItems / basics", () => {
  it("returns tasks from the current workflow with workflow/step names resolved", () => {
    const snapshots = {
      "wf-1": snapshot({
        steps: [{ id: "step-1", title: "Todo", color: "", position: 0 }],
        tasks: [
          {
            id: "task-a",
            workflowStepId: "step-1",
            title: "Implement auth",
            position: 0,
            state: "IN_PROGRESS",
          },
        ],
      }),
    };

    const items = buildTaskMentionItems(snapshots, null, [
      { id: "wf-1", workspaceId: "ws-1", name: "Main flow" },
    ]);
    expect(items).toHaveLength(1);
    expect(items[0]).toMatchObject({
      kind: "task",
      label: "Implement auth",
      description: "Main flow · Todo",
      task: {
        taskId: "task-a",
        title: "Implement auth",
        workflowId: "wf-1",
        workflowStepId: "step-1",
        state: "IN_PROGRESS",
      },
    });
  });

  it("excludes the current task by id", () => {
    const snapshots = {
      "wf-1": snapshot({
        tasks: [
          { id: "task-a", workflowStepId: "step-1", title: "A", position: 0 },
          { id: "task-b", workflowStepId: "step-1", title: "B", position: 1 },
        ],
      }),
    };

    const items = buildTaskMentionItems(snapshots, "task-a");
    expect(items.map((i) => i.task?.taskId)).toEqual(["task-b"]);
  });
});

describe("buildTaskMentionItems / merging and filtering", () => {
  it("merges tasks from workflow snapshot Query caches and dedupes by id", () => {
    const snapshots = {
      "wf-1": snapshot({
        workflowName: "Main",
        steps: [],
        tasks: [
          { id: "task-a", workflowStepId: "step-1", title: "A", position: 0 },
          { id: "task-a", workflowStepId: "step-1", title: "A (dup)", position: 1 },
          { id: "task-c", workflowStepId: "step-2", title: "C", position: 2 },
        ],
      }),
      "wf-2": snapshot({
        workflowId: "wf-2",
        workflowName: "Other",
        steps: [{ id: "step-9", title: "Review", color: "", position: 0 }],
        tasks: [{ id: "task-d", workflowStepId: "step-9", title: "D", position: 0 }],
      }),
    };

    const ids = buildTaskMentionItems(snapshots, null).map((i) => i.task?.taskId);
    expect(ids).toEqual(["task-a", "task-c", "task-d"]);
  });

  it("skips stale snapshot tasks whose step is not in that workflow's steps", () => {
    const snapshots = {
      "wf-1": snapshot({
        steps: [{ id: "step-current", title: "Todo", color: "", position: 0 }],
        tasks: [
          { id: "task-fresh", workflowStepId: "step-current", title: "Fresh", position: 0 },
          // Left over from a previous workflow: its step is not in wf-1's steps.
          { id: "task-stale", workflowStepId: "step-other", title: "Stale", position: 1 },
        ],
      }),
    };

    const ids = buildTaskMentionItems(snapshots, null).map((i) => i.task?.taskId);
    expect(ids).toEqual(["task-fresh"]);
  });

  it("falls back to placeholder names when workflow/step are missing", () => {
    const snapshots = {
      "wf-1": snapshot({
        workflowName: "",
        steps: [],
        tasks: [{ id: "task-a", workflowStepId: "step-missing", title: "A", position: 0 }],
      }),
    };

    const [item] = buildTaskMentionItems(snapshots, null);
    expect(item.description).toBe("Workflow · Step");
  });
});
