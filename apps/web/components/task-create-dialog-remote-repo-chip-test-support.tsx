import { render } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { vi } from "vitest";
import type { UseRemoteRepositoriesResult } from "@/hooks/domains/integrations/use-remote-repositories";
import { RemoteRepoChip, type RemoteRepoChipProps } from "./task-create-dialog-remote-repo-chip";
import type { TaskRemoteRepoRow } from "./task-create-dialog-types";

type AccessibleRepo = {
  provider: "github" | "gitlab" | "azure_devops";
  owner: string;
  name: string;
  full_name: string;
  default_branch: string;
  description?: string;
  private: boolean;
};

type AccessibleOverrides = Omit<Partial<UseRemoteRepositoriesResult>, "repos"> & {
  repos?: AccessibleRepo[];
};

export function makeAccessible(overrides: AccessibleOverrides = {}): UseRemoteRepositoriesResult {
  const repos = (overrides.repos ?? []).map((repo) => ({
    provider: repo.provider,
    id: repo.full_name,
    owner: repo.owner,
    name: repo.name,
    fullName: repo.full_name,
    url: remoteTestURL(repo),
    defaultBranch: repo.default_branch,
    private: repo.private,
  }));
  const availableProviders = overrides.availableProviders ?? [
    ...new Set(repos.map((repo) => repo.provider)),
  ];
  return {
    loading: false,
    unavailable: false,
    error: null,
    search: () => undefined,
    ...overrides,
    repos,
    availableProviders,
  };
}

export function githubSite(overrides: Partial<AccessibleRepo> = {}): AccessibleRepo {
  return {
    provider: "github",
    owner: "acme",
    name: "site",
    full_name: "acme/site",
    default_branch: "main",
    private: false,
    ...overrides,
  };
}

export function row(overrides: Partial<TaskRemoteRepoRow> = {}): TaskRemoteRepoRow {
  return { key: "remote-0", url: "", branch: "", source: "paste", ...overrides };
}

export function renderInProvider(ui: Parameters<typeof render>[0]) {
  return render(<TooltipProvider>{ui}</TooltipProvider>);
}

export function renderRemoteRepoChip(overrides: Partial<RemoteRepoChipProps> = {}) {
  return renderInProvider(
    <RemoteRepoChip
      row={row()}
      branches={[]}
      branchesLoading={false}
      accessibleRepos={makeAccessible()}
      onURLChange={vi.fn()}
      onBranchChange={noopBranch}
      onRemove={noopRemove}
      {...overrides}
    />,
  );
}

export const noopBranch = () => undefined;
export const noopRemove = () => undefined;

function remoteTestURL(repo: AccessibleRepo): string {
  if (repo.provider === "azure_devops") {
    return `https://dev.azure.com/acme/${repo.owner}/_git/${repo.name}`;
  }
  return `https://${repo.provider}.com/${repo.owner}/${repo.name}`;
}
