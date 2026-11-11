import { afterEach, describe, expect, it } from "vitest";
import { cleanup, renderHook } from "@testing-library/react";
import type { ReactNode } from "react";
import { StateProvider } from "@/components/state-provider";
import type { AppState } from "@/lib/state/store";
import { useTaskRepositoryRoots } from "./use-task-repository-roots";

const ROOT_TRACKER = ".claude/worktrees/fix-b";

afterEach(cleanup);

function wrapperFor(initialState: Partial<AppState>) {
  return function Wrapper({ children }: { children: ReactNode }) {
    return <StateProvider initialState={initialState}>{children}</StateProvider>;
  };
}

function stateWithRoots(): Partial<AppState> {
  return {
    tasks: { activeTaskId: "task-1", activeSessionId: "session-1" },
    kanban: {
      tasks: [
        {
          id: "task-1",
          workspaceId: "ws-1",
          workflowId: "wf-1",
          workflowStepId: "step",
          title: "Adoption",
          position: 0,
          repositories: [
            {
              id: "tr-primary",
              repository_id: "repo-1",
              base_branch: "main",
              checkout_branch: "main",
              position: 0,
            },
            {
              id: "tr-adopted",
              repository_id: "repo-1",
              base_branch: "main",
              checkout_branch: "fix-b",
              workspace_relative_path: ROOT_TRACKER,
              position: 1,
            },
          ],
        },
      ],
    },
    taskSessions: {
      items: {
        "session-1": {
          id: "session-1",
          task_id: "task-1",
          environment_id: "env-1",
          workspace_path: "/task-root",
          worktrees: [
            {
              id: "env-repo-primary",
              session_id: "session-1",
              worktree_id: "wt-primary",
              repository_id: "repo-1",
              position: 0,
              worktree_path: "/task-root/primary",
              worktree_branch: "main",
              worktree_origin: "kandev",
            },
            {
              id: "env-repo-adopted",
              session_id: "session-1",
              worktree_id: "wt-adopted",
              repository_id: "repo-1",
              position: 1,
              workspace_relative_path: ROOT_TRACKER,
              worktree_path: "/task-root/.claude/worktrees/fix-b",
              worktree_branch: "fix-b",
              worktree_origin: "adopted",
            },
          ],
        },
      },
    } as unknown as AppState["taskSessions"],
    repositories: {
      itemsByWorkspaceId: {
        "ws-1": [{ id: "repo-1", name: "kandev" }],
      },
    } as unknown as AppState["repositories"],
  } as unknown as Partial<AppState>;
}

describe("useTaskRepositoryRoots", () => {
  it("builds one removable root per worktree and keeps the primary root", () => {
    const { result } = renderHook(() => useTaskRepositoryRoots("task-1", "session-1"), {
      wrapper: wrapperFor(stateWithRoots()),
    });

    expect(result.current.roots).toHaveLength(2);
    const [primary, adopted] = result.current.roots;
    expect(primary.path).toBe("/task-root/primary");
    expect(primary.isPrimary).toBe(true);
    expect(primary.removable).toBe(false);
    expect(adopted.path).toBe("/task-root/.claude/worktrees/fix-b");
    expect(adopted.relativePath).toBe(ROOT_TRACKER);
    expect(adopted.removable).toBe(true);
    expect(adopted.adopted).toBe(true);
    expect(adopted.branch).toBe("fix-b");
    expect(adopted.displayLabel).toBe("kandev · fix-b");
    expect(result.current.byTrackerName.get(ROOT_TRACKER)?.taskRepositoryId).toBe("tr-adopted");
    expect(result.current.byTrackerName.get("")).toBe(primary);
    expect(result.current.byPath.get(ROOT_TRACKER)?.worktreeId).toBe("wt-adopted");
  });

  it("keeps a stable empty projection when no task is selected", () => {
    const { result, rerender } = renderHook(() => useTaskRepositoryRoots(null, null), {
      wrapper: wrapperFor({}),
    });
    const first = result.current;
    rerender();
    expect(result.current).toBe(first);
    expect(result.current.roots).toEqual([]);
  });

  it("maps a single-repo empty tracker name to the primary root", () => {
    const state = stateWithRoots();
    state.kanban!.tasks[0]!.repositories = [state.kanban!.tasks[0]!.repositories![0]!];
    state.taskSessions!.items["session-1"]!.worktrees = [
      state.taskSessions!.items["session-1"]!.worktrees![0]!,
    ];
    const { result } = renderHook(() => useTaskRepositoryRoots("task-1", "session-1"), {
      wrapper: wrapperFor(state),
    });
    expect(result.current.byTrackerName.get("")).toBe(result.current.roots[0]);
  });
});
