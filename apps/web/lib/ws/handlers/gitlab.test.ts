import { describe, expect, it, vi } from "vitest";
import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import { createAppStore } from "@/lib/state/store";
import type { TaskMR, TaskMRAutomationOptions } from "@/lib/types/gitlab";
import { registerGitLabHandlers } from "./gitlab";

const WORKSPACE_A = "workspace-a";

function makeStore(activeWorkspaceId: string | null, options?: TaskMRAutomationOptions) {
  const setTaskMR = vi.fn();
  const removeTaskMR = vi.fn();
  const setTaskMRAutomationOptions = vi.fn();
  const markTaskMRAutomationExternalUpdate = vi.fn();
  const state = {
    workspaces: { activeId: activeWorkspaceId },
    taskMRAutomation: { byTaskId: options ? { [options.task_id]: options } : {} },
    setTaskMR,
    removeTaskMR,
    setTaskMRAutomationOptions,
    markTaskMRAutomationExternalUpdate,
  } as unknown as AppState;
  return {
    store: { getState: () => state } as StoreApi<AppState>,
    setTaskMR,
    removeTaskMR,
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

  it("removes a task MR for the active workspace", () => {
    const { store, removeTaskMR } = makeStore(WORKSPACE_A);
    const handler = (registerGitLabHandlers(store) as Record<string, (message: never) => void>)[
      "gitlab.task_mr.deleted"
    ]!;

    handler({
      type: "notification",
      action: "gitlab.task_mr.deleted",
      payload: { workspace_id: WORKSPACE_A, task_id: "task-1", association_id: "mr-1" },
    } as never);

    expect(removeTaskMR).toHaveBeenCalledWith(WORKSPACE_A, "mr-1", 0);
  });

  it("rejects delayed MR updates and deletes around relink hydration", () => {
    const store = createAppStore();
    store.getState().setActiveWorkspace(WORKSPACE_A);
    const handlers = registerGitLabHandlers(store);
    const association = (revision: number, title: string): TaskMR & { workspace_id: string } => ({
      ...taskMR(WORKSPACE_A),
      id: "association-revisioned",
      association_revision: revision,
      mr_title: title,
    });
    store.getState().setTaskMRs(WORKSPACE_A, { "task-1": [association(4, "old")] });
    handlers["gitlab.task_mr.deleted"]!({
      payload: {
        workspace_id: WORKSPACE_A,
        task_id: "task-1",
        association_id: "association-revisioned",
        association_revision: 5,
      },
    } as never);
    handlers["gitlab.task_mr.updated"]!({
      payload: association(4, "stale"),
    } as never);
    expect(store.getState().taskMRs.byWorkspaceId[WORKSPACE_A]?.["task-1"]).toEqual([]);

    // Reconnect hydration installs the committed relink before replaying a
    // delayed delete from the prior association revision.
    store.getState().setTaskMRs(WORKSPACE_A, { "task-1": [association(6, "relinked")] });
    handlers["gitlab.task_mr.deleted"]!({
      payload: {
        workspace_id: WORKSPACE_A,
        task_id: "task-1",
        association_id: "association-revisioned",
        association_revision: 5,
      },
    } as never);
    expect(store.getState().taskMRs.byWorkspaceId[WORKSPACE_A]?.["task-1"]?.[0]?.mr_title).toBe(
      "relinked",
    );
  });
});

describe("GitLab WebSocket handler workspace filtering", () => {
  it("ignores task MR deletion from another workspace", () => {
    const { store, removeTaskMR } = makeStore("workspace-b");
    const handler = (registerGitLabHandlers(store) as Record<string, (message: never) => void>)[
      "gitlab.task_mr.deleted"
    ]!;

    handler({
      type: "notification",
      action: "gitlab.task_mr.deleted",
      payload: { workspace_id: WORKSPACE_A, task_id: "task-1", association_id: "mr-1" },
    } as never);

    expect(removeTaskMR).not.toHaveBeenCalled();
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

describe("GitLab task MR automation options handlers", () => {
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
