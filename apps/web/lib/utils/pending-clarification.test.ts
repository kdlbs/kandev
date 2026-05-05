import { describe, expect, it } from "vitest";
import type { Message } from "@/lib/types/http";
import { findPendingClarificationGroup, hasPendingClarification } from "./pending-clarification";

function message(overrides: Partial<Message>): Message {
  return {
    id: "msg-1",
    session_id: "session-1",
    task_id: "task-1",
    author_type: "agent",
    content: "",
    type: "message",
    created_at: "2026-05-02T00:00:00Z",
    ...overrides,
  };
}

describe("hasPendingClarification", () => {
  it("detects clarification requests with pending status", () => {
    expect(
      hasPendingClarification([
        message({
          type: "clarification_request",
          metadata: { status: "pending" },
        }),
      ]),
    ).toBe(true);
  });

  it("treats missing clarification status as pending", () => {
    expect(hasPendingClarification([message({ type: "clarification_request" })])).toBe(true);
  });

  it("ignores answered clarification requests and ordinary messages", () => {
    expect(
      hasPendingClarification([
        message({ type: "message" }),
        message({
          type: "clarification_request",
          metadata: { status: "answered" },
        }),
      ]),
    ).toBe(false);
  });

  it("treats rejected and expired clarifications as not pending", () => {
    expect(
      hasPendingClarification([
        message({
          type: "clarification_request",
          metadata: { status: "rejected" },
        }),
      ]),
    ).toBe(false);
    expect(
      hasPendingClarification([
        message({
          type: "clarification_request",
          metadata: { status: "expired" },
        }),
      ]),
    ).toBe(false);
  });
});

describe("findPendingClarificationGroup", () => {
  it("returns empty array when there are no messages", () => {
    expect(findPendingClarificationGroup([])).toEqual([]);
    expect(findPendingClarificationGroup(null)).toEqual([]);
    expect(findPendingClarificationGroup(undefined)).toEqual([]);
  });

  it("returns empty array when there is no pending clarification", () => {
    expect(
      findPendingClarificationGroup([
        message({ type: "clarification_request", metadata: { status: "answered" } }),
      ]),
    ).toEqual([]);
  });

  it("returns the single message when no pending_id is set", () => {
    const msg = message({ type: "clarification_request" });
    expect(findPendingClarificationGroup([msg])).toEqual([msg]);
  });

  it("returns every message that shares the latest pending_id", () => {
    const a = message({
      id: "a",
      type: "clarification_request",
      metadata: { pending_id: "p1", question_total: 3, question_index: 0, status: "pending" },
    });
    const b = message({
      id: "b",
      type: "clarification_request",
      metadata: { pending_id: "p1", question_total: 3, question_index: 1, status: "pending" },
    });
    const c = message({
      id: "c",
      type: "clarification_request",
      metadata: { pending_id: "p1", question_total: 3, question_index: 2, status: "pending" },
    });
    const noise = message({ id: "n", type: "message" });
    expect(findPendingClarificationGroup([noise, a, b, c]).map((m) => m.id)).toEqual([
      "a",
      "b",
      "c",
    ]);
  });

  it("gates on question_total — returns [] when not all messages have arrived", () => {
    // Backend declared 3 questions but only 1 has streamed in; the overlay
    // should stay hidden until the remaining 2 land.
    const a = message({
      id: "a",
      type: "clarification_request",
      metadata: { pending_id: "p1", question_total: 3, question_index: 0, status: "pending" },
    });
    expect(findPendingClarificationGroup([a])).toEqual([]);
  });

  it("ignores messages from a different pending bundle", () => {
    const old = message({
      id: "old",
      type: "clarification_request",
      metadata: { pending_id: "p0", status: "answered" },
    });
    const a = message({
      id: "a",
      type: "clarification_request",
      metadata: { pending_id: "p1", question_total: 1, status: "pending" },
    });
    expect(findPendingClarificationGroup([old, a]).map((m) => m.id)).toEqual(["a"]);
  });
});
