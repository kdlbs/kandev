import { describe, expect, it, vi } from "vitest";
import type { ApiClient } from "./api-client";
import { captureSilentRestartSnapshot } from "./silent-restart-recovery";

vi.mock("@playwright/test", () => ({ expect: vi.fn() }));

function snapshotApi(taskId = "task-restart") {
  return {
    listTaskSessions: vi.fn().mockResolvedValue({
      sessions: [{ id: "session-restart", task_id: "task-restart", state: "RUNNING" }],
    }),
    getTaskSession: vi.fn().mockResolvedValue({
      session: {
        id: "session-restart",
        task_id: taskId,
        state: "RUNNING",
        metadata: { acp: { session_id: "native-restart" } },
        agent_execution_id: "execution-restart",
        task_environment_id: "environment-restart",
      },
    }),
    listSessionMessages: vi.fn().mockResolvedValue({
      messages: [
        {
          id: "message-restart",
          author_type: "user",
          content: "Keep this turn",
          turn_id: "turn-restart",
        },
      ],
    }),
    listSessionTurns: vi.fn().mockResolvedValue({ turns: [{ id: "turn-restart" }] }),
    getQueueSessionIdentity: vi.fn().mockResolvedValue({
      taskId: "task-restart",
      sessionId: "session-restart",
      sessionIncarnationId: "incarnation-restart",
    }),
    getQueueStatus: vi.fn().mockResolvedValue({ count: 2 }),
  };
}

describe("silent restart snapshot", () => {
  it("retains native identity and history when session membership is compact", async () => {
    const api = snapshotApi();
    await expect(
      captureSilentRestartSnapshot(api as unknown as ApiClient, "task-restart", "session-restart"),
    ).resolves.toEqual({
      sessionId: "session-restart",
      nativeSessionId: "native-restart",
      agentExecutionId: "execution-restart",
      taskEnvironmentId: "environment-restart",
      messageIds: ["message-restart"],
      turnIds: ["turn-restart"],
      userMessages: [{ id: "message-restart", content: "Keep this turn", turn_id: "turn-restart" }],
      queuedMessageCount: 2,
    });
  });

  it("rejects a detailed session belonging to a different task", async () => {
    const api = snapshotApi("other-task");
    await expect(
      captureSilentRestartSnapshot(api as unknown as ApiClient, "task-restart", "session-restart"),
    ).rejects.toThrow("task task-restart no longer has session session-restart");
  });
});
