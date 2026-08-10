import { test, expect } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";

/**
 * Parked-session affordance — board card only (AC-58, AC-59;
 * docs/specs/parked-board-mvp/spec.md D-6: the board is the single surface
 * for this slice — sidebar / `/tasks` are V4).
 *
 * A session that settled to WAITING_FOR_INPUT while a background shell
 * workload is still live is "parked". The board card must render
 * `data-testid="task-state-background-running"` rather than the plain
 * WAITING_FOR_INPUT question-mark icon.
 *
 * Backend plumbing is not exercised here — deterministically driving a real
 * detached background process through the launch recogniser and liveness
 * probe is out of scope for this fixture. Instead the board is fed the
 * `parkedOnBackgroundWork` projection at the point it actually reads it —
 * `state.kanbanMulti.snapshots[workflowId].tasks` (components/kanban-board.tsx)
 * — via the `__KANDEV_E2E_STORE__` bridge.
 */
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
 * Immutably updates the task in state.kanbanMulti.snapshots[workflowId].tasks
 * (what the board card reads) AND its counterpart in state.kanban.tasks (the
 * "active workflow" slice). Builds fresh array and object references at
 * every level touched (rather than mutating items in place) so every Zustand
 * selector — however granular — observes the change.
 *
 * No parkedRevision sentinel (D-2): V1 ships no epoch/revision, so there is
 * no discard rule to defeat and nothing for a background refetch to race.
 */
async function injectParkedBoardTask(
  page: import("@playwright/test").Page,
  workflowId: string,
  taskId: string,
) {
  await page.evaluate(
    ({ workflowId, taskId }) => {
      const store = (window as E2EStoreWindow).__KANDEV_E2E_STORE__;
      if (!store) throw new Error("E2E store bridge missing");
      const parkTask = (t: Record<string, unknown>) =>
        t.id === taskId ? { ...t, state: "WAITING_FOR_INPUT", parkedOnBackgroundWork: true } : t;
      store.setState((state) => {
        const snapshot = state.kanbanMulti.snapshots[workflowId];
        if (!snapshot) throw new Error(`No kanbanMulti snapshot for workflow ${workflowId}`);
        if (!snapshot.tasks.some((t) => t.id === taskId)) {
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
 * Immutably marks the task as actively background-running (NOT parked) —
 * `foregroundActivity: "background"`, `parkedOnBackgroundWork` left unset.
 * Regression coverage for AC-59: this is the pre-existing signal that must
 * render byte-identically to before the parked feature shipped — a bare
 * IconLoader with no `data-testid` — and must NOT show
 * `task-state-background-running`, which is reserved for the distinct
 * parked condition. Same store-mutation shape as injectParkedBoardTask,
 * minus the parked fields.
 */
async function injectBackgroundActivityBoardTask(
  page: import("@playwright/test").Page,
  workflowId: string,
  taskId: string,
) {
  await page.evaluate(
    ({ workflowId, taskId }) => {
      const store = (window as E2EStoreWindow).__KANDEV_E2E_STORE__;
      if (!store) throw new Error("E2E store bridge missing");
      const markBackground = (t: Record<string, unknown>) =>
        t.id === taskId ? { ...t, foregroundActivity: "background" } : t;
      store.setState((state) => {
        const snapshot = state.kanbanMulti.snapshots[workflowId];
        if (!snapshot) throw new Error(`No kanbanMulti snapshot for workflow ${workflowId}`);
        if (!snapshot.tasks.some((t) => t.id === taskId)) {
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

test.describe("Parked-session affordance — board card", () => {
  test("board card shows background-running icon when task is parked (AC-58)", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const task = await apiClient.createTask(seedData.workspaceId, "Parked Board Card Test", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });

    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    const card = kanban.taskCard(task.id);
    await expect(card).toBeVisible({ timeout: 10_000 });

    await injectParkedBoardTask(testPage, seedData.workflowId, task.id);

    // The board card must show the violet background-running spinner (AC-58),
    // not the question-mark icon.
    await expect(card.getByTestId("task-state-background-running")).toBeVisible({ timeout: 5_000 });
    await expect(card.getByTestId("task-state-waiting-for-input")).not.toBeVisible();
  });

  // AC-59 regression guard: before the fix, getTaskStateIconConfig could
  // merge the pre-existing foregroundActivity === "background" branch and
  // the new parked branch into the same sentinel, so BOTH would render
  // task-state-background-running in the live DOM — silently breaking the
  // spec's "byte-identical to before this feature" requirement for a task
  // that is actively running background work but is NOT parked. A unit test
  // alone cannot catch this because both branches produce visually similar
  // output; this asserts the real, distinct rendering on a live board.
  test("board card keeps the pre-existing plain spinner for background activity that is NOT parked (AC-59)", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const task = await apiClient.createTask(seedData.workspaceId, "Background Not Parked Test", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });

    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    const card = kanban.taskCard(task.id);
    await expect(card).toBeVisible({ timeout: 10_000 });

    await injectBackgroundActivityBoardTask(testPage, seedData.workflowId, task.id);

    // Must render SOME icon (the pre-existing bare IconLoader — no
    // data-testid, identified by its tabler icon class, same selector the
    // frontend unit tests use)...
    await expect(card.locator(".tabler-icon-loader")).toBeVisible({ timeout: 5_000 });
    // ...and must NOT render the parked-specific affordance, which is
    // reserved for the distinct parked condition.
    await expect(card.getByTestId("task-state-background-running")).not.toBeVisible();
  });
});
