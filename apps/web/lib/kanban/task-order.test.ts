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
});
