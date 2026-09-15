import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { ReactNode } from "react";
import type { DialogFormState } from "./task-create-dialog-types";
import type { Repository } from "@/lib/types/http";

const touchDrawer = vi.hoisted(() => ({ enabled: true }));

vi.mock("@/hooks/use-compact-task-chrome", () => ({
  useTouchDrawer: () => touchDrawer.enabled,
}));

vi.mock("@/hooks/domains/integrations/use-remote-repositories", () => ({
  useRemoteRepositories: () => ({
    repos: [],
    availableProviders: [],
    loading: false,
    unavailable: false,
    error: null,
    search: () => undefined,
  }),
}));

vi.mock("@/components/task/mobile/mobile-picker-sheet", () => ({
  MobilePickerSheet: ({
    open,
    title,
    headerAction,
    children,
  }: {
    open: boolean;
    title: string;
    headerAction?: ReactNode;
    children: ReactNode;
  }) =>
    open ? (
      <div data-testid="mobile-repository-sheet">
        <h2>{title}</h2>
        {headerAction}
        {children}
      </div>
    ) : null,
}));

vi.mock("@/components/task-create-dialog-repository-picker", () => ({
  RepositoryPicker: () => <div data-testid="task-repository-picker" />,
  AddRepositoryPicker: () => <div data-testid="legacy-add-repository-picker" />,
}));

vi.mock("@/components/task-create-dialog-workspace-repo-chips", () => ({
  RepoChip: ({
    row,
    repositoryLocked,
    onRemove,
  }: {
    row: { key: string };
    repositoryLocked?: boolean;
    onRemove?: () => void;
  }) => (
    <div data-testid="mixed-local-row">
      {row.key}
      {repositoryLocked ? null : (
        <button type="button" data-testid="mixed-local-remove" onClick={onRemove} />
      )}
    </div>
  ),
  collectExcludedRepoIds: () => new Set<string>(),
  collectSelectedRepoIdentities: () => new Set<string>(),
}));

vi.mock("@/components/task-create-dialog-repository-branch-data", () => ({
  useRepositoryBranchData: () => ({
    branches: [],
    branchesLoading: false,
    branchesLoaded: false,
    refreshBranches: undefined,
  }),
}));

vi.mock("@/components/task-create-dialog-remote-repo-chip", () => ({
  RemoteRepoChip: ({ row }: { row: { key: string } }) => (
    <div data-testid="mixed-remote-row">{row.key}</div>
  ),
  selectedRemoteRepositoryIdentity: () => null,
}));

vi.mock("@/components/folder-picker", () => ({
  FolderPicker: () => <div data-testid="folder-picker" />,
}));

import {
  buildLocalRepositorySelection,
  MixedRepositoryChips,
} from "./task-create-dialog-mixed-repository-chips";

afterEach(() => {
  cleanup();
  touchDrawer.enabled = true;
});

function makeFs(): DialogFormState {
  const local = { kind: "local" as const, key: "local-1", branch: "main" };
  return {
    repositorySelections: [local],
    repositorySelectionsTouched: false,
    repositories: [local],
    remoteRepos: [],
    discoveredRepositories: [],
    noRepository: false,
    workspacePath: "",
    currentLocalBranch: "main",
    currentLocalBranchLoading: false,
    freshBranchEnabled: false,
    branchesByUrl: {
      branches: () => [],
      loading: () => false,
      error: () => undefined,
      ensure: vi.fn(),
    },
    prInfoByUrl: {
      info: () => undefined,
      error: () => undefined,
      ensure: vi.fn(),
      inspection: () => undefined,
    },
    appendRepositorySelection: vi.fn(),
    setNoRepository: vi.fn(),
    addRepository: vi.fn(),
    addRemoteRepo: vi.fn(),
    removeRepository: vi.fn(),
    removeRemoteRepo: vi.fn(),
    updateRepository: vi.fn(),
    updateRemoteRepo: vi.fn(),
  } as unknown as DialogFormState;
}

