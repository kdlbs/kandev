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

  it("applies history and pagination metadata on merge", () => {
    const store = makeStore();
    store.getState().mergeMessages(SESSION, snapshot(), {
      historyInitialized: true,
      hasMore: true,
      oldestCursor: "a",
    });
    const meta = store.getState().messages.metaBySession[SESSION];
    expect(meta.historyInitialized).toBe(true);
    expect(meta.hasMore).toBe(true);
    expect(meta.oldestCursor).toBe("a");
  });

  it("does not initialize history from live-message inserts", () => {
    const store = makeStore();
    store.getState().addMessage(makeMessage({ id: "live", author_type: "agent" }));

    const meta = store.getState().messages.metaBySession[SESSION];
    expect(meta).toBeUndefined();
  });
});

describe("mergeMessages — prompt_index reconciliation", () => {
  it("treats a newly present prompt_index as a change even when updated_at is unchanged", () => {
    const store = makeStore();
    const base = makeMessage({ id: "a", content: "one", updated_at: "2024-01-01T00:00:00Z" });
    store.getState().mergeMessages(SESSION, [base]);
    const first = store.getState().messages.bySession[SESSION];

    store.getState().mergeMessages(SESSION, [
      makeMessage({
        id: "a",
        content: "one",
        updated_at: "2024-01-01T00:00:00Z",
        prompt_index: 4,
      }),
    ]);
    const next = store.getState().messages.bySession[SESSION];
    expect(next).not.toBe(first);
    expect(next[0]).not.toBe(first[0]);
    expect(next[0].prompt_index).toBe(4);
  });

  it("carries a known prompt_index forward when the incoming payload omits it", () => {
    const store = makeStore();
    store
      .getState()
      .mergeMessages(SESSION, [makeMessage({ id: "a", content: "one", prompt_index: 4 })]);
    const first = store.getState().messages.bySession[SESSION];

    // A transient/older payload without the ordinal must not clear it.
    store.getState().mergeMessages(SESSION, [makeMessage({ id: "a", content: "one" })]);
    const next = store.getState().messages.bySession[SESSION];
    expect(next[0].prompt_index).toBe(4);

    // An explicit replacement still wins.
    store
      .getState()
      .mergeMessages(SESSION, [makeMessage({ id: "a", content: "one", prompt_index: 9 })]);
    expect(store.getState().messages.bySession[SESSION][0].prompt_index).toBe(9);
    expect(store.getState().messages.bySession[SESSION][0]).not.toBe(first[0]);
  });

  it("does not clear a known index from an older/transient payload", () => {
    const store = makeStore();
    store.getState().mergeMessages(SESSION, [makeMessage({ id: "a", prompt_index: 2 })]);
    const first = store.getState().messages.bySession[SESSION];

    // A refetch whose payload omits the ordinal keeps the known one, and the
    // array reference is preserved when nothing else changed.
    store.getState().mergeMessages(SESSION, [makeMessage({ id: "a" })]);
    expect(store.getState().messages.bySession[SESSION]).toBe(first);
    expect(store.getState().messages.bySession[SESSION][0].prompt_index).toBe(2);
  });
});

describe("prependMessages — coordinator-owned isLoadingMore", () => {
  it("does not clear isLoadingMore on an older-page merge (two cursors in flight stay loading)", () => {
    const store = makeStore();
    // The shared pagination coordinator raised the flag for a session with
    // two in-flight cursor requests; a prepend for the FIRST response must
    // not expose false while the second is still pending.
    store.getState().setMessagesMetadata(SESSION, { isLoadingMore: true });

    store.getState().prependMessages(SESSION, [makeMessage({ id: "older", content: "old" })], {
      hasMore: true,
      oldestCursor: "older",
    });

    expect(store.getState().messages.metaBySession[SESSION].isLoadingMore).toBe(true);
  });
});
