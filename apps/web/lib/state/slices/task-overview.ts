import type { StateCreator } from "zustand";
import type { AppState } from "../app-state-types";
import { generateUUID } from "@/lib/utils";
import { reconcileTaskOverviewRead } from "./task-overview-merge";
import { emptyTaskOverviewState, type TaskOverviewSlice } from "./task-overview-types";

export const createTaskOverviewSlice: StateCreator<AppState, [], [], TaskOverviewSlice> = (
  set,
) => ({
  taskOverview: emptyTaskOverviewState(),
  denyTaskOverviewAccess: () =>
    set((state) => {
      if (state.workspaceContextRead.snapshotError === "access_denied") return state;
      return {
        taskOverview: emptyTaskOverviewState(
          state.taskOverview.scope,
          state.taskOverview.generation + 1,
        ),
        kanban: { ...state.kanban, tasks: [] },
        kanbanMulti: { ...state.kanbanMulti, snapshots: {} },
        sidebarArchivedTasks: { ...state.sidebarArchivedTasks, itemsByWorkspaceId: {} },
        workspaceContextRead: { ...state.workspaceContextRead, snapshotError: "access_denied" },
      };
    }),
  beginTaskOverviewRead: () => {
    const id = generateUUID();
    set((state) => ({
      taskOverview: {
        ...state.taskOverview,
        reads: {
          ...state.taskOverview.reads,
          [id]: {
            scope: state.taskOverview.scope,
            changes: {},
            bytes: 0,
          },
        },
      },
    }));
    return id;
  },
  finishTaskOverviewRead: (id) =>
    set((state) => {
      if (!state.taskOverview.reads[id]) return state;
      const reads = { ...state.taskOverview.reads };
      delete reads[id];
      return { taskOverview: { ...state.taskOverview, reads } };
    }),
  retainTaskOverviews: (owner, tasks, readId) => {
    let accepted = false;
    set((state) => {
      if (tasks.some((task) => task.workspaceId && task.workspaceId !== state.workspaces.activeId))
        return state;
      const reconciled = reconcileTaskOverviewRead(state.taskOverview, tasks, readId);
      if (!reconciled) return state;
      accepted = true;
      const byId = { ...state.taskOverview.byId };
      for (const task of reconciled) byId[task.id] = task;
      return {
        taskOverview: {
          ...state.taskOverview,
          byId,
          owners: { ...state.taskOverview.owners, [owner]: reconciled.map((task) => task.id) },
        },
      };
    });
    return accepted;
  },
  releaseTaskOverviews: (owner) =>
    set((state) => {
      if (!state.taskOverview.owners[owner]) return state;
      const owners = { ...state.taskOverview.owners };
      delete owners[owner];
      return { taskOverview: { ...state.taskOverview, owners } };
    }),
});
