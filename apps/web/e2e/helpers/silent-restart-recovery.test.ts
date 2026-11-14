import { describe, expect, it, vi } from "vitest";
import type { ApiClient } from "./api-client";
import {
  captureSilentRestartSnapshot,
  expectRestartConversationIdentity,
} from "./silent-restart-recovery";

const playwrightExpect = vi.hoisted(() => Object.assign(vi.fn(), { any: vi.fn(), poll: vi.fn() }));
vi.mock("@playwright/test", () => ({ expect: playwrightExpect }));

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

describe("restart conversation identity", () => {
  it("preserves a recovery notice turn when it is already in the comparison baseline", async () => {
    const arrayEqualCalls: Array<{ actual: unknown[]; expected: unknown[] }> = [];
    playwrightExpect.mockReset();
    playwrightExpect.poll.mockReset();
    playwrightExpect.mockImplementation((actual: unknown) => ({
      toEqual: (expected: unknown) => {
        if (Array.isArray(actual) && Array.isArray(expected)) {
          arrayEqualCalls.push({ actual, expected });
        }
      },
      toBe: vi.fn(),
      toBeTruthy: vi.fn(),
      toContain: vi.fn(),
      toHaveLength: vi.fn(),
      toHaveCount: vi.fn(),
      toMatchObject: vi.fn(),
      not: { toBe: vi.fn() },
    }));
    playwrightExpect.poll.mockImplementation(() => ({ toBe: vi.fn(), toEqual: vi.fn() }));

    const snapshot = {
      sessionId: "session-restart",
      nativeSessionId: "native-restart",
      agentExecutionId: "execution-restart",
      taskEnvironmentId: "environment-restart",
      messageIds: ["notice-message"],
      turnIds: ["turn-before", "turn-notice"],
      userMessages: [],
      queuedMessageCount: 0,
    };
    const api = {
      getTaskSession: vi.fn().mockResolvedValue({
        session: {
          id: snapshot.sessionId,
          task_id: "task-restart",
          state: "WAITING_FOR_INPUT",
          metadata: { acp: { session_id: snapshot.nativeSessionId } },
          agent_execution_id: snapshot.agentExecutionId,
          task_environment_id: snapshot.taskEnvironmentId,
        },
      }),
      listSessionMessages: vi.fn().mockResolvedValue({
        messages: [
          {
            id: "notice-message",
            author_type: "agent",
            turn_id: "turn-notice",
            metadata: { kind: "session_restored_after_interruption" },
          },
        ],
      }),
      listSessionTurns: vi.fn().mockResolvedValue({
        turns: [
          { id: "turn-before" },
          {
            id: "turn-notice",
            completed_at: "2026-11-12T10:00:00Z",
            metadata: { lifecycle_only: true },
          },
        ],
      }),
      getQueueSessionIdentity: vi.fn().mockResolvedValue({
        taskId: "task-restart",
        sessionId: snapshot.sessionId,
        sessionIncarnationId: "incarnation-restart",
      }),
      getQueueStatus: vi.fn().mockResolvedValue({ count: 0 }),
    };
    const page = { getByTestId: vi.fn(() => ({ toHaveCount: vi.fn() })) };

    await expectRestartConversationIdentity({
      page: page as never,
      api: api as unknown as ApiClient,
      taskId: "task-restart",
      snapshot,
      traffic: [],
      expectedState: "WAITING_FOR_INPUT",
      executionDisposition: "preserved",
    });

    const turnIdentityAssertion = arrayEqualCalls.find(
      ({ expected }) => expected[0] === "turn-before" && expected[1] === "turn-notice",
    );
    expect(turnIdentityAssertion?.actual).toEqual(["turn-before", "turn-notice"]);
  });
});
