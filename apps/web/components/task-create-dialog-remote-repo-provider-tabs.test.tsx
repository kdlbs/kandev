import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { TooltipProvider } from "@kandev/ui/tooltip";
import type {
  RemoteRepository,
  RemoteRepositoryProvider,
  UseRemoteRepositoriesResult,
} from "@/hooks/domains/integrations/use-remote-repositories";
import { RemoteRepoChip } from "./task-create-dialog-remote-repo-chip";

const AZURE_PROVIDER = "azure_devops" as const;
const AZURE_REPO_NAME = "Platform/api";
const GITLAB_REPO_NAME = "acme/worker";

const GITHUB_REPO: RemoteRepository = {
  provider: "github",
  id: "acme/site",
  owner: "acme",
  name: "site",
  fullName: "acme/site",
  url: "https://github.com/acme/site",
  defaultBranch: "main",
  private: false,
};

const AZURE_REPO: RemoteRepository = {
  provider: AZURE_PROVIDER,
  id: "azure-api",
  owner: "Platform",
  name: "api",
  fullName: AZURE_REPO_NAME,
  url: "https://dev.azure.com/acme/Platform/_git/api",
  defaultBranch: "main",
  private: true,
};

const GITLAB_REPO: RemoteRepository = {
  provider: "gitlab",
  id: "gitlab-worker",
  owner: "acme",
  name: "worker",
  fullName: GITLAB_REPO_NAME,
  url: "https://gitlab.com/acme/worker",
  defaultBranch: "main",
  private: false,
};

afterEach(cleanup);

function accessibleRepos(
  repos: RemoteRepository[],
  availableProviders: RemoteRepositoryProvider[],
): UseRemoteRepositoriesResult {
  return {
    repos,
    availableProviders,
    loading: false,
    unavailable: false,
    error: null,
    search: () => undefined,
  };
}

function renderPicker(accessible: UseRemoteRepositoriesResult) {
  render(
    <TooltipProvider>
      <RemoteRepoChip
        row={{ key: "remote-0", url: "", branch: "", source: "paste" }}
        branches={[]}
        branchesLoading={false}
        accessibleRepos={accessible}
        onURLChange={vi.fn()}
        onBranchChange={() => undefined}
        onRemove={() => undefined}
      />
    </TooltipProvider>,
  );
  fireEvent.click(screen.getByTestId("remote-repo-chip-trigger"));
}

function activateProvider(name: string) {
  fireEvent.mouseDown(screen.getByRole("tab", { name }), { button: 0, ctrlKey: false });
}

describe("RemoteRepoChip provider tabs", () => {
  it("shows bottom tabs and filters repositories when multiple providers are available", () => {
    renderPicker(
      accessibleRepos([GITHUB_REPO, GITLAB_REPO, AZURE_REPO], ["github", "gitlab", AZURE_PROVIDER]),
    );

    expect(screen.getAllByRole("tab")).toHaveLength(3);
    expect(screen.getByRole("tab", { name: "GitHub" }).getAttribute("aria-selected")).toBe("true");
    expect(screen.getByText("acme/site")).toBeTruthy();
    expect(screen.queryByText(GITLAB_REPO_NAME)).toBeNull();
    expect(screen.queryByText(AZURE_REPO_NAME)).toBeNull();

    activateProvider("GitLab");
    expect(screen.getByText(GITLAB_REPO_NAME)).toBeTruthy();

    activateProvider("Azure DevOps");
    expect(screen.queryByText("acme/site")).toBeNull();
    expect(screen.getByText(AZURE_REPO_NAME)).toBeTruthy();
  });

  it("keeps a tab for an available provider that currently has no repositories", () => {
    renderPicker(accessibleRepos([GITHUB_REPO], ["github", AZURE_PROVIDER]));

    activateProvider("Azure DevOps");
    expect(screen.getByText("No repositories found.")).toBeTruthy();
  });

  it("does not show provider tabs when only one provider is available", () => {
    renderPicker(accessibleRepos([AZURE_REPO], [AZURE_PROVIDER]));

    expect(screen.queryByRole("tab")).toBeNull();
    expect(screen.getByText(AZURE_REPO_NAME)).toBeTruthy();
  });
});
