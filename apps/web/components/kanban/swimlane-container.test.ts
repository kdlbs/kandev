import { describe, expect, it, vi } from "vitest";
import type { Task } from "@/components/kanban-card";
import { filterTasks } from "./swimlane-container";
import { mapSelectedRepositoryIds } from "@/lib/kanban/filters";

function makeTask(overrides: Partial<Task> & { id: string }): Task {
  return {
    title: "Task",
    description: "",
    repositoryId: undefined,
    ...overrides,
  } as Task;
}

describe("filterTasks — plugin task filter predicate", () => {
  const NO_REPO_FILTER = mapSelectedRepositoryIds([], []);

  it("keeps every task when no predicate is supplied", () => {
    const snapshots = {
      wf: { tasks: [makeTask({ id: "1" }), makeTask({ id: "2" })] },
    };

    const result = filterTasks(snapshots, "wf", NO_REPO_FILTER);

    expect(result.map((t) => t.id)).toEqual(["1", "2"]);
  });

  it("excludes tasks the predicate rejects", () => {
    const snapshots = {
      wf: { tasks: [makeTask({ id: "1" }), makeTask({ id: "2" })] },
    };
    const matches = vi.fn((taskId: string) => taskId === "1");

    const result = filterTasks(snapshots, "wf", NO_REPO_FILTER, undefined, matches);

    expect(result.map((t) => t.id)).toEqual(["1"]);
    expect(matches).toHaveBeenCalledWith("1");
    expect(matches).toHaveBeenCalledWith("2");
  });

  it("composes with search and repository filtering (all must pass)", () => {
    const snapshots = {
      wf: {
        tasks: [
          makeTask({ id: "1", title: "Fix bug", repositoryId: "repo-a" }),
          makeTask({ id: "2", title: "Fix bug", repositoryId: "repo-b" }),
        ],
      },
    };
    const repoFilter = mapSelectedRepositoryIds(
      [{ id: "repo-a", name: "A" } as never, { id: "repo-b", name: "B" } as never],
      ["repo-a", "repo-b"],
    );
    const matches = (taskId: string) => taskId === "1";

    const result = filterTasks(snapshots, "wf", repoFilter, "fix", matches);

    expect(result.map((t) => t.id)).toEqual(["1"]);
  });
});
