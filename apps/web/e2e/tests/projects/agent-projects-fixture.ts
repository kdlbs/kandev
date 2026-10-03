import type { ApiClient } from "../../helpers/api-client";
import { startHTTPGitFixture } from "../../helpers/http-git-server";
import type { BackendContext } from "../../fixtures/backend";
import type { SeedData } from "../../fixtures/test-base";

export type AgentProjectFixture = {
  repositoryId: string;
  profileId: string;
  cleanup: (projectId?: string) => Promise<void>;
};

export async function setupAgentProjectFixture(
  apiClient: ApiClient,
  backend: BackendContext,
  seedData: SeedData,
  name: string,
): Promise<AgentProjectFixture> {
  const git = await startHTTPGitFixture(backend.tmpDir, name, {
    bridgeGateway: "127.0.0.1",
    providerOrigin: "https://github.com",
  });
  let releaseBackendEnv: (() => Promise<void>) | undefined;
  let repositoryId: string | undefined;
  let profileId: string | undefined;
  let originalExecutorId = "";
  try {
    releaseBackendEnv = await backend.useEnv({
      KANDEV_FEATURES_AGENT_PROJECTS: "true",
      ...git.backendEnv,
    });
    const workspaces = await apiClient.listWorkspaces();
    originalExecutorId =
      workspaces.workspaces.find((workspace) => workspace.id === seedData.workspaceId)
        ?.default_executor_id ?? "";
    const { executors } = await apiClient.listExecutors();
    const worktreeExecutor = executors.find((executor) =>
      executor.profiles?.some((profile) => profile.id === seedData.worktreeExecutorProfileId),
    );
    if (!worktreeExecutor) throw new Error("The worktree executor is unavailable");
    await apiClient.updateWorkspace(seedData.workspaceId, {
      default_executor_id: worktreeExecutor.id,
    });
    const { agents } = await apiClient.listAgents();
    const mockAgent = agents.find((agent) => agent.name === "mock-agent");
    if (!mockAgent) throw new Error("The ACP mock agent is unavailable");
    const projectProfile = await apiClient.createAgentProfile(
      mockAgent.id,
      `E2E Agent Projects ${name}`,
      { model: "mock-fast", cli_passthrough: false },
    );
    profileId = projectProfile.id;
    await apiClient.mockGitHubReset();
    await apiClient.mockGitHubAddBranches("fixture", name, [{ name: "main" }]);

    const repositoryResponse = await apiClient.rawRequest(
      "POST",
      `/api/v1/workspaces/${seedData.workspaceId}/repositories`,
      {
        name: `fixture/${name}`,
        source_type: "provider",
        local_path: git.checkoutPath,
        provider: "github",
        provider_host: "https://github.com",
        provider_owner: "fixture",
        provider_name: name,
        default_branch: "main",
        pull_before_worktree: false,
      },
    );
    if (!repositoryResponse.ok) {
      throw new Error(`Remote repository setup failed (${repositoryResponse.status})`);
    }
    repositoryId = ((await repositoryResponse.json()) as { id: string }).id;

    return {
      repositoryId,
      profileId,
      cleanup: async (projectId) => {
        try {
          if (projectId) {
            await apiClient
              .rawRequest(
                "DELETE",
                `/api/v1/workspaces/${seedData.workspaceId}/agent-projects/${projectId}?discard_worktree_changes=true&delete_context=true`,
              )
              .catch(() => undefined);
          }
          if (repositoryId) {
            await apiClient
              .rawRequest("DELETE", `/api/v1/repositories/${repositoryId}`)
              .catch(() => undefined);
          }
          if (profileId) {
            await apiClient.deleteAgentProfile(profileId, true).catch(() => undefined);
          }
          await apiClient.updateWorkspace(seedData.workspaceId, {
            default_executor_id: originalExecutorId,
          });
        } finally {
          try {
            await releaseBackendEnv?.();
          } finally {
            await git.close();
          }
        }
      },
    };
  } catch (error) {
    try {
      if (repositoryId) {
        await apiClient
          .rawRequest("DELETE", `/api/v1/repositories/${repositoryId}`)
          .catch(() => undefined);
      }
      if (profileId) {
        await apiClient.deleteAgentProfile(profileId, true).catch(() => undefined);
      }
      await releaseBackendEnv?.();
    } finally {
      await git.close();
    }
    throw error;
  }
}

export type AgentProjectView = {
  id: string;
  name: string;
  main_task_id: string;
  tasks: Array<{ id: string; title: string; state: string; parent_id?: string }>;
};

export async function readAgentProject(
  apiClient: ApiClient,
  workspaceId: string,
  projectId: string,
): Promise<AgentProjectView> {
  const response = await apiClient.rawRequest(
    "GET",
    `/api/v1/workspaces/${workspaceId}/agent-projects/${projectId}`,
  );
  if (!response.ok) throw new Error(`Agent Project read failed (${response.status})`);
  return (await response.json()) as AgentProjectView;
}
