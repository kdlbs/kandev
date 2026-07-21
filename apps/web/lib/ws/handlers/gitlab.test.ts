import { describe, expect, it, vi } from "vitest";
import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import type { TaskMR } from "@/lib/types/gitlab";
import { registerGitLabHandlers } from "./gitlab";

const WORKSPACE_A = "workspace-a";

function makeStore(activeWorkspaceId: string | null) {
  const setTaskMR = vi.fn();
  const state = {
    workspaces: { activeId: activeWorkspaceId },
    setTaskMR,
  } as unknown as AppState;
  return {
    store: { getState: () => state } as StoreApi<AppState>,
    setTaskMR,
  };
}

function taskMR(workspaceId: string) {
  return {
    workspace_id: workspaceId,
    id: "mr-1",
    task_id: "task-1",
    host: "https://gitlab.example.test",
    project_path: "group/project",
    mr_iid: 7,
  } as TaskMR & { workspace_id: string };
}

describe("GitLab WebSocket handlers", () => {
  it("upserts a task MR for the active workspace", () => {
    const { store, setTaskMR } = makeStore(WORKSPACE_A);
    const handler = registerGitLabHandlers(store)["gitlab.task_mr.updated"]!;
    const mr = taskMR(WORKSPACE_A);

    handler({ type: "notification", action: "gitlab.task_mr.updated", payload: mr });

    expect(setTaskMR).toHaveBeenCalledWith(WORKSPACE_A, "task-1", mr);
  });

  it("ignores task MRs from another workspace", () => {
    const { store, setTaskMR } = makeStore("workspace-b");
    const handler = registerGitLabHandlers(store)["gitlab.task_mr.updated"]!;

    handler({
      type: "notification",
      action: "gitlab.task_mr.updated",
      payload: taskMR(WORKSPACE_A),
    });

    expect(setTaskMR).not.toHaveBeenCalled();
  });
});
