import { describe, expect, it } from "vitest";
import type { Task } from "@/components/kanban-card";
import { filterTasks, projectWorkflowTasks } from "./task-projections";

const WORKFLOW_ID = "wf-1";
const CRITICAL_ID = "critical-1";
const HIGH_ID = "high-1";

function task(id: string, priority?: Task["priority"]): Task {
  return { id, title: id, workflowStepId: "step-1", priority } as Task;
}

const snapshots = {
  [WORKFLOW_ID]: {
    tasks: [
      task(CRITICAL_ID, "critical"),
      task(HIGH_ID, "high"),
      task("medium-1", "medium"),
      task("low-1", "low"),
      task("unranked-1"),
    ],
    steps: [{ id: "step-1" }],
  },
};

describe("filterTasks — priority filter", () => {
  it("admits every task under the empty (default) selection, including unranked", () => {
    const result = filterTasks(snapshots, WORKFLOW_ID, new Set(), { priorityFilterTokens: [] });
    expect(result.map((t) => t.id)).toEqual([
      CRITICAL_ID,
      HIGH_ID,
      "medium-1",
      "low-1",
      "unranked-1",
    ]);
  });

  it("admits only tasks whose priority is a member of a non-empty selection", () => {
    const result = filterTasks(snapshots, WORKFLOW_ID, new Set(), {
      priorityFilterTokens: ["critical", "high"],
    });
    expect(result.map((t) => t.id)).toEqual([CRITICAL_ID, HIGH_ID]);
  });

  it("excludes an unranked task under any non-empty selection", () => {
    const result = filterTasks(snapshots, WORKFLOW_ID, new Set(), {
      priorityFilterTokens: ["low"],
    });
    expect(result.map((t) => t.id)).toEqual(["low-1"]);
  });

  it("composes with an existing filter (search) rather than bypassing it", () => {
    const result = filterTasks(snapshots, WORKFLOW_ID, new Set(), {
      priorityFilterTokens: ["critical", "high", "medium", "low"],
      searchQuery: "high",
    });
    expect(result.map((t) => t.id)).toEqual([HIGH_ID]);
  });

  it("renders an empty result rather than omitting the workflow when nothing matches", () => {
    const result = filterTasks(snapshots, WORKFLOW_ID, new Set(), {
      priorityFilterTokens: [],
      searchQuery: "no-such-task",
    });
    expect(result).toEqual([]);
  });

  it("renders an empty result rather than omitting the workflow when a non-empty priority selection alone admits nothing (AC-001.5)", () => {
    // Every task in this step is "low"; selecting only "critical" must empty
    // the step via the priority filter itself, not via searchQuery (which
    // stays blank here) — the case the prior version of this test never
    // actually drove.
    const noMatchSnapshots = {
      [WORKFLOW_ID]: {
        tasks: [task("low-only-1", "low"), task("low-only-2", "low")],
        steps: [{ id: "step-1" }],
      },
    };
    const result = filterTasks(noMatchSnapshots, WORKFLOW_ID, new Set(), {
      priorityFilterTokens: ["critical"],
    });
    expect(result).toEqual([]);
  });
});

describe("projectWorkflowTasks — priority filter scoping", () => {
  it("applies the priority filter to visibleTasks only, never occupancyTasks", () => {
    const { visibleTasks, occupancyTasks } = projectWorkflowTasks(
      snapshots,
      WORKFLOW_ID,
      new Set(),
      {
        searchQuery: "",
        priorityFilterTokens: ["critical"],
      },
    );
    expect(visibleTasks.map((t) => t.id)).toEqual([CRITICAL_ID]);
    expect(occupancyTasks.map((t) => t.id)).toEqual([
      CRITICAL_ID,
      HIGH_ID,
      "medium-1",
      "low-1",
      "unranked-1",
    ]);
  });
});

// @covers AC-UI-BOARD-REPOSITORY-MATCHING-001.5, AC-UI-BOARD-REPOSITORY-MATCHING-001.6
describe("projectWorkflowTasks repository membership", () => {
  const linked: Task = {
    id: "multi",
    title: "Rendering fix",
    workflowStepId: "ready",
    repositoryId: "backend",
    repositories: [
      { id: "backend-link", repository_id: "backend", position: 0 },
      { id: "web-link", repository_id: "web", position: 1 },
    ],
  };
  const workflows = {
    first: {
      steps: [{ id: "ready" }],
      tasks: [
        linked,
        { ...linked, id: "empty", repositoryId: "web", repositories: [] },
        {
          ...linked,
          id: "other",
          repositories: [{ id: "other-link", repository_id: "other", position: 0 }],
        },
        { ...linked, id: "plugin-rejected" },
      ],
    },
    second: {
      steps: [{ id: "ready" }],
      tasks: [{ ...linked, id: "legacy", repositoryId: "web", repositories: undefined }],
    },
  };

  it("retains a secondary-linked task in occupancy even when search removes its card", () => {
    const first = projectWorkflowTasks(workflows, "first", new Set(["web"]), {
      searchQuery: "absent",
      matchesPluginTaskFilters: (id) => id !== "plugin-rejected",
    });
    expect(first.visibleTasks).toEqual([]);
    expect(first.occupancyTasks).toEqual([linked]);
    expect(first.occupancyTasks.map((entry) => entry.workflowStepId)).toEqual(["ready"]);
    const second = projectWorkflowTasks(workflows, "second", new Set(["web"]), { searchQuery: "" });
    expect(second.visibleTasks.map((entry) => entry.id)).toEqual(["legacy"]);
    expect(second.occupancyTasks.map((entry) => entry.id)).toEqual(["legacy"]);
  });

  it("composes membership with search, hidden steps and plugins for visible cards", () => {
    const options = {
      searchQuery: "rendering",
      matchesPluginTaskFilters: (id: string) => id !== "plugin-rejected",
    };
    expect(
      projectWorkflowTasks(workflows, "first", new Set(["backend", "web"]), options).visibleTasks,
    ).toEqual([linked]);
    const hidden = projectWorkflowTasks(workflows, "first", new Set(["web"]), {
      ...options,
      hiddenStepIds: new Set(["ready"]),
    });
    expect(hidden.visibleTasks).toEqual([]);
    expect(hidden.occupancyTasks).toEqual([linked]);
  });
});
