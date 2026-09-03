import { describe, expect, it, vi } from "vitest";
import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import type { BackendMessageMap } from "@/lib/types/backend";
import { createMessagesHandlerRegistration, createMessageUpdateScheduler } from "./messages";

type UpdatedMessage = BackendMessageMap["session.message.updated"];
const TEST_TIMESTAMP = "2026-08-02T00:00:00.000Z";
const PROJECTION_EPOCH = "7";
const SESSION_ID = "session-1";
const QUESTION_ID = "question-1";
const QUESTION_CONTENT = "Choose";
const ADD_ACTION = "session.message.added";

function pendingRevision(sequence: number) {
  return { epoch: PROJECTION_EPOCH, sequence };
}

function makePayload(
  sessionId: string,
  messageId: string,
  content: string,
  overrides: Record<string, unknown> = {},
) {
  return {
    task_id: "task-1",
    session_id: sessionId,
    message_id: messageId,
    turn_id: "turn-1",
    author_type: "agent" as const,
    author_id: "agent-1",
    content,
    type: "message",
    created_at: TEST_TIMESTAMP,
    updated_at: "2026-08-02T00:00:01.000Z",
    ...overrides,
  };
}

function makeUpdated(sessionId: string, messageId: string, content: string): UpdatedMessage {
  return {
    id: `${sessionId}-${messageId}`,
    type: "notification",
    action: "session.message.updated",
    payload: makePayload(sessionId, messageId, content),
    timestamp: TEST_TIMESTAMP,
  };
}

function makeStore(currentMessages: Record<string, unknown[]> = {}, completedTurn = false) {
  const updateMessage = vi.fn();
  const updateMessages = vi.fn();
  const addMessage = vi.fn();
  const removeMessage = vi.fn();
  const setTaskSessionPendingAction = vi.fn();
  const state = {
    updateMessage,
    updateMessages,
    addMessage,
    removeMessage,
    setTaskSessionPendingAction,
    messages: { bySession: currentMessages },
    turns: {
      bySession: completedTurn
        ? {
            "session-1": [{ id: "turn-1", completed_at: TEST_TIMESTAMP }],
          }
        : {},
    },
  };
  return {
    store: { getState: () => state } as unknown as StoreApi<AppState>,
    updateMessage,
    updateMessages,
    addMessage,
    removeMessage,
    setTaskSessionPendingAction,
  };
}

function makeFrameScheduler() {
  let frame: (() => void) | null = null;
  const schedule = vi.fn((callback: () => void) => {
    frame = callback;
    return 1;
  });
  const cancel = vi.fn(() => {
    frame = null;
  });
  return {
    schedule,
    cancel,
    runFrame: () => {
      const callback = frame;
      frame = null;
      callback?.();
    },
  };
}

