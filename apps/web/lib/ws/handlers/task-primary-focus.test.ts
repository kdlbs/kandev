import { beforeEach, describe, expect, it, vi } from "vitest";
import type { TaskSessionResponse, TaskSessionsResponse } from "@/lib/types/http";
import type * as SessionApi from "@/lib/api/domains/session-api";
import { fetchTaskSession, listTaskSessions } from "@/lib/api/domains/session-api";
import { ApiError } from "@/lib/api/client";
import { getSessionRestartStore } from "@/lib/session/session-restart-state";
import { registerTaskSessionHandlers } from "./agent-session";
import { registerTasksHandlers } from "./tasks";
import { makeMessage, makeStateChangedMessage, makeTask } from "./tasks.test-helpers";

import {
  SESSION_STATE_CHANGED,
  deferred,
  fail,
  flushFocusWork,
  membership,
  notifySibling,
  notifyState,
  promote,
  response,
  selectedStore,
} from "./task-primary-focus.test-helpers";

vi.mock("@/lib/api/domains/session-api", async (original) => ({
  ...(await original<typeof SessionApi>()),
  fetchTaskSession: vi.fn(),
  listTaskSessions: vi.fn(),
}));

const OTHER_TASK = "other-task";

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(fetchTaskSession).mockReset();
  vi.mocked(listTaskSessions).mockReset();
});

const AFTER_STARTUP = "2026-11-19T12:00:02Z";

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
        store.getState().setActiveTask(OTHER_TASK);
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

describe("primary follow coordination", () => {
  it.each([
    ["COMPLETED", true, "next", "RUNNING"],
    ["CANCELLED", true, "next", "RUNNING"],
    ["COMPLETED", false, "latest", "RUNNING"],
    ["CANCELLED", false, "latest", "RUNNING"],
    ["COMPLETED", true, "next", "COMPLETED"],
    ["CANCELLED", true, "next", "CANCELLED"],
  ] as const)(
    "selects %s with pending=%s at %s after %s",
    async (terminal, pending, expected, initialState) => {
      const store = selectedStore(initialState);
      store.setState(({ taskSessions, taskSessionsByTask }) => ({
        taskSessionsByTask: {
          ...taskSessionsByTask,
          itemsByTaskId: {
            ...taskSessionsByTask.itemsByTaskId,
            t1: ["old", "next", "latest"].map((id) => taskSessions.items[id]),
          },
        },
      }));
      const read = deferred();
      vi.mocked(fetchTaskSession).mockReturnValue(read.promise);
      if (pending) promote(store);
      registerTaskSessionHandlers(store)[SESSION_STATE_CHANGED]!({
        id: "terminal",
        type: "notification",
        action: SESSION_STATE_CHANGED,
        payload: { task_id: "t1", session_id: "old", new_state: terminal },
      });
      expect(store.getState().tasks.activeSessionId).toBe("old");
      read.resolve(response(terminal));
      await read.promise;
      expect(store.getState().tasks.activeSessionId).toBe(expected);
    },
  );

  it("follows the promoted primary after an unrelated task is deleted", async () => {
    const store = selectedStore();
    store.setState(({ kanban }) => ({
      kanban: {
        ...kanban,
        tasks: [
          ...kanban.tasks,
          { ...kanban.tasks[0], id: OTHER_TASK, primarySessionId: "other-session" },
        ],
      },
    }));
    const read = deferred();
    vi.mocked(fetchTaskSession).mockReturnValue(read.promise);
    promote(store);
    registerTasksHandlers(store)["task.deleted"]!({
      id: "unrelated-deletion",
      type: "notification",
      action: "task.deleted",
      payload: {
        task_id: OTHER_TASK,
        workflow_id: "wf1",
        workflow_step_id: "step1",
        title: "Task",
        is_ephemeral: false,
      },
    });
    expect(store.getState().tasks.activeTaskId).toBe("t1");
    read.resolve(response("WAITING_FOR_INPUT"));
    await read.promise;
    expect(store.getState().tasks.activeSessionId).toBe("next");
  });

  it("follows the promoted primary across workflow-focus changes on an unrelated task", async () => {
    const store = selectedStore();
    const read = deferred();
    vi.mocked(fetchTaskSession).mockReturnValue(read.promise);
    promote(store);
    store.setState({
      workflowSessionFocus: {
        nextRequestId: 2,
        intent: null,
        request: {
          requestId: 1,
          presentationToken: 1,
          taskId: OTHER_TASK,
          sessionId: "other-session",
        },
      },
    });
    read.resolve(response("COMPLETED"));
    await read.promise;
    expect(store.getState().tasks.activeSessionId).toBe("next");
  });
});

