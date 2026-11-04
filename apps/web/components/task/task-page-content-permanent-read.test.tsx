import { createElement, type ReactNode } from "react";
import { act, cleanup, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { StateProvider } from "@/components/state-provider";
import * as api from "@/lib/api";
import { ApiError } from "@/lib/api/client";
import { taskId, type Task } from "@/lib/types/http";
import { useTaskDetails } from "./task-page-content";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  vi.useRealTimers();
});

const TASK_A = "task-a";

function createStateWrapper(initialState: unknown) {
  return function StateTestWrapper({ children }: { children: ReactNode }) {
    return createElement(StateProvider, { initialState: initialState as never, children });
  };
}

describe("useTaskDetails permanent refresh failure", () => {
  // @covers AC-PLATFORM-INTERACTIVE-READS-005.5
  it("reports a permanent read failure that invalidates the loaded task presentation", async () => {
    vi.useFakeTimers();
    const initialTask = { id: taskId(TASK_A), title: "Current task" } as Task;
    vi.spyOn(api, "fetchTask").mockRejectedValue(new ApiError("missing", 404, null));
    vi.spyOn(api, "listTaskSessions").mockResolvedValue({ sessions: [], total: 0 });
    const { result } = renderHook(() => useTaskDetails(TASK_A, initialTask), {
      wrapper: createStateWrapper({ tasks: { activeTaskId: TASK_A } }),
    });

    await act(async () => {
      await result.current.refreshTask();
    });

    expect(result.current.taskLoadError).toMatchObject({ status: 404 });
    expect(result.current.isTemporaryTaskReadError).toBe(false);
  });
});