describe("session message frame scheduler", () => {
  it("applies hundreds of same-key updates once with the newest full payload", () => {
    const { store, updateMessage, updateMessages } = makeStore();
    const frame = makeFrameScheduler();
    const registration = createMessagesHandlerRegistration(store, frame);
    const handler = registration.handlers["session.message.updated"]!;

    for (let index = 0; index < 300; index += 1) {
      handler(makeUpdated("session-1", "message-1", `content-${index}`));
    }

    expect(updateMessage).not.toHaveBeenCalled();
    expect(frame.schedule).toHaveBeenCalledTimes(1);
    frame.runFrame();

    expect(updateMessage).not.toHaveBeenCalled();
    expect(updateMessages).toHaveBeenCalledTimes(1);
    expect(updateMessages).toHaveBeenLastCalledWith([
      expect.objectContaining({
        id: "message-1",
        session_id: "session-1",
        content: "content-299",
      }),
    ]);
    registration.dispose();
  });

  it("applies different message keys with one bulk store action", () => {
    const { store, updateMessage, updateMessages } = makeStore();
    const frame = makeFrameScheduler();
    const registration = createMessagesHandlerRegistration(store, frame);
    const handler = registration.handlers["session.message.updated"]!;

    handler(makeUpdated("session-1", "message-a", "a1"));
    handler(makeUpdated("session-2", "message-b", "b1"));
    handler(makeUpdated("session-1", "message-a", "a2"));
    frame.runFrame();

    expect(updateMessage).not.toHaveBeenCalled();
    expect(updateMessages).toHaveBeenCalledTimes(1);
    expect(
      updateMessages.mock.calls[0]?.[0].map((message: { content: string }) => message.content),
    ).toEqual(["a2", "b1"]);
    registration.dispose();
  });

  it("flushes pending updates before add and delete barriers", () => {
    const { store, updateMessage, updateMessages, addMessage, removeMessage } = makeStore();
    const frame = makeFrameScheduler();
    const registration = createMessagesHandlerRegistration(store, frame);
    const handlers = registration.handlers;

    handlers["session.message.updated"]!(makeUpdated("session-1", "message-1", "before-delete"));
    handlers["session.message.deleted"]!({
      id: "delete-1",
      type: "notification",
      action: "session.message.deleted",
      payload: makePayload("session-1", "message-1", "ignored"),
      timestamp: TEST_TIMESTAMP,
    });
    expect(updateMessage).not.toHaveBeenCalled();
    expect(updateMessages).toHaveBeenCalledTimes(1);
    expect(removeMessage).toHaveBeenCalledWith("session-1", "message-1");
    frame.runFrame();
    expect(updateMessages).toHaveBeenCalledTimes(1);

    handlers["session.message.updated"]!(makeUpdated("session-1", "message-2", "before-add"));
    handlers[ADD_ACTION]!({
      id: "add-1",
      type: "notification",
      action: ADD_ACTION,
      payload: makePayload("session-1", "message-3", "added"),
      timestamp: TEST_TIMESTAMP,
    });
    expect(updateMessages).toHaveBeenCalledTimes(2);
    expect(addMessage).toHaveBeenCalledTimes(1);
    frame.runFrame();
    expect(updateMessages).toHaveBeenCalledTimes(2);
    registration.dispose();
  });

  it("clears scheduled work when the handler registration is disposed", () => {
    const { store, updateMessage, updateMessages } = makeStore();
    const frame = makeFrameScheduler();
    const registration = createMessagesHandlerRegistration(store, frame);
    registration.handlers["session.message.updated"]!(
      makeUpdated("session-1", "message-1", "discarded"),
    );

    registration.dispose();
    expect(frame.cancel).toHaveBeenCalledTimes(1);
    frame.runFrame();
    expect(updateMessage).not.toHaveBeenCalled();
    expect(updateMessages).not.toHaveBeenCalled();
  });

  it("flushes a pending update as a turn-settle barrier", () => {
    const { store, updateMessage, updateMessages } = makeStore();
    const frame = makeFrameScheduler();
    const scheduler = createMessageUpdateScheduler(store, frame);
    scheduler.enqueue(makePayload("session-1", "message-1", "settled"));

    scheduler.flush();

    expect(updateMessage).not.toHaveBeenCalled();
    expect(updateMessages).toHaveBeenCalledTimes(1);
    expect(updateMessages).toHaveBeenLastCalledWith([
      expect.objectContaining({ content: "settled" }),
    ]);
    scheduler.dispose();
  });
});

