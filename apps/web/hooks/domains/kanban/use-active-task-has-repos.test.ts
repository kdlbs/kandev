import { describe, expect, it } from "vitest";
import { taskHasRepositories } from "./use-active-task-has-repos";

describe("taskHasRepositories", () => {
  it("treats legacy repositoryId as a repo association", () => {
    expect(taskHasRepositories({ repositoryId: "repo-a" })).toBe(true);
  });

  it("treats a non-empty repositories array as a repo association", () => {
    expect(
      taskHasRepositories({
        repositories: [
          {
            id: "link-a",
            repository_id: "repo-a",
            base_branch: "main",
            position: 0,
          },
        ],
      }),
    ).toBe(true);
  });

  it("returns false when both repository fields are absent", () => {
    expect(taskHasRepositories({})).toBe(false);
  });

  it("returns false for an empty repositories array with no repositoryId", () => {
    expect(taskHasRepositories({ repositories: [] })).toBe(false);
  });

  it("returns false for null input", () => {
    expect(taskHasRepositories(null)).toBe(false);
  });
});
