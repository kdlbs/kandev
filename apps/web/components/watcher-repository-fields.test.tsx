import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { TooltipProvider } from "@kandev/ui/tooltip";
import type { Branch } from "@/lib/types/http";
import { WatcherRepositoryFields } from "./watcher-repository-fields";

const branchState = vi.hoisted(() => ({
  branches: [] as Branch[],
  loading: false,
  refresh: vi.fn(),
}));
const touchState = vi.hoisted(() => ({ enabled: false }));

vi.mock("@/hooks/domains/workspace/use-repositories", () => ({
  useRepositories: () => ({ repositories: [] }),
}));

vi.mock("@/hooks/domains/workspace/use-repository-branches", () => ({
  useBranches: () => ({
    branches: branchState.branches,
    isLoading: branchState.loading,
    refresh: branchState.refresh,
  }),
}));

vi.mock("@/hooks/use-compact-task-chrome", () => ({
  useTouchDrawer: () => touchState.enabled,
}));

afterEach(() => {
  cleanup();
  branchState.branches = [];
  branchState.loading = false;
  branchState.refresh.mockReset();
  touchState.enabled = false;
});

const WORKSPACE_ID = "workspace-1";
const REPOSITORY_ID = "repository-1";
const BASE_BRANCH_LABEL = "Base Branch";
const QUALIFIED_BRANCH = "origin/main";

type FieldProps = {
  workspaceId: string;
  repositoryId: string;
  baseBranch: string;
  onRepositoryChange: (repositoryId: string) => void;
  onBaseBranchChange: (baseBranch: string) => void;
};

function renderFields(overrides: Partial<FieldProps> = {}) {
  const props: FieldProps = {
    workspaceId: WORKSPACE_ID,
    repositoryId: REPOSITORY_ID,
    baseBranch: "",
    onRepositoryChange: vi.fn(),
    onBaseBranchChange: vi.fn(),
    ...overrides,
  };
  return render(
    <TooltipProvider>
      <WatcherRepositoryFields {...props} />
    </TooltipProvider>,
  );
}

describe("WatcherRepositoryFields", () => {
  it("shows the repository-first placeholder while the branch selector is disabled", () => {
    renderFields({ repositoryId: "" });

    expect(screen.getByLabelText(BASE_BRANCH_LABEL).textContent).toContain(
      "Pick a repository first",
    );
  });

  it("shows the loading placeholder instead of selecting the default branch", () => {
    branchState.loading = true;
    renderFields();

    expect(screen.getByLabelText(BASE_BRANCH_LABEL).textContent).toContain("Loading…");
  });

  it("keeps local and qualified remote branches as distinct searchable choices", () => {
    branchState.branches = [
      { name: "main", type: "local" },
      { name: "main", type: "remote", remote: "origin" },
    ];

    renderFields();

    fireEvent.click(screen.getByRole("combobox", { name: BASE_BRANCH_LABEL }));

    expect(screen.queryByRole("option", { name: /^origin\/main origin/ })).toBeTruthy();
  });

  it("deduplicates exact refs, keeps provider branches short, filters unsupported remotes, and refreshes", () => {
    const onBaseBranchChange = vi.fn();
    branchState.branches = [
      { name: "main", type: "local" },
      { name: "main", type: "remote", remote: "origin" },
      { name: "main", type: "remote", remote: "origin" },
      { name: "provider/default", type: "remote" },
      { name: "release/candidate", type: "remote", remote: "upstream" },
    ];

    renderFields({ onBaseBranchChange });

    fireEvent.click(screen.getByRole("combobox", { name: BASE_BRANCH_LABEL }));

    expect(screen.getAllByRole("option", { name: /^main local/ })).toHaveLength(1);
    expect(screen.getAllByRole("option", { name: /^origin\/main origin/ })).toHaveLength(1);
    expect(screen.getByRole("option", { name: /^provider\/default remote/ })).toBeTruthy();
    expect(
      screen.queryByRole("option", { name: /^upstream\/release\/candidate upstream/ }),
    ).toBeNull();

    fireEvent.change(screen.getByPlaceholderText("Search branches..."), {
      target: { value: QUALIFIED_BRANCH },
    });
    expect(screen.getByRole("option", { name: /^origin\/main origin/ })).toBeTruthy();
    expect(screen.queryByRole("option", { name: /^main local/ })).toBeNull();

    fireEvent.change(screen.getByPlaceholderText("Search branches..."), { target: { value: "" } });
    fireEvent.click(screen.getByRole("option", { name: /^origin\/main origin/ }));
    expect(onBaseBranchChange).toHaveBeenCalledWith(QUALIFIED_BRANCH);

    fireEvent.click(screen.getByRole("combobox", { name: BASE_BRANCH_LABEL }));
    fireEvent.click(screen.getByTestId("branch-refresh-button"));
    expect(branchState.refresh).toHaveBeenCalledOnce();
  });

  it("keeps a stored branch visible when the current branch response omits it", () => {
    branchState.branches = [{ name: "main", type: "local" }];

    renderFields({ baseBranch: QUALIFIED_BRANCH });

    expect(screen.getByRole("combobox", { name: BASE_BRANCH_LABEL }).textContent).toContain(
      QUALIFIED_BRANCH,
    );
    fireEvent.click(screen.getByRole("combobox", { name: BASE_BRANCH_LABEL }));
    expect(screen.getByRole("option", { name: /^origin\/main origin/ })).toBeTruthy();
  });

  it("associates the base branch label with its trigger", () => {
    renderFields();

    const label = screen.getByText(BASE_BRANCH_LABEL, { exact: true });
    const trigger = screen.getByRole("combobox", { name: BASE_BRANCH_LABEL });

    expect(label.getAttribute("for")).toBeTruthy();
    expect(label.getAttribute("for")).toBe(trigger.getAttribute("id"));
  });

  it("maps the repository default choice back to an empty stored branch", () => {
    const onBaseBranchChange = vi.fn();
    branchState.branches = [{ name: "main", type: "local" }];

    renderFields({ baseBranch: "main", onBaseBranchChange });

    fireEvent.click(screen.getByRole("combobox", { name: BASE_BRANCH_LABEL }));
    fireEvent.click(screen.getByRole("option", { name: "(repository default branch)" }));
    expect(onBaseBranchChange).toHaveBeenCalledWith("");
  });

  it("uses touch-sized watcher controls on coarse pointers", () => {
    touchState.enabled = true;
    branchState.branches = [{ name: "main", type: "local" }];

    renderFields();

    const trigger = screen.queryByTestId("watcher-base-branch-selector");
    expect(trigger).toBeTruthy();
    expect(trigger?.getAttribute("aria-label")).toBe(BASE_BRANCH_LABEL);
    expect(trigger?.className).toContain("min-h-12");
    fireEvent.click(trigger!);
    expect(screen.getByRole("option", { name: /^main local/ }).className).toContain("sm:min-h-12");
    expect(screen.getByTestId("branch-refresh-button").className).toContain("h-12 w-12");
  });
});