describe("primary follow durable authority", () => {
  it.each([
    ["COMPLETED", "terminal-first"],
    ["CANCELLED", "terminal-first"],
    ["COMPLETED", "read-first"],
    ["CANCELLED", "read-first"],
  ] as const)(
    "keeps a failed predecessor after an old %s snapshot, %s",
    async (terminal, order) => {
      const store = selectedStore(terminal);
      const oldTime = "2026-11-19T12:00:00Z";
      const failedTime = AFTER_STARTUP;
      store.getState().setTaskSession({
        ...response(terminal).session,
        started_at: oldTime,
        updated_at: oldTime,
      });
      store.setState(({ taskSessions, taskSessionsByTask }) => ({
        taskSessionsByTask: {
          ...taskSessionsByTask,
          itemsByTaskId: {
            ...taskSessionsByTask.itemsByTaskId,
            t1: ["old", "next", "latest"].map((id) => taskSessions.items[id]),
          },
        },
      }));
      const read = deferred();
      vi.mocked(fetchTaskSession).mockReturnValue(read.promise);
      promote(store);
      if (order === "read-first") {
        read.resolve({ session: { ...response("FAILED").session, updated_at: failedTime } });
        await read.promise;
      }
      notifyState(store, terminal, oldTime);
      expect(store.getState().tasks.activeSessionId).toBe("old");
      notifyState(store, "STARTING", "2026-11-19T12:00:01Z");
      notifyState(store, "FAILED", failedTime);
      if (order === "terminal-first") {
        read.resolve({ session: { ...response("FAILED").session, updated_at: failedTime } });
      }
      await read.promise;
      await flushFocusWork();
      expect(store.getState().tasks.activeSessionId).toBe("old");
    },
  );

  it("does not follow an older healthy response after FAILED was accepted", async () => {
    const store = selectedStore();
    const read = deferred();
    vi.mocked(fetchTaskSession).mockReturnValue(read.promise);
    promote(store);
    notifyState(store, "FAILED", AFTER_STARTUP);
    read.resolve(response("WAITING_FOR_INPUT"));
    await read.promise;
    expect(store.getState().tasks.activeSessionId).toBe("old");
  });
});

describe("primary projection reconciliation", () => {
  it.each([false, true])("targets the latest state projection, newer pin=%s", async (pinned) => {
    const store = selectedStore();
    const first = deferred();
    const latest = deferred();
    vi.mocked(fetchTaskSession).mockReturnValueOnce(first.promise).mockReturnValue(latest.promise);
    promote(store);
    const projection = {
      ...makeTask("t1", "latest"),
      updated_at: "2026-11-19T12:00:03Z",
    };
    const handlers = registerTasksHandlers(store);
    handlers["task.state_changed"]!(makeStateChangedMessage(projection));
    handlers["task.updated"]!(makeMessage(projection));
    if (pinned) store.getState().setActiveSession("t1", "next");
    latest.resolve(response("WAITING_FOR_INPUT"));
    await latest.promise;
    expect(store.getState().tasks.activeSessionId).toBe(pinned ? "next" : "latest");
    first.resolve(response("WAITING_FOR_INPUT"));
    await first.promise;
    expect(store.getState().tasks.activeSessionId).toBe(pinned ? "next" : "latest");
  });

  it("retargets an owned follow when a creation snapshot refreshes its primary", async () => {
    const store = selectedStore();
    const first = deferred();
    const latest = deferred();
    vi.mocked(fetchTaskSession).mockReturnValueOnce(first.promise).mockReturnValue(latest.promise);
    promote(store);
    const projection = makeTask("t1", "latest");
    registerTasksHandlers(store)["task.created"]!({
      ...makeMessage(projection),
      action: "task.created",
    });
    latest.resolve(response("WAITING_FOR_INPUT"));
    await latest.promise;
    expect(store.getState().tasks.activeSessionId).toBe("latest");
    first.resolve(response("WAITING_FOR_INPUT"));
    await first.promise;
    expect(store.getState().tasks.activeSessionId).toBe("latest");
  });

  it("does not initiate a follow from a creation snapshot alone", async () => {
    const store = selectedStore();
    const read = deferred();
    vi.mocked(fetchTaskSession).mockReturnValue(read.promise);
    registerTasksHandlers(store)["task.created"]!({
      ...makeMessage(makeTask("t1", "latest")),
      action: "task.created",
    });
    read.resolve(response("WAITING_FOR_INPUT"));
    await flushFocusWork();
    expect(store.getState().tasks.activeSessionId).toBe("old");
  });
});

