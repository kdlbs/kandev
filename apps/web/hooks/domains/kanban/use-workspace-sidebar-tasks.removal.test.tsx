import { act, renderHook } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import { useWorkspaceSidebarTasks } from "./use-workspace-sidebar-tasks";

vi.mock("./use-sidebar-task-page", () => ({
  useSidebarTaskPage: () => ({
    response: {
      entries: [
        {
          kind: "task",
          task_id: "target",
          task: {
            id: "target",
            workspace_id: "ws",
            workflow_id: "wf",
            workflow_step_id: "step",
            title: "Target",
            description: "",
            state: "TODO",
            priority: "medium",
            position: 0,
            created_at: "2026-09-30T00:00:00Z",
            updated_at: "2026-09-30T00:00:00Z",
          },
        },
      ],
    },
    view: { id: "view", group: "none", filters: [] },
    isLoading: false,
    error: null,
  }),
}));

describe("sidebar removal subscription", () => {
  // @covers AC-TASKS-REMOVAL-NAVIGATION-005.1, AC-TASKS-REMOVAL-NAVIGATION-005.2
  it("reacts to real-store deletion intent and release while a request is deferred", async () => {
    const { result } = renderHook(
      () => ({
        store: useAppStoreApi(),
        sidebar: useWorkspaceSidebarTasks("ws"),
      }),
      { wrapper: StateProvider },
    );
    let settle!: () => void;
    const mutation = new Promise<void>((resolve) => {
      settle = resolve;
    });
    let removal!: Promise<void>;
    act(() => {
      const token = result.current.store.getState().beginTaskRemoval({
        action: "delete",
        workspaceId: "ws",
        taskIds: ["target"],
        requestIds: ["target"],
        departure: null,
      });
      expect(token).not.toBeNull();
      removal = mutation.finally(() => result.current.store.getState().releaseTaskRemoval(token!));
    });
    expect(result.current.sidebar.pendingRemovalTaskIds).toEqual(new Set(["target"]));
    await act(async () => {
      settle();
      await removal;
    });
    expect(result.current.sidebar.pendingRemovalTaskIds.size).toBe(0);
    expect(result.current.sidebar.allTasks[0].title).toBe("Target");
  });
});
