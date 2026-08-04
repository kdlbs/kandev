import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { Branch, Repository } from "@/lib/types/http";
import {
  WatcherRepositoryMultiFields,
  type WatcherRepoBinding,
} from "./watcher-repository-multi-fields";
import { useRepositories } from "@/hooks/domains/workspace/use-repositories";
import { useBranches } from "@/hooks/domains/workspace/use-repository-branches";

// The component renders every label through t(); the mock resolves the linear
// watcher keys to their English copy so assertions read like the real UI.
const { COPY } = vi.hoisted(() => ({
  COPY: {
    "linear:watcher.repositories": "Repositories",
    "linear:watcher.repositoriesDescription":
      "Optional — the repositories the agent works in. Select one or more.",
    "linear:watcher.noRepositories": "No repositories available in this workspace.",
    "linear:watcher.addRepository": "Add repository",
    "linear:watcher.selectRepository": "Select a repository…",
    "linear:watcher.repository": "Repository",
    "linear:watcher.baseBranch": "Base branch",
    "linear:watcher.defaultBranch": "(repository default branch)",
    "linear:watcher.removeRepository": "Remove repository",
    "linear:watcher.loadingBranches": "Loading…",
  } as Record<string, string>,
}));

vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string) => COPY[key] ?? key }),
}));
vi.mock("@/hooks/domains/workspace/use-repositories", () => ({
  useRepositories: vi.fn(),
}));
vi.mock("@/hooks/domains/workspace/use-repository-branches", () => ({
  useBranches: vi.fn(),
}));

function repo(id: string, name: string): Repository {
  return { id, name } as Repository;
}

function branch(name: string): { name: string; type: "local" } {
  return { name, type: "local" };
}

// Hoisted fixtures keep the repeated binding literals in one place.
const REPO_A: Repository = repo("repo-a", "Alpha");
const REPO_B: Repository = repo("repo-b", "Beta");
const REPO_A_MAIN: WatcherRepoBinding = { repositoryId: "repo-a", baseBranch: "main" };
const REPO_B_DEVELOP: WatcherRepoBinding = { repositoryId: "repo-b", baseBranch: "develop" };
const ADD_REPO_TRIGGER = "add-repository-trigger";

function renderPicker(
  bindings: WatcherRepoBinding[],
  repositories: Repository[],
  branches: Branch[],
  onChange = vi.fn(),
) {
  vi.mocked(useRepositories).mockReturnValue({
    repositories,
    isLoading: false,
    refresh: vi.fn(),
  });
  vi.mocked(useBranches).mockReturnValue({ branches, isLoading: false });
  render(
    <WatcherRepositoryMultiFields workspaceId="ws-1" bindings={bindings} onChange={onChange} />,
  );
  return onChange;
}

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

describe("WatcherRepositoryMultiFields", () => {
  it("renders one row per binding with the repo label and a branch select", () => {
    renderPicker(
      [REPO_A_MAIN, REPO_B_DEVELOP],
      [REPO_A, REPO_B],
      [branch("main"), branch("develop")],
    );
    expect(screen.getByText("Alpha")).toBeTruthy();
    expect(screen.getByText("Beta")).toBeTruthy();
    expect(screen.getByTestId("branch-trigger-repo-a")).toBeTruthy();
    expect(screen.getByTestId("branch-trigger-repo-b")).toBeTruthy();
    expect(screen.getAllByLabelText("Remove repository")).toHaveLength(2);
  });

  it("lists only repositories that are not already bound in the add control", () => {
    renderPicker([REPO_A_MAIN], [REPO_A, REPO_B], []);
    fireEvent.click(screen.getByTestId(ADD_REPO_TRIGGER));
    expect(screen.getByRole("option", { name: "Beta" })).toBeTruthy();
    expect(screen.queryByRole("option", { name: "Alpha" })).toBeNull();
  });

  it("appends a new binding with the repo's default branch when added", () => {
    const onChange = renderPicker([], [REPO_A, REPO_B], []);
    fireEvent.click(screen.getByTestId(ADD_REPO_TRIGGER));
    fireEvent.click(screen.getByRole("option", { name: "Alpha" }));
    expect(onChange).toHaveBeenCalledWith([{ repositoryId: "repo-a", baseBranch: "" }]);
  });

  it("updates only the matching row's branch", () => {
    const onChange = renderPicker(
      [REPO_A_MAIN, REPO_B_DEVELOP],
      [REPO_A, REPO_B],
      [branch("main"), branch("develop")],
    );
    fireEvent.click(screen.getByTestId("branch-trigger-repo-a"));
    fireEvent.click(screen.getByRole("option", { name: "develop" }));
    expect(onChange).toHaveBeenCalledWith([
      { repositoryId: "repo-a", baseBranch: "develop" },
      REPO_B_DEVELOP,
    ]);
  });

  it("removes the row when its remove button is clicked", () => {
    const onChange = renderPicker([REPO_A_MAIN, REPO_B_DEVELOP], [REPO_A, REPO_B], []);
    fireEvent.click(screen.getByTestId("remove-repo-repo-a"));
    expect(onChange).toHaveBeenCalledWith([REPO_B_DEVELOP]);
  });

  it("shows the add control and a hint when the workspace has no repositories", () => {
    renderPicker([], [], []);
    expect(screen.getByText("No repositories available in this workspace.")).toBeTruthy();
    expect(screen.queryByTestId(ADD_REPO_TRIGGER)).toBeNull();
  });

  it("hides the add control once every repository is bound", () => {
    renderPicker([REPO_A_MAIN], [REPO_A], []);
    expect(screen.queryByTestId(ADD_REPO_TRIGGER)).toBeNull();
  });
});
