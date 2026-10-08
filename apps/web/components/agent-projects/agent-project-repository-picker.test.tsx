import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { Repository } from "@/lib/types/http";

const mocks = vi.hoisted(() => ({
  inspect: vi.fn(),
  useRemoteRepositories: vi.fn(),
}));

vi.mock("@/app/actions/workspaces", () => ({
  inspectRemoteRepositorySelectionAction: mocks.inspect,
}));

vi.mock("@/hooks/domains/integrations/use-remote-repositories", () => ({
  useRemoteRepositories: mocks.useRemoteRepositories,
  builtInRepositoryProviderForURL: (value: string) =>
    value.includes("github.com") ? "github" : undefined,
  resolveBuiltInRepositoryURL: vi.fn(),
}));
vi.mock("@/lib/plugins/repository-provider-url-resolution", () => ({
  repositoryProviderMatchesURL: (
    provider: { matchesURL?: (url: string) => boolean },
    url: string,
  ) => provider.matchesURL?.(url) ?? true,
}));
vi.mock("@/lib/plugins/registry", () => ({
  usePluginRegistry: () => ({ getRepositoryProvider: () => undefined }),
  pluginRegistry: {
    getRepositoryProviders: () => [
      { id: "forge", matchesURL: (url: string) => url.includes("forge.example.test") },
    ],
  },
}));
vi.mock("@/components/task-create-dialog-remote-repo-chip", () => ({
  RemoteRepoPickerContent: ({ onPaste }: { onPaste: (value: string) => void }) => (
    <>
      <button
        type="button"
        data-testid="agent-project-test-paste"
        onClick={() => onPaste("https://forge.example.test/scm/team/app.git")}
      >
        Paste repository URL
      </button>
      <button
        type="button"
        data-testid="agent-project-test-paste-github"
        onClick={() => onPaste("https://github.com/acme/outside")}
      >
        Paste GitHub URL
      </button>
    </>
  ),
}));

import { AgentProjectRepositoryPicker } from "./agent-project-repository-picker";

const ADD_REPOSITORY_TEST_ID = "agent-project-add-repository";

function existingRepository(): Repository {
  return {
    id: "repository-existing" as Repository["id"],
    workspace_id: "workspace-1" as Repository["workspace_id"],
    name: "team/existing",
    source_type: "provider",
    local_path: "",
    provider: "github",
    provider_repo_id: "team/existing",
    provider_host: "https://github.com",
    provider_owner: "team",
    provider_name: "existing",
    remote_url: "https://github.com/team/existing.git",
    default_branch: "main",
    worktree_branch_prefix: "",
    worktree_branch_template: "",
    pull_before_worktree: true,
    setup_script: "",
    cleanup_script: "",
    dev_script: "",
    copy_files: "",
    created_at: "",
    updated_at: "",
  };
}

const emptyAccessible = {
  repos: [],
  allRepos: [],
  availableProviders: [],
  loading: false,
  error: null,
  unavailable: true,
  search: () => undefined,
};

afterEach(cleanup);
beforeEach(() => {
  mocks.inspect.mockReset();
  mocks.useRemoteRepositories.mockReset();
  mocks.useRemoteRepositories.mockReturnValue(emptyAccessible);
});

describe("AgentProjectRepositoryPicker", () => {
  it("lets a workspace select an existing repository without a provider connection", async () => {
    const onSelectExisting = vi.fn();
    render(
      <AgentProjectRepositoryPicker
        workspaceId="workspace-1"
        selected={[]}
        selectedIdentities={[]}
        existingRepositories={[existingRepository()]}
        selectedRepositoryIds={[]}
        disabled={false}
        onSelect={vi.fn()}
        onSelectExisting={onSelectExisting}
        onRemove={vi.fn()}
      />,
    );

    fireEvent.click(screen.getByTestId(ADD_REPOSITORY_TEST_ID));
    fireEvent.click(await screen.findByTestId("agent-project-existing-repository-option"));

    expect(onSelectExisting).toHaveBeenCalledWith("repository-existing");
    expect(mocks.inspect).not.toHaveBeenCalled();
  });
});

