import { afterEach, describe, expect, it, vi } from "vitest";
import {
  createRemoteExecutorStatusResource,
  remoteExecutorStatusScope,
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

// @covers AC-EXECUTORS-TASK-STATUS-001.1 and AC-EXECUTORS-TASK-STATUS-001.2
describe("mounted status refresh", () => {
  it("refreshes shared consumers without interaction and stops after the last unsubscribe", async () => {
    vi.useFakeTimers();
    const requester = vi
      .fn()
      .mockResolvedValueOnce(healthyStatus("old"))
      .mockResolvedValue(healthyStatus("new"));
    const resource = createRemoteExecutorStatusResource(requester);
    const scope = remoteExecutorStatusScope(REQUEST);
    const first = resource.subscribe(scope, () => undefined);
    const second = resource.subscribe(scope, () => undefined);
    try {
      await resource.load(REQUEST);
      first();
      await vi.advanceTimersByTimeAsync(90_000);
      expect(resource.getSnapshot(scope).status?.remote_name).toBe("new");
      expect(requester).toHaveBeenCalledTimes(2);
      second();
      await vi.advanceTimersByTimeAsync(180_000);
      expect(requester).toHaveBeenCalledTimes(2);
    } finally {
      first();
      second();
    }
  });

  it("retries unavailable transport and failed reads automatically", async () => {
    vi.useFakeTimers();
    const requester = vi
      .fn()
      .mockReturnValueOnce(null)
      .mockRejectedValueOnce(new Error("secret"))
      .mockResolvedValue(healthyStatus("recovered"));
    const resource = createRemoteExecutorStatusResource(requester);
    const scope = remoteExecutorStatusScope(REQUEST);
    const stop = resource.subscribe(scope, () => undefined);
    try {
      await resource.load(REQUEST);
      await vi.advanceTimersByTimeAsync(90_000);
      expect(resource.getSnapshot(scope).status?.remote_status_error).toBe(
        "Remote executor status is unavailable.",
      );
      await vi.advanceTimersByTimeAsync(90_000);
      expect(resource.getSnapshot(scope).status?.remote_name).toBe("recovered");
    } finally {
      stop();
    }
  });

  it("pauses hidden documents and refreshes expired scopes on return", async () => {
    vi.useFakeTimers();
    const requester = vi.fn().mockResolvedValue(healthyStatus("pod"));
    const resource = createRemoteExecutorStatusResource(requester);
    const stop = resource.subscribe(remoteExecutorStatusScope(REQUEST), () => undefined);
    const visibility = vi.spyOn(document, "visibilityState", "get");
    try {
      await resource.load(REQUEST);
      visibility.mockReturnValue("hidden");
      document.dispatchEvent(new Event("visibilitychange"));
      await vi.advanceTimersByTimeAsync(180_000);
      expect(requester).toHaveBeenCalledTimes(1);
      visibility.mockReturnValue("visible");
      document.dispatchEvent(new Event("visibilitychange"));
      await vi.advanceTimersByTimeAsync(0);
      expect(requester).toHaveBeenCalledTimes(2);
    } finally {
      stop();
      visibility.mockRestore();
    }
  });
});

describe("refresh lifecycle cleanup", () => {
  it("joins an in-flight refresh and resets the schedule after settlement", async () => {
    vi.useFakeTimers();
    let resolve!: (value: RemoteExecutorStatusData) => void;
    const requester = vi
      .fn()
      .mockResolvedValueOnce(healthyStatus("initial"))
      .mockImplementationOnce(
        () =>
          new Promise<RemoteExecutorStatusData>((done) => {
            resolve = done;
          }),
      )
      .mockResolvedValue(healthyStatus("latest"));
    const resource = createRemoteExecutorStatusResource(requester);
    const scope = remoteExecutorStatusScope(REQUEST);
    const stop = resource.subscribe(scope, () => undefined);
    try {
      await resource.load(REQUEST);
      await vi.advanceTimersByTimeAsync(90_000);
      const pending = resource.load(REQUEST, true);
      expect(requester).toHaveBeenCalledTimes(2);
      expect(resource.getSnapshot(scope).status?.remote_name).toBe("initial");
      await vi.advanceTimersByTimeAsync(180_000);
      expect(requester).toHaveBeenCalledTimes(2);
      resolve(healthyStatus("updated"));
      await pending;
      await vi.advanceTimersByTimeAsync(89_999);
      expect(requester).toHaveBeenCalledTimes(2);
      await vi.advanceTimersByTimeAsync(1);
      expect(resource.getSnapshot(scope).status?.remote_name).toBe("latest");
    } finally {
      stop();
    }
  });

  it("preserves fresh results on visibility return and detaches its listener on cleanup", async () => {
    vi.useFakeTimers();
    const requester = vi.fn().mockResolvedValue(healthyStatus("pod"));
    const resource = createRemoteExecutorStatusResource(requester);
    const stop = resource.subscribe(remoteExecutorStatusScope(REQUEST), () => undefined);
    const visibility = vi.spyOn(document, "visibilityState", "get");
    const removed = vi.spyOn(document, "removeEventListener");
    try {
      await resource.load(REQUEST);
      visibility.mockReturnValue("hidden");
      document.dispatchEvent(new Event("visibilitychange"));
      await vi.advanceTimersByTimeAsync(30_000);
      visibility.mockReturnValue("visible");
      document.dispatchEvent(new Event("visibilitychange"));
      await vi.advanceTimersByTimeAsync(59_999);
      expect(requester).toHaveBeenCalledTimes(1);
      await vi.advanceTimersByTimeAsync(1);
      expect(requester).toHaveBeenCalledTimes(2);
      stop();
      expect(removed).toHaveBeenCalledWith("visibilitychange", expect.any(Function));
    } finally {
      stop();
      visibility.mockRestore();
      removed.mockRestore();
    }
  });

  it("does not restart polling when an abandoned read settles", async () => {
    vi.useFakeTimers();
    let resolve!: (value: RemoteExecutorStatusData) => void;
    const requester = vi.fn(
      () =>
        new Promise<RemoteExecutorStatusData>((done) => {
          resolve = done;
        }),
    );
    const resource = createRemoteExecutorStatusResource(requester);
    const stop = resource.subscribe(remoteExecutorStatusScope(REQUEST), () => undefined);
    const pending = resource.load(REQUEST);
    stop();
    resolve(healthyStatus("old scope"));
    await pending;
    await vi.advanceTimersByTimeAsync(180_000);
    expect(requester).toHaveBeenCalledTimes(1);
  });
});
