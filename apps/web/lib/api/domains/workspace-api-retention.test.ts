import { describe, expect, it } from "vitest";
import { ApiError } from "../client";
import { getQuickChatRetainedSessionFromError } from "./workspace-api";

describe("retained Quick Chat error identity", () => {
  it("reads the server's typed retained identity", () => {
    const error = new ApiError("failed to start session", 500, {
      task_id: "task-1",
      session_id: "session-1",
    });
    expect(getQuickChatRetainedSessionFromError(error)).toEqual({
      taskId: "task-1",
      sessionId: "session-1",
    });
  });

  it.each([
    undefined,
    {},
    { task_id: "task-1" },
    { task_id: "task-1", session_id: "" },
    { task_id: 1, session_id: "session-1" },
  ])("rejects incomplete or malformed identity: %j", (body) => {
    expect(getQuickChatRetainedSessionFromError(new ApiError("failed", 500, body))).toBeNull();
  });

  it("does not infer retained identity from an arbitrary exception", () => {
    expect(
      getQuickChatRetainedSessionFromError({
        body: { task_id: "task-1", session_id: "session-1" },
      }),
    ).toBeNull();
  });
});
