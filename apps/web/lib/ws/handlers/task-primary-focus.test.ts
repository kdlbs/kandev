import { beforeEach, describe, expect, it, vi } from "vitest";
import { createAppStore, type AppState } from "@/lib/state/store";
import type { StoreApi } from "zustand";
import type { TaskSession, TaskSessionResponse } from "@/lib/types/http";
import type * as SessionApi from "@/lib/api/domains/session-api";
import { fetchTaskSession } from "@/lib/api/domains/session-api";
import { getSessionRestartStore } from "@/lib/session/session-restart-state";
import { registerTaskSessionHandlers } from "./agent-session";
import { registerTasksHandlers } from "./tasks";
import { makeMessage, makeTask } from "./tasks.test-helpers";

vi.mock("@/lib/api/domains/session-api", async (original) => ({
  ...(await original<typeof SessionApi>()),
  fetchTaskSession: vi.fn(),
}));

function response(state: TaskSession["state"], id = "old", taskId = "t1"): TaskSessionResponse {
  return { session: { id, task_id: taskId, state } as TaskSession };
}
function deferred() {
  return Promise.withResolvers<TaskSessionResponse>();
}
function selectedStore() {
  const store = createAppStore({
    kanban: {
      workflowId: "wf1",
      steps: [],
      tasks: [
        {
          id: "t1",
          workflowId: "wf1",
          workflowStepId: "step1",
          title: "Task",
          position: 0,
          primarySessionId: "old",
        },
      ],
    },
  });
  for (const [id, state] of [
    ["old", "STARTING"],
    ["next", "WAITING_FOR_INPUT"],
    ["latest", "WAITING_FOR_INPUT"],
  ] as const) {
    store
      .getState()
      .setTaskSession({ id, task_id: "t1", state, started_at: "", updated_at: "" } as TaskSession);
  }
  store.getState().setActiveSessionAuto("t1", "old");
  return store;
}
function promote(store: StoreApi<AppState>, sessionId = "next") {
  registerTasksHandlers(store)["task.updated"]!(makeMessage(makeTask("t1", sessionId)));
}
function fail(store: StoreApi<AppState>) {
  registerTaskSessionHandlers(store)["session.state_changed"]!({
    id: "failed",
    type: "notification",
    action: "session.state_changed",
    payload: { task_id: "t1", session_id: "old", new_state: "FAILED" },
  });
}

beforeEach(() => vi.clearAllMocks());

// @covers AC-TASKS-TASK-UNREAD-DOT-001.6
// @covers AC-TASKS-TASK-UNREAD-DOT-001.8
describe("authoritative primary focus follow", () => {
  it.each(["promotion-first", "failure-first"])(
    "keeps failure visible with %s delivery",
    async (order) => {
      const store = selectedStore();
      const read = deferred();
      vi.mocked(fetchTaskSession).mockReturnValue(read.promise);
      if (order === "failure-first") fail(store);
      promote(store);
      expect(store.getState().tasks.activeSessionId).toBe("old");
      read.resolve(response("FAILED"));
      await read.promise;
      expect(store.getState().tasks.activeSessionId).toBe("old");
      if (order === "promotion-first") fail(store);
      expect(store.getState().kanban.tasks[0].primarySessionId).toBe("next");
      expect(store.getState().tasks.activeSessionId).toBe("old");
    },
  );

  it.each(["WAITING_FOR_INPUT", "COMPLETED", "CANCELLED"] as const)(
    "follows an authoritative %s predecessor",
    async (state) => {
      const store = selectedStore();
      const read = deferred();
      vi.mocked(fetchTaskSession).mockReturnValue(read.promise);
      promote(store);
      expect(store.getState().tasks.activeSessionId).toBe("old");
      read.resolve(response(state));
      await read.promise;
      expect(store.getState().tasks.activeSessionId).toBe("next");
    },
  );
});

