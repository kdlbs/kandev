import { create } from "zustand";
import { immer } from "zustand/middleware/immer";
import { describe, expect, it } from "vitest";
import { sessionId as toSessionId, taskId as toTaskId, type Message } from "@/lib/types/http";
import { createSessionSlice } from "./session-slice";
import type { SessionSlice } from "./types";

const SESSION = "session-1";

function makeStore() {
  return create<SessionSlice>()(
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    immer((set) => ({ ...(createSessionSlice as any)(set) })),
  );
}

function makeMessage(
  id: string,
  content: string,
  session: string = SESSION,
  overrides: Partial<Message> = {},
): Message {
  return {
    id,
    task_id: toTaskId("task-1"),
    session_id: toSessionId(session),
    author_type: "agent",
    content,
    type: "message",
    created_at: "2026-08-27T00:00:00Z",
    updated_at: "2026-08-27T00:00:00Z",
    ...overrides,
  };
}

describe("updateMessages", () => {
  it("notifies subscribers once for one replacement frame", () => {
    const store = makeStore();
    const initial = [makeMessage("message-a", "a"), makeMessage("message-b", "b")];
    store.getState().setMessages(SESSION, initial);

    const notifications: Message[][] = [];
    const unsubscribe = store.subscribe((state) => {
      notifications.push(state.messages.bySession[SESSION] ?? []);
    });

    const updateMessages = store.getState().updateMessages;
    expect(updateMessages).toBeTypeOf("function");
    updateMessages([makeMessage("message-a", "a2"), makeMessage("message-b", "b2")]);

    expect(notifications).toHaveLength(1);
    expect(store.getState().messages.bySession[SESSION].map((message) => message.content)).toEqual([
      "a2",
      "b2",
    ]);
    unsubscribe();
  });

  it("preserves partial fields and unaffected session identities", () => {
    const store = makeStore();
    const first = makeMessage("message-a", "a", SESSION, {
      metadata: { retained: true },
      prompt_index: 3,
    });
    const second = makeMessage("message-b", "b");
    const otherSessionMessage = makeMessage("message-c", "c", "session-2");
    store.getState().setMessages(SESSION, [first, second]);
    store.getState().setMessages("session-2", [otherSessionMessage]);

    const sessionMessages = store.getState().messages.bySession[SESSION];
    const otherSessionMessages = store.getState().messages.bySession["session-2"];
    store.getState().updateMessages([
      makeMessage("message-a", "a2", SESSION, {
        metadata: undefined,
        prompt_index: undefined,
      }),
    ]);

    const nextSessionMessages = store.getState().messages.bySession[SESSION];
    expect(nextSessionMessages).not.toBe(sessionMessages);
    expect(nextSessionMessages[0].content).toBe("a2");
    expect(nextSessionMessages[0].metadata).toEqual({ retained: true });
    expect(nextSessionMessages[0].prompt_index).toBe(3);
    expect(nextSessionMessages[1]).toBe(sessionMessages[1]);
    expect(store.getState().messages.bySession["session-2"]).toBe(otherSessionMessages);
    expect(store.getState().messages.bySession["session-2"][0]).toBe(otherSessionMessage);
  });
});
