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

describe("useTaskBulkRemovalController", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    getState.mockReturnValue({
      taskSessionsByTask: {
        itemsByTaskId: { "task-1": sessions },
        loadingByTaskId: { "task-1": false },
        errorByTaskId: { "task-1": null },
      },
    });
  });

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
});