describe("primary follow ownership", () => {
  it.each(["navigation", "task-departure", "auto-selection", "restart", "workflow-focus"])(
    "does not override newer %s even after returning to the same selection",
    async (change) => {
      const store = selectedStore();
      const read = deferred();
      vi.mocked(fetchTaskSession).mockReturnValue(read.promise);
      promote(store);
      if (change === "navigation") {
        store.getState().setActiveSession("t1", "latest");
        store.getState().setActiveSession("t1", "old");
      } else if (change === "task-departure") {
        store.getState().setActiveTask("other-task");
        store.getState().setActiveSessionAuto("t1", "old");
      } else if (change === "auto-selection") {
        store.getState().setActiveSessionAuto("t1", "latest");
        store.getState().setActiveSessionAuto("t1", "old");
      } else if (change === "restart") {
        const restart = getSessionRestartStore(store);
        restart.setState(new Set(["old"]), true);
        restart.setState(new Set(), true);
      } else {
        store.setState({
          workflowSessionFocus: { ...store.getState().workflowSessionFocus, nextRequestId: 99 },
        });
      }
      read.resolve(response("COMPLETED"));
      await read.promise;
      expect(store.getState().tasks.activeSessionId).toBe("old");
    },
  );

  it("follows only the newest primary after overlapping reads", async () => {
    const store = selectedStore();
    const first = deferred();
    const second = deferred();
    vi.mocked(fetchTaskSession)
      .mockReturnValueOnce(first.promise)
      .mockReturnValueOnce(second.promise);
    promote(store);
    promote(store, "latest");
    first.resolve(response("COMPLETED"));
    await first.promise;
    expect(store.getState().tasks.activeSessionId).toBe("old");
    second.resolve(response("COMPLETED"));
    await second.promise;
    expect(store.getState().tasks.activeSessionId).toBe("latest");
  });

  it("keeps a newer committed workflow recipient despite an earlier pending read", async () => {
    const store = selectedStore();
    const read = deferred();
    vi.mocked(fetchTaskSession).mockReturnValue(read.promise);
    promote(store);
    const requestId = store.getState().beginWorkflowSessionFocus({
      taskId: "t1",
      workflowId: "wf1",
      destinationStepId: "step2",
      presentationToken: 1,
      navigationRevision: store.getState().taskRemoval.navigationRevision,
    });
    expect(requestId).not.toBeNull();
    store.getState().bindWorkflowSessionFocus({
      requestId: requestId!,
      presentationToken: 1,
      entryIdentity: "entry:00000000000000000042",
    });
    registerTasksHandlers(store)["task.updated"]!(
      makeMessage({
        ...makeTask("t1", "latest"),
        workflow_step_id: "step2",
        metadata: {
          workflow_session_route: {
            operation_id: "move",
            destination_step_id: "step2",
            entry_identity: "entry:00000000000000000042",
            destination_session_id: "latest",
            phase: "committed",
          },
        },
      }),
    );
    expect(store.getState().tasks.activeSessionId).toBe("latest");
    read.resolve(response("FAILED"));
    await read.promise;
    expect(store.getState().tasks.activeSessionId).toBe("latest");
  });
});

describe("primary follow readback", () => {
  it("does not hydrate an older read cursor from the verification response", async () => {
    const store = selectedStore();
    const read = deferred();
    vi.mocked(fetchTaskSession).mockReturnValue(read.promise);
    promote(store);
    store.setState(({ taskSessions }) => ({
      taskSessions: {
        ...taskSessions,
        items: {
          ...taskSessions.items,
          old: { ...taskSessions.items.old, last_read_message_id: "new-cursor" },
        },
      },
    }));
    const previous = response("WAITING_FOR_INPUT");
    previous.session.last_read_message_id = "old-cursor";
    read.resolve(previous);
    await read.promise;
    expect(store.getState().tasks.activeSessionId).toBe("next");
    expect(store.getState().taskSessions.items.old.last_read_message_id).toBe("new-cursor");
  });

  it.each([
    response("FAILED"),
    response("RUNNING", "foreign"),
    response("RUNNING", "old", "foreign-task"),
    { session: { id: "old", task_id: "t1" } } as TaskSessionResponse,
  ])("fails closed on failed or mismatched readback: %j", async (result) => {
    const store = selectedStore();
    const read = deferred();
    vi.mocked(fetchTaskSession).mockReturnValue(read.promise);
    promote(store);
    read.resolve(result);
    await read.promise;
    expect(store.getState().tasks.activeSessionId).toBe("old");
  });

  it("preserves selection when verification fails", async () => {
    const store = selectedStore();
    const read = deferred();
    vi.mocked(fetchTaskSession).mockReturnValue(read.promise);
    promote(store);
    read.reject(new Error("read unavailable"));
    await expect(read.promise).rejects.toThrow("read unavailable");
    expect(store.getState().tasks.activeSessionId).toBe("old");
  });
});
