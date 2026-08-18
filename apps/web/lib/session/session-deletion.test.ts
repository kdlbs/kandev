import { describe, expect, it } from "vitest";
import { resolveSessionDeletionTarget } from "./session-deletion";

describe("resolveSessionDeletionTarget", () => {
  it("routes a Quick Chat member through its backing task", () => {
    expect(
      resolveSessionDeletionTarget("session-1", "fallback-task", [
        { sessionId: "session-1", taskId: "task-1", workspaceId: "workspace-1", kind: "chat" },
      ]),
    ).toEqual({ kind: "quick-chat-task", taskId: "task-1" });
  });

  it("routes an ordinary session through session deletion", () => {
    expect(resolveSessionDeletionTarget("session-1", "task-1", [])).toEqual({
      kind: "session",
      sessionId: "session-1",
    });
  });
});