describe("AgentProjectRepositoryPicker URL inspection", () => {
  it("cancels a pending URL inspection when a configured repository is selected", async () => {
    let finishInspection!: (value: unknown) => void;
    mocks.inspect.mockImplementation(() => new Promise((resolve) => (finishInspection = resolve)));
    const onSelectExisting = vi.fn();
    const onSelect = vi.fn();
    render(
      <AgentProjectRepositoryPicker
        workspaceId="workspace-1"
        selected={[]}
        selectedIdentities={[]}
        existingRepositories={[existingRepository()]}
        selectedRepositoryIds={[]}
        disabled={false}
        onSelect={onSelect}
        onSelectExisting={onSelectExisting}
        onRemove={vi.fn()}
      />,
    );

    fireEvent.click(screen.getByTestId(ADD_REPOSITORY_TEST_ID));
    fireEvent.click(await screen.findByTestId("agent-project-test-paste"));
    await waitFor(() => expect(mocks.inspect).toHaveBeenCalledOnce());
    const request = mocks.inspect.mock.calls[0]![2] as AbortSignal;
    fireEvent.click(await screen.findByTestId("agent-project-existing-repository-option"));

    expect(request.aborted).toBe(true);
    expect(onSelectExisting).toHaveBeenCalledWith("repository-existing");
    expect((screen.getByTestId(ADD_REPOSITORY_TEST_ID) as HTMLButtonElement).disabled).toBe(false);
    await act(async () => {
      finishInspection({
        remote_url: "https://forge.example.test/scm/team/app.git",
        provider: "forge",
        provider_repo_id: "app-1",
        provider_owner: "team",
        provider_name: "app",
        provider_host: "https://forge.example.test",
        provider_scope: "group-1",
        default_branch: "main",
      });
    });
    expect(onSelect).not.toHaveBeenCalled();
  });
});

describe("AgentProjectRepositoryPicker stale URL inspection", () => {
  it("discards a delayed inspection after the workspace changes", async () => {
    let finishInspection!: (value: unknown) => void;
    mocks.inspect.mockImplementation(() => new Promise((resolve) => (finishInspection = resolve)));
    const onSelect = vi.fn();
    const { rerender } = render(
      <AgentProjectRepositoryPicker
        workspaceId="workspace-1"
        selected={[]}
        selectedIdentities={[]}
        existingRepositories={[]}
        selectedRepositoryIds={[]}
        disabled={false}
        onSelect={onSelect}
        onSelectExisting={vi.fn()}
        onRemove={vi.fn()}
      />,
    );

    fireEvent.click(screen.getByTestId(ADD_REPOSITORY_TEST_ID));
    fireEvent.click(await screen.findByTestId("agent-project-test-paste"));
    await waitFor(() => expect(mocks.inspect).toHaveBeenCalledOnce());
    const request = mocks.inspect.mock.calls[0]![2] as AbortSignal;
    rerender(
      <AgentProjectRepositoryPicker
        workspaceId="workspace-2"
        selected={[]}
        selectedIdentities={[]}
        existingRepositories={[]}
        selectedRepositoryIds={[]}
        disabled={false}
        onSelect={onSelect}
        onSelectExisting={vi.fn()}
        onRemove={vi.fn()}
      />,
    );
    expect(request.aborted).toBe(true);

    await act(async () => {
      finishInspection({
        remote_url: "https://forge.example.test/scm/team/app.git",
        provider: "forge",
        provider_repo_id: "app-1",
        provider_owner: "team",
        provider_name: "app",
        provider_host: "https://forge.example.test",
        provider_scope: "group-1",
        default_branch: "main",
      });
    });

    expect(onSelect).not.toHaveBeenCalled();
  });
});

describe("AgentProjectRepositoryPicker built-in URL inspection", () => {
  it("verifies a built-in pasted URL outside the initially loaded catalog", async () => {
    const onSelect = vi.fn();
    mocks.inspect.mockResolvedValue({
      remote_url: "https://github.com/acme/outside.git",
      provider: "github",
      provider_repo_id: "acme/outside",
      provider_owner: "acme",
      provider_name: "outside",
      provider_host: "https://github.com",
      default_branch: "trunk",
    });
    render(
      <AgentProjectRepositoryPicker
        workspaceId="workspace-1"
        selected={[]}
        selectedIdentities={[]}
        existingRepositories={[]}
        selectedRepositoryIds={[]}
        disabled={false}
        onSelect={onSelect}
        onSelectExisting={vi.fn()}
        onRemove={vi.fn()}
      />,
    );

    fireEvent.click(screen.getByTestId(ADD_REPOSITORY_TEST_ID));
    fireEvent.click(await screen.findByTestId("agent-project-test-paste-github"));

    await waitFor(() => expect(mocks.inspect).toHaveBeenCalledOnce());
    expect(mocks.inspect.mock.calls[0]?.[1]).toEqual({
      remote_url: "https://github.com/acme/outside",
      provider: "github",
    });
    await waitFor(() =>
      expect(onSelect).toHaveBeenCalledWith(
        expect.objectContaining({
          id: "acme/outside",
          defaultBranch: "trunk",
          url: "https://github.com/acme/outside.git",
        }),
      ),
    );
  });
});
