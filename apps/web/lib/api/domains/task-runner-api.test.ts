import { beforeEach, describe, it, expect, vi } from "vitest";

const getWebSocketClientMock = vi.hoisted(() => vi.fn());

vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: getWebSocketClientMock,
}));

import { switchTaskRunner } from "./task-runner-api";

beforeEach(() => {
  getWebSocketClientMock.mockReset();
});

describe("switchTaskRunner", () => {
  it("sends id and executor_profile_id through task.runner", async () => {
    const request = vi.fn().mockResolvedValue({ id: "task-1", runner_editable: false });
    getWebSocketClientMock.mockReturnValue({ request });

    const result = await switchTaskRunner("task-1", "profile-2");

    expect(request).toHaveBeenCalledWith("task.runner", {
      id: "task-1",
      executor_profile_id: "profile-2",
    });
    expect(result).toEqual({ id: "task-1", runner_editable: false });
  });

  it("throws when no WebSocket client is connected", async () => {
    getWebSocketClientMock.mockReturnValue(null);

    await expect(switchTaskRunner("task-1", "profile-2")).rejects.toThrow(
      "WebSocket client not available",
    );
  });

  it("propagates a rejection from the WebSocket client", async () => {
    const request = vi.fn().mockRejectedValue({
      code: "CONFLICT",
      message: "The runner cannot be changed right now.",
      details: { error_code: "session_exists" },
    });
    getWebSocketClientMock.mockReturnValue({ request });

    await expect(switchTaskRunner("task-1", "profile-2")).rejects.toMatchObject({
      code: "CONFLICT",
      details: { error_code: "session_exists" },
    });
  });
});
