import { describe, expect, it } from "vitest";
import {
  compareTasksByCreatedDesc,
  compareTasksByPriorityThenCreatedDesc,
  compareTasksByPriorityThenPositionAsc,
  sortIdsByCreatedDesc,
  sortIdsByDisplayOrder,
  sortTasksForPipelineView,
} from "./task-order";

const BASE_CREATED_AT = "2026-01-01T00:00:00Z";

describe("compareTasksByCreatedDesc", () => {
  it("sorts newer created tasks first", () => {
    const tasks = [
      { id: "old", createdAt: "2026-05-01T10:00:00Z" },
      { id: "new", createdAt: "2026-05-02T10:00:00Z" },
    ];

    expect([...tasks].sort(compareTasksByCreatedDesc).map((task) => task.id)).toEqual([
      "new",
      "old",
    ]);
  });

  it("sorts tasks without createdAt after dated tasks", () => {
    const tasks = [
      { id: "missing" },
      { id: "old", createdAt: "2026-05-01T10:00:00Z" },
      { id: "new", createdAt: "2026-05-02T10:00:00Z" },
    ];

    expect([...tasks].sort(compareTasksByCreatedDesc).map((task) => task.id)).toEqual([
      "new",
      "old",
      "missing",
    ]);
  });

  it("sorts by actual timestamp when ISO offsets differ", () => {
    const tasks = [
      { id: "later-offset", createdAt: "2026-05-02T09:30:00-04:00" },
      { id: "earlier-zulu", createdAt: "2026-05-02T13:00:00Z" },
    ];

    expect([...tasks].sort(compareTasksByCreatedDesc).map((task) => task.id)).toEqual([
      "later-offset",
      "earlier-zulu",
    ]);
  });

  it("keeps equal missing createdAt tasks stable", () => {
    const tasks: Array<{ id: string; createdAt?: string }> = [{ id: "first" }, { id: "second" }];

    expect([...tasks].sort(compareTasksByCreatedDesc).map((task) => task.id)).toEqual([
      "first",
      "second",
    ]);
  });

  it("returns 0 when both tasks are missing createdAt", () => {
    expect(compareTasksByCreatedDesc({}, {})).toBe(0);
  });
});

describe("sortIdsByCreatedDesc", () => {
  // d newest … a oldest → board order is d, c, b, a.
  const taskById = new Map<string, { createdAt?: string }>([
    ["a", { createdAt: BASE_CREATED_AT }],
    ["b", { createdAt: "2026-01-02T00:00:00Z" }],
    ["c", { createdAt: "2026-01-03T00:00:00Z" }],
    ["d", { createdAt: "2026-01-04T00:00:00Z" }],
  ]);

  it("reorders a backward range selection into board (created-desc) order", () => {
    // Anchor on the oldest then shift up leaves the Set as [a, c, b] (insertion).
    expect(sortIdsByCreatedDesc(["a", "c", "b"], taskById)).toEqual(["c", "b", "a"]);
  });

  it("sorts ids without a known task last (transitive fallback)", () => {
    expect(sortIdsByCreatedDesc(["zzz", "a"], taskById)).toEqual(["a", "zzz"]);
  });

  it("stays transitive when only some ids are missing", () => {
    // c (newest present) < a (older present) < missing → deterministic order.
    expect(sortIdsByCreatedDesc(["a", "missing", "c"], taskById)).toEqual(["c", "a", "missing"]);
  });
});

describe("compareTasksByPriorityThenCreatedDesc", () => {
  it("orders by priority rank first: critical, high, medium, low, unranked", () => {
    const tasks = [
      { id: "u", createdAt: BASE_CREATED_AT },
      { id: "l", createdAt: BASE_CREATED_AT, priority: "low" as const },
      { id: "m", createdAt: BASE_CREATED_AT, priority: "medium" as const },
      { id: "h", createdAt: BASE_CREATED_AT, priority: "high" as const },
      { id: "c", createdAt: BASE_CREATED_AT, priority: "critical" as const },
    ];
    expect([...tasks].sort(compareTasksByPriorityThenCreatedDesc).map((t) => t.id)).toEqual([
      "c",
      "h",
      "m",
      "l",
      "u",
    ]);
  });

  it("breaks a tied rank by createdAt descending", () => {
    const tasks = [
      { id: "older", createdAt: BASE_CREATED_AT, priority: "high" as const },
      { id: "newer", createdAt: "2026-01-02T00:00:00Z", priority: "high" as const },
    ];
    expect([...tasks].sort(compareTasksByPriorityThenCreatedDesc).map((t) => t.id)).toEqual([
      "newer",
      "older",
    ]);
  });

  it("breaks a fully tied rank and createdAt by task id ascending", () => {
    const tasks = [
      { id: "zeta", priority: "high" as const },
      { id: "alpha", priority: "high" as const },
    ];
    expect([...tasks].sort(compareTasksByPriorityThenCreatedDesc).map((t) => t.id)).toEqual([
      "alpha",
      "zeta",
    ]);
  });

  it("only ever lifts higher-priority cards, never reorders same-rank cards beyond id/createdAt", () => {
    // Same rank + same createdAt collapses to the id key, so nothing "scrambles".
    const tasks = [
      { id: "b", createdAt: BASE_CREATED_AT, priority: "medium" as const },
      { id: "a", createdAt: BASE_CREATED_AT, priority: "medium" as const },
    ];
    expect([...tasks].sort(compareTasksByPriorityThenCreatedDesc).map((t) => t.id)).toEqual([
      "a",
      "b",
    ]);
  });
});