function renderMixed(
  fs = makeFs(),
  options: { repositoryLocked?: boolean; branchLocked?: boolean } = {},
) {
  const props = {
    fs,
    repositories: fs.repositories as unknown as Repository[],
    workspaceId: "workspace-1",
    isLocalExecutor: true,
    onRowRepositoryChange: vi.fn(),
    onRowBranchChange: vi.fn(),
    ...options,
  } as React.ComponentProps<typeof MixedRepositoryChips>;
  return render(<MixedRepositoryChips {...props} />);
}

describe("MixedRepositoryChips on touch drawers", () => {
  it("opens one repository sheet and navigates between management and add views", () => {
    renderMixed();

    fireEvent.click(screen.getByTestId("mobile-repository-manager"));
    expect(screen.getByTestId("mobile-repository-sheet")).toBeTruthy();
    expect(screen.getByTestId("mixed-local-row")).toBeTruthy();
    expect(screen.getByTestId("mobile-repository-add")).toBeTruthy();

    fireEvent.click(screen.getByTestId("mobile-repository-add"));
    expect(screen.getByTestId("workspace-source-menu-options")).toBeTruthy();
    fireEvent.click(screen.getByTestId("workspace-source-menu-repository"));
    expect(screen.getByTestId("task-repository-picker")).toBeTruthy();
    expect(screen.getByTestId("mobile-repository-back")).toBeTruthy();

    fireEvent.click(screen.getByTestId("mobile-repository-back"));
    expect(screen.getByTestId("workspace-source-menu-options")).toBeTruthy();
    expect(screen.queryByTestId("task-repository-picker")).toBeNull();

    fireEvent.click(screen.getByTestId("mobile-repository-back"));
    expect(screen.getByTestId("mixed-local-row")).toBeTruthy();
    expect(screen.queryByTestId("task-repository-picker")).toBeNull();

    fireEvent.click(screen.getByTestId("mobile-repository-done"));
    expect(screen.queryByTestId("mobile-repository-sheet")).toBeNull();
  });

  it("selects the no-repository state when the final row is removed", () => {
    const fs = makeFs();
    renderMixed(fs);

    fireEvent.click(screen.getByTestId("mobile-repository-manager"));
    fireEvent.click(screen.getByTestId("mixed-local-remove"));

    expect(fs.removeRepository).toHaveBeenCalledWith("local-1");
    expect(fs.setNoRepository).toHaveBeenCalledWith(true);
  });

  it("keeps locked repository rows fixed and hides add controls", () => {
    renderMixed(makeFs(), { repositoryLocked: true, branchLocked: true });

    fireEvent.click(screen.getByTestId("mobile-repository-manager"));

    expect(screen.getByTestId("mixed-local-row")).toBeTruthy();
    expect(screen.queryByTestId("mixed-local-remove")).toBeNull();
    expect(screen.queryByTestId("mobile-repository-add")).toBeNull();
  });
});

describe("local repository selections", () => {
  it.each([
    [{ repositoryId: "repo-1", defaultBranch: "main" }, { repositoryId: "repo-1" }],
    [
      { localPath: "/checkouts/feature", defaultBranch: "main" },
      { localPath: "/checkouts/feature" },
    ],
  ])("leaves the branch unresolved for %j", (choice, expectedIdentity) => {
    expect(buildLocalRepositorySelection(choice)).toEqual({
      kind: "local",
      ...expectedIdentity,
      branch: "",
    });
  });

  it("uses the repository default for a non-local executor", () => {
    expect(
      buildLocalRepositorySelection({ repositoryId: "repo-1", defaultBranch: "develop" }, true),
    ).toEqual({ kind: "local", repositoryId: "repo-1", branch: "" });
    expect(
      buildLocalRepositorySelection({ repositoryId: "repo-1", defaultBranch: "develop" }, false),
    ).toEqual({ kind: "local", repositoryId: "repo-1", branch: "develop" });
  });
});
