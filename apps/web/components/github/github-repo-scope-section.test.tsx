import { act, cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { SettingsSaveProvider } from "@/components/settings/settings-save-provider";
import type { RemoteRepository } from "@/hooks/domains/integrations/use-remote-repositories";
import type { GitHubWorkspaceSettings } from "@/lib/types/github";

const mocks = vi.hoisted(() => ({
  fetchGitHubWorkspaceSettings: vi.fn(),
  updateGitHubWorkspaceSettings: vi.fn(),
  useRemoteRepositories: vi.fn(),
  listUserOrgs: vi.fn(),
  toast: vi.fn(),
}));

vi.mock("@/components/toast-provider", () => ({ useToast: () => ({ toast: mocks.toast }) }));
vi.mock("@/lib/api/domains/github-api", () => ({
  fetchGitHubWorkspaceSettings: mocks.fetchGitHubWorkspaceSettings,
  updateGitHubWorkspaceSettings: mocks.updateGitHubWorkspaceSettings,
  listUserOrgs: mocks.listUserOrgs,
}));
vi.mock("@/hooks/domains/integrations/use-remote-repositories", () => ({
  useRemoteRepositories: mocks.useRemoteRepositories,
}));

import { GitHubRepoScopeSection } from "./github-repo-scope-section";

const ORG_ADD = "github-organization-add";
const SAVE_BAR = "settings-floating-save";
const SAVE_CHANGES = "Save changes";
const ALL_REPOSITORIES = "All repositories";
const EXACT_REPOSITORY = "outside/tool";

function expectAllRepositoriesChecked(checked: boolean) {
  expect(screen.getByRole("switch", { name: ALL_REPOSITORIES }).getAttribute("aria-checked")).toBe(
    String(checked),
  );
}

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

const SAVED: GitHubWorkspaceSettings = {
  workspace_id: "ws-1",
  github_host: "github.com",
  repo_scope_mode: "selected",
  repo_scope_orgs: [],
  repo_scope_repos: [{ owner: "kdlbs", name: "kandev" }],
  created_at: "2026-01-01T00:00:00Z",
  updated_at: "2026-01-01T00:00:00Z",
};

const UNION_SAVED: GitHubWorkspaceSettings = {
  ...SAVED,
  repo_scope_mode: "selected",
  repo_scope_orgs: ["acme"],
  repo_scope_repos: [{ owner: "outside", name: "tool" }],
};

const EXACT_ONLY_SAVED: GitHubWorkspaceSettings = {
  ...SAVED,
  repo_scope_mode: "selected",
  repo_scope_orgs: [],
  repo_scope_repos: [{ owner: "outside", name: "tool" }],
};

beforeEach(() => {
  mocks.fetchGitHubWorkspaceSettings.mockResolvedValue(SAVED);
  mocks.updateGitHubWorkspaceSettings.mockImplementation(async (payload) => ({
    ...SAVED,
    repo_scope_mode: payload.repo_scope_mode,
    repo_scope_orgs: payload.repo_scope_orgs ?? SAVED.repo_scope_orgs,
    repo_scope_repos: payload.repo_scope_repos ?? SAVED.repo_scope_repos,
  }));
  mocks.listUserOrgs.mockResolvedValue({
    orgs: ["octocat", "kdlbs", "acme"].map((login) => ({ login, avatar_url: "" })),
  });
  mocks.useRemoteRepositories.mockReturnValue({
    repos: [repo("kdlbs", "kandev"), repo("acme", "api")],
    availableProviders: ["github"],
    loading: false,
    error: null,
    unavailable: false,
    search: vi.fn(),
  });
});

afterEach(() => {
  cleanup();
  vi.resetAllMocks();
});

function renderSection() {
  render(
    <TooltipProvider>
      <SettingsSaveProvider>
        <GitHubRepoScopeSection workspaceId="ws-1" />
      </SettingsSaveProvider>
    </TooltipProvider>,
  );
}

async function waitForLoadedSection() {
  await screen.findByText("kdlbs/kandev");
  await waitFor(() =>
    expect(screen.getByRole("button", { name: /add repository/i })).toHaveProperty(
      "disabled",
      false,
    ),
  );
}

describe("GitHubRepoScopeSection repositories", () => {
  it("renders saved repositories as removable entries", async () => {
    renderSection();
    expect(await screen.findByText("kdlbs/kandev")).toBeTruthy();
    expect(screen.queryByTestId("github-scope-repos-input")).toBeNull();
  });

  it("adds a repository from the unfiltered picker and saves the selected list", async () => {
    renderSection();
    await waitForLoadedSection();
    fireEvent.click(screen.getByRole("button", { name: /add repository/i }));
    expect(mocks.useRemoteRepositories).toHaveBeenCalledWith("ws-1", {
      providers: ["github"],
      githubRepositorySource: "scope-editor",
    });
    const options = screen.getAllByTestId("remote-repo-option");
    expect(options).toHaveLength(2);
    fireEvent.click(options[1]!);

    fireEvent.click(await screen.findByRole("button", { name: SAVE_CHANGES }));
    await waitFor(() =>
      expect(mocks.updateGitHubWorkspaceSettings).toHaveBeenCalledWith(
        expect.objectContaining({
          workspace_id: "ws-1",
          repo_scope_mode: "selected",
          repo_scope_orgs: [],
          repo_scope_repos: [
            { owner: "kdlbs", name: "kandev" },
            { owner: "acme", name: "api" },
          ],
        }),
      ),
    );
  });

  it("removes a saved repository and saves an empty list", async () => {
    renderSection();
    await waitForLoadedSection();
    fireEvent.click(screen.getByRole("button", { name: /remove kdlbs\/kandev/i }));
    expect(screen.queryByText("kdlbs/kandev")).toBeNull();

    fireEvent.click(await screen.findByRole("button", { name: SAVE_CHANGES }));
    await waitFor(() =>
      expect(mocks.updateGitHubWorkspaceSettings).toHaveBeenCalledWith(
        expect.objectContaining({ repo_scope_repos: [] }),
      ),
    );
  });
});

describe("GitHubRepoScopeSection organizations", () => {
  const ORGS_SAVED: GitHubWorkspaceSettings = {
    ...SAVED,
    repo_scope_mode: "selected",
    repo_scope_orgs: ["kdlbs"],
  };

  it("replaces the comma-separated field with organization chips", async () => {
    mocks.fetchGitHubWorkspaceSettings.mockResolvedValue(ORGS_SAVED);
    renderSection();
    await waitFor(() =>
      expect(screen.getAllByTestId("github-organization-chip").map((c) => c.textContent)).toEqual([
        "kdlbs",
      ]),
    );
    expect(screen.queryByTestId("github-scope-orgs-input")).toBeNull();
  });

  it("adds an organization from the picker and saves the login list", async () => {
    mocks.fetchGitHubWorkspaceSettings.mockResolvedValue(ORGS_SAVED);
    renderSection();
    await waitFor(() =>
      expect(screen.getByTestId<HTMLButtonElement>(ORG_ADD).disabled).toBe(false),
    );
    fireEvent.click(screen.getByTestId(ORG_ADD));
    await waitFor(() =>
      expect(screen.getAllByTestId("github-organization-option")).toHaveLength(3),
    );
    fireEvent.click(screen.getAllByTestId("github-organization-option")[2]!);

    fireEvent.click(await screen.findByRole("button", { name: SAVE_CHANGES }));
    await waitFor(() =>
      expect(mocks.updateGitHubWorkspaceSettings).toHaveBeenCalledWith(
        expect.objectContaining({
          repo_scope_mode: "selected",
          repo_scope_orgs: ["kdlbs", "acme"],
          repo_scope_repos: [{ owner: "kdlbs", name: "kandev" }],
        }),
      ),
    );
  });

  it("keeps the organization picker enabled for selected scope", async () => {
    renderSection();
    await waitForLoadedSection();
    expect(screen.getByTestId<HTMLButtonElement>(ORG_ADD).disabled).toBe(false);
  });
});

describe("GitHubRepoScopeSection union selection", () => {
  it("saves owner and repository scope together", async () => {
    mocks.fetchGitHubWorkspaceSettings.mockResolvedValue(EXACT_ONLY_SAVED);
    renderSection();
    expect(await screen.findByText(EXACT_REPOSITORY)).toBeTruthy();

    fireEvent.click(screen.getByTestId(ORG_ADD));
    fireEvent.click(await screen.findByRole("button", { name: "octocat" }));
    expect(await screen.findByText("octocat")).toBeTruthy();

    fireEvent.click(await screen.findByRole("button", { name: SAVE_CHANGES }));
    await waitFor(() =>
      expect(mocks.updateGitHubWorkspaceSettings).toHaveBeenCalledWith(
        expect.objectContaining({
          workspace_id: "ws-1",
          repo_scope_mode: "selected",
          repo_scope_orgs: ["octocat"],
          repo_scope_repos: [{ owner: "outside", name: "tool" }],
        }),
      ),
    );
  });

  it("toggles all repositories without dropping either saved selection", async () => {
    mocks.fetchGitHubWorkspaceSettings.mockResolvedValue(UNION_SAVED);
    renderSection();
    expect(await screen.findByText(EXACT_REPOSITORY)).toBeTruthy();
    const allRepositories = screen.getByRole("switch", { name: ALL_REPOSITORIES });
    expectAllRepositoriesChecked(false);

    fireEvent.click(allRepositories);
    expectAllRepositoriesChecked(true);
    expect(screen.queryByTestId("github-scope-orgs")).toBeNull();
    expect(screen.queryByTestId("github-scope-repos")).toBeNull();

    fireEvent.click(allRepositories);
    expectAllRepositoriesChecked(false);
    expect(await screen.findByText("acme")).toBeTruthy();
    expect(screen.getByText(EXACT_REPOSITORY)).toBeTruthy();
  });

  it("keeps both pickers available for an empty selected scope", async () => {
    mocks.fetchGitHubWorkspaceSettings.mockResolvedValue({
      ...UNION_SAVED,
      repo_scope_orgs: [],
      repo_scope_repos: [],
    });
    renderSection();
    expect(await screen.findByRole("switch", { name: ALL_REPOSITORIES })).toBeTruthy();
    expect(screen.getByTestId<HTMLButtonElement>(ORG_ADD).disabled).toBe(false);
    expect(screen.getByTestId<HTMLButtonElement>("github-repository-add").disabled).toBe(false);
  });
});

describe("GitHubRepoScopeSection save state", () => {
  it("discards unsaved changes to the combined selection", async () => {
    mocks.fetchGitHubWorkspaceSettings.mockResolvedValue(UNION_SAVED);
    renderSection();
    expect(await screen.findByText(EXACT_REPOSITORY)).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: `Remove ${EXACT_REPOSITORY}` }));
    expect(screen.queryByText(EXACT_REPOSITORY)).toBeNull();

    fireEvent.click(within(screen.getByTestId(SAVE_BAR)).getByRole("button", { name: "Reset" }));
    expect(await screen.findByText(EXACT_REPOSITORY)).toBeTruthy();
    expect(screen.getByText("acme")).toBeTruthy();
  });

  it("preserves a combined draft for retry after a failed save", async () => {
    mocks.fetchGitHubWorkspaceSettings.mockResolvedValue(UNION_SAVED);
    mocks.updateGitHubWorkspaceSettings.mockRejectedValueOnce(new Error("save failed"));
    renderSection();
    expect(await screen.findByText(EXACT_REPOSITORY)).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: `Remove ${EXACT_REPOSITORY}` }));

    fireEvent.click(
      within(screen.getByTestId(SAVE_BAR)).getByRole("button", {
        name: SAVE_CHANGES,
      }),
    );
    await waitFor(() => expect(mocks.updateGitHubWorkspaceSettings).toHaveBeenCalledTimes(1));
    expect(screen.queryByText(EXACT_REPOSITORY)).toBeNull();
    const actions = within(screen.getByTestId(SAVE_BAR)).getAllByRole("button");
    fireEvent.click(actions[1]!);
    await waitFor(() => expect(mocks.updateGitHubWorkspaceSettings).toHaveBeenCalledTimes(2));
    expect(mocks.updateGitHubWorkspaceSettings).toHaveBeenLastCalledWith(
      expect.objectContaining({
        repo_scope_mode: "selected",
        repo_scope_orgs: ["acme"],
        repo_scope_repos: [],
      }),
    );
  });

  it("preserves edits made while an all-scope save is pending", async () => {
    let resolveSave!: (settings: GitHubWorkspaceSettings) => void;
    mocks.fetchGitHubWorkspaceSettings.mockResolvedValue(UNION_SAVED);
    mocks.updateGitHubWorkspaceSettings.mockImplementationOnce(
      () => new Promise((resolve) => (resolveSave = resolve)),
    );
    renderSection();
    expect(await screen.findByText(EXACT_REPOSITORY)).toBeTruthy();
    const allRepositories = screen.getByRole("switch", { name: ALL_REPOSITORIES });
    fireEvent.click(allRepositories);
    fireEvent.click(
      within(screen.getByTestId(SAVE_BAR)).getByRole("button", {
        name: SAVE_CHANGES,
      }),
    );
    await waitFor(() => expect(mocks.updateGitHubWorkspaceSettings).toHaveBeenCalledTimes(1));
    expect(mocks.updateGitHubWorkspaceSettings).toHaveBeenCalledWith(
      expect.objectContaining({
        repo_scope_mode: "all",
        repo_scope_orgs: [],
        repo_scope_repos: [],
      }),
    );

    fireEvent.click(allRepositories);
    await act(async () => {
      resolveSave({
        ...UNION_SAVED,
        repo_scope_mode: "all",
        repo_scope_orgs: [],
        repo_scope_repos: [],
      });
    });
    expectAllRepositoriesChecked(false);
    expect(await screen.findByText("acme")).toBeTruthy();
    expect(screen.getByText(EXACT_REPOSITORY)).toBeTruthy();
    expect(screen.getByTestId(SAVE_BAR).getAttribute("data-status")).toBe("dirty");
  });
});
