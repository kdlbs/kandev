import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { TooltipProvider } from "@kandev/ui/tooltip";
import type { RemoteRepository } from "@/hooks/domains/integrations/use-remote-repositories";
import type { GitHubWorkspaceSettings, RepoFilter } from "@/lib/types/github";

const mocks = vi.hoisted(() => ({
  listUserOrgs: vi.fn(),
  fetchGitHubWorkspaceSettings: vi.fn(),
  useRemoteRepositories: vi.fn(),
}));

vi.mock("@/lib/api/domains/github-api", () => ({
  listUserOrgs: mocks.listUserOrgs,
  fetchGitHubWorkspaceSettings: mocks.fetchGitHubWorkspaceSettings,
}));
vi.mock("@/hooks/domains/integrations/use-remote-repositories", () => ({
  useRemoteRepositories: mocks.useRemoteRepositories,
}));

import { RepoFilterSelector } from "./repo-filter-selector";

function repo(owner: string, name: string): RemoteRepository {
  return {
    provider: "github",
    id: `github.com/${owner}/${name}`,
    owner,
    name,
    fullName: `${owner}/${name}`,
    url: `https://github.com/${owner}/${name}`,
    defaultBranch: "main",
    private: false,
  };
}

const ACCESSIBLE = [repo("kdlbs", "kandev"), repo("acme", "api")];
let query = "";

function scope(overrides: Partial<GitHubWorkspaceSettings> = {}): GitHubWorkspaceSettings {
  return { workspace_id: "ws-1", repo_scope_mode: "all", ...overrides } as GitHubWorkspaceSettings;
}

beforeEach(() => {
  query = "";
  mocks.listUserOrgs.mockResolvedValue({ orgs: [] });
  mocks.fetchGitHubWorkspaceSettings.mockResolvedValue(scope());
  mocks.useRemoteRepositories.mockImplementation(() => ({
    repos: ACCESSIBLE.filter((r) => r.fullName.toLowerCase().includes(query.toLowerCase())),
    availableProviders: ["github"],
    loading: false,
    error: null,
    unavailable: false,
    search: (value: string) => {
      query = value;
    },
  }));
});

afterEach(() => {
  cleanup();
  vi.resetAllMocks();
});

function renderSelector(selectedRepos: RepoFilter[] = []) {
  const onSelectedReposChange = vi.fn();
  render(
    <TooltipProvider>
      <RepoFilterSelector
        allRepos={false}
        selectedRepos={selectedRepos}
        onAllReposChange={vi.fn()}
        onSelectedReposChange={onSelectedReposChange}
        workspaceId="ws-1"
      />
    </TooltipProvider>,
  );
  return onSelectedReposChange;
}

function openPicker() {
  fireEvent.click(screen.getByRole("button", { name: /add repository/i }));
}

function optionTexts() {
  return screen.queryAllByTestId("remote-repo-option").map((option) => option.textContent ?? "");
}

