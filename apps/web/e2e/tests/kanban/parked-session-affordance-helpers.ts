import type { Page } from "@playwright/test";

type E2EStoreState = {
  kanban: { tasks: Array<Record<string, unknown>> };
  kanbanMulti: { snapshots: Record<string, { tasks: Array<Record<string, unknown>> }> };
};

type E2EStoreWindow = Window & {
  __KANDEV_E2E_STORE__?: {
    getState: () => E2EStoreState;
    setState: (updater: (state: E2EStoreState) => Partial<E2EStoreState> | void) => void;
  };
};

/**
 * Immutably updates the task in both board slices. The board reads the
 * workflow snapshot, while the active-workflow slice stays in sync for
 * selectors that still use the legacy path.
 */
export async function injectParkedBoardTask(page: Page, workflowId: string, taskId: string) {
  await page.evaluate(
    ({ workflowId, taskId }) => {
      const store = (window as E2EStoreWindow).__KANDEV_E2E_STORE__;
      if (!store) throw new Error("E2E store bridge missing");
      const parkTask = (task: Record<string, unknown>) =>
        task.id === taskId
          ? { ...task, state: "WAITING_FOR_INPUT", parkedOnBackgroundWork: true }
          : task;
      store.setState((state) => {
        const snapshot = state.kanbanMulti.snapshots[workflowId];
        if (!snapshot) throw new Error(`No kanbanMulti snapshot for workflow ${workflowId}`);
        if (!snapshot.tasks.some((task) => task.id === taskId)) {
          throw new Error(`Task ${taskId} not found in kanbanMulti snapshot`);
        }
        return {
          kanban: { ...state.kanban, tasks: state.kanban.tasks.map(parkTask) },
          kanbanMulti: {
            ...state.kanbanMulti,
            snapshots: {
              ...state.kanbanMulti.snapshots,
              [workflowId]: { ...snapshot, tasks: snapshot.tasks.map(parkTask) },
            },
          },
        };
      });
    },
    { workflowId, taskId },
  );
}

/**
 * Marks the task as active background work without setting the parked
 * projection. This is the AC-59 regression shape.
 */
export async function injectBackgroundActivityBoardTask(
  page: Page,
  workflowId: string,
  taskId: string,
) {
  await page.evaluate(
    ({ workflowId, taskId }) => {
      const store = (window as E2EStoreWindow).__KANDEV_E2E_STORE__;
      if (!store) throw new Error("E2E store bridge missing");
      const markBackground = (task: Record<string, unknown>) =>
        task.id === taskId ? { ...task, foregroundActivity: "background" } : task;
      store.setState((state) => {
        const snapshot = state.kanbanMulti.snapshots[workflowId];
        if (!snapshot) throw new Error(`No kanbanMulti snapshot for workflow ${workflowId}`);
        if (!snapshot.tasks.some((task) => task.id === taskId)) {
          throw new Error(`Task ${taskId} not found in kanbanMulti snapshot`);
        }
        return {
          kanban: { ...state.kanban, tasks: state.kanban.tasks.map(markBackground) },
          kanbanMulti: {
            ...state.kanbanMulti,
            snapshots: {
              ...state.kanbanMulti.snapshots,
              [workflowId]: { ...snapshot, tasks: snapshot.tasks.map(markBackground) },
            },
          },
        };
      });
    },
    { workflowId, taskId },
  );
}
