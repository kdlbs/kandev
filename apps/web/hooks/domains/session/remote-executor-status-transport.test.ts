import { afterEach, describe, expect, it, vi } from "vitest";
import {
  createRemoteExecutorStatusResource,
  remoteExecutorStatusScope,
} from "./remote-executor-status-resource";

const mocks = vi.hoisted(() => ({
  getStatus: vi.fn(() => "disconnected"),
  request: vi.fn().mockResolvedValue({ remote_state: "running" }),
}));
vi.mock("@/lib/ws/connection", () => ({ getWebSocketClient: () => mocks }));

afterEach(() => {
  vi.useRealTimers();
  vi.clearAllMocks();
});

describe("remote executor status transport", () => {
  it("does not queue offline polling reads and resumes after reconnect", async () => {
    vi.useFakeTimers();
    const request = {
      executorType: "sprites",
      executorId: "executor",
      taskId: "task",
      sessionId: "session",
    };
    const resource = createRemoteExecutorStatusResource();
    const scope = remoteExecutorStatusScope(request);
    const unsubscribe = resource.subscribe(scope, () => undefined);
    try {
      resource.load(request);
      await vi.advanceTimersByTimeAsync(270_000);
      expect(mocks.request).not.toHaveBeenCalled();
      expect(resource.getSnapshot(scope)).toEqual({ status: null, loading: false });

      mocks.getStatus.mockReturnValue("connected");
      await vi.advanceTimersByTimeAsync(90_000);
      expect(mocks.request).toHaveBeenCalledTimes(1);
      expect(resource.getSnapshot(scope).status?.remote_state).toBe("running");
    } finally {
      unsubscribe();
    }
  });
});