describe("session message prompt_index mapping", () => {
  it("maps prompt_index from a session.message.added payload", () => {
    const { store, addMessage } = makeStore();
    const registration = createMessagesHandlerRegistration(store);

    registration.handlers[ADD_ACTION]!({
      id: "add-numbered",
      type: "notification",
      action: ADD_ACTION,
      payload: makePayload("session-1", "message-1", "numbered prompt", {
        author_type: "user",
        prompt_index: 2,
      }),
      timestamp: TEST_TIMESTAMP,
    });

    expect(addMessage).toHaveBeenCalledWith(
      expect.objectContaining({ id: "message-1", prompt_index: 2 }),
    );
    registration.dispose();
  });

  it("maps prompt_index from a session.message.updated payload", () => {
    const { store, updateMessage, updateMessages } = makeStore();
    const registration = createMessagesHandlerRegistration(store);

    const payload = makePayload("session-1", "message-1", "edited", {
      author_type: "user",
      prompt_index: 5,
    });
    registration.handlers["session.message.updated"]!({
      ...makeUpdated("session-1", "message-1", "edited"),
      payload,
    });
    registration.scheduler.flush();

    expect(updateMessage).not.toHaveBeenCalled();
    expect(updateMessages).toHaveBeenLastCalledWith([
      expect.objectContaining({ id: "message-1", prompt_index: 5 }),
    ]);
    registration.dispose();
  });

  it("leaves prompt_index undefined when the payload omits it (store merge preserves known ordinals)", () => {
    const { store, addMessage } = makeStore();
    const registration = createMessagesHandlerRegistration(store);

    registration.handlers[ADD_ACTION]!({
      id: "add-plain",
      type: "notification",
      action: ADD_ACTION,
      payload: makePayload("session-1", "message-1", "plain prompt", {
        author_type: "user",
      }),
      timestamp: TEST_TIMESTAMP,
    });

    expect(addMessage).toHaveBeenCalledWith(
      expect.objectContaining({ id: "message-1", prompt_index: undefined }),
    );
    registration.dispose();
  });
});

describe("session message pending-action projection", () => {
  it("applies the authoritative projection carried by a message event", () => {
    const { store, setTaskSessionPendingAction } = makeStore();
    const registration = createMessagesHandlerRegistration(store);

    registration.handlers[ADD_ACTION]!({
      id: "add-clarification",
      type: "notification",
      action: ADD_ACTION,
      payload: {
        ...makePayload(SESSION_ID, QUESTION_ID, QUESTION_CONTENT),
        type: "clarification_request",
        pending_action: "clarification",
        pending_action_revision: pendingRevision(1),
      },
      timestamp: TEST_TIMESTAMP,
    });

    expect(setTaskSessionPendingAction).toHaveBeenCalledWith(
      SESSION_ID,
      "clarification",
      pendingRevision(1),
      "task-1",
    );

    registration.handlers["session.message.updated"]!({
      ...makeUpdated(SESSION_ID, QUESTION_ID, QUESTION_CONTENT),
      payload: {
        ...makePayload(SESSION_ID, QUESTION_ID, QUESTION_CONTENT),
        type: "clarification_request",
        pending_action: null,
        pending_action_revision: pendingRevision(2),
      },
    });
    registration.scheduler.flush();

    expect(setTaskSessionPendingAction).toHaveBeenLastCalledWith(
      SESSION_ID,
      null,
      pendingRevision(2),
      "task-1",
    );
    registration.dispose();
  });

  it("preserves event order when batched updates revisit a message", () => {
    const { store, setTaskSessionPendingAction } = makeStore();
    const registration = createMessagesHandlerRegistration(store);
    const handler = registration.handlers["session.message.updated"]!;
    let sequence = 0;
    const update = (messageId: string, pendingAction: "clarification" | "permission" | null) => {
      handler({
        ...makeUpdated(SESSION_ID, messageId, QUESTION_CONTENT),
        payload: {
          ...makePayload(SESSION_ID, messageId, QUESTION_CONTENT),
          type: "clarification_request",
          pending_action: pendingAction,
          pending_action_revision: pendingRevision(++sequence),
        },
      });
    };

    update("question-a", "clarification");
    update("question-b", null);
    update("question-a", "permission");
    registration.scheduler.flush();

    expect(setTaskSessionPendingAction).toHaveBeenLastCalledWith(
      SESSION_ID,
      "permission",
      pendingRevision(3),
      "task-1",
    );
    registration.dispose();
  });
});

