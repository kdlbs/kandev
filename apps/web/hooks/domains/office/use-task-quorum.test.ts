import { describe, expect, it, vi, beforeEach } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import { useTaskQuorum } from "./use-task-quorum";

const mocks = vi.hoisted(() => ({
  getTaskQuorum: vi.fn(),
}));

vi.mock("@/lib/api/domains/office-extended-api", () => ({
  getTaskQuorum: mocks.getTaskQuorum,
}));

const setTaskQuorum = vi.fn();
let byTaskId: Record<string, unknown> = {};
let refetchTrigger: { types: string[]; timestamp: number } | null = null;
const WORKSPACE_ID = "workspace-1";

vi.mock("@/components/state-provider", () => ({
  useAppStore: (sel: (state: unknown) => unknown) =>
    sel({
      office: {
        taskQuorum: { byTaskId },
        refetchTrigger,
      },
      setTaskQuorum,
    }),
}));

describe("useTaskQuorum", () => {
  beforeEach(() => {
    setTaskQuorum.mockReset();
    mocks.getTaskQuorum.mockReset();
    mocks.getTaskQuorum.mockResolvedValue({ guards: [], reevaluation_blocked: false });
    byTaskId = {};
    refetchTrigger = null;
  });

  it("fetches once on mount for a given task", async () => {
    renderHook(() => useTaskQuorum("task-1", WORKSPACE_ID));
    await waitFor(() => expect(mocks.getTaskQuorum).toHaveBeenCalledTimes(1));
    expect(mocks.getTaskQuorum).toHaveBeenCalledWith("task-1", WORKSPACE_ID);
  });

  it("refetches when taskId changes, even though it already fetched once (AC: task switch)", async () => {
    const { rerender } = renderHook(
      ({ taskId }: { taskId: string | null }) => useTaskQuorum(taskId, WORKSPACE_ID),
      {
        initialProps: { taskId: "task-1" },
      },
    );
    await waitFor(() => expect(mocks.getTaskQuorum).toHaveBeenCalledTimes(1));

    rerender({ taskId: "task-2" });
    await waitFor(() => expect(mocks.getTaskQuorum).toHaveBeenCalledTimes(2));
    expect(mocks.getTaskQuorum).toHaveBeenLastCalledWith("task-2", WORKSPACE_ID);
  });

  it("refetches when a task leaves and re-enters review (null -> id -> null -> id), driven by the WS-bumped trigger rather than the taskId prop alone", async () => {
    // office.task.status_changed (lib/ws/handlers/office.ts) bumps
    // `task:${taskId}` on every status transition, including re-entering
    // review with the same task id — this is what actually invalidates the
    // stale snapshot in production, not the taskId prop toggling alone.
    const { rerender } = renderHook(
      ({ taskId }: { taskId: string | null }) => useTaskQuorum(taskId, WORKSPACE_ID),
      {
        initialProps: { taskId: "task-1" as string | null },
      },
    );
    await waitFor(() => expect(mocks.getTaskQuorum).toHaveBeenCalledTimes(1));

    rerender({ taskId: null });
    refetchTrigger = { types: ["task:task-1"], timestamp: 1 };
    rerender({ taskId: "task-1" });
    await waitFor(() => expect(mocks.getTaskQuorum).toHaveBeenCalledTimes(2));
  });

  it("refetches when office.task.decision_recorded bumps the task's refetch trigger", async () => {
    const { rerender } = renderHook(() => useTaskQuorum("task-1", WORKSPACE_ID));
    await waitFor(() => expect(mocks.getTaskQuorum).toHaveBeenCalledTimes(1));

    refetchTrigger = { types: ["task:task-1"], timestamp: 1 };
    rerender();
    await waitFor(() => expect(mocks.getTaskQuorum).toHaveBeenCalledTimes(2));
  });

  it("does not refetch on mount just because the trigger already has an unrelated type", async () => {
    refetchTrigger = { types: ["task:task-other"], timestamp: 3 };
    renderHook(() => useTaskQuorum("task-1", WORKSPACE_ID));
    await waitFor(() => expect(mocks.getTaskQuorum).toHaveBeenCalledTimes(1));
    expect(mocks.getTaskQuorum).toHaveBeenCalledWith("task-1", WORKSPACE_ID);
  });
});
