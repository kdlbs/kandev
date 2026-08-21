import { describe, expect, it, vi } from "vitest";
import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import type { TaskMR, TaskMRAutomationOptions } from "@/lib/types/gitlab";
import { registerGitLabHandlers } from "./gitlab";

const WORKSPACE_A = "workspace-a";

function makeStore(activeWorkspaceId: string | null, options?: TaskMRAutomationOptions) {
  const setTaskMR = vi.fn();
  const setTaskMRAutomationOptions = vi.fn();
  const markTaskMRAutomationExternalUpdate = vi.fn();
  const state = {
    workspaces: { activeId: activeWorkspaceId },
    taskMRAutomation: { byTaskId: options ? { [options.task_id]: options } : {} },
    setTaskMR,
    setTaskMRAutomationOptions,
    markTaskMRAutomationExternalUpdate,
  } as unknown as AppState;
  return {
    store: { getState: () => state } as StoreApi<AppState>,
    setTaskMR,
    setTaskMRAutomationOptions,
    markTaskMRAutomationExternalUpdate,
  };
}

function taskMRAutomationOptions(
  overrides: Partial<TaskMRAutomationOptions> = {},
): TaskMRAutomationOptions {
  return {
    task_id: "task-1",
    auto_fix_enabled: false,
    auto_merge_enabled: false,
    auto_fix_max_rounds: 10,
    effective_auto_fix_prompt: "",
    using_default_prompt: true,
    prompt_on_review_requested: false,
    prompt_on_merged: true,
    prompt_on_closed: false,
    review_reviewer_username: "",
    updated_at: "2026-01-01T00:00:00Z",
    mr_states: [],
    mr_options: [],
    ...overrides,
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

  it("stores a task MR automation options update pushed for the current task", () => {
    const { store, setTaskMRAutomationOptions, markTaskMRAutomationExternalUpdate } =
      makeStore(WORKSPACE_A);
    const handler = registerGitLabHandlers(store)["gitlab.task_mr_options.updated"]!;
    const options = taskMRAutomationOptions();

    handler({ type: "notification", action: "gitlab.task_mr_options.updated", payload: options });

    expect(setTaskMRAutomationOptions).toHaveBeenCalledWith("task-1", options);
    // Marks the write as externally-sourced so an in-flight local
    // refresh()/update() in the hook knows not to overwrite it.
    expect(markTaskMRAutomationExternalUpdate).toHaveBeenCalledWith("task-1");
  });

  it("drops an older MR automation snapshot pushed after a newer one", () => {
    const current = taskMRAutomationOptions({ automation_revision: 2 });
    const { store, setTaskMRAutomationOptions, markTaskMRAutomationExternalUpdate } = makeStore(
      WORKSPACE_A,
      current,
    );
    const handler = registerGitLabHandlers(store)["gitlab.task_mr_options.updated"]!;

    handler({
      type: "notification",
      action: "gitlab.task_mr_options.updated",
      payload: taskMRAutomationOptions({ automation_revision: 1 }),
    });

    expect(setTaskMRAutomationOptions).not.toHaveBeenCalled();
    expect(markTaskMRAutomationExternalUpdate).not.toHaveBeenCalled();
  });

  it("ignores a task MR automation options update with no task_id", () => {
    const { store, setTaskMRAutomationOptions } = makeStore(WORKSPACE_A);
    const handler = registerGitLabHandlers(store)["gitlab.task_mr_options.updated"]!;

    handler({
      type: "notification",
      action: "gitlab.task_mr_options.updated",
      payload: taskMRAutomationOptions({ task_id: "" }),
    });

    expect(setTaskMRAutomationOptions).not.toHaveBeenCalled();
  });
});
