import { describe, expect, it } from "vitest";
import type { Message } from "@/lib/types/http";
import { hasPendingClarification } from "./pending-clarification";

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
});
