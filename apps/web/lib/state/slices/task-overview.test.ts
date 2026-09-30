import { expect, it } from "vitest";
import { createAppStore } from "../store";
import type { TaskOverview } from "./task-overview-types";
import { recordTaskOverviewChange } from "./task-overview-merge";
import { registerTasksHandlers } from "@/lib/ws/handlers/tasks";
import { defaultKanbanState } from "./kanban/kanban-slice";

const DISPLAY_OWNER = "sidebar:display";

const task = (id: string, fields: Partial<TaskOverview> = {}): TaskOverview => ({
  id,
  workspaceId: "workspace",
  workflowId: "workflow",
  workflowStepId: "step",
  title: id,
  position: 0,
  updatedAt: "2026-09-29T12:00:00Z",
  ...fields,
});

function sharedStore() {
  return createAppStore({
    workspaces: { items: [], activeId: "workspace" },
    kanban: { workflowId: "workflow", steps: [], tasks: [task("one")] },
    kanbanMulti: {
      ...defaultKanbanState.kanbanMulti,
      snapshots: {
        workflow: {
          workflowId: "workflow",
          workflowName: "Workflow",
          steps: [],
          tasks: [task("one")],
        },
      },
    },
  });
}

it("inherits missing task workflow identity from its containing board and snapshot", () => {
  const store = createAppStore({
    kanban: { workflowId: "workflow", steps: [], tasks: [task("one", { workflowId: undefined })] },
    kanbanMulti: {
      ...defaultKanbanState.kanbanMulti,
      snapshots: {
        workflow: {
          workflowId: "workflow",
          workflowName: "Workflow",
          steps: [],
          tasks: [task("one", { workflowId: undefined })],
        },
      },
    },
  });
  const canonical = store.getState().taskOverview.byId.one;
  expect(canonical.workflowId).toBe("workflow");
  expect(store.getState().kanban.tasks).toEqual([canonical]);
  expect(store.getState().kanbanMulti.snapshots.workflow.tasks[0]).toBe(canonical);
});

it("publishes one accepted object to board, workflow and sidebar owners atomically", () => {
  const store = sharedStore();
  store.getState().retainTaskOverviews(DISPLAY_OWNER, [task("one")]);
  const initial = store.getState().taskOverview.byId.one;
  expect(store.getState().kanban.tasks[0]).toBe(initial);
  expect(store.getState().kanbanMulti.snapshots.workflow.tasks[0]).toBe(initial);
  expect(store.getState().kanbanMulti.snapshots.workflow.taskIds).toEqual(["one"]);
  const unsubscribe = store.subscribe((state) => {
    expect(state.kanban.tasks[0]).toBe(state.taskOverview.byId.one);
    expect(state.kanbanMulti.snapshots.workflow.tasks[0]).toBe(state.taskOverview.byId.one);
  });
  store.getState().updateMultiTask("workflow", task("one", { title: "Updated" }));
  expect(store.getState().taskOverview.byId.one.title).toBe("Updated");
  store.getState().retainTaskOverviews(DISPLAY_OWNER, [task("one", { title: "Updated" })]);
  const unchanged = store.getState().taskOverview.byId.one;
  store.getState().retainTaskOverviews(DISPLAY_OWNER, [task("one", { title: "Updated" })]);
  expect(store.getState().taskOverview.byId.one).toBe(unchanged);
  unsubscribe();
});

it("merges partial fields, explicit clears and task/summary freshness independently", () => {
  const store = sharedStore();
  store.setState((state) => ({
    taskOverview: recordTaskOverviewChange(state.taskOverview, "one", {
      id: "one",
      primarySessionId: "session",
      title: "Live",
      updatedAt: "2026-09-29T14:00:00Z",
    }),
  }));
  store.setState((state) => ({
    taskOverview: recordTaskOverviewChange(state.taskOverview, "one", {
      id: "one",
      primarySessionId: null,
    }),
  }));
  expect(store.getState().taskOverview.byId.one.title).toBe("Live");
  expect(store.getState().taskOverview.byId.one.primarySessionId).toBeNull();
  store.getState().retainTaskOverviews(DISPLAY_OWNER, [
    task("one", {
      title: "Old",
      statusSummary: { revision: 8, updated_at: "2026-09-29T15:00:00Z" },
    }),
  ]);
  const accepted = store.getState().taskOverview.byId.one;
  expect(accepted.title).toBe("Live");
  expect(accepted.statusSummary?.revision).toBe(8);
  expect(accepted.updatedAt).toBe("2026-09-29T14:00:00Z");
});

it("releases replaced pages while preserving board and active-detail ownership", () => {
  const store = sharedStore();
  store
    .getState()
    .retainTaskOverviews(DISPLAY_OWNER, [task("archive", { isArchived: true }), task("one")]);
  store.getState().setActiveTask("archive");
  for (let page = 0; page < 30; page++) {
    store
      .getState()
      .retainTaskOverviews(DISPLAY_OWNER, [task(`page-${page}`, { isArchived: true })]);
    expect(Object.keys(store.getState().taskOverview.byId)).toHaveLength(3);
  }
  store.getState().setActiveTask("one");
  expect(store.getState().taskOverview.byId.archive).toBeUndefined();
  store.getState().releaseTaskOverviews(DISPLAY_OWNER);
  expect(Object.keys(store.getState().taskOverview.byId)).toEqual(["one"]);
});

it("reconciles real task updates across an older read without restoring deleted records", () => {
  const store = sharedStore();
  const handlers = registerTasksHandlers(store);
  const read = store.getState().beginTaskOverviewRead();
  handlers["task.updated"]?.({
    type: "notification",
    action: "task.updated",
    payload: {
      task_id: "one",
      workspace_id: "workspace",
      workflow_id: "workflow",
      workflow_step_id: "step",
      title: "Live",
      updated_at: "2026-09-29T15:00:00Z",
    },
  } as Parameters<NonNullable<(typeof handlers)["task.updated"]>>[0]);
  store.setState((state) => ({
    taskOverview: recordTaskOverviewChange(state.taskOverview, "deleted", null),
  }));
  expect(
    store.getState().retainTaskOverviews(DISPLAY_OWNER, [task("one"), task("deleted")], read),
  ).toBe(true);
  expect(store.getState().taskOverview.byId.one.title).toBe("Live");
  expect(store.getState().taskOverview.owners[DISPLAY_OWNER]).toEqual(["one"]);
  store.getState().finishTaskOverviewRead(read);
  expect(store.getState().taskOverview.reads).toEqual({});
});

it("rejects expired read protection after overflow or a context change", () => {
  const store = sharedStore();
  const read = store.getState().beginTaskOverviewRead();
  for (let index = 0; index <= 1000; index++) {
    store.setState((state) => ({
      taskOverview: recordTaskOverviewChange(state.taskOverview, `deleted-${index}`, null),
    }));
  }
  expect(store.getState().taskOverview.reads[read]).toBeUndefined();
  expect(store.getState().retainTaskOverviews(DISPLAY_OWNER, [task("deleted-0")], read)).toBe(
    false,
  );
  const anotherRead = store.getState().beginTaskOverviewRead();
  store.getState().resetKanbanWorkspaceContext();
  expect(store.getState().retainTaskOverviews(DISPLAY_OWNER, [task("one")], anotherRead)).toBe(
    false,
  );
  expect(store.getState().taskOverview.byId).toEqual({});
});
