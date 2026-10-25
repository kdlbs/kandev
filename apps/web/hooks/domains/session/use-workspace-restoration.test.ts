import { act, renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { restoreSessionWorkspace } from "@/lib/services/session-recovery-service";
import {
  beginWorkspaceRestoration as beginAttempt,
  clearWorkspaceRestoration as clearAttempt,
  completeWorkspaceRestoration as completeAttempt,
  failWorkspaceRestoration as failAttempt,
  type WorkspaceRestorationState,
} from "@/lib/state/slices/session-runtime/workspace-restoration";
import { useWorkspaceRestoration } from "./use-workspace-restoration";

vi.mock("@/lib/services/session-recovery-service", () => ({
  restoreSessionWorkspace: vi.fn(),
}));

const mockRestoreSessionWorkspace = vi.mocked(restoreSessionWorkspace);
const taskId = "task-1";
const sessionId = "session-1";
const environmentId = "environment-1";
const agentExecutionId = "execution-1";

let workspaceRestoration: WorkspaceRestorationState;
let state: Record<string, unknown>;
const bumpWorkspaceFilesRefresh = vi.fn();

function resetStore() {
  workspaceRestoration = { byEnvironmentId: {} };
  state = {
    environmentIdBySessionId: { [sessionId]: environmentId },
    taskSessions: { items: { [sessionId]: { task_environment_id: environmentId } } },
    sessionAgentctl: { itemsBySessionId: {} },
    get workspaceRestoration() {
      return workspaceRestoration;
    },
    beginWorkspaceRestoration: (
      nextTaskId: string,
      nextSessionId: string,
      nextEnvironmentId: string,
    ) =>
      beginAttempt(workspaceRestoration, {
        taskId: nextTaskId,
        sessionId: nextSessionId,
        environmentId: nextEnvironmentId,
      }),
    completeWorkspaceRestoration: (attempt: Parameters<typeof completeAttempt>[1]) =>
      completeAttempt(workspaceRestoration, attempt),
    failWorkspaceRestoration: (attempt: Parameters<typeof failAttempt>[1], details: string) =>
      failAttempt(workspaceRestoration, attempt, details),
    clearWorkspaceRestoration: (attempt: Parameters<typeof clearAttempt>[1]) =>
      clearAttempt(workspaceRestoration, attempt),
    bumpWorkspaceFilesRefresh,
  };
}

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (value: Record<string, unknown>) => unknown) => selector(state),
  useAppStoreApi: () => ({ getState: () => state }),
}));

vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (key: string) => key,
  }),
}));

beforeEach(() => {
  vi.clearAllMocks();
  resetStore();
});

describe("useWorkspaceRestoration attempt lifecycle", () => {
  it("settles a failed restore before workspace consumers become ready", async () => {
    mockRestoreSessionWorkspace.mockRejectedValueOnce(new Error("backend\u0000failure"));
    const { result, rerender } = renderHook(() =>
      useWorkspaceRestoration(taskId, sessionId, environmentId),
    );

    await act(async () => {
      expect(await result.current.restore()).toBe(false);
    });
    rerender();

    expect(result.current.status).toBe("error");
    expect(result.current.attempt?.details).toBe("backendfailure");
    expect(bumpWorkspaceFilesRefresh).not.toHaveBeenCalled();
  });

  it("retries a failed restore, clears the error, and refreshes workspace consumers", async () => {
    mockRestoreSessionWorkspace
      .mockRejectedValueOnce(new Error("first failure"))
      .mockResolvedValueOnce({ success: true, agent_execution_id: agentExecutionId } as never);
    const { result, rerender } = renderHook(() =>
      useWorkspaceRestoration(taskId, sessionId, environmentId),
    );

    await act(async () => {
      await result.current.restore();
    });
    rerender();
    expect(result.current.status).toBe("error");

    state.sessionAgentctl = {
      itemsBySessionId: { [sessionId]: { status: "ready", agentExecutionId } },
    };

    await act(async () => {
      expect(await result.current.restore()).toBe(true);
    });
    rerender();

    expect(result.current.status).toBe("ready");
    expect(result.current.attempt?.details).toBeUndefined();
    expect(mockRestoreSessionWorkspace).toHaveBeenCalledTimes(2);
    expect(bumpWorkspaceFilesRefresh).toHaveBeenCalledWith(sessionId);
  });
});

