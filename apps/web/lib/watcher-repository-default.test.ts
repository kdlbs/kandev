import { describe, it, expect } from "vitest";
import {
  NO_REPOSITORY,
  DEFAULT_BRANCH,
  DEFAULT_BRANCH_LABEL,
  resolveRepositoryId,
  resolveBaseBranch,
  clearWorkspaceScopedForm,
  branchPlaceholder,
} from "./watcher-repository-default";

describe("resolveRepositoryId", () => {
  it("collapses the no-repository sentinel to an empty string", () => {
    expect(resolveRepositoryId(NO_REPOSITORY)).toBe("");
  });
  it("passes a real repository id through unchanged", () => {
    expect(resolveRepositoryId("repo-123")).toBe("repo-123");
  });
});

describe("resolveBaseBranch", () => {
  it("collapses the default-branch sentinel to an empty string", () => {
    expect(resolveBaseBranch(DEFAULT_BRANCH)).toBe("");
  });
  it("passes a real branch name through unchanged", () => {
    expect(resolveBaseBranch("main")).toBe("main");
  });
});

describe("branchPlaceholder", () => {
  it("prompts to pick a repository first when none is selected", () => {
    expect(branchPlaceholder("", false)).toBe("Pick a repository first");
    // No repo takes precedence over the loading state.
    expect(branchPlaceholder("", true)).toBe("Pick a repository first");
  });
  it("shows a loading hint while branches stream in", () => {
    expect(branchPlaceholder("repo-1", true)).toBe("Loading…");
  });
  it("shows the default-branch label once a repo is selected and loaded", () => {
    expect(branchPlaceholder("repo-1", false)).toBe(DEFAULT_BRANCH_LABEL);
  });
});

describe("clearWorkspaceScopedForm", () => {
  const base = {
    workspaceId: "ws-1",
    workflowId: "wf-1",
    workflowStepId: "step-1",
    repositoryId: "repo-1",
    baseBranch: "main",
    // an unrelated field that must be preserved
    prompt: "keep me",
  };

  it("clears workflow/step + repository binding when the workspace changes", () => {
    const next = clearWorkspaceScopedForm(base, "ws-2");
    expect(next).toEqual({
      workspaceId: "ws-2",
      workflowId: "",
      workflowStepId: "",
      repositoryId: "",
      baseBranch: "",
      prompt: "keep me",
    });
  });

  it("returns the previous object unchanged when the workspace is the same", () => {
    const next = clearWorkspaceScopedForm(base, "ws-1");
    // Same reference: no-op so the user's selections are preserved.
    expect(next).toBe(base);
  });
});
