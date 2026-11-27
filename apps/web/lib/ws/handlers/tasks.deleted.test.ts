import { beforeEach, describe, expect, it, vi } from "vitest";
import { createAppStore, type AppState } from "@/lib/state/store";
import type { TaskSession } from "@/lib/types/http";
import { sessionId, taskId } from "@/lib/types/ids";
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

const SESS_NORMALIZED = "sess-normalized";

function storedSession(id: string, owner = "t1"): TaskSession {
  return {
    id: sessionId(id),
    task_id: taskId(owner),
    state: "WAITING_FOR_INPUT",
    started_at: "2026-11-19T12:00:00Z",
    updated_at: "2026-11-19T12:00:00Z",
    last_read_message_id: `${id}-read`,
  };
}

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
        resumeSkippedSessionIds: {},
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
    expect(state.clearQueueStatus).toHaveBeenCalledWith(SESS_PINNED);
  });

  it("clears normalized sessions even when no queue metadata was fetched", () => {
    const store = makeStore({
      taskSessions: {
        items: {
          [SESS_NORMALIZED]: {
            id: SESS_NORMALIZED,
            task_id: "t1",
            queue_incarnation_id: "inc-normalized",
          },
        },
      },
    } as unknown as Partial<AppState>);

    registerTasksHandlers(store)["task.deleted"]!(
      makeDeletedMessage({ task_id: "t1", workflow_id: "wf1" }),
    );

    expect(store.getState().clearQueueStatus).toHaveBeenCalledWith(SESS_NORMALIZED);
  });

  it("removes deleted tasks from the archived sidebar projection", () => {
    const store = makeStore({
      sidebarArchivedTasks: {
        itemsByWorkspaceId: {
          "ws-1": [{ id: "t1", workspaceId: "ws-1", isArchived: true }],
        },
        loadedByWorkspaceId: { "ws-1": true },
        loadingByWorkspaceId: { "ws-1": false },
        errorByWorkspaceId: { "ws-1": null },
      },
    } as unknown as Partial<AppState>);

    registerTasksHandlers(store)["task.deleted"]!(
      makeDeletedMessage({ task_id: "t1", workflow_id: "wf1" }),
    );

    expect(store.getState().sidebarArchivedTasks.itemsByWorkspaceId["ws-1"]).toEqual([]);
  });
});

describe("task.deleted session eviction", () => {
  it("removes every session of the deleted task from the store", () => {
    const foreign = storedSession("sess-t2", "t2");
    const store = createAppStore({
      kanban: {
        workflowId: "wf1",
        steps: [],
        tasks: [
          {
            id: "t1",
            title: "Task",
            position: 0,
            workflowId: "wf1",
            workflowStepId: "step1",
            primarySessionId: SESS_PINNED,
          },
        ],
      },
      taskSessionsByTask: {
        itemsByTaskId: { t1: [storedSession(SESS_OTHER)] },
        loadedByTaskId: {},
        loadingByTaskId: {},
      },
      taskSessions: {
        items: { [SESS_NORMALIZED]: storedSession(SESS_NORMALIZED), "sess-t2": foreign },
      },
    });
    registerTasksHandlers(store)["task.deleted"]!(
      makeDeletedMessage({ task_id: "t1", workflow_id: "wf1" }),
    );
    for (const id of [SESS_OTHER, SESS_PINNED, SESS_NORMALIZED]) {
      store.getState().upsertTaskSessionFromEvent("t1", storedSession(id));
      expect(store.getState().taskSessions.items[id]).toBeUndefined();
    }
    expect(store.getState().taskSessionsByTask.itemsByTaskId.t1).toEqual([]);
    expect(store.getState().taskSessions.items["sess-t2"]).toEqual(foreign);
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
        resumeSkippedSessionIds: {},
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
      expect(window.location.search).toBe("?home=overview");
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
