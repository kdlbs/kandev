import { describe, expect, it } from "vitest";
import { mergeTaskRepositoryFields } from "./task-repositories";

const REPO_A = "repo-a";
const REPO_B = "repo-b";

function taskRepo(repositoryId: string) {
  return {
    id: `task-${repositoryId}`,
    repository_id: repositoryId,
    base_branch: "main",
    position: 0,
  };
}

describe("mergeTaskRepositoryFields", () => {
  it("derives repositoryId from provided repositories when repositoryId is omitted", () => {
    const repositories = [taskRepo(REPO_B)];

    expect(
      mergeTaskRepositoryFields(
        { repositoryId: REPO_A, repositories: undefined },
        { repositoryId: undefined, repositories },
      ),
    ).toEqual({ repositoryId: REPO_B, repositories });
  });

  it("clears repository fields when an empty repository list is provided", () => {
    expect(
      mergeTaskRepositoryFields(
        {
          repositoryId: REPO_A,
          repositories: [taskRepo(REPO_A)],
        },
        { repositoryId: undefined, repositories: [] },
      ),
    ).toEqual({ repositoryId: undefined, repositories: [] });
  });

  it("clears stale repositories when the primary repository changes", () => {
    expect(
      mergeTaskRepositoryFields(
        {
          repositoryId: REPO_A,
          repositories: [taskRepo(REPO_A)],
        },
        { repositoryId: REPO_B, repositories: undefined },
      ),
    ).toEqual({ repositoryId: REPO_B, repositories: undefined });
  });

  it("preserves existing repository fields when both fields are omitted", () => {
    const existing = {
      repositoryId: REPO_A,
      repositories: [taskRepo(REPO_A)],
    };

    expect(
      mergeTaskRepositoryFields(existing, { repositoryId: undefined, repositories: undefined }),
    ).toEqual(existing);
  });

  it("returns omitted fields when no existing task is available", () => {
    expect(
      mergeTaskRepositoryFields(undefined, { repositoryId: undefined, repositories: undefined }),
    ).toEqual({ repositoryId: undefined, repositories: undefined });
  });
});
