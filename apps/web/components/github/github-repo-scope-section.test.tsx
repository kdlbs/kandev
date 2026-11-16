import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
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
  repo_scope_mode: "repos",
  repo_scope_orgs: [],
  repo_scope_repos: [{ owner: "kdlbs", name: "kandev" }],
  created_at: "2026-01-01T00:00:00Z",
  updated_at: "2026-01-01T00:00:00Z",
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

    fireEvent.click(await screen.findByRole("button", { name: "Save changes" }));
    await waitFor(() =>
      expect(mocks.updateGitHubWorkspaceSettings).toHaveBeenCalledWith(
        expect.objectContaining({
          workspace_id: "ws-1",
          repo_scope_mode: "repos",
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

    fireEvent.click(await screen.findByRole("button", { name: "Save changes" }));
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
    repo_scope_mode: "orgs",
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
      expect(screen.getByTestId<HTMLButtonElement>("github-organization-add").disabled).toBe(false),
    );
    fireEvent.click(screen.getByTestId("github-organization-add"));
    await waitFor(() =>
      expect(screen.getAllByTestId("github-organization-option")).toHaveLength(3),
    );
    fireEvent.click(screen.getAllByTestId("github-organization-option")[2]!);

    fireEvent.click(await screen.findByRole("button", { name: "Save changes" }));
    await waitFor(() =>
      expect(mocks.updateGitHubWorkspaceSettings).toHaveBeenCalledWith(
        expect.objectContaining({ repo_scope_mode: "orgs", repo_scope_orgs: ["kdlbs", "acme"] }),
      ),
    );
  });

  it("disables the organization picker outside orgs mode", async () => {
    renderSection();
    await waitForLoadedSection();
    expect(screen.getByTestId<HTMLButtonElement>("github-organization-add").disabled).toBe(true);
  });
});
