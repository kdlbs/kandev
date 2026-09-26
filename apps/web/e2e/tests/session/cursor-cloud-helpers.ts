import type { BackendContext } from "../../fixtures/backend";
import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";

export type CursorCloudSessionStatus = {
  state: string;
  remote_state?: string;
  remote_branch?: string;
  remote_pull_request_url?: string;
  remote_history_gap?: boolean;
};

const GITHUB_OWNER = "mock-user";
const GITHUB_REPO = "cursor-cloud-e2e";

export async function createCursorCloudTask(
  apiClient: ApiClient,
  backend: BackendContext,
  seedData: SeedData,
  title: string,
  options: { workflowPrompt?: string } = {},
) {
  await apiClient.mockGitHubAddBranches(GITHUB_OWNER, GITHUB_REPO, [{ name: "main" }]);
  await apiClient.updateRepository(seedData.repositoryId, {
    provider: "github",
    provider_owner: GITHUB_OWNER,
    provider_name: GITHUB_REPO,
  });

  const secret = await apiClient.createSecret("Cursor Cloud E2E", "cursor-cloud-e2e-key");
  const executor = await apiClient.createExecutor("Cursor Cloud E2E", "cursor_cloud");
  const callbackUrl = `${backend.baseUrl}/api/v1/managed-agent-mcp`;
  const executorProfile = await apiClient.createExecutorProfile(executor.id, {
    name: "Cursor Cloud E2E profile",
    config: {
      cursor_cloud_api_key_secret_id: secret.id,
      cursor_cloud_callback_url: callbackUrl,
    },
    prepare_script: "",
    cleanup_script: "",
    env_vars: [],
  });

  const { agents: availableAgents } = await apiClient.listAvailableAgents();
  const cloudAgentType = availableAgents.find((agent) => agent.name === "cursor_cloud");
  if (!cloudAgentType) {
    throw new Error(
      `Cursor Cloud agent was not exposed after configuring its executor (available: ${availableAgents.map((agent) => agent.name).join(", ")})`,
    );
  }
  const { agents } = await apiClient.listAgents();
  const cloudAgent =
    agents.find((agent) => agent.name === "cursor_cloud") ??
    (await apiClient.createAgent("cursor_cloud"));
  const agentProfile = await apiClient.createAgentProfile(
    cloudAgent.id,
    "Cursor Cloud E2E profile",
    {
      model: "mock-cursor-model",
    },
  );

  let workflowId = seedData.workflowId;
  let workflowStepId = seedData.startStepId;
  let workflowWorkStepId: string | undefined;
  if (options.workflowPrompt) {
    const workflow = await apiClient.createWorkflow(seedData.workspaceId, `${title} workflow`);
    const inboxStep = await apiClient.createWorkflowStep(workflow.id, "Inbox", 0, {
      is_start_step: true,
    });
    const workStep = await apiClient.createWorkflowStep(workflow.id, "Remote Work", 1);
    await apiClient.updateWorkflowStep(workStep.id, {
      agent_profile_id: agentProfile.id,
      prompt: `${options.workflowPrompt}\n{{task_prompt}}`,
      events: { on_enter: [{ type: "auto_start_agent" }] },
    });
    workflowId = workflow.id;
    workflowStepId = inboxStep.id;
    workflowWorkStepId = workStep.id;
  }

  const task = await apiClient.createTaskWithAgent(seedData.workspaceId, title, agentProfile.id, {
    description: "Implement the requested remote change",
    workflow_id: workflowId,
    workflow_step_id: workflowStepId,
    repositories: [{ repository_id: seedData.repositoryId, base_branch: "main" }],
    executor_id: executor.id,
    executor_profile_id: executorProfile.id,
    ...(workflowWorkStepId ? { start_agent: false } : {}),
  });

  if (workflowWorkStepId) {
    await apiClient.moveTask(task.id, workflowId, workflowWorkStepId);
  }

  return { task, agentProfile, executor, executorProfile, secret };
}

export async function findCursorCloudSessionId(
  apiClient: ApiClient,
  taskId: string,
  executorProfileId: string,
): Promise<string> {
  const { sessions } = await apiClient.listTaskSessions(taskId);
  return sessions.find((session) => session.executor_profile_id === executorProfileId)?.id ?? "";
}

export async function disableCursorCloudExecutors(apiClient: ApiClient): Promise<void> {
  const { executors } = await apiClient.listExecutors();
  await Promise.all(
    executors
      .filter((executor) => executor.type === "cursor_cloud" && executor.status !== "disabled")
      .map((executor) => apiClient.updateExecutor(executor.id, { status: "disabled" })),
  );
}

export function getCursorCloudSessionStatus(
  apiClient: ApiClient,
  taskId: string,
  sessionId: string | undefined,
): Promise<CursorCloudSessionStatus> {
  if (!sessionId) throw new Error("Cursor Cloud task has no session ID");
  return apiClient.wsRequest<CursorCloudSessionStatus>("task.session.status", {
    task_id: taskId,
    session_id: sessionId,
  });
}

export const cursorCloudGitHubURL = `https://github.com/${GITHUB_OWNER}/${GITHUB_REPO}`;
