import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({
  fetchAccessibleRepos: vi.fn(),
  listUserProjects: vi.fn(),
  useGitHubStatus: vi.fn(),
  useGitHubEnabled: vi.fn(),
  useGitLabStatus: vi.fn(),
  useGitLabEnabled: vi.fn(),
}));

vi.mock("@/lib/api/domains/github-api", () => ({
  fetchAccessibleRepos: mocks.fetchAccessibleRepos,
}));
vi.mock("@/lib/api/domains/gitlab-api", () => ({ listUserProjects: mocks.listUserProjects }));
vi.mock("@/hooks/domains/github/use-github-status", () => ({
  useGitHubStatus: mocks.useGitHubStatus,
}));
vi.mock("@/hooks/domains/github/use-github-enabled", () => ({
  useGitHubEnabled: mocks.useGitHubEnabled,
}));
vi.mock("@/hooks/domains/gitlab/use-gitlab-status", () => ({
  useGitLabStatus: mocks.useGitLabStatus,
}));
vi.mock("@/hooks/domains/gitlab/use-gitlab-enabled", () => ({
  useGitLabEnabled: mocks.useGitLabEnabled,
}));

import { useRemoteRepositories } from "./use-remote-repositories";
import { pluginRegistry } from "@/lib/plugins/registry";
import { invalidateIntegrationAvailability } from "@/lib/integrations/integration-availability-events";

const WORKSPACE_ID = "workspace-1";
const PLUGIN_ID = "test-bitbucket-readiness";

function disableBuiltInProviders() {
  mocks.useGitHubStatus.mockReturnValue({
    status: null,
    loaded: true,
    loading: false,
    refresh: vi.fn(),
  });
  mocks.useGitHubEnabled.mockReturnValue({ enabled: false, loaded: true });
  mocks.useGitLabStatus.mockReturnValue({
    status: null,
    loaded: true,
    loading: false,
    refresh: vi.fn(),
  });
  mocks.useGitLabEnabled.mockReturnValue({ enabled: false, loaded: true });
}

beforeEach(() => {
  disableBuiltInProviders();
  mocks.fetchAccessibleRepos.mockResolvedValue([]);
  mocks.listUserProjects.mockResolvedValue({ projects: [] });
});

afterEach(() => {
  cleanup();
  pluginRegistry.unregisterPlugin(PLUGIN_ID);
  vi.resetAllMocks();
});

it("requires verified authentication instead of token configuration", async () => {
  mocks.useGitHubEnabled.mockReturnValue({ enabled: true, loaded: true });
  mocks.useGitHubStatus.mockReturnValue({
    status: { authenticated: false, token_configured: true },
    loaded: true,
    loading: false,
    refresh: vi.fn(),
  });

  const { result } = renderHook(() =>
    useRemoteRepositories(WORKSPACE_ID, { requireProviderReadiness: true }),
  );

  await waitFor(() => expect(result.current.loading).toBe(false));
  expect(mocks.fetchAccessibleRepos).not.toHaveBeenCalled();
  expect(result.current.availableProviders).not.toContain("github");
});

it("does not list a plugin provider without positive availability", async () => {
  const getAvailability = vi.fn().mockResolvedValue({
    configured: true,
    enabled: false,
    tested: true,
  });
  const listRepositories = vi.fn().mockResolvedValue([]);
  pluginRegistry.forPlugin(PLUGIN_ID).registerRepositoryProvider({
    id: "bitbucket",
    label: "Bitbucket",
    getAvailability,
    listRepositories,
    matchesURL: () => false,
    listBranches: async () => [],
    inspectURL: async () => null,
  });

  const { result } = renderHook(() =>
    useRemoteRepositories(WORKSPACE_ID, { requireProviderReadiness: true }),
  );

  await waitFor(() => expect(result.current.loading).toBe(false));
  expect(getAvailability).toHaveBeenCalledWith(
    expect.objectContaining({ workspaceId: WORKSPACE_ID }),
  );
  expect(listRepositories).not.toHaveBeenCalled();
  expect(result.current.availableProviders).not.toContain("bitbucket");
});

it("keeps a ready provider available when its repository list fails", async () => {
  const error = new Error("Bitbucket repositories unavailable");
  pluginRegistry.forPlugin(PLUGIN_ID).registerRepositoryProvider({
    id: "bitbucket",
    label: "Bitbucket",
    getAvailability: async () => ({ configured: true, enabled: true, tested: true }),
    matchesURL: () => false,
    listBranches: async () => [],
    inspectURL: async () => null,
    listRepositories: vi.fn().mockRejectedValue(error),
  });

  const { result } = renderHook(() =>
    useRemoteRepositories(WORKSPACE_ID, { requireProviderReadiness: true }),
  );

  await waitFor(() => expect(result.current.loading).toBe(false));
  expect(result.current.availableProviders).toContain("bitbucket");
  expect(result.current.sourceErrors).toContainEqual({ provider: "bitbucket", error });
});

it("rechecks a registered provider after its connection becomes unavailable", async () => {
  let available = true;
  const getAvailability = vi.fn(async () => ({
    configured: true,
    enabled: true,
    tested: available,
  }));
  const listRepositories = vi.fn().mockResolvedValue([]);
  pluginRegistry.forPlugin(PLUGIN_ID).registerRepositoryProvider({
    id: "bitbucket",
    label: "Bitbucket",
    getAvailability,
    listRepositories,
    matchesURL: () => false,
    listBranches: async () => [],
    inspectURL: async () => null,
  });

  const { result } = renderHook(() =>
    useRemoteRepositories(WORKSPACE_ID, { requireProviderReadiness: true }),
  );

  await waitFor(() => expect(result.current.availableProviders).toContain("bitbucket"));
  await waitFor(() => expect(listRepositories).toHaveBeenCalledTimes(1));

  act(() => invalidateIntegrationAvailability());
  await waitFor(() => {
    expect(result.current.providerCatalog).toEqual(
      expect.arrayContaining([
        expect.objectContaining({ provider: "bitbucket", readiness: "ready" }),
      ]),
    );
  });
  expect(listRepositories).toHaveBeenCalledTimes(1);

  available = false;
  act(() => invalidateIntegrationAvailability());

  await waitFor(() => {
    expect(result.current.providerCatalog).toEqual(
      expect.arrayContaining([
        expect.objectContaining({ provider: "bitbucket", readiness: "unavailable" }),
      ]),
    );
  });
  expect(result.current.availableProviders).not.toContain("bitbucket");
  expect(getAvailability).toHaveBeenCalledTimes(3);
  expect(listRepositories).toHaveBeenCalledTimes(1);
});
