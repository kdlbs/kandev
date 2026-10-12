import { describe, expect, it } from "vitest";
import { filterTasksByRepositories } from "./filters";

const tasks = [
  {
    id: "multi",
    repositoryId: "backend",
    repositories: [
      { id: "backend-link", repository_id: "backend" },
      { id: "web-link", repository_id: "web" },
    ],
  },
  { id: "single", repositoryId: "web", repositories: [{ repository_id: "web" }] },
  { id: "legacy", repositoryId: "web" },
  { id: "empty", repositoryId: "web", repositories: [] },
  { id: "contradictory", repositoryId: "web", repositories: [{ repository_id: "other" }] },
  { id: "collection-only", repositories: [{ repository_id: "web" }] },
  { id: "unlinked" },
];

// @covers AC-UI-BOARD-REPOSITORY-MATCHING-001.1, AC-UI-BOARD-REPOSITORY-MATCHING-001.2
describe("filterTasksByRepositories", () => {
  it.each([
    { selected: ["web"], expected: ["multi", "single", "legacy", "collection-only"] },
    { selected: ["backend"], expected: ["multi"] },
    { selected: ["backend", "web"], expected: ["multi", "single", "legacy", "collection-only"] },
    { selected: ["missing"], expected: [] },
    { selected: ["web-link"], expected: [] },
    { selected: ["other"], expected: ["contradictory"] },
  ])("matches repository IDs in $selected with collection authority", ({ selected, expected }) => {
    const original = structuredClone(tasks);
    const result = filterTasksByRepositories(tasks, new Set(selected));
    expect(result.map((task) => task.id)).toEqual(expected);
    expect(tasks).toEqual(original);
    for (const task of result) expect(task).toBe(tasks.find((entry) => entry.id === task.id));
  });

  // @covers AC-UI-BOARD-REPOSITORY-MATCHING-001.3
  it("preserves every task and the array reference with no repository selection", () => {
    expect(filterTasksByRepositories(tasks, new Set())).toBe(tasks);
  });
});
