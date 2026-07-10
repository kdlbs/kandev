import { describe, it, expect } from "vitest";
import { create } from "zustand";
import { immer } from "zustand/middleware/immer";
import { createSessionSlice } from "./session-slice";
import type { SessionSlice } from "./types";
import { sessionId as toSessionId, taskId as toTaskId, type Message } from "@/lib/types/http";

function makeStore() {
  return create<SessionSlice>()(
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    immer((set) => ({ ...(createSessionSlice as any)(set) })),
  );
}

const SESSION = "sess-1";

function makeMessage(overrides: Partial<Message>): Message {
  return {
    id: "m1",
    task_id: toTaskId("task-1"),
    session_id: toSessionId(SESSION),
    author_type: "user",
    content: "hello",
    type: "message",
    created_at: "2024-01-01T00:00:00Z",
    ...overrides,
  } as Message;
}

function snapshot(): Message[] {
  return [makeMessage({ id: "a", content: "one" }), makeMessage({ id: "b", content: "two" })];
}

describe("mergeMessages", () => {
  it("preserves the array reference on a no-op refetch", () => {
    const store = makeStore();
    store.getState().mergeMessages(SESSION, snapshot());
    const first = store.getState().messages.bySession[SESSION];
    // Fresh, equal objects as a refetch would produce.
    store.getState().mergeMessages(SESSION, snapshot());
    expect(store.getState().messages.bySession[SESSION]).toBe(first);
  });

  it("changes the array ref and only the changed element when one message changes", () => {
    const store = makeStore();
    store.getState().mergeMessages(SESSION, snapshot());
    const first = store.getState().messages.bySession[SESSION];

    store
      .getState()
      .mergeMessages(SESSION, [
        makeMessage({ id: "a", content: "one" }),
        makeMessage({ id: "b", content: "two-edited" }),
      ]);
    const next = store.getState().messages.bySession[SESSION];
    expect(next).not.toBe(first);
    expect(next[0]).toBe(first[0]); // unchanged message kept its identity
    expect(next[1]).not.toBe(first[1]);
    expect(next[1].content).toBe("two-edited");
  });

  it("appends new messages while reusing existing references", () => {
    const store = makeStore();
    store.getState().mergeMessages(SESSION, [makeMessage({ id: "a", content: "one" })]);
    const first = store.getState().messages.bySession[SESSION];

    store
      .getState()
      .mergeMessages(SESSION, [
        makeMessage({ id: "a", content: "one" }),
        makeMessage({ id: "b", content: "two" }),
      ]);
    const next = store.getState().messages.bySession[SESSION];
    expect(next).not.toBe(first);
    expect(next[0]).toBe(first[0]);
    expect(next).toHaveLength(2);
  });

  it("detects a metadata-only change", () => {
    const store = makeStore();
    store.getState().mergeMessages(SESSION, [makeMessage({ id: "a", metadata: { x: 1 } })]);
    const first = store.getState().messages.bySession[SESSION];

    store.getState().mergeMessages(SESSION, [makeMessage({ id: "a", metadata: { x: 2 } })]);
    const next = store.getState().messages.bySession[SESSION];
    expect(next).not.toBe(first);
    expect((next[0].metadata as { x: number }).x).toBe(2);
  });

  it("applies metadata (hasMore / oldestCursor) on merge", () => {
    const store = makeStore();
    store.getState().mergeMessages(SESSION, snapshot(), { hasMore: true, oldestCursor: "a" });
    const meta = store.getState().messages.metaBySession[SESSION];
    expect(meta.hasMore).toBe(true);
    expect(meta.oldestCursor).toBe("a");
  });
});
