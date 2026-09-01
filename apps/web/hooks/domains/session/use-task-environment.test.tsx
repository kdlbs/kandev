import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { TaskEnvironment } from "@/lib/api/domains/task-environment-api";
import type { KubernetesSession } from "@/lib/types/http-kubernetes";

const mocks = vi.hoisted(() => ({
  fetchTaskEnvironmentLive: vi.fn(),
  getKubernetesTaskSession: vi.fn(),
  resetTaskEnvironment: vi.fn(),
}));

vi.mock("@/lib/api/domains/task-environment-api", () => ({
  fetchTaskEnvironmentLive: mocks.fetchTaskEnvironmentLive,
  resetTaskEnvironment: mocks.resetTaskEnvironment,
}));

vi.mock("@/lib/api/domains/kubernetes-api", () => ({
  getKubernetesTaskSession: mocks.getKubernetesTaskSession,
}));

vi.mock("@/lib/toast/sonner", () => ({
  toast: { success: vi.fn(), error: vi.fn() },
}));

import { useTaskEnvironment } from "./use-task-environment";

const TASK_ONE = "task-1";
const SESSION_ONE = "session-1";
const EXECUTOR_ONE = "executor-1";
const POD_ONE = "kandev-pod-1";

const useEnvironmentWithSession = useTaskEnvironment as unknown as (
  taskId: string,
  sessionId: string,
  active: boolean,
) => ReturnType<typeof useTaskEnvironment> & {
  kubernetes: KubernetesSession | null;
  kubernetesError: string | null;
  refreshing: boolean;
};

const ENVIRONMENT: TaskEnvironment = {
  id: "environment-1",
  task_id: TASK_ONE,
  repository_id: "repository-1",
  executor_type: "k8s",
  executor_id: EXECUTOR_ONE,
  executor_profile_id: "profile-1",
  agent_execution_id: "execution-1",
  control_port: 8765,
  status: "ready",
  created_at: "2026-08-25T09:30:00Z",
};

const KUBERNETES_SESSION: KubernetesSession = {
  session_id: SESSION_ONE,
  task_id: TASK_ONE,
  pod_name: POD_ONE,
  pod_phase: "Running",
  container_state: "running",
  restarts: 0,
  workspace_kind: "empty_dir",
};

function deferred<T>() {
  let resolve: (value: T) => void = () => undefined;
  const promise = new Promise<T>((resolvePromise) => {
    resolve = resolvePromise;
  });
  return { promise, resolve };
}

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

describe("useTaskEnvironment Kubernetes status", () => {
  it("loads the exact active task session and publishes its live status", async () => {
    mocks.fetchTaskEnvironmentLive.mockResolvedValue({ environment: ENVIRONMENT });
    mocks.getKubernetesTaskSession.mockResolvedValue(KUBERNETES_SESSION);

    const { result } = renderHook(() => useEnvironmentWithSession(TASK_ONE, SESSION_ONE, true));

    await waitFor(() => {
      expect(mocks.getKubernetesTaskSession).toHaveBeenCalledWith(
        EXECUTOR_ONE,
        TASK_ONE,
        SESSION_ONE,
      );
    });
    expect(result.current.kubernetes).toEqual(KUBERNETES_SESSION);
    expect(result.current.kubernetesError).toBeNull();
    expect(result.current.status).toEqual({ label: "Running", tone: "running" });
  });

  it("does not publish a late Kubernetes response after the active session changes", async () => {
    let resolveFirst: (session: KubernetesSession | null) => void = () => undefined;
    const first = new Promise<KubernetesSession | null>((resolve) => {
      resolveFirst = resolve;
    });
    const secondSession: KubernetesSession = {
      ...KUBERNETES_SESSION,
      task_id: "task-2",
      session_id: "session-2",
      pod_name: "kandev-pod-2",
    };
    mocks.fetchTaskEnvironmentLive.mockImplementation(async (taskId: string) => ({
      environment: {
        ...ENVIRONMENT,
        task_id: taskId,
        executor_id: taskId === TASK_ONE ? EXECUTOR_ONE : "executor-2",
      },
    }));
    mocks.getKubernetesTaskSession.mockImplementation((executorId: string) =>
      executorId === EXECUTOR_ONE ? first : Promise.resolve(secondSession),
    );
    const { result, rerender } = renderHook(
      ({ taskId, sessionId }) => useEnvironmentWithSession(taskId, sessionId, true),
      { initialProps: { taskId: TASK_ONE, sessionId: SESSION_ONE } },
    );
    await waitFor(() => expect(mocks.getKubernetesTaskSession).toHaveBeenCalledTimes(1));

    rerender({ taskId: "task-2", sessionId: "session-2" });
    await waitFor(() => expect(result.current.kubernetes?.pod_name).toBe("kandev-pod-2"));
    await act(async () => resolveFirst(KUBERNETES_SESSION));

    expect(result.current.kubernetes?.pod_name).toBe("kandev-pod-2");
    expect(result.current.env?.task_id).toBe("task-2");
  });

  it("publishes a generic status error without exposing the request failure", async () => {
    mocks.fetchTaskEnvironmentLive.mockResolvedValue({ environment: ENVIRONMENT });
    mocks.getKubernetesTaskSession.mockRejectedValue(
      new Error("GET https://cluster.invalid?token=secret failed"),
    );

    const { result } = renderHook(() => useEnvironmentWithSession(TASK_ONE, SESSION_ONE, true));

    await waitFor(() => expect(result.current.kubernetesError).not.toBeNull());
    expect(result.current.kubernetesError).toBe("Failed to load Kubernetes sessions.");
    expect(result.current.kubernetesError).not.toContain("cluster.invalid");
    expect(result.current.status).toEqual({ label: "Error", tone: "error" });
  });
});