// eslint-disable-next-line max-lines-per-function -- concurrency and mapping races share one harness.
describe("useWorkspaceRestoration concurrency guards", () => {
  it("rejects a duplicate retry while the first restore is pending", async () => {
    let resolveRestore: (() => void) | undefined;
    mockRestoreSessionWorkspace.mockReturnValueOnce(
      new Promise((resolve) => {
        resolveRestore = () => resolve({ success: true } as never);
      }),
    );
    const { result, rerender } = renderHook(() =>
      useWorkspaceRestoration(taskId, sessionId, environmentId),
    );

    let firstRestore: Promise<boolean> | undefined;
    await act(async () => {
      firstRestore = result.current.restore();
    });
    rerender();
    expect(result.current.status).toBe("pending");

    await act(async () => {
      expect(await result.current.restore()).toBe(false);
    });
    expect(mockRestoreSessionWorkspace).toHaveBeenCalledTimes(1);

    await act(async () => {
      resolveRestore?.();
      expect(await firstRestore).toBe(true);
    });
    rerender();
    expect(result.current.status).toBe("pending");
  });

  it("settles a fallback-key attempt after its environment mapping is registered", async () => {
    let resolveRestore: (() => void) | undefined;
    mockRestoreSessionWorkspace.mockReturnValueOnce(
      new Promise((resolve) => {
        resolveRestore = () =>
          resolve({ success: true, agent_execution_id: agentExecutionId } as never);
      }),
    );
    const { result, rerender } = renderHook(
      ({ currentEnvironmentId }) =>
        useWorkspaceRestoration(taskId, sessionId, currentEnvironmentId),
      { initialProps: { currentEnvironmentId: sessionId } },
    );

    let restorePromise: Promise<boolean> | undefined;
    await act(async () => {
      restorePromise = result.current.restore();
    });
    const fallbackAttempt = workspaceRestoration.byEnvironmentId[sessionId];
    expect(fallbackAttempt).toMatchObject({
      taskId,
      sessionId,
      environmentId: sessionId,
      status: "pending",
    });

    workspaceRestoration.byEnvironmentId[environmentId] = {
      ...fallbackAttempt!,
      environmentId,
    };
    delete workspaceRestoration.byEnvironmentId[sessionId];
    state.sessionAgentctl = {
      itemsBySessionId: { [sessionId]: { status: "ready", agentExecutionId } },
    };
    rerender({ currentEnvironmentId: environmentId });

    await act(async () => {
      resolveRestore?.();
      expect(await restorePromise).toBe(true);
    });
    expect(workspaceRestoration.byEnvironmentId[environmentId]).toMatchObject({
      environmentId,
      status: "ready",
    });
  });

  it("does not let a restore from the previous task settle the current attempt", async () => {
    const restores: Array<() => void> = [];
    mockRestoreSessionWorkspace.mockImplementation(
      () =>
        new Promise((resolve) =>
          restores.push(() =>
            resolve({ success: true, agent_execution_id: agentExecutionId } as never),
          ),
        ),
    );
    const { result, rerender } = renderHook(
      ({ currentTaskId }) => useWorkspaceRestoration(currentTaskId, sessionId, environmentId),
      { initialProps: { currentTaskId: taskId } },
    );

    let firstRestore: Promise<boolean> | undefined;
    await act(async () => {
      firstRestore = result.current.restore();
    });
    rerender({ currentTaskId: "task-2" });
    let secondRestore: Promise<boolean> | undefined;
    await act(async () => {
      secondRestore = result.current.restore();
    });
    rerender({ currentTaskId: "task-2" });
    expect(result.current.status).toBe("pending");

    await act(async () => {
      restores[0]?.();
      expect(await firstRestore).toBe(false);
    });
    rerender({ currentTaskId: "task-2" });
    expect(result.current.attempt?.taskId).toBe("task-2");
    expect(result.current.status).toBe("pending");

    state.sessionAgentctl = {
      itemsBySessionId: { [sessionId]: { status: "ready", agentExecutionId } },
    };
    await act(async () => {
      restores[1]?.();
      expect(await secondRestore).toBe(true);
    });
    rerender({ currentTaskId: "task-2" });
    expect(result.current.status).toBe("ready");
    expect(result.current.attempt?.taskId).toBe("task-2");
  });
});
