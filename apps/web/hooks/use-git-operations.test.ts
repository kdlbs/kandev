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

  it("sends exact contribution heads and one explicit repository scope", async () => {
    const executeOperation = vi.fn().mockResolvedValue({
      success: true,
      operation: "use_remote_contribution",
      output: "",
      recovery_branch: "kandev/recovery-123",
    }) as unknown as Parameters<typeof buildGitOperationCallbacks>[0];
    const operations = buildGitOperationCallbacks(executeOperation);
    const expectedHead = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb";

    await operations.replaceRemoteContribution(expectedHead, "");
    await operations.useRemoteContribution(expectedHead, "frontend");

    expect(executeOperation).toHaveBeenNthCalledWith(1, "worktree.replace_contribution", {
      expected_remote_head: expectedHead,
      repo: "",
    });
    expect(executeOperation).toHaveBeenNthCalledWith(2, "worktree.use_contribution", {
      expected_remote_head: expectedHead,
      repo: "frontend",
    });
  });
});
