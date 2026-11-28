import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { TooltipProvider } from "@kandev/ui/tooltip";
import type { UseRemoteRepositoriesResult } from "@/hooks/domains/integrations/use-remote-repositories";
import type { Repository } from "@/lib/types/http";
import { RepositoryPicker } from "./task-create-dialog-repository-picker";

const inspectRepositoryCloneSource = vi.hoisted(() => vi.fn());
const LOCAL_OPTION_TEST_ID = "task-repository-local-option";

vi.mock("@/lib/api/domains/workspace-api", () => ({ inspectRepositoryCloneSource }));

afterEach(() => {
  cleanup();
  sessionStorage.clear();
  inspectRepositoryCloneSource.mockReset();
});

const workspaceRepository = {
  id: "repo-local",
  name: "local-app",
  local_path: "/work/local-app",
  default_branch: "main",
} as Repository;

function accessible(): UseRemoteRepositoriesResult {
  return {
    repos: [
      {
        provider: "github",
        id: "acme/remote-app",
        owner: "acme",
        name: "remote-app",
        fullName: "acme/remote-app",
        url: "https://github.com/acme/remote-app",
        defaultBranch: "main",
        private: false,
      },
    ],
    availableProviders: ["github"],
    providerCatalog: [{ provider: "github", readiness: "ready" }],
    loading: false,
    error: null,
    sourceErrors: [],
    unavailable: false,
    search: vi.fn(),
    matchesURL: () => false,
  };
}

function renderPicker(scopeKey?: string, provider: "github" | "gitlab" = "github") {
  const value = accessible();
  if (provider === "gitlab") {
    value.repos = [
      {
        ...value.repos[0],
        provider: "gitlab",
        url: "https://gitlab.com/acme/remote-app",
      },
    ];
    value.availableProviders = ["gitlab"];
    value.providerCatalog = [{ provider: "gitlab", readiness: "ready" }];
  }
  return render(
    <TooltipProvider>
      <RepositoryPicker
        repositories={[workspaceRepository]}
        discoveredRepositories={[]}
        accessible={value}
        onSelectLocal={vi.fn()}
        onSelectRemote={vi.fn()}
        onPasteRemote={vi.fn()}
        onRefresh={vi.fn()}
        scopeKey={scopeKey}
      />
    </TooltipProvider>,
  );
}