describe("useTaskEnvironment explicit refresh", () => {
  it("reports a distinct busy state while an explicit refresh keeps the last Pod visible", async () => {
    mocks.fetchTaskEnvironmentLive.mockResolvedValueOnce({ environment: ENVIRONMENT });
    mocks.getKubernetesTaskSession.mockResolvedValue(KUBERNETES_SESSION);
    const { result } = renderHook(() => useEnvironmentWithSession(TASK_ONE, SESSION_ONE, true));
    await waitFor(() => expect(result.current.kubernetes?.pod_name).toBe(POD_ONE));

    const refreshResponse = deferred<{ environment: TaskEnvironment }>();
    mocks.fetchTaskEnvironmentLive.mockImplementationOnce(() => refreshResponse.promise);
    let refreshPromise: Promise<void> = Promise.resolve();
    act(() => {
      refreshPromise = result.current.refresh();
    });

    expect(result.current.refreshing).toBe(true);
    expect(result.current.loading).toBe(false);
    expect(result.current.kubernetes?.pod_name).toBe(POD_ONE);

    await act(async () => {
      refreshResponse.resolve({ environment: ENVIRONMENT });
      await refreshPromise;
    });
    expect(result.current.refreshing).toBe(false);
  });

  it("makes a duplicate refresh await the exact request already in flight", async () => {
    mocks.fetchTaskEnvironmentLive.mockResolvedValueOnce({ environment: ENVIRONMENT });
    mocks.getKubernetesTaskSession.mockResolvedValue(KUBERNETES_SESSION);
    const { result } = renderHook(() => useEnvironmentWithSession(TASK_ONE, SESSION_ONE, true));
    await waitFor(() => expect(result.current.kubernetes?.pod_name).toBe(POD_ONE));

    const refreshResponse = deferred<{ environment: TaskEnvironment }>();
    mocks.fetchTaskEnvironmentLive.mockImplementationOnce(() => refreshResponse.promise);
    let firstRefresh: Promise<void> = Promise.resolve();
    let joinedRefresh: Promise<void> = Promise.resolve();
    let joinedSettled = false;
    act(() => {
      firstRefresh = result.current.refresh();
      joinedRefresh = result.current.refresh();
      void joinedRefresh.then(() => {
        joinedSettled = true;
      });
    });
    await act(async () => Promise.resolve());

    expect(mocks.fetchTaskEnvironmentLive).toHaveBeenCalledTimes(2);
    expect(joinedSettled).toBe(false);

    await act(async () => {
      refreshResponse.resolve({ environment: ENVIRONMENT });
      await Promise.all([firstRefresh, joinedRefresh]);
    });
    expect(joinedSettled).toBe(true);
  });
});
