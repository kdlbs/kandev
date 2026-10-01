import { expect, it } from "vitest";
import { createAppStore } from "../store";
import { toKanbanTask } from "@/lib/kanban/map-task";
import { registerTasksHandlers } from "@/lib/ws/handlers/tasks";
import { reconcileTaskOverviewRead } from "./task-overview-merge";

const TASK_CREATED = "task.created";
const TASK_UPDATED = "task.updated";

function fixture() {
  const store = createAppStore();
  const task = toKanbanTask({
    id: "task",
    workspace_id: "ws",
    workflow_id: "a",
    workflow_step_id: "step-a",
    title: "Original",
    created_at: "2026-09-29T00:00:00Z",
    updated_at: "2026-09-29T00:00:00Z",
  });
  store.setState((state) => {
    state.workspaces.activeId = "ws";
    state.kanban = { workflowId: "a", steps: [], tasks: [task] };
    for (const id of ["a", "b"])
      state.kanbanMulti.snapshots[id] = {
        workflowId: id,
        workflowName: id,
        steps: [{ id: `step-${id}`, title: id, color: "", position: 0 }],
        tasks: id === "a" ? [task] : [],
        taskCoverage: {
          workspace_id: "ws",
          workflow_id: id,
          membership: "active",
          complete: true,
          total: id === "a" ? 1 : 0,
          ordering_profile: "sqlite_nocase_v1",
        },
      };
    state.workflows.taskWorkflowCoverage = {
      workspace_id: "ws",
      workflow_ids: ["a"],
      complete: true,
    };
  });
  const handlers = registerTasksHandlers(store);
  const event = (
    action: typeof TASK_CREATED | typeof TASK_UPDATED,
    payload: Record<string, unknown>,
  ) => {
    if (action === TASK_CREATED) {
      handlers[action]?.({ type: "notification", action, payload } as Parameters<
        NonNullable<(typeof handlers)[typeof TASK_CREATED]>
      >[0]);
    } else {
      handlers[action]?.({ type: "notification", action, payload } as Parameters<
        NonNullable<(typeof handlers)[typeof TASK_UPDATED]>
      >[0]);
    }
  };
  return { store, task, event };
}

it("keeps atomic placement and step metadata through moves and optimistic rollback", () => {
  const { store, task } = fixture();
  const before = store.getState().kanbanMulti.snapshots.a;
  store.getState().setWorkflowSnapshot("a", {
    ...before,
    tasks: [{ ...task, parentTaskId: "parent", position: 7 }],
  });
  expect(store.getState().kanban.tasks[0]).toBe(store.getState().taskOverview.byId.task);
  store.getState().setWorkflowSnapshot("a", before);
  expect(store.getState().taskOverview.byId.task.parentTaskId).toBeUndefined();
  expect(store.getState().taskOverview.byId.task.position).toBe(0);
  const unsub = store.subscribe((state) => {
    expect(state.kanbanMulti.snapshots.a.tasks).toEqual([]);
    expect(state.kanbanMulti.snapshots.b.tasks[0]).toBe(state.taskOverview.byId.task);
    expect(state.taskOverview.byId.task.workflowStepId).toBe(
      state.kanbanMulti.snapshots.b.steps[0].id,
    );
  });
  store.setState((state) => {
    state.kanbanMulti.snapshots.a.tasks = [];
    state.kanbanMulti.snapshots.b.tasks = [{ ...task, workflowId: "b", workflowStepId: "step-b" }];
  });
  expect(store.getState().kanban.tasks).toEqual([]);
  expect(store.getState().kanbanMulti.snapshots.a.taskCoverage?.total).toBe(0);
  expect(store.getState().kanbanMulti.snapshots.b.taskCoverage?.total).toBe(1);
  unsub();
});

it("keeps live create, archive and unarchive membership across older workflow reads", () => {
  const { store, task, event } = fixture();
  const read = store.getState().beginTaskOverviewRead();
  event(TASK_CREATED, {
    task_id: "created",
    workspace_id: "ws",
    workflow_id: "a",
    workflow_step_id: "step-a",
    title: "Created",
    state: "TODO",
  });
  event(TASK_UPDATED, {
    task_id: "task",
    workspace_id: "ws",
    workflow_id: "a",
    workflow_step_id: "step-a",
    archived_at: "2026-09-29T01:00:00Z",
  });
  const tasks = reconcileTaskOverviewRead(store.getState().taskOverview, [task], read, true)!;
  expect(tasks.filter((item) => !item.isArchived).map((item) => item.id)).toEqual(["created"]);
  event(TASK_UPDATED, {
    task_id: "task",
    workspace_id: "ws",
    workflow_id: "a",
    workflow_step_id: "step-a",
    archived_at: null,
  });
  expect(store.getState().kanbanMulti.snapshots.a.tasks.map((item) => item.id)).toContain("task");
  expect(store.getState().taskOverview.byId.task.isArchived).toBe(false);
  store.getState().finishTaskOverviewRead(read);
});

it("preserves nanosecond task freshness independently of newer summaries", () => {
  const { store, task } = fixture();
  store
    .getState()
    .retainTaskOverviews("sidebar:live", [
      { ...task, title: "Latest", updatedAt: "2026-09-29T00:00:00.123456789Z" },
    ]);
  store
    .getState()
    .retainTaskOverviews("sidebar:late", [
      { ...task, title: "Stale", updatedAt: "2026-09-29T00:00:00.123456788Z" },
    ]);
  expect(store.getState().taskOverview.byId.task.title).toBe("Latest");
});

it("invalidates workspace-wide coverage for unknown or unassigned live workflow membership", () => {
  for (const workflow of ["hidden", ""]) {
    const { store, event } = fixture();
    event(TASK_CREATED, {
      task_id: "new",
      workspace_id: "ws",
      workflow_id: workflow,
      title: "New",
    });
    expect(store.getState().workflows.taskWorkflowCoverage?.complete).toBe(false);
  }
});

it("extends active scope coverage when live work enters a completely loaded empty workflow", () => {
  const { store, event } = fixture();
  event(TASK_CREATED, {
    task_id: "new",
    workspace_id: "ws",
    workflow_id: "b",
    workflow_step_id: "step-b",
    title: "New",
    created_at: "2026-09-29T01:00:00Z",
    updated_at: "2026-09-29T01:00:00Z",
  });
  expect(store.getState().workflows.taskWorkflowCoverage).toMatchObject({
    complete: true,
    workflow_ids: ["a", "b"],
  });
  expect(store.getState().kanbanMulti.snapshots.b.tasks.map((task) => task.id)).toEqual(["new"]);
  expect(store.getState().kanbanMulti.snapshots.b.taskCoverage).toMatchObject({
    complete: true,
    total: 1,
  });
});

it("clears canonical entities and rejects pending reads on access denial", () => {
  const { store, task } = fixture();
  const read = store.getState().beginTaskOverviewRead();
  store.getState().denyTaskOverviewAccess();
  expect(store.getState().taskOverview.byId).toEqual({});
  expect(store.getState().retainTaskOverviews("sidebar:late", [task], read)).toBe(false);
});
