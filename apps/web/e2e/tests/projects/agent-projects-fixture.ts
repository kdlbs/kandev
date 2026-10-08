import type { ApiClient } from "../../helpers/api-client";
import { startHTTPGitFixture } from "../../helpers/http-git-server";
import { dwell } from "../../helpers/causal-waits";
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
  archived_at?: string | null;
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

export async function restoreArchivedAgentProject(
  apiClient: ApiClient,
  workspaceId: string,
  projectId: string,
  retainedNotes: string,
): Promise<number> {
  const deadline = Date.now() + 30_000;
  let cleanupRaceRetries = 0;
  while (Date.now() < deadline) {
    await assertArchivedProjectContext(apiClient, workspaceId, projectId, retainedNotes);
    const response = await apiClient.rawRequest(
      "POST",
      `/api/v1/workspaces/${workspaceId}/agent-projects/${projectId}/restore`,
    );
    if (response.status === 200) {
      await assertProjectContextContent(apiClient, workspaceId, projectId, retainedNotes);
      return cleanupRaceRetries;
    }

    const body = await response.text();
    if (response.status !== 500 || !body.includes("cleanup cancellation lost lifecycle race")) {
      throw new Error(`Project restore failed (${response.status}): ${body}`);
    }
    cleanupRaceRetries += 1;
    await dwell(
      500,
      "poll-interval",
      "archive cleanup cancellation guard may clear after cleanup settles",
    );
  }
  throw new Error(
    `Project restore did not clear its archive cleanup race after ${cleanupRaceRetries} retries`,
  );
}

async function assertArchivedProjectContext(
  apiClient: ApiClient,
  workspaceId: string,
  projectId: string,
  retainedNotes: string,
) {
  const archivedResponse = await apiClient.rawRequest(
    "GET",
    `/api/v1/workspaces/${workspaceId}/agent-projects?archived=true`,
  );
  if (!archivedResponse.ok) {
    throw new Error(`Archived project list failed (${archivedResponse.status})`);
  }
  const archived = (await archivedResponse.json()) as { projects: AgentProjectView[] };
  const project = archived.projects.find((candidate) => candidate.id === projectId);
  if (!project?.archived_at) {
    throw new Error("The project was not retained as archived during restore");
  }

  await assertProjectContextContent(apiClient, workspaceId, projectId, retainedNotes);
}

async function assertProjectContextContent(
  apiClient: ApiClient,
  workspaceId: string,
  projectId: string,
  retainedNotes: string,
) {
  const contextResponse = await apiClient.rawRequest(
    "GET",
    `/api/v1/workspaces/${workspaceId}/agent-projects/${projectId}/context/content?path=notes.md`,
  );
  if (!contextResponse.ok) {
    throw new Error(`Archived project context read failed (${contextResponse.status})`);
  }
  const context = (await contextResponse.json()) as { content: string };
  if (context.content !== retainedNotes) {
    throw new Error("Project context changed during archive restoration");
  }
}
