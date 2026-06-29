import { describe, expect, it } from "vitest";
import { compareTasksByCreatedDesc, sortIdsByCreatedDesc } from "./task-order";

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

  it("keeps relative order for ids without a known task", () => {
    expect(sortIdsByCreatedDesc(["zzz", "a"], taskById)).toEqual(["zzz", "a"]);
  });
});
