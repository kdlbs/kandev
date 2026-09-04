import { describe, expect, it } from "vitest";
import {
  compareStepOrder,
  compareTasksByCreatedDesc,
  sortIdsByCreatedDesc,
  type StepOrderTask,
} from "./task-order";

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

function stepOrderTask(overrides: Partial<StepOrderTask> = {}): StepOrderTask {
  return {
    id: "task",
    position: 0,
    priority: "medium",
    queuedAt: "2026-08-12T10:00:00Z",
    createdAt: "2026-08-12T09:00:00Z",
    ...overrides,
  };
}

describe("compareStepOrder", () => {
  it("orders by position first", () => {
    const tasks = [
      stepOrderTask({ id: "b", position: 2 }),
      stepOrderTask({ id: "a", position: 1 }),
    ];
    expect([...tasks].sort(compareStepOrder).map((t) => t.id)).toEqual(["a", "b"]);
  });

  it("breaks a position tie by priority rank, highest first", () => {
    const tasks = [
      stepOrderTask({ id: "low", position: 1, priority: "low" }),
      stepOrderTask({ id: "critical", position: 1, priority: "critical" }),
    ];
    expect([...tasks].sort(compareStepOrder).map((t) => t.id)).toEqual(["critical", "low"]);
  });

  it("breaks a priority tie by queuedAt ascending, falling back to createdAt when absent", () => {
    const neverQueued = stepOrderTask({
      id: "never-queued",
      queuedAt: null,
      createdAt: "2026-08-12T07:00:00Z",
    });
    const queuedLater = stepOrderTask({ id: "queued-later", queuedAt: "2026-08-12T09:00:00Z" });
    expect([queuedLater, neverQueued].sort(compareStepOrder).map((t) => t.id)).toEqual([
      "never-queued",
      "queued-later",
    ]);
  });

  it("breaks a queuedAt tie by createdAt ascending, then id ascending", () => {
    const tasks = [
      stepOrderTask({ id: "z", createdAt: "2026-08-12T08:00:00Z" }),
      stepOrderTask({ id: "a", createdAt: "2026-08-12T08:00:00Z" }),
    ];
    expect([...tasks].sort(compareStepOrder).map((t) => t.id)).toEqual(["a", "z"]);
  });

  it("treats an absent priority as ranking after every named priority", () => {
    const tasks = [
      stepOrderTask({ id: "absent", position: 1, priority: undefined }),
      stepOrderTask({ id: "low", position: 1, priority: "low" }),
    ];
    expect([...tasks].sort(compareStepOrder).map((t) => t.id)).toEqual(["low", "absent"]);
  });
});
