import { act, renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { TaskSession } from "@/lib/types/http";
import { useTaskBulkRemovalController } from "./use-task-bulk-removal-controller";

const { toast, suppress, clear, getState } = vi.hoisted(() => ({
  toast: vi.fn(),
  suppress: vi.fn(),
  clear: vi.fn(),
  getState: vi.fn(),
}));

vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (key: string) => key }) }));
vi.mock("@/components/toast-provider", () => ({ useToast: () => ({ toast }) }));
vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: unknown) => unknown) => selector(getState()),
  useAppStoreApi: () => ({ getState }),
}));
vi.mock("@/lib/session/session-auto-provisioning-fence", () => ({
  suppressTaskSessionAutoProvisioning: suppress,
  clearTaskSessionAutoProvisioningSuppression: clear,
}));

const sessions = [
  { id: "first", state: "COMPLETED", task_id: "task-1", is_primary: false },
  { id: "second", state: "COMPLETED", task_id: "task-1", is_primary: true },
] as TaskSession[];

beforeEach(() => {
  vi.clearAllMocks();
  getState.mockReturnValue({
    taskSessionsByTask: {
      itemsByTaskId: { "task-1": sessions },
      loadingByTaskId: { "task-1": false },
      loadedByTaskId: { "task-1": true },
      errorByTaskId: { "task-1": null },
    },
  });
});

describe("useTaskBulkRemovalController eligibility", () => {
  it("@covers AC-TASKS-BULK-SESSION-REMOVAL-001.3 waits for an authoritative session list", () => {
    const state = getState();
    state.taskSessionsByTask.loadedByTaskId["task-1"] = false;
    const removeById = vi.fn(async () => true);
    const { result } = renderHook(() =>
      useTaskBulkRemovalController({
        taskId: "task-1",
        sessionId: "first",
        sessions,
        isLoading: false,
        loadSessions: vi.fn(async () => undefined),
        removeById,
      }),
    );

    act(() => result.current.request("others", "first"));

    expect(result.current.bulkRemoval.snapshot).toBeNull();
    expect(toast).toHaveBeenCalledWith({
      title: "task:bulkRemovalUnavailable_loading",
      variant: "error",
    });
    expect(removeById).not.toHaveBeenCalled();
  });

  it("@covers AC-TASKS-BULK-SESSION-REMOVAL-001.3 cancels confirmation if the list becomes incomplete", async () => {
    const state = getState();
    const removeById = vi.fn(async () => true);
    const { result } = renderHook(() =>
      useTaskBulkRemovalController({
        taskId: "task-1",
        sessionId: "first",
        sessions,
        isLoading: false,
        loadSessions: vi.fn(async () => undefined),
        removeById,
      }),
    );

    act(() => result.current.request("others", "first"));
    state.taskSessionsByTask.loadedByTaskId["task-1"] = false;
    await act(async () => result.current.bulkRemoval.confirm());

    expect(removeById).not.toHaveBeenCalled();
    expect(result.current.bulkRemoval.snapshot).toBeNull();
    expect(toast).toHaveBeenCalledWith({
      title: "task:bulkRemovalUnavailable_loading",
      variant: "error",
    });
  });
});

describe("useTaskBulkRemovalController", () => {
  it("clears the Remove All auto-provisioning fence after a partial failure", async () => {
    const removeById = vi.fn(async (id: string) => id !== "first");
    const { result } = renderHook(() =>
      useTaskBulkRemovalController({
        taskId: "task-1",
        sessionId: "first",
        sessions,
        isLoading: false,
        loadSessions: vi.fn(async () => undefined),
        removeById,
      }),
    );

    act(() => result.current.request("all", "first"));
    await act(async () => result.current.bulkRemoval.confirm());

    expect(suppress).toHaveBeenCalledWith("task-1");
    expect(clear).toHaveBeenCalledWith("task-1");
    expect(removeById.mock.calls.map(([id]) => id)).toEqual(["second", "first"]);
  });

  it("reports refresh failures as errors instead of saying sessions are loading", async () => {
    const state = {
      taskSessionsByTask: {
        itemsByTaskId: { "task-1": sessions },
        loadingByTaskId: { "task-1": false },
        loadedByTaskId: { "task-1": true },
        errorByTaskId: { "task-1": null as string | null },
      },
    };
    getState.mockReturnValue(state);
    const { result } = renderHook(() =>
      useTaskBulkRemovalController({
        taskId: "task-1",
        sessionId: "first",
        sessions,
        isLoading: false,
        loadSessions: vi.fn(async () => undefined),
        removeById: vi.fn(async () => true),
      }),
    );

    act(() => result.current.request("all", "first"));
    state.taskSessionsByTask.errorByTaskId["task-1"] = "offline";
    await act(async () => result.current.bulkRemoval.confirm());

    expect(toast).toHaveBeenCalledWith({
      title: "task:bulkRemovalUnavailable_error",
      variant: "error",
    });
  });

  it("dismisses Remove Others and reports a changed session when its selection disappears", async () => {
    const state = {
      taskSessionsByTask: {
        itemsByTaskId: { "task-1": sessions },
        loadingByTaskId: { "task-1": false },
        loadedByTaskId: { "task-1": true },
        errorByTaskId: { "task-1": null },
      },
    };
    getState.mockReturnValue(state);
    const removeById = vi.fn(async () => true);
    const { result } = renderHook(() =>
      useTaskBulkRemovalController({
        taskId: "task-1",
        sessionId: "first",
        sessions,
        isLoading: false,
        loadSessions: vi.fn(async () => {
          state.taskSessionsByTask.itemsByTaskId["task-1"] = sessions.filter(
            (session) => session.id !== "first",
          );
        }),
        removeById,
      }),
    );

    act(() => result.current.request("others", "first"));
    await act(async () => result.current.bulkRemoval.confirm());

    expect(result.current.bulkRemoval.snapshot).toBeNull();
    expect(removeById).not.toHaveBeenCalled();
    expect(toast).toHaveBeenCalledWith({ title: "task:sessionsChangedReviewAgain" });
  });
});