describe("confirmed primary removal", () => {
  it.each(["next", "latest"])(
    "follows the proven primary %s only after scoped absence",
    async (target) => {
      const store = selectedStore("COMPLETED");
      const read = deferred();
      const members = Promise.withResolvers<TaskSessionsResponse>();
      vi.mocked(fetchTaskSession).mockReturnValue(read.promise);
      vi.mocked(listTaskSessions).mockReturnValue(members.promise);
      promote(store);
      read.reject(new ApiError("", 404, null));
      await flushFocusWork();
      expect(store.getState().tasks.activeSessionId).toBe("old");
      members.resolve(membership(target));
      await flushFocusWork();
      expect(store.getState().tasks.activeSessionId).toBe(target);
      expect(store.getState().taskSessions.items.old).toBeUndefined();
    },
  );

  it.each([
    membership("old", "next"),
    { ...membership("next"), total: 2 },
    membership("next", "next"),
    { sessions: [response("WAITING_FOR_INPUT", "next", "foreign").session], total: 1 },
  ])("retains the view for incomplete or ambiguous membership: %j", async (members) => {
    const store = selectedStore();
    const read = deferred();
    vi.mocked(fetchTaskSession).mockReturnValue(read.promise);
    vi.mocked(listTaskSessions).mockResolvedValue(members);
    promote(store);
    read.reject(new ApiError("", 404, null));
    await flushFocusWork();
    expect(store.getState().tasks.activeSessionId).toBe("old");
    expect(store.getState().taskSessions.items.old?.id).toBe("old");
  });

  it("does not mistake a generic read failure for removal", async () => {
    const store = selectedStore();
    const read = deferred();
    vi.mocked(fetchTaskSession).mockReturnValue(read.promise);
    vi.mocked(listTaskSessions).mockResolvedValue(membership("next"));
    promote(store);
    read.reject(new ApiError("server unavailable", 503, null));
    await flushFocusWork();
    expect(store.getState().tasks.activeSessionId).toBe("old");
  });

  it("does not displace a newer pin with a delayed absence proof", async () => {
    const store = selectedStore();
    const read = deferred();
    const members = Promise.withResolvers<TaskSessionsResponse>();
    vi.mocked(fetchTaskSession).mockReturnValue(read.promise);
    vi.mocked(listTaskSessions).mockReturnValue(members.promise);
    promote(store);
    read.reject(new ApiError("", 404, null));
    await flushFocusWork();
    store.getState().setActiveSession("t1", "latest");
    members.resolve(membership("next"));
    await flushFocusWork();
    expect(store.getState().tasks.activeSessionId).toBe("latest");
  });
});

describe("removed primary proof ownership", () => {
  it("preserves the view when membership cannot be read", async () => {
    const store = selectedStore();
    const read = deferred();
    vi.mocked(fetchTaskSession).mockReturnValue(read.promise);
    vi.mocked(listTaskSessions).mockRejectedValue(new ApiError("unavailable", 503, null));
    promote(store);
    read.reject(new ApiError("", 404, null));
    await flushFocusWork();
    expect(store.getState().tasks.activeSessionId).toBe("old");
    expect(store.getState().taskSessions.items.old?.id).toBe("old");
  });

  it.each(["old-first", "new-first"])(
    "keeps retargeted absence proofs scoped: %s",
    async (order) => {
      const store = selectedStore();
      const oldMembers = Promise.withResolvers<TaskSessionsResponse>();
      const newMembers = Promise.withResolvers<TaskSessionsResponse>();
      vi.mocked(fetchTaskSession).mockRejectedValue(new ApiError("", 404, null));
      vi.mocked(listTaskSessions)
        .mockReturnValueOnce(oldMembers.promise)
        .mockReturnValue(newMembers.promise);
      promote(store);
      await flushFocusWork();
      registerTasksHandlers(store)["task.state_changed"]!(
        makeStateChangedMessage(makeTask("t1", "latest")),
      );
      await flushFocusWork();
      if (order === "old-first") {
        oldMembers.resolve(membership("next", "latest"));
        await flushFocusWork();
        expect(store.getState().tasks.activeSessionId).toBe("old");
      }
      newMembers.resolve(membership("latest", "next"));
      await flushFocusWork();
      expect(store.getState().tasks.activeSessionId).toBe("latest");
      if (order === "new-first") {
        oldMembers.resolve(membership("next", "latest"));
        await flushFocusWork();
        expect(store.getState().tasks.activeSessionId).toBe("latest");
      }
    },
  );
});

