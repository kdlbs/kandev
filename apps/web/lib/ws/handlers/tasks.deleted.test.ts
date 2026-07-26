import { beforeEach, describe, expect, it, vi } from "vitest";
import type { AppState } from "@/lib/state/store";
import { removeRecentTask } from "@/lib/recent-tasks";
import { registerTasksHandlers } from "./tasks";
import {
  makeActiveStore,
  makeDeletedMessage,
  makeInactiveStore,
  makeStore,
  REVIEW_TITLE,
  SESS_OTHER,
  SESS_PINNED,
} from "./tasks.test-helpers";

vi.mock("@/lib/recent-tasks", () => ({ removeRecentTask: vi.fn() }));

describe("task.deleted cleanup", () => {
  beforeEach(() => {
    vi.mocked(removeRecentTask).mockClear();
  });

  it("removes the deleted task from recent task history", () => {
    const store = makeStore({
      kanban: {
        workflowId: "wf1",
        steps: [],
        tasks: [{ id: "t1", primarySessionId: "sess-old", workflowId: "wf1" }],
      } as unknown as AppState["kanban"],
      environmentIdBySessionId: {},
    });

    registerTasksHandlers(store)["task.deleted"]!(
      makeDeletedMessage({ task_id: "t1", workflow_id: "wf1" }),
    );

    expect(removeRecentTask).toHaveBeenCalledOnce();
    expect(removeRecentTask).toHaveBeenCalledWith("t1");
  });

  it("clears deleted task session state", () => {
    const store = makeStore({
      kanban: {
        workflowId: "wf1",
        steps: [],
        tasks: [{ id: "t1", primarySessionId: SESS_PINNED, workflowId: "wf1" }],
      } as unknown as AppState["kanban"],
      tasks: {
        activeTaskId: "t1",
        activeSessionId: SESS_PINNED,
        pinnedSessionId: SESS_PINNED,
        lastSessionByTaskId: { t1: SESS_PINNED, t2: SESS_OTHER },
      },
      environmentIdBySessionId: {},
    });

    registerTasksHandlers(store)["task.deleted"]!(
      makeDeletedMessage({ task_id: "t1", workflow_id: "wf1" }),
    );

    const state = store.getState();
    expect(state.tasks.pinnedSessionId).toBeNull();
    expect(state.tasks.lastSessionByTaskId).not.toHaveProperty("t1");
    expect(state.tasks.lastSessionByTaskId).toHaveProperty("t2", SESS_OTHER);
  });
});

describe("task.deleted live notification + redirect", () => {
  it("sets a task-deleted notification (with title + reason) when the focused task is deleted", () => {
    const store = makeActiveStore();
    registerTasksHandlers(store)["task.deleted"]!(
      makeDeletedMessage({
        task_id: "t1",
        workflow_id: "wf1",
        title: REVIEW_TITLE,
        reason: "pr_approved_by_user",
      }),
    );

    expect(store.getState().setTaskDeletedNotification).toHaveBeenCalledWith({
      taskId: "t1",
      title: REVIEW_TITLE,
      reason: "pr_approved_by_user",
    });
  });

  it("does not notify when a non-focused task is deleted", () => {
    const store = makeStore({
      kanban: { workflowId: "wf1", steps: [], tasks: [{ id: "t1", workflowId: "wf1" }] },
      tasks: {
        activeTaskId: "t2",
        activeSessionId: null,
        pinnedSessionId: null,
        lastSessionByTaskId: {},
      },
      environmentIdBySessionId: {},
    } as unknown as Partial<AppState>);

    registerTasksHandlers(store)["task.deleted"]!(
      makeDeletedMessage({ task_id: "t1", workflow_id: "wf1" }),
    );

    expect(store.getState().setTaskDeletedNotification).not.toHaveBeenCalled();
  });

  it("does not redirect or notify for a user-initiated delete on the task route", () => {
    window.history.replaceState({}, "", "/t/t1");
    const store = makeActiveStore();

    registerTasksHandlers(store)["task.deleted"]!(
      makeDeletedMessage({ task_id: "t1", workflow_id: "wf1", title: REVIEW_TITLE }),
    );

    expect(window.location.pathname).toBe("/t/t1");
    expect(store.getState().setTaskDeletedNotification).not.toHaveBeenCalled();
  });

  it.each(["/t/t1", "/tasks/t1"])(
    "redirects home and notifies when parked on %s before activeTaskId hydrates",
    (path) => {
      window.history.replaceState({}, "", path);
      const store = makeInactiveStore();

      registerTasksHandlers(store)["task.deleted"]!(
        makeDeletedMessage({
          task_id: "t1",
          workflow_id: "wf1",
          title: REVIEW_TITLE,
          reason: "pr_approved_by_user",
        }),
      );

      expect(window.location.pathname).toBe("/");
      expect(store.getState().setTaskDeletedNotification).toHaveBeenCalledWith({
        taskId: "t1",
        title: REVIEW_TITLE,
        reason: "pr_approved_by_user",
      });
    },
  );

  it("does not redirect an auto-deletion when viewing a different route", () => {
    window.history.replaceState({}, "", "/t/other");
    const store = makeActiveStore();

    registerTasksHandlers(store)["task.deleted"]!(
      makeDeletedMessage({
        task_id: "t1",
        workflow_id: "wf1",
        title: REVIEW_TITLE,
        reason: "pr_approved_by_user",
      }),
    );

    expect(window.location.pathname).toBe("/t/other");
  });
});