describe("session message deleted pending-action projection", () => {
  it("applies the cleared projection carried by a deleted message event", () => {
    const { store, removeMessage, setTaskSessionPendingAction } = makeStore();
    const registration = createMessagesHandlerRegistration(store);

    registration.handlers["session.message.deleted"]!({
      id: "delete-clarification",
      type: "notification",
      action: "session.message.deleted",
      payload: {
        ...makePayload(SESSION_ID, QUESTION_ID, QUESTION_CONTENT),
        type: "clarification_request",
        pending_action: null,
        pending_action_revision: pendingRevision(4),
      },
      timestamp: TEST_TIMESTAMP,
    });

    expect(removeMessage).toHaveBeenCalledWith(SESSION_ID, QUESTION_ID);
    expect(setTaskSessionPendingAction).toHaveBeenCalledWith(
      SESSION_ID,
      null,
      pendingRevision(4),
      "task-1",
    );
    registration.dispose();
  });
});

describe("session message pending-action ordering", () => {
  it("ignores a projection from an update older than the current message", () => {
    const { store, setTaskSessionPendingAction } = makeStore({
      [SESSION_ID]: [{ id: QUESTION_ID, updated_at: "2026-08-02T00:00:02.000Z" }],
    });
    const registration = createMessagesHandlerRegistration(store);

    registration.handlers["session.message.updated"]!({
      ...makeUpdated(SESSION_ID, QUESTION_ID, QUESTION_CONTENT),
      payload: {
        ...makePayload(SESSION_ID, QUESTION_ID, QUESTION_CONTENT),
        type: "clarification_request",
        pending_action: "clarification",
        pending_action_revision: pendingRevision(1),
      },
    });
    registration.scheduler.flush();

    expect(setTaskSessionPendingAction).not.toHaveBeenCalled();
    registration.dispose();
  });
});

describe("session message snapshot ordering", () => {
  it("does not apply a batched update older than a refetched snapshot", () => {
    const { store, updateMessage, updateMessages } = makeStore({
      "session-1": [
        {
          id: "message-1",
          updated_at: "2026-08-02T00:00:02.000Z",
        },
      ],
    });
    const frame = makeFrameScheduler();
    const registration = createMessagesHandlerRegistration(store, frame);

    registration.handlers["session.message.updated"]!(
      makeUpdated("session-1", "message-1", "stale"),
    );
    frame.runFrame();

    expect(updateMessage).not.toHaveBeenCalled();
    expect(updateMessages).not.toHaveBeenCalled();
    registration.dispose();
  });
});

describe("late tool messages", () => {
  it("settles a tool message added after its turn completed", () => {
    const { store, addMessage } = makeStore({}, true);
    const registration = createMessagesHandlerRegistration(store);

    registration.handlers[ADD_ACTION]!({
      id: "add-tool",
      type: "notification",
      action: ADD_ACTION,
      payload: makePayload("session-1", "tool-1", "Terminal", {
        type: "tool_call",
        metadata: { tool_call_id: "call-1", status: "running" },
      }),
      timestamp: TEST_TIMESTAMP,
    } as never);

    expect(addMessage).toHaveBeenCalledWith(
      expect.objectContaining({
        metadata: { tool_call_id: "call-1", status: "complete" },
      }),
    );
    registration.dispose();
  });

  it("does not let a late running update reintroduce a spinner", () => {
    const { store, updateMessage, updateMessages } = makeStore({}, true);
    const registration = createMessagesHandlerRegistration(store);

    registration.handlers["session.message.updated"]!(
      makeUpdated("session-1", "tool-1", "Terminal") as never,
    );
    registration.handlers["session.message.updated"]!({
      id: "updated-tool",
      type: "notification",
      action: "session.message.updated",
      payload: makePayload("session-1", "tool-1", "Terminal", {
        type: "tool_call",
        metadata: { tool_call_id: "call-1", status: "running" },
      }),
      timestamp: TEST_TIMESTAMP,
    } as never);
    registration.scheduler.flush();

    expect(updateMessage).not.toHaveBeenCalled();
    expect(updateMessages).toHaveBeenLastCalledWith([
      expect.objectContaining({
        metadata: { tool_call_id: "call-1", status: "complete" },
      }),
    ]);
    registration.dispose();
  });
});
