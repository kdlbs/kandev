import { describe, expect, it, vi } from "vitest";
import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import type { BackendMessageMap } from "@/lib/types/backend";
import { createMessagesHandlerRegistration, createMessageUpdateScheduler } from "./messages";

type UpdatedMessage = BackendMessageMap["session.message.updated"];
const TEST_TIMESTAMP = "2026-08-02T00:00:00.000Z";

function makePayload(sessionId: string, messageId: string, content: string) {
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

function makeStore(currentMessages: Record<string, unknown[]> = {}) {
  const updateMessage = vi.fn();
  const addMessage = vi.fn();
  const removeMessage = vi.fn();
  const state = {
    updateMessage,
    addMessage,
    removeMessage,
    messages: { bySession: currentMessages },
  };
  return {
    store: { getState: () => state } as unknown as StoreApi<AppState>,
    updateMessage,
    addMessage,
    removeMessage,
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
    const { store, updateMessage } = makeStore();
    const frame = makeFrameScheduler();
    const registration = createMessagesHandlerRegistration(store, frame);
    const handler = registration.handlers["session.message.updated"]!;

    for (let index = 0; index < 300; index += 1) {
      handler(makeUpdated("session-1", "message-1", `content-${index}`));
    }

    expect(updateMessage).not.toHaveBeenCalled();
    expect(frame.schedule).toHaveBeenCalledTimes(1);
    frame.runFrame();

    expect(updateMessage).toHaveBeenCalledTimes(1);
    expect(updateMessage).toHaveBeenLastCalledWith(
      expect.objectContaining({
        id: "message-1",
        session_id: "session-1",
        content: "content-299",
      }),
    );
    registration.dispose();
  });

  it("updates different message keys once each in insertion order", () => {
    const { store, updateMessage } = makeStore();
    const frame = makeFrameScheduler();
    const registration = createMessagesHandlerRegistration(store, frame);
    const handler = registration.handlers["session.message.updated"]!;

    handler(makeUpdated("session-1", "message-a", "a1"));
    handler(makeUpdated("session-2", "message-b", "b1"));
    handler(makeUpdated("session-1", "message-a", "a2"));
    frame.runFrame();

    expect(updateMessage).toHaveBeenCalledTimes(2);
    expect(updateMessage.mock.calls.map(([message]) => message.content)).toEqual(["a2", "b1"]);
    registration.dispose();
  });

  it("flushes pending updates before add and delete barriers", () => {
    const { store, updateMessage, addMessage, removeMessage } = makeStore();
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
    expect(updateMessage).toHaveBeenCalledTimes(1);
    expect(removeMessage).toHaveBeenCalledWith("session-1", "message-1");
    frame.runFrame();
    expect(updateMessage).toHaveBeenCalledTimes(1);

    handlers["session.message.updated"]!(makeUpdated("session-1", "message-2", "before-add"));
    handlers["session.message.added"]!({
      id: "add-1",
      type: "notification",
      action: "session.message.added",
      payload: makePayload("session-1", "message-3", "added"),
      timestamp: TEST_TIMESTAMP,
    });
    expect(updateMessage).toHaveBeenCalledTimes(2);
    expect(addMessage).toHaveBeenCalledTimes(1);
    frame.runFrame();
    expect(updateMessage).toHaveBeenCalledTimes(2);
    registration.dispose();
  });

  it("clears scheduled work when the handler registration is disposed", () => {
    const { store, updateMessage } = makeStore();
    const frame = makeFrameScheduler();
    const registration = createMessagesHandlerRegistration(store, frame);
    registration.handlers["session.message.updated"]!(
      makeUpdated("session-1", "message-1", "discarded"),
    );

    registration.dispose();
    expect(frame.cancel).toHaveBeenCalledTimes(1);
    frame.runFrame();
    expect(updateMessage).not.toHaveBeenCalled();
  });

  it("flushes a pending update as a turn-settle barrier", () => {
    const { store, updateMessage } = makeStore();
    const frame = makeFrameScheduler();
    const scheduler = createMessageUpdateScheduler(store, frame);
    scheduler.enqueue(makePayload("session-1", "message-1", "settled"));

    scheduler.flush();

    expect(updateMessage).toHaveBeenCalledTimes(1);
    expect(updateMessage).toHaveBeenLastCalledWith(expect.objectContaining({ content: "settled" }));
    scheduler.dispose();
  });
});

describe("session message snapshot ordering", () => {
  it("does not apply a batched update older than a refetched snapshot", () => {
    const { store, updateMessage } = makeStore({
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
    registration.dispose();
  });
});
