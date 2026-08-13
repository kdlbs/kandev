import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { WatcherRepositoryFields } from "./watcher-repository-fields";

const branchState = vi.hoisted(() => ({ loading: false }));

vi.mock("@/hooks/domains/workspace/use-repositories", () => ({
  useRepositories: () => ({ repositories: [] }),
}));

vi.mock("@/hooks/domains/workspace/use-repository-branches", () => ({
  useBranches: () => ({ branches: [], isLoading: branchState.loading }),
}));

afterEach(() => {
  cleanup();
  branchState.loading = false;
});

describe("WatcherRepositoryFields", () => {
  it("shows the repository-first placeholder while the branch selector is disabled", () => {
    render(
      <WatcherRepositoryFields
        workspaceId="workspace-1"
        repositoryId=""
        baseBranch=""
        onRepositoryChange={vi.fn()}
        onBaseBranchChange={vi.fn()}
      />,
    );

    expect(screen.getByLabelText("Base Branch").textContent).toContain("Pick a repository first");
  });

  it("shows the loading placeholder instead of selecting the default branch", () => {
    branchState.loading = true;
    render(
      <WatcherRepositoryFields
        workspaceId="workspace-1"
        repositoryId="repository-1"
        baseBranch=""
        onRepositoryChange={vi.fn()}
        onBaseBranchChange={vi.fn()}
      />,
    );

    expect(screen.getByLabelText("Base Branch").textContent).toContain("Loading…");
  });
});
