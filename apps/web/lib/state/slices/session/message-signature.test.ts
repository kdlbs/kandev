import { describe, expect, it } from "vitest";
import { sessionId as toSessionId, taskId as toTaskId, type Message } from "@/lib/types/http";
import { reconcileMessages, signatureOf } from "./message-signature";

const TS = "2024-01-01T00:00:00Z";

function makeMessage(overrides: Partial<Message>): Message {
  return {
    id: "msg-1",
    task_id: toTaskId("task-1"),
    session_id: toSessionId("sess-1"),
    author_type: "user",
    content: "hello",
    type: "message",
    created_at: TS,
    ...overrides,
  } as Message;
}

describe("signatureOf", () => {
  it("returns equal signatures for messages with equal content fields", () => {
    const a = makeMessage({ content: "one", metadata: { foo: { bar: 1 } } });
    const b = makeMessage({ content: "one", metadata: { foo: { bar: 1 } } });
    expect(signatureOf(a)).toBe(signatureOf(b));
  });

  it("differs when content changes", () => {
    expect(signatureOf(makeMessage({ content: "one" }))).not.toBe(
      signatureOf(makeMessage({ content: "two" })),
    );
  });

  it("differs when metadata changes", () => {
    expect(signatureOf(makeMessage({ metadata: { a: 1 } }))).not.toBe(
      signatureOf(makeMessage({ metadata: { a: 2 } })),
    );
  });

  it("is stable regardless of metadata key insertion order", () => {
    const a = makeMessage({ metadata: { a: 1, b: 2 } });
    const b = makeMessage({ metadata: { b: 2, a: 1 } });
    expect(signatureOf(a)).toBe(signatureOf(b));
  });

  it("differs when turn_id or requests_input change", () => {
    const base = makeMessage({ content: "x" });
    expect(signatureOf(base)).not.toBe(signatureOf(makeMessage({ content: "x", turn_id: "t1" })));
    expect(signatureOf(base)).not.toBe(
      signatureOf(makeMessage({ content: "x", requests_input: true })),
    );
  });

  it("caches per object (same object returns identical signature)", () => {
    const m = makeMessage({ content: "cached" });
    expect(signatureOf(m)).toBe(signatureOf(m));
  });

  it("short-circuits to updated_at when present (authoritative signal)", () => {
    const a = makeMessage({ content: "x", updated_at: TS });
    const b = makeMessage({ content: "different", updated_at: TS });
    expect(signatureOf(a)).toBe(signatureOf(b));
  });

  it("differs when updated_at advances", () => {
    const a = makeMessage({ content: "x", updated_at: TS });
    const b = makeMessage({ content: "x", updated_at: "2024-01-01T00:00:01Z" });
    expect(signatureOf(a)).not.toBe(signatureOf(b));
  });

  it("falls back to content hash when updated_at is absent", () => {
    expect(signatureOf(makeMessage({ content: "one" }))).not.toBe(
      signatureOf(makeMessage({ content: "two" })),
    );
  });
});

describe("reconcileMessages", () => {
  it("returns the prev array unchanged when every message is signature-equal", () => {
    const prev = [
      makeMessage({ id: "a", content: "one" }),
      makeMessage({ id: "b", content: "two", metadata: { foo: { bar: 1 } } }),
    ];
    const next = [
      makeMessage({ id: "a", content: "one" }),
      makeMessage({ id: "b", content: "two", metadata: { foo: { bar: 1 } } }),
    ];
    expect(reconcileMessages(prev, next)).toBe(prev);
  });

  it("reuses prior references for unchanged messages and keeps the changed one fresh", () => {
    const prev = [
      makeMessage({ id: "a", content: "one" }),
      makeMessage({ id: "b", content: "two" }),
    ];
    const next = [
      makeMessage({ id: "a", content: "one" }),
      makeMessage({ id: "b", content: "two-edited" }),
    ];
    const result = reconcileMessages(prev, next);
    expect(result).not.toBe(prev);
    expect(result[0]).toBe(prev[0]);
    expect(result[1]).toBe(next[1]);
  });

  it("returns next when a message is appended, reusing existing references", () => {
    const prev = [makeMessage({ id: "a", content: "one" })];
    const next = [
      makeMessage({ id: "a", content: "one" }),
      makeMessage({ id: "b", content: "two" }),
    ];
    const result = reconcileMessages(prev, next);
    expect(result).not.toBe(prev);
    expect(result[0]).toBe(prev[0]);
    expect(result[1]).toBe(next[1]);
  });

  it("returns next directly when prev is empty or undefined", () => {
    const next = [makeMessage({ id: "a" })];
    expect(reconcileMessages([], next)).toBe(next);
    expect(reconcileMessages(undefined, next)).toBe(next);
  });

  it("detects a metadata-only change", () => {
    const prev = [makeMessage({ id: "a", content: "one", metadata: { x: 1 } })];
    const next = [makeMessage({ id: "a", content: "one", metadata: { x: 2 } })];
    const result = reconcileMessages(prev, next);
    expect(result).not.toBe(prev);
    expect(result[0]).toBe(next[0]);
  });
});
