import { describe, expect, it } from "vitest";
import { compareTasksByCreatedDesc } from "./task-order";

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
});
