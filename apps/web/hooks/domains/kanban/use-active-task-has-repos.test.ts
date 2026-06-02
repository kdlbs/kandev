import { describe, expect, it } from "vitest";
import { taskHasRepositories } from "./use-active-task-has-repos";

describe("taskHasRepositories", () => {
  it("treats legacy repositoryId as a repo association", () => {
    expect(taskHasRepositories({ repositoryId: "repo-a" })).toBe(true);
  });
});