describe("RepositoryPicker", () => {
  it("renders named source tabs before the search field", () => {
    renderPicker();

    const tabs = screen.getByTestId("task-repository-source-tabs");
    const input = screen.getByTestId("task-repository-picker-input");
    expect(tabs.textContent).toContain("Local");
    expect(tabs.textContent).toContain("GitHub");
    expect(tabs.compareDocumentPosition(input) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
  });

  it("adds a selected repository from the active provider source", () => {
    const onSelectRemote = vi.fn();
    render(
      <TooltipProvider>
        <RepositoryPicker
          repositories={[]}
          discoveredRepositories={[]}
          accessible={accessible()}
          onSelectLocal={vi.fn()}
          onSelectRemote={onSelectRemote}
          onPasteRemote={vi.fn()}
          onRefresh={vi.fn()}
        />
      </TooltipProvider>,
    );

    fireEvent.click(screen.getByRole("tab", { name: "GitHub" }));
    fireEvent.click(screen.getByTestId("task-repository-remote-option"));
    expect(onSelectRemote).toHaveBeenCalledWith(
      expect.objectContaining({ fullName: "acme/remote-app" }),
    );
  });

  it("includes the local repository default branch when selected", () => {
    const onSelectLocal = vi.fn();
    render(
      <TooltipProvider>
        <RepositoryPicker
          repositories={[workspaceRepository]}
          discoveredRepositories={[]}
          accessible={accessible()}
          onSelectLocal={onSelectLocal}
          onSelectRemote={vi.fn()}
          onPasteRemote={vi.fn()}
          onRefresh={vi.fn()}
        />
      </TooltipProvider>,
    );

    fireEvent.click(screen.getByTestId(LOCAL_OPTION_TEST_ID));

    expect(onSelectLocal).toHaveBeenCalledWith({
      repositoryId: "repo-local",
      defaultBranch: "main",
    });
  });

  it("offers local repository creation when the caller provides it", () => {
    const onCreateRepository = vi.fn();
    render(
      <TooltipProvider>
        <RepositoryPicker
          repositories={[workspaceRepository]}
          discoveredRepositories={[]}
          accessible={accessible()}
          onSelectLocal={vi.fn()}
          onSelectRemote={vi.fn()}
          onPasteRemote={vi.fn()}
          onRefresh={vi.fn()}
          onCreateRepository={onCreateRepository}
        />
      </TooltipProvider>,
    );

    fireEvent.click(screen.getByTestId("create-local-repository-button"));

    expect(onCreateRepository).toHaveBeenCalledOnce();
  });

  it("restores the last eligible provider for the picker scope", () => {
    sessionStorage.setItem("kandev.task-repository-picker-source:workspace-1", "gitlab");
    renderPicker("workspace-1", "gitlab");

    expect(screen.getByRole("tab", { name: "GitLab" }).getAttribute("aria-selected")).toBe("true");
  });
});

describe("RepositoryPicker provider availability", () => {
  it("falls back to Local and offers retry when the selected provider becomes unavailable", async () => {
    const value = accessible();
    const onRefresh = vi.fn();
    const view = render(
      <TooltipProvider>
        <RepositoryPicker
          repositories={[workspaceRepository]}
          discoveredRepositories={[]}
          accessible={value}
          onSelectLocal={vi.fn()}
          onSelectRemote={vi.fn()}
          onPasteRemote={vi.fn()}
          onRefresh={onRefresh}
        />
      </TooltipProvider>,
    );

    fireEvent.click(screen.getByRole("tab", { name: "GitHub" }));
    value.availableProviders = [];
    value.providerCatalog = [{ provider: "github", readiness: "unavailable" }];
    view.rerender(
      <TooltipProvider>
        <RepositoryPicker
          repositories={[workspaceRepository]}
          discoveredRepositories={[]}
          accessible={value}
          onSelectLocal={vi.fn()}
          onSelectRemote={vi.fn()}
          onPasteRemote={vi.fn()}
          onRefresh={onRefresh}
        />
      </TooltipProvider>,
    );

    await waitFor(() => {
      expect(screen.getByRole("tab", { name: "Local" }).getAttribute("aria-selected")).toBe("true");
      expect(screen.getByTestId("task-repository-source-unavailable")).toBeTruthy();
    });
    fireEvent.click(screen.getByRole("button", { name: "Retry" }));
    expect(onRefresh).toHaveBeenCalledOnce();
  });
});

describe("RepositoryPicker local source filtering", () => {
  it("excludes provider-only saved repositories but keeps a local checkout with provider metadata", () => {
    const onSelectLocal = vi.fn();
    const localProviderRepository = {
      ...workspaceRepository,
      id: "repo-local-provider",
      name: "local-provider-app",
      source_type: "provider",
      provider: "bitbucket",
    } as Repository;
    const remoteOnlyRepository = {
      ...workspaceRepository,
      id: "repo-remote-only",
      name: "remote-only-app",
      local_path: "",
      source_type: "provider",
      provider: "bitbucket",
    } as Repository;

    render(
      <TooltipProvider>
        <RepositoryPicker
          repositories={[localProviderRepository, remoteOnlyRepository]}
          discoveredRepositories={[]}
          accessible={accessible()}
          onSelectLocal={onSelectLocal}
          onSelectRemote={vi.fn()}
          onPasteRemote={vi.fn()}
          onRefresh={vi.fn()}
        />
      </TooltipProvider>,
    );

    expect(screen.getAllByTestId(LOCAL_OPTION_TEST_ID)).toHaveLength(1);
    expect(screen.getByText("local-provider-app")).toBeTruthy();
    expect(screen.queryByText("remote-only-app")).toBeNull();
    fireEvent.click(screen.getByTestId(LOCAL_OPTION_TEST_ID));
    expect(onSelectLocal).toHaveBeenCalledWith(
      expect.objectContaining({ repositoryId: "repo-local-provider" }),
    );
  });
});

describe("RepositoryPicker remote origin selection", () => {
  it("only enables local choices after a remote origin is verified", async () => {
    inspectRepositoryCloneSource.mockResolvedValue({
      ready: true,
      origin: "https://github.com/acme/local-app.git",
      current_branch: "feature/local",
      default_branch: "main",
      branches: [
        { name: "main", type: "remote", remote: "origin" },
        { name: "release", type: "remote", remote: "origin" },
      ],
    });
    const onSelectLocal = vi.fn();
    render(
      <TooltipProvider>
        <RepositoryPicker
          repositories={[workspaceRepository]}
          discoveredRepositories={[]}
          accessible={accessible()}
          workspaceId="workspace-1"
          remoteOriginMode
          onSelectLocal={onSelectLocal}
          onSelectRemote={vi.fn()}
          onPasteRemote={vi.fn()}
          onRefresh={vi.fn()}
        />
      </TooltipProvider>,
    );

    await waitFor(() => expect(screen.getByText("Clone from remote")).toBeTruthy());
    fireEvent.click(screen.getByTestId(LOCAL_OPTION_TEST_ID));
    expect(onSelectLocal).toHaveBeenCalledWith({
      repositoryId: "repo-local",
      defaultBranch: "main",
      checkoutSource: "remote_origin",
      expectedOrigin: "https://github.com/acme/local-app.git",
      remoteBranches: [
        { name: "main", type: "remote", remote: "origin" },
        { name: "release", type: "remote", remote: "origin" },
      ],
    });
  });

  it("keeps a local choice disabled when its origin is unavailable", async () => {
    inspectRepositoryCloneSource.mockResolvedValue({
      ready: false,
      reason: "missing_origin",
      branches: [],
    });
    render(
      <TooltipProvider>
        <RepositoryPicker
          repositories={[workspaceRepository]}
          discoveredRepositories={[]}
          accessible={accessible()}
          workspaceId="workspace-1"
          remoteOriginMode
          onSelectLocal={vi.fn()}
          onSelectRemote={vi.fn()}
          onPasteRemote={vi.fn()}
          onRefresh={vi.fn()}
        />
      </TooltipProvider>,
    );

    await waitFor(() => expect(screen.getByText("No usable remote origin.")).toBeTruthy());
    expect(screen.getByTestId(LOCAL_OPTION_TEST_ID)).toHaveProperty("disabled", true);
  });
});