describe("compareTasksByPriorityThenPositionAsc", () => {
  it("orders by priority rank, then position ascending, then id ascending", () => {
    const tasks = [
      { id: "z", position: 0, priority: "low" as const },
      { id: "b", position: 2, priority: "critical" as const },
      { id: "a", position: 1, priority: "critical" as const },
    ];
    expect([...tasks].sort(compareTasksByPriorityThenPositionAsc).map((t) => t.id)).toEqual([
      "a",
      "b",
      "z",
    ]);
  });

  it("treats a missing position as 0", () => {
    const tasks = [
      { id: "has-pos", position: 1, priority: "high" as const },
      { id: "no-pos", priority: "high" as const },
    ];
    expect([...tasks].sort(compareTasksByPriorityThenPositionAsc).map((t) => t.id)).toEqual([
      "no-pos",
      "has-pos",
    ]);
  });
});

describe("sortTasksForPipelineView", () => {
  const displaySteps = [{ id: "todo" }, { id: "review" }, { id: "done" }];

  it("under created_desc, keeps step index then position ascending untouched (no id tiebreak)", () => {
    const tasks = [
      { id: "z1", workflowStepId: "review", position: 1 },
      { id: "z0", workflowStepId: "todo", position: 5 },
      { id: "z2", workflowStepId: "todo", position: 1 },
    ];
    expect(sortTasksForPipelineView(tasks, displaySteps, "created_desc").map((t) => t.id)).toEqual([
      "z2",
      "z0",
      "z1",
    ]);
  });

  it("under priority_desc, orders by step index, then priority rank, then position, then id", () => {
    const tasks = [
      { id: "todo-low", workflowStepId: "todo", position: 0, priority: "low" as const },
      { id: "todo-critical", workflowStepId: "todo", position: 1, priority: "critical" as const },
      { id: "review-high", workflowStepId: "review", position: 0, priority: "high" as const },
    ];
    expect(sortTasksForPipelineView(tasks, displaySteps, "priority_desc").map((t) => t.id)).toEqual(
      ["todo-critical", "todo-low", "review-high"],
    );
  });

  it("never regroups cards across steps under priority_desc: step index stays the outermost key", () => {
    const tasks = [
      { id: "done-critical", workflowStepId: "done", position: 0, priority: "critical" as const },
      { id: "todo-low", workflowStepId: "todo", position: 0, priority: "low" as const },
    ];
    expect(sortTasksForPipelineView(tasks, displaySteps, "priority_desc").map((t) => t.id)).toEqual(
      ["todo-low", "done-critical"],
    );
  });
});

describe("sortIdsByDisplayOrder", () => {
  const taskById = new Map<
    string,
    {
      id: string;
      createdAt?: string;
      priority?: "critical" | "high" | "medium" | "low";
      position?: number;
      workflowStepId?: string;
    }
  >([
    ["a", { id: "a", createdAt: BASE_CREATED_AT, priority: "low" }],
    ["b", { id: "b", createdAt: "2026-01-02T00:00:00Z", priority: "critical" }],
    ["c", { id: "c", createdAt: "2026-01-03T00:00:00Z" }],
  ]);

  it("matches sortIdsByCreatedDesc for the kanban view under created_desc", () => {
    expect(
      sortIdsByDisplayOrder(["a", "c", "b"], taskById, {
        sortToken: "created_desc",
        isPipelineView: false,
      }),
    ).toEqual(sortIdsByCreatedDesc(["a", "c", "b"], taskById));
  });

  it("orders by priority rank for the kanban view under priority_desc", () => {
    expect(
      sortIdsByDisplayOrder(["a", "c", "b"], taskById, {
        sortToken: "priority_desc",
        isPipelineView: false,
      }),
    ).toEqual(["b", "a", "c"]);
  });

  it("orders by step index then position for the pipeline view under created_desc", () => {
    const pipelineTaskById = new Map([
      ["p1", { id: "p1", workflowStepId: "review", position: 0 }],
      ["p2", { id: "p2", workflowStepId: "todo", position: 0 }],
    ]);
    const stepIndexOf = (stepId?: string) => (stepId === "todo" ? 0 : 1);
    expect(
      sortIdsByDisplayOrder(["p1", "p2"], pipelineTaskById, {
        sortToken: "created_desc",
        isPipelineView: true,
        stepIndexOf,
      }),
    ).toEqual(["p2", "p1"]);
  });
});