describe("RepoFilterSelector repository picker", () => {
  it("lists accessible repositories on open and matches a bare repository name", async () => {
    const onChange = renderSelector();
    openPicker();
    expect(optionTexts()).toEqual([
      expect.stringContaining("kdlbs/kandev"),
      expect.stringContaining("acme/api"),
    ]);

    fireEvent.change(screen.getByTestId("remote-repo-input"), { target: { value: "kandev" } });
    await waitFor(() => expect(optionTexts()).toEqual([expect.stringContaining("kdlbs/kandev")]));
    fireEvent.click(screen.getByTestId("remote-repo-option"));

    expect(onChange).toHaveBeenCalledWith([{ owner: "kdlbs", name: "kandev" }]);
  });

  it("requests GitHub repositories only", () => {
    renderSelector();
    expect(mocks.useRemoteRepositories).toHaveBeenCalledWith("ws-1", { providers: ["github"] });
  });

  it("offers only repositories allowed by a repos-mode workspace scope", async () => {
    mocks.fetchGitHubWorkspaceSettings.mockResolvedValue(
      scope({ repo_scope_mode: "repos", repo_scope_repos: [{ owner: "acme", name: "api" }] }),
    );
    renderSelector();
    await waitFor(() => expect(mocks.fetchGitHubWorkspaceSettings).toHaveBeenCalled());
    openPicker();
    await waitFor(() => expect(optionTexts()).toEqual([expect.stringContaining("acme/api")]));
  });

  it("adds a typed owner/name that is not listed", () => {
    const onChange = renderSelector();
    openPicker();
    const input = screen.getByTestId("remote-repo-input");
    fireEvent.change(input, { target: { value: "other/tool" } });
    fireEvent.keyDown(input, { key: "Enter" });
    expect(onChange).toHaveBeenCalledWith([{ owner: "other", name: "tool" }]);
  });

  it("rejects a typed repository outside the workspace scope", async () => {
    mocks.fetchGitHubWorkspaceSettings.mockResolvedValue(
      scope({ repo_scope_mode: "orgs", repo_scope_orgs: ["kdlbs"] }),
    );
    const onChange = renderSelector();
    await waitFor(() => expect(mocks.fetchGitHubWorkspaceSettings).toHaveBeenCalled());
    openPicker();
    const input = screen.getByTestId("remote-repo-input");
    fireEvent.change(input, { target: { value: "other/tool" } });
    fireEvent.keyDown(input, { key: "Enter" });
    expect(onChange).not.toHaveBeenCalled();
    expect(screen.getByRole("alert")).toBeTruthy();
  });

  it("marks a selected repository and does not add it twice", () => {
    const onChange = renderSelector([{ owner: "kdlbs", name: "kandev" }]);
    openPicker();
    expect(screen.getAllByTestId("already-added-repository-marker")).toHaveLength(1);
    fireEvent.click(screen.getAllByTestId("remote-repo-option")[0]!);
    expect(onChange).not.toHaveBeenCalled();
  });

  it("hides the organization picker in repos mode and keeps saved organization entries removable", async () => {
    mocks.fetchGitHubWorkspaceSettings.mockResolvedValue(
      scope({ repo_scope_mode: "repos", repo_scope_repos: [{ owner: "acme", name: "api" }] }),
    );
    const onChange = renderSelector([
      { owner: "kdlbs", name: "kandev" },
      { owner: "acme", name: "" },
    ]);
    await waitFor(() => expect(screen.queryByTestId("github-organization-add")).toBeNull());
    fireEvent.click(screen.getByRole("button", { name: /remove acme\/\*/i }));
    expect(onChange).toHaveBeenCalledWith([{ owner: "kdlbs", name: "kandev" }]);
  });
});

const org = (login: string) => ({ login, avatar_url: "" });

function orgOptionTexts() {
  return screen.queryAllByTestId("github-organization-option").map((o) => o.textContent ?? "");
}

describe("RepoFilterSelector organization picker", () => {
  it("adds an organization as an owner-wide filter", async () => {
    mocks.listUserOrgs.mockResolvedValue({ orgs: [org("octocat"), org("kdlbs")] });
    const onChange = renderSelector([{ owner: "acme", name: "api" }]);
    fireEvent.click(screen.getByTestId("github-organization-add"));
    await waitFor(() => expect(orgOptionTexts()).toEqual(["octocat", "kdlbs"]));
    fireEvent.click(screen.getAllByTestId("github-organization-option")[1]!);
    expect(onChange).toHaveBeenCalledWith([
      { owner: "kdlbs", name: "" },
      { owner: "acme", name: "api" },
    ]);
  });

  it("separates saved organizations from repositories and removes an organization", () => {
    const onChange = renderSelector([
      { owner: "kdlbs", name: "kandev" },
      { owner: "acme", name: "" },
    ]);
    expect(screen.getAllByTestId("github-organization-chip").map((c) => c.textContent)).toEqual([
      "acme",
    ]);
    expect(screen.getAllByTestId("github-repository-chip").map((c) => c.textContent)).toEqual([
      "kdlbs/kandev",
    ]);
    fireEvent.click(screen.getByRole("button", { name: /remove acme/i }));
    expect(onChange).toHaveBeenCalledWith([{ owner: "kdlbs", name: "kandev" }]);
  });

  it("offers only organizations allowed by an orgs-mode workspace scope", async () => {
    mocks.listUserOrgs.mockResolvedValue({ orgs: [org("octocat"), org("kdlbs"), org("acme")] });
    mocks.fetchGitHubWorkspaceSettings.mockResolvedValue(
      scope({ repo_scope_mode: "orgs", repo_scope_orgs: ["kdlbs"] }),
    );
    renderSelector();
    await waitFor(() => expect(mocks.fetchGitHubWorkspaceSettings).toHaveBeenCalled());
    fireEvent.click(screen.getByTestId("github-organization-add"));
    await waitFor(() => expect(orgOptionTexts()).toEqual(["kdlbs"]));
  });
});
