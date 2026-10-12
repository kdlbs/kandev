import { describe, expect, it, vi } from "vitest";
import type { ApiClient } from "./api-client";
import { recoverySnapshot } from "./delivery-record-recovery";

vi.mock("@playwright/test", () => ({ expect: vi.fn() }));

function recoveryApi(taskId = "task-recovery") {
  return {
    listTaskSessions: vi.fn().mockResolvedValue({
      sessions: [{ id: "session-recovery", task_id: "task-recovery" }],
    }),
    getTaskSession: vi.fn().mockResolvedValue({
      session: {
        id: "session-recovery",
        task_id: taskId,
        metadata: { agent_delivery_recovery: { revision: 1 } },
      },
    }),
    listSessionMessages: vi.fn().mockResolvedValue({
      messages: [
        { id: "saved-prompt", author_type: "user" },
        { id: "assistant-reply", author_type: "agent" },
      ],
    }),
  };
}

describe("delivery record recovery snapshot", () => {
  it("retains recovery revision when session membership is compact", async () => {
    const api = recoveryApi();
    await expect(
      recoverySnapshot(api as unknown as ApiClient, "task-recovery", "session-recovery"),
    ).resolves.toEqual({ revision: 1, messageCount: 1 });
  });

  it("rejects a detailed session belonging to a different task", async () => {
    const api = recoveryApi("other-task");
    await expect(
      recoverySnapshot(api as unknown as ApiClient, "task-recovery", "session-recovery"),
    ).rejects.toThrow("task task-recovery no longer has session session-recovery");
  });
});
