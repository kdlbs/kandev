import { describe, expect, it } from "vitest";
import {
  compareTasksByBoardOrder,
  compareTasksByCreatedDesc,
  isAdmittedKanbanTask,
  isQueuedOverflowKanbanTask,
  partitionKanbanColumnTasks,
  sortIdsByBoardOrder,
  sortIdsByCreatedDesc,
} from "./task-order";

const CREATED_MAY_01 = "2026-05-01T10:00:00Z";
const CREATED_MAY_02 = "2026-05-02T10:00:00Z";

describe("compareTasksByCreatedDesc", () => {
  it("sorts newer created tasks first", () => {
    const tasks = [
      { id: "old", createdAt: CREATED_MAY_01 },
      { id: "new", createdAt: CREATED_MAY_02 },
    ];

    expect([...tasks].sort(compareTasksByCreatedDesc).map((task) => task.id)).toEqual([
      "new",
      "old",
    ]);
  });

  it("sorts tasks without createdAt after dated tasks", () => {
    const tasks = [
      { id: "missing" },
      { id: "old", createdAt: CREATED_MAY_01 },
      { id: "new", createdAt: CREATED_MAY_02 },
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

describe("compareTasksByBoardOrder", () => {
  it("sorts by position ascending before createdAt", () => {
    const tasks = [
      { id: "b", position: 1, createdAt: CREATED_MAY_02 },
      { id: "a", position: 0, createdAt: CREATED_MAY_01 },
    ];
    expect([...tasks].sort(compareTasksByBoardOrder).map((task) => task.id)).toEqual(["a", "b"]);
  });

  it("uses newest-first when every position is zero", () => {
    const tasks = [
      { id: "old", position: 0, createdAt: CREATED_MAY_01 },
      { id: "new", position: 0, createdAt: CREATED_MAY_02 },
    ];
    expect([...tasks].sort(compareTasksByBoardOrder).map((task) => task.id)).toEqual([
      "new",
      "old",
    ]);
  });

  it("treats missing position as zero", () => {
    const tasks = [
      { id: "with", position: 1, createdAt: CREATED_MAY_01 },
      { id: "missing", createdAt: CREATED_MAY_02 },
    ];
    expect([...tasks].sort(compareTasksByBoardOrder).map((task) => task.id)).toEqual([
      "missing",
      "with",
    ]);
  });

  it("breaks remaining ties with id", () => {
    const tasks = [
      { id: "b", position: 0, createdAt: CREATED_MAY_01 },
      { id: "a", position: 0, createdAt: CREATED_MAY_01 },
    ];
    expect([...tasks].sort(compareTasksByBoardOrder).map((task) => task.id)).toEqual(["a", "b"]);
  });
});

describe("partitionKanbanColumnTasks", () => {
  it("splits admitted and queued overflow and sorts each group", () => {
    const tasks = [
      { id: "q2", position: 1, queuedForStepId: "step", wipAdmitted: false },
      { id: "a2", position: 1, createdAt: CREATED_MAY_01 },
      { id: "q1", position: 0, queuedForStepId: "step", wipAdmitted: false },
      { id: "a1", position: 0, createdAt: CREATED_MAY_02 },
    ];
    const { admitted, queued } = partitionKanbanColumnTasks(tasks);
    expect(admitted.map((task) => task.id)).toEqual(["a1", "a2"]);
    expect(queued.map((task) => task.id)).toEqual(["q1", "q2"]);
  });

  it("treats wipAdmitted true as admitted even when queuedForStepId is set", () => {
    const task = { id: "x", queuedForStepId: "step", wipAdmitted: true };
    expect(isAdmittedKanbanTask(task)).toBe(true);
    expect(isQueuedOverflowKanbanTask(task)).toBe(false);
  });
});

describe("sortIdsByCreatedDesc", () => {
  // d newest … a oldest → board order is d, c, b, a.
  const taskById = new Map<string, { createdAt?: string }>([
    ["a", { createdAt: "2026-01-01T00:00:00Z" }],
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

describe("sortIdsByBoardOrder", () => {
  it("orders by position then createdAt", () => {
    const taskById = new Map([
      ["a", { position: 1, createdAt: "2026-01-02T00:00:00Z" }],
      ["b", { position: 0, createdAt: "2026-01-01T00:00:00Z" }],
    ]);
    expect(sortIdsByBoardOrder(["a", "b"], taskById)).toEqual(["b", "a"]);
  });
});
