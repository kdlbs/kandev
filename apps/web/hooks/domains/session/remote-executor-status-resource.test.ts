import { afterEach, describe, expect, it, vi } from "vitest";
import {
  createRemoteExecutorStatusResource,
  type RemoteExecutorStatusData,
  type RemoteExecutorStatusRequest,
} from "./remote-executor-status-resource";

const REQUEST: RemoteExecutorStatusRequest = {
  executorId: "executor-1",
  executorType: "k8s",
  taskId: "task-1",
  sessionId: "session-1",
};
const RECOVERED_POD_NAME = "pod-recovered";

function healthyStatus(name: string): RemoteExecutorStatusData {
  return {
    remote_name: name,
    remote_state: "running",
    remote_checked_at: new Date().toISOString(),
  };
}

afterEach(() => vi.useRealTimers());

describe("remote executor status resource", () => {
  it("reuses a successful result for 90 seconds and refreshes after it expires", async () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date("2026-09-01T10:00:00Z"));
    const requester = vi
      .fn<(request: RemoteExecutorStatusRequest) => Promise<RemoteExecutorStatusData>>()
      .mockResolvedValueOnce(healthyStatus("pod-1"))
      .mockResolvedValueOnce(healthyStatus("pod-2"));
    const resource = createRemoteExecutorStatusResource(requester);

    await resource.load(REQUEST);
    vi.advanceTimersByTime(89_999);
    await resource.load(REQUEST);
    expect(requester).toHaveBeenCalledTimes(1);

    vi.advanceTimersByTime(2);
    await resource.load(REQUEST);
    expect(requester).toHaveBeenCalledTimes(2);
  });

  it("keeps a failed result visible but retries it on the next load", async () => {
    const requester = vi
      .fn<(request: RemoteExecutorStatusRequest) => Promise<RemoteExecutorStatusData>>()
      .mockRejectedValueOnce(new Error("expired credential"))
      .mockResolvedValueOnce(healthyStatus(RECOVERED_POD_NAME));
    const resource = createRemoteExecutorStatusResource(requester);

    const failed = await resource.load(REQUEST);
    expect(failed?.status?.remote_status_error).toBe("Remote executor status is unavailable.");

    const recovered = await resource.load(REQUEST);
    expect(requester).toHaveBeenCalledTimes(2);
    expect(recovered?.status?.remote_name).toBe(RECOVERED_POD_NAME);
  });

  it("retries a sanitized failed-status payload on the next load", async () => {
    const requester = vi
      .fn<(request: RemoteExecutorStatusRequest) => Promise<RemoteExecutorStatusData>>()
      .mockResolvedValueOnce({
        ...healthyStatus("pod-1"),
        remote_status_error: "Unauthorized",
      })
      .mockResolvedValueOnce(healthyStatus(RECOVERED_POD_NAME));
    const resource = createRemoteExecutorStatusResource(requester);

    const failed = await resource.load(REQUEST);
    expect(failed?.status?.remote_status_error).toBe("Unauthorized");

    const recovered = await resource.load(REQUEST);
    expect(requester).toHaveBeenCalledTimes(2);
    expect(recovered?.status?.remote_name).toBe(RECOVERED_POD_NAME);
  });

  it("replaces non-Kubernetes backend diagnostics with translated safe copy", async () => {
    const requester = vi.fn(async () => ({
      ...healthyStatus("sprite-1"),
      remote_status_error: "/home/operator/.config/provider?token=secret",
    }));
    const resource = createRemoteExecutorStatusResource(requester);

    const failed = await resource.load({ ...REQUEST, executorType: "sprites" });

    expect(failed?.status?.remote_status_error).toBe("Remote executor status is unavailable.");
  });

  it("does not issue malformed Kubernetes requests", () => {
    const requester = vi.fn();
    const resource = createRemoteExecutorStatusResource(requester);

    expect(resource.load({ ...REQUEST, executorId: null })).toBeNull();
    expect(resource.load({ ...REQUEST, taskId: "" })).toBeNull();
    expect(resource.load({ ...REQUEST, sessionId: "" })).toBeNull();
    expect(requester).not.toHaveBeenCalled();
  });

  it("evicts the least recently used inactive result after 128 scopes", async () => {
    const requester = vi.fn(async (request: RemoteExecutorStatusRequest) =>
      healthyStatus(request.taskId),
    );
    const resource = createRemoteExecutorStatusResource(requester);

    for (let index = 0; index < 129; index += 1) {
      await resource.load({
        ...REQUEST,
        taskId: `task-${index}`,
        sessionId: `session-${index}`,
      });
    }
    expect(requester).toHaveBeenCalledTimes(129);

    await resource.load({ ...REQUEST, taskId: "task-0", sessionId: "session-0" });
    expect(requester).toHaveBeenCalledTimes(130);
  });
});
