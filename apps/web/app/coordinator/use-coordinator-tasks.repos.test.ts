import { describe, expect, it } from "vitest";
import { taskRepositoryIds } from "./use-coordinator-tasks";

describe("taskRepositoryIds", () => {
  it("prefers the repository list", () => {
    expect(
      taskRepositoryIds({
        repositories: [{ repository_id: "a" }, { repository_id: "b" }],
        repositoryId: "a",
      }),
    ).toEqual(["a", "b"]);
  });

  it("falls back to the primary repository id", () => {
    expect(taskRepositoryIds({ repositoryId: "a" })).toEqual(["a"]);
    expect(taskRepositoryIds({ repositories: [], repositoryId: "a" })).toEqual(["a"]);
  });

  it("is empty for a task with no repository", () => {
    expect(taskRepositoryIds({})).toEqual([]);
  });
});
