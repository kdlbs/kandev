import { describe, expect, it, vi } from "vitest";
import type { Task } from "@/components/kanban-card";
import { filterTasks, projectWorkflowTasks } from "@/lib/kanban/task-projections";
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
      wf: { tasks: [makeTask({ id: "1" }), makeTask({ id: "2" })], steps: [] },
    };

    const result = filterTasks(snapshots, "wf", NO_REPO_FILTER);

    expect(result.map((t) => t.id)).toEqual(["1", "2"]);
  });

  it("excludes tasks the predicate rejects", () => {
    const snapshots = {
      wf: { tasks: [makeTask({ id: "1" }), makeTask({ id: "2" })], steps: [] },
    };
    const matches = vi.fn((taskId: string) => taskId === "1");

    const result = filterTasks(snapshots, "wf", NO_REPO_FILTER, {
      matchesPluginTaskFilters: matches,
    });

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
        steps: [],
      },
    };
    const repoFilter = mapSelectedRepositoryIds(
      [{ id: "repo-a", name: "A" } as never, { id: "repo-b", name: "B" } as never],
      ["repo-a", "repo-b"],
    );
    const matches = (taskId: string) => taskId === "1";

    const result = filterTasks(snapshots, "wf", repoFilter, {
      searchQuery: "fix",
      matchesPluginTaskFilters: matches,
    });

    expect(result.map((t) => t.id)).toEqual(["1"]);
  });
});

describe("filterTasks — hiddenStepIds (per-workflow step visibility filter)", () => {
  const NO_REPO_FILTER = mapSelectedRepositoryIds([], []);
  const snapshots = {
    wf: {
      tasks: [
        makeTask({ id: "1", workflowStepId: "step-a" }),
        makeTask({ id: "2", workflowStepId: "step-b" }),
      ],
      steps: [{ id: "step-a" }, { id: "step-b" }],
    },
  };

  it("hides tasks whose workflowStepId is in the hidden set", () => {
    const result = filterTasks(snapshots, "wf", NO_REPO_FILTER, {
      hiddenStepIds: new Set(["step-a"]),
    });

    expect(result.map((t) => t.id)).toEqual(["2"]);
  });

  it("hides nothing when the hidden set is empty", () => {
    const result = filterTasks(snapshots, "wf", NO_REPO_FILTER, {
      hiddenStepIds: new Set(),
    });

    expect(result.map((t) => t.id)).toEqual(["1", "2"]);
  });

  it("hides nothing when hiddenStepIds is omitted", () => {
    const result = filterTasks(snapshots, "wf", NO_REPO_FILTER);

    expect(result.map((t) => t.id)).toEqual(["1", "2"]);
  });

  it("is a no-op for a stale hidden id that no longer exists in the snapshot's steps (H ∩ liveStepIds)", () => {
    const result = filterTasks(snapshots, "wf", NO_REPO_FILTER, {
      hiddenStepIds: new Set(["step-deleted"]),
    });

    // Identical to rendering with an empty hidden set — the stale id hides nothing.
    expect(result.map((t) => t.id)).toEqual(["1", "2"]);
  });

  it("hides only the live id when the hidden set mixes a live and a stale id", () => {
    const result = filterTasks(snapshots, "wf", NO_REPO_FILTER, {
      hiddenStepIds: new Set(["step-a", "step-deleted"]),
    });

    expect(result.map((t) => t.id)).toEqual(["2"]);
  });

  it("composes with the repository filter (AND semantics)", () => {
    const scoped = {
      wf: {
        tasks: [
          makeTask({ id: "1", workflowStepId: "step-a", repositoryId: "repo-a" }),
          makeTask({ id: "2", workflowStepId: "step-b", repositoryId: "repo-a" }),
          makeTask({ id: "3", workflowStepId: "step-b", repositoryId: "repo-b" }),
        ],
        steps: [{ id: "step-a" }, { id: "step-b" }],
      },
    };
    const repoFilter = mapSelectedRepositoryIds(
      [{ id: "repo-a", name: "A" } as never, { id: "repo-b", name: "B" } as never],
      ["repo-a"],
    );

    const result = filterTasks(scoped, "wf", repoFilter, {
      hiddenStepIds: new Set(["step-a"]),
    });

    // step-a hidden removes task 1; repo filter (repo-a only) removes task 3;
    // task 2 (step-b, repo-a) survives both.
    expect(result.map((t) => t.id)).toEqual(["2"]);
  });

  it("scopes the hidden set to the requested workflow only", () => {
    const sharedStepId = "step-shared";
    const twoWorkflows = {
      a: {
        tasks: [makeTask({ id: "1", workflowStepId: sharedStepId })],
        steps: [{ id: sharedStepId }],
      },
      b: {
        tasks: [makeTask({ id: "2", workflowStepId: sharedStepId })],
        steps: [{ id: sharedStepId }],
      },
    };

    const resultA = filterTasks(twoWorkflows, "a", NO_REPO_FILTER, {
      hiddenStepIds: new Set([sharedStepId]),
    });
    const resultB = filterTasks(twoWorkflows, "b", NO_REPO_FILTER);

    expect(resultA.map((t) => t.id)).toEqual([]);
    expect(resultB.map((t) => t.id)).toEqual(["2"]);
  });
});

describe("projectWorkflowTasks — visible cards vs column occupancy", () => {
  const NO_REPO_FILTER = mapSelectedRepositoryIds([], []);

  it("keeps search and manual hiding out of occupancy while applying plugin filters", () => {
    const snapshots = {
      wf: {
        tasks: [
          makeTask({ id: "visible", title: "Needle", workflowStepId: "step-a" }),
          makeTask({ id: "hidden", title: "Other", workflowStepId: "step-b" }),
          makeTask({ id: "plugin-rejected", title: "Needle", workflowStepId: "step-c" }),
        ],
        steps: [{ id: "step-a" }, { id: "step-b" }, { id: "step-c" }],
      },
    };

    const projection = projectWorkflowTasks(snapshots, "wf", NO_REPO_FILTER, {
      searchQuery: "needle",
      hiddenStepIds: new Set(["step-b"]),
      matchesPluginTaskFilters: (taskId) => taskId !== "plugin-rejected",
    });

    expect(projection.visibleTasks.map((task) => task.id)).toEqual(["visible"]);
    expect(projection.occupancyTasks.map((task) => task.id)).toEqual(["visible", "hidden"]);
  });
});
