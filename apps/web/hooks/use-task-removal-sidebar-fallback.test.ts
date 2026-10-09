import { renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { StoreApi } from "zustand";

const performLayoutSwitchMock = vi.fn();
const listTaskSessionsMock = vi.fn();
const fetchTaskMock = vi.fn();
const querySidebarTasksMock = vi.fn();
const softNavigateMock = vi.fn();

vi.mock("@/lib/links", () => ({
  linkToTask: (taskId: string) => `/t/${taskId}`,
  linkToTaskOverview: () => "/?home=overview",
}));

vi.mock("@/lib/state/dockview-store", () => ({
  performLayoutSwitch: (...args: unknown[]) => performLayoutSwitchMock(...args),
}));

vi.mock("@/lib/api", () => ({
  listTaskSessions: (...args: unknown[]) => listTaskSessionsMock(...args),
  fetchTask: (...args: unknown[]) => fetchTaskMock(...args),
  querySidebarTasks: (...args: unknown[]) => querySidebarTasksMock(...args),
}));

vi.mock("@/lib/routing/client-router", () => ({
  softNavigate: (...args: unknown[]) => softNavigateMock(...args),
}));

import { useTaskRemoval } from "./use-task-removal";
import { setRecentTasks } from "@/lib/recent-tasks";
import type { SidebarTaskPageResponse, Task } from "@/lib/types/http";
import { makeTaskRemovalStore } from "./use-task-removal.test-utils";

const TASK_A_ID = "task-A";
const OFF_PAGE_TASK_ID = "task-off-page";
const WORKSPACE_ID = "workspace-1";
const RECENT_TASK_VISITED_AT = "2026-06-07T10:00:00Z";

function makeFallbackTask(title: string): Task {
  return {
    id: OFF_PAGE_TASK_ID,
    workspace_id: WORKSPACE_ID,
    workflow_id: "wf-1",
    workflow_step_id: "step-1",
    title,
    state: "IN_PROGRESS",
    priority: "medium",
    position: 0,
    primary_session_id: "sess-off-page",
    created_at: "2026-09-26T10:00:00Z",
    updated_at: "2026-09-26T10:00:00Z",
  } as unknown as Task;
}

beforeEach(() => {
  vi.clearAllMocks();
  querySidebarTasksMock.mockReset();
  fetchTaskMock.mockResolvedValue({ archived_at: null });
  window.localStorage.clear();
});

describe("useTaskRemoval bounded fallback pagination", () => {
  it("loads the next server page when the first page has no live fallback candidate", async () => {
    const store = makeTaskRemovalStore({
      activeTaskId: TASK_A_ID,
      activeSessionId: "sess-A",
      remainingTasks: [{ id: TASK_A_ID, primarySessionId: "sess-A", workspaceId: WORKSPACE_ID }],
    });
    const serverTask = makeFallbackTask("Second-page fallback");
    querySidebarTasksMock
      .mockResolvedValueOnce({ entries: [], has_next: true } as unknown as SidebarTaskPageResponse)
      .mockResolvedValueOnce({
        entries: [{ kind: "task", task_id: serverTask.id, task: serverTask }],
        has_next: false,
      } as unknown as SidebarTaskPageResponse);
    const { result } = renderHook(() =>
      useTaskRemoval({ store: store as unknown as StoreApi<never> }),
    );

    const removal = await result.current.removeTaskFromBoard(TASK_A_ID, {
      wasActiveTaskId: TASK_A_ID,
      wasActiveSessionId: "sess-A",
      workspaceId: WORKSPACE_ID,
    });

    expect(removal.switchedTaskId).toBe(OFF_PAGE_TASK_ID);
    expect(querySidebarTasksMock).toHaveBeenCalledTimes(2);
    expect(
      querySidebarTasksMock.mock.calls.map((call) => (call[1] as { page: number }).page),
    ).toEqual([1, 2]);
  });

  it("uses a recent eligible cached successor without waiting for a sidebar read", async () => {
    const cachedTask = {
      id: "task-next",
      primarySessionId: "sess-next",
      workspaceId: WORKSPACE_ID,
    };
    const store = makeTaskRemovalStore({
      activeTaskId: TASK_A_ID,
      activeSessionId: "sess-A",
      remainingTasks: [
        { id: TASK_A_ID, primarySessionId: "sess-A", workspaceId: WORKSPACE_ID },
        cachedTask,
      ],
      canonicalTasks: [cachedTask],
    });
    setRecentTasks([
      {
        taskId: cachedTask.id,
        title: "Cached",
        visitedAt: RECENT_TASK_VISITED_AT,
        workspaceId: WORKSPACE_ID,
      },
    ]);
    const { result } = renderHook(() =>
      useTaskRemoval({ store: store as unknown as StoreApi<never> }),
    );

    const removal = await result.current.removeTaskFromBoard(TASK_A_ID, {
      wasActiveTaskId: TASK_A_ID,
      wasActiveSessionId: "sess-A",
      workspaceId: WORKSPACE_ID,
    });

    expect(removal.switchedTaskId).toBe(cachedTask.id);
    expect(querySidebarTasksMock).not.toHaveBeenCalled();
    expect(store.getRecorded().setActiveSession).toHaveBeenCalledWith(cachedTask.id, "sess-next");
  });

  it("uses a cached live successor when no unseen recent task can outrank it", async () => {
    const cachedTask = {
      id: "task-next",
      primarySessionId: "sess-next",
      workspaceId: WORKSPACE_ID,
    };
    const store = makeTaskRemovalStore({
      activeTaskId: TASK_A_ID,
      activeSessionId: "sess-A",
      remainingTasks: [
        { id: TASK_A_ID, primarySessionId: "sess-A", workspaceId: WORKSPACE_ID },
        cachedTask,
      ],
      canonicalTasks: [cachedTask],
    });
    setRecentTasks([
      {
        taskId: TASK_A_ID,
        title: "Removed",
        visitedAt: RECENT_TASK_VISITED_AT,
        workspaceId: WORKSPACE_ID,
      },
    ]);
    const { result } = renderHook(() =>
      useTaskRemoval({ store: store as unknown as StoreApi<never> }),
    );

    const removal = await result.current.removeTaskFromBoard(TASK_A_ID, {
      wasActiveTaskId: TASK_A_ID,
      wasActiveSessionId: "sess-A",
      workspaceId: WORKSPACE_ID,
    });

    expect(removal.switchedTaskId).toBe(cachedTask.id);
    expect(querySidebarTasksMock).not.toHaveBeenCalled();
  });
});
