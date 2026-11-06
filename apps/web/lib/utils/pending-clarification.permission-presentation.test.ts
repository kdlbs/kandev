import { describe, expect, it } from "vitest";
import { sessionId as toSessionId, taskId as toTaskId, type Message } from "@/lib/types/http";
import { hasPendingPermissionRequest } from "./pending-clarification";

function message(overrides: Partial<Message>): Message {
  return {
    id: "message-1",
    session_id: toSessionId("session-1"),
    task_id: toTaskId("task-1"),
    author_type: "agent",
    content: "",
    type: "message",
    created_at: "2026-10-29T12:00:00Z",
    ...overrides,
  } as Message;
}

describe("hasPendingPermissionRequest presentation scope", () => {
  it("does not raise input attention for an automatic pending row", () => {
    expect(
      hasPendingPermissionRequest([
        message({
          type: "permission_request",
          metadata: { status: "pending", permission_presentation: "automatic" },
        }),
      ]),
    ).toBe(false);
  });

  it("does not raise input attention for an unavailable row", () => {
    expect(
      hasPendingPermissionRequest([
        message({
          type: "permission_request",
          metadata: { status: "pending", permission_presentation: "unavailable" },
        }),
      ]),
    ).toBe(false);
  });

  it("keeps an explicitly interactive row actionable", () => {
    expect(
      hasPendingPermissionRequest([
        message({
          type: "permission_request",
          metadata: { status: "pending", permission_presentation: "interactive" },
        }),
      ]),
    ).toBe(true);
  });

  it("does not resurrect an older manual row behind a newer automatic row", () => {
    expect(
      hasPendingPermissionRequest([
        message({ id: "manual", type: "permission_request", metadata: { status: "pending" } }),
        message({
          id: "automatic",
          type: "permission_request",
          metadata: { status: "pending", permission_presentation: "automatic" },
        }),
      ]),
    ).toBe(false);
  });
});
