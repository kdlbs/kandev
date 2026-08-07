import { describe, expect, it, vi } from "vitest";
import {
  buildGitOperationCallbacks,
  getChangeRequestTerminology,
  repositoryScopePayload,
} from "./use-git-operations";

describe("getChangeRequestTerminology", () => {
  it("uses merge request terminology for GitLab", () => {
    expect(getChangeRequestTerminology("gitlab")).toEqual({
      longName: "Merge Request",
      shortName: "MR",
    });
  });

  it("keeps pull request terminology for other providers", () => {
    expect(getChangeRequestTerminology("github")).toEqual({
      longName: "Pull Request",
      shortName: "PR",
    });
  });
});

describe("repository scope payloads", () => {
  it("keeps an explicit empty repository name", () => {
    expect(repositoryScopePayload("")).toEqual({ repo: "" });
    expect(repositoryScopePayload(undefined)).toEqual({});
  });

  it("sends the root sentinel to scoped git operations", async () => {
    const executeOperation = vi.fn() as unknown as Parameters<typeof buildGitOperationCallbacks>[0];
    const operations = buildGitOperationCallbacks(executeOperation);

    await operations.stage(undefined, "");

    expect(executeOperation).toHaveBeenCalledWith("worktree.stage", {
      paths: [],
      repo: "",
    });
  });
});