describe("newly observed sessions", () => {
  it.each(["FAILED", "COMPLETED", "CANCELLED", "IDLE"] as const)(
    "preserves failed focus but retains normal adoption after %s",
    async (state) => {
      const store = selectedStore(state);
      store.getState().removeTaskSession("t1", "latest");
      const read = deferred();
      vi.mocked(fetchTaskSession).mockReturnValue(read.promise);
      notifySibling(store);
      expect(store.getState().tasks.activeSessionId).toBe("old");
      read.resolve(response(state));
      await read.promise;
      expect(store.getState().tasks.activeSessionId).toBe(state === "FAILED" ? "old" : "latest");
    },
  );

  it("does not let an unseen sibling bypass an owned authoritative read", async () => {
    const store = selectedStore("COMPLETED");
    store.getState().removeTaskSession("t1", "latest");
    const read = deferred();
    vi.mocked(fetchTaskSession).mockReturnValue(read.promise);
    promote(store);
    notifySibling(store);
    expect(store.getState().tasks.activeSessionId).toBe("old");
    read.resolve(response("COMPLETED"));
    await read.promise;
    expect(store.getState().tasks.activeSessionId).toBe("next");
  });
});

// @covers AC-TASKS-TASK-UNREAD-DOT-001.6
describe("primary promotions across an in-place startup", () => {
  it.each(["CANCELLED", "STARTING"] as const)(
    "keeps the displayed attempt when cached %s but durable state is STARTING",
    async (state) => {
      const store = selectedStore(state);
      const read = deferred();
      vi.mocked(fetchTaskSession).mockReturnValue(read.promise);
      const selected: Array<string | null> = [];
      const release = store.subscribe((current) => selected.push(current.tasks.activeSessionId));
      try {
        promote(store);
        read.resolve(response("STARTING"));
        await flushFocusWork();
        expect(store.getState().tasks.activeSessionId).toBe("old");
        notifyState(store, "FAILED", AFTER_STARTUP);
        await flushFocusWork();
        expect(selected).not.toContain("next");
        expect(store.getState().kanban.tasks[0].primarySessionId).toBe("next");
      } finally {
        release();
      }
    },
  );

  it("rejects an older healthy read after an accepted STARTING observation", async () => {
    const store = selectedStore("CANCELLED");
    const read = deferred();
    vi.mocked(fetchTaskSession).mockReturnValue(read.promise);
    promote(store);
    notifyState(store, "STARTING", "2026-11-19T12:00:01Z");
    read.resolve(response("WAITING_FOR_INPUT"));
    await flushFocusWork();
    expect(store.getState().tasks.activeSessionId).toBe("old");
    notifyState(store, "FAILED", AFTER_STARTUP);
    await flushFocusWork();
    expect(store.getState().tasks.activeSessionId).toBe("old");
  });

  it("reproves a healthy accepted lifecycle state under the original owner", async () => {
    const store = selectedStore("CANCELLED");
    const starting = deferred();
    const healthy = deferred();
    vi.mocked(fetchTaskSession)
      .mockReturnValueOnce(starting.promise)
      .mockReturnValue(healthy.promise);
    promote(store);
    starting.resolve(response("STARTING"));
    await flushFocusWork();
    expect(store.getState().tasks.activeSessionId).toBe("old");
    notifyState(store, "WAITING_FOR_INPUT", AFTER_STARTUP);
    await flushFocusWork();
    healthy.resolve(response("WAITING_FOR_INPUT"));
    await flushFocusWork();
    expect(store.getState().tasks.activeSessionId).toBe("next");
  });

  it("retains healthy manual-primary following", async () => {
    const store = selectedStore("RUNNING");
    vi.mocked(fetchTaskSession).mockResolvedValue(response("RUNNING"));
    promote(store);
    await flushFocusWork();
    expect(store.getState().tasks.activeSessionId).toBe("next");
  });
});
