import { describe, expect, it } from "vitest";
import type { DialogFormState } from "./task-create-dialog-types";
import {
  computeHasAllBranches,
  localRepositoryCreationEnabled,
} from "./task-create-dialog-prop-builders";

function formState(overrides: Partial<DialogFormState>): DialogFormState {
  return {
    noRepository: false,
    useRemote: false,
    repositories: [],
    remoteRepos: [],
    ...overrides,
  } as DialogFormState;
}

describe("computeHasAllBranches", () => {
  it("accepts no-repository tasks", () => {
    expect(computeHasAllBranches(formState({ noRepository: true }))).toBe(true);
  });

  it("requires a branch on every populated remote row", () => {
    expect(
      computeHasAllBranches(
        formState({
          useRemote: true,
          remoteRepos: [
            { key: "one", url: "https://example.com/one.git", branch: "main", source: "paste" },
            { key: "two", url: "https://example.com/two.git", branch: "", source: "paste" },
          ],
        }),
      ),
    ).toBe(false);
  });

  it("rejects remote mode without a populated row", () => {
    expect(computeHasAllBranches(formState({ useRemote: true }))).toBe(false);
  });

  it("accepts a branched remote row and ignores an empty trailing row", () => {
    expect(
      computeHasAllBranches(
        formState({
          useRemote: true,
          remoteRepos: [
            { key: "one", url: "https://example.com/one.git", branch: "main", source: "paste" },
            { key: "two", url: "", branch: "", source: "paste" },
          ],
        }),
      ),
    ).toBe(true);
  });

  it("rejects local mode without a repository row", () => {
    expect(computeHasAllBranches(formState({}))).toBe(false);
  });

  it("rejects a local repository row without a branch", () => {
    expect(
      computeHasAllBranches(
        formState({ repositories: [{ key: "one", repositoryId: "repo-one", branch: "" }] }),
      ),
    ).toBe(false);
  });

  it("requires a branch on every local repository row", () => {
    expect(
      computeHasAllBranches(
        formState({
          repositories: [
            { key: "one", repositoryId: "repo-one", branch: "main" },
            { key: "two", repositoryId: "repo-two", branch: "develop" },
          ],
        }),
      ),
    ).toBe(true);
  });
});

describe("localRepositoryCreationEnabled", () => {
  it("allows repository creation only for an unlocked new-task form", () => {
    expect(localRepositoryCreationEnabled(true, false)).toBe(true);
    expect(localRepositoryCreationEnabled(false, false)).toBe(false);
    expect(localRepositoryCreationEnabled(true, true)).toBe(false);
  });
});
